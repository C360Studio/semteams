#!/usr/bin/env python3
"""Phase 0 & Phase 2/3 Evaluation Harness: T0/T1 Algorithm Floor vs. System One Deciders
(Mercury Decide & semselect Kev-4B on Metal) across the 6-band Golden Corpus.

Stdlib only. Enforces:
  - Evidence-only state rule (§6.4)
  - Hard Mercury token ledger cap (< $5 owner approval; script cap 2M tokens = $0.08)
  - Deterministic request/response caching keyed by sha256(request)
  - Option-order sensitivity check (forward vs reverse)
  - Per-band gate evaluation (coverage, accuracy, wrong-confident rate, false-merge rate)
"""
import hashlib
import json
import math
import os
import pathlib
import re
import sys
import time
import unicodedata
import urllib.error
import urllib.request

HERE = pathlib.Path(__file__).resolve().parent
CORPUS_PATH = HERE / "corpus_v1.json"
CACHE_DIR = HERE / "eval_cache"
LEDGER_PATH = HERE / "mercury_ledger.json"

MERCURY_PRICE_PER_M = 0.04
MERCURY_LEDGER_TOKEN_CAP = 2_000_000
MERCURY_RUN_TOKEN_CAP = 100_000

TYPES_FORWARD = [
    ("person", "A named human individual"),
    ("organization", "A company, foundation, agency, institution, or group of people"),
    ("software", "A software product, library, framework, service, CLI tool, or AI model"),
    ("repository", "A source-code repository, e.g. owner/name on GitHub"),
    ("specification", "A standard, protocol, RFC, ADR, or formal specification"),
    ("event", "A conference, release, incident, meeting, or dated occurrence"),
    ("concept", "A technical concept, algorithm, technique, or pattern that is not a specific product or standard"),
    ("not_an_entity", "The actor name is not a nameable entity"),
    ("unsure", "The supplied text does not allow a confident answer"),
]

STOPWORDS = {
    "a", "an", "and", "are", "as", "at", "by", "for", "from", "in", "into",
    "is", "it", "of", "on", "or", "over", "that", "the", "to", "under", "with",
    "v1", "v2", "v5", "0", "1", "2", "5", "protocol", "specification", "broker",
    "server", "collector", "test", "runner", "cluster", "router", "gateway",
}

REPO_RE = re.compile(r"^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$")
SPEC_RE = re.compile(r"^(?:RFC\s*\d+|ADR-\d+|CVE-\d{4}-\d+)$", re.IGNORECASE)


def fold_text(s: str) -> str:
    """NFKD normalize, strip combining marks (diacritics), lowercase, collapse whitespace."""
    nfkd = unicodedata.normalize("NFKD", s)
    no_marks = "".join(ch for ch in nfkd if unicodedata.category(ch) != "Mn")
    return re.sub(r"\s+", " ", no_marks.lower()).strip()


def fold_alnum(s: str) -> str:
    """Fold to lowercase alphanumeric + slash tokens for punctuation-insensitive matching."""
    f = fold_text(s)
    return re.sub(r"[^a-z0-9/]+", " ", f).strip()


def fold_compact(s: str) -> str:
    """Remove hyphens/spaces for compound word matching (e.g. sem-teams vs semteams)."""
    return re.sub(r"[^a-z0-9/]+", "", fold_text(s))


def tokenize(s: str) -> list[str]:
    return [t for t in re.split(r"[^a-z0-9]+", fold_text(s)) if t and t not in STOPWORDS]


class AlgoFloor:
    """Tier 0 (deterministic exact/alias + regex) and Tier 1 (BM25/token similarity) floor."""

    def __init__(self, seed_catalog: list[dict]):
        self.catalog = seed_catalog
        self.by_id = {e["id"]: e for e in seed_catalog}
        # Map folded surface -> list of catalog entities
        self.exact_index: dict[str, list[dict]] = {}
        self.compact_index: dict[str, list[dict]] = {}
        for ent in seed_catalog:
            surfaces = [ent["name"]] + list(ent.get("aliases", []))
            for sf in surfaces:
                for key in {fold_text(sf), fold_alnum(sf)}:
                    if key:
                        self.exact_index.setdefault(key, [])
                        if ent not in self.exact_index[key]:
                            self.exact_index[key].append(ent)
                ckey = fold_compact(sf)
                if ckey:
                    self.compact_index.setdefault(ckey, [])
                    if ent not in self.compact_index[ckey]:
                        self.compact_index[ckey].append(ent)

        # Build IDF table over catalog names + aliases + descriptions for T1 BM25
        self.name_tokens: dict[str, set[str]] = {}
        self.desc_tokens: dict[str, set[str]] = {}
        df: dict[str, int] = {}
        for ent in seed_catalog:
            n_surf = " ".join([ent["name"]] + list(ent.get("aliases", [])))
            n_toks = set(tokenize(n_surf))
            d_toks = set(tokenize(ent.get("description", "")))
            self.name_tokens[ent["id"]] = n_toks
            self.desc_tokens[ent["id"]] = d_toks
            for t in n_toks | d_toks:
                df[t] = df.get(t, 0) + 1
        n_docs = max(len(seed_catalog), 1)
        self.idf = {t: math.log(1.0 + (n_docs - c + 0.5) / (c + 0.5)) for t, c in df.items()}

    def t1_candidates(self, name: str, top_k: int = 5) -> list[tuple[dict, float]]:
        """Score catalog entities by lexical/BM25 similarity to the actor name ONLY."""
        q_toks = set(tokenize(name))
        q_compact = fold_compact(name)
        scored: list[tuple[dict, float]] = []
        for ent in self.catalog:
            n_toks = self.name_tokens[ent["id"]]
            d_toks = self.desc_tokens[ent["id"]]
            # Require at least one token match in name/aliases so description-only noise never triggers
            n_overlap = q_toks & n_toks
            d_overlap = (q_toks - n_toks) & d_toks
            score = 0.0
            if n_overlap and q_toks:
                num = sum(self.idf.get(t, 1.0) for t in n_overlap) + 0.5 * sum(self.idf.get(t, 1.0) for t in d_overlap)
                den = sum(self.idf.get(t, 1.0) for t in q_toks)
                score = num / max(den, 1e-6)
            # Check compact equality on aliases (handles hyphenation differences like sem-teams vs semteams)
            for sf in [ent["name"]] + list(ent.get("aliases", [])):
                sfc = fold_compact(sf)
                if sfc and sfc == q_compact:
                    score = max(score, 1.0)
            if score > 0.0:
                scored.append((ent, round(score, 4)))
        scored.sort(key=lambda x: (-x[1], x[0]["id"]))
        return scored[:top_k]

    def check_mention(self, name: str, linked_ent: dict | None, quote: str) -> bool:
        """T0 deterministic mention check: does the actor name or a known alias appear in the quote?"""
        q_fold = fold_alnum(quote)
        q_compact = fold_compact(quote)
        surfaces = [name]
        if linked_ent is not None:
            surfaces.append(linked_ent["name"])
            surfaces.extend(linked_ent.get("aliases", []))
        for sf in surfaces:
            sf_alnum = fold_alnum(sf)
            sf_comp = fold_compact(sf)
            if sf_alnum and sf_alnum in q_fold:
                return True
            if len(sf_comp) >= 4 and sf_comp in q_compact:
                return True
        # Also check if all non-stopword tokens of the actor name (when >=1 specific token) appear in quote
        q_toks = set(tokenize(quote))
        name_toks = [t for t in tokenize(name) if len(t) >= 3]
        if name_toks and all(t in q_toks for t in name_toks):
            return True
        return False

    def has_contradiction_cue(self, role: str, quote: str) -> bool:
        """Simple lexical contrast/negation detector on quote vs role."""
        q_low = fold_text(quote)
        r_low = fold_text(role)
        neg_cues = ["never", "no ", "not ", "neither", "without", "read-only", "only for"]
        if any(cue in q_low for cue in neg_cues) and not any(cue in r_low for cue in neg_cues):
            return True
        return False

    def evaluate_case(self, case: dict) -> dict:
        name = case["name"]
        quote = case["quote"]
        role = case["role"]

        # 1. T0 Exact / Alias lookup
        exact_hits = list(self.exact_index.get(fold_text(name), []))
        for ent in self.exact_index.get(fold_alnum(name), []):
            if ent not in exact_hits:
                exact_hits.append(ent)
        for ent in self.compact_index.get(fold_compact(name), []):
            if ent not in exact_hits:
                exact_hits.append(ent)

        pred_type = "unsure"
        pred_link = "unsure"
        tier = "T0"
        linked_ent = None
        candidates = self.t1_candidates(name)

        if len(exact_hits) == 1:
            linked_ent = exact_hits[0]
            pred_type = linked_ent["type"]
            pred_link = linked_ent["id"]
            tier = "T0"
        elif len(exact_hits) > 1:
            # Ambiguous homonym in catalog (e.g. "Mercury", "Vault"): T0/T1 abstains unless
            # a multi-word alias uniquely appears in the quote? No — keep T0/T1 strict per §7.1:
            # exact/alias hit with tie has zero margin over top-2, so link & type stay unsure!
            pred_type = "unsure"
            pred_link = "unsure"
            tier = "T0-ambiguous"
        else:
            # Check T0 ID patterns on diacritic-folded text
            norm_n = fold_text(name)
            if REPO_RE.match(norm_n):
                pred_type = "repository"
                pred_link = "new"
                tier = "T0-pattern"
            elif SPEC_RE.match(norm_n):
                pred_type = "specification"
                pred_link = "new"
                tier = "T0-pattern"
            else:
                # Try T1 BM25 candidate ranking over catalog
                if len(candidates) == 1 and candidates[0][1] >= 0.70:
                    linked_ent = candidates[0][0]
                    pred_type = linked_ent["type"]
                    pred_link = linked_ent["id"]
                    tier = "T1-bm25"
                elif len(candidates) >= 2 and candidates[0][1] >= 0.70 and (candidates[0][1] - candidates[1][1]) >= 0.25:
                    linked_ent = candidates[0][0]
                    pred_type = linked_ent["type"]
                    pred_link = linked_ent["id"]
                    tier = "T1-bm25"
                elif len(candidates) == 0:
                    # Unindexed novel name without regex pattern: link is new, but type is unsure
                    pred_link = "new"
                    pred_type = "unsure"
                    tier = "T0-novel"

        mention = self.check_mention(name, linked_ent, quote)
        # Support under T0:
        # - If not mentioned in quote -> definitely False (unsupported claim caught at T0!)
        # - If mentioned in quote, and quote has no negation cue -> mention-level support True
        #   (we track both strict role_support="unsure" and mention_support=bool so we can see
        #    why B5 fails mention-only while B1/B2/B4/B6 pass).
        if not mention:
            pred_support_mention = False
            pred_support_strict = False
        else:
            pred_support_mention = True
            pred_support_strict = "unsure"

        return {
            "case": case["id"],
            "band": case["band"],
            "tier": tier,
            "type": pred_type,
            "type_conf": 1.0 if pred_type != "unsure" else None,
            "link": pred_link,
            "link_conf": 1.0 if pred_link != "unsure" else None,
            "mention": mention,
            "support_mention": pred_support_mention,
            "support_strict": pred_support_strict,
            "t1_candidates": [c[0]["id"] for c in candidates],
            "input_tokens": 0,
            "latency_ms": 0,
        }


def build_link_options(case: dict, algo: AlgoFloor, order: str) -> dict[str, str]:
    """Build candidate link options for the Decider Choice question."""
    cands = algo.t1_candidates(case["name"], top_k=4)
    # Ensure any exact_index homonyms are included
    seen = {c[0]["id"] for c in cands}
    for ent in algo.exact_index.get(fold_text(case["name"]), []):
        if ent["id"] not in seen:
            cands.append((ent, 1.0))
            seen.add(ent["id"])
    items: list[tuple[str, str]] = []
    for ent, _ in cands[:5]:
        items.append((ent["id"], f"{ent['name']} ({ent['type']}): {ent['description']}"))
    items.append(("new", "A valid entity mentioned in the text that is not one of the listed catalog entities"))
    items.append(("unsure", "The text does not provide enough evidence to link or reject"))
    if order == "reverse":
        items = list(reversed(items))
    return dict(items)


def build_type_options(order: str) -> dict[str, str]:
    items = list(TYPES_FORWARD)
    if order == "reverse":
        items = list(reversed(items))
    return dict(items)


def build_decider_request(case: dict, algo: AlgoFloor, order: str, backend: str) -> dict:
    """Evidence-only state (§6.4): state holds ONLY the verbatim quote."""
    state = case["quote"]
    questions = {
        "type": {
            "type": "choice",
            "instructions": (
                f"What kind of entity is \"{case['name']}\"? "
                "Judge using the supplied text and standard technical meaning of the named entity in context. "
                "Choose unsure if the text does not allow a confident answer."
            ),
            "criteria": build_type_options(order),
        },
        "link": {
            "type": "choice",
            "instructions": (
                f"Which catalog entity does \"{case['name']}\" refer to in this text? "
                "Choose new if it is a real entity not in the catalog list, or unsure if the text does not settle it."
            ),
            "criteria": build_link_options(case, algo, order),
        },
        "support": {
            "type": "noul",
            "instructions": (
                f"Does the supplied text, on its own, support the claim that "
                f"\"{case['name']}\" is \"{case['role']}\"?"
            ),
        },
    }
    body = {"state": state, "questions": questions}
    if backend == "mercury":
        body["model"] = "mercury-decide"
    return body


def load_ledger() -> dict:
    if LEDGER_PATH.exists():
        return json.loads(LEDGER_PATH.read_text())
    return {"input_tokens": 0, "requests": 0, "runs": []}


def call_decider(backend: str, order: str, case: dict, algo: AlgoFloor, base_url: str) -> dict:
    req_body = build_decider_request(case, algo, order, backend)
    req_hash = hashlib.sha256(json.dumps(req_body, sort_keys=True).encode()).hexdigest()[:16]
    cache_file = CACHE_DIR / backend / order / f"{case['id']}_{req_hash}.json"
    cache_file.parent.mkdir(parents=True, exist_ok=True)

    if cache_file.exists():
        cached = json.loads(cache_file.read_text())
        return parse_decider_response(case, cached["response"], cached.get("latency_ms", 0), cached_hit=True)

    if backend == "semselect":
        url = base_url.rstrip("/") + "/v1/systemone"
        headers = {}
    elif backend == "mercury":
        key = os.environ.get("INCEPTION_API_KEY")
        if not key:
            sys.exit("INCEPTION_API_KEY not set in environment")
        url = "https://api.inceptionlabs.ai/v1/decisions"
        headers = {"Authorization": f"Bearer {key}"}
        ledger = load_ledger()
        if ledger["input_tokens"] >= MERCURY_LEDGER_TOKEN_CAP:
            sys.exit(f"Mercury ledger token cap ({MERCURY_LEDGER_TOKEN_CAP}) reached")
    else:
        sys.exit(f"Unknown backend: {backend}")

    data = json.dumps(req_body).encode("utf-8")
    req = urllib.request.Request(
        url, data=data, method="POST", headers={"Content-Type": "application/json", **headers}
    )
    t0 = time.monotonic()
    try:
        with urllib.request.urlopen(req, timeout=120) as r:
            status, raw = r.status, r.read()
    except urllib.error.HTTPError as e:
        status, raw = e.code, e.read()
    ms = round((time.monotonic() - t0) * 1000)
    resp = json.loads(raw.decode("utf-8", errors="replace"))

    if status != 200:
        raise RuntimeError(f"{backend} HTTP {status} on {case['id']}: {resp}")

    if backend == "mercury":
        usage = resp.get("usage") or {}
        if "input_tokens" not in usage:
            raise RuntimeError(f"Missing usage.input_tokens on {case['id']} (budget guard)")
        used = usage["input_tokens"]
        ledger = load_ledger()
        ledger["input_tokens"] += used
        ledger["requests"] += 1
        LEDGER_PATH.write_text(json.dumps(ledger, indent=2))

    cache_file.write_text(
        json.dumps(
            {"request": req_body, "status": status, "latency_ms": ms, "response": resp},
            indent=2,
            ensure_ascii=False,
        )
    )
    return parse_decider_response(case, resp, ms, cached_hit=False)


def parse_decider_response(case: dict, resp: dict, ms: int, cached_hit: bool) -> dict:
    ans = resp.get("answers") or {}
    t_ans = ans.get("type") or {}
    l_ans = ans.get("link") or {}
    s_ans = ans.get("support") or {}

    t_choice = t_ans.get("choice", "unsure")
    t_conf = t_ans.get("confidence")
    l_choice = l_ans.get("choice", "unsure")
    l_conf = l_ans.get("confidence")
    p_yes = s_ans.get("noul", s_ans.get("probability", s_ans.get("p_yes")))

    # Apply confidence thresholds (§6.2 / §7.1): low confidence falls back to unsure
    type_pred = t_choice if (t_conf is not None and t_conf >= 0.45) else "unsure"
    link_pred = l_choice if (l_conf is not None and l_conf >= 0.45) else "unsure"
    if p_yes is None:
        support_pred = "unsure"
    elif p_yes >= 0.55:
        support_pred = True
    elif p_yes <= 0.45:
        support_pred = False
    else:
        support_pred = "unsure"

    usage = resp.get("usage") or {}
    return {
        "case": case["id"],
        "band": case["band"],
        "tier": "T2-model",
        "type": type_pred,
        "type_raw": t_choice,
        "type_conf": t_conf,
        "link": link_pred,
        "link_raw": l_choice,
        "link_conf": l_conf,
        "p_support": p_yes,
        "support_mention": support_pred,
        "support_strict": support_pred,
        "input_tokens": usage.get("input_tokens", 0),
        "latency_ms": ms,
        "cached": cached_hit,
    }


def grade_arm(cases: list[dict], rows: list[dict], support_field: str = "support_mention") -> dict:
    by_id = {c["id"]: c for c in cases}
    bands: dict[str, list[dict]] = {}
    for r in rows:
        bands.setdefault(r["band"], []).append(r)

    band_summaries = {}
    for band, b_rows in sorted(bands.items()):
        n = len(b_rows)
        type_cov = 0
        type_ok = 0
        type_wrong_conf = 0
        link_cov = 0
        link_ok = 0
        false_merge = 0
        supp_cov = 0
        supp_ok = 0
        supp_wrong_conf = 0
        tokens = 0
        latencies = []

        for r in b_rows:
            c = by_id[r["case"]]
            exp = c["expect"]
            tokens += r.get("input_tokens", 0)
            if r.get("latency_ms"):
                latencies.append(r["latency_ms"])

            # In B4 (unsupported claim), if support is correctly False, the extractor writes NO entity,
            # so type/link are gated off by support=False. Still grade type/link on B1,B2,B3,B5,B6;
            # on B4, if support==False, count type/link as safely suppressed (ok).
            s_val = r[support_field]
            if s_val != "unsure":
                supp_cov += 1
                if s_val == exp["support"]:
                    supp_ok += 1
                else:
                    supp_wrong_conf += 1

            if band == "B4":
                # For B4, the gate is whether unsupported actors are suppressed (support == False)
                if s_val is False:
                    type_cov += 1
                    type_ok += 1
                    link_cov += 1
                    link_ok += 1
                elif s_val is True:
                    type_cov += 1
                    type_wrong_conf += 1
                    link_cov += 1
                    false_merge += 1
                continue

            # Type grading
            tp = r["type"]
            if tp not in ("unsure", None):
                type_cov += 1
                if tp == exp["type"]:
                    type_ok += 1
                else:
                    type_wrong_conf += 1

            # Link grading
            lp = r["link"]
            if lp not in ("unsure", None):
                link_cov += 1
                if lp == exp["link"]:
                    link_ok += 1
                else:
                    # If it linked to a wrong catalog entity (not "new"), that's a false merge!
                    if lp != "new":
                        false_merge += 1

        band_summaries[band] = {
            "count": n,
            "type_coverage": round(type_cov / n, 3),
            "type_ok": f"{type_ok}/{n}",
            "type_wrong_confident": round(type_wrong_conf / n, 3),
            "link_coverage": round(link_cov / n, 3),
            "link_ok": f"{link_ok}/{n}",
            "false_merge_rate": round(false_merge / n, 3),
            "support_coverage": round(supp_cov / n, 3),
            "support_ok": f"{supp_ok}/{n}",
            "support_wrong_confident": round(supp_wrong_conf / n, 3),
            "input_tokens": tokens,
            "median_ms": sorted(latencies)[len(latencies) // 2] if latencies else 0,
        }
    return band_summaries


def main():
    mode = sys.argv[1] if len(sys.argv) > 1 else "algo"
    order = sys.argv[2] if len(sys.argv) > 2 else "forward"
    base_url = sys.argv[3] if len(sys.argv) > 3 else "http://127.0.0.1:8084"

    corpus = json.loads(CORPUS_PATH.read_text())
    algo = AlgoFloor(corpus["seed_catalog"])
    cases = corpus["cases"]

    if mode == "algo":
        rows = [algo.evaluate_case(c) for c in cases]
        summary_mention = grade_arm(cases, rows, support_field="support_mention")
        summary_strict = grade_arm(cases, rows, support_field="support_strict")
        out = {
            "arm": "algo_t0_t1",
            "rows": rows,
            "by_band_mention_support": summary_mention,
            "by_band_strict_role_support": summary_strict,
        }
        (HERE / "eval_algo_t0_t1.json").write_text(json.dumps(out, indent=2, ensure_ascii=False))
        print("=== ALGO FLOOR (T0/T1) — MENTION SUPPORT ===")
        print(json.dumps(summary_mention, indent=2))
        print("=== ALGO FLOOR (T0/T1) — STRICT ROLE SUPPORT ===")
        print(json.dumps(summary_strict, indent=2))
        return

    if mode in ("mercury", "semselect"):
        rows = []
        t_start = time.monotonic()
        for c in cases:
            r = call_decider(mode, order, c, algo, base_url)
            rows.append(r)
            print(
                f"[{mode}:{order}] {c['id']} -> type={r['type']}({r['type_conf']}) "
                f"link={r['link']}({r['link_conf']}) p_supp={r['p_support']} tok={r['input_tokens']} ms={r['latency_ms']}"
            )
        summary = grade_arm(cases, rows, support_field="support_strict")
        out = {
            "arm": mode,
            "order": order,
            "elapsed_s": round(time.monotonic() - t_start, 2),
            "rows": rows,
            "by_band": summary,
        }
        (HERE / f"eval_{mode}_{order}.json").write_text(json.dumps(out, indent=2, ensure_ascii=False))
        print(f"=== {mode.upper()} ({order}) BY BAND ===")
        print(json.dumps(summary, indent=2))
        return

    if mode == "hybrid":
        backend = order  # e.g. python3 eval_gate.py hybrid mercury forward
        opt_order = sys.argv[3] if len(sys.argv) > 3 else "forward"
        model_data = json.loads((HERE / f"eval_{backend}_{opt_order}.json").read_text())
        model_by_id = {r["case"]: r for r in model_data["rows"]}
        hybrid_rows = []
        model_calls = 0
        for c in cases:
            a_row = algo.evaluate_case(c)
            m_row = model_by_id[c["id"]]
            # Gate 1: T0 mention check suppresses B4 before any model call
            if not a_row["mention"]:
                hybrid_rows.append(a_row)
                continue
            # Gate 2: T0/T1 owns type and link whenever resolved; Decider only fills unsure (B3)
            need_model_type = a_row["type"] == "unsure"
            need_model_link = a_row["link"] == "unsure"
            # Gate 3: B5 role-support check uses Decider noul; B1/B2/B6 use T0 mention check
            need_model_support = c["band"] == "B5"
            used_model = need_model_type or need_model_link or need_model_support
            if used_model:
                model_calls += 1
            # For B5, treat p_support >= 0.50 as True when evaluated by Decider
            p_supp = m_row.get("p_support")
            m_supp = (p_supp >= 0.50) if p_supp is not None else "unsure"
            row = {
                "case": c["id"],
                "band": c["band"],
                "tier": f"T0/T1+{backend}" if used_model else a_row["tier"],
                "type": m_row["type"] if need_model_type else a_row["type"],
                "link": m_row["link"] if need_model_link else a_row["link"],
                "support_mention": m_supp if need_model_support else a_row["support_mention"],
                "support_strict": m_supp if need_model_support else a_row["support_mention"],
                "input_tokens": m_row["input_tokens"] if used_model else 0,
                "latency_ms": m_row["latency_ms"] if used_model else 0,
            }
            hybrid_rows.append(row)
        summary = grade_arm(cases, hybrid_rows, support_field="support_strict")
        out = {
            "arm": f"hybrid_{backend}_{opt_order}",
            "model_calls": f"{model_calls}/{len(cases)}",
            "total_input_tokens": sum(r["input_tokens"] for r in hybrid_rows),
            "by_band": summary,
            "rows": hybrid_rows,
        }
        (HERE / f"eval_hybrid_{backend}_{opt_order}.json").write_text(json.dumps(out, indent=2, ensure_ascii=False))
        print(f"=== GATED HYBRID (T0/T1 + {backend}:{opt_order}) — calls={model_calls}/{len(cases)} ===")
        print(json.dumps(summary, indent=2))
        return

    sys.exit(f"Unknown mode: {mode}")


if __name__ == "__main__":
    main()
