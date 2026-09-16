# Integrating openrecord into an AI ecosystem

openrecord calls no model to decide. It holds the records and answers questions about them; every
judgement it depends on is made by whoever is driving — a person, or the agents of an ecosystem built
around one. The one model in the picture is the embedding model behind `search`'s meaning half, which
is the search itself, not a verdict about what governs a piece of work.
This document is the contract between the two: what your ecosystem has to do so that a store of records
actually changes what your agents write, rather than sitting in the repo being technically present.

The distinction matters because the failure is quiet. An ecosystem that reads records and then does
whatever it was going to do produces exactly the same code as one that never read them, and nothing in
either the tool or the store reports the difference.

## The three moments

Integration is not a feature you switch on. It is three moments in the life of a change, and an
ecosystem that covers only some of them gets a store that decays.

| Moment | What happens | The emitted skill |
| --- | --- | --- |
| Before writing code | Find what already governs this work, and obey it | `openrecord-consult` |
| After the work ships | Record what was decided, while the reasoning is still in someone's head | `openrecord-capture` |
| Starting a project | Populate an empty store by asking, not by inferring | `openrecord-bootstrap` |
| Inheriting a codebase | Reconstruct candidate records from what is already built | `openrecord-mine` |
| Once per project | Make the store findable by meaning | `openrecord-setup-search` |

The first two are the loop. The others run once, or rarely.

## Take the skills, do not write your own

```bash
openrecord skills --emit .claude/skills/            # or --with-qmd, if the project has qmd
```

The skills ship with the binary and are emitted into the project. Your ecosystem should hand them to
its agents the way it hands them any other project-local skill, and re-emit when it upgrades the
binary.

Writing your own copies is the tempting move and the wrong one. The skills encode behaviour that the
store's shape depends on — which level accepts which kind of prose, that a subgroup is never nested,
that `openrecord search` reports which of its two passes actually ran. A private copy drifts from the
binary silently, and the first symptom is a store that fails `validate` for reasons nobody can trace.

`--with-qmd` is not decoration: without it, no emitted skill mentions semantic search at all. An agent
must never read about a tool the project does not have, so the flag has to match reality. Installing
qmd later leaves the already-emitted skills describing a smaller tool; `skills --emit` reports that
mismatch, and re-emitting is what reconciles it.

## The contract that makes it effective

Everything above is plumbing. This section is the part that decides whether integrating openrecord
changes anything.

**A record is only worth writing if it can stop something.** A store your agents read and then write
around is documentation, and documentation that nobody enforces is a lie with a timestamp on it. So
the integration contract has one hard requirement, and it is about contradiction.

### When the work contradicts an accepted record

Every skill in your ecosystem that writes code, proposes a design, or records a decision must carry
this. Not a paraphrase of it — the behaviour:

**Stop the line of work that depends on the conflict.** Not everything: the other work continues. What
stops is what cannot proceed without resolving this.

**Surface both sides, with their reasons.** A fixed block, so the shape does not vary with the agent's
mood:

```
STOPPED: adding the limit inside the /users handler

What the record settles:
  decisions/api/security/rate-limits/at-the-gateway.md   [accepted]
  Rate limiting is applied at the gateway, not in each handler.
  Its reason: a per-handler limit cannot see a client's total across endpoints.

What the work requires:
  A different limit for /users than for the rest of the API.

Why both cannot hold:
  The gateway does not distinguish endpoints; doing this there requires that it does.

Ways forward:
  A. Extend the gateway to per-route limits. Touches the gateway; the record still holds.
  B. Make an exception in the handler. Contradicts the record; it would have to be edited.
  C. Leave it global. The /users requirement goes unmet.

Recommendation: A.
```

Arriving with *"this conflicts, what do I do?"* hands the whole problem back to the person. Arriving
with both sides and their reasons lets them decide in one step — which is the difference between
stopping usefully and just stopping.

**Never resolve it alone, and never quietly write a record that supersedes one a person ratified.** A
record was accepted by someone. An agent that overrides it silently makes the store a lie while
everyone downstream keeps trusting it. Resolving it alone and reporting afterwards is not a
consultation; it is a decision that was taken and then announced.

The record turning out to be wrong or stale is a perfectly good outcome. It is just not the agent's to
conclude alone.

### Where this contract lives today

Of the skills openrecord emits, `openrecord-consult` and `openrecord-capture` carry the contradiction
section in full. `openrecord-bootstrap` and `openrecord-mine` do not — mine's nearest equivalent is a
duplicate check, which is a different concern. `openrecord-setup-search` touches no record content and
has no reason to carry it.

State this plainly rather than assume: an integrating ecosystem that relies on the emitted skills alone
inherits the contract unevenly, and the gap is on the two skills that *write* records. If your flow has
its own writing or proposing phases, those phases carry the contract themselves.

### `status: pending` is neither permission nor constraint

A record marked `pending` says someone explicitly left the question open. It constrains nothing — and
it authorises nothing either. An agent that reads `pending` as "so I can choose" has invented a
mandate. If the work depends on that question being settled, that is worth saying out loud, which is
itself a useful thing for the person to hear.

## Finding the record is three moves, not one

The most common integration mistake is wiring up semantic search and calling the retrieval problem
solved. `openrecord-consult` uses three distinct moves, and each one catches what the others miss:

1. **Descend the store's own index.** `openrecord component owners <path>` maps a file to the surface
   that owns it; `openrecord map --for decisions/<component>` lists its concerns *with the description
   of when to descend into each*. This is the only move that enumerates, and the only one that works
   when you do not yet know what you are looking for.
2. **Search.** `openrecord search "<term>" --for decisions/<component>` runs both passes in one call:
   the literal one, exact and blind to synonyms, and — when qmd reports an embedding model — a pass by
   meaning over the same scope. It hands back store paths, never a `qmd://` URL to translate by hand,
   and it says which passes actually ran. Give it `--omit` for the paths the descent already surfaced
   and it leaves them out. The meaning pass is what earns the command its place: a question asked in
   the words of the task rarely matches the words of a record written months earlier by someone else.
3. **Follow the links.** Records cite each other with `[[slug]]`. In practice a large share of the
   relevant records are reached not by any search but by following a link out of the first one found.
   An ecosystem that stops at the first hit systematically misses the records that depend on it.

A worked example from a real store: a question posed as *"what stops two consultations of the same
professional from overlapping"* against a store whose records say *turno* and *médico* — no shared
vocabulary at all. Index descent narrowed it to one concern, the search's literal pass found nothing
there, its meaning pass surfaced the governing decision, and two of the four relevant records arrived
by following links out of it. Any single move on its own would have returned less than the whole answer.

### Report how each record surfaced

When an agent reports what governs a piece of work, each record should carry how it was found:

```
decisions/agenda/domain-logic/no-solapamiento-en-la-base.md   [accepted]
  Surfaced by: index descent (agenda/domain-logic) + search

specs/flow/reserva-de-turno.md                                [accepted]
  Surfaced by: link from the no-overlap decision
```

This is not bookkeeping. A person reading the report can tell the difference between *the store was
searched thoroughly* and *one lucky query hit*, and can see which move would have to improve for the
next question to go better.

The same applies in reverse: a section naming records that were **examined and ruled out** is worth
emitting. Without it, nobody can distinguish *considered and judged irrelevant* from *never looked at*.

## Failure modes worth designing against

These are observed, not hypothetical.

**Confirmed but never written.** An agent reads a record back to the user, the user confirms it, the
agent moves on to the next topic, and the write never happens. Several records can be lost this way in
a single session, and the conversation looks complete throughout — the transcript says they exist.
**Write each record the moment it is confirmed, and when asked to "write everything", verify against
the store rather than against the conversation.** `openrecord map` is the check; the chat history is
not evidence.

**Editing the file instead of the record.** An agent with a file-editing tool will reach for it, because
a record is a `.md` and editing one looks like editing any other file. Every guard lives on the write
path, so an edit that goes around `record write` / `record edit` goes around all of them. This is not
hypothetical: in a session where the skills were loaded and followed, an agent made 21 `record write`
calls, zero `record edit` calls, and one direct file edit — the one change nothing checked.

Since the body hash exists, that edit no longer passes unnoticed: `validate` reports
`body-hash-mismatch` on a record whose body no longer matches the hash the tool stamped. Detection, not
prevention — an ecosystem that wants to stop it before it happens still needs its own guard, because the
binary does not police anyone's editor.

**Inventing the why.** The reasoning is the only part of a record that cannot be reconstructed from the
code. An agent that fills in a plausible-sounding rationale has produced something worse than an empty
store, because it reads exactly like a real record. If the reason is not known, the honest record says
so, or no record is written.

**Creating a store because one is missing.** A project without records is not a broken project. Offer,
explain what it buys, and let the person decide. Never bootstrap a store as a side effect of noticing
its absence.

**Duplicating the store into the ecosystem's own memory.** If your ecosystem has a memory system of its
own, decisions belong in records and the memory holds everything else. Two copies of a decision drift,
and the drift is discovered by an agent acting on the stale one.

**Treating silence as agreement.** Nothing in openrecord ratifies anything. A record reaches
`status: accepted` because a person put it there. An ecosystem that writes `accepted` on an agent's own
conclusion has changed what the status means for every reader after it.

## What openrecord will not do for you

It validates **shape**, never truth. `openrecord validate` confirms a record is well-formed, that its
level exists, that a spec names declared components — it cannot know whether the decision is still the
one the team holds, or whether the prose describes the system as built. That gap is what the consult
and capture moments exist to close, and it closes only as often as your ecosystem runs them.

There is one thing in between, and it is worth knowing exactly how far it reaches. The body hash does
not tell you whether a record is *true*; it tells you whether its body is the one the tool wrote. That
is the only check here that survives without your cooperation — it needs no hook, no harness and no
commit, so it holds for an agent you did not write, a tool you did not configure, and a person editing
by hand.

Everything else never blocks: the guards described here are behaviour your agents carry, not gates the
binary enforces. openrecord will happily hold a store that contradicts the code it lives beside; keeping
the two in step is the integration's job, which is the whole reason this document exists.
