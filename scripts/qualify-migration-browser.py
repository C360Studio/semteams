#!/usr/bin/env python3
"""Run a mock-only matrix with pre-resolved, exclusively owned Compose resources.

Each scenario starts fresh volumes, records config/fixture/source/image identities,
actively samples state and logs, runs Playwright without retries, and removes only
its scoped stack. A source mutation during a scenario invalidates its result.
"""
import argparse
import copy
import hashlib
import json
import os
from pathlib import Path
import subprocess
import time
import urllib.error
import urllib.request


def save(path, value):
    path.write_text(json.dumps(value, indent=2) + "\n")


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def source_snapshot(source):
    paths = [source / name for name in ("go.mod", "go.sum", "ui/package-lock.json")]
    for directory in ("cmd", "configs", "ui/src", "ui/e2e", "test/fixtures/journeys"):
        paths.extend(path for path in (source / directory).rglob("*") if path.is_file())
    return {str(path.relative_to(source)): sha(path) for path in sorted(paths)}


def tree_sha(snapshot, prefix=""):
    return hashlib.sha256("".join(
        f"{name} {digest}\n" for name, digest in snapshot.items() if name.startswith(prefix)
    ).encode()).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for field in ("source", "compose", "project", "output", "names"):
        parser.add_argument("--" + field, required=True)
    parser.add_argument("--port", type=int, required=True)
    parser.add_argument("--matrix", default=str(Path(__file__).with_name("semstreams-migration-matrix.json")))
    args = parser.parse_args()
    source = Path(args.source).resolve()
    output = Path(args.output).resolve()
    output.mkdir(parents=True, exist_ok=True)
    base = json.loads(Path(args.compose).read_text())
    all_scenarios = json.loads(Path(args.matrix).read_text())
    requested = set(args.names.split(","))
    unknown = requested - {scenario["name"] for scenario in all_scenarios}
    if unknown:
        parser.error(f"Unknown scenarios: {sorted(unknown)}")
    scenarios = [scenario for scenario in all_scenarios if scenario["name"] in requested]
    if not args.project.startswith("stmig") or base.get("name") != args.project:
        parser.error("Use an exclusively owned stmig-prefixed project matching the resolved Compose name")
    if any("container_name" in service for service in base["services"].values()):
        parser.error("Resolved Compose must remove all fixed container names")
    for resource in ("volumes", "networks"):
        if any(not value.get("name", "").startswith(args.project + "_")
               for value in base.get(resource, {}).values()):
            parser.error(f"Every {resource} name must belong exclusively to the migration project")
    if any(base["services"]["backend"]["environment"].get(key)
           for key in ("GEMINI_API_KEY", "ANTHROPIC_API_KEY", "BRAVE_SEARCH_API_KEY")):
        parser.error("Paid-model/search credentials must be blank")
    env = {**os.environ, "E2E_AGENTIC_UI_PORT": str(args.port), "GEMINI_API_KEY": "",
           "ANTHROPIC_API_KEY": "", "BRAVE_SEARCH_API_KEY": "", "BACKEND_BRAVE_SEARCH_API_KEY": ""}
    env.pop("CI", None)  # Existing local Playwright contract: zero retries.
    initial = source_snapshot(source)
    save(output / "source-before.json", initial)
    summary = []

    def read_http(path):
        try:
            with urllib.request.urlopen(f"http://127.0.0.1:{args.port}{path}", timeout=4) as response:
                body = response.read().decode()
                try:
                    body = json.loads(body)
                except ValueError:
                    pass
                return {"status": response.status, "body": body}
        except urllib.error.HTTPError as error:
            return {"status": error.code, "body": error.read().decode()}
        except Exception as error:
            return {"error": str(error)}

    for scenario in scenarios:
        directory = output / scenario["name"]
        directory.mkdir(exist_ok=True)
        compose_config = copy.deepcopy(base)
        compose_config["services"]["mock-llm"]["environment"]["FIXTURE"] = scenario["fixture"]
        compose_config["services"]["backend"]["environment"]["SEMSTREAMS_PERSONA_OVERLAY_PATH"] = ""
        config = json.loads((source / "configs/e2e-flow-bootstrap.json").read_text())
        name = scenario["name"]
        if name == "coordinator-routing-matrix":
            config["components"]["rule"]["config"]["rules_files"] = [
                "/app/configs/rules/coordinator/03b-respond-direct.json",
                "/app/configs/rules/coordinator/03-ask-user.json"]
            config["components"]["rule"]["config"]["inline_rules"] = []
        elif name == "coordinator-team-spawn":
            config["components"]["rule"]["config"]["rules_files"] = [
                "/app/configs/rules/research/01-coordinator-research-spawn.json",
                "/app/configs/rules/autoresearch/01-coordinator-autoresearch-spawn.json"]
            config["components"]["rule"]["config"]["inline_rules"] = []
        elif name == "clarification-autonomous":
            config["components"]["agentic-tools"]["config"]["restricted_decide_actions"] = ["ask_user"]
            compose_config["services"]["backend"]["environment"]["SEMSTREAMS_PERSONA_OVERLAY_PATH"] = "configs/personas/fragments-autonomous"
        if name == "run-approval-boundary":
            config["components"]["agentic-tools"]["config"]["approval_required"] = ["emit_plan"]
        env.pop("APPROVAL_BOUNDARY_ACTION", None)
        if name.startswith("approval-boundary-"):
            config["components"]["agentic-tools"]["config"]["approval_required"] = ["request_sandbox"]
            env["APPROVAL_BOUNDARY_ACTION"] = scenario["action"]
        config_path = directory / "config.json"
        save(config_path, config)
        for volume in compose_config["services"]["backend"]["volumes"]:
            if volume["target"] == "/etc/semstreams/config.json":
                volume["source"] = str(config_path)
        compose_path = directory / "compose.json"
        save(compose_path, compose_config)
        compose = ["docker", "compose", "-p", args.project, "-f", str(compose_path)]
        before = source_snapshot(source)
        manifest = {**scenario, "source": str(source), "project": args.project, "port": args.port,
                    "runner_sha256": sha(Path(__file__)), "matrix_sha256": sha(Path(args.matrix)),
                    "source_tree_sha256": tree_sha(before), "ui_source_tree_sha256": tree_sha(before, "ui/src/"),
                    "derived_config_sha256": sha(config_path), "compose_sha256": sha(compose_path),
                    "sha256": {path: before[path] for path in ("go.mod", "go.sum", "ui/package-lock.json",
                        "configs/e2e-flow-bootstrap.json", "test/fixtures/journeys/" + scenario["fixture"],
                        "ui/e2e/agentic/" + scenario["spec"])}}
        images = sorted({service["image"] for service in compose_config["services"].values() if "image" in service})
        inspected = subprocess.run(["docker", "image", "inspect", *images], capture_output=True, text=True)
        if inspected.returncode == 0:
            manifest["images"] = [{key: image.get(key) for key in ("Id", "RepoTags", "RepoDigests")}
                                  for image in json.loads(inspected.stdout)]
        else:
            manifest["image_inspection_error"] = inspected.stderr
        save(directory / "manifest.json", manifest)

        def readiness():
            # Caddy does not forward /readyz; the UI URL is a Svelte 404.
            try:
                result = subprocess.run(compose + ["exec", "-T", "backend", "wget", "-T", "4", "-S", "-O", "-",
                                         "http://localhost:8080/readyz"], capture_output=True, text=True, timeout=10)
                return {"exit": result.returncode, "stdout": result.stdout, "stderr": result.stderr}
            except subprocess.TimeoutExpired:
                return {"error": "Backend readiness command timed out after 10 seconds"}

        def sample():
            return {"timestamp": time.time(), "loops": read_http("/teams-dispatch/loops"),
                    "backend_readiness": readiness(), "messages": read_http("/message-logger/entries?limit=10")}

        def run(command, log, timeout):
            with log.open("w") as stream:
                process = subprocess.Popen(command, cwd=source / "ui", env=env, stdout=stream, stderr=subprocess.STDOUT)
                start, last_sample = time.monotonic(), 0
                while process.poll() is None:
                    now = time.monotonic()
                    if now - last_sample >= 25:
                        last_sample = now
                        stamp = str(int(time.time()))
                        state = sample()
                        save(directory / f"{log.stem}-state-{stamp}.json", state)
                        with (directory / f"{log.stem}-backend-{stamp}.log").open("w") as backend_log:
                            subprocess.run(compose + ["logs", "--since=30s", "backend"], stdout=backend_log,
                                           stderr=subprocess.STDOUT, timeout=15)
                        loops = state["loops"]
                        if isinstance(loops.get("body"), list):
                            loops = [{key: loop.get(key) for key in ("loop_id", "role", "state")}
                                     for loop in loops["body"]]
                        print(json.dumps({"scenario": name, "command": command[-1],
                                          "elapsed": int(now - start), "loops": loops}), flush=True)
                    if now - start > timeout:
                        process.terminate()
                        process.wait(timeout=10)
                        return 124
                    time.sleep(1)
                return process.returncode

        skipped = "test.describe.skip(" in (source / "ui/e2e/agentic" / scenario["spec"]).read_text()
        result = 0
        try:
            if not skipped:
                result = run(compose + ["up", "-d", "--wait", "--wait-timeout", "120"], directory / "up.log", 150)
                if result == 0:
                    for _ in range(30):
                        if read_http("/teams-dispatch/commands").get("status") == 200:
                            break
                        time.sleep(1)
            if result == 0:
                env.update(FIXTURE=scenario["fixture"], AGENTIC_CONFIG=scenario["config"])
                result = run(["npx", "playwright", "test", "e2e/agentic/" + scenario["spec"],
                              "--retries=0", "--reporter=list,json", "--output=" + str(directory / "test-results")],
                             directory / "playwright.log", 260)
        finally:
            if not skipped:
                state = sample()
                state["messages"] = read_http("/message-logger/entries?limit=500")
                state["run_phases"] = read_http("/graph/triples?predicate=agent.run.phase&limit=500")
                state["run_approval"] = {field: read_http("/graph/triples?predicate=agent.run.approval-" + field + "&limit=500")
                                         for field in ("pending", "answered", "outstanding")}
                save(directory / "final-state.json", state)
                with (directory / "backend.log").open("w") as stream:
                    subprocess.run(compose + ["logs", "backend", "mock-llm"], stdout=stream,
                                   stderr=subprocess.STDOUT, timeout=30)
                with (directory / "down.log").open("w") as stream:
                    subprocess.run(compose + ["down", "-v", "--remove-orphans"], stdout=stream,
                                   stderr=subprocess.STDOUT, timeout=60, check=True)
        after = source_snapshot(source)
        manifest.update(source_after_tree_sha256=tree_sha(after), ui_source_after_tree_sha256=tree_sha(after, "ui/src/"),
                        source_changed=before != after)
        save(directory / "manifest.json", manifest)
        row = {"name": name, "exit": 125 if before != after else result, "playwright_exit": result,
               "skipped": skipped, "source_changed": before != after}
        summary.append(row)
        save(output / "summary.json", summary)
        print(json.dumps(row), flush=True)
    final = source_snapshot(source)
    save(output / "source-after.json", final)
    save(output / "source-comparison.json", {"before": tree_sha(initial), "after": tree_sha(final),
                                             "unchanged": initial == final})
    return int(initial != final or any(row["exit"] for row in summary))


if __name__ == "__main__":
    raise SystemExit(main())
