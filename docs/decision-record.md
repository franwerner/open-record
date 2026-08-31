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

**Eleven concerns — the folder vocabulary.** `structure`, `domain-logic`, `runtime`, `data`,
`data-lifecycle`, `contracts`, `integration`, `security`, `observability`, `delivery`, `quality`. This is
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

### What is deliberately not here

- **`date`** — git knows when the file was created and last touched, and cannot drift. A hand-written
  date lies the moment someone edits without updating it. If *when* a decision was taken matters, that
  is context and belongs in the prose.
- **`component`, `concern`** — carried by the path.
- **relations (`related`, `supersedes`)** — a list of other records goes stale without anyone noticing,
  and it is the one thing in a record that breaks when another one is renamed or moves down a level.
  Where one record genuinely bears on another, the prose says so in words: nothing is written down that
  has to be repaired later, and there are already two ways to reach a record — descend with `map`, or
  find it with `grep`.

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
