---
name: openrecord-setup-search
description: Register a project's openrecord stores with qmd so records can be found by meaning, not only by exact wording. USE THIS SKILL once per project — when semantic search over the records is not available and should be, when a store is added or removed and the registration no longer matches, or when the user asks to set up record search. This is setup, not searching; it runs once, not on every lookup.
---

# Make records findable by meaning

Literal search finds a record when you remember its wording. Semantic search finds it when you know the
topic but not this project's words for it — which is the usual case, and the reason a record someone
wrote two years ago goes unfound.

This registers the stores with `qmd`. It runs **once per project**, not per lookup.

## Before anything: is it wanted?

Semantic search is optional by design. openrecord works without it — searches return what the
deterministic steps found and say the semantic way was unavailable.

So do not set this up because you noticed it was missing. It costs real disk (local embedding models)
and a first indexing pass. Offer it, explain what it buys, and let the user decide.

## The collections mirror the store's isolation

`qmd` filters by collection and by nothing else — there is no way to narrow a query to a subpath once
documents are in. So whatever separation the results need has to exist as separate collections.

| Collection | Covers |
| --- | --- |
| `<project>-decisions-<component>` | `.openrecord/decisions/<component>/` — one per declared component |
| `<project>-specs` | `.openrecord/specs/` |

**Decisions are split by component because decisions are closed by component.** Two components can
reach the same conclusion and they are still two different records, justified by two different
contexts. A search run while working in `api` that comes back with `cli`'s decision has punched through
the separation the whole format is built on — and it does not read as a bug, it reads as an answer.

**Specs are not split**, because a capability crosses components by definition. Partitioning them would
mean choosing one surface for a behaviour that has several, which is the thing the spec format exists
to avoid.

**Searching across components is not lost.** `qmd` takes several collections in one query and merges
them, so breadth is a flag, not a second search:

```
qmd query "caching" -c proj-decisions-api -c proj-decisions-cli   # across surfaces
qmd query "caching" -c proj-decisions-api                          # only what governs api
```

**Keeping stores apart matters for the same reason.** A search for *behaviour* should not come back
with decisions about how that behaviour is built; those are different questions, and one collection
cannot tell them apart.

**The project prefix is not decoration.** One search server serves every repository from one
configuration. An unprefixed collection name silently repoints another project's collection at this
one — and it fails quietly, by returning the wrong project's records rather than erroring.

## What to index

- **Mask:** `**/*.md` under each store's path.
- **Include the `INDEX.md` files.** They are not tables of contents here — they carry a `title` and a
  `description` saying *when to descend into this level*, which is exactly what someone searching for a
  topic wants to land on. A hit on an index means "descend here"; a hit on a record means "open this".
- **Paths are absolute** when registering, resolved against the repository root, even though everything
  else in openrecord speaks in store-relative coordinates.

## Check the provider before registering anything

Whatever provides the embeddings, its configuration belongs in the environment — **never in a file that
gets committed.** A store is versioned and shared; a key in it is a key published.

Without a working provider, registration still *succeeds*: `qmd collection add` indexes the text and
only the embedding pass needs the provider. So the failure arrives after every collection is registered
and half-built — searches then return real-looking results over a fraction of the store, and nothing
announces the gap.

Which means the check has to come **first**, before the first `collection add`:

```
openrecord qmd status     # is qmd there, does it run, is it the pinned version
qmd doctor                # is the provider reachable and the index healthy
```

If `qmd doctor` reports the provider failing — an expired key shows as a `401` — say so and **stop**.
Do not register collections that cannot be embedded.

## Register

Two commands per collection, and the first one indexes as it registers:

```
qmd collection add "$(pwd)/.openrecord/decisions/api" \
    --name myproject-decisions-api --mask '**/*.md'

qmd collection add "$(pwd)/.openrecord/specs" \
    --name myproject-specs --mask '**/*.md'
```

`openrecord qmd status` lists exactly the collection names this project needs, derived from its declared
components — use that list rather than composing the names yourself:

```
openrecord qmd status
→ collections_needed: myproject-decisions-api, myproject-decisions-web,
                      myproject-decisions-cli, myproject-decisions-root,
                      myproject-specs
```

Then embed, once, and confirm nothing is left pending:

```
qmd embed
qmd status        # "Pending: 0 need embedding"
```

A run that ends with documents still pending is the half-built state above. It is not done until that
number is zero.

## Re-run it when the components change

A registration that no longer matches the repository **does not fail — it goes silent.** The records it
was supposed to cover simply stop being found, and nothing says why.

Because decisions are one collection per component, this is the standing cost of the split: **a
component added or removed leaves the registration stale.** Run this again then, and when the
repository moves. It is cheap, and the failure it prevents is invisible.

`openrecord map --for decisions` lists the declared components, which is what the collections should
mirror. Anything in one and not the other is drift.

## What this does not do

It does not make search authoritative. Even fully indexed, **no search proves an absence** — coming
back empty means you did not find something, not that there is nothing there. Coverage still comes from
reading the indexes with `map`.

And it cannot make an empty answer trustworthy on its own. A query whose provider is unreachable prints
`No results found.` and exits 0, exactly as a genuine miss does. Anything reporting that a search found
nothing has to have established that the search ran — which is what the check above is for, and why it
is worth re-running when a result surprises you.
