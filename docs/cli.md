# The CLI

`openrecord` is the reader of the format. Everything it does is deterministic; anything that needs
judgement is a document a host's agent reads, never something the binary decides.

> Status of this document: settled.

## Principles

**JSON on stdout, by default.** The primary consumer is an agent, so the machine-readable form is the
default and the human view is the flag — not the other way round.

**It never calls a model.** A framework that decided which records govern a piece of work would need
one, and would stop being a framework about a format.

**Semantic search is not its job.** The binary owns the deterministic half — navigating, literal
search, validation. Semantic search lives in `qmd`, a separate project in its own repository, which
composes the full search order by calling this binary for the deterministic steps. It is **optional**:
without it, searches return what the deterministic steps found and say the semantic way was
unavailable. The binary never fails for its absence.

## Commands

| Command | What it does |
| --- | --- |
| `map` | Navigate the store, one level at a time. |
| `component` | Declare, remove and resolve components. |
| `level` | Create a concern or subgroup folder with its index. |
| `record` | Write and edit records. |
| `validate` | Check a coordinate is well-formed, recursively. |
| `grep` | Literal search, scoped to a store. |
| `diagram` | Emit a spec as Mermaid on stdout. |
| `skills` | Emit the bundled agent skills into a directory you name. |
| `concerns` | Print the concerns catalogue. |
| `qmd` | Report on semantic search, and install it. |
| `version` | Print the build, and the store format it understands. |

**There is no `init`.** The first `component add` creates everything the store needs, so a command
whose only job is writing an empty file earns nothing.

### Writing is validate-then-persist

Everything that enters the store enters through the binary, and **the same check engine that `validate`
runs also runs on the way in**. A malformed record cannot exist, because it can never be written.

That is what the write path buys. Several of `validate`'s error codes stop being things you find later
and become things that cannot happen: a record with broken frontmatter, a record in a place that does
not correspond to anything, a branch anchored to a step its flow does not have.

The binary validates the prose; it never authors it. The body comes from the agent or the person doing
the work — the *why* of a decision is not something a binary can produce, and a framework that tried
would be inventing.

---

## `map`

The way in. An agent descends one level at a time, reading each level's descriptions to decide whether
to go deeper — never loading a record it has not chosen.

### The coordinate

`--for` takes a path **inside `.openrecord/`**, and the store is simply its first segment. One rule, no
special cases: the coordinate is literally the folder.

```
openrecord map                            → decisions, specs
openrecord map --for decisions            → the components
openrecord map --for decisions/api        → api's concerns
openrecord map --for decisions/api/data   → subgroups and loose records
openrecord map --for specs                → the four types
openrecord map --for specs/flow           → subgroups and loose capabilities
openrecord map --for specs/flow/checkout  → the subgroup's capabilities
```

This is also what makes search and navigation speak the same language. A `grep` hit comes back as
`decisions/api/data/queries.md` — a path in the same vocabulary, but a **file**, not a level. Open it;
the coordinate to descend into is its parent, `decisions/api/data`. Handing the hit itself to `map` is
the obvious next move and the command says so rather than reporting that a path nobody wrote is
missing:

```
openrecord map --for decisions/api/data/queries.md
→ decisions/api/data/queries.md is a record — open it.
  The level holding it is decisions/api/data
```

### What a level returns

The `title` and `description` of everything hanging off that level — read from each child's
frontmatter, whether that child is a folder's `INDEX.md` or a record.

Entries must say which kind they are. At a concern level the answer is mixed — subgroups and loose
records live side by side — and without that an agent cannot tell what it descends into from what it
opens.

```json
{
  "for": "decisions/api/data",
  "entries": [
    {
      "kind": "group",
      "path": "decisions/api/data/queries",
      "title": "Queries",
      "description": "How queries are written and where they live. Descend here if you touch the query layer."
    },
    {
      "kind": "record",
      "path": "decisions/api/data/orm-vs-hand-written-sql.md",
      "title": "Hand-written SQL over an ORM",
      "description": "Persistence uses hand-written SQL; no ORM sits between the domain and the database.",
      "status": "accepted"
    }
  ]
}
```

`status` travels because a `pending` record settles nothing, so an agent can skip it without opening it.

### No counts

Deliberately. A count would tell an agent the next level is large, but it can act on that — the descent
has no level to skip, so it asks for the concern either way and reads what is there.

The one place a count means something is maintenance: a concern holding forty loose records is the
signal that a subgroup is missing. That belongs in `validate` as a warning, not in every navigation
response.

---

## `component`

The component is the surface axis, and everything that depends on scope depends on it being declared.

**Declaring is mandatory.** An empty store is a benign state; an undeclared surface is not. Work whose
surface nobody declared cannot be checked against anything, so the binary reports it and refuses to
resolve scope rather than proceeding silently.

**The binary never proposes names.** Detecting which directories hold source would be mechanical, but
*naming a surface* is not, and those names become the vocabulary every later scope is expressed in. The
names are settled between the user and their agent; the binary only records what it is told.

### `component add`

```
openrecord component add api \
  --path src/api \
  --title "API" \
  --description "The HTTP surface. Descend here if you touch endpoints or request/response contracts."
```

Creates `components.json` if it is absent — with the format version in it — adds the entry, and creates
`decisions/api/` with its `INDEX.md`.

`--title` and `--description` are required, and the command fails without them. The description is what
tells a later reader when to descend into this component, and it is prose the binary cannot invent.
This is the same principle as declaring the component at all: a surface nobody described does not
navigate.

It touches `decisions/` only. Specs are organised by type, not by component — a spec names its
components in its frontmatter and lives elsewhere.

### `component remove`

Removes the declaration and its folder, but **only when nothing references the component**. Two things
hold it:

- records living inside `decisions/<component>/`
- specs listing it in their `components` frontmatter

Either one and the command fails, naming what is holding it. Deleting records as a side effect of a
configuration change is the worst thing this command could do.

### Paths are directories, not globs

A component's paths are plain directories: `src/api` covers that folder and everything under it. No
glob engine, and drift is easier to read when it fails — "this directory does not exist" rather than
"this pattern matches nothing".

**A file has exactly one owner: the longest matching prefix wins.** `src/api/handlers/user.go` belongs
to `api` (`src/api`), not to `root` (`.`), even though both prefixes match. Single ownership is what
keeps this coherent with decisions being closed by component — a file is governed by one component's
decisions, never by two sets at once.

### `component owners`

Resolves a repository path to the component that owns it. This is the bridge from the repo into the
store: an agent knows which files it is about to touch, and this turns that into somewhere to start
reading.

```
openrecord component owners src/api/handlers/user.go
→ owner: api
  map:   decisions/api
  specs: specs/flow/checkout/place-an-order.md
         specs/rule/usage-limits.md
```

**It answers both halves, and they are found differently.** `map` is the coordinate where this
surface's decisions are filed — a decision is closed inside one component, so it resolves from the
path. `specs` is every capability that declares this surface, which cannot be resolved from a path at
all: a capability crosses surfaces and names them in its frontmatter instead of living under one, so it
has to be looked up from the other end.

A path no surface claims reports `owner: null` alongside what *is* declared, and no specs — they are
reached through the owner, and there is no owner. Reported rather than defaulted, because a file
outside every declared surface is exactly what a scope check needs to see.

---

## `level`

Creates a concern or subgroup folder together with its `INDEX.md`. A level cannot appear by accident:
every one of them has to declare when a reader should descend into it, and that declaration is the
whole reason the level exists.

```
openrecord level add decisions/api/security                              → from the catalogue
openrecord level add specs/flow                                          → from the binary
openrecord level add decisions/api/our-own-thing --title "…" --description "…"
openrecord level add specs/flow/checkout       --title "…" --description "…"
```

**Three cases, and the reply says which one it was** in its `source` field:

| Level | Where its prose comes from | `source` |
| --- | --- | --- |
| A concern the catalogue knows | The shipped catalogue | `catalogue` |
| One of the four spec types | The binary — the types are the format, not a project's choice | `shipped` |
| A concern the catalogue does not have, or **any subgroup** | You, via `--title` and `--description` | `given` |

A subgroup never has a default and never will: it is named for whatever its records share, and that is
a judgement about those records. The refusal says so rather than listing the concerns, which are a
different axis and not what was being named.

**The catalogue supplies the default description.** When the name matches one of the shipped concerns,
the command takes its title and description rather than asking. This is the one mechanical use the
catalogue has — everywhere else it is a document you read.

`--description` overrides it, and usually should. The catalogue's text is generic by construction: it
has to hold for any project. An index description is supposed to say when to descend **in this
project**, so the catalogue's wording is a correct starting point without an edge.

A name the catalogue does not have is not a problem — the command just requires `--title` and
`--description`, since there is nothing to default from.

---

## `record`

### `record write`

Writes a record whole. The frontmatter comes in as flags, the body as a file.

```
openrecord record write decisions/api/security/rate-limiting.md \
  --title "Rate limiting at the gateway" \
  --description "Rate limiting is applied at the gateway, not in each handler." \
  --status accepted \
  --body-file ./body.md
```

Both halves are validated: the frontmatter against what the store requires (fields present, every
component declared), and the body against the type's template — the sections that must be there, branch
anchors pointing at steps that exist, transition lines in the fixed shape.

**If anything fails, nothing is written.** The command returns the same `code` and `severity` findings
`validate` returns, so there is one vocabulary for what is wrong with a record regardless of when you
find out.

An existing file is replaced without a flag. The caller always sends complete content, so a
confirmation flag would be one the agent sets every time.

### `record edit`

Replaces one section, addressed by its heading.

```
openrecord record edit decisions/api/security/rate-limiting.md \
  --section "## Alternatives" \
  --body-file ./new-alternatives.md
```

The binary swaps that section, revalidates the whole file, and writes only if it still holds.

**Addressed by section, not by line.** Line numbers are the fragile way to do this: a caller working
from a stale view of the file patches the wrong place, and the result can still pass validation. Here
the sections are known and fixed, so the caller says what it means — *change the alternatives* —
instead of translating that into coordinates that may have moved.

---

## `validate`

Checks a coordinate is well-formed, recursively — the whole store by default, or anything under a
coordinate.

```
openrecord validate                        → the whole store
openrecord validate --for decisions/api    → api and everything below it
``` It never judges whether a record is *honest* or still true of the
code — that is reading, not linting.

### Findings are classified

Every finding carries a **stable code**, and the severity is one more field rather than the axis. The
code is what lets an agent act differently per kind without parsing prose: some findings it can fix on
its own (a missing description), others it has to take to a person (a component whose directory
disappeared).

```json
{
  "findings": [
    {
      "code": "branch-anchor-not-found",
      "severity": "error",
      "path": "specs/flow/user-signup.md",
      "message": "Branch anchored to step [7]; the flow has 4 steps."
    },
    {
      "code": "component-path-missing",
      "severity": "warning",
      "path": "components.json",
      "message": "Component 'ui' declares src/ui, which does not exist in the repository."
    }
  ]
}
```

| Code | Severity | What it catches |
| --- | --- | --- |
| `orphan-component-folder` | error | A folder under `decisions/` with no declared component. |
| `undeclared-component` | error | A spec naming a component that does not exist. |
| `empty-components` | error | A spec with no components. |
| `branch-anchor-not-found` | error | A branch anchored to a step its flow does not have. |
| `invalid-frontmatter` | error | Frontmatter absent or malformed. |
| `index-missing-description` | error | An `INDEX.md` with no `title` or no `description`. |
| `component-path-missing` | warning | A declared directory that is not in the repository — silent drift. |
| `unreachable-state` / `dead-end-state` | warning | A state with no way in, or no way out. A design bug nobody sees today. |
| `concern-too-flat` | warning | Many loose records in one concern: a subgroup is missing. |

### Exit code

Non-zero when any finding is an `error`. The classification is for the agent; the exit code is for the
pipeline, and the two do not get in each other's way — `validate` works as a CI gate and a pre-commit
hook without a second mode.

---

## `grep`

Literal search: the cheap, precise half of finding a record. It hits exactly when you remember the
wording, and misses entirely when the record says the same thing in other words — which is why it never
travels alone.

It is scoped with the **same coordinate as `map`**, so there is one way to say *where* across the whole
CLI:

```
openrecord grep "rate limit" --for decisions/api
→ decisions/api/security/rate-limits/at-the-gateway.md  [record]  line 12, 6 hits
  decisions/api/security/INDEX.md                       [group]   line 3,  1 hit
```

### One entry per file, not per line

A record that says the term six times is still one record, and one thing to go and read. Reporting a
hit per line would make anything counting results over-count by however many times the wording happened
to repeat, and leave the same deduplication to every caller separately.

So each file appears once, carrying the first matching line as the evidence and `hits` as the weight —
which is what separates *mentioned once in passing* from *this is what the record is about*.

### Indexes are searched too

This is a deliberate departure from how the reference behaves. There, `grep` skipped index files, for a
good reason: an index enumerated every record under it, so it matched nearly any term and crowded out
the records themselves.

Our indexes enumerate nothing — they are a `title` and a `description`, and that description is
meaningful content. A hit in one tells you *which level to descend into*, which is exactly what an
agent is trying to find out. The reason for skipping them is gone.

Results say which kind they hit, because the two mean different things: an index hit is *descend
here*, a record hit is *open this*.

---

## `diagram`

```
openrecord diagram specs/flow/user-signup.md
```

Emits **raw Mermaid on stdout** — the one command that does not return JSON. A diagram exists to be
rendered or piped, so `openrecord diagram … > flow.mmd` should work without post-processing, and
anything wanting metadata already has `map`.

Asked for a `rule`, it fails with a clear message rather than returning nothing: an invariant is not a
drawing, and silence would read like a bug.

---

## `skills`

```
openrecord skills --emit .claude/skills/
openrecord skills --emit .claude/skills/ --with-qmd
openrecord skills --emit .claude/skills/ --dry-run
```

Writes the bundled agent skills into the directory you name — `SKILL.md` files with `name` and
`description` frontmatter, the convention a model uses to decide on its own whether a skill is
relevant.

### It remembers what it wrote

Emitting leaves a `.openrecord-emitted.json` in the target directory: the path and content hash of
every file written.

Without it, a skill renamed or dropped in a later release would stay on disk forever — nothing would
know the file was ours, so nothing would remove it, and an agent would keep loading something this
build no longer ships. That is the failure this record exists to prevent, and it is silent by nature.

The hash is what makes deletion safe. On each emit:

| Situation | What happens |
| --- | --- |
| Shipped, absent on disk | `created` |
| Shipped, identical | `unchanged` |
| Shipped, differs but matches what we last wrote | `updated` |
| Shipped, differs from what we last wrote | `overwritten` — these are generated files, but the local edit is reported rather than lost quietly |
| No longer shipped, still matches what we wrote | `removed`, and an emptied directory is pruned |
| No longer shipped, but locally edited | **`kept`** — deleting would destroy work nobody asked us to touch |
| Never written by us | untouched, and not in the plan at all |

A `kept` file also drops out of the record, so a later emit does not decide it may delete it after all.

An absent or unreadable record reads as empty. The worst that costs is a stale file nobody removes,
which is better than deleting on the strength of something we cannot trust.

`--dry-run` reports the same plan and writes nothing.

### `--with-qmd`

Semantic search is optional, so what gets emitted depends on whether the project wants it.

Without the flag, the skills come out with **no mention of `qmd` anywhere**, and
`openrecord-setup-search` is not emitted at all. An agent should never read about a tool the project does
not have — that is how you get it trying to run something that is not installed.

With the flag, the semantic step appears in `openrecord-consult` and `openrecord-setup-search` comes along.

**The command only offers; the person installing decides.** No interactive prompt — that would break
piping and be useless in CI.

Behind this is one authoring rule for the skills themselves: **the `qmd` passages are purely
additive.** The base text is written to be true whether or not semantic search exists, so the flag only
ever adds. If a passage comes out as *"without qmd do X, with qmd do Y"*, the base is claiming too much
and gets rewritten — which is why the rule that matters here, *no search proves an absence, only the
indexes enumerate*, is stated once and holds in both worlds.

**It emits; it does not place.** The binary does not know what Claude Code is, or Cursor, or Codex. It
knows what the content is and writes it where it is told. Whoever installs does the placing — and an
agent already knows where its own skills go.

This is deliberate. The alternative is a table mapping every tool to its layout, and that table is the
first thing to break when a tool moves its directory. Supporting a new tool here is a line in a README,
not a code change. If the table ever earns its place, it is this command plus a lookup — nothing has to
be undone to add it.

**Why skills and not slash commands.** A slash command is invoked by a person typing it. A skill is
loaded by the model when its description matches what it is about to do. The behaviour that matters
here — *before touching this code, find out what governs it* — is exactly what nobody remembers to
invoke, so it has to fire on its own.

---

## `concerns`

```
openrecord concerns
openrecord concerns --json
```

Prints the catalogue — eleven concerns and, under each, the places projects usually have a decision
worth recording.

**This is how the catalogue is reached, and the only way.** It ships inside the binary and never enters
a project, so a skill or an instruction that named a file path would be pointing at something that does
not exist where it is read. Everything a consumer needs comes through a command.

The document is printed **verbatim** from the embedded copy rather than rebuilt from parsed entries:
the document is the deliverable, and anything that reassembled it would drift from what ships.

`--json` returns the concern names with their descriptions, for a caller that wants to pick one rather
than read. The topics stay out of it — they name nothing and exist to be read.

---

## `qmd`

Semantic search is a separate project and openrecord does not own it. These two subcommands are the
whole relationship.

```
openrecord qmd status
openrecord qmd install
```

**`status`** reports whether `qmd` is on the PATH, whether it **runs**, its version against the one
openrecord pins, and **the collections this project needs** — one per component for decisions, one for
specs.

*Present* and *usable* are separate answers because present-and-broken is a real state and a common
one: a qmd whose native database bindings are missing answers `--version` and dies on every command
that opens the index. `--version` is therefore the one question that proves nothing, so `status` also
runs a command that opens the index, and reports what it said when it failed:

```
openrecord qmd status
→ installed: true
  usable:    false
  version:   qmd 2.5.3     pinned_version: 2.8.3-mate.4
  trouble:   Error: Could not locate the bindings file.
  note:      on the PATH but it does not run, so every search will fail —
             reinstall with `openrecord qmd install --force`
```

It does not check whether those collections are registered. That lives in qmd's configuration, in
qmd's format, and reading it would break the day that format changes. openrecord says what
registration is required; performing it is qmd's business.

**`install`** installs it, and says what to do next. It refuses early and clearly when npm is absent,
rather than surfacing whatever npm prints when it is not there, and it streams the build — installing
from source is slow enough that silence reads as a hang.

**Three states, and only one of them declines.** Presence is not the question: a qmd on the PATH that
does not run would otherwise leave a caller with no way forward through openrecord's own commands.

| What is there | What `install` does |
| --- | --- |
| Nothing | Installs. |
| A qmd that does not run | Installs, without being asked twice — that is what the command is for. |
| A working qmd, pinned version | Nothing, and says so. |
| A working qmd, some other version | **Nothing**, and reports the mismatch. Replacing something that works is the caller's call, so it takes `--force`. |

The pinned version is named in two places — the binary and `scripts/install.sh` — and a test fails if
they drift, because nothing else would notice: the script would install one version while the binary
reported a mismatch against the other.

### Installing it later means reconciling

Skills already emitted describe a smaller tool than the one now present. Nothing else would notice:
the files are intact and match exactly what was written.

The emit manifest records which variant was written, so the mismatch is reported:

| | |
| --- | --- |
| `--with-qmd`, qmd absent | *not on the PATH; searches will report the semantic way as unavailable* |
| no flag, qmd present | *installed but these skills were emitted without it; re-run with `--with-qmd`* |
| no flag, previously emitted with it | *these skills previously included the passages and no longer do* |

**Re-emitting is what reconciles.** `--with-qmd` turns the stripped passages back into `updated` files
and brings `openrecord-setup-search` along; dropping the flag removes it again.

**The report never changes what is emitted.** Deciding by what happens to be installed would make the
same command produce different files on different machines, which is worse than the problem it solves.

---

## `version`

Prints what this build is and, more usefully, **which store format it understands**.

```
openrecord version
→ { "version": "0.1.0", "commit": "847a465", "date": "2026-08-31T15:19:05Z", "store_format": "1" }
```

The format is the field that matters. A store records the format it was written in, and a build reading
one it does not understand fails loudly rather than misreading it — so when that happens, this is the
command that says which side is out of date.

The version, commit and date are stamped in at build time. A binary built without them reports `dev`
and `unknown`, which is how a local build is told apart from a released one.
