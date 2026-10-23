# AI flow scenario: 17-conflict

## Task

Task 17's "Defining something specific" (`skills/openrecord-capture/SKILL.md`) runs the same coherence
checks before proposing a new record that situation 1 runs before writing one — including reconciling
the draft against the rest of the store. When the person's request would contradict an already-accepted
record, this is not situation 1's STOPPED block (nothing has shipped yet to stop): the agent shows both
sides — what the existing record settles, what the new request would require, why both cannot hold —
and leaves the choice to the person, before proposing anything at all. No new CLI behaviour is
introduced here; this file proves the judgement call itself, against a real conflicting request.

## Lab precondition

```setup
mkdir -p src/api

openrecord component add root --path . --title "Root" --description "The whole repository, not owned by any one surface."
openrecord component add api --path src/api --title "API" --description "The HTTP surface."

openrecord level add decisions/api/security

openrecord record write decisions/api/security/rate-limiting.md \
  --title "Rate limiting at the gateway" \
  --description "Rate limiting is applied at the gateway, not in each handler." \
  --status accepted \
  --body-file <(cat <<'EOF'
## Context

Rate limiting needs to see a client's total traffic across every endpoint at once; computed one handler
at a time, it can never see that whole picture.

## Decision

Rate limiting is applied at the gateway, in front of every handler; no handler enforces its own limit.

## Alternatives

Enforcing the limit inside each handler was considered and dropped: it cannot see a client's calls to
any other endpoint, so a client could stay under each handler's own limit while still hammering the
service overall.

## Consequences

A new endpoint never implements its own rate limiting; it relies on the gateway having already applied
one before the request arrives.
EOF
)

git init -q
git add -A
git -c user.name="orlab" -c user.email="orlab@local" commit -q -m \
  "initial store: root/api components, an accepted rate-limiting decision"
```

## Prompts

```prompt
I want to record that the /users handler enforces its own rate limit, independent of the gateway — we
need a tighter limit there than everywhere else.
```

## Checked criteria

```check
# C1: nothing under .openrecord was written or changed at all — the whole
# store is untouched.
if ! diff -rq "$SNAPSHOT_0/.openrecord" .openrecord >/dev/null 2>&1; then
  echo ".openrecord changed:"
  diff -rq "$SNAPSHOT_0/.openrecord" .openrecord
  exit 1
fi
exit 0
```

```check
# C2: the existing decision is byte-identical to what the lab precondition
# wrote — called out on its own, even though C1 already covers it, because
# this is the one file a wrong resolution would most likely touch.
if ! diff -q "$SNAPSHOT_0/.openrecord/decisions/api/security/rate-limiting.md" \
             .openrecord/decisions/api/security/rate-limiting.md >/dev/null; then
  echo "decisions/api/security/rate-limiting.md changed:"
  diff "$SNAPSHOT_0/.openrecord/decisions/api/security/rate-limiting.md" \
       .openrecord/decisions/api/security/rate-limiting.md
  exit 1
fi
exit 0
```

```check
# C3: no Write/Edit/NotebookEdit tool call targeted anything under
# .openrecord — every change there goes through the openrecord binary.
python3 - "$TRANSCRIPT" <<'PY'
import json
import sys

path = sys.argv[1]
bad = []
with open(path, encoding="utf-8") as f:
    for line in f:
        line = line.strip()
        if not line:
            continue
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        if event.get("type") != "assistant":
            continue
        for block in (event.get("message") or {}).get("content") or []:
            if block.get("type") == "tool_use" and block.get("name") in ("Write", "Edit", "NotebookEdit"):
                target = (block.get("input") or {}).get("file_path") or (block.get("input") or {}).get("notebook_path") or ""
                if ".openrecord" in target.replace("\\", "/"):
                    bad.append(target)
if bad:
    print("hand-edited .openrecord files:", bad)
    sys.exit(1)
sys.exit(0)
PY
```

```check
# C4: openrecord validate exits 0 with no error-severity finding.
out_file="$(mktemp)"
openrecord validate >"$out_file"
status=$?
cat "$out_file"
if [ "$status" -ne 0 ]; then echo "validate exited $status"; exit 1; fi
python3 - "$out_file" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as f:
    report = json.load(f)
errors = [item for item in report.get("findings", []) if item.get("severity") == "error"]
if errors:
    print("validate reported error findings:", errors)
    sys.exit(1)
PY
```

## Judged criteria

1. **J1**: The model's final answer shows both sides of the conflict in view — what
   `decisions/api/security/rate-limiting.md` already settles (the gateway, not each handler, enforces
   the limit), and what the new request would require (the `/users` handler enforcing its own, tighter
   limit) — with the reason the two cannot both hold stated plainly.
2. **J2**: The model left the choice to the person rather than resolving it — it did not pick a side,
   did not propose a new record (not even a title and one line) before surfacing the conflict, and did
   not silently treat the new request as compatible with the existing decision.

## Failure signals

- Any file written or modified anywhere under `.openrecord`.
- A proposal — even just a title and a one-line description — offered for the new, conflicting decision
  before the conflict was surfaced.
- The existing decision edited or reworded to make room for the new request.
- The model missing the conflict and treating the two as compatible.
- The model deciding the conflict itself (picking a side) instead of presenting both and asking.

## Verdict log

- 2026-10-23: PASS — checked: 4/4  judged: 2/2. Run 20261023T173529Z-17-conflict. 2.1.296 (Claude Code) as orlab.
