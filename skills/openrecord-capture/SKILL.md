---
name: openrecord-capture
description: Decide what a project's durable records should say after a piece of work, and write it. USE THIS SKILL when work is finished — a feature, a fix, a refactor, a config change — in a project that has an `.openrecord/` store. Most work settles nothing and writes nothing; this skill is as much about not writing as about writing.
---

# Capture what the work settled

Work just finished. The question is what, if anything, the store should say now.

**The answer is usually nothing.** Read that again before continuing, because your default is to
produce something, and a store with a record per change is a store nobody reads. The value of these
records comes from their scarcity: every one of them is something a reader has to hold in their head.

## The presence gate

No `.openrecord/` directory, or an empty one → this skill does not apply. Say nothing about it. Never
create a store because you noticed one is missing; offer, do not impose, and only if the user is
heading somewhere it would help.

## Nobody approves a record

The store maintains itself. You do not present a list and wait for someone to approve it — you resolve
what should be written and you write it.

**The one thing that stops is a contradiction with an accepted record.** That is the only gate, and it
exists because a record was ratified by a person and overriding it is not yours to do.

This is a deliberate trade. Asking for approval on every task would make the store expensive enough
that it stops being kept, and a store nobody updates is worse than one with an imperfect record in it —
a wrong record gets corrected the next time someone works in that area, but a missing one is invisible
forever.

What keeps this honest is not approval. It is the restraint below: writing only what actually settled
something, and never inventing a why.

## Write last, never during

Write when the work is **done**, not when you decide something mid-way.

Halfway through, what you would write is what you *expected* to hold. At the end, it is what actually
held — and those differ more often than they feel like they will. A record written early records a
plan; a record written late records a fact.

## Two independent questions

The common mistake is treating this as one classification with one answer. It is two questions, and
both can be yes about the same change.

**1. Did this change what someone using the product observes?** → there may be a **capability spec**.

**2. Did this settle a choice between alternatives about how it is built?** → there may be a **decision
record**.

Rate limiting answers yes to both. The spec fixes what the actor meets — *"past 60 req/min you get a
429"*. The decision fixes the mechanism and why that one — *"the limit is applied at the gateway, not
in each handler"*. Pick only one and you lose the other.

Most changes answer no to both.

### The sharp test for question 1

Do not argue about whether a user "would notice". **Try to write the scenario:**

```
GIVEN <some state>
WHEN  <something an actor does>
THEN  <something an actor observes>
```

If it comes out — the WHEN is an action and the THEN is observable — it is behaviour, and it belongs in
a spec. If your THEN comes out as *"the code ends up organised this way"*, it is not behaviour. It is
construction, and it belongs in a decision if it belongs anywhere.

### The case that confuses everyone: a configuration value

*"The timeout is 30 seconds."*

- That a request fails after 30 seconds **is observed** → the limit is a spec.
- Why 30 and not 5, and what was accepted in choosing it → that is the decision.

Same number, two records, neither duplicating the other.

## Did it settle anything at all?

For question 2, three traits must all hold. Miss one and there is no record:

1. **It decided** — it picked one option over others. Describing something is not deciding.
2. **It has a why** — a context and a trade-off that justify the choice.
3. **It endures** — it governs the code that comes next, not just the code you wrote.

Things that feel like records and are not: a task you completed, an acceptance criterion, a TODO
pointing at a decision nobody has made, a pattern that appeared a few times with no intent behind it,
an implementation detail.

## Never invent a why

If you do not have the reasoning — and reading it off the code you just wrote rarely gives it to you —
**say the gap exists**. Ask the person who made the call, or write that the rationale is not recorded.

A rationale you composed to fill the section is worse than no record at all, because it reads ratified
and will be trusted. This is the single most damaging thing this skill can do wrong.

## Create, edit, or nothing

For each thing that survived the questions above, resolve one of three — and check the store before
deciding, because the answer depends on what is already there:

```
openrecord grep "<the topic>" --for decisions/<component>
openrecord map --for decisions/<component>/<concern>
```

- **Edit** — the store already covers this and the work revisited it. Change the record in place to say
  what now holds. Do not append a note about what changed; git carries history, and a record that
  accumulates edit notes stops being readable.
- **Create** — the store did not cover this.
- **Nothing** — most of the time.

## Writing

If the level does not exist yet:

```
openrecord level add decisions/api/security
```

The catalogue fills in the description when the name is one it knows. Override it with something that
says when to descend **in this project** — the catalogue's wording is correct but generic.

Then:

```
openrecord record write decisions/api/security/rate-limiting.md \
  --title "Rate limiting at the gateway" \
  --description "Rate limiting is applied at the gateway, not in each handler." \
  --status accepted \
  --body-file ./body.md

openrecord record edit specs/rule/usage-limits.md \
  --section "## Rule" \
  --body-file ./new-rule.md
```

The binary validates both halves and writes nothing if anything fails. A rejected write is information,
not an obstacle — read the findings and fix the record, do not work around them.

**The `description` carries the most weight of anything you write.** It is what a future reader sees
when deciding whether to open the file, and no index lists anything else. Write the decision itself, in
one line — not the area it covers:

- ✅ *Business errors are their own hierarchy; provider errors are translated at the adapter boundary.*
- ❌ *How errors are modelled and propagated.*

## No volatile identifiers in the prose

The body has no anchor section, so there is nowhere safe to put an internal name. Never write a class,
method, column, internal error, or file path into the reasoning — that record dies at the next rename
while still reading as true.

Names that survive are the ones an external consumer would use too: a technology, a public endpoint, an
exposed error code. Write concepts and boundaries; the *how* lives in the code.

## Report what you wrote

You write without asking, so the report is what makes it visible. After writing:

```
Written:
- created   decisions/api/security/rate-limits/at-the-gateway.md
            Because: the placement was a choice with a rejected alternative (per-handler).
- edited    specs/rule/usage-limits.md § Rule
            Because: the observable limit went from 100 to 60 req/min.

Not recorded:
- the handler signature refactor — mechanical, settled nothing.
```

**"Not recorded" is the section that earns its place.** It shows a judgement was made rather than
something being quietly skipped. Without it nobody can tell *decided it did not belong* from *never
looked at it*, and the restraint this whole skill is built on becomes invisible.

## When the work contradicts an accepted record

Stop that line of work — only what depends on the conflict — and surface it with both sides in view:

```
STOPPED: recording that limits are enforced per handler

What the record settles:
  decisions/api/security/rate-limits/at-the-gateway.md   [accepted]
  Rate limiting is applied at the gateway, not in each handler.
  Its reason: a per-handler limit cannot see a client's total across endpoints.

What the work requires:
  The work just shipped enforces the limit inside the /users handler.

Why both cannot hold:
  The record forbids exactly what was built.

Ways forward:
  A. The record still holds and the code is wrong — revert the placement.
  B. The record is out of date — edit it, with the reason the placement changed.
  C. Both are right for different cases — the record needs to say which applies when.

Recommendation: B, if the placement was deliberate; otherwise A.
```

**Never resolve it alone, and never quietly write a record that supersedes one a person ratified.**
Arriving with *"this conflicts, what do I do?"* hands the whole problem back. Arriving with both sides
and their reasons lets them decide in one step.
