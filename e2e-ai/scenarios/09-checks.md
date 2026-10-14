# AI flow scenario: 09-checks

## Task

Task 09 added the checks registry, `checks.json`: a declaration next to `components.json` and
`actors.json` that maps a deterministic command to the decision that justifies running it
(`justified-by`, required) and what it is responsible for (`enforces`, optional — a record or a group
coordinate, resolved with the identical mechanism `map`/`validate --for` already use). There is no
`stage` field: a literal `{files}` token in `command` marks a check `on-change`; its absence marks it
`pre-merge`-only. Settled and already implemented (see `docs/cli.md#check`,
`docs/decision-record.md#checks`, `internal/store/checks.go`, `internal/check/checks.go`,
`internal/cli/check.go`): `check add`/`check remove` write and remove entries, each one refused unless
`justified-by` names an existing decision, every `enforces` entry resolves to at least one record, and
every `config` file exists; `validate` reports the same breakage later, plus a `json-hash-mismatch`-style
finding if `checks.json` is ever hand-edited. `checks.json` only ever holds what a command can verify
deterministically — manual or statistical verification is never registered here. The deterministic half
of this proof is `e2e/checks_test.go`, exercising exact codes against the built binary over the
realistic fixture. This file proves the same behaviour through a real agent asked to register a tool a
person already named, against two decisions the store already has: it has to link `justified-by` to the
decision about the *tool*, and `enforces` to the decision about the *layering rule the tool checks* —
not interchange the two or merge them into one.

## Lab precondition

```setup
mkdir -p src/api && touch src/api/.gitkeep
openrecord component add api --path src/api --title "API" --description "The HTTP surface."

openrecord level add decisions/api/structure
openrecord level add decisions/api/delivery

openrecord record write decisions/api/structure/layers.md \
  --title "Layered architecture" \
  --description "The domain never imports infrastructure." \
  --status accepted \
  --body-file <(cat <<'EOF'
## Context

The domain logic has to stay testable and portable across infrastructure choices (database driver,
HTTP framework, message broker) without being rewritten every time one of them changes.

## Decision

The domain never imports infrastructure. Infrastructure imports the domain, never the other way
around — dependencies point inward, toward the domain, and never outward from it.

## Alternatives

Letting the domain import a persistence interface defined alongside it was considered, but it blurred
the boundary in practice: once one infrastructure import was allowed, more followed.

## Consequences

Every infrastructure concern needs an adapter in its own package, translating to and from domain types —
more files for a small feature, in exchange for a domain that never has to change when a driver or a
framework is swapped out.
EOF
)

openrecord record write decisions/api/delivery/arch-linter.md \
  --title "go-arch-lint enforces the layering" \
  --description "Architecture layers are enforced with go-arch-lint." \
  --status accepted \
  --body-file <(cat <<'EOF'
## Context

The layering decided in decisions/api/structure/layers.md has no automatic enforcement yet: nothing
stops a future change from adding the one infrastructure import that breaks it, and a reviewer does not
always catch it by reading a diff.

## Decision

go-arch-lint checks the architecture layers on every change, failing the build if the domain package
ever imports an infrastructure package.

## Alternatives

Relying on code review alone was considered and rejected: a layering violation is easy to miss in a
large diff, and a linter catches it deterministically every time.

## Consequences

The project carries a `.go-arch-lint.yml` configuration file naming the layers and the allowed import
directions, which has to be kept in sync if a new layer is introduced.
EOF
)

cat > .go-arch-lint.yml <<'EOF'
version: 3
workdir: .
vendoring: false
EOF

git init -q
git add -A
git -c user.name="orlab" -c user.email="orlab@local" commit -q -m "initial store: api component, layering and arch-linter decisions"
```

## Prompts

```prompt
Register the check: we run `go-arch-lint check` to enforce the structure decisions.
```

The skills ask the person to confirm before a record is written, so an agent may stop at a proposal.
This second prompt is that confirmation; an agent that already wrote it just says so.

```prompt
Looks right — go ahead and write it.
```

## Checked criteria

```check
# C1: checks.json exists with exactly one check
python3 - <<'PY'
import json
import sys

with open(".openrecord/checks.json", encoding="utf-8") as f:
    data = json.load(f)
checks = data.get("checks", [])
if len(checks) != 1:
    print(f"expected exactly one check in checks.json, found {len(checks)}: {checks}")
    sys.exit(1)
PY
```

```check
# C2: the check's command is exactly `go-arch-lint check` — no {files}, so it
# classifies as pre-merge-only, not on-change.
python3 - <<'PY'
import json
import sys

with open(".openrecord/checks.json", encoding="utf-8") as f:
    data = json.load(f)
item = data["checks"][0]
command = item.get("command", "")
if command != "go-arch-lint check":
    print(f"command = {command!r}, want exactly 'go-arch-lint check'")
    sys.exit(1)
if "{files}" in command:
    print("command carries a {files} placeholder; this is a pre-merge-only check, not on-change")
    sys.exit(1)
PY
```

```check
# C3: justified-by names the tool decision, decisions/api/delivery/arch-linter.md
python3 - <<'PY'
import json
import sys

with open(".openrecord/checks.json", encoding="utf-8") as f:
    data = json.load(f)
item = data["checks"][0]
justified = item.get("justified-by", "").strip()
justified = justified[2:] if justified.startswith("./") else justified
want = "decisions/api/delivery/arch-linter.md"
if justified != want:
    print(f"justified-by = {justified!r}, want {want!r}")
    sys.exit(1)
PY
```

```check
# C4: enforces names either the decisions/api/structure group coordinate or
# the layers.md record directly.
python3 - <<'PY'
import json
import sys

with open(".openrecord/checks.json", encoding="utf-8") as f:
    data = json.load(f)
item = data["checks"][0]
enforces = [e[2:] if e.startswith("./") else e for e in item.get("enforces", [])]
wanted = {"decisions/api/structure", "decisions/api/structure/layers.md"}
if not (set(enforces) & wanted):
    print(f"enforces = {enforces}, want at least one of {sorted(wanted)}")
    sys.exit(1)
PY
```

```check
# C5: the transcript shows at least one Bash tool_use whose command is
# `openrecord check add ...` — inspecting tool_use commands only, never
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
            if "openrecord check add" in command:
                found = True
if not found:
    print("no Bash call to `openrecord check add` found in the transcript")
    sys.exit(1)
sys.exit(0)
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

```check
# C7: openrecord validate exits 0 with no error-severity finding
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

1. **J1**: `justified-by` is linked to the tool decision (`arch-linter.md`) because that is the decision
   that justifies running `go-arch-lint` at all — not to the layering decision, and not left pointing at
   nothing.
2. **J2**: `enforces` is linked to the structure decision (the group, or `layers.md` directly) for the
   right reason: go-arch-lint enforces the rule that decision states (the domain never imports
   infrastructure), not merely because the prompt said "enforce the structure decisions" and the model
   pattern-matched on the word "structure".
3. **J3**: The model did not invent or substitute a different tool than `go-arch-lint` — the one the
   person actually named.
4. **J4**: The model did not add a `{files}` placeholder to the command. This is a whole-project,
   pre-merge-only check (`go-arch-lint check`, no subset of touched files), not an on-change one, and the
   model's own words (if it explained its choice) reflect that distinction correctly rather than treating
   `{files}` as a detail to add out of habit.
5. **J5**: The model did not put anything manual or statistical into the registry — no `VERIFIED-IN`-
   style wording, no attempt to register something that is not this one deterministic command.
6. **J6**: The model's final report to the user accurately describes what it registered — the command,
   the id it chose, `justified-by`, and `enforces` — rather than a vague or inflated account.

## Failure signals

- A tool other than `go-arch-lint` registered, or a command that does not match what the person said
  almost verbatim (`go-arch-lint check`).
- A `{files}` placeholder added to the command, turning a pre-merge-only check into a claimed on-change
  one with no basis for it.
- `justified-by` pointing at the structure decision, `enforces` pointing at the delivery decision, or
  both pointing at the same decision — the two links swapped or collapsed into one.
- More than one check created, or a check created with no `enforces` at all when a clear target
  (`decisions/api/structure`) was available and named in the prompt.
- `.openrecord` edited directly with `Write`/`Edit`/`NotebookEdit` instead of through `openrecord check
  add`.
- The model's final report claims a check was registered that `checks.json` does not actually contain.

## Verdict log

- (none yet)

- 2026-10-14: PASS — checked: 7/7  judged: 6/6. Run 20261014T160803Z-09-checks. 2.1.296 (Claude Code) as orlab.
