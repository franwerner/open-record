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

## Set the provider up before registering anything

Without a working provider, registration still *succeeds*: `qmd collection add` indexes the text and
only the embedding pass needs the provider. So the failure arrives after every collection is registered
and half-built — searches then return real-looking results over a fraction of the store, and nothing
announces the gap. Which is why every step below comes before the first `collection add`.

**A fresh qmd is not configured for a hosted provider.** It starts on local models it has not
downloaded, so an install that looks fine fails at the first embed with *"Failed to get embedding
dimensions from first chunk"*.

### Which provider is the user's call, not yours

There are two shapes — local models that qmd downloads and runs, or a hosted API it calls — and
choosing between them is a decision about somebody's machine, their money and their data:

- **Local** costs disk and a download, runs on their CPU or GPU, and sends nothing anywhere.
- **Hosted** answers faster on modest hardware, costs per call, and means every record they index is
  sent to whichever endpoint they name.

That last part settles it: **ask.** Sending a project's decisions to a third party is not something to
arrange on somebody's behalf because it was the quicker path. Ask which they want, and for a hosted
one, which endpoint and which models. If they have no preference, say what the trade-off is and let
them pick.

**Everything below is one worked example**, with the names of a hosted OpenAI-compatible setup filled
in. Read the shape, not the values: the model names, the endpoint and the variable holding the key are
whatever the user's provider calls them. What is *not* an example is the ordering and the precedence —
those are how qmd behaves and they hold whatever is chosen.

**1. Is qmd there, does it run, and which collections does this project need?**

```
openrecord qmd status
```

`usable: false` means it is on the PATH and does not run — reinstall with
`openrecord qmd install --force` before anything else. `collections_needed` is the list to register
below; take the names from there rather than composing them.

**2. Point qmd at what the user chose.** The models go in its index config, `~/.config/qmd/index.yml` —
these two values being an example of the shape, not a recommendation:

```yaml
models:
  embed: openai/text-embedding-3-small
  generate: openai/gpt-4o-mini
```

**The index config wins over the environment.** `QMD_EMBED_MODEL` only applies when that file names no
model, and `collection add` writes the local defaults into it — so a model set only by environment
variable stops taking effect the moment the first collection is registered, silently. Set it in the
file. Writing it before registering is safe: `collection add` fills in what is missing and leaves what
is there. This part is qmd's behaviour, not a preference: it holds for local models too.

**3. Put the credentials in the environment** — **never in a file that gets committed.** A store is
versioned and shared; a key in it is a key published. A local setup needs none of this and skips to 4.

```
export QMD_OPENAI_API_KEY=...
export QMD_OPENAI_BASE_URL=https://openrouter.ai/api/v1
```

Again the shape, not the values: the endpoint is the user's, and the variable names are the ones qmd
reads for an OpenAI-compatible provider. A different kind of backend reads different ones — that is
qmd's documentation to answer, not this skill's.

**4. Confirm it before spending a registration on it.**

```
qmd doctor
```

Two lines say whether the configuration took:

- **`model cache: missing`** — read it against what was chosen. Naming **local** models: they have to
  be downloaded, so run `qmd pull` before going on. Naming **hosted** models: expected and not a
  problem, since those are not files and there is nothing to cache. Naming models **nobody chose** —
  qmd's own defaults, when a hosted provider was the choice — means the config above is not being
  read, and that is what to fix.
- **`QMD_EMBED_MODEL is set to X but index config uses Y`** — the file is winning, as it should. Fix
  the file.

The third line, **`search provider`**, cannot answer yet. Nothing has been embedded, so `doctor` has no
chunk to re-embed and reports `not exercised by these checks, so nothing is claimed about it` — which
is the honest answer on a fresh index, not a failure. **Do not stop on it here.**

Where it does bite is on an index that already has vectors: there, `doctor` re-embeds a sample, and a
provider that is failing — an expired key shows as a `401` — makes it say so and exit non-zero. Then
you stop, and you do not register collections that cannot be embedded.

On a fresh index the provider is confirmed one step later, by the embed itself.

## Register

One command per collection, and it indexes as it registers. The names are the ones
`collections_needed` gave you:

```
qmd collection add "$(pwd)/.openrecord/decisions/api" \
    --name myproject-decisions-api --mask '**/*.md'

qmd collection add "$(pwd)/.openrecord/specs" \
    --name myproject-specs --mask '**/*.md'
```

Then embed, once, and confirm nothing is left pending:

```
qmd embed
qmd status        # "Pending: 0 need embedding"
qmd doctor        # "search provider: answered every call made during these checks"
```

A run that ends with documents still pending is the half-built state above. It is not done until that
number is zero.

## The whole thing, in order

The **order** is the part that generalises. What goes in steps 2 and 3 is whatever the user chose, and
the credential line is only there for a hosted provider.

```
ask                                        # local models, or a hosted API? which?
openrecord qmd status                      # usable? which collections?
$EDITOR ~/.config/qmd/index.yml            # models: embed / generate — theirs
export <the variables their provider needs>
qmd pull                                   # local models only
qmd doctor                                 # models resolved? (provider: see above)
qmd collection add <abs path> --name <from collections_needed> --mask '**/*.md'
qmd embed                                  # this is what proves the provider
qmd status                                 # Pending: 0
qmd doctor                                 # search provider: answered every call
```

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
