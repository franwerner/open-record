---
name: openrecord-consult
description: Find which durable records govern code before changing it, and run the project's checks against it before handing a change back. USE THIS SKILL before editing, adding, or refactoring any file in a project that has an `.openrecord/` store — before the first edit, not after — and again before calling a change finished, to verify it against the project's checks. Also when asked what governs a file, what was decided about some area, whether an approach is allowed, or why the code is the way it is. And before changing, retiring or re-evaluating a record itself — when asked what depends on a record, or what changing or replacing one would affect. Also when asked to continue, resume, or pick up an openrecord run that is already in progress.
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

## If an openrecord run is in progress

A run (task 15) is a unit of work — one scope, one plan, one set of tasks — that can span several
sessions. Before assuming there is nothing to pick up, find out:

```
openrecord run list
```

Cheap, and the one way to find an open run without knowing `.openrecord/runs/` exists. Then:

```
openrecord run status <run-id>
```

Reports the run's own state, a count of its tasks by state, and `drift` against the plan it started
with. Pick up the first task that is `pending` with every dependency already `done`, or resume one
already `in_progress`:

```
openrecord run task start <run-id> <task-id>
openrecord run task done <run-id> <task-id>
```

**`run task done` takes no `--files` flag** — it infers what you touched itself, from a snapshot taken
at `run task start`, by diffing the working tree against it. Edit the files the task needs between
those two calls, then call `done` with nothing else: it runs every check that takes `{files}` against
what actually changed, plus every check that already verifies the task's records (even with nothing
changed at all — those simply have nothing to run), and only moves the task to `done` once everything
that did run passes; a failure leaves it `in_progress` with the results that did run. If it fails, the
fix belongs in the code, exactly as step 5 below says for `check run`.

**Never hand-edit anything under `.openrecord/runs/`.** Every file there carries the same integrity
hash a record does, and a command that reads a tampered one refuses rather than trusting it. Move a
task through `run task start`/`run task done`; replan with `run tasks set`; register a question only a
person can answer with `run gap add`, never by writing the JSON yourself.

## Before changing a record itself

The walk below is for code. When what you are about to touch is a **record** — editing it, changing
its status, retiring it — or when you are asked what depends on or is affected by one, skip the walk
and ask the binary directly:

```
openrecord impact decisions/api/security/rate-limits/at-the-gateway.md
```

It follows every declared, validated link one hop — `qualifies`, `serves`, checks, annotations — and
`depends-on` as a full chain in both directions, so what is affected is read off the store instead of
guessed from memory or a search that cannot prove it found everything.

## The walk

**1. Which decisions and specs govern these files?**

```
openrecord impact --files src/api/handlers/user.go
→ { "files": { "src/api/handlers/user.go": ["api"] },
    "decisions": { "api": [
      { "path": "decisions/api/security/rate-limits/at-the-gateway.md", "status": "accepted",
        "description": "...", "verified_by": [...], "annotated_in": [...] } ] },
    "specs": [
      { "path": "specs/flow/checkout/place-an-order.md", "components": ["api"], "status": "accepted",
        "description": "...", "verified_by": [...], "annotated_in": [...], "scenarios": {...} },
      { "path": "specs/rule/usage-limits.md", "components": ["api"], ... } ] }
```

Pass every file you are about to touch in one call — `--files a.go b.go`, several at once — so whatever
is shared across them comes back once, not duplicated per file. If it reports an error (an
`error`-severity finding anywhere in the store), stop and say so — a neighbourhood built on top of a
broken store is not trustworthy, and resolving by guesswork instead defeats the whole mechanism.

**It answers both halves in one call.** `files` names every declared component whose path contains
each file — not only the most specific; a component declared as `.` (root) governs everything by the
same mechanism a narrower one does, so both can apply at once. `decisions` is every decision filed
under one of those components; `specs` is every capability naming one of them, plus any quality spec
that qualifies one of those capabilities even if it names no component in common — those relations
cannot be resolved from a path alone, which is why they are not part of `files` itself. Each decision
and spec already carries its own `verified_by`, `annotated_in` and (when it has anchors) `scenarios` —
not raw candidates you still have to open to know whether they apply.

If `impact --files` reports no owner at all for a path (`[]`), that path is outside every declared
surface — a gap worth saying, not a reason to guess which component it belongs to.

Before editing, also run `openrecord compliance status --files <the same files>` (task 14): it shows,
among the records that govern what you are about to touch, which ones are `outdated` (edited since the
code last satisfied them), `possibly_stale` (their code changed and nobody looked again), or `blocked`
— worth knowing before you add to that code, not just after.

**2. Search.**

```
openrecord search --for decisions/api --literal "rate limit" \
  --semantic "rate limiting policy for a new endpoint" \
  --context "adding a per-user rate limit to the signup endpoint"
→ { "id": "20261004T153012Z-a1b2c3", "model": "typesafe/jev-1.13-20260917",
    "records": [
      { "path": "decisions/api/security/rate-limits/at-the-gateway.md",
        "literal": { "terms": ["rate limit"], "line": 12, "text": "...", "hits": 6 } },
      { "path": "decisions/api/security/throttling.md" } ],
    "discarded": [ "decisions/api/data/queries.md" ], "omitted": 0 }
```

Every call requires qmd (registered semantic search) and `OPENROUTER_API_KEY` — there is no degraded
mode. `--literal` is exact wording you remember; `--semantic` is a paraphrase of what you are looking
for; `--context` is the work you are about to do. Behind the scenes, a small ranking model (Jev, over
OpenRouter) scores every record in scope against `--context`, from its title and description only —
record bodies are never sent anywhere. `records` is what was served: the top-ranked ones, plus anything
either search mode hit directly, highest-ranked first. `discarded` is everything else in scope, so
nothing silently vanishes.

A served record carries `literal` and/or `semantic` when one of those modes hit it directly; a record
served on ranking alone carries neither. A `literal` hit is one entry per file, never per line — `hits`
says how many lines matched, which is the difference between *mentioned once in passing* and *this is
what the record is about*.

**Every hit `search` returns is a file, never a level.** `records` and `discarded` only ever name
records — a `group` cannot appear in either, because `search` drops index hits before they reach you.
Descending by level is `map`'s job (step 2, above); every path `search` gives you is one to open.

**Every search is stored, and reviewing it is not optional.** The `id` in the response names it, and
before you move on to step 4 you work through every record it served:

```
openrecord review open 20261004T153012Z-a1b2c3 decisions/api/security/rate-limits/at-the-gateway.md
```

Open **every** record in `records`, in the order served, and read each one. Then mark each with your
judgment:

```
openrecord review mark 20261004T153012Z-a1b2c3 \
  decisions/api/security/rate-limits/at-the-gateway.md --verdict governs
```

`--verdict` is `governs`, `contradicts`, or `unrelated`. Marking a record that was never opened fails —
there is no shortcut past actually reading it. A `discarded` record is not required, but open and mark
one anyway when something about it looks worth a second look.

**Do not proceed with the work until `openrecord review status ID` exits 0.** While any served record
still lacks a verdict, this is unfinished, not optional — the review is what turns a list of hits into a
judgment that survives past this conversation, and skipping it leaves every one of them unread.

**No search proves an absence.** Whatever you searched with, coming back empty tells you that you did
not find something, never that there is nothing to find. Coverage — *has everything been accounted
for* — comes from reading the indexes on the way down, because a record that never surfaced in a search
leaves no trace of its absence. Search is for locating quickly. It is never the evidence that nothing
was missed.

**3. Read the ones that govern you.** Fully. A record is prose written to be understood, not scanned.

**4. Check each one against the code in front of you.** Only the ones you just decided govern this
work — one or two, never everything that surfaced.

You are holding both sides already. The record is open, and the file you came here to change is the
reason you opened it. Saying whether they still agree costs one more look at something you are
reading anyway, and this is the only moment it is free: nobody runs a consistency pass over a store
on a quiet afternoon, which is why a store drifts in the first place.

**Report it; do not act on it.** A record that no longer matches the code does not block your work
unless your work depends on that gap. It is a finding somebody needs, and it goes in the block
below next to everything else you found.

**Keep it to what the file in front of you shows.** This is a glance, not an audit — one claim, the
line that supports or contradicts it. When the answer is not visible in the code you are already
reading, say it is not visible and move on. Stretching this step into a claim-by-claim pass over a
whole record is how a walk stops being run at all, and that pass is a skill of its own anyway.

**5. Before you hand back: run the checks.**

```
openrecord check run --files src/api/handlers/user.go
```

This is mechanical, not judgment — the same reason step 1 is a command and not a memory. It resolves
the same file → components → records chain step 1 already did, selects every declared check whose
`enforces` covers a record governing those files, completes `{files}` with what you actually touched,
and runs them. **Exit `0` is the verdict** — every selected check passed; a nonzero exit means something
still fails, and the fix belongs in the code, never in the check, the script it runs, or the record
behind it. Use `--per-change` for a quicker loop while you are still editing (only the checks whose
command takes `{files}`); run it without the flag — the full sweep — before you actually call the work
done.

## Surfacing is not governing

The steps above produce **candidates**, deduplicated by path. They speak the store's own paths — a hit
from one is comparable with a hit from another without conversion, whichever half of `search` found it.

They find different things, which is why none of them travels alone:

| | Finds | Misses |
| --- | --- | --- |
| `impact --files` | Every decision and spec whose declared containment covers the file(s) — the only one that *enumerates* by declared relation. | Anything about the same area that is not reachable by component or `qualifies` — a record filed elsewhere because of an earlier author's different judgement call. |
| `search` | Exact wording (`--literal`), meaning (`--semantic`), and a ranking of everything in scope against `--context`. | Nothing in scope — ranking can leave a low-scoring record out of `records`, but it is always in `discarded`, never gone. |

**Then comes the part no command can do: deciding which of them actually governs your work.** A record
can surface in every one of them and still not apply. Making that call needs the work in view — which
files, what change — and that is yours.

## Say what you found

End the walk with this stated, not just held in your head:

```
Governs this work:

- decisions/api/security/rate-limits/at-the-gateway.md        [accepted]
  Constrains you: the limit lives in the gateway; do not add per-handler checks.
  Surfaced by: impact --files (src/api/handlers/user.go)
  Still true of the code: no — src/api/handlers/users.go:88 keeps a counter of
  its own. Reporting it, not resolving it.

- specs/rule/usage-limits.md                                  [accepted]
  Constrains you: 60 req/min, 429 with Retry-After.
  Surfaced by: search --literal "rate limit" (20261004T153012Z-a1b2c3)
  Reviewed: governs
  Still true of the code: yes — src/gateway/limiter.go:41.

Looked at, does not apply:
- decisions/api/data — queries and the storage model; nothing there touches request limits.
```

Five things make this worth writing rather than holding in your head:

- **"Constrains you"** says what you may not do. It is not a summary of the record — a summary makes a
  reader work out the consequence themselves, and they will not.
- **"Surfaced by"** says how it was found, which is how much weight it carries. A record found by
  descending into the concern that owns your work is stronger evidence than one a search ranked highly.
- **"Reviewed"** is the verdict `review mark` recorded for anything surfaced by `search` — `governs`,
  `contradicts`, or `unrelated`. It is not your own restatement; it is what the review step above
  produced, and a record with no `Reviewed` line here is one the walk has not finished with yet.
- **"Looked at, does not apply"** is the only thing separating *does not apply* from *nobody looked*.
  Leave it out and a reader cannot tell which happened, so they have to redo the walk.
- **"Still true of the code"** is the one line nobody else is positioned to write. It gets recorded
  here because you had the record and the file open at the same time, which is a coincidence worth
  spending: a `no` that goes unsaid here is a store that keeps reading as settled while the code has
  already moved on.

## What to do with what you find

**A record with `status: accepted` governs you.** Build within it.

**A record with `status: pending` settles nothing.** Someone explicitly said this is undecided, so it
constrains nothing. Do not treat it as a constraint, and do not treat it as permission either — if your
work depends on that question being settled, that is worth saying.

**Specs are behaviour, decisions are construction.** If you are changing what a user observes, the spec
is your contract. If you are changing how it is built, the decision is.

## When your work contradicts an accepted record

This is the one thing that stops.

**What step 5 turns up is not this.** Code that already contradicts a record was contradicting it
before you arrived and will keep doing so if you walk away — that is a finding, and it belongs in
the block above with everything else you found. What stops you is narrower: the change *you are
about to make* cannot coexist with a record that governs it.

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
