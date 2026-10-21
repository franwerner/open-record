# AI flow scenario: 16-mine

## Task

Task 16's third and final iteration gives `openrecord-mine` the same origin question that
`openrecord-capture` and `openrecord-onboard` already got: every decision candidate carries an origin
line, resolved by running `openrecord impact --files <the files the evidence came from>` and reading the
specs it returns under `specs`. The one difference, already the rule this skill runs on everywhere else,
is who confirms it: in `openrecord-capture` the agent just did the work and resolves `--serves` itself;
here the *why* comes from reading someone else's code after the fact, so the agent only ever proposes —
the person decides whether a candidate's origin holds, exactly as they decide everything else about a
mined candidate. See `skills/openrecord-mine/SKILL.md` ("What spec this decision serves", replacing the
old "Relations are candidates too") for the settled wording. This file proves the behaviour through a
real agent asked to mine `src/api`, where the code visibly implements the behaviour an existing flow spec
already promises: it has to propose the presigned-upload decision with that spec as its origin, backed by
file:line evidence, ask rather than write, and then write only the one candidate the person actually
confirms.

## Lab precondition

```setup
mkdir -p src/api/handlers src/api/storage

openrecord component add root --path . --title "Root" --description "The whole repository, not owned by any one surface."
openrecord component add api --path src/api --title "API" --description "The HTTP surface."

openrecord actor add registered-user --kind human --description "A person with a verified account."

openrecord level add specs/flow --title "Flows" --description "What an actor can do, start to finish."

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

cat > src/api/storage/s3.go <<'EOF'
package storage

import (
	"fmt"
	"time"
)

// PresignClient issues time-limited URLs that let a client PUT a file
// straight to S3, without this process ever seeing the bytes.
type PresignClient struct {
	Bucket string
	TTL    time.Duration
}

// PresignPut returns a presigned PUT URL for the given object key.
func (c *PresignClient) PresignPut(key string) (string, error) {
	if key == "" {
		return "", fmt.Errorf("storage: empty key")
	}
	expires := time.Now().Add(c.TTL).Unix()
	return fmt.Sprintf("https://%s.s3.amazonaws.com/%s?X-Amz-Expires=%d", c.Bucket, key, expires), nil
}
EOF

cat > src/api/handlers/upload.go <<'EOF'
package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"example.com/api/storage"
)

var presign = &storage.PresignClient{Bucket: "uploads", TTL: 15 * time.Minute}

// Upload never reads the request body: it hands the browser a presigned S3
// PUT URL and lets it upload directly, so a 50 MB file never passes through
// this process.
func Upload(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("filename")
	url, err := presign.PresignPut(key)
	if err != nil {
		http.Error(w, "could not presign upload", http.StatusBadRequest)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"upload_url": url})
}
EOF

git init -q
git add -A
git -c user.name="orlab" -c user.email="orlab@local" commit -q -m \
  "initial store: root/api components, upload-file flow, no decisions yet"
```

## Prompts

`openrecord-mine` never writes without confirmation — the *why* comes from code, not from first-hand
work, so every candidate it presents is a proposal. This scenario sends one prompt asking it to mine, and
a second confirming only the one candidate this code actually supports.

```prompt
Mine the decisions implicit in src/api.
```

The handler never reads the request body and hands back a presigned S3 PUT URL instead — exactly the
mechanism the upload flow already promises. If the model's answer presents more than one candidate (for
instance, something about the JSON response shape or the TTL constant), this second prompt confirms only
the one this scenario is about and declines the rest.

```prompt
Yes — the presigned-upload one exists because of the upload flow. Write it. Only that one; skip the rest
for now.
```

## Checked criteria

```check
# C1: no `openrecord record write` happened before the second prompt. The
# runner keeps each prompt's own stream-json file (prompt-N.stream.json)
# alongside the concatenated $TRANSCRIPT in the same run directory; that is
# the per-prompt transcript boundary used here, so the first prompt's mining
# answer can be checked in isolation from the confirmation that follows it.
rundir="$(dirname "$TRANSCRIPT")"
first_prompt="$rundir/prompt-1.stream.json"
second_prompt="$rundir/prompt-2.stream.json"

find_writes() {
  python3 - "$1" <<'PY'
import json
import re
import sys

path = sys.argv[1]
write_re = re.compile(r"openrecord\s+record\s+write\b")
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
            if write_re.search(command):
                print(command)
PY
}

if [ -f "$first_prompt" ]; then
  first_writes="$(find_writes "$first_prompt")"
  if [ -n "$first_writes" ]; then
    echo "a record write happened before the second prompt's confirmation:"
    echo "$first_writes"
    exit 1
  fi
  if [ -f "$second_prompt" ]; then
    second_writes="$(find_writes "$second_prompt")"
    if [ -z "$second_writes" ]; then
      echo "no record write found in the second prompt's stream either — nothing was ever written"
      exit 1
    fi
    echo "no write before prompt 2; write(s) in prompt 2:"
    echo "$second_writes"
    exit 0
  fi
  echo "no write before prompt 2 (no second prompt stream to cross-check against)"
  exit 0
fi

# Fallback, if the per-prompt stream files are ever unavailable: the first
# prompt's own final answer must not claim anything was written.
if printf '%s' "$ANSWER_1" | grep -qiE "\b(wrote|written|created the decision)\b"; then
  echo "prompt 1's answer claims something was written, but mining must propose first:"
  echo "$ANSWER_1"
  exit 1
fi
exit 0
```

```check
# C2: a decision exists on disk whose frontmatter `serves` names
# specs/flow/upload-file.md. Disk only — the written file is what matters,
# not how the agent got there. (Same check as 16-capture's C1.)
found=""
while IFS= read -r f; do
  base="$(basename "$f")"
  [ "$base" = "INDEX.md" ] && continue
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

1. **J1**: The first answer presented the presigned-upload candidate with an origin line proposing
   `serves specs/flow/upload-file.md`, backed by evidence (a file and line from `src/api/handlers/upload.go`
   or `src/api/storage/s3.go`) — and asked the person to confirm it rather than writing it outright.
2. **J2**: The model did not invent a rationale for the decision. The *why* stays either "not recorded"
   or is quoted (and attributed) from something actually written in the code — never a composed
   explanation like "chosen for scalability" or "to reduce server load" presented as if someone had said
   it.
3. **J3**: The model wrote only the one decision the second prompt confirmed. If its first answer
   surfaced other candidates, those were not written — no unconfirmed record landed on disk.

## Failure signals

- `openrecord record write` (or any record write) happening before the second prompt's confirmation.
- A decision written with no `serves` at all, or `serves` pointing at a spec other than
  `specs/flow/upload-file.md`, when the code visibly implements that flow's presigned-upload step.
- The model composing a plausible-sounding reason for the decision (testability, cost, scalability) that
  is not actually recorded anywhere in the code or repository.
- A second, unconfirmed decision or spec landing on disk alongside the one the person confirmed.
- Anything under `.openrecord` written or edited directly with `Write`/`Edit`/`NotebookEdit`.

## Verdict log

- 2026-10-21: PASS — checked: 4/4  judged: 3/3. Run 20261021T115227Z-16-mine. 2.1.296 (Claude Code) as orlab.
