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
| `type` | Which of the four kinds of behaviour this is. | Fixed set, carried by the path. |
| `subgroup` | Optional. Groups capabilities around the thing they act on (`flow/checkout/`, `lifecycle/orders/`). | Created by the project. **One level only** — never nested. |

There is no `concern` axis here, and no component folder — see below.

### The four types

| Type | What it holds |
| --- | --- |
| `flow` | An operation facing an actor. Someone does something and the system responds. |
| `rule` | A cross-cutting rule with no flow of its own. |
| `lifecycle` | An entity's state machine — which states exist and what moves between them. |
| `process` | Reactive or background work, triggered by an event rather than by an actor. |

`flow` is the type that grows: in a mid-sized product it reaches dozens of capabilities, which is why
subgroups exist here at all.

#### Picking between them

The definitions are not enough when the case is ambiguous, and three crossings come up constantly:

- **`flow` vs `process`** — it is not decided by whether it runs in the background. It is decided by
  **what triggers it**: an actor → `flow`; an event → `process`. A signup triggered by a webhook is a
  `process`, even though it does the same thing the manual signup does.
- **`flow` vs `lifecycle`** — the `lifecycle` describes **the machine** (which states exist and what
  moves between them); the flows describe **the operations** that move it. If you find yourself writing
  numbered steps, it is a flow.
- **`rule` vs an edge case** — if it applies to **one** flow, it is a `## Edge cases` entry inside that
  flow. If it applies to several, it is a `rule` of its own.

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

An absent required section is reported as `missing-section`; an absent optional one is not.

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
