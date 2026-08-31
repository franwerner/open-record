# openrecord

A project's durable records: **what was chosen and why**, and **what the system does**. A directory
layout, two kinds of document, and a CLI that reads and writes them.

openrecord is not an agent framework and it never calls a model. Everything it does is deterministic;
everything that needs judgement is prose a person or an agent writes.

## Two kinds of record

They are orthogonal, and the boundary between them is one question: **would someone who only *uses* the
product notice?**

| | Answers | Lives in |
| --- | --- | --- |
| **[Decision record](decision-record.md)** | What was chosen and why — the trade-off, the rejected alternative. *No*, a user would not notice. | `decisions/<component>/<concern>/` |
| **[Capability spec](capability-spec.md)** | What the system does — the flow, its branches, its edge cases. *Yes*, a user would notice. | `specs/<type>/` |

They coexist over the same topic without overlapping. The spec fixes the limit facing the actor —
*"past N req/min you get a 429"*. The decision fixes the mechanism and why that one — *"rate limiting is
applied at the gateway, not in each handler"*.

## Three layers

openrecord is the bottom two. The third is where opinion about process lives, and it ships separately.

**1 — The format.** The convention: what a record is, how the store is organised, what each frontmatter
carries. Documents, no code. See [decision-record.md](decision-record.md) and
[capability-spec.md](capability-spec.md).

**2 — The CLI.** The mechanical tools for working with the format: navigating, writing, validating,
searching, rendering. Deterministic, with no opinion about *when* you should do any of it. See
[cli.md](cli.md).

**3 — Skills and search.** Guidance on top: how to populate a store, how to reconstruct records from
existing code, when to write one at all — plus semantic search over the records, which is `qmd`, a
separate project in its own repository. It consumes the two layers below; **nothing below depends on
it.**

The skills ride inside the binary and come out with `openrecord skills --emit <dir>`, but they are not
part of the format or the tooling: strip them and layers 1 and 2 still work. openrecord itself takes no
position on who may write a record or when — there is no role vocabulary, and the commands are
mechanical actions a consumer calls.

Four skills, each with its own trigger — the CLI already does the mechanics, so what these carry is
judgement:

| Skill | Fires when |
| --- | --- |
| `openrecord-consult` | Before writing or changing code. Find what governs it first. |
| `openrecord-capture` | Work finished. Decide what the store should say — usually nothing. |
| `openrecord-bootstrap` | An empty store. Fill it by asking the user, never by inferring. |
| `openrecord-mine` | Code but no records. Reconstruct candidates; never write. |
| `setup-record-search` | Once per project, and only with `--with-qmd`. Registers the stores so records can be found by meaning. |

## The store

```
.openrecord/
├── decisions/
│   └── <component>/            api, ui, cli, root — declared in components.json
│       ├── INDEX.md
│       └── <concern>/          structure, runtime, data, security…
│           ├── INDEX.md
│           ├── <record>.md
│           └── [<subgroup>/]   one level, never nested
├── specs/
│   └── <type>/                 flow | rule | lifecycle | process
│       ├── INDEX.md
│       ├── <capability>.md
│       └── [<subgroup>/]
└── components.json             the declared surfaces, and the format version
```

Two rules generate most of this layout:

**A single-valued axis becomes a folder; a multi-valued axis becomes a property.** A decision governs
one surface, so the component is its folder. A capability crosses surfaces, so its components are a
frontmatter list — a signup that starts in the UI, passes through the API and can also be triggered
from the CLI is *one* capability, not three.

**Every level declares when to descend into it.** Each folder carries an `INDEX.md` holding only
`title` and `description`. It never lists what is inside — enumeration is generated from frontmatter,
the way a host builds a list of available skills, so there is nothing to fall out of sync.

## Documents

| | |
| --- | --- |
| [decision-record.md](decision-record.md) | What a decision record is and is not, where it lives, its frontmatter and body. |
| [capability-spec.md](capability-spec.md) | The same for capability specs, plus the four types and Mermaid rendering. |
| [cli.md](cli.md) | The seven commands. |
| [concerns.md](concerns.md) | The catalogue: eleven concerns and the places projects usually have a decision worth recording. |
