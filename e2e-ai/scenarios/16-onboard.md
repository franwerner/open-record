# AI flow scenario: 16-onboard

## Task

Task 16 renamed the skill that interviews a user to populate an empty store from
`openrecord-bootstrap` to `openrecord-onboard` — the verb-first naming every other skill already
follows — and reordered that interview: components, then actors, then behaviour (`flow`/`process`,
also `rule`/`lifecycle`), then decisions (walking the concerns catalogue, now asking whether each
decision exists because of a spec already written, to fill `serves`), then `quality` specs. Actors
move ahead of behaviour because `initiated-by` is required on every `flow` and `process` spec; decisions
move after behaviour so `serves` can be filled in at the point of writing instead of guessed at or left
for later. See `skills/openrecord-onboard/SKILL.md` ("The interview, in order") for the settled
sequence. The deterministic half of this proof is `internal/cli/skills_test.go`
(`TestSkillsReportsTheRenamedOnboardSkillAsRemovedAndCreated`), which pins the rename mechanics — an
existing installation picks up the new name and drops the old one — directly against the binary. This
file proves the interview order through a real agent asked to set up an empty store for a small
file-sharing app from one paragraph that mixes an actor-triggered flow, a webhook-triggered process, and
two decisions together: it has to interview in the new order and connect at least one decision back to
the spec it exists because of.

## Lab precondition

```setup
mkdir -p src/api src/web && touch src/api/.gitkeep src/web/.gitkeep
openrecord component add api --path src/api --title "API" --description "The HTTP surface."
openrecord component add web --path src/web --title "Web app" --description "The browser application."

git init -q
git add -A
git -c user.name="orlab" -c user.email="orlab@local" commit -q -m "initial store: api and web components only"
```

## Prompts

`openrecord-onboard` interviews step by step and confirms "each one... before it is written — not in a
batch at the end" (see the skill's "Nothing gets written without confirmation"). A run against an
earlier draft of this scenario showed exactly that: three prompts got the agent through components and
actors and partway into asking about the upload flow, never reaching a spec or decision write, so C1–C4
passed only vacuously (nothing to check yet) and the Lab precondition's own components came back
hand-edited instead of going through `openrecord component edit` — the gap that command now closes. This
version answers each step the skill actually asks about, in the order it asks, and follows every answer
with its own explicit confirmation, so the conversation has enough turns to legitimately reach
behaviour and decisions rather than stalling on open questions.

```prompt
Set up openrecord for this project. It's a file-sharing app: registered users upload files and share
links to them; a payment provider calls our webhook to confirm plan upgrades. Uploads go straight to S3
with presigned URLs so the API never receives the file bytes, and we keep accounts in Postgres.
```

The agent's first answer typically asks about the two components already declared (descriptions, and
whether a separate `repo` or infrastructure component is needed) and about actors (whether an
unregistered visitor needs its own actor, what to call the webhook's actor, whether S3 notifies the API
back, scheduled jobs, an admin role). This prompt settles all of it at once, the way a person answering
a list of questions would, and only explicitly rules out what was never asked for.

```prompt
Just api and web — no separate repo or infrastructure component for now. For api, use this description:
"The HTTP surface. Descend here if you touch endpoints, the payment webhook, presigned-URL issuance, or
the Postgres account model." For web: "The browser application. Descend here if you touch the upload UI,
share-link pages, or account and plan screens." For actors: only registered users can open a share link,
so there's no separate visitor actor. The webhook is an external system — call it payment-provider. No
S3 event notifications back to the API, no scheduled jobs, and no admin or support role.
```

```prompt
Yes, write those.
```

Next, behaviour: the upload flow. The skill asks what the system does — the happy path, the limits, and
what the actor sees when it fails — before proposing a `flow` spec.

```prompt
For the upload flow, done by a registered user: the web app asks the API for a presigned URL; the API
checks the file size against a 50 MB limit and issues the URL if it's under that; the browser PUTs the
file straight to S3; once that succeeds the browser tells the API, which marks the file uploaded. If the
file is over 50 MB, the person sees "This file is larger than the 50 MB limit." If the presigned URL
expires before the upload finishes, they see "This upload link expired — try again."
```

```prompt
Yes, write that.
```

Then the payment-webhook process — triggered by the payment provider, not by an actor inside a session,
so it needs its own answer before it can be a `process` spec.

```prompt
For the payment provider's webhook: it calls our endpoint with a subscription id and the new plan; we
verify the call is genuinely from them, then update the account's plan. If we can't verify it, we ignore
it and log that it happened.
```

```prompt
Yes, write that.
```

Finally the two decisions the opening prompt already named, including why the first one exists: it is
the mechanism behind the upload flow just written, not a free-standing choice.

```prompt
Now the two decisions. Uploads go straight from the browser to S3 with a presigned URL, so the API
never receives the file bytes — that's the reason the upload flow works the way it does, so it exists
because of it; we chose it over proxying uploads through the API, which would mean streaming large files
through our own servers for no benefit. And we keep accounts in Postgres rather than a document store,
because the account and plan data is relational and small, and the team already knows SQL well.
```

An agent that already wrote everything after an earlier prompt in this sequence just says so, for this
and every confirmation above.

```prompt
Yes, write those.
```

## Checked criteria

```check
# C1: the first `openrecord actor add` Bash call comes before the first
# `openrecord record write specs/flow` or `specs/process` call — actors have
# to exist first because initiated-by is required on both. Robust to the
# agent declaring actors one at a time: only the FIRST actor-add call and the
# FIRST flow/process write call are compared.
python3 - "$TRANSCRIPT" <<'PY'
import json
import re
import sys

path = sys.argv[1]
actor_idx = None
behavior_idx = None
idx = 0
actor_re = re.compile(r"openrecord\s+actor\s+add\b")
behavior_re = re.compile(r"openrecord\s+record\s+write\s+[\"']?specs/(flow|process)\b")

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
            if "openrecord" not in command:
                continue
            idx += 1
            if actor_idx is None and actor_re.search(command):
                actor_idx = idx
            if behavior_idx is None and behavior_re.search(command):
                behavior_idx = idx

if behavior_idx is None:
    print("no `openrecord record write specs/flow` or `specs/process` call found — "
          "the scenario describes an actor-triggered upload and a webhook-triggered "
          "confirmation, so at least one of each type is expected")
    sys.exit(1)
if actor_idx is None:
    print("no `openrecord actor add` call found at all, but a flow/process spec was written")
    sys.exit(1)
if actor_idx > behavior_idx:
    print(f"first actor add was call #{actor_idx}, after the flow/process spec write at #{behavior_idx}")
    sys.exit(1)
print(f"actor add at call #{actor_idx}, first flow/process write at call #{behavior_idx}")
sys.exit(0)
PY
```

```check
# C2: the first `openrecord record write specs/...` call comes before the
# first `openrecord record write decisions/...` call — decisions are
# interviewed after behaviour so `serves` can point at something that exists.
python3 - "$TRANSCRIPT" <<'PY'
import json
import re
import sys

path = sys.argv[1]
spec_idx = None
decision_idx = None
idx = 0
spec_re = re.compile(r"openrecord\s+record\s+write\s+[\"']?specs/")
decision_re = re.compile(r"openrecord\s+record\s+write\s+[\"']?decisions/")

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
            if "openrecord" not in command:
                continue
            idx += 1
            if spec_idx is None and spec_re.search(command):
                spec_idx = idx
            if decision_idx is None and decision_re.search(command):
                decision_idx = idx

if spec_idx is None:
    print("no `openrecord record write specs/...` call found at all")
    sys.exit(1)
if decision_idx is None:
    print("no `openrecord record write decisions/...` call found — the scenario describes two "
          "decisions (S3 presigned uploads, Postgres for accounts), so at least one is expected")
    sys.exit(1)
if spec_idx > decision_idx:
    print(f"first spec write was call #{spec_idx}, after the first decision write at #{decision_idx}")
    sys.exit(1)
print(f"first spec write at call #{spec_idx}, first decision write at call #{decision_idx}")
sys.exit(0)
PY
```

```check
# C3: at least one `record write decisions/...` call carried --serves naming
# a spec this session also wrote, and the decision file on disk really
# carries that spec in its `serves` frontmatter.
result="$(python3 - "$TRANSCRIPT" <<'PY'
import json
import re
import sys

path = sys.argv[1]
write_re = re.compile(r"record\s+write\s+(\S+)")
serves_re = re.compile(r"--serves\s+(\S+)")

spec_writes = []
decision_serves = []

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
            if "openrecord" not in command or "record write" not in command:
                continue
            match = write_re.search(command)
            if not match:
                continue
            target = match.group(1).strip("\"'")
            if target.startswith("specs/"):
                spec_writes.append(target)
            elif target.startswith("decisions/"):
                serves = [s.strip("\"'") for s in serves_re.findall(command)]
                if serves:
                    decision_serves.append((target, serves))

for decision_path, serves in decision_serves:
    for s in serves:
        if s in spec_writes:
            print(f"DECISION={decision_path}")
            print(f"SPEC={s}")
            sys.exit(0)

print("no decision write carried --serves naming a spec written in this session", file=sys.stderr)
print("spec_writes: " + repr(spec_writes), file=sys.stderr)
print("decision_serves: " + repr(decision_serves), file=sys.stderr)
sys.exit(1)
PY
)"
status=$?
echo "$result"
if [ "$status" -ne 0 ]; then exit 1; fi

decision="$(echo "$result" | grep '^DECISION=' | cut -d= -f2-)"
spec="$(echo "$result" | grep '^SPEC=' | cut -d= -f2-)"
f=".openrecord/$decision"
if [ ! -f "$f" ]; then echo "decision file not found on disk: $f"; exit 1; fi
grep -qF "$spec" "$f" || { echo "$f does not carry $spec in its serves frontmatter"; cat "$f"; exit 1; }
exit 0
```

```check
# C4: at least one flow spec and at least one process spec exist on disk —
# grounding C1: both the upload (actor-triggered) and the webhook
# confirmation (event-triggered) are expected to land as specs with an actor.
# No -maxdepth: a spec may legitimately sit inside a subgroup.
flow_count="$(find .openrecord/specs/flow -name '*.md' ! -name 'INDEX.md' 2>/dev/null | wc -l | tr -d ' ')"
process_count="$(find .openrecord/specs/process -name '*.md' ! -name 'INDEX.md' 2>/dev/null | wc -l | tr -d ' ')"
if [ "$flow_count" -lt 1 ]; then echo "expected at least one flow spec, found $flow_count"; exit 1; fi
if [ "$process_count" -lt 1 ]; then echo "expected at least one process spec, found $process_count"; exit 1; fi
exit 0
```

```check
# C5: actors.json declares at least one human actor (the registered user
# uploading) and at least one external-system actor (the payment provider's
# webhook) — the two kinds the prompt actually describes.
python3 -c "
import json
with open('.openrecord/actors.json') as f:
    data = json.load(f)
kinds = {a['kind'] for a in data['actors']}
import sys
sys.exit(0 if 'human' in kinds and 'external-system' in kinds else 1)
" || { echo "actors.json is missing a human or an external-system actor"; cat .openrecord/actors.json; exit 1; }
exit 0
```

```check
# C6: openrecord validate exits 0 with no error-severity finding
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

1. **J1**: Before writing a record (a spec, a decision, or the quality spec if it proposed one), the
   model asked how it would be verified, or stated an approach itself (a check, a sandbox, or a declared
   `VERIFIED-IN`/manual note) — not silently skipped, and not only brought up after the fact.
2. **J2**: The model did not invent actors, decisions, or behaviour the prompt never described — no
   extra role (an "admin"), no extra flow or process (a notification system, an analytics pipeline), no
   extra decision (a specific database version, a caching layer) beyond registered users uploading and
   sharing, the payment provider's webhook, presigned S3 uploads, and Postgres for accounts.
3. **J3**: The model's final report accurately describes what it wrote (which actors, which specs, which
   decisions, and the `serves` link) rather than a vague or inflated account, and does not claim a record
   exists that is not actually on disk.

## Failure signals

- A `flow` or `process` spec written, or attempted, before any actor existed to name in `initiated-by`.
- An actor invented beyond "a registered user" and "the payment provider" (or equivalent wording the
  prompt supports) — an "admin", a generic "system", or similar.
- The S3-presigned-upload decision or the Postgres decision written with no `--serves` at all, when the
  prompt ties each directly to the upload behaviour it exists because of.
- Any decision written before a behaviour spec existed for it to possibly serve.
- `.openrecord` edited directly with `Write`/`Edit`/`NotebookEdit` instead of through `openrecord`.
- The model never asked about or stated a verification approach for anything it wrote.

## Verdict log

- 2026-10-21: PASS — checked: 7/7  judged: 3/3. Run 20261021T074413Z-16-onboard. 2.1.296 (Claude Code) as orlab.
