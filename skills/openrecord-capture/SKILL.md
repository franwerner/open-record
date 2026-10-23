---
name: openrecord-capture
description: Decide what a project's durable records should say after a piece of work, and write it — or, when the user wants to define or record a specific decision, spec, actor, component, verification, or resolve an open gap, interview them for it first. USE THIS SKILL when work is finished — a feature, a fix, a refactor, a config change — in a project that has an `.openrecord/` store. Also USE THIS SKILL when the user wants to define, record, or write down a specific decision, spec, actor, component, or how something is verified, or wants to resolve a gap an `openrecord run` left open. Most work settles nothing and writes nothing; this skill is as much about not writing as about writing.
---

# Capture what the work settled

This skill covers two separate moments, and they are told apart by one question: **does the *why*
already exist, or does it need to be asked for?**

1. **Work just finished.** A feature, a fix, a refactor, a config change. You did the work, so you
   already have the reasoning — nobody is asked, nobody confirms, you resolve what the store should say
   and write it. This is everything below through "When the work contradicts an accepted record."
2. **Someone wants to define something specific** — an actor, a single decision, a new spec, a new
   component, how something existing is verified, or a gap an `openrecord run` left open — before any
   of it is settled by code at all. The *why* here is never yours to invent; it comes from the person,
   or from whatever raised the gap. See "## Defining something specific" near the end.

**In situation 1, the answer is usually nothing.** Read that again before continuing, because your
default is to produce something, and a store with a record per change is a store nobody reads. The
value of these records comes from their scarcity: every one of them is something a reader has to hold
in their head. Situation 2 does not share this restraint the same way — there, someone explicitly asked
for a specific record, so the question is not *whether* to write but *what*, confirmed with them before
it happens.

Everything else about this skill — the presence gate, never inventing a why, writing so a check could
fail, the store's own vocabulary — holds in both. Only *who confirms* and *when something is written*
differ, and the table in the next section says exactly how.

## The presence gate

No `.openrecord/` directory, or an empty one → this skill does not apply. Say nothing about it. Never
create a store because you noticed one is missing; offer, do not impose, and only if the user is
heading somewhere it would help.

## Nobody approves a record — here, when the why is yours

The store maintains itself. You do not present a list and wait for someone to approve it — you resolve
what should be written and you write it. **This holds for situation 1, finishing work, below.** It does
not hold for situation 2 — defining something specific, also this skill — where the person confirms
every record before it is written; see "## Defining something specific" for how.

**Who confirms depends on where the reasoning came from.** One rule, four rows — two of them this
skill:

| The *why* came from | Who confirms |
| --- | --- |
| Work you just did — **this skill**, finishing work | Nobody. You resolve it and write it, then report. |
| The person, or a gap an `openrecord run` left open — **this skill**, defining something specific | They do, record by record: title and a one-line description, then their yes writes it straight to `accepted`. See "## Defining something specific". |
| The user, answering questions — `openrecord-onboard` | They do, record by record. |
| The existing code — `openrecord-mine` | They do, over candidates. Nothing is materialised unconfirmed. |

The line is authorship, not caution. In the first row you have the reasoning first-hand, because you
did the work. In every other row it belongs to somebody else — the person, or whoever wrote the pending
record a gap points at — and writing it down unratified puts words in their mouth that will later read
as settled.

**In situation 1, the one thing that stops is a contradiction with an accepted record.** That is the
only gate, and it exists because a record was ratified by a person and overriding it is not yours to
do. In situation 2 the person is already in the loop for every record, so a contradiction is surfaced to
them before anything is even proposed — see "## Defining something specific".

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

### When the THEN needs a percentile, not a yes or no

*"Uploading a 50 MB file takes under 10 seconds."* Still question 1, but the GIVEN/WHEN/THEN test above
comes out "it depends" instead of a clean yes: whether it holds needs a percentile, a load or a
measurement window, not one run. That is a `quality` spec, not a `flow`/`rule`/`lifecycle`/`process`
one and not a decision either — write it with `qualifies: [specs/flow/upload-file.md#successful-upload]`
linking it to the functional spec it characterizes, a `MEASURE` line stating the target (`p95 <
10s`), and — when the person said how it is measured — a `VERIFIED-IN` line saying so
(`VERIFIED-IN pre-release — nightly load test in CI`). It declares the intended verification, so write
it even if that test does not exist yet; leaving it out is what raises `quality-unverified`.

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

## What spec this decision serves

Before writing a decision, resolve this yourself — nobody is there to ask, and you just did the work,
so the answer is yours to give:

```
openrecord impact --files <the files this work touched>
```

Under `specs`, this returns every spec that governs the component(s) those files belong to. Three
outcomes, and only one of them writes `--serves`:

- **The work was done to fulfil one of them** — the decision exists because that spec had to hold.
  Write it with `--serves <that spec>`.
- **A spec governs the files, but this decision is not because of it** — it is a separate choice made
  along the way, not the reason the spec holds. Do not link it. Sharing a file with a spec is not a
  reason.
- **None of them motivated this decision** — say so in `## Context`: that no spec motivates it, that
  it is an engineering choice, and why.

If the same capture writes both a new spec and a decision that serves it, write the spec first — a
decision cannot name a spec that is not yet on the books.

This is the same discipline as above, aimed at one flag: **a false `--serves` is worse than no
`--serves` at all.** It tells a future reader "this decision exists because the product has to behave
this way," and whoever later changes that spec will think this decision needs re-evaluating when it
never actually depended on it.

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

## Write it so a check could fail

Before writing, look up the claim you are about to make in the verification catalogue:

```
openrecord verification <key>
```

A functional spec's key is its type and aspect (`flow/main-flow`, `lifecycle/transitions`,
`rule`...); a `quality` spec's key is its attribute (`latency`, `accessibility`...); a decision's key is
its concern and topic (`security/rate-limiting`...). No key in hand yet? `openrecord verification` with
none prints the whole catalogue. Each entry says what the record needs to contain to be verifiable at
all, and where that kind of claim typically lands (`on-change`, `pre-merge`, `pre-release`,
`production`, `manual`).

Write the record so a check could fail against it, and if the person has not said how it would be
verified, suggest one — as a suggestion, never a tool picked on their behalf.

**If the verification clearly lands `on-change` or `pre-merge`, and the person has already named the
concrete tool** (not you suggesting one — *them* saying "we run `go-arch-lint check`" or similar),
register it: `openrecord check add <id> --command "<command>" --justified-by <decision> [--enforces
<record-or-coordinate>]`. Still never pick the tool yourself, and never register anything manual or
statistical in `checks.json` — a `pre-release`, `production` or `manual` verification, or anything a
person has to look at, stays out of the registry and is said in the record's own prose or `VERIFIED-IN`
line instead.

**If the person describes an environment to bring up and observe** — a check that needs the system
running, a local load test, a way to review a decision against real output — register it with
`openrecord sandbox add <id> --description "<what it's for>" ...` (`openrecord sandbox edit`/`sandbox
remove` after). A local load test **approximates** a `quality` scenario's threshold (`--approximates
specs/quality/<name>.md#<anchor>`): it detects a regression against the previous run, never certifies
the number, and it never goes in `checks.json` either way. Same rule as above: register what the person
describes, never pick the tool (Docker Compose, a bare process, whatever they already run) yourself.

**A test, rule, config or script that verifies a record names it in a comment:**
`@record:<path>#<anchor>` for one scenario or transition, `@record:<path>` for a whole rule spec or
decision. Take the anchor from openrecord — the `anchors` a write or edit reports, or the spec itself —
never from a guess at the slug; `openrecord validate` reports one that points nowhere. Annotate only the
piece that verifies; production code never carries one.

## Reconcile the draft before writing

By this point you have a title, a description, and a body — nothing of it is on disk yet. Before the
`record write` call itself, run `openrecord-reconcile` on the draft.

Reconcile does not need a written record to work from: `map`, `component owners`, and `search --context
<the draft's title, description and body, pasted in whole>` all score a draft exactly the way they
score a written one. The one step that does need something on disk is `impact <anchor>` — a draft has
no anchor yet, so run `impact` instead on the records it will declare in `--serves` or `--depends-on`
(the spec you resolved above, the decision this one depends on), which already exist.

A conflict reconcile finds here is the same thing "When the work contradicts an accepted record" below
describes — stop, surface both sides, let the person decide. Nothing with a live conflict is written
first and reconciled after.

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

A decision can also carry **`--serves`** — the spec you resolved above, if any, named by its whole
path — and **`--depends-on`** (another decision that has to stay `accepted` for this one to still make
sense — the test: if that other decision changed, would this one need re-evaluating?). Both are
optional, both take a whole record path, repeatable, and both are refused on a spec:

```
openrecord record write decisions/api/data/pgx-driver.md \
  --title "pgx as the database driver" \
  --description "Database access uses pgx directly, without an ORM." \
  --status accepted \
  --depends-on decisions/api/data/postgres.md \
  --body-file ./body.md
```

**After writing or editing a decision, read `dependents` in the reply** — every decision that depends
on it, directly or transitively, computed by the binary. After writing or editing a spec, read
`served_by` the same way. Neither blocks anything and neither is yours to act on: tell the user what
showed up and why it matters (*"pgx-driver depends on this; it's a Postgres driver, so switching
databases probably affects it"*), and let them decide whether and how to follow up. Changing the
dependent record yourself, even to a wording tweak, is not this skill's call to make.

A spec also needs **`--components`**, which is mandatory and validated — one flag per surface,
repeatable. A capability that declares none crosses with no path and drops out of every scope check
silently, which is why it is refused rather than defaulted:

```
openrecord record write specs/flow/sign-up.md \
  --title "Sign up" \
  --description "A visitor registers with email and password; the account stays pending until they verify." \
  --status accepted \
  --components api --components web \
  --initiated-by visitor \
  --body-file ./body.md
```

A `flow` or a `process` also needs **`--initiated-by`** — who starts it: a `flow` only a `human` actor,
a `process` only an `external-system` or `time` one, declared in `actors.json` first (`openrecord actor
add <id> --kind ... --description ...` if it is not there yet). Mark the same actor at least once in
the body as `@actor:<id>`, and mark any other actor the prose names in a spec or a decision the same
way. If the prose uses a word that is not a registered actor or alias, use the actor that fits rather
than inventing a new one, and propose `openrecord actor edit <id> --alias <word>` to the person if the
wording is worth keeping.

The binary validates both halves and writes nothing if anything fails. A rejected write is information,
not an obstacle — read the findings and fix the record, do not work around them.

**A clean write is not a clean store.** A successful write reports only what is wrong with *that
record*; findings about the store as a whole — a level grown flat, a surface pointing at a directory
that no longer exists, a folder no component claims — are produced by `openrecord validate` and by
nothing else. `"warnings": []` on a write says nothing about them. Run `validate` once when you are
done writing.

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
            Serves: specs/flow/sign-up.md
- edited    specs/rule/usage-limits.md § Rule
            Because: the observable limit went from 100 to 60 req/min.

Not recorded:
- the handler signature refactor — mechanical, settled nothing.
```

**Every decision also reports which spec it serves** — or, when none motivated it, that it is an
engineering choice with no spec behind it. That line is what lets a reader tell the two apart without
opening the record.

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

## Defining something specific

Everything above is about work that already happened. This section is the other situation: someone
asks you to **define** or **record** something specific — before any code settles it — or asks you to
resolve a gap an `openrecord run` left open. The *why* is not yours here. It belongs to the person, or
to whatever raised the gap, and the record it produces is only ever written after they say yes.

### The why is never yours to invent

If the person names *what* they want recorded but not *why*, ask. Never read a reason off the code, off
a convention, off what seems likely — that is exactly the invented-why "Never invent a why" above warns
against, and it is just as damaging here. A gap already carries its own evidence (see "Resolving a gap,
by kind" below), which is often enough to ask a sharp question instead of a blank "why?".

### No specific thing named → list open gaps

If the person says only "let's define what's missing" or "let's resolve what's pending" and names
nothing, do not guess. List what is actually open:

```
openrecord run gaps
```

This is the one command for this — it reports every open gap across every run, each carrying which run
it belongs to, so you never have to know `.openrecord/runs/` exists at all. **Never read, list, or
`cat` anything under `.openrecord/runs/` directly** — every look at a run's state goes through the
binary (`run gaps`, `run status`, `run list`), the same way every write to it does.

Let the person pick which gap to take on, unless there is exactly one.

### Resolving a gap, by kind

A gap's `--kind` says what is missing, and that is what decides how it gets resolved. In every case,
resolving it is this last step:

```
openrecord run gap resolve <run> <gap> --record <path>
```

which requires `<path>` to already be an **existing, `accepted`** record — resolving is never the act
that creates or accepts one, only the act that points the gap at one that already is.

- **`missing`** — nobody has written this decision (or spec) down at all. Define a new record with the
  person exactly the way the rest of this section describes (ask what's missing per the table below,
  propose title and one line, write on their yes), then resolve the gap with the record you just wrote.
- **`pending`** — the record already exists, but it is `pending`: someone else wrote it — `openrecord-
  mine`, most often — and the person has never actually read it. This is the one place in this skill
  where you **show the full body** before asking anything: open the record and show its `## Context`,
  decision, and alternatives in full, then ask whether to accept it, change it, or discard it.
  - **Accept**: `openrecord record edit <path> --status accepted`, then resolve the gap with that path.
  - **Change**: edit the section that needs to change with the person first (`record edit --section ...
    --body-file ...`), then accept and resolve the same way.
  - **Discard**: the record stays as it is — do not accept it — and the gap is not resolved with it;
    treat the question as if it were `missing` instead, and define a new record with the person.
- **`contradicts`** — an existing record says something incompatible with what the gap's evidence
  found. Show both sides, the same pairing `openrecord-reconcile` uses and the STOPPED block above
  uses: what the existing record settles, what the new evidence requires, and why both cannot hold. The
  person decides which one is right. Edit whichever record should change (the surviving one's `##
  Alternatives` is where the losing reasoning goes, per `openrecord-reconcile`'s own resolution rule),
  then resolve the gap with the record that holds.

**What resolving actually does, so you report it accurately**: it moves the gap to `resolved`, records
which accepted record resolved it, and moves every task it blocked back to `pending` **in the same
run** — unless another still-open gap also blocks that task, in which case it stays blocked. It does
**not** write anything to `.openrecord/compliance/` — that only happens once a task is actually finished
(`run task done`), not when the question blocking it is answered.

**Never resolve a gap on your own read of its evidence.** The evidence is what makes a gap specific
enough to ask about; it is never a substitute for the person actually deciding.

### What to ask for, by kind

| Defining | Ask the person for |
| --- | --- |
| An actor | Which kind (`human`, `external-system`, `time`) and a description of what it is, in their own words — never invent the id or the kind from a guess. |
| A single decision | The context (what forced a choice), the decision itself, the alternative(s) considered and why they lost, the consequences, and — per "What spec this decision serves" above — whether it exists because of a spec already on the books. |
| A new spec | Its purpose, the main flow or rule in GIVEN/WHEN/THEN terms, which components it reaches (`--components`), and, for a `flow` or `process`, which actor initiates it. |
| A new component | The path it owns, a title, and a description of when a reader should descend into it — see "Component add mechanics" below. |
| A gap an `openrecord run` left open | Which gap, found through `openrecord run gaps` — then follow "Resolving a gap, by kind" above. |
| How something existing is verified | A check (`on-change`/`pre-merge`, only if they already name the concrete tool), a sandbox (an environment to bring up and observe), or a `VERIFIED-IN` line (`pre-release`/`production`/`manual`) — exactly the rules "Write it so a check could fail" above already gives; never pick the tool yourself. |

### Coherence before proposing anything

Before you propose a single line — not only before you write — run the same coherence steps situation
1 runs before writing: the neighbourhood (`openrecord grep`/`openrecord map` for what is already filed
nearby, `openrecord impact`/`impact --files` for what governs the same ground), the origin (which spec,
if any, this decision serves), the dependents or `served_by` a write/edit reply will report, the
verification catalogue (`openrecord verification <key>`), and — see "## Reconcile the draft before
writing" above — reconcile the draft before it is ever shown to the person, not only before it is
written.

**What this deliberately does not check: whether the code already complies.** No audit, no `check run`
run against existing code to see if it already does this. A newly defined record is simply `unknown` in
compliance the moment it is written; whether the code already satisfies it, or needs changing to, is
later work — a run's own job, not this skill's.

**A conflict changes what happens next, and it is not the same as situation 1's STOPPED block.** There,
work already shipped and the conflict stops only the part of it that depends on the conflict. Here,
nothing has been built yet: show both sides — what the existing record settles, what the new ask would
require, why both cannot hold — and stop. Nothing is proposed, let alone written, until the person
decides. If they confirm the new one instead, follow through exactly like a `contradicts` gap above:
edit the record that should change, write the one that holds.

### Propose title and one line — never the body

Once the coherence steps above raise nothing that needs a person's decision, propose **one line per
record**: its title, and its one-line description — exactly the thing the "description" of a written
record already has to carry, never the drafted body, never the full frontmatter. The person has not
read any of this yet; showing them a whole record to approve is the `pending`-gap exception above, not
the default here.

```
Proposed:
- decision  decisions/api/uploads/presigned-url.md
            "Uploads go directly to S3 via a presigned URL; the API never receives the file body."
```

**A recap of the body is the body.** Do not follow the proposal with "the record will contain…" and
its sections, however condensed — Context, Decision, Alternatives, Consequences restated as bullets is
exactly the read this step exists to spare them. What you may add after the proposal is only what you
**left out** and why (*"you didn't say how long the URL lasts, so it isn't in"*), because that is
something they did not say, not something they already did.

**Their yes writes it directly as `accepted`, and counts as read** — nothing waits at `pending` for a
second look: the body was built from what they said in this conversation, so confirming the one line
*is* the read-back. Nothing is written before that yes, for any record this
section produces.

**The same rule applies to every registry write this situation makes** — `actor add`, `component add`,
`check add`, `sandbox add`: propose what it would declare in one line, write it only once the person
says yes. Never pick an id, a kind, a command, or a sandbox's setup yourself; those come from what the
person told you to ask for, per the table above.

### Component add mechanics

`openrecord-capture` otherwise never needs this, because finishing work rarely invents a new surface —
but defining something specific sometimes does. The mechanics are `openrecord-onboard`'s own, used the
same way:

```
openrecord component add <id> \
  --path <the directory it owns> \
  --title "<Title>" \
  --description "<when a reader should descend here>"
```

`--title` and `--description` are both required, and both are the person's words, not yours to invent —
the same rule the table above already states. Ask what the project's surfaces are called before naming
one; do not propose a name they have not used.

### Write it, then report

Once a record is confirmed, write it exactly the way "## Writing" above describes — `record write` with
`--status accepted` (never `pending`; the person's yes already settled it), `--serves`/`--depends-on`
on a decision, `--components`/`--initiated-by` on a spec — and read `dependents`/`served_by` from the
reply the same way. Report what was written per record, the same shape "Report what you wrote" above
uses, plus, when the record resolved a gap, which gap and which run.
