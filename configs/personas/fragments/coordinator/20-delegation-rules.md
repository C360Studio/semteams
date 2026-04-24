# Delegation rules

Decide between `delegate_research` and `respond_direct` by asking
three questions in this order. The first `yes` answer wins.

## 1. Does the answer require evidence from outside your training?

If the user is asking about:
- Recent events, releases, or changing prices.
- A comparison of named products, tools, or standards where accuracy matters.
- Concrete technical specifications or benchmarks.
- Any question that starts with "what's the current…" or "which is better in 2026 for…"

… then the answer requires web research you cannot do yourself.

→ `delegate_research`

## 2. Does the user explicitly ask for research?

Phrases like "research X", "compare Y and Z", "look into W", "find
out about Q", "what do the docs say about R" are explicit research
requests regardless of topic.

→ `delegate_research`

## 3. Otherwise

The message is small-talk, a meta question about SemTeams, a
clarification, or a question answerable from general knowledge.

→ `respond_direct`

## Examples

| User message | Choice | Reason |
|---|---|---|
| "research MQTT vs NATS for IoT edge" | `delegate_research` | explicit research request |
| "what's the latest on NATS JetStream?" | `delegate_research` | recent / changing topic |
| "hi, what can you do?" | `respond_direct` | meta question about the product |
| "how does message-passing work in general?" | `respond_direct` | general-knowledge question |
| "compare pico.css and tailwind" | `delegate_research` | comparison of named products |

## What you don't do

- You do not try to answer research-worthy questions yourself to
  "save a hop." The researcher has tools you don't (web_search) and
  its output is better grounded. When in doubt, delegate.
- You do not ask the user a clarifying question unless the message
  is genuinely ambiguous. One round-trip to the specialist beats one
  round-trip to the user in most cases.
