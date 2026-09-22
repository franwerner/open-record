---
name: openrecord-audit
description: Check whether a single accepted record still describes the code it governs, claim by claim, with file-and-line evidence for what holds, what is contradicted, and what the code has nothing to say about. USE THIS SKILL when asked whether a record is still true of the code, when someone suspects a record is stale or was never implemented, before relying heavily on an accepted record for a big decision, or when asked to audit, verify, or check a specific record against the implementation.
---

# Audit a record against the code it governs

A record was written once, deliberately, describing how something works or how it must work. The
code kept moving after that — a handler got rewritten, a branch was added, an edge case nobody went
back and updated the prose for. Nothing in the store notices any of this on its own; a record sits
there reading as true for as long as nobody checks.

This skill is a person's eyes, borrowed. It reads one record and the code it governs, side by side,
and says plainly whether they still agree. It does not fix anything it finds — it looks, and it
reports.

## The presence gate — read this first

**If there is no `.openrecord/` directory, or it is empty, this skill does not apply.** Act exactly
as though openrecord were not installed — no mention, no warning, no suggestion that a store should
exist here. A project that never adopted this is not missing anything, and an empty store is a
legitimate state, not evidence of neglect.

## One record at a time

The unit of audit is a single record. If the user names several, that is several independent audits,
each producing its own verdict — run them one at a time. Never collapse them into a merged verdict
like "the component is healthy": that average hides exactly the information the person asked for,
mixing a claim that holds with one that doesn't into a sentence that means nothing.

This skill never enumerates a component and audits everything it finds there on its own initiative.
The caller chooses the record, always. If asked to audit a whole coordinate without the caller having
looked at what's under it, say so, and point them at the catalogue instead of guessing which ones
matter:

```
openrecord map --for decisions/api/security
→ [group]  rate-limits    "How limits are computed and applied."
  [record] token-identity "Identity travels in a signed token…"
```

A list nobody chose is a list nobody reads.

## What this skill is not

`openrecord validate` already checks structure — frontmatter present and well-formed, the
`body-hash` matching the body, a spec naming a component that does not exist, an orphan folder under
`decisions/`, whether a record's sections are the ones its kind expects. None of that is this skill's
job, and none of it should be repeated here:

```
openrecord validate --for decisions/api/security/rate-limits/at-the-gateway.md
```

Run that first if you have not. What is left once structure passes is the part the tool's own
documentation names directly: "It never judges whether a record is honest or still true of the code
— that is reading, not linting." That reading is this skill.

Nothing in a record's body is machine-checkable, and that was a deliberate choice, not an oversight —
a section of checkable assertions and a section of file-glob scope were both considered for the
format and rejected, because the record is prose and a checkable rule pulled out of it drifts from
the sentence it came from the moment either one changes. So there is no rule list to run here. There
is only prose to understand and code to read against it.

## The walk

**1. Open the record and enumerate what it claims.** Read it whole, start to finish — not the
description, not whichever section sounds relevant. Then separate what it claims from why it claims
it. A record says a handful of things that are either true or false of the code today, and a lot of
prose explaining *why* someone chose that. Only the first half is auditable; the second is judgement
no file can confirm or deny.

Write the claims out as a short list before touching any code:

```
decisions/api/security/rate-limits/at-the-gateway.md [accepted]
1. Rate limiting is applied at the gateway.
2. No handler performs its own rate limiting.
3. The limit is per client, not per endpoint.
```

Do this from the record itself, not from memory of having read it a minute ago — auditing from
memory collapses into confirming what you already believe it says, which is the one failure this
whole skill exists to catch.

**2. Resolve the surface it governs.** A spec's frontmatter already names it —
`components: [api, web]`. A decision carries no `components` field; the folder it is filed under is
the component, on purpose, because a decision is closed by where it lives, not by a list that could
drift from it.

```
openrecord component owners src/api/handlers/rate_limit.go
→ owner: api
```

Then turn each component id into real repository paths through `components.json` — flat
directories, never globs, so a stale declaration reads as *this directory does not exist* rather
than *this pattern matches nothing*:

```json
{"id": "api", "paths": ["src/api"]}
```

If the component declares no path that exists in the repository, stop and say so. There is nothing
to audit against, and guessing at a path defeats the same mechanism `openrecord component owners`
exists to make unnecessary.

**3. Read that code.** Whole, in the order a reader encountering it for the first time would — not
by jumping straight to whatever line the record's own wording seems to point at.

**Do not grep the code for the record's own wording.** This is the single most common way this audit
returns a false clean. Searching for the record's phrases finds exactly what the record already
agrees with, and is structurally blind to what contradicts it — the handler that quietly runs its
own rate limiting does not use the word "gateway" anywhere, so a search for "gateway" never finds
it. The claim is that nothing else does this; grep can only ever tell you where something does.

**4. Give each claim a verdict, with evidence.** Three verdicts, and only three:

- `holds` — `file.go:line` that implements it.
- `contradicted` — `file.go:line` that does something else, plus what it does instead.
- `unverifiable` — the code does not speak to this claim either way.

A verdict with no `file:line` behind it is an opinion, not a verdict. Say so if that is all you have,
rather than presenting it dressed as a finding.

## Unverifiable is not a failure

Half of a good record is the *why*, and a why does not live in the code. "We chose the gateway
because a per-handler limit cannot see a client's total across endpoints" is not something any file
can confirm or deny — no amount of reading `src/api` recovers a rejected alternative nobody wrote
down anywhere but the record itself.

Marking that as drift manufactures work out of nothing, and it trains whoever reads the report to
stop trusting `unverifiable` as a real category — which is exactly the discipline this skill needs to
hold onto. A record can come back with more `unverifiable` claims than `holds` ones and still be in
perfect health.

## Say what you found

```
Audit: decisions/api/security/rate-limits/at-the-gateway.md [accepted]

1. Rate limiting is applied at the gateway.
   holds — src/gateway/limiter.go:41, the gateway applies a token-bucket limit
   before any request reaches a handler.

2. No handler performs its own rate limiting.
   contradicted — src/api/handlers/users.go:118 calls limiter.Allow() again,
   inside the handler, on top of the gateway's check.

3. The limit is per client, not per endpoint.
   unverifiable — the gateway keys its bucket by client id (limiter.go:41), but
   nothing in the code speaks to whether that was the deliberate reason for
   choosing "per client" over "per endpoint".

Bottom line: contradicted — a second rate limit now exists inside a handler
the record says should not have one.
```

Each part earns its place:

- **Evidence on every verdict** is what makes the verdict checkable by the person who has to act on
  it, rather than something they have to take on faith or go re-derive themselves.
- **The bottom line** is what gets read if nothing else is. It has to carry the actual state of the
  record on its own, without requiring the claims above it.
- **Claims listed individually**, instead of folded into one verdict, is what separates *the record
  is wrong* from *one sentence in it aged badly* — those call for very different fixes, and
  collapsing the claims loses the distinction before anyone gets a chance to make that call.

For several records, one block like this per record, plus a compact summary table at the end:

```
| Record                                              | Status       |
| --------------------------------------------------- | ------------ |
| decisions/api/security/rate-limits/at-the-gateway.md | contradicted |
| specs/rule/usage-limits.md                           | holds        |
```

Never a merged verdict across records. The table summarises independent audits; it is not a verdict
of its own.

## It never writes

This is the hard limit. This skill does not edit the record, does not fix the code, and does not
change `status` — not even when the contradiction looks obvious and the fix looks small.

A `contradicted` claim means one of two things is wrong, and which one is somebody's call, not this
skill's. The code may be the bug — someone added a shortcut and never told the record about it. Or
the record may have aged out — the gateway-only rule may have been deliberately relaxed and nobody
wrote that down. Present both sides with their reasons, the way `openrecord-consult`'s `STOPPED`
block does, and stop there:

```
STOPPED: decisions/api/security/rate-limits/at-the-gateway.md contradicts the code

What the record settles:
  Rate limiting is applied at the gateway, not in each handler.

What the code does:
  src/api/handlers/users.go:118 also calls limiter.Allow() inside the handler.

Ways forward:
  A. The handler is the bug — remove the duplicate check.
  B. The record is stale — a per-handler exception was deliberately added;
     edit the record to say so.

Recommendation: needs a person who knows why users.go was touched.
```

This is stated plainly elsewhere in this tool's own guidance: "the record turning out to be wrong or
stale is a perfectly good outcome. It is just not the agent's to conclude alone."

If a person then decides the record should change, `openrecord-capture` is the skill that writes it
— not this one.
