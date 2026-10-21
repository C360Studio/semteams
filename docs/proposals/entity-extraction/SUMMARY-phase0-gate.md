# Phase 0 & Phase 2/3 Gate Baseline — Algorithm Floor vs. System One Deciders

**Date:** 2026-10-21 · **Corpus:** `corpus_v1.json` (`sha256:842981592bd3a224...`, 12 research artifact snapshots, 36 labeled actor claims across 6 bands B1–B6, 23 seed catalog entities) · **Harness:** `eval_gate.py` (`sha256:6bebbe2df90adf82...`) · **Machine:** Apple Silicon macOS (Metal GPU for `semselect` `Kev-4B-Q4_K_M`, hosted API for `mercury-decide`) · **Orders tested:** `forward` and `reverse` option orders.

## Verdict: T0/T1 carries 4 of 6 bands (B1, B2, B4, B6); Decider is gated ON only for B3 (ambiguous/novel typing & homonym linking) and optional B5 (role-contradiction audit)

Under the pre-declared gate semantics (`coverage >= 0.80`, `wrong_confident <= 0.05`, `false_merge == 0.00`):
- **Bands B1 (`exact_or_alias`), B2 (`lexical_variant`), B4 (`unsupported_claim`), and B6 (`non_ascii`) PASS at the T0/T1 algorithm floor (`24/24` across `type`, `link`, and `mention`, `0.0` wrong-confident, `0.0` false-merge, `0` tokens, `<1 ms`).** The model tier stays **OFF** for these bands.
- **Band B3 (`ambiguous_name` — homonyms like `Mercury` and `Vault`, plus unindexed novel people, events, and concepts) FAILS coverage at T0/T1 (`0/6` type, `3/6` link) and PASSES with the Decider (`6/6` type and `6/6` link in Gated Hybrid on both Mercury and Kev-4B).** The model tier is enabled **only when T0/T1 returns `unsure`**.
- **Band B5 (`role_only_support` — actor is mentioned in the quote, but the free-text `role` is contradicted in 3/6 cases):** T0/T1 passes `type` (`6/6`) and `link` (`6/6`), but a mention check cannot detect role negation (`3/6`, `50%` wrong-confident). Routing the `noul` `support` question to the Decider eliminates all contradicted-role false positives (`6/6` on `semselect` Kev-4B; `5/6` on `mercury-decide`, which also rejected one paraphrase where the role said `"graph ingest"` while the quote said `"graph"`).

## Per-Band Gate Table (36 cases, 6 per band)

| Band | Arm 1: T0/T1 Algo Floor | Arm 2: Mercury Decide (fwd / rev) | Arm 3: Kev-4B semselect (fwd / rev) | Arm 4: Gated Hybrid (T0/T1 + residual Decider) | Gate Verdict |
| --- | --- | --- | --- | --- | --- |
| **B1** exact / alias | **Type 6/6 · Link 6/6 · Supp 6/6** (0 tok) | Type 6/6, 6/6 · Link 4/6, 6/6 · Supp 4/6, 4/6 | Type 5/6, 4/6 · Link 5/6, 4/6 · Supp 5/6, 5/6 | **6/6 · 6/6 · 6/6** (0 model calls) | **ALGO PASS → MODEL OFF** |
| **B2** lexical variant | **Type 6/6 · Link 6/6 · Supp 6/6** (0 tok) | Type 6/6, 6/6 · Link 6/6, 6/6 · Supp 1/6, 1/6* | Type 5/6, 5/6 · Link 4/6, 4/6 · Supp 4/6, 4/6 | **6/6 · 6/6 · 6/6** (0 model calls) | **ALGO PASS → MODEL OFF** |
| **B3** ambiguous / novel | Type 0/6 · Link 3/6 · Supp 6/6 (0 tok) | Type 6/6, 5/6 · Link 6/6, 6/6 · Supp 4/6, 4/6 | Type 6/6, 6/6 · Link 4/6, 4/6 · Supp 5/6, 5/6 | **6/6 · 6/6 · 6/6** (6 model calls) | **ALGO FAIL / MODEL PASS → MODEL ON (residual)** |
| **B4** unmentioned claim | **Suppressed 6/6 at T0 mention check** (0 tok) | Suppressed 6/6, 6/6 (`p_yes <= 0.0097`) | Suppressed 6/6, 6/6 (`p_yes <= 0.0507`) | **6/6 suppressed at T0** (0 model calls) | **ALGO PASS → MODEL OFF** |
| **B5** role contradiction | Type 6/6 · Link 6/6 · Supp 3/6 (50% wrong-conf) | Type 6/6, 6/6 · Link 5/6, 5/6 · Supp 4/6, 4/6 | Type 5/6, 4/6 · Link 2/6, 2/6 · Supp 5/6, 5/6 | **Type 6/6 · Link 6/6 · Supp 5/6 (Mercury) / 6/6 (Kev)** (6 model calls) | **ALGO PASS TYPE/LINK; MODEL ON FOR ROLE SUPPORT** |
| **B6** non-ASCII / emoji | **Type 6/6 · Link 6/6 · Supp 6/6** (0 tok) | Type 6/6, 6/6 · Link 6/6, 6/6 · Supp 5/6, 5/6 | Type 5/6, 6/6 · Link 5/6, 2/6 · Supp 5/6, 5/6 | **6/6 · 6/6 · 6/6** (0 model calls) | **ALGO PASS → MODEL OFF** |
| **TOTAL** | **30/36 Type · 33/36 Link · 0 false-merge · 0 tok** | **36/36 Type (fwd) · 33/36 Link (fwd) · 20.2k tok** | **32/36 Type (fwd) · 26/36 Link (fwd) · 14.4k tok** | **36/36 Type · 36/36 Link · 35–36/36 Supp · 67–83% fewer calls** | **0 false merges across all arms** |

\* *Why raw Mercury `support` (`noul`) scored 1/6 on B2 and 4/6 on B1/B3:* when asked `"Does the supplied text, on its own, support that <name> is <role>?"`, Mercury acted as a strict NLI verifier and rejected any `role` string containing background gloss not literally present in the one-sentence quote (e.g., expanding `ADR-061` to `"architecture decision record"`, calling Mosquitto a `"C MQTT broker"` when the quote didn't mention C, or calling Zürich Instruments a `"Swiss instrumentation company"` when the quote didn't mention Switzerland). For entity extraction (`type` + `link` + `mention`), T0's verbatim quote derivation + mention check is the right support gate; `noul` should only be used when auditing whether a quote contradicts a claimed role.
