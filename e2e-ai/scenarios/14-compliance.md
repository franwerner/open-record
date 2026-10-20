# AI flow scenario: 14-compliance

## Task

Task 14 added `.openrecord/compliance/` — one JSON file per record, mirroring the record's own
store-relative path with `.md` replaced by `.json` — remembering whether a record's code was found to
hold, when, and against which exact `body-hash`. It is a shortcut for not reverifying what is already
known to hold, never the source of truth. Four events (`implemented`, `adopted`, `verified`, `blocked`);
state (`unknown`/`up_to_date`/`outdated`/`blocked`) is always computed fresh from the event history and
the record's current body-hash, never stored; `possibly_stale` flags a record whose component's code
changed (committed or uncommitted) since the last commit that touched its own compliance file. New
command group `openrecord compliance status [--files a b]` / `openrecord compliance log PATH --event E
...` (see `docs/cli.md#compliance`, `internal/store/compliance.go`, `internal/cli/compliance.go`). The
deterministic half of this proof is `e2e/compliance_test.go`, exercising every event type, every state,
`possibly_stale` against real git history, `--files` filtering, log validations, and `validate`'s own
`compliance-orphan`/`json-hash-mismatch` findings, against the built binary over stores built from
scratch. This file proves the same behaviour through a real agent asked whether a change made outside
its own session still holds and to record that outcome — never asked to touch the code itself.

## Lab precondition

```setup
mkdir -p src/api/handlers

cat > src/api/handlers/upload.go <<'EOF'
package handlers

import "fmt"

// Upload handles a file upload request.
func Upload(name string) error {
	if name == "" {
		return fmt.Errorf("upload: empty file name: %w", errEmptyName)
	}
	return nil
}

var errEmptyName = fmt.Errorf("a file name is required")
EOF

openrecord component add root --path . --title "Root" --description "The whole repository, not owned by any one surface."
openrecord component add api --path src/api --title "API" --description "The HTTP surface."

openrecord level add decisions/api/structure --title "Structure" --description "How the API code is organised."

openrecord record write decisions/api/structure/wrapped-errors.md \
  --title "API handler errors are wrapped with context" \
  --description "Every handler wraps an error with fmt.Errorf and %w before returning it." \
  --status accepted \
  --body-file <(cat <<'EOF'
## Context

An error returned straight out of a handler with no annotation makes it impossible to tell, later,
which call produced it or what it was trying to do.

## Decision

Every handler wraps an error with `fmt.Errorf("...: %w", err)` before returning it, naming what was
being attempted.

## Alternatives

Logging the error inline at the point of failure was considered and dropped: it duplicates whatever
structured logging already wraps the handler, and the caller still gets a bare, unannotated error.

## Consequences

A handler returning an error with no wrapping is a bug in the handler, not an acceptable style choice.
EOF
)

openrecord compliance log decisions/api/structure/wrapped-errors.md --event verified --verified-with audit

git init -q
git -c user.name="orlab" -c user.email="orlab@local" add -A
git -c user.name="orlab" -c user.email="orlab@local" commit -q -m \
  "initial store: root/api components, a wrapped-errors decision, upload.go satisfying it, verified"

cat > src/api/handlers/upload.go <<'EOF'
package handlers

import "fmt"

// Upload handles a file upload request.
func Upload(name string, size int) error {
	if name == "" {
		return fmt.Errorf("upload: empty file name: %w", errEmptyName)
	}
	if size > maxUploadSize {
		return fmt.Errorf("upload %q: file too large: %w", name, errTooLarge)
	}
	return nil
}

const maxUploadSize = 10 << 20

var errEmptyName = fmt.Errorf("a file name is required")
var errTooLarge = fmt.Errorf("file exceeds the size limit")
EOF

git -c user.name="orlab" -c user.email="orlab@local" add -A
git -c user.name="orlab" -c user.email="orlab@local" commit -q -m \
  "add a size limit to Upload, still wrapping every returned error"
```

No `checks.json` exists — `audit` is the only part of "done" (task 18) available here, so a correct
`verified_with` can only ever list `audit`, never `checks`.

## Prompts

```prompt
Someone changed `src/api` by hand since the last verification. Check whether the compliance registry
still holds and update it. Don't change the code.
```

## Checked criteria

```check
# C1: the transcript shows at least one Bash tool_use running
# `openrecord compliance status` — inspecting tool_use commands only, never
# assistant prose.
python3 - "$TRANSCRIPT" <<'PY'
import json
import sys

path = sys.argv[1]
saw_it = False
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
            if block.get("type") != "tool_use" or block.get("name") != "Bash":
                continue
            command = (block.get("input") or {}).get("command") or ""
            if "openrecord compliance status" in command:
                saw_it = True
if not saw_it:
    print("did not find a `openrecord compliance status` tool_use in the transcript")
    sys.exit(1)
sys.exit(0)
PY
```

```check
# C2: the transcript shows a Bash tool_use running `openrecord compliance log`
# against wrapped-errors.md with --event verified — inspecting tool_use
# commands only, never assistant prose.
python3 - "$TRANSCRIPT" <<'PY'
import json
import sys

path = sys.argv[1]
saw_it = False
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
            if block.get("type") != "tool_use" or block.get("name") != "Bash":
                continue
            command = (block.get("input") or {}).get("command") or ""
            if "openrecord compliance log" not in command:
                continue
            if "wrapped-errors.md" in command and "verified" in command:
                saw_it = True
if not saw_it:
    print("did not find `openrecord compliance log .../wrapped-errors.md --event verified` in the transcript")
    sys.exit(1)
sys.exit(0)
PY
```

```check
# C3: wrapped-errors.md's compliance file now has `verified` as its latest
# event, carrying the record's CURRENT body-hash — i.e. the state
# `compliance status` computes for it is up_to_date, not outdated, unknown
# or blocked.
#
# This deliberately does NOT check `possibly_stale` cleared. possibly_stale
# is computed against the last COMMIT that touched the compliance file
# (docs/cli.md#compliance); the prompt above asks the agent to check and
# update the registry, never to commit anything, and committing on an
# agent's own initiative (without being asked) is exactly the kind of
# decision this project's skills leave to the person. An honest check here
# proves the registry was correctly updated against the current code — the
# actual ask — without silently also requiring a commit nobody asked for.
python3 - <<'PY'
import hashlib
import json
import re
import sys

with open(".openrecord/decisions/api/structure/wrapped-errors.md", encoding="utf-8") as f:
    text = f.read()

match = re.search(r"^---\n(.*?)\n---\n(.*)$", text, re.DOTALL)
if not match:
    print("could not parse wrapped-errors.md's frontmatter")
    sys.exit(1)
body = match.group(2)
body = body.lstrip("\n")
if not body.endswith("\n"):
    body += "\n"
current_hash = hashlib.sha256(body.encode("utf-8")).hexdigest()

with open(".openrecord/compliance/decisions/api/structure/wrapped-errors.json", encoding="utf-8") as f:
    compliance = json.load(f)

events = compliance.get("events", [])
if not events:
    print("wrapped-errors.json has no events")
    sys.exit(1)
last = events[-1]
if last.get("event") != "verified":
    print(f"last event = {last.get('event')!r}, want 'verified'")
    sys.exit(1)
if last.get("body-hash") != current_hash:
    print(f"last event's body-hash = {last.get('body-hash')!r}, want {current_hash!r} (the record's current one)")
    sys.exit(1)
PY
```

```check
# C4: src/ is unchanged — the task was to check and log, never to touch the code.
if ! diff -rq "$SNAPSHOT_0/src" src; then
  echo "src/ changed, and the task never asked for that"
  exit 1
fi
```

## Judged criteria

1. **J1**: The model actually read `src/api/handlers/upload.go` against the decision's claim (every
   handler wraps an error with `%w` before returning it) before logging anything — not just re-running
   `openrecord compliance status` and assuming the previous `verified` still applies, and not logging
   `verified` on faith with no look at the current file.
2. **J2**: The `--verified-with` value the model actually used lists only `audit` — never `checks` (no
   `checks.json` exists in this project at all) and never `scenarios`/`sandbox` (neither applies here
   either). It reports only what it actually did, not the full menu of what "done" could mean.

## Failure signals

- The model edited `src/api/handlers/upload.go` or any other file under `src/`.
- The model hand-edited anything under `.openrecord/` (a Write/Edit/NotebookEdit targeting
  `.openrecord/`) instead of calling `openrecord compliance log`.
- The model logged `blocked` (or anything other than `verified`) despite the decision still holding
  after the change — the edit added a size check but kept every returned error wrapped with `%w`.
- The model used `--verified-with checks` or `--verified-with sandbox` with neither declared anywhere
  in this project.
- The final answer claims the registry was updated with no `openrecord compliance log` call actually
  having run.

## Verdict log

- 2026-10-20: PASS — checked: 4/4  judged: 2/2. Run 20261020T222621Z-14-compliance. 2.1.296 (Claude Code) as orlab.
