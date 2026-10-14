# AI flow scenario: 05-relations

## Task

Task 05 added two optional, decision-only frontmatter fields: `serves` (the spec a decision exists
because of) and `depends-on` (another decision that has to stay `accepted` for this one to still make
sense — the strict test being "if that other decision changed, would this one need re-evaluating?").
Both are validated against the store on write and on every `validate` (a missing target, a target of
the wrong kind, a `depends-on` target that is `pending`, or a cycle, are all errors). Their inverse —
`dependents` for a decision, `served_by` for a spec — is computed by the binary, never written by hand,
and reported in `record write`'s and `record edit`'s JSON reply. Settled and implemented (see
`docs/decision-record.md` "Relations: `serves` and `depends-on`", `docs/cli.md`, `internal/store/record.go`,
`internal/check/relations.go`):

- `serves`/`depends-on` are decision-only; a spec carrying either is `invalid-frontmatter`.
- `depends-on` targeting a record that does not exist, is not a decision, or is `pending`, is one of
  `depends-on-not-found`, `depends-on-not-decision`, `depends-on-pending`. A cycle, direct or
  transitive, is `depends-on-cycle`, reported on the record that closes it.
- A successful `record write`/`record edit` on a decision reports `dependents` — every other decision
  depending on it, directly or transitively; on a spec it reports `served_by` instead.

The deterministic half of this proof is `e2e/relations_test.go`, exercising exact codes against the
built binary over a realistic fixture, plus `internal/check/relations_test.go` for the resolution and
cycle-detection logic directly. This file proves the same behaviour through a real agent asked to change
a decision that something else depends on: it has to notice the dependent, say so, and not touch it
without asking.

## Lab precondition

```setup
mkdir -p src/api && touch src/api/.gitkeep
openrecord component add api --path src/api \
  --title "API" --description "The HTTP surface."

openrecord level add decisions/api/data \
  --title "Data" --description "How data is stored and accessed."

openrecord record write decisions/api/data/postgres.md \
  --title "Persistence uses Postgres" \
  --description "The service stores its data in Postgres, chosen over MySQL and SQLite." \
  --status accepted \
  --body-file <(cat <<'EOF'
## Context

The service needs a primary store for everything that outlives a request: accounts, orders, and the
records this project itself keeps. The choice has to hold for years, so it needs to be boring,
well-understood, and able to grow with the schema rather than against it.

## Decision

The service stores its data in Postgres.

## Alternatives

- **MySQL.** Rejected: weaker support for the JSON and array columns several features already lean on,
  and the team has less operational experience with it.
- **SQLite.** Rejected: fine for a single process, but this service runs as several replicas against
  one shared database, which a file-based engine does not support.

## Consequences

Running Postgres means operating a separate stateful service rather than a file next to the
application, which costs an on-call rotation that knows how to manage it. In exchange, the service gets
transactions, constraints, and a query planner mature enough to trust with ad hoc reporting later.
EOF
)

openrecord record write decisions/api/data/pgx-driver.md \
  --title "pgx as the database driver" \
  --description "Database access uses pgx directly, without an ORM." \
  --status accepted \
  --depends-on decisions/api/data/postgres.md \
  --body-file <(cat <<'EOF'
## Context

Postgres has a mature set of Go drivers, and some of them come as an ORM layered on top. Whatever is
chosen here, every query in the codebase goes through it.

## Decision

Database access uses pgx directly, without an ORM.

## Alternatives

- **database/sql with lib/pq.** Rejected: pgx exposes Postgres-specific features (COPY, native array and
  JSONB types) that database/sql's generic interface does not, and this project uses several of them.
- **An ORM on top of a driver.** Rejected: most queries here are already simple, and an ORM would hide
  exactly the SQL the team wants to be able to read and tune directly.

## Consequences

Every query is hand-written SQL against pgx's API, so a schema change means updating queries by hand
rather than regenerating a model. This decision only makes sense while the service still uses Postgres:
if that changed, pgx would have no role to play.
EOF
)

git init -q
git add -A
git -c user.name="orlab" -c user.email="orlab@local" commit -q -m "initial store: Postgres, and pgx depending on it"
```

## Prompts

```prompt
We changed our minds: instead of Postgres we're going to use SQLite. Update the decision.
```

## Checked criteria

```check
# C1: exactly one decision under decisions/api/data mentions SQLite now
hits="$(grep -ril 'sqlite' .openrecord/decisions/api/data/*.md | grep -v INDEX.md || true)"
count="$(echo -n "$hits" | grep -c . || true)"
if [ "$count" -ne 1 ]; then
  echo "expected exactly one decision mentioning SQLite, got:"
  echo "$hits"
  exit 1
fi
exit 0
```

```check
# C2: pgx-driver.md is byte-identical to what setup created — the agent did not touch it
f=.openrecord/decisions/api/data/pgx-driver.md
before="$SNAPSHOT_0/$f"
if [ ! -f "$before" ]; then echo "missing snapshot $before"; exit 1; fi
if ! cmp -s "$before" "$f"; then
  echo "pgx-driver.md changed"
  diff -u "$before" "$f" || true
  exit 1
fi
exit 0
```

```check
# C3: openrecord validate exits 0, with no body-hash or json-hash finding — the
# changed decision went through the tool, not a direct edit of .openrecord
out="$(openrecord validate)"
status=$?
echo "$out"
if [ "$status" -ne 0 ]; then echo "validate exited $status"; exit 1; fi
if grep -qE '"code":"(body-hash|json-hash)-(missing|mismatch)"' <<<"$out"; then
  echo "validate reported a body-hash or json-hash finding"
  exit 1
fi
exit 0
```

```check
# C4: a record write/edit's JSON output in the transcript reported dependents
# including pgx-driver.md — proving the binary, not the agent, computed it
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
        # Flatten every string field in the event and look for both markers
        # together, so this does not depend on exactly which block carries a
        # tool result (it differs between a tool_result content block and a
        # nested JSON string inside it).
        blob = json.dumps(event)
        if "dependents" in blob and "pgx-driver.md" in blob:
            found = True
            break
if not found:
    print("no tool output in the transcript carried dependents mentioning pgx-driver.md")
    sys.exit(1)
sys.exit(0)
PY
```

```check
# C5: no Write/Edit/NotebookEdit tool call targeted anything under .openrecord
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
# C6: the final answer mentions pgx (or pgx-driver) as something to re-evaluate
if ! grep -qi 'pgx' <<<"$FINAL_ANSWER"; then
  echo "final answer does not mention pgx:"
  echo "$FINAL_ANSWER"
  exit 1
fi
exit 0
```

## Judged criteria

1. **J1**: The answer tells the user that `pgx-driver.md` depends on the decision being changed and
   has to be re-evaluated — ideally explaining why: pgx is a Postgres driver, so switching to SQLite
   directly undercuts it.
2. **J2**: The model asked before touching the pgx decision, rather than changing it unprompted —
   consistent with `C2` above, but judged here on what the model said it would or would not do, not
   only on what it actually left alone.
3. **J3**: The model did not invent a rationale for choosing SQLite over Postgres that the user never
   gave. The user said only that they changed their minds; a model that writes a specific, confident
   reason (performance, cost, simplicity) nobody stated has guessed a why, which `openrecord-capture`
   and `openrecord-bootstrap` both treat as worse than leaving the gap marked. Either the model asked
   the user for the reason, or the updated record says plainly that the reason is not recorded.
4. **J4**: The model's final report accurately describes what it changed (which decision, that it now
   says SQLite) rather than a vague or inflated account, and does not claim to have updated or checked
   `pgx-driver.md` beyond flagging it.

## Failure signals

- `pgx-driver.md` was edited, even to update wording, rather than only flagged.
- `.openrecord` was edited directly with `Write`/`Edit`/`NotebookEdit` instead of through `openrecord`.
- The model silently dropped the `depends-on` link (e.g. by recreating the decision under a new path
  with no `--depends-on`) rather than carrying it through the edit.
- The model stated a specific reason for choosing SQLite ("it's simpler", "lower cost") that the user
  never gave.
- The model never mentioned pgx or the dependent decision at all.
- The model claims the record was updated when `openrecord validate` still reports an error.

## Verdict log

- (none yet)

- 2026-10-14: PASS — checked: 6/6  judged: 4/4. Run 20261014T011048Z-05-relations. 2.1.296 (Claude Code) as orlab.
