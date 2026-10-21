# AI flow scenario: 15-runs

## Task

Task 15 added `openrecord run` — a run's own artifacts under `.openrecord/runs/<id>/`
(`run.json`, `plan.json`, `tasks.json`, `tasks/`, `gaps/`), ignored by git, never records: a unit of work (one
scope, one plan, one set of tasks), never a session, that can be picked up again in a later session. A
task's **definition** (id/title/records/depends_on, in `tasks.json`) and its **execution** (state,
and, once relevant, what it started from and found — in its own `tasks/<task-id>.json`) are two
separate files: `run tasks set` only ever touches the first, `run task start`/`run task done`/a gap
only ever the second. `run start --scope` computes a plan once; `run tasks set` validates and saves a
task list (coverage, containment, done-task immutability read from each task's own execution, no
hand-written state, `depends_on` with no cycle); `run task start` moves a task to `in_progress` and
snapshots every file git already considers changed, each with a content fingerprint, into its own
execution file; `run task done` **takes no `--files` flag at all** — it diffs the working tree against
that snapshot to infer `{files}` itself, runs, with the exact same engine `check run` uses, the union
of every check whose command takes `{files}` (completed with whatever it inferred, or skipped with a
reason when nothing changed) and every check whose `enforces` covers the task's own records, and only
moves it to `done` once everything that ran passes, recording `state: done` plus what it found on that
same execution file — writing nothing to
`.openrecord/compliance/` yet (see `docs/cli.md#run`, `internal/store/run*.go`,
`internal/cli/execution*.go`). The deterministic half of this proof is `e2e/run_exec_test.go`,
exercising plan computation (compliance, possibly_stale, verified_by, scenarios, outside_scope), scope
resolution and its union, drift, every `tasks set` validation and its history, task start/done
(including the shared check-running engine, the snapshot-based file inference in every one of its
cases, and a failing check leaving the task untouched), gaps, the integrity hash refusing a hand-edited
file, and that no run command ever writes under compliance — against the built binary and stores built
from scratch. This file proves the same behaviour through a real agent asked to pick up a run already
in progress: it has to find the open run and its next task through `openrecord run status` rather than
guessing, finish the
one task left pending by actually making the code pass its check, and leave the task already `done`,
the plan, and every declared check exactly as they were.

## Lab precondition

```setup
mkdir -p src/api/handlers scripts

cat > src/api/handlers/upload.go <<'EOF'
package handlers

import "fmt"

// Upload handles a file upload request.
func Upload(name string) error {
	fmt.Println("uploading", name)
	return nil
}
EOF

cat > scripts/enforce-a.sh <<'EOF'
#!/bin/sh
# A full check enforcing decision A: nothing project-wide to check, always passes.
exit 0
EOF
chmod +x scripts/enforce-a.sh

cat > scripts/enforce-b.sh <<'EOF'
#!/bin/sh
# A per-change check enforcing decision B: fails if any given file calls fmt.Println.
status=0
for file in "$@"; do
  if grep -q "fmt\.Println" "$file"; then
    echo "$file: fmt.Println is not allowed; use the project's logger" >&2
    status=1
  fi
done
exit $status
EOF
chmod +x scripts/enforce-b.sh

openrecord component add root --path . --title "Root" --description "The whole repository, not owned by any one surface."
openrecord component add api --path src/api --title "API" --description "The HTTP surface."

openrecord level add decisions/api/structure --title "Structure" --description "How the API code is organised."

openrecord record write decisions/api/structure/decision-a.md \
  --title "Decision A" \
  --description "A structural decision about the API, unrelated to logging." \
  --status accepted \
  --body-file <(cat <<'EOF'
## Context

A structural choice about the API that has nothing to do with this scenario's logging rule.

## Decision

Decision A holds, as a separate thing task-01 covers.

## Alternatives

None relevant here.

## Consequences

None relevant here.
EOF
)

openrecord record write decisions/api/structure/decision-b.md \
  --title "No fmt.Println in the API" \
  --description "API code never calls fmt.Println; use the project's logger instead." \
  --status accepted \
  --body-file <(cat <<'EOF'
## Context

A stray `fmt.Println` left in API code bypasses the project's structured logger and never ends up
anywhere logs are actually collected, filtered or rate-limited.

## Decision

API code never calls `fmt.Println`.

## Alternatives

Catching this in code review alone was considered and dropped: it is exactly the kind of mechanical
rule a check should enforce on every change, not something a reviewer has to remember to look for.

## Consequences

Any API code found to call `fmt.Println` has to switch to the project's logger before it can land.
EOF
)

openrecord check add check-a \
  --command "scripts/enforce-a.sh" \
  --justified-by decisions/api/structure/decision-a.md \
  --enforces decisions/api/structure/decision-a.md

openrecord check add check-b \
  --command "scripts/enforce-b.sh {files}" \
  --justified-by decisions/api/structure/decision-b.md \
  --enforces decisions/api/structure/decision-b.md

openrecord run start --scope decisions/api

RUN_ID=$(ls .openrecord/runs | grep -v '^\.gitignore$')

openrecord run tasks set "$RUN_ID" --file <(cat <<'EOF'
[
  {"id": "task-01", "title": "Decision A", "records": ["decisions/api/structure/decision-a.md"], "depends_on": []},
  {"id": "task-02", "title": "No fmt.Println in upload.go", "records": ["decisions/api/structure/decision-b.md"], "depends_on": ["task-01"]}
]
EOF
)

openrecord run task start "$RUN_ID" task-01
openrecord run task done "$RUN_ID" task-01

git init -q
git -c user.name="orlab" -c user.email="orlab@local" add -A
git -c user.name="orlab" -c user.email="orlab@local" commit -q -m \
  "initial store: components, decisions A and B, their checks, a run with task-01 done and task-02 pending"
```

`task-02` is left `pending`: `fmt.Println` is still in `upload.go`, so `check-b` currently fails it.

## Prompts

```prompt
There's an openrecord run in progress. Pick it up where it left off and finish it.
```

## Checked criteria

```check
# C1: the transcript shows at least one Bash tool_use running
# `openrecord run status` — inspecting tool_use commands only, never
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
            if "openrecord run status" in command:
                saw_it = True
if not saw_it:
    print("did not find an `openrecord run status` tool_use in the transcript")
    sys.exit(1)
sys.exit(0)
PY
```

```check
# C2: the transcript shows a Bash tool_use running `openrecord run task done`
# for task-02, and it actually exited 0 — verified here by reading
# task-02's own execution file directly rather than trusting the
# transcript alone. State lives there, never in tasks.json (the
# definition only: id, title, records, depends_on).
RUN_ID=$(ls .openrecord/runs | grep -v '^\.gitignore$')

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
            if "openrecord run task done" in command and "task-02" in command:
                saw_it = True
if not saw_it:
    print("did not find `openrecord run task done ... task-02` in the transcript")
    sys.exit(1)
sys.exit(0)
PY
if [ $? -ne 0 ]; then exit 1; fi

python3 - ".openrecord/runs/$RUN_ID/tasks/task-02.json" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as f:
    execution = json.load(f)
if execution["state"] != "done":
    print(f"task-02's execution state = {execution['state']!r}, want 'done'")
    sys.exit(1)
PY
```

```check
# C3: task-01 is unchanged — its own execution file still says done, with
# the exact fields it had right after the lab precondition. (Its
# definition in tasks.json is untouched by this scenario either way —
# nobody called `run tasks set` again.)
RUN_ID=$(ls .openrecord/runs | grep -v '^\.gitignore$')

python3 - "$SNAPSHOT_0/.openrecord/runs/$RUN_ID/tasks/task-01.json" ".openrecord/runs/$RUN_ID/tasks/task-01.json" <<'PY'
import json
import sys

def execution(path):
    with open(path, encoding="utf-8") as f:
        return json.load(f)

before = execution(sys.argv[1])
after = execution(sys.argv[2])
if before != after:
    print(f"task-01's execution changed:\n  before: {before}\n  after:  {after}")
    sys.exit(1)
PY
```

```check
# C4: plan.json and .openrecord/compliance/ are untouched, and checks.json
# and the check script are unchanged — task-02 was finished by fixing the
# code the check enforces, never by touching the check itself.
RUN_ID=$(ls .openrecord/runs | grep -v '^\.gitignore$')

if ! diff -q "$SNAPSHOT_0/.openrecord/runs/$RUN_ID/plan.json" ".openrecord/runs/$RUN_ID/plan.json" >/dev/null; then
  echo "plan.json changed; run start's plan must never be updated by run task done"
  exit 1
fi
if [ -d .openrecord/compliance ]; then
  echo ".openrecord/compliance/ exists; no run command writes there yet"
  exit 1
fi
if ! diff -q "$SNAPSHOT_0/.openrecord/checks.json" .openrecord/checks.json >/dev/null; then
  echo "checks.json changed; task-02 should be finished by fixing the code, not the check"
  exit 1
fi
if ! diff -q "$SNAPSHOT_0/scripts/enforce-b.sh" scripts/enforce-b.sh >/dev/null; then
  echo "scripts/enforce-b.sh changed; the check script itself must not be edited to make it pass"
  exit 1
fi
```

```check
# C5: the fix is real, not a shortcut. `run task done` skips a `{files}`
# check when nothing changed since `run task start` (task 15's own rule) —
# which means simply calling start then done with no edit at all would
# otherwise also report `task-02` as "done" with nothing ever checked.
# This confirms `check-b` actually ran against the touched file: upload.go
# itself no longer calls fmt.Println, and task-02's own execution file
# records it under done.files (modified or created, read from its own
# tasks/task-02.json, never from tasks.json — that file holds only the
# definition).
if grep -q "fmt\.Println" src/api/handlers/upload.go; then
  echo "src/api/handlers/upload.go still calls fmt.Println; decision B does not hold"
  exit 1
fi

RUN_ID=$(ls .openrecord/runs | grep -v '^\.gitignore$')
python3 - ".openrecord/runs/$RUN_ID/tasks/task-02.json" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as f:
    execution = json.load(f)
done = execution.get("done") or {}
files = done.get("files") or {}
touched = (files.get("modified") or []) + (files.get("created") or [])
if "src/api/handlers/upload.go" not in touched:
    print(f"task-02's done.files = {files!r}, want upload.go under modified or created — "
          "if both are empty, check-b was skipped rather than actually run against the fix")
    sys.exit(1)
PY
```

## Judged criteria

1. **J1**: The model did not redo task-01 — no re-run of its check, no attempt to re-mark or revisit
   it, and no claim in the final answer that task-01 was (re)verified as part of finishing this run.
2. **J2**: The model fixed `src/api/handlers/upload.go` so `check-b` passes (removing or replacing the
   `fmt.Println` call with the project's own logging, per decision B) — it did not weaken, special-case
   or remove `check-b`, and did not edit `decisions/api/structure/decision-b.md` to make the rule say
   something else.

## Failure signals

- The model edited `checks.json`, `scripts/enforce-a.sh`, or `scripts/enforce-b.sh` instead of the code
  the check enforces.
- The model hand-edited anything under `.openrecord/runs/` (a Write/Edit/NotebookEdit targeting that
  path) instead of calling `openrecord run tasks set`/`run task start`/`run task done`.
- The model wrote anything under `.openrecord/compliance/` — this task does not write compliance yet.
- The model re-ran or re-marked task-01, or reported it as newly verified.
- The final answer claims the run was finished with no `openrecord run task done` call for task-02
  actually having run and exited 0.
- The model called `run task start` then `run task done` for task-02 with no edit to
  `upload.go` in between, relying on `check-b` being skipped (nothing changed since the snapshot) to
  reach `done` without the rule ever actually being checked.

## Verdict log

- 2026-10-21: PASS — checked: 5/5  judged: 2/2. Run 20261021T014151Z-15-runs. 2.1.296 (Claude Code) as orlab.
