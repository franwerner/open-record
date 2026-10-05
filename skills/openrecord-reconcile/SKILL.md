---
name: openrecord-reconcile
description: Check one record against the other records it could contradict, returning pairs with both sides in view — not a verdict on the record alone. USE THIS SKILL right after a record has been written or edited, before trusting the store to be consistent; when asked whether two records agree, whether a decision is already settled elsewhere, or whether a spec promises something a decision forbids; and when a record is suspected of duplicating or contradicting another.
---

# Reconcile a record with the rest of the store

There is no `superseded` status. Every record with `status: accepted` is claimed to be true right
now, simultaneously, alongside every other accepted record in the store. So two accepted records
that cannot both hold is not a historical artifact somebody will get to — it is a live defect,
today, in a store that presents itself as settled.

The code can be perfectly aligned with each of those two records separately and the store is still
broken. `openrecord-audit` would pass both of them. Nothing notices a defect like this on its own;
a store reads as consistent for as long as nobody compares one record against another.

The moment of highest risk is right after a record is written. It was added without reading the
two hundred records already there — nobody does that on every write, and nobody should have to.
This skill is the targeted read that stands in for it.

## The presence gate — read this first

**No `.openrecord/` directory, or an empty one → this skill does not apply.** Behave exactly as if
openrecord were not installed: no warning, no mention, no offer to check something that was never
written. A project that never adopted this is not missing anything, and an empty store settles
nothing, which is a legitimate state, not evidence of neglect.

## The anchor, and why there is always one

Comparing records pairwise does not scale: two hundred records are twenty thousand possible pairs,
and no walk reads through those. So this skill never starts from the whole store. It starts from
one record — the anchor — and builds a bounded neighbourhood around it.

The default anchor is the record that was just written or edited. Anything else, the caller names.

**The anchor is not what gets a verdict.** Every finding this skill reports is a **pair** — the anchor and one other record,
both in view, both carrying their own status. A report that says "record X is fine" or "record X is
broken" has already lost the thing that made the check meaningful.

## Building the neighbourhood

Three sources. Each is a real command. Each catches a different kind of pair and misses the others
structurally — read what each one cannot see, not only what it can.

**1. Its siblings.** Everything filed under the same parent level shares a subject by construction,
which is why it was filed together in the first place.

```
openrecord map --for decisions/api/security
→ [group]  rate-limits    "How limits are computed and applied."
  [record] token-identity "Identity travels in a signed token…"
```

Catches the near-duplicate and the direct contradiction — two records about the same concern are
the two most likely to disagree. Misses anything filed elsewhere, including a record about the same
subject that landed in a different concern by an earlier author's different judgement call.

**2. The declared crossing — spec against decision.** The highest-value source, and the most
expensive one. A spec names its surfaces in frontmatter — `components: [api, web]`. A decision
carries no such field; it is scoped by the folder it lives in. So from a spec, look at the decisions
of every component it names:

```
openrecord map --for decisions/api
→ [group] security   "Auth, permissions, limits."
  [group] runtime    "Errors, retries, caching."
```

From a decision, the reverse is one call, because a component's declared repository path resolves
straight to the specs that name it:

```
openrecord component owners src/api
→ owner: api
  specs: specs/rule/idempotent-writes.md
         specs/flow/place-an-order.md
```

This pair is worth the cost because a spec and a decision **contradict without sharing vocabulary.**
A spec promises what an actor observes; a decision constrains how it is built. "Past 60 req/min you
get a 429" and "the gateway keys its bucket by client, not by endpoint" can directly conflict and
share not one word. Neither the sibling read nor a search over key terms finds this pair — only the
declared link does, which is exactly why skipping it is the most common way this check goes wrong.

**3. Semantic and literal neighbours, across the whole store.**

```
openrecord map --for decisions
→ api
  cli
  web

openrecord search --for decisions/api --for decisions/cli --for decisions/web --for specs \
  --literal "rate limit" --semantic "rate limit per client" \
  --context "<the anchor's title, description and full body, pasted in — not a sentence about this task>" \
  --omit decisions/api/security/rate-limits/at-the-gateway.md
```

Repeat `--for` once per declared component plus `specs` — the search covers the **whole store**, not
just the anchor's own surface. A cross-component conflict is exactly what this pass exists to catch: a
decision in `cli` and one in `web` can contradict each other without sharing a parent, without either
naming the other in a declared crossing, and without a search scoped to one component ever seeing both
sides. `--omit` drops the anchor's own path, so it is never scored against itself.

**`--context` is the anchor record itself** — its `title`, `description` and full body, pasted in whole
— never a sentence describing the reconciliation task. This was measured, not assumed: scoring every
candidate against the anchor's own text gets recall 0.94; describing what you are doing in prose does
not come close. Jev is answering "how close is this record to the anchor," and the anchor's own words
are the only text that actually poses that question.

If the whole-store search's scope is unwieldy — a store with many components — a narrower one scoped to
the anchor's own component plus whatever the declared crossing named is a reasonable fallback, but say so
plainly in the report: a narrowed scope is a weaker coverage claim, and a reader relying on this check
needs to know which one they got.

Together, these three return five to fifteen records, not two hundred. That bound is the whole
reason this skill is runnable at all — a person can read fifteen records against one anchor in one
sitting, and nobody can read two hundred against two hundred.

## Review every record the search returns

`search` stores what it served; skipping the review step is not an option. For every path in `records`
(and any `discarded` record worth a second look):

```
openrecord review open <search-id> <path>
```

Read it as a pair against the anchor — exactly the comparison "What counts as a finding" below asks
for — then mark it:

```
openrecord review mark <search-id> <path> --verdict contradicts   # a genuine finding, defined below
openrecord review mark <search-id> <path> --verdict governs       # same subject, compatible — no conflict, worth the pairing on record
openrecord review mark <search-id> <path> --verdict unrelated     # not actually about the same subject
```

`openrecord review status <search-id>` must exit 0 — every served record carrying a verdict — before
you report anything in "Say what you found" below. A record marked `contradicts` is what gets written up
as a finding there; `governs` and `unrelated` are both "checked, no conflict," and the only difference
between them is whether the two records actually share a subject.

## What counts as a finding

Four categories. State the sentence in each record that cannot both hold — if you cannot state it,
you do not have a finding, no matter which source surfaced the pair.

- **Direct contradiction.** One record says X, the other says not-X, and both are `accepted`.
  *"The limit is applied at the gateway, not in each handler"* against a second record that says
  *"the `/users` handler enforces its own limit."*

- **Spec asks for what a decision forbids.** The declared crossing above, stated as a finding: a
  spec's `## Rule` or `## Scenarios` requires something the named component's decisions rule out.
  *"A client gets a distinct limit per endpoint"* against *"the gateway does not distinguish
  endpoints."*

- **Duplication.** Two records stating the same thing in two places. Not a contradiction today — a
  guaranteed one later, because one of the two will be edited when the topic comes up again and the
  other will not, and from then on the store disagrees with itself. A duplicate found by wording
  alone is the weak signal; the one found by meaning, where that is set up, is the one that
  actually matters.

- **Broken dependency.** A record rests on something a later, separate record reversed. *"A queue
  buys at-least-once delivery under a crash, a guarantee this service already has for free: there is
  one instance"* — written to justify rejecting a queue — against a later record that says the
  worker now runs several replicas. The first record's own reasoning no longer holds, even though
  nothing in it was directly negated.

**Two records covering the same area is not a finding.** A decision about error handling and a spec
about a checkout flow can both mention retries without disagreeing about anything. If you cannot
write the one sentence from each side that cannot both be true, say nothing. Overreporting here is
what gets this skill ignored, and an ignored consistency check is worse than none — it reads as
coverage that was never actually there.

## Status changes what a finding means

Only two `accepted` records in conflict is a live defect: both claim to govern right now.

If either side is `pending`, nothing is settled there. Someone explicitly said this question is
open, so a `pending` record cannot be in the store's way — it is at most worth flagging as a
question the anchor's own acceptance depends on, never as a contradiction. Do not report a `pending`
record as conflicting with anything.

## Say what you found

One block per pair: both paths, both statuses, the conflicting sentence quoted from each side, and
one line on why both cannot hold. Then say how the neighbourhood was built for that pair — which of
the three sources surfaced it — because that says how much weight the finding carries.

```
Reconcile: decisions/api/security/rate-limits/at-the-gateway.md [accepted]

1. decisions/api/runtime/per-endpoint-overrides.md [accepted]
   "/users gets its own limit, independent of the gateway's client-wide bucket."
   vs.
   "The limit is applied at the gateway, not in each handler; the gateway does
   not distinguish endpoints."
   Why both cannot hold: the gateway cannot enforce a per-endpoint override it
   has no way to see.
   Surfaced by: declared crossing (decisions/api)

2. specs/rule/usage-limits.md [accepted]
   "Past 60 req/min for /users specifically, a client gets 429."
   vs.
   the same sentence as above.
   Why both cannot hold: the spec promises exactly the per-endpoint behaviour
   the decision rules out.
   Surfaced by: search "rate limit per client" (20261004T153012Z-a1b2c3)
   Reviewed: contradicts

Checked, no conflict:
- decisions/api/security/token-identity.md — sibling in the same concern, about
  identity, not about limits.
- decisions/api/runtime/retry-budget.md — search "rate limit per client"
  (20261004T153012Z-a1b2c3), reviewed: governs. Same subject (request throttling),
  compatible: this one covers retries after a 429, not the limit itself.
```

Every pair that came from `search` carries the verdict `review mark` recorded for it — `contradicts` is
what makes a pair a finding above; `governs` and `unrelated` are both what makes a pair land in "checked,
no conflict" instead. A pair with no `Reviewed` line is one the review step has not reached yet, and
nothing below should be reported until `review status` says otherwise.

If nothing was found, say what the neighbourhood contained and how each source contributed —
otherwise a reader cannot tell "checked and clean" from "nobody looked," which is the same failure
`openrecord-consult`'s "looked at, does not apply" line exists to prevent.

## It never resolves

This is the hard limit, same as every sibling skill. Reconcile does not edit either record, does not
change a `status`, and does not pick a winner between two accepted records.

The store's own resolution mechanism is worth stating so the report is actionable: there is no
`superseded` status, so resolving a contradiction means editing the surviving record in place and
moving the displaced reasoning into its `## Alternatives` section, with git carrying the history.
That is a person's decision, and the writing skill's job to carry out once they have made it — not
this one's.

A record turning out to be wrong or stale is a perfectly good outcome. It is just not the agent's
to conclude alone.
