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

**Who confirms depends on where the reasoning came from.** One rule, three skills:

| The *why* came from | Who confirms |
| --- | --- |
| The existing code — **this skill** | They do, over candidates. Nothing is materialised unconfirmed. |
| The user, answering questions — `openrecord-bootstrap` | They do, record by record. |
| Work you just did — `openrecord-capture` | Nobody. It resolves and writes, then reports. |

The line is authorship, not caution. Capture has the reasoning first-hand. Here the code cannot tell you
why, so anything you write down unratified puts words in somebody's mouth that will later read as
settled.

Present them ordered by how strong the evidence is, each one anchored to what it was mined from: *this
candidate, from these files, because of this*. A candidate a reader cannot trace back is one they have
to take on faith, and this is exactly the material that should not be taken on faith.

### The anchor is for the gate, never for the record

That file and line belong to the **presentation** — they let somebody confirming a candidate go and
check it. They must not end up in the record you write once they confirm it.

A record never names a class, a method, a column, an internal error or a **file path** — not in its
body, and **least of all in its `description`**. There is no anchor section to put one in, and a record
that names one dies at the next rename, silently, while still reading as true. This bites hardest here
of anywhere, because mining means reading files all day and the path is right there in your hand.

The description is the worst place for one: it is what a generated listing shows, so it is what a
reader sees *before* deciding whether to open the file. A path that has rotted there misleads people
who never open the record at all.

| In the candidate you show ✅ | In the record you write ✅ |
| --- | --- |
| *from `src/store/postgres.go:1-7`* | *persistence is hand-written SQL owned by one layer* |
| *the package comment on `bank.go` says…* | *provider errors are translated at the adapter boundary* |

Names that survive are the ones an external consumer would use too: a technology, a public endpoint, a
contract header, an exposed error code. Quoting a comment is still quoting — attribute it as *the
package comment that owns this* rather than by its path.

Nothing checks this. `validate` reads structure, not prose, so a record full of paths passes every
check and rots on the first rename.

### A section with nothing in it is deleted, not filled

Thin evidence is the normal case when mining, and it produces a specific mistake: writing *"None
recorded."* or *"Not applicable"* under a heading rather than removing the heading.

An empty section reads as **nobody wrote this yet**. An absent one reads as **this does not apply
here**. Those are different facts, and when the whole point of a mined record is to be honest about
what is and is not known, saying the wrong one undoes the honesty everywhere else in the file.

So: a spec type's sections are optional. If a flow has no branches, delete `## Branches` — do not
leave it holding a placeholder. `validate` will not catch this, because the section is *allowed* for
that type; it simply has nothing in it.

The exception is a gap worth naming. *"What happens when a key already seen arrives again is not
recorded anywhere"* is content — it says something true about the system and about the evidence, and
it belongs under `## Edge cases` as prose. The test is whether the sentence carries a fact: **"we do
not know what happens here" is a finding; "there is nothing to say" is an empty section.**

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

### A why that is written down is evidence, not invention

Sometimes the reasoning **is** in the repository: a package comment explaining why a dependency was
avoided, a line above a pinned version saying what broke without the pin, a commit message that argues
rather than describes. That is a person stating their reason, in a place the store does not index.

Quoting it is neither recovering the why from behaviour nor composing one. So:

- **Quote it, marked as quoted**, and say where it came from — the reader can go and check.
- **Do not paraphrase it into the record's voice.** A paraphrase reads as ratified prose and loses the
  fact that it came from a comment somebody may since have outgrown.
- **Do not stretch it.** A comment explaining why *one* package is hand-rolled is not a statement about
  the project's stance on dependencies, however tempting the generalisation.

Where there is no such comment, the rule above is unchanged: say the reasoning is not recorded.

## Do not mine what is already there

Check the store before proposing anything:

```
openrecord map --for decisions/<component>
openrecord grep "<topic>" --for decisions
```

A candidate duplicating an existing record is noise, and worse, it invites a second record saying
almost the same thing in different words. If the store covers it but the code has drifted, that is a
different and more interesting finding — say that instead.

**`grep` is literal, and this is the check where that hurts most.** A record covering your candidate in
other words passes the check and the duplicate gets written — which is precisely the failure the check
exists to prevent. So read the level's listing with `map` as well: the descriptions are one line each
and they state the decision, so a duplicate is visible there even when its wording differs.
<!-- qmd:start -->
Better, ask by meaning rather than by wording:

```
qmd query "how request limits are applied" -c <project>-decisions-api
```

This is the one step where semantic search is doing work nothing else can: it is looking for a record
whose words you do not know, which is the definition of a duplicate you are about to create.
<!-- qmd:end -->

## Do not try to mine everything

A repository of any size contains more implicit decisions than anyone wants to read. Aim for the ones
that would change how someone writes code tomorrow, and say plainly what you left out and why.

Silent truncation is the failure mode here: a list that stops at the twenty strongest candidates, with
no mention that it stopped, reads as *"this is everything"*.
