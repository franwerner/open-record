# AI flow scenario: 17-gaps

## Task

Task 17 teaches `openrecord-capture` how to resolve a gap an `openrecord run` left open (task 15's own
mechanism, `docs/cli.md#run`, "Gaps"). When nothing specific is named, the agent finds what is open with
`openrecord run gaps` — never by reading `.openrecord/runs/` directly — and lets the person pick. How a
gap gets resolved depends on its `--kind`: a `missing` gap gets a brand-new record, defined and written
with the person the same way "## Defining something specific" describes; a `pending` gap already has a
record, written by someone else (`openrecord-mine`, typically) that the person has never read — the one
place in this skill where the agent shows the **full body** before asking whether to accept, change, or
discard it. Either way, `openrecord run gap resolve <run> <gap> --record <path>` only ever takes an
existing, `accepted` record, and resolving moves every task the gap blocked back to `pending` in the
same run (unless another open gap still blocks it) — it never writes to `.openrecord/compliance/`. No
new CLI behaviour is introduced here: `run gaps`, `run gap resolve`, and `record edit --status` already
exist and are covered by `e2e/run_exec_test.go` and unit tests. This file proves the skill's own
judgement calls against a real run with two different kinds of open gap.

## Lab precondition

```setup
mkdir -p src/api/jobs

cat > src/api/jobs/export.go <<'EOF'
package jobs

import "time"

// RunExport executes one user-data export job.
// TODO: decide a retry policy for transient failures before this ships.
func RunExport() error {
	time.Sleep(time.Millisecond)
	return nil
}
EOF

openrecord component add root --path . --title "Root" --description "The whole repository, not owned by any one surface."
openrecord component add api --path src/api --title "API" --description "The HTTP surface, including background jobs it queues."

openrecord level add decisions/api/exports --title "Exports" --description "How user data is exported: triggers, formats, and what a background job owns."

openrecord record write decisions/api/exports/background-job.md \
  --title "Exports run as a background job" \
  --description "A user data export runs as an asynchronous background job, not inline with the request." \
  --status accepted \
  --body-file <(cat <<'EOF'
## Context

A full data export can take long enough that holding the request open risks a client timeout well
before the export is actually done.

## Decision

A user data export runs as a background job; the triggering request only enqueues it.

## Alternatives

Running the export inline, in the request handler, was considered and dropped: a slow export would tie
up a handler for the whole duration, with nothing to show for it if the client gave up first.

## Consequences

The endpoint that starts an export returns straight away with a job reference; nothing about the
export's own completion is synchronous.
EOF
)

openrecord record write decisions/api/exports/csv-encoding.md \
  --title "CSV export uses UTF-8 with BOM" \
  --description "The CSV export is written as UTF-8 with a leading byte-order mark, for compatibility with Excel." \
  --status pending \
  --body-file <(cat <<'EOF'
## Context

A CSV opened directly in Excel without a byte-order mark is routinely misread as a different encoding,
which corrupts any non-ASCII character in it.

## Decision

The CSV export is written as UTF-8, with a leading byte-order mark.

## Alternatives

Plain UTF-8 with no BOM was considered and dropped: it reads correctly in most tools but breaks the one
spreadsheet application most of this export's actual readers use.

## Consequences

Every CSV writer in the export path has to emit the BOM the same way; a reader that does not expect one
(some older *nix tools) sees three extra bytes at the top of the file.
EOF
)

openrecord run start --scope decisions/api/exports

RUN_ID=$(ls .openrecord/runs | grep -v '^\.gitignore$')

openrecord run tasks set "$RUN_ID" --file <(cat <<'EOF'
[
  {"id": "task-01", "title": "Background job retry policy", "records": ["decisions/api/exports/background-job.md"], "depends_on": []},
  {"id": "task-02", "title": "CSV encoding", "records": ["decisions/api/exports/csv-encoding.md"], "depends_on": []}
]
EOF
)

openrecord run gap add "$RUN_ID" \
  --id retry-policy \
  --question "How many times should a failed export job retry before giving up, and with what backoff?" \
  --kind missing \
  --evidence decisions/api/exports/background-job.md \
  --evidence src/api/jobs/export.go:6 \
  --blocks task-01

openrecord run gap add "$RUN_ID" \
  --id csv-encoding-confirm \
  --question "This CSV-encoding decision was drafted already — should it be accepted as written?" \
  --kind pending \
  --record decisions/api/exports/csv-encoding.md \
  --evidence decisions/api/exports/csv-encoding.md \
  --blocks task-02

git init -q
git add -A
git -c user.name="orlab" -c user.email="orlab@local" commit -q -m \
  "initial store: components, an accepted and a pending export decision, a run with two open gaps blocking task-01 and task-02"
```

## Prompts

```prompt
Let's resolve what's left pending on the open run.
```

```prompt
On the retry policy: 3 attempts, exponential backoff starting at 1 second and capped at 30 seconds — we
picked 3 because more than that rarely helps and just delays the failure signal reaching the user.

And on the CSV-encoding decision you just showed me — yes, accept it exactly as drafted.
```

```prompt
Looks right — go ahead and write it, and resolve both gaps.
```

## Checked criteria

```check
# C1: the transcript shows a Bash tool_use running `openrecord run gaps`
# (the one command for finding open gaps without a run id in hand).
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
            if "openrecord run gaps" in command:
                saw_it = True
if not saw_it:
    print("did not find an `openrecord run gaps` tool_use in the transcript")
    sys.exit(1)
sys.exit(0)
PY
```

```check
# C2: nothing under .openrecord/runs/ was read directly — no Bash tool_use
# inspecting that path with cat/ls/find/head/tail/grep, and no Read
# tool_use targeting a file under it. Every look goes through the binary
# (`run gaps`, `run status`).
python3 - "$TRANSCRIPT" <<'PY'
import json
import re
import sys

path = sys.argv[1]
bad = []
read_tools = re.compile(r"\b(cat|ls|find|head|tail|grep|less|more)\b")
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
            if block.get("type") != "tool_use":
                continue
            name = block.get("name")
            inp = block.get("input") or {}
            if name == "Read":
                target = inp.get("file_path") or ""
                if ".openrecord/runs" in target.replace("\\", "/"):
                    bad.append(("Read", target))
            elif name == "Bash":
                command = inp.get("command") or ""
                if ".openrecord/runs" in command and read_tools.search(command):
                    bad.append(("Bash", command))
if bad:
    print("read .openrecord/runs/ directly instead of through the binary:", bad)
    sys.exit(1)
sys.exit(0)
PY
```

```check
# C3: both gaps' own files show they were resolved through the binary —
# `state: resolved` with a `resolved_by` record — read from disk, not the
# transcript.
RUN_ID=$(ls .openrecord/runs | grep -v '^\.gitignore$')

python3 - ".openrecord/runs/$RUN_ID/gaps/retry-policy.json" ".openrecord/runs/$RUN_ID/gaps/csv-encoding-confirm.json" "$SNAPSHOT_0" <<'PY'
import json
import os
import sys

retry_path, csv_path, snapshot0 = sys.argv[1], sys.argv[2], sys.argv[3]

def load(path):
    with open(path, encoding="utf-8") as f:
        return json.load(f)

retry = load(retry_path)
if retry.get("state") != "resolved":
    print(f"retry-policy gap state = {retry.get('state')!r}, want 'resolved'")
    sys.exit(1)
resolved_record = retry.get("resolved_by") or ""
if not resolved_record:
    print("retry-policy gap has no resolved_by")
    sys.exit(1)
# resolved_by is a store-relative path (no leading .openrecord/) — the
# record's own file on disk always carries that prefix.
on_disk = os.path.join(".openrecord", resolved_record)
if os.path.exists(os.path.join(snapshot0, on_disk)):
    print(f"retry-policy's resolved_by ({resolved_record}) already existed before any prompt ran; "
          "a 'missing' gap has to be resolved with a genuinely new record")
    sys.exit(1)
if not os.path.exists(on_disk):
    print(f"retry-policy's resolved_by ({resolved_record}) does not exist on disk")
    sys.exit(1)

csv = load(csv_path)
if csv.get("state") != "resolved":
    print(f"csv-encoding-confirm gap state = {csv.get('state')!r}, want 'resolved'")
    sys.exit(1)
if csv.get("resolved_by") != "decisions/api/exports/csv-encoding.md":
    print(f"csv-encoding-confirm's resolved_by = {csv.get('resolved_by')!r}, "
          "want decisions/api/exports/csv-encoding.md")
    sys.exit(1)
PY
```

```check
# C4: the CSV-encoding decision is now accepted (it was pending in the lab
# precondition) — this is what `run gap resolve` required of it.
status="$(awk 'NR==1 && /^---$/{fm=1; next} fm && /^---$/{exit} fm' \
  .openrecord/decisions/api/exports/csv-encoding.md | awk -F': *' '/^status:/{print $2; exit}')"
if [ "$status" != "accepted" ]; then
  echo "decisions/api/exports/csv-encoding.md status = $status, want accepted"
  exit 1
fi
exit 0
```

```check
# C5: both blocked tasks are back to pending in the same run, read through
# the binary — a gap resolving never writes to .openrecord/compliance/,
# and no run command here is a stand-in for one.
RUN_ID=$(ls .openrecord/runs | grep -v '^\.gitignore$')

out_file="$(mktemp)"
openrecord run status "$RUN_ID" >"$out_file"
python3 - "$out_file" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as f:
    report = json.load(f)
items = {item["id"]: item for item in report["tasks"]["items"]}
for task_id in ("task-01", "task-02"):
    state = items[task_id]["state"]
    if state != "pending":
        print(f"{task_id} state = {state!r}, want 'pending'")
        sys.exit(1)
PY
status=$?
if [ "$status" -ne 0 ]; then exit 1; fi
if [ -d .openrecord/compliance ]; then
  echo ".openrecord/compliance/ exists; resolving a gap never writes there"
  exit 1
fi
exit 0
```

```check
# C6: openrecord validate exits 0 with no error-severity finding.
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

1. **J1**: For the `pending` gap (csv-encoding-confirm), the model showed the record's full body —
   context, decision, alternatives — before asking whether to accept, change, or discard it. It did not
   jump straight to editing the status, and did not summarise the record instead of actually showing it.
2. **J2**: The model's final account of what happened describes the two blocked tasks returning to
   `pending` within this same run — it does not claim this wrote anything to a compliance log, and does
   not claim either task is now `done`.

## Failure signals

- Any direct `ls`/`cat`/`find`/`grep` (or equivalent) over `.openrecord/runs/` instead of `openrecord
  run gaps`/`run status`.
- A gap resolved with no answer from the person in between — jumping from `run gaps` straight to `run
  gap resolve`.
- The `pending` gap resolved by editing `csv-encoding.md`'s status without ever showing its body first.
- A claim that this wrote to `.openrecord/compliance/`, or that either task reached `done`.
- Anything under `.openrecord` written or edited directly with `Write`/`Edit`/`NotebookEdit`.

## Verdict log

- 2026-10-23: PASS — checked: 6/6  judged: 2/2. Run 20261023T173245Z-17-gaps. 2.1.296 (Claude Code) as orlab.
