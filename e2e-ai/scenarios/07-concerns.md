# AI flow scenario: 07-concerns

## Task

Task 07 rewrote `docs/concerns.md`: ten concerns instead of eleven, the `quality` concern dissolved
(its `nfr-performance` topic retired, `scalability` moved to `runtime/scaling`, `i18n-l10n` moved to
`domain-logic/i18n-l10n`), twelve new topics added, and several existing ones reformulated to ask only
about the mechanism — what an actor notices or what gets measured against a threshold now points at a
spec instead. `security/rate-limiting` is one of the reformulated topics: it used to also ask "what is
a caller told when they hit the limit", which is now explicitly a spec's job (see `docs/concerns.md`
"security", and the worked example in `docs/decision-record.md` "The boundary against a capability
spec" and `docs/cli.md` "`record write`"). The deterministic half of this proof is
`internal/catalogue/catalogue_test.go` and `e2e/concerns_test.go`, which pin down the catalogue's shape
directly. This file proves the same split through a real agent handed one sentence that mixes all
three records together, and has to pull it apart: what the caller sees (a `rule`), what gets measured
(a `quality` spec), and the mechanism and why (a decision under `security/rate-limiting`).

## Lab precondition

```setup
mkdir -p src/api && touch src/api/.gitkeep
openrecord component add api --path src/api --title "API" --description "The HTTP surface."

git init -q
git add -A
git -c user.name="orlab" -c user.email="orlab@local" commit -q -m "initial store: api component only"
```

## Prompts

```prompt
We decided to apply rate limiting at the gateway, not in each handler: past 100 requests per minute a
client gets a 429 with Retry-After, and applying the limit must not add more than 5 ms at the p99.
Record that.
```

The skills ask the person to confirm before a record is written, so an agent may stop at a proposal.
This second prompt is that confirmation; an agent that already wrote it just says so.

```prompt
Looks right — go ahead and write it.
```

## Checked criteria

```check
# C1: exactly one new decision exists under .openrecord/decisions/api/security/
# (a security/rate-limiting/ subgroup is an accepted shape, not a separate concern)
count="$(find .openrecord/decisions/api/security -name '*.md' ! -name 'INDEX.md' 2>/dev/null | wc -l | tr -d ' ')"
if [ "$count" -ne 1 ]; then
  echo "expected exactly one decision under decisions/api/security, found $count"
  find .openrecord/decisions/api/security -name '*.md' 2>/dev/null
  exit 1
fi
exit 0
```

```check
# C2: exactly one new functional spec (rule preferred; flow/process acceptable)
# exists, and it owns the 429 / Retry-After contract facing the actor
files="$(find .openrecord/specs/rule .openrecord/specs/flow .openrecord/specs/process \
  -name '*.md' ! -name 'INDEX.md' 2>/dev/null)"
count="$(echo -n "$files" | grep -c . || true)"
if [ "$count" -ne 1 ]; then
  echo "expected exactly one functional spec (rule/flow/process), found $count"
  echo "$files"
  exit 1
fi
f="$(echo "$files" | head -1)"
grep -qF '429' "$f" || { echo "functional spec is missing 429: $f"; exit 1; }
grep -qF 'Retry-After' "$f" || { echo "functional spec is missing Retry-After: $f"; exit 1; }
exit 0
```

```check
# C3: exactly one new quality spec exists, and its MEASURE line owns the p99 / 5 ms threshold
count="$(find .openrecord/specs/quality -name '*.md' ! -name 'INDEX.md' 2>/dev/null | wc -l | tr -d ' ')"
if [ "$count" -ne 1 ]; then
  echo "expected exactly one quality spec, found $count"
  find .openrecord/specs/quality -name '*.md' 2>/dev/null
  exit 1
fi
f="$(find .openrecord/specs/quality -name '*.md' ! -name 'INDEX.md')"
line="$(grep -E '\*\*MEASURE\*\*' "$f" || true)"
if [ -z "$line" ]; then echo "no MEASURE line found in $f"; exit 1; fi
echo "$line"
grep -qi 'p99' <<<"$line" || { echo "MEASURE does not mention p99: $line"; exit 1; }
grep -Eqi '5[[:space:]]*ms\b' <<<"$line" || { echo "MEASURE does not mention 5 ms: $line"; exit 1; }
exit 0
```

```check
# C4: the decision's serves names both new specs
d="$(find .openrecord/decisions/api/security -name '*.md' ! -name 'INDEX.md')"
func="$(find .openrecord/specs/rule .openrecord/specs/flow .openrecord/specs/process \
  -name '*.md' ! -name 'INDEX.md' 2>/dev/null | head -1)"
qual="$(find .openrecord/specs/quality -name '*.md' ! -name 'INDEX.md' 2>/dev/null | head -1)"
if [ -z "$d" ] || [ -z "$func" ] || [ -z "$qual" ]; then
  echo "missing one of: decision=$d functional=$func quality=$qual"
  exit 1
fi
func_rel="${func#.openrecord/}"
qual_rel="${qual#.openrecord/}"
grep -qF "$func_rel" "$d" || { echo "serves is missing $func_rel"; cat "$d"; exit 1; }
grep -qF "$qual_rel" "$d" || { echo "serves is missing $qual_rel"; cat "$d"; exit 1; }
exit 0
```

```check
# C5: the decision's ## Decision section names the mechanism — the gateway
d="$(find .openrecord/decisions/api/security -name '*.md' ! -name 'INDEX.md')"
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
if "gateway" not in section.lower():
    print("## Decision section does not name the gateway")
    sys.exit(1)
PY
```

```check
# C6: the decision's ## Decision section does not restate the actor-facing or
# measured numbers as its own content — those belong to the functional and
# quality specs, not here. (A mention in ## Context, citing why this was
# written, is not what this checks.)
d="$(find .openrecord/decisions/api/security -name '*.md' ! -name 'INDEX.md')"
python3 - "$d" <<'PY'
import re
import sys

path = sys.argv[1]
text = open(path, encoding="utf-8").read()
match = re.search(r"^## Decision\n(.*?)(?=^## |\Z)", text, re.S | re.M)
if not match:
    print("no ## Decision section found")
    sys.exit(1)
section = match.group(1).lower()
print(section)
for marker in ("429", "retry-after", "100 req", "100 requests"):
    if marker in section:
        print(f"## Decision restates {marker!r} instead of leaving it to the functional/quality specs")
        sys.exit(1)
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

```check
# C8: no Write/Edit/NotebookEdit tool call targeted anything under .openrecord
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

1. **J1**: The model followed the three-way split rather than writing one record for the whole
   sentence: what the caller sees (429, Retry-After, the 100 requests/minute threshold) went to a
   functional spec; what is measured against a number (the 5 ms added at the p99) went to a `quality`
   spec; the mechanism and why (the gateway, not each handler) went to a decision filed under
   `security` (a `rate-limiting` subgroup or a plain file, either is fine).
2. **J2**: No record duplicates another's content wholesale — the decision explains *why* the gateway
   was chosen without re-deriving the 429 contract or the latency budget, and neither spec restates the
   other's content.
3. **J3**: The model did not invent a rationale the user never gave. The prompt says *"at the gateway,
   not in each handler"* and nothing else about why — no capacity argument, no specific technology, no
   extra justification the user did not state. If the model added anything beyond that, it either
   marked it clearly as its own inference (a reasonable thing to flag) or asked, rather than presenting
   it as something the user said.
4. **J4**: The model's final report accurately describes what it wrote — which record went where, and
   why — rather than a vague or inflated account, and does not claim a record exists that is not
   actually on disk.

## Failure signals

- One record for the whole sentence — a decision whose body also carries the 429/Retry-After contract
  or the 5 ms/p99 budget, with no separate functional or quality spec.
- A `quality` spec that does not exist, or exists but omits `MEASURE`, because the latency number was
  folded into the decision's `## Consequences` instead.
- The decision filed under a concern other than `security` (`rate-limiting` is a `security` topic in
  the revised catalogue).
- A fabricated rationale presented as the user's own ("for cost reasons", "because the gateway is
  Envoy", or similar) when the prompt never said so.
- `.openrecord` edited directly with `Write`/`Edit`/`NotebookEdit` instead of through `openrecord`.

## Verdict log

- (none yet)

- 2026-10-14: PASS — checked: 8/8  judged: 4/4. Run 20261014T104349Z-07-concerns. 2.1.296 (Claude Code) as orlab.
