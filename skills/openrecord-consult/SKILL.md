---
name: openrecord-consult
description: Find which durable records govern code before changing it. USE THIS SKILL before editing, adding, or refactoring any file in a project that has an `.openrecord/` store — before the first edit, not after. Also when asked what governs a file, what was decided about some area, whether an approach is allowed, or why the code is the way it is.
---

# Consult the records before you write

A project with an `.openrecord/` store has already decided things about the code you are about to
touch. Those decisions were made once, deliberately, and the point of writing them down was that the
next person would not have to guess or re-litigate them.

You are that next person.

## The presence gate — read this first

**If there is no `.openrecord/` directory, or it is empty, this skill does not apply.** Behave exactly
as if openrecord were not installed: no guard, no warning, no mention. Do not tell someone their
project is missing records it never had, and do not offer to create a store because you noticed one is
absent.

An empty store is a project that has not adopted this. That is a legitimate state, not a gap.

## The walk

**1. Which surface are you touching?**

```
openrecord component owners src/api/handlers/user.go
→ owner: api
  map:   decisions/api
  specs: specs/flow/checkout/place-an-order.md
         specs/rule/usage-limits.md
```

Returns the component that owns the path. If it reports that nothing is declared, stop and say so —
without a declared surface there is nothing to check against, and guessing which component a file
belongs to defeats the whole mechanism.

**It answers both halves.** `map` is where the decisions governing this file are filed. `specs` is every
capability that declares this surface — those cannot be resolved from a path, because a capability
crosses surfaces and names them instead of living under one. Both lists are the starting point, not the
answer: descending is still what enumerates.

**2. Descend, reading descriptions.**

Each level returns the `title` and `description` of everything hanging off it, plus a `kind` — `group`
means descend, `record` means open. The descriptions arrive in the response; you never open an index
yourself.

```
openrecord map --for decisions/api
→ [group] security   "Auth, permissions, limits. Descend if you touch who may do what."
  [group] runtime    "Errors, retries, caching. Descend if you touch error propagation."
  [group] data       "Queries and the storage model."

openrecord map --for decisions/api/security
→ [group]  rate-limits    "How limits are computed and applied."
  [record] token-identity "Identity travels in a signed token…"

openrecord map --for decisions/api/security/rate-limits
→ [record] …
```

**Descend into every level whose description matches what you are about to do — not the one that
matches best.** Adding rate limiting to a handler touches `security`, and it also touches `runtime`,
because the 429 is an error response. Picking one is the most common way to miss the record that
governs you.

The descent bottoms out on its own: a subgroup is capped at one level, so there is never a third
step down.

**Descending is cheap; opening is not.** A level costs a few hundred tokens of titles and descriptions
— less than reading one record. So when in doubt, descend, and be selective about what you open.

**Stop when every branch you descended reached records and you decided open-or-not on each.** Not when
you found something: finding one record tells you nothing about whether a neighbouring concern holds
another.

**3. Search.**

```
openrecord search "rate limit" --for decisions/api
→ decisions/api/security/rate-limits/at-the-gateway.md   [record]  line 12, 6 hits
  decisions/api/security/INDEX.md                        [group]   line 3,  1 hit
```

One entry per file, never per line, with `hits` saying how many lines matched — which is the difference
between *mentioned once in passing* and *this is what the record is about*.

**A hit is a file, not a level.** Open a `record`; descend into a `group`'s **parent**. Feeding a hit
path straight to `map` is the obvious next move and it is wrong — `map --for decisions/api/security` is
the level, `decisions/api/security/rate-limits/at-the-gateway.md` is the file. The tool says so if you
try it, rather than reporting that a path nobody wrote does not exist.

`search` always runs a literal pass — it hits exactly when you remember the wording, and misses
entirely when the record says the same thing in other words.

<!-- qmd:start -->
When qmd is registered, the same command also runs a pass by meaning over the same scope — no second
command, no URL to translate. The envelope's `semantic` field says which halves actually ran: `used`
means both did, `lexical-only` means only the wording match did (an older qmd, or one whose embedding
model cannot be reached right now), `unavailable` means neither did. Check `openrecord qmd status` if
you want to know why.

If `semantic` is anything other than `used`, say so and continue with what `search` found — it is a
missing capability, not a failure.
<!-- qmd:end -->

**No search proves an absence.** Whatever you searched with, coming back empty tells you that you did
not find something, never that there is nothing to find. Coverage — *has everything been accounted
for* — comes from reading the indexes on the way down, because a record that never surfaced in a search
leaves no trace of its absence. Search is for locating quickly. It is never the evidence that nothing
was missed.

**4. Read the ones that govern you.** Fully. A record is prose written to be understood, not scanned.

## Surfacing is not governing

The steps above produce **candidates**, deduplicated by path. They speak the store's own paths — a hit
from one is comparable with a hit from another without conversion, whichever half of `search` found it.

They find different things, which is why none of them travels alone:

| | Finds | Misses |
| --- | --- | --- |
| `component owners` | Which surface the file is in, and every capability that names it. | Nothing about *which* of them applies. |
| The descent | Everything filed under the concerns you entered. The only one that *enumerates*. | What sits in a concern you did not think to enter. |
| `search` | Exact wording, plus meaning when semantic search is set up for this project. | The record that says the same thing in other words, when it is not set up. |
<!-- qmd:start -->
| `search`, meaning half | Meaning, across the whole component at once. | Nothing systematically — but it ranks, so it can leave something out. |
<!-- qmd:end -->

**Then comes the part no command can do: deciding which of them actually governs your work.** A record
can surface in every one of them and still not apply. Making that call needs the work in view — which
files, what change — and that is yours.

## Say what you found

End the walk with this stated, not just held in your head:

```
Governs this work:

- decisions/api/security/rate-limits/at-the-gateway.md        [accepted]
  Constrains you: the limit lives in the gateway; do not add per-handler checks.
  Surfaced by: descent (api/security)

- specs/rule/usage-limits.md                                  [accepted]
  Constrains you: 60 req/min, 429 with Retry-After.
  Surfaced by: search "rate limit"

Looked at, does not apply:
- decisions/api/data — queries and the storage model; nothing there touches request limits.
```

Three things make this worth writing rather than holding in your head:

- **"Constrains you"** says what you may not do. It is not a summary of the record — a summary makes a
  reader work out the consequence themselves, and they will not.
- **"Surfaced by"** says how it was found, which is how much weight it carries. A record found by
  descending into the concern that owns your work is stronger evidence than one a search ranked highly.
- **"Looked at, does not apply"** is the only thing separating *does not apply* from *nobody looked*.
  Leave it out and a reader cannot tell which happened, so they have to redo the walk.

## What to do with what you find

**A record with `status: accepted` governs you.** Build within it.

**A record with `status: pending` settles nothing.** Someone explicitly said this is undecided, so it
constrains nothing. Do not treat it as a constraint, and do not treat it as permission either — if your
work depends on that question being settled, that is worth saying.

**Specs are behaviour, decisions are construction.** If you are changing what a user observes, the spec
is your contract. If you are changing how it is built, the decision is.

## When your work contradicts an accepted record

This is the one thing that stops.

**Stop that line of work** — not everything, only what depends on the conflict — and surface it with
both sides in view:

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

Arriving with *"this conflicts, what do I do?"* puts the whole problem back on the person. Arriving with
both sides and their reasons lets them decide in one step — which is the difference between stopping
usefully and just stopping.

**Never resolve it on your own.** A record was ratified by a person; overriding it silently makes the
store a lie, and everyone downstream keeps trusting it. Resolving it alone and reporting afterwards is
not a consultation — it is a decision you took and then announced.

If the record turns out to be wrong or stale, that is a fine outcome. It is just not yours to conclude
alone.
