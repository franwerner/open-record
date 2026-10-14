# AI flow scenario: 04-quality

## Task

Task 04 asked for a fifth spec type, `quality`, for a non-functional attribute
of behaviour that already exists — one that needs a percentile, a load or a measurement window rather
than a per-case yes or no — so it stops ending up either buried inside a functional spec's edge cases
or disguised as a decision. Settled and already implemented (see `docs/capability-spec.md` "The five
types" and "The `quality` type", `internal/check/body.go`, `internal/check/quality.go`,
`internal/store/record.go`):

- A fifth spec folder, `specs/quality/`, with the same core sections every spec has — `## Purpose` and
  `## Scenarios` — and no type-specific section of its own; anything else is `unexpected-section`.
- A `quality` scenario is the common `### Scenario: <name>` shape, anchored exactly like any other
  (no special `[Qn]` identifier), plus a fourth required field, `MEASURE`. Missing it is
  `malformed-scenario`, the same code a missing GIVEN/WHEN/THEN gets.
- An optional, repeatable `- **VERIFIED-IN** <value> — <where to look>` line, `value` one of
  `pre-release`, `production`, `manual`. A scenario with none gets the warning `quality-unverified`; an
  out-of-domain value is `verified-in-invalid`; the line on anything other than a `quality` spec is
  `verified-in-not-quality`.
- An optional, whole-file frontmatter list `qualifies`, linking the spec to the functional spec(s) (or
  spec#anchor) it characterizes, validated against what is actually on disk: a missing target is
  `qualifies-not-found`, a missing anchor is `qualifies-anchor-not-found`, a target that is itself a
  `quality` spec is `qualifies-not-functional`, and `qualifies` on a non-`quality` spec is
  `qualifies-not-quality`.

The deterministic half of this proof is `e2e/quality_test.go`, exercising exact codes against the built
binary over a realistic fixture, plus `internal/check/quality_test.go` for the parsing/validation logic
directly. This file proves the same behaviour through a real agent deciding, on its own, that a
performance target belongs in a new `quality` spec rather than in the flow it is about.

## Lab precondition

```setup
mkdir -p src/app && touch src/app/.gitkeep
openrecord component add app --path src/app \
  --title "App" --description "The application."

openrecord level add specs/flow

openrecord record write specs/flow/upload-file.md \
  --title "Upload a file" --description "A user uploads a file; it is stored and confirmed." \
  --status accepted --components app \
  --body-file <(cat <<'EOF'
## Purpose

A user uploads a file, so it is stored and available afterward.

## Main flow

1. The user selects a file and starts the upload.
2. The system stores the file.
3. The system confirms the upload to the user.

## Scenarios

### Scenario: Successful upload

- **GIVEN** a user with a file under the size limit
- **WHEN** they upload it
- **THEN** the upload completes and is confirmed
EOF
)

git init -q
git add -A
git -c user.name="orlab" -c user.email="orlab@local" commit -q -m "initial store: one component, one flow spec for file upload"
```

## Prompts

```prompt
We want uploading a 50 MB file to take under 10 seconds at the p95, and we'll measure it with a
nightly load test in CI. Record that.
```

## Checked criteria

```check
# C1: a new file exists under .openrecord/specs/quality/
count="$(find .openrecord/specs/quality -name '*.md' ! -name 'INDEX.md' 2>/dev/null | wc -l)"
if [ "$count" -lt 1 ]; then
  echo "no record was written under .openrecord/specs/quality/"
  exit 1
fi
exit 0
```

```check
# C2: the flow spec is byte-identical to what setup created — not modified
f=.openrecord/specs/flow/upload-file.md
before="$SNAPSHOT_0/$f"
if [ ! -f "$before" ]; then echo "missing snapshot $before"; exit 1; fi
if ! cmp -s "$before" "$f"; then
  echo "the flow spec changed"
  diff -u "$before" "$f" || true
  exit 1
fi
exit 0
```

```check
# C3: no new decision record was created
count="$(find .openrecord/decisions -name '*.md' ! -name 'INDEX.md' 2>/dev/null | wc -l)"
if [ "$count" -ne 0 ]; then
  echo "a decision record was created, but this is behaviour someone using the product would notice:"
  find .openrecord/decisions -name '*.md' ! -name 'INDEX.md'
  exit 1
fi
exit 0
```

```check
# C4: the quality spec's qualifies points at specs/flow/upload-file.md, whole or by #successful-upload
f="$(find .openrecord/specs/quality -name '*.md' ! -name 'INDEX.md' | head -1)"
if [ -z "$f" ]; then echo "no quality spec to check"; exit 1; fi
if ! grep -qE 'qualifies:.*specs/flow/upload-file\.md(#successful-upload)?' "$f"; then
  echo "qualifies does not point at specs/flow/upload-file.md:"
  grep '^qualifies' "$f" || echo "(no qualifies field at all)"
  exit 1
fi
exit 0
```

```check
# C5: the body has a MEASURE line mentioning p95 and 10 (seconds)
f="$(find .openrecord/specs/quality -name '*.md' ! -name 'INDEX.md' | head -1)"
if [ -z "$f" ]; then echo "no quality spec to check"; exit 1; fi
line="$(grep -E '\*\*MEASURE\*\*' "$f" || true)"
if [ -z "$line" ]; then echo "no MEASURE line found"; exit 1; fi
if ! grep -qi 'p95' <<<"$line"; then echo "MEASURE does not mention p95: $line"; exit 1; fi
if ! grep -q '10' <<<"$line"; then echo "MEASURE does not mention 10: $line"; exit 1; fi
exit 0
```

```check
# C6: a VERIFIED-IN line with value pre-release
f="$(find .openrecord/specs/quality -name '*.md' ! -name 'INDEX.md' | head -1)"
if [ -z "$f" ]; then echo "no quality spec to check"; exit 1; fi
if ! grep -qE '\*\*VERIFIED-IN\*\*\s+pre-release\b' "$f"; then
  echo "no VERIFIED-IN pre-release line found:"
  grep 'VERIFIED-IN' "$f" || echo "(no VERIFIED-IN line at all)"
  exit 1
fi
exit 0
```

```check
# C7: openrecord validate exits 0 with no errors
out="$(openrecord validate)"
status=$?
echo "$out"
if [ "$status" -ne 0 ]; then echo "validate exited $status"; exit 1; fi
if grep -q '"severity":"error"' <<<"$out" || grep -q '"severity": "error"' <<<"$out"; then
  echo "validate reported an error finding"
  exit 1
fi
exit 0
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

1. **J1**: The answer explains, or its actions show, that this is a `quality` spec because the target
   needs a percentile/load measurement rather than a plain per-case yes or no — not a `flow` edge case
   and not a decision record.
2. **J2**: The `MEASURE` line faithfully states the target asked for (a 50 MB upload, under 10 seconds,
   at the p95) rather than a vaguer or different number.
3. **J3**: The `VERIFIED-IN` line's free text plausibly describes "a nightly load test in CI" — it does
   not have to be a verbatim quote, but it should name something like a scheduled/nightly CI run.
4. **J4**: The model's final report to the user accurately describes what it wrote (the new `quality`
   spec, its path, and that it links to the upload flow) rather than a vague or inflated account.

## Failure signals

- A `## Edge cases` or similar section was added to `specs/flow/upload-file.md` instead of a new spec.
- A decision record was written for this performance target.
- `qualifies` is missing, or points somewhere other than the upload flow.
- `MEASURE` is absent, or states a different number than asked (not p95, not 10 seconds, not 50 MB).
- `.openrecord` was edited directly with `Write`/`Edit`/`NotebookEdit` instead of through `openrecord`.
- The model claims the record was written when `openrecord validate` still reports an error.

## Verdict log

- (none yet)

- 2026-10-13: PASS — checked: 8/8  judged: 4/4. Run 20261013T231246Z-04-quality. 2.1.296 (Claude Code) as orlab.
