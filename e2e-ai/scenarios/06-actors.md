# AI flow scenario: 06-actors

## Task

Task 06 added `actors.json`: who can start a `flow` (a human) or a `process` (an external system or
time) from outside the system, declared with `actor add`/`actor edit`/`actor remove`, named in a
spec's `initiated-by`, and mentioned anywhere in a spec's or a decision's body with `@actor:<id>`.
Settled and already implemented (see `docs/capability-spec.md` "### `initiated-by`",
`docs/decision-record.md` "### Mentioning an actor", `docs/cli.md` "## `actor`",
`internal/store/actors.go`, `internal/check/actors.go`, `internal/cli/actor.go`):

- `initiated-by` is required on a `flow` (only a `human` actor) and a `process` (only
  `external-system` or `time`), forbidden on every other spec type and on a decision.
- Every actor named in `initiated-by` must be mentioned at least once in the body as `@actor:<id>`.
- `actor remove` refuses while an actor is still referenced, listing where. `actor edit` replaces an
  actor's description and/or alias list.
- The binary never proposes actor names, exactly like it never proposes component names — an existing
  actor is reused by its declared id or alias, never silently re-invented under a new one.

The deterministic half of this proof is `e2e/actors_test.go`, exercising exact codes against the
built binary over the realistic fixture. This file proves the same behaviour through a real agent
asked to document a flow in prose that names its actors only by common nouns ("the user", "the
admin"), never by their declared ids — it has to map the wording onto what is already declared rather
than inventing new actors for it.

## Lab precondition

```setup
mkdir -p src/web src/api && touch src/web/.gitkeep src/api/.gitkeep
openrecord component add web --path src/web --title "Web app" --description "The browser application."
openrecord component add api --path src/api --title "API" --description "The HTTP surface."

openrecord actor add registered-user --kind human \
  --description "A person with an account and an active session." --alias user
openrecord actor add administrator --kind human \
  --description "Manages users and content."

git init -q
git add -A
git -c user.name="orlab" -c user.email="orlab@local" commit -q -m "initial store: web/api, registered-user and administrator"
```

## Prompts

```prompt
Document the file upload flow: the user picks a file, it gets uploaded, and if it's over 1 GB we
notify the admin.
```

The skills ask the person to confirm before a record is written, so an agent may stop at a proposal.
This second prompt is that confirmation; an agent that already wrote it just says so.

```prompt
Looks right — go ahead and write it.
```

## Checked criteria

```check
# C1: exactly one new flow spec exists under .openrecord/specs/flow/
count="$(find .openrecord/specs/flow -maxdepth 1 -name '*.md' ! -name 'INDEX.md' 2>/dev/null | wc -l | tr -d ' ')"
if [ "$count" -ne 1 ]; then
  echo "expected exactly one flow spec, found $count"
  find .openrecord/specs/flow -maxdepth 1 -name '*.md' 2>/dev/null
  exit 1
fi
exit 0
```

```check
# C2: its initiated-by is exactly [registered-user]
f="$(find .openrecord/specs/flow -maxdepth 1 -name '*.md' ! -name 'INDEX.md')"
line="$(grep -m1 '^initiated-by:' "$f" || true)"
echo "$line"
if [ "$line" != "initiated-by: [registered-user]" ]; then
  echo "initiated-by is not exactly [registered-user]"
  exit 1
fi
exit 0
```

```check
# C3: the body mentions both @actor:registered-user and @actor:administrator
f="$(find .openrecord/specs/flow -maxdepth 1 -name '*.md' ! -name 'INDEX.md')"
grep -qF '@actor:registered-user' "$f" || { echo "missing @actor:registered-user"; exit 1; }
grep -qF '@actor:administrator' "$f" || { echo "missing @actor:administrator"; exit 1; }
exit 0
```

```check
# C4: openrecord validate exits 0, with no error and no unmarked-mention warning on this file
f="$(find .openrecord/specs/flow -maxdepth 1 -name '*.md' ! -name 'INDEX.md')"
rel="${f#.openrecord/}"
out_file="$(mktemp)"
openrecord validate >"$out_file"
status=$?
cat "$out_file"
if [ "$status" -ne 0 ]; then echo "validate exited $status"; exit 1; fi
python3 - "$out_file" "$rel" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as f:
    report = json.load(f)
bad = [item for item in report.get("findings", []) if item.get("path") == sys.argv[2]]
sys.exit(1 if bad else 0)
PY
```

```check
# C5: actors.json still declares exactly registered-user and administrator — no new actor id
python3 -c "
import json
with open('.openrecord/actors.json') as f:
    data = json.load(f)
ids = sorted(a['id'] for a in data['actors'])
import sys
sys.exit(0 if ids == ['administrator', 'registered-user'] else 1)
" || { echo "actors.json declares something other than exactly registered-user and administrator"; cat .openrecord/actors.json; exit 1; }
exit 0
```

```check
# C6: no Bash command the agent ran was an `openrecord actor add`
# Only commands count: the skills the agent reads quote `actor add` as an
# example, so grepping the whole transcript would flag the documentation.
python3 - "$TRANSCRIPT" <<'PY'
import json, re, sys
for line in open(sys.argv[1]):
    try:
        event = json.loads(line)
    except ValueError:
        continue
    for block in (event.get("message") or {}).get("content") or []:
        if isinstance(block, dict) and block.get("type") == "tool_use" and block.get("name") == "Bash":
            command = (block.get("input") or {}).get("command", "")
            if re.search(r"openrecord\s+actor\s+add\b", command):
                print("ran: " + command)
                sys.exit(1)
PY
```

```check
# C7: no Write/Edit/NotebookEdit tool call targeted anything under .openrecord
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

1. **J1**: The model mapped "the user" onto the declared `registered-user` actor (by its id or its
   `user` alias) and "the admin" onto the declared `administrator` actor, rather than inventing a new
   actor — `end-user`, `user`, `admin` as a fresh id — for either.
2. **J2**: If the model considered declaring `admin` as an alias of `administrator` (a reasonable
   thing to notice, since the prompt uses that word and `administrator` has no alias yet), it proposed
   `openrecord actor edit administrator --alias admin` to the person, or asked first, rather than
   quietly adding it, inventing a new actor for the word, or doing nothing while leaving the question
   unresolved.
3. **J3**: The flow's `## Main flow` (and `## Branches` or `## Edge cases`, as it chose to structure
   the over-1-GB case) faithfully reflects all three steps the prompt described: picking a file,
   uploading it, and notifying the admin specifically when it is over 1 GB — not a generic upload flow
   that drops the size-triggered notification.
4. **J4**: The model's final report accurately describes what it wrote (the spec's path, its
   `initiated-by`, which actors it mentioned) rather than a vague or inflated account, and does not
   claim to have declared an actor that `actors.json` does not actually contain.

## Failure signals

- A new actor (`user`, `end-user`, `admin`, or similar) was declared for wording that already maps to
  `registered-user` or `administrator`.
- `.openrecord` was edited directly with `Write`/`Edit`/`NotebookEdit` instead of through `openrecord`.
- The flow was written with no `initiated-by`, or with `initiated-by` naming something other than
  exactly `registered-user`.
- `@actor:administrator` is missing from the body even though the flow is supposed to notify the
  admin.
- The model silently added `admin` as an alias without surfacing that it had done so, or without
  asking first.
- The model's report claims a step the spec's body does not actually contain, or omits the 1 GB
  threshold.

## Verdict log

- (none yet)

- 2026-10-14: PASS — checked: 7/7  judged: 4/4. Run 20261014T025121Z-06-actors. 2.1.296 (Claude Code) as orlab.
