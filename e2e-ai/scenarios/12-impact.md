# AI flow scenario: 12-impact

## Task

Task 12 added `openrecord impact <record>`: given a record's path, it runs `validate` over the whole
store first, then reports that record's full neighbourhood — every declared, validated link one hop
away (`qualifies`, `serves`, checks, `@record:` annotations), and `depends-on` followed as a full chain
in both directions, `depends_on` (what the record rests on) and `dependents` (what rests on it).
Settled and already implemented (see `docs/cli.md#impact`, `internal/cli/impact.go`,
`internal/check/relations.go`, `internal/check/checks.go`, `internal/check/sandbox.go`,
`internal/check/quality.go`): JSON only, no `--human`; an error anywhere in the store refuses with no
neighbourhood; a nonexistent record is a usage error. The deterministic half of this proof is
`e2e/impact_test.go`, exercising every section and the chain depth against the built binary over a
store built from scratch. This file proves the same behaviour through a real agent asked what it would
need to re-evaluate before replacing a base decision — it has to follow the `depends-on` chain all the
way to its end (not stop after one hop), name the spec served along the way, and read a `pending`
decision resting on the same base as open, not as something already broken.

## Lab precondition

```setup
mkdir -p src/api
openrecord component add root --path . --title "Root" --description "The whole repository, not owned by any one surface."
openrecord component add api --path src/api --title "API" --description "The HTTP surface."

openrecord actor add visitor --kind human \
  --description "A person with no account yet."

openrecord level add decisions/root/data
openrecord level add decisions/api/data
openrecord level add specs/flow

openrecord record write decisions/root/data/postgres.md \
  --title "Postgres is the system of record" \
  --description "Account and order data is stored in Postgres, not in a document store." \
  --status accepted \
  --body-file <(cat <<'EOF'
## Context

The service needs one durable store for account and order data, with real transactions across both.

## Decision

Postgres is the system of record for the whole service.

## Alternatives

A document store was considered and dropped: orders touch several related rows at once, and the
service would end up re-implementing transactions on top of it.

## Consequences

Every schema change goes through a migration tool, and every service that needs this data talks to
Postgres directly or through the API, never through a second copy of it.
EOF
)

openrecord record write specs/flow/sign-up.md \
  --title "Sign-up" \
  --description "A visitor registers with an email and password." \
  --status accepted \
  --components api \
  --initiated-by visitor \
  --body-file <(cat <<'EOF'
## Purpose

A @actor:visitor registers for an account.

## Main flow

1. The @actor:visitor submits an email and password.
2. The system creates the account and stores it.

## Scenarios

### Scenario: Account is created

- **GIVEN** a @actor:visitor with a free email address
- **WHEN** they submit the sign-up form
- **THEN** the account is created
EOF
)

openrecord record write decisions/api/data/pgx.md \
  --title "pgx is the Postgres driver" \
  --description "Database access from the API goes through pgx, never database/sql." \
  --status accepted \
  --depends-on decisions/root/data/postgres.md \
  --serves specs/flow/sign-up.md \
  --body-file <(cat <<'EOF'
## Context

The API needs a Postgres driver with native support for the types and the connection pooling the
service relies on.

## Decision

The API talks to Postgres through pgx, not database/sql with a generic driver underneath.

## Alternatives

database/sql plus lib/pq was considered and dropped: it is in maintenance mode, and pgx's native
protocol support is measurably faster for the service's query shapes.

## Consequences

Every query in the API goes through pgx's own API, not database/sql's generic one.
EOF
)

openrecord record write decisions/api/data/atlas-migrations.md \
  --title "Schema changes go through Atlas" \
  --description "Every schema change is an Atlas migration, applied in order, never a hand-run SQL script." \
  --status accepted \
  --depends-on decisions/api/data/pgx.md \
  --body-file <(cat <<'EOF'
## Context

Schema changes need to be ordered, reviewable, and applicable the same way in every environment.

## Decision

Every schema change is authored as an Atlas migration and applied in order; nobody runs SQL against
the database by hand.

## Alternatives

Hand-run SQL scripts were considered and dropped: nothing orders them or proves they ran everywhere in
the same sequence.

## Consequences

A schema change that is not an Atlas migration does not exist, as far as any other environment is
concerned.
EOF
)

openrecord record write decisions/root/data/read-replica.md \
  --title "A read replica might help reporting" \
  --description "Not decided: whether reporting queries need their own replica, separate from Postgres." \
  --status pending \
  --depends-on decisions/root/data/postgres.md \
  --body-file <(cat <<'EOF'
## Context

Reporting queries are getting heavier, and it is not yet clear whether they need to run against a
replica instead of the primary.

## Decision

Undecided. Whoever picks this up next needs real numbers on reporting's actual load before choosing.

## Alternatives

Running reporting against the primary, unchanged, is the default until this is decided.

## Consequences

None yet — nothing has been chosen.
EOF
)

git init -q
git add -A
git -c user.name="orlab" -c user.email="orlab@local" commit -q -m \
  "initial store: root/api components, postgres, sign-up flow, pgx, atlas-migrations, a pending read-replica decision"
```

## Prompts

```prompt
We're considering replacing Postgres with MySQL (decisions/root/data/postgres.md). What would we need
to re-evaluate? Don't change anything.
```

## Checked criteria

```check
# C1: the transcript shows at least one Bash tool_use whose command is
# `openrecord impact ...` — inspecting tool_use commands only, never
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
            if "openrecord impact" in command:
                found = True
if not found:
    print("no Bash call to `openrecord impact` found in the transcript")
    sys.exit(1)
sys.exit(0)
PY
```

```check
# C2: nothing under .openrecord/ changed — the task only asked what would
# need re-evaluating, never to change anything.
if [ -n "$(git status --porcelain -- .openrecord)" ]; then
  echo "the store changed, and the task said not to change anything:"
  git status --porcelain -- .openrecord
  git diff -- .openrecord
  exit 1
fi
```

## Judged criteria

1. **J1**: The model's final answer names all three decisions in the `depends-on` chain —
   `decisions/root/data/postgres.md`, `decisions/api/data/pgx.md`, and
   `decisions/api/data/atlas-migrations.md` — including the third one, two hops from Postgres itself.
   Stopping after only `pgx.md` (one hop) is a failure: the chain is exactly what `impact`'s
   `dependents` exists to follow all the way to its end.
2. **J2**: The model's final answer includes `specs/flow/sign-up.md`, the spec `pgx.md` serves —
   reached through `served_by`/`serves`, not invented or guessed from the file tree.
3. **J3**: The model treats `decisions/root/data/read-replica.md` as open and undecided — a `pending`
   decision resting on Postgres that settles nothing yet — not as something already broken, contradicted,
   or itself requiring a fix before Postgres can be replaced.

## Failure signals

- The model never ran `openrecord impact`, and instead grepped or searched the store by hand to answer.
- The final answer stops at `decisions/api/data/pgx.md` and never names `atlas-migrations.md`.
- The final answer omits `specs/flow/sign-up.md` entirely.
- The final answer describes `decisions/root/data/read-replica.md` as broken, contradicted, or blocking
  — rather than as a `pending`, undecided question.
- Anything under `.openrecord/` was written, edited, or otherwise changed.
- The model proposed or started the MySQL migration itself, rather than only reporting what would need
  re-evaluating.

## Verdict log

- 2026-10-16: PASS — checked: 2/2  judged: 3/3. Run 20261016T213948Z-12-impact. 2.1.296 (Claude Code) as orlab.
