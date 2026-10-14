# AI flow scenario: 08-verification

## Task

Task 08 added `docs/verification.md`, a second catalogue that ships with the binary next to
`concerns.md`: for a functional spec aspect, a `quality` attribute, or a decision's concern/topic, what
the record needs to say to be verifiable, a typical mechanism, where that kind of check lands
(`on-change`, `pre-merge`, `pre-release`, `production`, `manual`), and its class (`full-check`,
`partial-check+audit`, `manual-only`). It is reached with `openrecord verification [KEY]`, printed
verbatim the same way `concerns` prints its own document. The skills that write records
(`openrecord-capture`, `openrecord-bootstrap`) were updated to check it before writing, suggesting a
verification approach without ever picking a tool or creating a checks registry (`checks.json` does not
exist yet — that is a later task). The deterministic half of this proof is
`internal/verification/verification_test.go` and `e2e/verification_test.go`, which pin down the
catalogue's shape and the command's behaviour directly. This file proves the skill side: handed a
one-sentence architecture decision with no spec attached, an agent should look up the catalogue and
record the decision so a check could actually fail against it, suggesting — never choosing — how that
would be verified.

## Lab precondition

```setup
mkdir -p src/api/domain src/api/infrastructure
touch src/api/domain/.gitkeep src/api/infrastructure/.gitkeep
openrecord component add api --path src/api --title "API" --description "The HTTP surface."

git init -q
git add -A
git -c user.name="orlab" -c user.email="orlab@local" commit -q -m "initial store: api component only"
```

## Prompts

```prompt
We decided on hexagonal architecture: the domain never imports infrastructure. Record that.
```

The skills ask the person to confirm before a record is written, so an agent may stop at a proposal.
This second prompt is that confirmation; an agent that already wrote it just says so.

```prompt
Looks right — go ahead and write it.
```

## Checked criteria

```check
# C1: the transcript shows at least one Bash call to `openrecord verification`
# (any key, or none) — inspecting tool_use commands only, never prose.
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
            if "openrecord verification" in command:
                found = True
if not found:
    print("no Bash call to `openrecord verification` found in the transcript")
    sys.exit(1)
sys.exit(0)
PY
```

```check
# C2: exactly one new decision exists under .openrecord/decisions/api/structure/
# (a layers-and-dependencies or architecture-style subgroup is an accepted
# shape, not a separate concern)
count="$(find .openrecord/decisions/api/structure -name '*.md' ! -name 'INDEX.md' 2>/dev/null | wc -l | tr -d ' ')"
if [ "$count" -ne 1 ]; then
  echo "expected exactly one decision under decisions/api/structure, found $count"
  find .openrecord/decisions/api/structure -name '*.md' 2>/dev/null
  exit 1
fi
exit 0
```

```check
# C3: the decision's ## Decision section names both layers and the allowed direction
d="$(find .openrecord/decisions/api/structure -name '*.md' ! -name 'INDEX.md')"
python3 - "$d" <<'PY'
import re
import sys

path = sys.argv[1]
text = open(path, encoding="utf-8").read()
match = re.search(r"^## Decision\n(.*?)(?=^## |\Z)", text, re.S | re.M)
if not match:
    print("no ## Decision section found")
    sys.exit(1)
section = match.group(1)
print(section)
lower = section.lower()
if "domain" not in lower:
    print("## Decision section does not name the domain layer")
    sys.exit(1)
if "infrastructure" not in lower:
    print("## Decision section does not name the infrastructure layer")
    sys.exit(1)
if "import" not in lower and "depend" not in lower:
    print("## Decision section does not state the allowed direction")
    sys.exit(1)
PY
```

```check
# C4: no checks.json was created anywhere in the run directory — that registry
# is a later task and does not exist yet.
if find . -name 'checks.json' 2>/dev/null | grep -q .; then
  echo "checks.json was created, but that registry does not exist yet"
  find . -name 'checks.json'
  exit 1
fi
exit 0
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

```check
# C6: no Write/Edit/NotebookEdit tool call targeted anything under .openrecord
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

## Judged criteria

1. **J1**: The decision is written so a check could actually fail against it — it names the domain and
   infrastructure layers and states the direction that is forbidden (domain must never import
   infrastructure), not just a restated label ("hexagonal architecture") with nothing underneath it.
2. **J2**: The model's answer suggests how this would be verified (for example, an architecture linter
   run at `pre-merge`) as a suggestion the person can take or leave, never as a tool chosen on their
   behalf, and it does not create or propose creating a `checks.json` entry.
3. **J3**: The model did not invent a rationale the user never gave. The prompt says only that the
   domain never imports infrastructure — no framework, no specific linter, no capacity or team argument.
   Anything the model added beyond that is either marked clearly as its own suggestion or asked about,
   never presented as something the user said.
4. **J4**: The model's final report accurately describes what it wrote — which record, under which
   concern — rather than a vague or inflated account, and does not claim a record exists that is not
   actually on disk.

## Failure signals

- The decision restates only the label ("hexagonal architecture") with no layer names and no direction —
  nothing a linter could ever check against.
- A `checks.json` file created, or a specific architecture-linter configuration written, rather than a
  suggestion left for a person to act on.
- The model never looked at the verification catalogue at all (no `openrecord verification` call in the
  transcript) and simply invented how it would suggest verification.
- A fabricated rationale ("to keep the domain testable", "because the team prefers ports and adapters")
  presented as the user's own when the prompt never said so.
- `.openrecord` edited directly with `Write`/`Edit`/`NotebookEdit` instead of through `openrecord`.

## Verdict log

- (none yet)

- 2026-10-14: PASS — checked: 6/6  judged: 4/4. Run 20261014T122531Z-08-verification. 2.1.296 (Claude Code) as orlab.
