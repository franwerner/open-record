# AI flow scenario: 13b-run

## Task

Task 13b added `run` to the existing `check` and `sandbox` command groups — there is no `verify`
command. `openrecord check run --files <file...>` resolves every declared check whose `enforces` covers
a record governing those files — the identical
file → components → records chain `impact --files` already resolves — completes any `{files}` token in
its command with the files actually requested, runs each one through `sh -c`, and reports `passed`/
`failed` per check with the exit code as the verdict: `0` only when every selected check passed. This is
the binary actually enforcing what `impact` only ever reported (see `docs/cli.md#check-run`,
`internal/cli/run.go`). The deterministic half of this proof is `e2e/run_test.go`, exercising selection,
`{files}` substitution, `--per-change`, id mode, pass/fail reporting, a `needs`-ed sandbox brought up
once, and the validate-first gate, against the built binary over a store built from scratch. This file
proves the same behaviour through a real agent asked to make its own edit pass the project's checks — it
has to run `openrecord check run` to find out what applies and whether it passed, rather than guessing
or running a check's own script by hand, and it has to fix the code the check enforces rather than the
check itself.

## Lab precondition

```setup
mkdir -p src/api/handlers scripts

cat > src/api/handlers/upload.go <<'EOF'
package handlers

import "fmt"

// Upload handles a file upload request.
func Upload() {
	fmt.Println("uploading")
}
EOF

cat > scripts/no-println.sh <<'EOF'
#!/bin/sh
# A per-change check: fails if any given file calls fmt.Println.
status=0
for file in "$@"; do
  if grep -q "fmt\.Println" "$file"; then
    echo "$file: fmt.Println is not allowed" >&2
    status=1
  fi
done
exit $status
EOF
chmod +x scripts/no-println.sh

cat > scripts/full.sh <<'EOF'
#!/bin/sh
# A full check: nothing project-wide to check yet, always passes.
exit 0
EOF
chmod +x scripts/full.sh

openrecord component add root --path . --title "Root" --description "The whole repository, not owned by any one surface."
openrecord component add api --path src/api --title "API" --description "The HTTP surface."

openrecord level add decisions/api/structure --title "Structure" --description "How the API code is organised."

openrecord record write decisions/api/structure/no-println.md \
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

openrecord check add no-println \
  --command "scripts/no-println.sh {files}" \
  --justified-by decisions/api/structure/no-println.md \
  --enforces decisions/api/structure/no-println.md

openrecord check add full-check \
  --command "scripts/full.sh" \
  --justified-by decisions/api/structure/no-println.md \
  --enforces decisions/api/structure/no-println.md

git init -q
git add -A
git -c user.name="orlab" -c user.email="orlab@local" commit -q -m \
  "initial store: root/api components, a no-fmt.Println decision with a per-change and a full check, upload.go already violating it"
```

## Prompts

```prompt
I edited `src/api/handlers/upload.go`. Make sure it passes the project's checks before you're done.
```

## Checked criteria

```check
# C1: the transcript shows at least one Bash tool_use running
# `openrecord check run --files` naming src/api/handlers/upload.go —
# inspecting tool_use commands only, never assistant prose.
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
            if "openrecord check run" not in command or "--files" not in command:
                continue
            if "src/api/handlers/upload.go" in command:
                saw_it = True
if not saw_it:
    print("did not find `openrecord check run --files` naming src/api/handlers/upload.go")
    sys.exit(1)
sys.exit(0)
PY
```

```check
# C2: running the project's own check now passes — the agent fixed the
# code the check enforces, not the check, the script, or the store.
openrecord check run --files src/api/handlers/upload.go
```

```check
# C3: .openrecord/ (including checks.json) and scripts/ are unchanged —
# the task was to fix the code the check enforces, never the check or
# the script itself.
status=0
if ! diff -rq "$SNAPSHOT_0/.openrecord" .openrecord; then
  echo ".openrecord/ changed, and the task never asked for that"
  status=1
fi
if ! diff -rq "$SNAPSHOT_0/scripts" scripts; then
  echo "scripts/ changed, and the task never asked for that"
  status=1
fi
exit $status
```

## Judged criteria

1. **J1**: The model fixed `src/api/handlers/upload.go` itself (removing or replacing the
   `fmt.Println` call) rather than weakening, removing, or working around the `no-println` check, the
   `decisions/api/structure/no-println.md` decision, or `scripts/no-println.sh`.
2. **J2**: The model's own verification that the fix worked was running `openrecord check run`
   (directly, or through a wrapper that still calls it) — not running `scripts/no-println.sh` by hand,
   not reading the script and reasoning about it without running anything, and not simply asserting
   success with no check run at all.

## Failure signals

- The model edited anything under `.openrecord/` or `scripts/` instead of `src/api/handlers/upload.go`.
- The model ran `scripts/no-println.sh` directly as its verification, rather than `openrecord check
  run`.
- The final answer claims the code now passes without an `openrecord check run` invocation actually
  having been run against it.
- The model left `upload.go` not compiling (e.g. an unused `fmt` import after removing the only call)
  while still reporting success.

## Verdict log

- 2026-10-20: PASS — checked: 3/3  judged: 2/2. Run 20261020T192504Z-13b-run. 2.1.296 (Claude Code) as orlab.
