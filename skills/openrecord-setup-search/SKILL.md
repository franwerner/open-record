---
name: openrecord-setup-search
description: Register a project's openrecord stores with qmd so records can be found by meaning, not only by exact wording. USE THIS SKILL once per project — when semantic search over the records is not available and should be, when a store is added or removed and the registration no longer matches, or when the user asks to set up record search. This is setup, not searching; it runs once, not on every lookup.
---

# Make records findable by meaning

Literal search finds a record when you remember its wording. Semantic search finds it when you know the
topic but not this project's words for it — which is the usual case, and the reason a record someone
wrote two years ago goes unfound.

This registers the stores with `qmd`. It runs **once per project**, not per lookup.

## Before anything: `openrecord search` requires this

Semantic search is not optional here: `openrecord search` requires a working qmd with embeddings, and
fails outright without one — there is no degraded mode. If this project wants to use `search` at all,
this setup has to happen first.

It still costs something real — disk for local embedding models, or a provider call per record for a
hosted one, and a first indexing pass either way. **Which provider** is still the user's call (see
below); whether to set this up at all usually is not, once `search` is wanted.

`openrecord search` also sends every scoped record's title, description and your `--context` to a
ranking model (Jev, over OpenRouter) at query time — record bodies are never sent. That is a second,
separate disclosure from the embedding provider this skill configures, and both are worth saying
plainly to the user before either runs.

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
- **The mask also catches `INDEX.md` files, and there is no need to carve them out.** `openrecord
  search` is the only thing these collections feed, and it discards every hit on an index before it
  reaches you — scope is records only, by design, so a `group` can never appear in `records` or
  `discarded`. (That is a departure from `grep`, which still returns an index hit meaning *descend
  here* — but `grep` scans the filesystem directly and never touches qmd or these collections.) Leaving
  `INDEX.md` files in the mask costs a few extra chunks at embed time and nothing more.
- **Paths are absolute** when registering, resolved against the repository root, even though everything
  else in openrecord speaks in store-relative coordinates.

## Set the provider up before registering anything

Without a working provider, registration still *succeeds*: `openrecord qmd index` registers every
collection and only the embedding pass at the end needs the provider. So the failure arrives after
every collection is registered and half-built — searches then return real-looking results over a
fraction of the store, and nothing announces the gap. Which is why every step below comes before the
first `openrecord qmd index`.

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
`openrecord qmd install --force` before anything else. `collections_needed` is what
`openrecord qmd index` will register; `sources` shows, for each model and credential key, whether it
currently comes from `env`, `dotenv` or is `unset` — never the value itself.

**2. Point qmd at what the user chose, in `.openrecord/.env`** — not qmd's own global config, and not
the shell. Every openrecord command for this project merges this file under the process environment and
pins it onto every qmd call it makes, so a value set here governs this project and nothing else:

```
# .openrecord/.env
QMD_EMBED_MODEL=openai/text-embedding-3-small
QMD_GENERATE_MODEL=openai/gpt-4o-mini
```

These two values are an example of the shape, not a recommendation. `openrecord` creates
`.openrecord/.qmd/` as this project's own, private qmd state — separate from the global
`~/.config/qmd`, and from any other project's — so the model a collection is built with is this
project's choice alone, recorded the moment `openrecord qmd index` first registers something.

**3. Put the credentials in the same file.** `.openrecord/.env` is git-ignored automatically the first
time any openrecord command runs against a declared store — a hand-made one, or one from an older
version of openrecord, is protected the moment that happens. A store is still versioned and shared, so
a key belongs in this file, never in a record or in `components.json`:

```
# .openrecord/.env
QMD_OPENAI_API_KEY=...
QMD_OPENAI_BASE_URL=https://openrouter.ai/api/v1
```

Again the shape, not the values: the endpoint is the user's, and the variable names are the ones qmd
reads for an OpenAI-compatible provider. A different kind of backend reads different ones — that is
qmd's documentation to answer, not this skill's. The process environment still works too, and still
wins over `.env` when both set the same key (useful for CI, where there is no file to commit at all) —
but there is no longer a reason to keep secrets out of this one file.

**4. Set the ranking model's key in the same file.** `openrecord search` also needs
`OPENROUTER_API_KEY` for Jev, the model that ranks records against `--context`. Without it, qmd can be
fully set up and every search still fails. It is not a qmd setting, but it belongs in the same file
for the same reasons:

```
# .openrecord/.env
OPENROUTER_API_KEY=...
```

**5. Confirm it through openrecord, not through raw qmd.** Only openrecord's own commands read
`.openrecord/.env`; a raw `qmd doctor`, `qmd status` or `qmd pull` never does, so on a fresh project it
reports qmd's defaults even when the file is right. Do not judge the configuration from a raw qmd call.

```
openrecord qmd status
openrecord jev status
```

- In `qmd status`, `sources` should name `dotenv` for each key just written. `unset` for one of them
  means `.openrecord/.env` does not hold it as you think — fix the file before going on.
- `jev status` should report `api_key: true`, `reachable: true` and `sources.key: "dotenv"`. It exits
  non-zero when either is false.

There is no separate download step. Local models are fetched by the embed that `openrecord qmd index`
runs, with this project's `.env` applied, so the model it fetches is the one the user chose. The
provider itself is confirmed by that same embed: a provider that does not answer makes the command fail.

Once a collection exists, the model it was built with is recorded in this project's index config and
wins over a later `.env` edit. Change the model with `openrecord qmd index --rebuild`, never by editing
that config.

## Register

```
openrecord qmd index
```

One command: it reads `collections_needed` itself, registers whatever is missing, re-embeds whatever it
kept, and runs the embedding pass — all against this project's own, pinned qmd state, never the global
one. Its own output says what it added, what it kept, and whether the embed finished:

```json
{"project":"myproject","project_index_dir":"/abs/.openrecord/.qmd","rebuilt":false,
 "added":["myproject-decisions-api","myproject-specs"],"kept":[],"skipped":[],
 "unmanaged":[],"embedded":true}
```

`"embedded": false` or a non-zero exit is the half-built state above — the command failed before
reaching the embed, and nothing announces a silent gap because there is none: a provider failure here
is a hard failure, not a quiet one.

## The whole thing, in order

The **order** is the part that generalises. What goes into `.openrecord/.env` in steps 2 to 4 is
whatever the user chose; the qmd credential lines are only there for a hosted provider, and
`OPENROUTER_API_KEY` is always there.

```
ask                      # local models, or a hosted API? which?
openrecord qmd status    # usable? which collections? current sources?
$EDITOR .openrecord/.env # models, qmd credentials, OPENROUTER_API_KEY — this project's own
openrecord qmd status    # sources: dotenv for every key just written
openrecord jev status    # api_key and reachable both true — exits non-zero otherwise
openrecord qmd index     # registers, fetches models, embeds — exits non-zero on failure
```

## Re-run it when the components change

A registration that no longer matches the repository **does not fail — it goes silent.** The records it
was supposed to cover simply stop being found, and nothing says why.

Because decisions are one collection per component, this is the standing cost of the split: **a
component added or removed leaves the registration stale.** `openrecord qmd index` is safe to run again
any time the repository moves — it only adds what is missing and leaves what is already registered
alone, so re-running it costs nothing when nothing changed.

`openrecord map --for decisions` lists the declared components, which is what the collections should
mirror. Anything `qmd status`'s `collections_missing` names is drift; `openrecord qmd index` closes it.

## What this does not do

It does not make search authoritative. Even fully indexed, **no search proves an absence** — coming
back empty means you did not find something, not that there is nothing there. Coverage still comes from
reading the indexes with `map`.

And it cannot make an empty answer trustworthy on its own. A query whose provider is unreachable prints
`No results found.` and exits 0, exactly as a genuine miss does. Anything reporting that a search found
nothing has to have established that the search ran — which is what the check above is for, and why it
is worth re-running when a result surprises you.
