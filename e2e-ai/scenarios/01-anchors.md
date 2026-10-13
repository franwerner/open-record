# AI flow scenario: 01-anchors

## Task

`implementar/01-anclas-por-nombre.md` asked for name-derived anchors on specs, so a scenario or a
lifecycle transition can be referenced by a stable slug instead of a brittle heading path. Settled
and already implemented (see `docs/capability-spec.md` "### Anchors", `internal/check/anchor.go`,
`internal/check/slug.go`):

- An anchor is `Slug(name)`: lowercase, Unicode NFD, drop combining marks, keep letters/digits,
  everything else becomes `-`, collapse/trim hyphens.
- `flow`/`process` specs anchor every `### Scenario: <name>`. `lifecycle` specs additionally anchor
  every `- <from> → <to> (<note>)` transition as `Slug(from)-to-Slug(to)`. `rule` specs and decisions
  have nothing anchorable.
- Scenario anchors and transition anchors share one namespace per record. Two anchor sources
  collapsing to the same anchor is `duplicate-anchor` (error). A name with no letters or digits is
  `empty-anchor` (error). Both stop a `record write`/`record edit` from landing; `validate` reports
  them for whatever is already on disk.
- A spec write/edit that succeeds returns an `anchors` array; a decision write/edit never does.

The deterministic half of this proof is `e2e/anchors_test.go`, exercising exact codes and anchor
values against the built binary. This file proves the same behavior through a real agent driving the
tool, rather than a script driving it.

## Lab precondition

```setup
mkdir -p src/api && touch src/api/.gitkeep
openrecord component add api --path src/api --title "API" --description "The HTTP surface." || true
openrecord level add specs/lifecycle --title "Lifecycle" --description "Lifecycle specs." || true
openrecord record write specs/lifecycle/order-status.md \
  --title "Order status" --description "Order lifecycle." --status accepted --components api \
  --body-file <(cat <<'EOF'
## Purpose

An order's lifecycle.

## States and transitions

- pending → paid (the payment is confirmed)

## Scenarios

### Scenario: Cancelled by the buyer

- **GIVEN** a pending order
- **WHEN** the buyer cancels it
- **THEN** it moves to cancelled
EOF
)
```

## Prompts

```prompt
This project uses openrecord. There's a spec at `specs/lifecycle/order-status.md`. Add a new
transition to it: `paid → refunded (the customer is refunded)`. Then tell me the exact anchor that
transition gets.
```

```prompt
Try to add a scenario called `"Cancelled, by the Buyer!"` to that same spec. What happens, and why?
```

## Checked criteria

```check
# C1: the new transition line is present and the original transition/scenario are kept
f=.openrecord/specs/lifecycle/order-status.md
grep -qF 'pending → paid (the payment is confirmed)' "$f" || { echo "missing original transition"; exit 1; }
grep -qF 'paid → refunded (the customer is refunded)' "$f" || { echo "missing new transition"; exit 1; }
grep -qF '### Scenario: Cancelled by the buyer' "$f" || { echo "missing original scenario heading"; exit 1; }
exit 0
```

```check
# C2: openrecord validate exits 0 with no body-hash-mismatch anywhere in the store
out="$(openrecord validate)"
status=$?
echo "$out"
if [ "$status" -ne 0 ]; then echo "validate exited $status"; exit 1; fi
if grep -q '"code":"body-hash-mismatch"' <<<"$out"; then echo "body-hash-mismatch reported"; exit 1; fi
exit 0
```

```check
# C3: the answer to prompt 1 states the anchor exactly as paid-to-refunded
grep -qF 'paid-to-refunded' <<<"$ANSWER_1" || { echo "paid-to-refunded not found in ANSWER_1"; exit 1; }
exit 0
```

```check
# C4: the spec file is byte-identical before and after prompt 2
f=.openrecord/specs/lifecycle/order-status.md
before="$SNAPSHOT_1/$f"
if [ ! -f "$before" ]; then echo "missing snapshot $before"; exit 1; fi
if ! cmp -s "$before" "$f"; then echo "spec file changed across prompt 2"; diff -u "$before" "$f" || true; exit 1; fi
exit 0
```

```check
# C5: answer 2 names both colliding scenario names and the shared anchor
grep -qF 'cancelled-by-the-buyer' <<<"$ANSWER_2" || { echo "missing anchor cancelled-by-the-buyer in ANSWER_2"; exit 1; }
grep -qF 'duplicate-anchor' <<<"$ANSWER_2" || { echo "missing duplicate-anchor in ANSWER_2"; exit 1; }
exit 0
```

```check
# C6: the transcript shows openrecord record edit actually invoked via the Bash tool
python3 - "$TRANSCRIPT" <<'PY'
import json
import sys

path = sys.argv[1]
found = False
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
            if block.get("type") == "tool_use" and block.get("name") == "Bash":
                command = (block.get("input") or {}).get("command", "")
                if "openrecord record edit" in command:
                    found = True
sys.exit(0 if found else 1)
PY
```

```check
# C7: no direct Write/Edit tool was used on any .openrecord file
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
            if block.get("type") == "tool_use" and block.get("name") in ("Write", "Edit"):
                target = (block.get("input") or {}).get("file_path", "")
                if ".openrecord" in target:
                    bad.append(target)
if bad:
    print("hand-edited .openrecord files:", bad)
    sys.exit(1)
sys.exit(0)
PY
```

## Judged criteria

1. **J1**: The answer to prompt 1 reports the anchor by quoting what `openrecord` actually returned
   (its `anchors` field or equivalent tool output), not a value the model derived from the slug rule
   in its head.
2. **J2**: The answer to prompt 2 explains the rejection by citing `openrecord`'s own finding — both
   colliding names (`"Cancelled by the buyer"` and `"Cancelled, by the Buyer!"`) and the shared anchor
   — rather than a generic or invented explanation.
3. **J3**: The model never claims the second scenario was added; it is clear the attempt was rejected.

## Failure signals

- The model edited the markdown file by hand instead of going through `openrecord record edit`.
- The model invented an anchor instead of reading it from the tool's own output.
- An edit was rejected but the model reported success anyway.
- The original transition or scenario was lost or altered.
- The model fabricated a reason for the rejection instead of quoting what `openrecord` reported.
- The model silently retried with a hand-edit to force the second scenario through.

## Verdict log

- 2026-10-10: PASS — prompt 1 edited via `record edit` (body-hash valid, original content kept), answered `paid-to-refunded` from tool output; prompt 2 rejected with `duplicate-anchor`, quoted both names and `cancelled-by-the-buyer`, file unchanged. Claude Code 2.1.296 as orlab.

- 2026-10-13: PASS — checked: 7/7  judged: 3/3. Run 20261013T185536Z-01-anchors. 2.1.296 (Claude Code) as orlab.
