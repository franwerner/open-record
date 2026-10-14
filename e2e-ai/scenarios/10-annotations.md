# AI flow scenario: 10-annotations

## Task

Task 10 added `@record:<path>[#<anchor>]`: a comment convention any test, linter rule, configuration
file or script can carry to declare which record it tries to verify, found by scanning the
repository's text rather than by parsing each language's comment syntax — `//`, `#`, `--`, `/* */`
all work the same way. `<anchor>` is exactly a capability spec's own anchor (issue 01's slug rule), and
it is optional: a decision or a `rule` spec has no anchors at all, so those are always referenced
whole, and any other record may be referenced whole too. Settled and already implemented (see
`docs/decision-record.md#annotations`, `docs/cli.md` "## `validate`", `internal/annotation/
annotation.go`, `internal/check/annotations.go`, `internal/check/check.go`): `validate` reports
`annotation-record-not-found` when the path does not resolve to a record, and
`annotation-anchor-not-found` when the anchor does not exist on the target — at the annotation's own
`file:line`, never at the record. A scenario with no annotation naming it at all is never an error,
only pending work for later issues. The deterministic half of this proof is
`e2e/annotations_test.go`, exercising exact codes and scopes against the built binary over the
realistic fixture. This file proves the same behaviour through a real agent asked to make an
*existing, unannotated* test traceable to the scenario it already covers — it has to find the
scenario's real anchor from the store (reading the spec, or `openrecord`'s own anchor output) rather
than guessing a slug, and it has to annotate the test, never the production code sitting right next
to it.

## Lab precondition

```setup
mkdir -p src/api
openrecord component add api --path src/api --title "API" --description "The HTTP surface."

openrecord actor add registered-user --kind human \
  --description "A person with an account and an active session."

openrecord level add specs/flow

openrecord record write specs/flow/upload-file.md \
  --title "Upload a file" \
  --description "A registered user uploads a file under a size limit." \
  --status accepted \
  --components api \
  --initiated-by registered-user \
  --body-file <(cat <<'EOF'
## Purpose

A @actor:registered-user uploads a file to their account.

## Main flow

1. The @actor:registered-user selects a file and submits it.
2. The system stores the file and confirms the upload.

## Scenarios

### Scenario: Successful upload

- **GIVEN** a @actor:registered-user with a file under the size limit
- **WHEN** they submit it
- **THEN** the upload is confirmed

### Scenario: File too large

- **GIVEN** a @actor:registered-user with a file over the size limit
- **WHEN** they submit it
- **THEN** they are told the file is too large
EOF
)

cat > src/api/upload.go <<'EOF'
package api

import "errors"

const maxUploadBytes = 50 * 1024 * 1024

// ErrFileTooLarge is returned when the given data is over the size limit.
var ErrFileTooLarge = errors.New("file too large")

// Upload stores data if it fits under the size limit.
func Upload(data []byte) error {
	if len(data) > maxUploadBytes {
		return ErrFileTooLarge
	}
	// ... store the file ...
	return nil
}
EOF

cat > src/api/upload_test.go <<'EOF'
package api

import "testing"

func TestUpload_RejectsFilesOverTheLimit(t *testing.T) {
	data := make([]byte, maxUploadBytes+1)
	if err := Upload(data); err != ErrFileTooLarge {
		t.Fatalf("got %v, want ErrFileTooLarge", err)
	}
}
EOF

git init -q
git add -A
git -c user.name="orlab" -c user.email="orlab@local" commit -q -m \
  "initial store: api component, registered-user, upload-file spec, upload.go/upload_test.go"
```

## Prompts

```prompt
This test covers the file-too-large scenario; make it traceable.
```

The skills ask the person to confirm before a record is written, so an agent may stop at a proposal.
This second prompt is that confirmation; an agent that already wrote it just says so.

```prompt
Looks right — go ahead and write it.
```

## Checked criteria

```check
# C1: upload_test.go now contains exactly one occurrence of
# @record:specs/flow/upload-file.md#file-too-large — the slug of "File too
# large", the scenario the test already covers.
python3 - <<'PY'
import sys

with open("src/api/upload_test.go", encoding="utf-8") as f:
    content = f.read()
want = "@record:specs/flow/upload-file.md#file-too-large"
count = content.count(want)
if count != 1:
    print(f"expected exactly one occurrence of {want!r} in upload_test.go, found {count}")
    sys.exit(1)
PY
```

```check
# C2: src/api/upload.go (production code) is byte-identical to what the
# setup wrote — nothing was annotated there, and nothing else changed.
if ! diff -q "$SNAPSHOT_0/src/api/upload.go" src/api/upload.go >/dev/null; then
  echo "src/api/upload.go was modified:"
  diff "$SNAPSHOT_0/src/api/upload.go" src/api/upload.go
  exit 1
fi
```

```check
# C3: no Write/Edit/NotebookEdit tool call targeted upload.go
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
                if target.replace("\\", "/").endswith("src/api/upload.go"):
                    bad.append(target)
if bad:
    print("production code was edited:", bad)
    sys.exit(1)
sys.exit(0)
PY
```

```check
# C4: no Write/Edit/NotebookEdit tool call targeted anything under .openrecord
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
# C5: openrecord validate exits 0 with no error-severity finding
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

1. **J1**: The anchor `file-too-large` was taken from the store — reading `specs/flow/upload-file.md`,
   or an `openrecord` command reporting its anchors — not guessed from the test's own name or invented
   from the prompt's wording. The transcript shows evidence of looking at the actual scenario name
   before writing the annotation.
2. **J2**: The annotation was added to the test that verifies the scenario (`upload_test.go`), never to
   the production code it exercises (`upload.go`) — the model's own reasoning, if it explained itself,
   reflects that this is the rule rather than an accident.
3. **J3**: The model did not invent a new scenario, rename the existing one, or touch
   `specs/flow/upload-file.md` in any way — this task is about annotating code that already exists, not
   about changing the record.
4. **J4**: The model's final report to the user accurately describes what it did — which file it
   annotated, the exact annotation it added, and which scenario it ties the test to — rather than a
   vague or inflated account.

## Failure signals

- The annotation landed in `upload.go` instead of `upload_test.go`, or in both.
- A wrong or invented anchor (`#successful-upload`, `#File too large`, `#file_too_large`, a freehand
  slug that does not match issue 01's rule).
- `specs/flow/upload-file.md` edited, directly or through `record edit`, when the task never asked for
  that.
- `.openrecord` edited directly with `Write`/`Edit`/`NotebookEdit` instead of being left untouched.
- The model's final report claims a different file or a different anchor than what was actually
  written.

## Verdict log

- (none yet)

- 2026-10-14: PASS — checked: 5/5  judged: 4/4. Run 20261014T174232Z-10-annotations. 2.1.296 (Claude Code) as orlab.
