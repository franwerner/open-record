---
name: openrecord-bootstrap
description: Populate an empty openrecord store by interviewing the user. USE THIS SKILL when a project has no `.openrecord/` store or an empty one and the user wants to define its components, its engineering decisions, or its behaviour from scratch — a project starting out, or a codebase whose decisions live only in people's heads. Also when the user says "set up openrecord", "define our decisions", or "let's write down what we decided".
---

# Populate a store from scratch

An empty store gets filled by asking, not by inferring. Everything here comes out of the user's head —
the decisions were made by people, and the reasoning is the part that only they have.

**Never create a store because you noticed one is missing.** Offer, and only if the user is heading
somewhere it would help. A project without records is not a broken project.

## Components come first, and the user names them

Nothing works before components are declared: they are the folder vocabulary for every decision, and
the axis every spec references.

**Do not propose names.** Detecting which directories hold source is mechanical, but *naming a surface*
is not — and those names become the vocabulary every scope is expressed in from then on. A name you
invented is one the user has to live with without ever having agreed to it.

Ask what the surfaces of this project are. A surface is a part someone would talk about as a thing:
"the API", "the web app", "the CLI". If they hesitate, ask what parts of the codebase change for
different reasons, or what a new contributor would need explained separately.

Then, one per surface:

```
openrecord component add api \
  --path src/api \
  --title "API" \
  --description "The HTTP surface. Descend here if you touch endpoints or request/response contracts."
```

The description says **when a reader should descend here**, in this project's terms. It is not a
definition of the component; it is a routing instruction for someone who does not yet know where their
work belongs.

Include a component for the repository itself — tooling, CI, configuration. Without it, work on those
files sits outside every declared surface forever.

## Then walk the catalogue and ask

```
openrecord concerns
```

The catalogue ships inside the binary. It lists eleven concerns and, under each, the places projects
usually have a decision worth recording. It exists so you know **what to ask**, not what to write.

Go concern by concern, for each component, and ask what was decided. Real questions, not a form:

> *For the API — how do errors get modelled? Is there a hierarchy, or does each handler shape its own?
> And was that a choice, or just how it ended up?*

That last part is the one that matters. **"Just how it ended up" is not a decision.** A pattern with no
intent behind it is incidental code, and recording it as a decision manufactures a constraint nobody
agreed to.

Ask for the trade-off too. A choice with no rejected alternative is a convention someone wrote down; a
choice with one is a decision, and the alternative is often the most useful thing in the record two
years later.

## Behaviour is a different interview

Capability specs are not decisions and the questions are not the same. For behaviour, ask what the
system *does*: what an actor triggers, what happens on the happy path, what happens when it goes
differently, and what the limits are.

The test for whether something belongs in a spec: try writing it as **GIVEN / WHEN / THEN** with an
observable outcome. If the THEN is something a user sees, it is a spec. If it is something about how
the code is arranged, it is a decision.

Pick the type from what the thing is:

- an operation an actor triggers → `flow`
- a rule that holds across several flows → `rule`
- an entity's states and what moves between them → `lifecycle`
- work triggered by an event rather than an actor → `process`

## Writing what they tell you

A record lives in a level, and a level is created before the record — never implicitly, because
creating one means writing the prose that says when to descend into it.

```
openrecord level add decisions/api/security
openrecord level add specs/flow
```

The concerns come from the catalogue and the four spec types come from the binary, so both fill in their
own title and description. A **subgroup** has neither, and needs `--title` and `--description`: it is
named for whatever its records share, and that is a judgement about those records.

Then the record itself. The body is a file, so the prose can be written before it is filed:

```
openrecord record write decisions/api/security/rate-limiting.md \
  --title "Rate limiting at the gateway" \
  --description "Rate limiting is applied at the gateway, not in each handler." \
  --status accepted \
  --body-file ./body.md
```

A decision's body is four sections and nothing else: `## Context`, `## Decision`, `## Alternatives`,
`## Consequences`.

A spec is the same command with **`--components`**, which is mandatory and validated — a capability that
declares no surface crosses with no path and drops out of every scope check silently:

```
openrecord record write specs/flow/sign-up.md \
  --title "Sign up" \
  --description "A visitor registers with email and password; the account stays pending until they verify." \
  --status accepted \
  --components api --components web \
  --body-file ./body.md
```

Its body is `## Purpose` and `## Scenarios`, plus whatever its type asks for — a `flow` adds
`## Main flow`, `## Branches`, `## Edge cases` and `## Errors facing the actor`. A section that does not
apply is deleted entirely, never left empty.

**Nothing lands unless it is well-formed**, so a rejected write is information rather than an obstacle:
read the findings and fix the record. The reply carries the findings in the same shape `validate` uses.

**And a clean write is not a clean store.** A successful write reports only what is wrong with *that
record*. Findings about the store as a whole — a level grown flat, a surface pointing at a directory
that no longer exists — come from `openrecord validate`, so run it once when the interview ends.

## Nothing gets written without confirmation

**Who confirms depends on where the reasoning came from.** One rule, three skills:

| The *why* came from | Who confirms |
| --- | --- |
| The user, answering you now — **this skill** | They do, record by record, before it is written. |
| Work you just did — `openrecord-capture` | Nobody. You resolve it and write it, then report. |
| The existing code — `openrecord-mine` | They do, over candidates. Nothing is materialised unconfirmed. |

The line is not about caution, it is about authorship: capture writes reasoning it *has*, because it
just did the work. Here and in mining the reasoning belongs to somebody else, and writing it down
without them ratifying it puts words in their mouth that will later read as settled.

So: write records as you go, but **each one is confirmed before it is written** — not in a batch at the
end, and not silently because they answered the question.

Read back what you understood, in the record's own words, and let them correct it. What you write is
what they settle, not your reading of what they said.

## An honest gap beats a plausible story

The user will not remember why some things are the way they are. **Write that.** *"The reasoning for
this is not recorded"* is a true and useful sentence; a reconstructed rationale is neither, and it will
be trusted precisely because it reads confident.

Do not fill a section to make the record look complete. A short honest record is worth more than a
whole one that is partly invented.

## Do not try to finish

A store does not need to be complete to be useful — it needs to be true. Cover what the user actually
knows and stop; the rest gets written when it gets decided.

An interview that runs until the user is exhausted produces records answered to make the questions end,
and those are exactly the ones that are wrong.
