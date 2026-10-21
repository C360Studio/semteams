#!/usr/bin/env python3
"""Decider smoke: same entity-style System One questions against semselect (Kev on
Metal, /v1/systemone) and Mercury Decide (/v1/decisions). Stdlib only.

Usage:
  decider_smoke.py semselect [--url http://127.0.0.1:8084]
  decider_smoke.py mercury   (needs INCEPTION_API_KEY; hard token cap below)

Every raw request/response is written to fixtures/<backend>/<case>.json so the
responses can become offline contract-test fixtures. The Mercury key is read
from the environment only and never written anywhere.
"""
import json, os, sys, time, urllib.request, urllib.error, pathlib

HERE = pathlib.Path(__file__).resolve().parent
FIX = HERE / "fixtures"
LEDGER = HERE / "mercury_ledger.json"

# Budget: owner approved < $5 total for Mercury smoke runs. Mercury Decide lists
# $0.04 / 1M input tokens (output free). Cap the whole ledger far below that.
MERCURY_PRICE_PER_M = 0.04
MERCURY_LEDGER_TOKEN_CAP = 2_000_000   # = $0.08 at list price; 1.6% of the $5 approval
MERCURY_RUN_TOKEN_CAP = 50_000

TYPES = {
    "person": "A named human individual",
    "organization": "A company, foundation, agency, regulator or other group of people",
    "software": "A software product, library, framework, service or model",
    "repository": "A source-code repository, e.g. owner/name on GitHub",
    "specification": "A standard, protocol or formal specification",
    "event": "A release, incident, meeting or other dated occurrence",
    "concept": "An idea, technique, market or phenomenon that is not one of the above",
    "not_an_entity": "The actor name is not a nameable entity",
    "unsure": "The text does not allow a confident answer",
}

CASES = [
    {"id": "c1-software", "name": "NATS JetStream", "role": "persistence layer for agent loop state",
     "quote": "SemStreams stores agent loop state in NATS JetStream key-value buckets.",
     "expect": {"type": "software", "support": True}},
    {"id": "c2-person", "name": "Derek Collison", "role": "creator of NATS",
     "quote": "NATS was created by Derek Collison, who later founded Synadia to steward the project.",
     "expect": {"type": "person", "support": True}},
    {"id": "c3-org", "name": "Synadia", "role": "company that stewards NATS",
     "quote": "NATS was created by Derek Collison, who later founded Synadia to steward the project.",
     "expect": {"type": "organization", "support": True}},
    {"id": "c4-unsupported", "name": "Kubernetes", "role": "graph database used by SemEngine",
     "quote": "SemEngine runs on NATS JetStream and needs no external database.",
     "expect": {"type": "software", "support": False}},
    {"id": "c5-unicode", "name": "G\u00f6del-SDK", "role": "robotics SDK released by Z\u00fcrich Instruments",
     "quote": "Z\u00fcrich Instruments \U0001F680 released G\u00f6del-SDK v2.1 for caf\u00e9 robotics teams.",
     "expect": {"type": "software", "support": True}},
    {"id": "c6-ambiguous", "name": "Mercury", "role": "decision model from Inception",
     "quote": "Inception's Mercury Decide model returns typed decisions without generating text.",
     "expect": {"type": "software", "support": True}},
]


STATE_MODE = os.environ.get("STATE_MODE", "quote")  # "quote": evidence only; "claim": actor+role+quote (v1, confounded)


def state_of(c):
    if STATE_MODE == "claim":
        return f"Actor: {c['name']}\nRole: {c['role']}\nQuote: {c['quote']}"
    return c["quote"]


def questions(c, with_extract):
    q = {
        "type": {"type": "choice",
                 "instructions": f"What kind of entity is the actor \"{c['name']}\"? Use only the supplied text. Choose unsure if the text does not settle it.",
                 "criteria": TYPES},
        "support": {"type": "noul",
                    "instructions": f"Does the quote, on its own, support that \"{c['name']}\" is {c['role']}?"},
        "centrality": {"type": "score",
                       "instructions": f"How central is \"{c['name']}\" to what the quote says?",
                       "criteria": ["Not mentioned", "Mentioned in passing", "The main subject"]},
    }
    if with_extract:
        q["mentions"] = {"type": "extract",
                         "instructions": "Mark named entities in the text.",
                         "criteria": {"person": "Names of people",
                                      "organization": "Names of companies or organizations",
                                      "software": "Names of software products, libraries, services or models"}}
    return q


def post(url, body, headers, timeout=120):
    data = json.dumps(body).encode()
    req = urllib.request.Request(url, data=data, method="POST",
                                 headers={"Content-Type": "application/json", **headers})
    t0 = time.monotonic()
    try:
        with urllib.request.urlopen(req, timeout=timeout) as r:
            status, raw = r.status, r.read()
    except urllib.error.HTTPError as e:
        status, raw = e.code, e.read()
    ms = round((time.monotonic() - t0) * 1000)
    try:
        parsed = json.loads(raw)
    except Exception:
        parsed = {"_raw": raw.decode(errors="replace")[:2000]}
    return status, parsed, ms


def check_spans(state, answer):
    """Verify extract spans against code-point, UTF-16 and UTF-8 byte slicing."""
    out = []
    for field, spans in (answer.get("spans") or {}).items():
        for s in spans:
            st, en, txt = s.get("start"), s.get("end"), s.get("text")
            cp = state[st:en] == txt
            u16 = state.encode("utf-16-le")[2 * st:2 * en].decode("utf-16-le", "replace") == txt
            b8 = state.encode()[st:en].decode("utf-8", "replace") == txt
            out.append({"field": field, "text": txt, "start": st, "end": en,
                        "conf": s.get("confidence"), "codepoint_ok": cp, "utf16_ok": u16, "byte_ok": b8})
    return out


def summarize(c, resp):
    a = resp.get("answers", {}) if isinstance(resp, dict) else {}
    t = a.get("type", {})
    s = a.get("support", {})
    sc = a.get("centrality", {})
    pyes = s.get("noul", s.get("probability", s.get("p_yes")))
    return {
        "case": c["id"],
        "type": t.get("choice"), "type_conf": t.get("confidence"),
        "type_ok": t.get("choice") == c["expect"]["type"],
        "p_support": pyes,
        "support_ok": None if pyes is None else ((pyes >= 0.5) == c["expect"]["support"]),
        "centrality": sc.get("score"),
    }


def load_ledger():
    if LEDGER.exists():
        return json.loads(LEDGER.read_text())
    return {"input_tokens": 0, "requests": 0, "runs": []}


def main():
    backend = sys.argv[1] if len(sys.argv) > 1 else "semselect"
    (FIX / backend / STATE_MODE).mkdir(parents=True, exist_ok=True)
    results, spans_report = [], []
    if backend == "semselect":
        url = (sys.argv[3] if len(sys.argv) > 3 and sys.argv[2] == "--url" else "http://127.0.0.1:8084") + "/v1/systemone"
        headers = {}
        with_extract = False
    elif backend == "mercury":
        key = os.environ.get("INCEPTION_API_KEY")
        if not key:
            sys.exit("INCEPTION_API_KEY is not set")
        url = "https://api.inceptionlabs.ai/v1/decisions"
        headers = {"Authorization": f"Bearer {key}"}
        with_extract = True
        ledger = load_ledger()
        run_tokens = 0
    else:
        sys.exit("backend must be semselect or mercury")

    for c in CASES:
        body = {"state": state_of(c), "questions": questions(c, with_extract)}
        if backend == "mercury":
            if ledger["input_tokens"] >= MERCURY_LEDGER_TOKEN_CAP or run_tokens >= MERCURY_RUN_TOKEN_CAP:
                print("token cap reached; stopping"); break
            body["model"] = "mercury-decide"
        status, resp, ms = post(url, body, headers)
        (FIX / backend / STATE_MODE / f"{c['id']}.json").write_text(json.dumps(
            {"request": body, "status": status, "latency_ms": ms, "response": resp}, indent=2, ensure_ascii=False))
        row = {"status": status, "ms": ms}
        if status == 200:
            row.update(summarize(c, resp))
            if with_extract and "mentions" in resp.get("answers", {}):
                for r in check_spans(body["state"], resp["answers"]["mentions"]):
                    spans_report.append({"case": c["id"], **r})
        else:
            row["error"] = resp
        if backend == "mercury":
            usage = resp.get("usage") if isinstance(resp, dict) else None
            if status == 200 and (not usage or "input_tokens" not in usage):
                print("missing usage; stopping (budget guard)"); results.append(row); break
            used = (usage or {}).get("input_tokens", 0)
            run_tokens += used
            ledger["input_tokens"] += used
            ledger["requests"] += 1
            row["input_tokens"] = used
        results.append(row)
        print(json.dumps(row, ensure_ascii=False))

    if backend == "mercury":
        ledger["runs"].append({"at": time.strftime("%Y-%m-%dT%H:%M:%S%z"), "input_tokens": run_tokens})
        LEDGER.write_text(json.dumps(ledger, indent=2))
        print(f"run input tokens={run_tokens} ledger total={ledger['input_tokens']} "
              f"est cost=${ledger['input_tokens'] * MERCURY_PRICE_PER_M / 1e6:.6f} (list price)")
    if spans_report:
        print("SPANS")
        for s in spans_report:
            print(json.dumps(s, ensure_ascii=False))
    (HERE / f"summary_{backend}_{STATE_MODE}.json").write_text(json.dumps(
        {"results": results, "spans": spans_report}, indent=2, ensure_ascii=False))


if __name__ == "__main__":
    main()
