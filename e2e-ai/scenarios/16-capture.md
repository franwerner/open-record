# AI flow scenario: 16-capture

## Task

Task 16's second iteration adds an origin question to `openrecord-capture`: before writing a decision,
the agent resolves on its own — nobody is there to ask, because it just did the work — which spec the
decision exists because of, if any. It runs `openrecord impact --files <the files the work touched>`,
reads the specs that command returns under `specs`, and then: if the work was done to fulfil one of
them, writes the decision with `--serves <that spec>`; if a spec merely governs the same files without
being the reason for the decision, it does not link it; if none of them motivated the decision, it says
so in `## Context` and names the decision as an engineering choice. When the same capture also writes a
new spec the decision serves, the spec is written first. See `skills/openrecord-capture/SKILL.md` ("What
spec this decision serves") for the settled wording — extended from the `--serves` paragraph that
already existed in "Writing" rather than duplicating it. `openrecord-mine` is explicitly out of scope
for this iteration. The deterministic half of this task is the skill text itself, read directly:
`internal/cli/skills_test.go` and `e2e/skills_test.go` pin the emit mechanics (byte-identical content,
the rename), not prose wording, and no existing test asserts specific phrases from a shipped skill's
body, so none was added for this change either. This file proves the behaviour through a real agent
asked to finish a piece of work and
then capture what it settled: it has to run `impact --files`, link the resulting decision to the spec it
actually serves, leave an unrelated accepted decision untouched, and never ask for permission before
writing.

## Lab precondition

```setup
mkdir -p src/api/handlers

openrecord component add root --path . --title "Root" --description "The whole repository, not owned by any one surface."
openrecord component add api --path src/api --title "API" --description "The HTTP surface."

openrecord actor add registered-user --kind human --description "A person with a verified account."

openrecord level add specs/flow --title "Flows" --description "What an actor can do, start to finish."
openrecord level add decisions/api/contracts --title "Contracts" --description "How the API shapes what it sends back."

openrecord record write specs/flow/upload-file.md \
  --title "Upload a file" \
  --description "A registered user uploads a file through the API, up to a 50 MB limit." \
  --status accepted --components api --initiated-by registered-user \
  --body-file <(cat <<'EOF'
## Purpose

A @actor:registered-user uploads a file through the API.

## Main flow

1. The @actor:registered-user selects a file and submits it.
2. The API stores the file if it is within the size limit.

## Scenarios

### Scenario: Upload within the limit succeeds

- **GIVEN** a @actor:registered-user with a file up to 50 MB
- **WHEN** they upload it
- **THEN** the file is stored

### Scenario: A larger file is rejected

- **GIVEN** a @actor:registered-user with a file larger than 50 MB
- **WHEN** they upload it
- **THEN** the upload is rejected
EOF
)

openrecord record write decisions/api/contracts/error-format.md \
  --title "Errors are problem+json" \
  --description "API errors are returned as application/problem+json, not a bespoke error shape." \
  --status accepted \
  --body-file <(cat <<'EOF'
## Context

Every handler needs one error shape so clients can parse failures the same way everywhere.

## Decision

API errors are returned as `application/problem+json`.

## Alternatives

A bespoke `{error: string}` shape was considered and dropped: it carries no machine-readable error
code and every client would need its own ad hoc parsing.

## Consequences

A new endpoint that returns an error reuses the existing problem+json writer, not its own shape.
EOF
)

cat > src/api/handlers/upload.go <<'EOF'
package handlers

import (
	"io"
	"net/http"
)

// Upload reads the entire request body into memory and stores it.
func Upload(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "could not read upload", http.StatusBadRequest)
		return
	}
	store(data)
}

func store(data []byte) {
	// persists the uploaded bytes
}
EOF

git init -q
git add -A
git -c user.name="orlab" -c user.email="orlab@local" commit -q -m \
  "initial store: root/api components, upload-file flow, an unrelated error-format decision"
```

## Prompts

`openrecord-capture` never asks for confirmation before writing — the skill resolves the record and
writes it, then reports. This scenario sends one prompt doing both the work and the capture, the way a
person would actually use it.

```prompt
Change the upload so the client asks the API for a presigned S3 URL and uploads directly — the API
shouldn't receive 50 MB bodies. Then capture what this settled.
```

If the agent stops to ask for confirmation anyway before writing the decision — contrary to what the
skill says — a second prompt unblocks it, and that departure from the skill's own stated behaviour is
worth noting in the verdict log rather than silently worked around.

```prompt
Go ahead.
```

## Checked criteria

```check
# C1: a decision exists on disk under .openrecord/decisions/ whose frontmatter
# `serves` names specs/flow/upload-file.md. Disk only — the written file is
# what matters, not how the agent got there.
found=""
while IFS= read -r f; do
  base="$(basename "$f")"
  [ "$base" = "INDEX.md" ] && continue
  if [ "$f" = ".openrecord/decisions/api/contracts/error-format.md" ]; then
    continue
  fi
  # serves is written inline (`serves: [a, b]`) by the binary; a block list
  # (`serves:` then `- a`) is accepted too. Only the frontmatter is read.
  if awk 'NR==1 && /^---$/{fm=1; next} fm && /^---$/{exit} fm' "$f" \
     | awk '/^serves:/{print; flag=1; next} /^[a-zA-Z0-9_-]+:/{flag=0} flag' \
     | grep -q 'specs/flow/upload-file\.md'; then
    found="$f"
    break
  fi
done < <(find .openrecord/decisions -name '*.md' 2>/dev/null)

if [ -z "$found" ]; then
  echo "no decision file on disk carries serves: specs/flow/upload-file.md"
  find .openrecord/decisions -name '*.md' -exec sh -c 'echo "--- {} ---"; cat "{}"' \;
  exit 1
fi
echo "found: $found"
exit 0
```

```check
# C2: the unrelated decision's file is byte-identical to what the lab
# precondition committed — this work never touched it.
if ! git diff --quiet HEAD -- .openrecord/decisions/api/contracts/error-format.md; then
  echo "the unrelated decision changed:"
  git diff HEAD -- .openrecord/decisions/api/contracts/error-format.md
  exit 1
fi
if [ -n "$(git status --porcelain -- .openrecord/decisions/api/contracts/error-format.md)" ]; then
  echo "the unrelated decision shows as changed in git status:"
  git status --porcelain -- .openrecord/decisions/api/contracts/error-format.md
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

1. **J1**: The model linked the new decision to `specs/flow/upload-file.md` on its own — it ran
   `openrecord impact --files` (or otherwise resolved the spec governing the files it touched) and
   decided `--serves` for itself, rather than asking the person whether the decision should be linked to
   anything.
2. **J2**: The model wrote no more records than the work justifies. At most the one decision (the
   presigned-URL mechanism); at most one spec edit if the upload flow's observable behaviour genuinely
   changed (for example, a new scenario for the presigned-URL step) — judge whether such an edit, if
   present, was actually warranted by what changed, or whether the model wrote extra records (an
   unrelated decision, a quality spec, a second unjustified decision) beyond what this one piece of work
   settled.

## Failure signals

- A decision written with no `serves` at all while the model's own report frames it as settling how the
  upload flow works.
- `serves` pointing at `decisions/api/contracts/error-format.md`, or at any spec other than
  `specs/flow/upload-file.md`.
- The model asking the person for approval before writing the decision (beyond the one "go ahead" this
  scenario allows if it happens).
- `decisions/api/contracts/error-format.md` changed in any way.
- Anything under `.openrecord` written or edited directly with `Write`/`Edit`/`NotebookEdit`.

## Verdict log

- 2026-10-21: PASS — checked: 4/4  judged: 2/2. Run 20261021T110938Z-16-capture. 2.1.296 (Claude Code) as orlab.
- 2026-10-23: PASS — checked: 4/4  judged: 2/2. Run 20261023T173743Z-16-capture. 2.1.296 (Claude Code) as orlab. Regression after task 17 extended the skill.
