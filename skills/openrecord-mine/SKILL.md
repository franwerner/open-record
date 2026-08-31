---
name: openrecord-mine
description: Reconstruct openrecord records from an existing codebase, as candidates a person confirms. USE THIS SKILL on a repository that has code but few or no records, when the user asks what decisions or behaviour are implicit in the code, wants to bootstrap a store from what is already built, or says "mine the decisions", "what did we decide here", "find the specs in this code".
---

# Reconstruct records from the code

An existing codebase has decisions in it — someone chose the layering, the error model, where
validation happens. None of it is written down, and everyone who knew why is busy or gone.

You can recover **what** was chosen by reading the code. You usually cannot recover **why**, and that
limit governs everything below.

## It returns candidates. It never writes.

Every output of this skill is a candidate for a person to confirm, reject or correct. Nothing is
materialised without that confirmation — not in bulk, not because the evidence looks strong, not
because the user said "go ahead" about a different candidate.

Present them ordered by how strong the evidence is, each one anchored to what it was mined from: *this
candidate, from these files, because of this*. A candidate a reader cannot trace back is one they have
to take on faith, and this is exactly the material that should not be taken on faith.

## What counts as evidence of a decision

Not every pattern is a decision. Look for **deliberateness**:

- **Structural** — a layering that holds across the whole codebase, a dependency direction that is
  never violated, a folder shape applied consistently. Consistency under pressure is intent.
- **Configuration** — a lint rule, an import boundary, an architecture test. Someone spent effort
  making a violation fail; that is a decision with its enforcement attached.
- **Repetition with intent** — the same approach in twenty places where an easier one was available.
- **Absence** — no ORM anywhere in a codebase that touches a database. A consistent absence is often a
  louder decision than a presence.

**What is not evidence:** a pattern in three files and not in the other forty; something a framework's
default produced; anything you would have to assume intent for. Without deliberateness there is no
decision, and a record manufactured from incidental code creates a constraint nobody agreed to.

## What counts as evidence of behaviour

For capability specs, read what the system does at its edges:

- **Route and command handlers** — each is usually one `flow`.
- **Validation and business rules** — the branches and edge cases of a flow, or a `rule` of its own if
  it holds across several.
- **State machines** — an entity's states and transitions, a `lifecycle`.
- **Event handlers and scheduled work** — a `process`; what triggers it is the fact that defines it.

**Tests are the strongest oracle available.** A test asserts an observable outcome someone cared about,
which is exactly what a scenario is. Where a test exists, the behaviour is corroborated; where the
behaviour has no test, say so — an uncorroborated candidate is weaker and the reader should know which
is which.

## The hard limit: never invent a why

This is where mining goes wrong, and it goes wrong in a way nobody catches later.

You can read *what* the code does. The **context** and the **trade-off** are not in it — the alternative
that was considered and rejected left no trace, and neither did the pressure that forced the choice.

So for a decision candidate:

- State the **what** from the evidence.
- State that the **why is not recorded**, and leave it for the person to fill in.
- **Do not compose a plausible rationale.** You will be good at it, and that is the problem: a
  reconstructed why reads exactly like a real one, and from then on it is trusted as if a person had
  said it.

A candidate that says *"this is what the code does; the reasoning is not recorded"* is honest and
useful. One that says *"this was chosen for testability"* — when no one said that — is a fabrication
wearing the store's authority.

## Do not mine what is already there

Check the store before proposing anything:

```
openrecord map --for decisions/<component>
openrecord grep "<topic>" --for decisions
```

A candidate duplicating an existing record is noise, and worse, it invites a second record saying
almost the same thing in different words. If the store covers it but the code has drifted, that is a
different and more interesting finding — say that instead.

## Do not try to mine everything

A repository of any size contains more implicit decisions than anyone wants to read. Aim for the ones
that would change how someone writes code tomorrow, and say plainly what you left out and why.

Silent truncation is the failure mode here: a list that stops at the twenty strongest candidates, with
no mention that it stopped, reads as *"this is everything"*.
