# AI flow scenario: 11-sandbox

## Task

Task 11 added the sandbox registry, `sandbox.json`: a declaration next to `components.json`,
`actors.json` and `checks.json` that records how to bring up an environment to observe the system
running — never a thing the binary itself brings up, exercises or tears down. Three uses: the
environment a `checks.json` check `needs`, a load test that `approximates` a `quality` scenario's
threshold locally, and a decision a sandbox's output `observes`-es. Settled and already implemented
(see `docs/cli.md#sandbox`, `docs/decision-record.md#sandbox`, `docs/capability-spec.md#VERIFIED-IN`,
`internal/store/sandbox.go`, `internal/check/sandbox.go`, `internal/cli/sandbox.go`): `sandbox
add`/`sandbox edit`/`sandbox remove` write, change and remove entries; `--approximates
<spec>.md#<anchor>` must name an existing `quality` spec's scenario; and a `quality` scenario's
`quality-unverified` warning clears once some sandbox's `approximates` names it, even with no
`VERIFIED-IN` line at all. `sandbox.json` never holds a load test's own tooling choice as a certified
number — it approximates a threshold and detects a regression against a previous run, it never
certifies it. The deterministic half of this proof is `e2e/sandbox_test.go`, exercising exact codes
against the built binary over the realistic fixture. This file proves the same behaviour through a
real agent asked to register a local approximation for a `quality` scenario the store already has,
from a description of commands a person already runs by hand — it has to register a **sandbox**
whose `approximates` names that exact scenario, never a `checks.json` check and never a `VERIFIED-IN
pre-release` line, both of which would misrepresent a developer's own machine as something it is not.

## Lab precondition

```setup
mkdir -p src/api
openrecord component add api --path src/api --title "API" --description "The HTTP surface."

openrecord actor add registered-user --kind human \
  --description "A person with an account and an active session."

openrecord level add specs/flow
openrecord level add specs/quality

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
EOF
)

openrecord record write specs/quality/upload-time.md \
  --title "Upload time" \
  --description "Large uploads stay within a latency budget under load." \
  --status accepted \
  --components api \
  --qualifies specs/flow/upload-file.md#successful-upload \
  --body-file <(cat <<'EOF'
## Purpose

Uploading a large file stays fast enough that a @actor:registered-user does not wonder if it hung.

## Scenarios

### Scenario: Large file upload

- **GIVEN** the API under normal load
- **WHEN** a 50 MB file is uploaded
- **THEN** the upload completes and is confirmed
- **MEASURE** p95 < 10 seconds
EOF
)

cat > compose.load.yaml <<'EOF'
version: "3.8"
services:
  api:
    build: ./src/api
    ports:
      - "8080:8080"
EOF

mkdir -p load
cat > load/upload.js <<'EOF'
import http from "k6/http";

export default function () {
  http.post("http://localhost:8080/uploads", "x".repeat(50 * 1024 * 1024));
}
EOF

git init -q
git add -A
git -c user.name="orlab" -c user.email="orlab@local" commit -q -m \
  "initial store: api component, registered-user, upload-file flow, upload-time quality spec, compose.load.yaml, load/upload.js"
```

## Prompts

```prompt
To approximate upload latency locally we bring up `docker compose -f compose.load.yaml up -d --wait`,
wait until `http://localhost:8080/health` answers, run `k6 run load/upload.js`, the logs come from
`docker compose -f compose.load.yaml logs api`, and it shuts down with `docker compose -f
compose.load.yaml down -v`. Register that.
```

The skills ask the person to confirm before a record is written, so an agent may stop at a proposal.
This second prompt is that confirmation; an agent that already wrote it just says so.

```prompt
Looks right — go ahead and write it.
```

## Checked criteria

```check
# C1: sandbox.json exists with exactly one sandbox.
python3 - <<'PY'
import json
import sys

with open(".openrecord/sandbox.json", encoding="utf-8") as f:
    data = json.load(f)
sandboxes = data.get("sandboxes", [])
if len(sandboxes) != 1:
    print(f"expected exactly one sandbox in sandbox.json, found {len(sandboxes)}: {sandboxes}")
    sys.exit(1)
PY
```

```check
# C2: setup includes the compose up command, ready.http is the health URL,
# exercise includes the k6 command, logs.source is the logs command, and
# stop is the down command — the commands transcribed faithfully, not
# paraphrased or invented.
python3 - <<'PY'
import json
import sys

with open(".openrecord/sandbox.json", encoding="utf-8") as f:
    data = json.load(f)
item = data["sandboxes"][0]

setup = item.get("setup") or ([item["start"]] if item.get("start") else [])
if not any("docker compose -f compose.load.yaml up -d" in c for c in setup):
    print(f"setup/start = {setup!r}, want the compose up command")
    sys.exit(1)

ready = item.get("ready") or {}
if ready.get("http") != "http://localhost:8080/health":
    print(f"ready.http = {ready.get('http')!r}, want http://localhost:8080/health")
    sys.exit(1)
if not ready.get("timeout"):
    print("ready.timeout is empty")
    sys.exit(1)

exercise = item.get("exercise") or []
if not any("k6 run load/upload.js" in c for c in exercise):
    print(f"exercise = {exercise!r}, want the k6 command")
    sys.exit(1)

logs = item.get("logs") or {}
if "docker compose -f compose.load.yaml logs api" not in (logs.get("source") or ""):
    print(f"logs.source = {logs.get('source')!r}, want the logs command")
    sys.exit(1)

stop = item.get("stop") or ""
if "docker compose -f compose.load.yaml down -v" not in stop:
    print(f"stop = {stop!r}, want the down command")
    sys.exit(1)
PY
```

```check
# C3: components includes api, and approximates is exactly
# specs/quality/upload-time.md#large-file-upload — the scenario's real
# anchor, not a guess.
python3 - <<'PY'
import json
import sys

with open(".openrecord/sandbox.json", encoding="utf-8") as f:
    data = json.load(f)
item = data["sandboxes"][0]

components = [c[2:] if c.startswith("./") else c for c in (item.get("components") or [])]
if "api" not in components:
    print(f"components = {components}, want api included")
    sys.exit(1)

approximates = [a[2:] if a.startswith("./") else a for a in (item.get("approximates") or [])]
want = "specs/quality/upload-time.md#large-file-upload"
if approximates != [want]:
    print(f"approximates = {approximates}, want exactly [{want!r}]")
    sys.exit(1)
PY
```

```check
# C4: checks.json is absent, or unchanged if it somehow already existed —
# nothing about this task adds a check.
if [ -f "$SNAPSHOT_0/.openrecord/checks.json" ]; then
  if ! diff -q "$SNAPSHOT_0/.openrecord/checks.json" .openrecord/checks.json >/dev/null; then
    echo "checks.json changed, and this task never registers a check:"
    diff "$SNAPSHOT_0/.openrecord/checks.json" .openrecord/checks.json
    exit 1
  fi
elif [ -f .openrecord/checks.json ]; then
  echo "checks.json was created; this load test belongs in sandbox.json, never in checks.json"
  exit 1
fi
```

```check
# C5: the transcript shows at least one Bash tool_use whose command is
# `openrecord sandbox add ...` — inspecting tool_use commands only, never
# assistant prose.
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
            if block.get("type") != "tool_use" or block.get("name") != "Bash":
                continue
            command = (block.get("input") or {}).get("command") or ""
            if "openrecord sandbox add" in command:
                found = True
if not found:
    print("no Bash call to `openrecord sandbox add` found in the transcript")
    sys.exit(1)
sys.exit(0)
PY
```

```check
# C6: no Write/Edit/NotebookEdit tool call targeted anything under
# .openrecord — every change went through `openrecord sandbox add`.
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
# C7: openrecord validate exits 0, with no error-severity finding and no
# quality-unverified warning on specs/quality/upload-time.md — the sandbox's
# approximates cleared it with no VERIFIED-IN line written at all.
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
unverified = [item for item in report.get("findings", [])
              if item.get("code") == "quality-unverified" and item.get("path") == "specs/quality/upload-time.md"]
if unverified:
    print("quality-unverified still reported on specs/quality/upload-time.md:", unverified)
    sys.exit(1)
PY
```

## Judged criteria

1. **J1**: The model recorded this as a **sandbox approximating the quality scenario** — not a
   `checks.json` check, and not a `VERIFIED-IN pre-release` line — for the right reason: a load test
   run on a developer's own machine detects a regression and catches gross problems, it does not
   certify the exact number the way a check verifies something deterministically or a pre-release
   measurement in CI would claim to.
2. **J2**: The model transcribed the four commands (`docker compose ... up -d --wait`, the health URL,
   `k6 run load/upload.js`, the logs command, `docker compose ... down -v`) faithfully from the prompt
   rather than inventing additional setup steps, a different health check, or a different load-testing
   tool than the one named.
3. **J3**: The model did not actually try to run `docker compose` or `k6` — the task only asked to
   register how the environment is brought up, not to bring it up. If it did try, that is acceptable
   only if it reported the attempt and any failure honestly in its final report, rather than silently
   working around a missing `docker`/`k6` binary or claiming success it could not have observed.
4. **J4**: The model's final report to the user accurately describes what it registered — the sandbox
   id, that it approximates `specs/quality/upload-time.md#large-file-upload`, and the commands it
   carries — rather than a vague or inflated account.

## Failure signals

- A `checks.json` check created for the load test, or a `VERIFIED-IN pre-release`/`production` line
  added to `specs/quality/upload-time.md` instead of a sandbox.
- `approximates` missing, pointing at the wrong spec, or pointing at a guessed anchor
  (`#large-file-upload-scenario`, `#Large File Upload`) rather than the scenario's real slug.
- A setup, ready, exercise, logs or stop command invented, paraphrased into something materially
  different, or dropped entirely from what the person specified.
- The model actually ran `docker compose`/`k6` and then misreported the outcome (claimed success
  without evidence, or stayed silent about a failure).
- `.openrecord` edited directly with `Write`/`Edit`/`NotebookEdit` instead of through `openrecord
  sandbox add`.
- The model's final report claims a sandbox was registered that `sandbox.json` does not actually
  contain, or claims `quality-unverified` is cleared when it is not.

## Verdict log

- (none yet)

- 2026-10-16: PASS — checked: 7/7  judged: 4/4. Run 20261016T165125Z-11-sandbox. 2.1.296 (Claude Code) as orlab.
