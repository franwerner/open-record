# Capability spec

A capability spec captures **what the system does**: the observable behaviour of one capability — its
flow, its branches, its edge cases, its rules.

> Status of this document: settled.

## What a capability spec IS

Three traits.

1. **It describes behaviour, not construction** — what the system does, never how it is built.
2. **Every rule is checkable** with a concrete Given / When / Then. A rule that cannot be turned into a
   scenario is a description, not a contract.
3. **It endures as the source of truth** — it states what the capability does today, not what one
   change did to it. It is not a delta.

## What a capability spec is NOT

- **Not a change log.** A spec is edited in place to say what now holds. What it used to say is git's
  job.
- **Not the implementation.** If the sentence stops being true after an internal refactor that no user
  could notice, it was construction, not behaviour.
- **Not a decision** — see the boundary below.

### The boundary against a decision record

One test: **would someone who only *uses* the product notice?**

- **Yes** → it is behaviour. It belongs here.
- **No** → it is a decision. It belongs in a [decision record](decision-record.md).

The two often coexist over the same topic. The spec fixes the limit facing the actor — *"past N req/min
you get a 429"*. The decision fixes the mechanism and why that one — *"rate limiting is applied at the
gateway, not in each handler"*.

## Where it lives

```
.openrecord/specs/<type>/[<subgroup>/]<capability>.md
```

| Axis | What it says | Source |
| --- | --- | --- |
| `type` | Which of the five kinds of behaviour this is. | Fixed set, carried by the path. |
| `subgroup` | Optional. Groups capabilities around the thing they act on (`flow/checkout/`, `lifecycle/orders/`). | Created by the project. **One level only** — never nested. |

There is no `concern` axis here, and no component folder — see below.

### The five types

| Type | What it holds |
| --- | --- |
| `flow` | An operation facing an actor. Someone does something and the system responds. |
| `rule` | A cross-cutting rule with no flow of its own. |
| `lifecycle` | An entity's state machine — which states exist and what moves between them. |
| `process` | Reactive or background work, triggered by an event rather than by an actor. |
| `quality` | A quality attribute of behaviour the other four types already describe — a percentile, a load or a measurement window, never a plain per-case yes or no. |

`flow` is the type that grows: in a mid-sized product it reaches dozens of capabilities, which is why
subgroups exist here at all.

The first four describe **functional** behaviour: what the system does, answerable yes or no for any
concrete case. `quality` is the odd one out — it does not describe a new shape of behaviour, it
characterizes behaviour the other four already describe, which is why it has no sections of its own
(see ["By type"](#by-type) below) and why it is documented in its own section,
["The `quality` type"](#the-quality-type), rather than folded into the table above.

#### Picking between them

The definitions are not enough when the case is ambiguous, and four crossings come up constantly:

- **`flow` vs `process`** — it is not decided by whether it runs in the background. It is decided by
  **what triggers it**: an actor → `flow`; an event → `process`. A signup triggered by a webhook is a
  `process`, even though it does the same thing the manual signup does.
- **`flow` vs `lifecycle`** — the `lifecycle` describes **the machine** (which states exist and what
  moves between them); the flows describe **the operations** that move it. If you find yourself writing
  numbered steps, it is a flow.
- **`rule` vs an edge case** — if it applies to **one** flow, it is a `## Edge cases` entry inside that
  flow. If it applies to several, it is a `rule` of its own.
- **a functional type vs `quality`** — try to write the scenario's `THEN` as a plain fact about one
  concrete case: *"the upload is confirmed"* is functional, no matter how fast or slow it was. The
  moment the `THEN` needs a percentile, a load or a time window to mean anything — *"the upload is
  confirmed in under 10 seconds at the p95"* — it is `quality`, and it belongs in its own spec,
  `qualifies`-linked to the functional one, never mixed into that functional spec's own `## Edge cases`
  or `## Scenarios`.

### Specs cross components

This is the deliberate asymmetry with decision records. A decision is closed inside one component; a
capability is not. A signup that starts in the UI, passes through the API and can also be triggered
from the CLI is **one** capability, not three.

So the component axis is a **frontmatter list**, not a folder — and that is the general rule the format
follows: *a single-valued axis becomes a folder; a multi-valued axis becomes a property.*

## Frontmatter

```yaml
---
title: User signup
description: A visitor registers with email and password; the account stays pending until they verify the email.
status: accepted
components: [api, ui]
body-hash: 50d858e0985ecc7f60418aaf0cc5ab587f42c2570a884095a9e8ccacd0f6545c
---
```

`title`, `description` and `status` work exactly as in a
[decision record](decision-record.md#frontmatter) — the description states the behaviour in one line,
so an agent can decide whether to open the file without opening it, and the status is `accepted` or
`pending` with no third state. `body-hash` is stamped by the tool, never written by hand — see
[decision-record.md](decision-record.md#body-hash) for the field description; it is identical on both
record kinds.

### `components`

**Mandatory, and validated.** It lists the declared surfaces this capability reaches.

- **Mandatory** because a spec with no components cannot be crossed with any path, and drops out of
  every scope check silently.
- **Validated against `components.json`** because a typo (`components: [api, apis]`) never fails on its
  own — it simply stops matching, forever.

This line is the *only* thing the component axis does inside a spec. It is not a way to split the
store: the store always lives at the repository root, and nothing under a component owns records of
its own.

### `qualifies`

**Optional, and only on a `quality` spec.** It links a `quality` spec to the one or more functional
specs it characterizes — a `flow`, `rule`, `lifecycle` or `process`, whole or by anchor:

```yaml
qualifies: [specs/flow/upload-file.md#successful-upload]
```

- **Whole-file, not per scenario.** It governs the whole record. A `quality` spec with two scenarios
  qualifying two different functional behaviours is two mistakes away from clarity — write two separate
  `quality` specs instead.
- **Omitted when the quality attribute is not about one functional behaviour** but about a global
  property of the system — *"99.9% availability"*, *"WCAG AA across the whole interface"*. A `quality`
  spec with no `qualifies` carries only `components`, exactly like one that has it.
- **A different relation from a decision's "serves" link.** The two are named differently on purpose,
  so a report walking a record's neighbourhood never confuses a spec characterizing a behaviour with a
  decision serving one.
- **Validated.** `record write`, `record edit` and `validate` all check it:

  | Situation | Code |
  | --- | --- |
  | The target spec file does not exist | `qualifies-not-found` |
  | The target exists, but the given anchor is not among its anchors | `qualifies-anchor-not-found` |
  | The target is itself a `quality` spec — `qualifies` must point at a functional one | `qualifies-not-functional` |
  | `qualifies` is declared on a spec that is not `quality` | `qualifies-not-quality` |

  All four are errors: a `qualifies` link that resolves to nothing is not a record worth keeping until
  someone notices.

### `initiated-by`

**Required on `flow` and `process`, forbidden on every other type — and on a decision.** It names the
actor(s), declared in `actors.json`, that start this spec from outside the system. The system itself is
never an actor: it is who responds.

```yaml
initiated-by: [visitor, administrator]   # flow: a flow can have more than one actor starting it
initiated-by: [payment-provider]         # process: an external system or time, never a human
```

- **A list**, because more than one actor can start the same flow — a visitor and an administrator both
  uploading a file, for instance.
- **`flow` only accepts a `human` actor.** `process` only accepts `external-system` or `time` — a
  `## Trigger` keeps describing *what happened* in prose; `initiated-by` says only *who* (or what)
  caused it.
- **`lifecycle`, `rule` and `quality` never carry it.** A lifecycle's transitions are fired by the flows
  that describe them, a rule holds for everyone, and a quality spec characterizes behaviour that already
  has its own actor, if any.
- **Validated**, the same way `components` is: `record write`, `record edit` and `validate` all check it.

  | Situation | Code |
  | --- | --- |
  | A `flow` or a `process` with no `initiated-by` | `initiated-by-missing` |
  | `initiated-by` declared on `lifecycle`, `rule`, `quality` or a decision | `initiated-by-forbidden` |
  | The same actor listed twice | `initiated-by-duplicate` |
  | An id not declared in `actors.json` | `initiated-by-unknown-actor` |
  | An actor of the wrong class for the type (a non-`human` initiating a `flow`, a `human` initiating a `process`) | `initiated-by-wrong-kind` |
  | An `initiated-by` actor never mentioned as `@actor:<id>` anywhere in the body | `initiated-by-not-mentioned` |

See [cli.md](cli.md#actor) for `actor add`/`actor edit`/`actor remove` and the `@actor:<id>` mention
convention, which every record — a spec of any type, or a decision — may use in its body regardless of
whether it declares `initiated-by` at all.

## Body

A small fixed core, plus the sections the type asks for. The type is what makes the format ask the
right questions: a `rule` needs two sections, a `flow` needs six, and asking both for the same ten
turns the spec into a form somebody fills in out of obligation.

### The core — every type

**`## Purpose`** — what this capability achieves and for whom. One or two lines: the business outcome,
not the mechanism. It is not the `description` restated — the description says what *happens*, so a
reader can route; the purpose says what it is *for*.

**`## Scenarios`** — the anchor, and what makes the spec verifiable. Every important rule, branch and
edge case has at least one:

```markdown
### Scenario: <name>

- **GIVEN** <initial state>
- **WHEN** <action>
- **THEN** <expected observable outcome>
```

### By type

| Type | Required | Optional |
| --- | --- | --- |
| `flow` | `## Main flow` | `## Branches` · `## Edge cases` · `## Errors facing the actor` |
| `rule` | — | `## Rule` — the invariant, with concrete values |
| `lifecycle` | `## States and transitions` | — |
| `process` | `## Trigger` · `## Main flow` | `## Edge cases` |
| `quality` | — | — |

An absent required section is reported as `missing-section`; an absent optional one is not. `quality`
adds no section of its own — see ["The `quality` type"](#the-quality-type) below — so any section
outside `## Purpose` and `## Scenarios` on a `quality` spec is reported as `unexpected-section`.

- **`## Main flow`** — the happy path, numbered. Each step is one observable action.
- **`## Branches`** — each fork off the happy path, anchored to the step it forks from:
  `- **[<step>] <condition>** → <outcome>`. The step number is the one piece of ceremony the format
  asks for, and it is what makes the branch placeable in a diagram and checkable by `validate`.
- **`## Edge cases`** — limit situations and what the system does in each. This is the section most
  often lost when nobody writes it down, which is the reason it is named rather than left to prose.
- **`## Errors facing the actor`** — what the actor receives on each failure. The observable error
  contract, never the internal exception.
- **`## Rule`** — singular on purpose. The file *is* one rule; a plural section invites several into
  one file, and then the type stops being useful for filtering.
- **`## States and transitions`** — one transition per line, `- <state> → <state> (<what triggers
  it>)`. The entity is already the capability's name. The fixed shape costs nothing: it is how a
  transition gets written anyway.
- **`## Trigger`** — what event starts this. A process is defined by being triggered by an event rather
  than an actor, so this is the fact that constitutes it.

### The `quality` type

A `quality` scenario has no numbered steps, no branches, no states — it rides on the behaviour a
functional spec already describes, via `qualifies`, and asks only how *well* that behaviour has to
hold. Its scenario shape is the common one plus one extra, mandatory field:

```markdown
### Scenario: <name>

- **GIVEN** <initial state>
- **WHEN** <action>
- **THEN** <expected observable outcome>
- **MEASURE** <the percentile, load or window that gives THEN a sense>
```

`MEASURE` is why the type exists: *"the upload completes"* is already true or false per case, but *"p95
< 10 seconds"* is only ever true over many cases. A `quality` scenario missing `MEASURE` (or any of
`GIVEN`/`WHEN`/`THEN`) is `malformed-scenario`, the same code a functional spec gets for the same
problem.

**Anchors follow the shared rule, with no special-cased identifier.** Earlier drafts of this type
considered a `[Q1]`, `[Q2]`... numbering scheme of its own, parallel to a `## Branches` step number.
That was dropped: a `quality` scenario is still a `### Scenario: <name>`, slugged exactly like any
other (see ["Anchors"](#anchors) below), sharing the one anchor namespace every other spec already
shares. One rule for what an anchor is, regardless of which type is asking.

#### `VERIFIED-IN`

An optional, repeatable line after `MEASURE`, for a scenario no deterministic check or sandbox can
cover — only a `quality` spec may carry it. It declares **how the scenario is meant to be verified**,
not that the verification already exists: a record is the desired state, written when the scenario is
defined, usually before the load test or the dashboard it names has been built. The gap between the
two is what reporting (`impact`, `plan`) surfaces, not something to hold the line back for:

```markdown
- **VERIFIED-IN** <value> — <where to look>
```

| Value | When | Examples |
| --- | --- | --- |
| `pre-release` | Before the change reaches users, automatically (CI, a staging environment) | A nightly load test, Lighthouse on every PR, chaos testing in staging |
| `production` | Against the real system with real users | An SLO on an observability dashboard, a field metric |
| `manual` | A person, by hand | A usability test, a screen-reader review |

A value outside those three is `verified-in-invalid`. The free text after the em dash says **where** to
look; the binary never reads it, and never follows it: reading a CI result or a dashboard from an
unsupervised agent would put credentials outside the sandbox within its reach. `VERIFIED-IN` on anything
other than a `quality` spec is `verified-in-not-quality`: a performance note hiding inside a functional
scenario is exactly the mixing this type exists to prevent.

A `quality` scenario with no `VERIFIED-IN` line at all gets the warning `quality-unverified` — it is not
an error, because `record write`/`record edit` still produce a usable record, but it is the real alarm:
nobody has said how this scenario is verified. When the person has said how (a nightly load test in
CI, an SLO, a manual review), write that as `VERIFIED-IN` even if it does not exist yet. Today `VERIFIED-IN` is the *only* way to clear it. Two
more sources will exist once they land: a **check** (issue 09) covering the scenario with `enforces`,
and a **sandbox** (issue 11) approximating it in local with `approximates`. Neither is implemented yet.

Not sure what to write, or what a given attribute's `VERIFIED-IN` usually looks like? `openrecord
verification <attribute>` (for example `openrecord verification latency`) says what the scenario needs
to contain to be verifiable at all, and which of the three values it typically takes.

### Anchors

Every `### Scenario: <name>` has a stable, addressable identifier: its **anchor**. A `lifecycle`
additionally anchors every transition in `## States and transitions`. Nothing else in a spec is
anchorable — a `rule`'s body is one undivided statement, and so is everything outside `## Scenarios`
and `## States and transitions`.

The anchor is a **slug** of the name: lowercase, accents and diacritics stripped (á→a, ñ→n, ü→u),
everything that is not a letter or digit turned into a hyphen, repeated hyphens collapsed, and the
result trimmed at both ends.

An anchor is never stored — every consumer recomputes it from the name — so the slug rule is part of
every reference already written, and it must never change. Accents are stripped by Unicode
decomposition, which Unicode guarantees stable, not by a hand-written table that could grow and move
existing anchors. A letter with no decomposition (`ß`, `ø`, `ł`) is kept as written.

```markdown
### Scenario: Archivo muy grande (más de 50 MB)
```

anchors as `archivo-muy-grande-mas-de-50-mb`, referenced as
`specs/flow/subir-archivo.md#archivo-muy-grande-mas-de-50-mb`.

A `lifecycle` transition anchors as `<source>-to-<target>`, each side slugged the same way:

```markdown
## States and transitions
- pending → paid (the payment is confirmed)
```

anchors as `pending-to-paid`, referenced as `specs/lifecycle/order.md#pending-to-paid`. The
parenthesised note is not part of the anchor.

**Why a name, not a step number.** A branch anchor (`## Branches`) is a step number because a branch
exists to fork off one specific step. A scenario or a transition needs an identifier that survives
editing: inserting a step renumbers everything after it, silently moving any reference that used a
number, while a name does not move when content is inserted around it — if it is renamed or deleted, the
reference breaks in a way that can be detected.

**One namespace per record.** A scenario and a transition share it, so a scenario whose name slugs to
`pending-to-paid` collides with a transition between `pending` and `paid` just as much as it would
collide with another scenario. `record write` and `record edit` reject two anchors that collide — error
`duplicate-anchor` — and so does `validate` on a record already in the store. An anchor pointing nowhere
(a stale reference from outside the record) is a problem for whatever consumes that reference, not for
this check.

A name with no letters or digits at all — nothing survives the slug — has no anchor to give it, reported
as `empty-anchor`.

A `rule` and a decision reference the whole file, with no `#` at all — see
[decision-record.md](decision-record.md#no-internal-anchors).

### Sections that do not apply are deleted

Entirely — never left empty, never `N/A`. An empty section reads as "nobody wrote this yet"; an absent
one reads as "this does not apply here", and those are different facts.

### Diagrams

A spec can be rendered as a Mermaid diagram. Three rules govern this, and the first is the one that
matters:

**The diagram is never written into the record.** `openrecord diagram <spec>` reads the prose and emits
Mermaid on stdout. Nothing is stored, so the diagram is always a *view* and never a second source that
can drift from the first. The cost, stated plainly: opening a spec in a markdown viewer shows no
diagram — you run the command.

| Type | Emits |
| --- | --- |
| `flow` | `flowchart` — the numbered steps as nodes, the branches as decision points |
| `process` | `flowchart`, starting from the trigger |
| `lifecycle` | `stateDiagram-v2` |
| `rule` | Nothing. An invariant is not a drawing. |
| `quality` | Nothing. A measurement is not a drawing either. |

**`flowchart`, not `sequenceDiagram`.** A sequence diagram needs every step to declare who talks to
whom, which would both turn `## Main flow` into a form and force each step to name internal components
— exactly what the vocabulary rule below forbids.

**The grammar buys checking, not only drawing.** Because those two sections parse, `validate` can catch
things no reader would: a branch anchored to a step the flow does not have, or a state that is never
reached or never left. That is a design bug, and today nobody sees it.

### Vocabulary

Every section is written in the domain's language plus the public contract — public endpoints, exposed
error codes. **Never volatile internal identifiers**: classes, methods, database columns, internal
errors, file paths. The *how* belongs to the code; the *why* belongs to a decision record.
