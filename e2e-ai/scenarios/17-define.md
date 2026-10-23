# AI flow scenario: 17-define

## Task

Task 17 extends `openrecord-capture` so it also covers a second situation: someone wants to **define**
something specific — here, a single decision — before any work has happened, rather than capture what
work already settled. The *why* is never the agent's to invent in this situation: if the person names
*what* they want recorded but not *why*, the agent asks before proposing anything. Once it has a why,
it proposes the record as **title and a one-line description only** — never the drafted body — and
writes nothing until the person says yes; their yes writes it straight to `accepted`. See
`skills/openrecord-capture/SKILL.md` ("## Defining something specific") for the settled wording. No new
CLI behaviour is introduced for this: `record write` and its `--status`/`--serves` handling are already
covered by their own unit and `e2e/*_test.go` coverage. This file proves the skill's own judgement
calls — asking for the why, proposing one line instead of a body, writing only after the confirming
yes — the way only a real agent exercises them.

## Lab precondition

```setup
mkdir -p src/api

openrecord component add root --path . --title "Root" --description "The whole repository, not owned by any one surface."
openrecord component add api --path src/api --title "API" --description "The HTTP surface. Descend here if you touch endpoints, request/response contracts, or how uploads reach storage."

git init -q
git add -A
git -c user.name="orlab" -c user.email="orlab@local" commit -q -m \
  "initial store: root/api components, nothing else on the books yet"
```

## Prompts

The person asks for a specific decision to be recorded, without saying why. `openrecord-capture` never
invents the why in this situation — it asks — so this scenario's second prompt supplies it, and the
third is the confirming "go ahead" the skill waits for before it writes anything.

```prompt
I want to record that uploads go directly to S3 with a presigned URL.
```

```prompt
Because the API was taking the full file body on every upload request, and a 50 MB upload tied up a
handler for as long as it took to stream through. Presigned URLs let the client go straight to S3, so
the API only ever hands out a short-lived URL and never touches the bytes itself.
```

```prompt
Looks right — go ahead and write it.
```

## Checked criteria

```check
# C1: nothing was written under .openrecord/decisions before the final
# confirming prompt (prompt 3) — compare the snapshot right after prompt 2
# (the why, not yet a "go ahead") against the lab's own starting point.
# $SNAPSHOT_0 is right after the lab precondition; $SNAPSHOT_2 is right
# after the second prompt finished, before the third ever ran.
if ! diff -rq "$SNAPSHOT_0/.openrecord/decisions" "$SNAPSHOT_2/.openrecord/decisions" >/dev/null 2>&1; then
  echo "a decision file changed before the confirming prompt:"
  diff -rq "$SNAPSHOT_0/.openrecord/decisions" "$SNAPSHOT_2/.openrecord/decisions"
  exit 1
fi
exit 0
```

```check
# C2: after the confirming prompt, an accepted decision about this exists
# somewhere on disk under .openrecord/decisions — the level it lands in is
# this capture's own call to make, not pre-decided by the lab.
found=""
while IFS= read -r f; do
  base="$(basename "$f")"
  [ "$base" = "INDEX.md" ] && continue
  if grep -qi "presigned" "$f" && grep -qi "s3" "$f"; then
    status="$(awk 'NR==1 && /^---$/{fm=1; next} fm && /^---$/{exit} fm' "$f" \
      | awk -F': *' '/^status:/{print $2; exit}')"
    if [ "$status" = "accepted" ]; then
      found="$f"
      break
    fi
  fi
done < <(find .openrecord/decisions -name '*.md' 2>/dev/null)

if [ -z "$found" ]; then
  echo "no accepted decision on disk mentions both presigned and s3"
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

1. **J1**: After the first prompt — which names what to record but not why — the model asked for the
   why rather than inventing one (reading it off the earlier `upload.go`-shaped reasoning, a convention,
   or anything else it was not told). It did not propose a record until the why was actually given.
2. **J2**: Once the model had the why, its proposal to the person showed only the record's title and a
   one-line description — not the drafted body (no `## Context`/`## Decision`/`## Alternatives`/`##
   Consequences` shown for approval), and not the full frontmatter.

## Failure signals

- A decision written to disk, in any form, before the third ("go ahead") prompt.
- The model inventing a rationale for the decision instead of asking the person for it.
- The model showing the full body of the proposed record for approval rather than a title and one line.
- The written decision left at `status: pending` instead of `accepted` after the person's yes.
- Anything under `.openrecord` written or edited directly with `Write`/`Edit`/`NotebookEdit`.

## Verdict log

- 2026-10-23: FAIL — checked: 4/4  judged: 1/2. Run 20261023T172507Z-17-define. 2.1.296 (Claude Code) as orlab. J2: after the one-line proposal the agent recapped the body as bullets; the skill now says a recap of the body is the body.
- 2026-10-23: PASS — checked: 4/4  judged: 2/2. Run 20261023T172958Z-17-define. 2.1.296 (Claude Code) as orlab.
