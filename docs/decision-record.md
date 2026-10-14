# Decision record

A decision record captures **what was chosen and why**: the trade-off, the alternative that was
rejected, and what the choice leaves governing the code.

> Status of this document: settled.

## What a decision IS

Three traits, and all three must hold.

1. **It decides** — it picks one option over others. It does not describe, and it does not execute.
2. **It has a why** — the context and the trade-off that justify the choice.
3. **It endures as a constraint** — it governs the code that comes next, not only the code that was
   there.

Three shapes all qualify: a real **trade-off** (which persistence engine), a **convention** of style or
structure (how modules are named), and a **policy** (rate limiting is applied at the gateway, not in
each handler). The format does not tag which is which — the concern says what area it belongs to, and
that is enough.

## What a decision is NOT

The rule that does the most work here: **most work settles nothing.** A framework that pushes one
record per change produces a store nobody reads.

- **Not a unit of work.** A task runs and finishes; it decides nothing. Most work is mechanical and
  deserves no record.
- **Not a verification.** An acceptance criterion confirms something works. It has no why, no rejected
  alternative, and it does not endure as a constraint.
- **Not a signal or a reminder.** *"We still need to decide X"*, a TODO, a note — those point at an
  absent decision. They are the finger, not the decision. (If leaving something undecided is itself
  deliberate, that is a record with `status: pending`.)
- **Not incidental code.** A pattern that shows up a handful of times with no clear intent is noise.
  With no evidence of a deliberate choice, there is no decision.
- **Not the how.** Implementation detail lives in the code.
- **Not observable behaviour** — see the boundary below.
- **Not a guessed why.** Composing a plausible reason for a decision nobody stated is inventing, and a
  record whose rationale was reconstructed to fill the section is **worse than no record, because it
  will be trusted.** Where you do not have the why — and reading it off existing code rarely gives it
  to you — say so, rather than filling the gap with a story that reads ratified.

### The boundary against a capability spec

One test: **would someone who only *uses* the product notice?**

- **Yes** → it is behaviour. It belongs in a [capability spec](capability-spec.md).
- **No** → it is a decision. It belongs here.

The two often coexist over the same topic, and that is where the test earns its keep. The spec fixes
the limit facing the actor — *"past N req/min you get a 429"*. The decision fixes the mechanism and why
that one — *"rate limiting is applied at the gateway, not in each handler"*.

### No volatile identifiers, anywhere

The body has no anchor section — no globs, no assertions — so there is nowhere safe to put a concrete
internal name. Which makes the rule simple: **the prose never names a class, a method, a column, an
internal error or a file path.** A record that does dies at the next rename, silently, while still
reading as true.

Names that survive are the ones an external consumer would also use: a technology, a public endpoint, a
contract header, an exposed error code. A technology name *is* the decision and belongs in it. A method
or a column is only how it was implemented.

| Traced from the code ❌ | Written as a concept ✅ |
| --- | --- |
| *transitions happen via `markActive()` / `markDone(resultId)`* | *state transitions happen only through the entity, never by setting the state by hand* |
| *table `orders` with `id`, `tenantId` FK, `refId` nullable* | *the operation is persisted with an audit trail: identity, owner, optional reference, failure reason* |
| *`BaseError` → `ExpiredError` under `src/features/*/domain/errors/`* | *business errors are their own hierarchy; provider errors are translated at the adapter boundary* |

## Where it lives

```
.openrecord/decisions/<component>/<concern>/[<subgroup>/]<record>.md
```

Three axes, all carried by the path, never by a header field:

| Axis | What it says | Source |
| --- | --- | --- |
| `component` | Which surface of the project the decision governs (`api`, `ui`, `cli`, `root`). | Declared in `components.json`. Declaring one creates its folder under `decisions/`, even empty. |
| `concern` | What kind of decision it is (`structure`, `runtime`, `data`, `security`…). | Chosen from the shipped catalogue, or invented. Either way it is a folder, and it must declare when to descend into it. |
| `subgroup` | Optional. Groups a cluster of records on a specific topic, to give them shared context. | Named freely, for whatever the cluster is about. **One level only** — never nested. |

`root` is the default component name for projects that declare nothing else. It is a component like
any other, not a catch-all.

### The concerns catalogue

openrecord ships a catalogue of concerns. It lives with the binary, **never in the project**, and it
creates nothing: it is a document you consult — with `openrecord concerns` — and the store only ever
holds concerns that actually have a record in them.

It has two levels, and each does a different job.

**Ten concerns — the folder vocabulary.** `structure`, `domain-logic`, `runtime`, `data`,
`data-lifecycle`, `contracts`, `integration`, `security`, `observability`, `delivery`. This is
what a writer picks from when filing a record. A project that needs one the catalogue does not have
simply invents it, and writes its `INDEX.md` without having anything to copy from.

Some sit next to each other closely enough to be confused, and the catalogue says where the line is:
`domain-logic` is how the business is modelled while `structure` is how the code is organised;
`integration` is what the system consumes while `contracts` is what it exposes; `data-lifecycle` is
what happens to data over time while `data` is its shape and access.

**Their topics — the discovery checklist.** Under each concern, the catalogue lists the places
projects usually have a decision worth recording: *rate-limiting*, *caching*, *error-handling*,
*folder-structure*, *feature-flags*, and so on. **These name nothing.** They are there so an agent
facing an empty component knows what to ask — *did anyone decide something about rate limiting? about
caching?* — rather than staring at a blank folder.

Subgroups are **not** taken from this list. A subgroup is named for whatever topic its cluster shares,
and that is a judgement made by whoever writes it.

### Decisions are closed by component

A decision never spans components. If `api` and `cli` reach the same conclusion, those are **two
records**, one in each component — because the context that justifies each is different, and each
component has its own way of doing things.

This is what lets `decisions/api/` be read on its own, without dragging the rest of the project in.

## Frontmatter

```yaml
---
title: Business errors are their own hierarchy
description: Business errors are modelled as their own hierarchy; provider errors are translated at the adapter boundary.
status: accepted
body-hash: 50d858e0985ecc7f60418aaf0cc5ab587f42c2570a884095a9e8ccacd0f6545c
---
```

A decision that exists because of a spec, or that only makes sense while another decision holds, adds
one or both of `serves` and `depends-on` — see [Relations](#relations) below:

```yaml
---
title: pgx as the database driver
description: Database access uses pgx directly, without an ORM.
status: accepted
serves:
  - specs/flow/upload-file.md
depends-on:
  - decisions/api/data/postgres.md
body-hash: ...
---
```

### `title`

The record's title. It lives here and **not** as an `# H1` in the body — one source, so the two can
never drift apart, and a reader gets it without parsing markdown.

### `description`

**The decision itself, in one line.** Not the area it covers.

This property does the heaviest work in the format. There are no hand-written index files that list
records: the listing is *generated* from these descriptions, the way a host builds its list of
available skills. So the description is what an agent reads to decide whether to open the file — and
if it only names the area (*"how errors are modelled"*), the agent has to open the file anyway to know
whether its work contradicts the record, and the description saves nothing.

Write the decision, not the topic:

| | |
| --- | --- |
| ✅ | *Business errors are their own hierarchy; provider errors are translated at the adapter boundary.* |
| ❌ | *How errors are modelled and propagated.* |

### `status`

Two values, and only two:

- **`accepted`** — the default. This is what governs the code and what work is verified against.
- **`pending`** — someone explicitly said "leave this undecided". A pending record settles nothing, so
  it constrains nothing and is never verified against.

There is no approval queue and no third state. Deliberately absent:

- **`proposed`** — nothing sits waiting to be ratified.
- **`rejected`** — a rejected option is an alternative, and it lives inside the record that rejected
  it, not as a file of its own.
- **`superseded` / `deprecated`** — you edit the record in place. Git carries the history.

### `body-hash`

The SHA-256 of the body only — never the frontmatter — hex-encoded, always last in the frontmatter.
**Stamped by the tool, never written by hand:** `record write` computes it when the record is created,
and `record edit` recomputes and re-stamps it on every edit, including one that touches only `title`,
`description` or `status`. `validate` reports a body that no longer matches its stamped value, so a
change made outside the tool becomes detectable without depending on git or on any agent harness.

A writer never recomputes this blindly over whatever is already on disk: `record edit` refuses, before
touching anything, when the existing body no longer matches its stamped `body-hash` — recomputing it
first would silently re-stamp a hand-edit into a hash that matches again, which is exactly what this
mechanism exists to catch. `record write`, which replaces an existing file whole with no flag, applies
the same refusal when a file already sits at that path. Either refusal reports `body-hash-mismatch` and
leaves the file byte-for-byte as it was; the only way out is to restore the original content (`git
checkout`, typically), never to force a re-stamp.

### `hash` — the same mechanism, for JSON files {#hash}

Every JSON file the binary writes inside `.openrecord/` carries the same idea as `body-hash`, under the
field name `hash`: a SHA-256 hex digest the binary stamps on write and `validate` checks on read, so a
hand-edit to a JSON file is just as detectable as a hand-edit to a record's body. `components.json` is
the first file to carry it; `actors.json`, `checks.json`, `sandbox.json`, and the files under
`implementation/` and `runs/` adopt it unchanged as the issues that introduce them land
(`internal/store/integrity.go` is the one implementation every one of them reuses).

**The formula**, defined once here so no later issue has to invent it again:

1. Take the value about to be written, with its `hash` field cleared. An empty string excludes the
   field from the JSON entirely, because the field is declared with `omitempty`.
2. Serialize it the same way the binary always does: `json.MarshalIndent(value, "", "  ")`.
3. Append the one trailing newline every file the binary writes ends with.
4. The hash is the SHA-256 of those exact bytes, hex-encoded.

The binary then sets `hash` to that value and serializes a second time — now with the field present, so
it lands as the very last key, the same visual convention `body-hash` follows at the end of a record's
frontmatter:

```json
{
  "version": "1",
  "components": [
    { "id": "api", "paths": ["src/api"] }
  ],
  "hash": "3f9a2c..."
}
```

To verify a file already on disk, parse it, clear its `hash` field, and recompute the same digest — and
then also require the file to be byte-for-byte what serializing the parsed value produces. The second
step is not optional: parsing drops any key the format does not declare, so a field added by hand would
vanish before the digest is recomputed and the file would pass on the hash alone. The binary is the only
writer and always writes the same bytes for the same value, so any byte it would not have written —
an unknown key, a reordered one, reformatted whitespace — means something else wrote the file. Two
findings cover the ways that check fails, used by every JSON file this mechanism protects:

| Code | Severity | What it catches |
| --- | --- | --- |
| `json-hash-missing` | Error | The file has no `hash` field at all. |
| `json-hash-mismatch` | Error | The hash no longer matches the content, or the file is not byte-for-byte what the binary writes — it was edited outside the tool. |

**`validate` reports both**, for `components.json` today and for every later JSON file this mechanism
covers, as part of its normal recursive walk. Files under `runs/` are a deliberate exception: they are
gitignored working data, not part of the coordinate `validate` walks by default, so an old or corrupt
run never blocks a `validate` over records that have nothing to do with it; the binary still stamps and
can check their `hash`, just not from `validate` itself (see issue 15).

**A writer refuses rather than re-stamping.** `component add` and `component remove` check
`components.json`'s hash before touching it: a missing hash or a mismatch both refuse the write with the
matching code, leaving the file untouched, the same way `record edit` refuses over a tampered
`body-hash`. The fix is always to restore the original file — `git checkout .openrecord/components.json`
is the usual case — never to patch the `hash` field by hand or ask the tool to re-stamp over the
tampered content.

**Read-only commands are unaffected.** `map`, `grep`, `search`, `diagram` and anything else that only
reads keep working against a `components.json` with a missing or mismatching hash: reading it does not
launder a tamper the way writing over it would. Only `validate` and a writer refuse.

### Relations: `serves` and `depends-on` {#relations}

Two optional fields, both lists of **record paths**, both on a decision and never on a spec — a spec
declaring either is an error. Each points at the whole file; neither ever carries a `#anchor`, because
there is nothing narrower than a decision to point *at* either.

```yaml
serves:
  - specs/flow/upload-file.md
depends-on:
  - decisions/api/data/postgres.md
```

A reference goes stale the moment its target is renamed or moved — the exact failure mode that kept
relations out of the format originally (see "What is deliberately not here" below). What changed is that
the binary now validates every reference on write and on every `validate`, so a stale one is reported,
not silent. That is what makes these two links safe to add back, for exactly these two, and no others.

**`serves`** names the spec, functional or quality, this decision exists because of. It is optional: a
decision with no spec behind it (team knowledge, cost, a licence — nothing a user would notice) carries
none, and in that case the why has to be developed in `## Context` instead.

**`depends-on`** names another decision, with one strict meaning: **this decision only makes sense while
that one stays `accepted`.** The test: if the other decision changed, would this one need
re-evaluating? If the answer is no, it is not a `depends-on` — it is never a generic "related", and two
decisions in the same component and concern are not linked this way just for being close together; the
store's own structure and the neighbourhood (`map`) already surface that proximity.

Both are read regardless of which component or concern the target lives in or the one carrying the
field lives in — a decision in `api` can depend on one filed under `root`.

**The inverse of each is computed, never written.** Asking "what depends on this?" or "what serves this
spec?" by hand-maintaining the other side is exactly the staleness this section already rejected once.
Instead, `record write` and `record edit` report it in their JSON reply: a decision's `dependents` —
every other decision that depends on it, directly or transitively — and a spec's `served_by` — every
decision whose `serves` names it. Neither blocks anything; both are informational, there for whoever
just changed a decision to see what else has to be looked at again.

**What is validated**, by `record write`, `record edit` and `validate` alike:

| Situation | Code |
| --- | --- |
| `serves` names a record that does not exist | `serves-not-found` |
| `serves` names something other than a spec | `serves-not-spec` |
| `depends-on` names a record that does not exist | `depends-on-not-found` |
| `depends-on` names something other than a decision | `depends-on-not-decision` |
| `depends-on` names a decision that is `pending`, not `accepted` | `depends-on-pending` |
| `depends-on` forms a cycle, direct or transitive | `depends-on-cycle` |

A decision with no `serves` is never flagged by the binary — whether that is honest depends on whether
`## Context` carries a real why, and that is a judgement for a person or a writing skill, not a
mechanical check.

### Mentioning an actor: `@actor:<id>` {#actors}

A decision carries no `initiated-by` — nothing initiates a choice the team made, it is never started by
anyone — but it may still **mention** an actor anywhere in its body, exactly like a capability spec:

```markdown
## Decision
Permissions are checked in a middleware before every handler. @actor:administrator carries a role of
its own; @actor:support can read but never change another user's data.
```

This is often where an actor matters most: `authorization`-shaped decisions are usually *about* who can
do what, and naming the actor the same way a spec does lets a later reader (or `openrecord impact`, once
it exists) answer "what decisions talk about this actor?" without reading every record in the store.

`@actor:<id>` is validated the same way here as in a spec: an id `actors.json` does not declare is
`actor-mention-unknown`, and an actor's id or alias written in the open without the mention syntax is
the warning `unmarked-actor-mention`. See [cli.md](cli.md#actor) for the full convention, `actor
add`/`actor edit`/`actor remove`, and the pattern that keeps an email address from ever matching.

**If an actor is removed, every mention of it becomes an error at the next `validate`.** Better still,
`actor remove` refuses to delete an actor still mentioned anywhere — in an `initiated-by` or in a bare
`@actor:` — and lists where, the same principle `component remove` already follows for a component.

### What is deliberately not here

- **`date`** — git knows when the file was created and last touched, and cannot drift. A hand-written
  date lies the moment someone edits without updating it. If *when* a decision was taken matters, that
  is context and belongs in the prose.
- **`component`, `concern`** — carried by the path.
- **a generic "related" link, or `supersedes`** — unlike `serves`/`depends-on` above, a bare "related"
  carries no test for when it applies, so it gets used for everything and stops meaning anything; and
  `supersedes` is unnecessary because you edit the record in place and git carries the history. Where
  one record genuinely bears on another in a way that is not `serves` or `depends-on`, the prose says so
  in words — there are already two ways to reach a record even without a written link: descend with
  `map`, or find it with `grep`.

## Body

Four sections, and nothing else.

### `## Context`

What situation forced a choice. Without it the record cannot be re-evaluated when the situation
changes — and re-evaluation is the only thing that keeps a store of decisions from becoming a museum.

### `## Decision`

The choice, developed. The `description` states it in one line; this is where it is explained.

### `## Alternatives`

What was rejected, and why. **This is what separates a decision from a rule someone wrote down**: with
no rejected alternative, there was no choice.

### `## Consequences`

What you accept by choosing this — the cost that came with the benefit.

### No internal anchors

A decision does not repartition its content into addressable parts the way a [capability
spec](capability-spec.md#anchors) does for a scenario or a transition: it is one undivided statement, a
*what* and a *why*, and there is nothing narrower than the whole file to point at. The same holds for a
`rule` spec — see [capability-spec.md](capability-spec.md#anchors).

So a reference to either one names the file and stops there, with no `#`:

```
decisions/api/structure/layers.md
specs/rule/usage-limits.md
```

## Recording a deliberate absence

"We looked at this and there is nothing to decide" is itself a decision, with a *what* and a *why*, and
it governs the code just as much (nobody adds auth to the api, because it sits behind the gateway). So
it is a normal record with `status: accepted`. There is no special mechanism for it.

The distinction matters because a generated listing can only show what exists. Without such a record, a
reader cannot tell "nobody ever looked at this" from "someone looked and it does not apply" — and those
call for opposite behaviour.

### What the body deliberately does not have

Two sections were considered and dropped, and the reason is the same for both: **the record is prose,
and nothing in it is machine-checkable.**

- **`## Scope` with globs** — pinning a decision to files (`src/**/domain/errors/**`). The component
  already ties it to a surface and `components.json` already holds that surface's globs, so the section
  would only narrow *within* the component — redundant in most cases, and one more glob that rots
  silently when a folder moves.
- **`## Verifiable rules`** — a list of checkable assertions with `[auto]` / `[manual]` marks. It would
  have made drift measurable, but someone has to run those checks, and that someone is not this
  framework.

The consequence is worth stating plainly: `validate` can check that a record is **well-formed** — its
frontmatter present and its status one of the two allowed values, the components a spec names actually
declared, its sections exactly the ones its kind asks for, a branch anchored to a step that exists, a
scenario carrying all three of its parts — and never that it is **still true of the code**. Judging
whether a decision still holds is reading, not linting.
