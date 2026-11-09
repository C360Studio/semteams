"""Keep browser qualification sensitive to changes in product-owned Go packages."""
import importlib.util
from pathlib import Path
import tempfile
import unittest

MODULE = importlib.util.spec_from_file_location(
    "qualify_migration_browser", Path(__file__).with_name("qualify-migration-browser.py")
)
runner = importlib.util.module_from_spec(MODULE)
MODULE.loader.exec_module(runner)


class SourceSnapshotTest(unittest.TestCase):
    def test_internal_runtime_change_invalidates_source_identity(self):
        with tempfile.TemporaryDirectory() as directory:
            source = Path(directory)
            for name in ("go.mod", "go.sum", "ui/package-lock.json", "internal/runtimecatalog/register.go"):
                path = source / name
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text("before\n")
            before = runner.source_snapshot(source)
            (source / "internal/runtimecatalog/register.go").write_text("after\n")
            after = runner.source_snapshot(source)
            self.assertNotEqual(runner.tree_sha(before), runner.tree_sha(after))
            self.assertIn("internal/runtimecatalog/register.go", before)

    def test_approval_pause_and_resume_derive_advertised_bash_gate(self):
        base_config = {
            "components": {
                "rule": {"config": {"rules_files": [], "inline_rules": []}},
                "agentic-tools": {"config": {"approval_required": ["create_rule"]}},
            }
        }
        compose_config = {
            "services": {
                "mock-llm": {"environment": {}},
                "backend": {"environment": {}},
            }
        }
        env = {}
        for name in ("approval-pause", "approval-resume"):
            derived = runner.derive_scenario_config(
                {"name": name, "fixture": f"{name}.yaml"},
                base_config,
                compose_config,
                env,
            )
            self.assertEqual(
                derived["components"]["agentic-tools"]["config"]["approval_required"],
                ["create_rule", "bash"],
            )
        self.assertEqual(
            base_config["components"]["agentic-tools"]["config"]["approval_required"],
            ["create_rule"],
        )
        research = runner.derive_scenario_config(
            {"name": "research-mvp", "fixture": "research-mvp.yaml"},
            base_config,
            compose_config,
            env,
        )
        self.assertEqual(
            research["components"]["agentic-tools"]["config"]["approval_required"],
            ["create_rule"],
        )

    def test_wait_for_dispatch_and_loops_waits_past_cold_start_503(self):
        loops_statuses = iter([503, 503, 200])
        calls = []

        def fake_read_http(path):
            calls.append(path)
            if path == "/teams-dispatch/commands":
                return {"status": 200}
            if path == "/teams-dispatch/loops":
                return {"status": next(loops_statuses)}
            raise AssertionError(f"unexpected path: {path}")

        self.assertTrue(
            runner.wait_for_dispatch_and_loops(fake_read_http, attempts=5, sleep=lambda _: None)
        )
        self.assertEqual(calls.count("/teams-dispatch/loops"), 3)
        self.assertFalse(
            runner.wait_for_dispatch_and_loops(
                lambda path: {"status": 200 if path == "/teams-dispatch/commands" else 503},
                attempts=3,
                sleep=lambda _: None,
            )
        )


if __name__ == "__main__":
    unittest.main()
