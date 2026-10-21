# The CLI

`openrecord` is the reader of the format. Everything it does is deterministic; anything that needs
judgement is a document a host's agent reads, never something the binary decides.

> Status of this document: settled.

## Principles

**JSON on stdout, by default.** The primary consumer is an agent, so the machine-readable form is the
default and the human view is the flag — not the other way round.

**Two models run at query time, and only at query time.** `search` executes `qmd`'s embedding model as
a subprocess for its meaning-based pass, and sends every scoped record's title and description, plus
your `--context`, to Jev (a ranking model, over OpenRouter) to score how well each one fits. Neither
model decides which records govern a piece of work — the embedding model **is** the meaning-based pass,
and Jev's score is a ranking input to what gets served, never a verdict. No query-expansion model and no
rerank model ever run on top of either. A framework that decided which records govern a piece of work
would need a judgement-making model of its own, and would stop being a framework about a format.

**Semantic search is not openrecord's own — it executes another program for it.** The binary owns the
deterministic half directly: navigating, literal search, validation. For the meaning half, `search`
executes `qmd`, a separate project in its own repository, as a subprocess and merges what it returns.
Both qmd and Jev are **required**: `search` fails outright without a usable qmd with embeddings, and
without `OPENROUTER_API_KEY` set — there is no degraded mode and no partial result.

## Commands

| Command | What it does |
| --- | --- |
| `map` | Navigate the store, one level at a time. |
| `component` | Declare, remove and resolve components. |
| `actor` | Declare, edit and remove actors — who can initiate a flow or a process. |
| `check` | Declare and remove checks — task 09's registry of what verifies a decision or a spec. |
| `level` | Create a concern or subgroup folder with its index. |
| `record` | Write and edit records. |
| `run` | Task 15's core: a run's own artifacts — scope, plan, tasks and gaps — never records. |
| `validate` | Check a coordinate is well-formed, recursively. |
| `impact` | Report a record's full neighbourhood, or — with `--files` — which decisions and specs govern a set of code files. |
| `grep` | Literal search, scoped to a store. |
| `search` | Rank every scoped record against a context with Jev, and store the result for review. |
| `diagram` | Emit a spec as Mermaid on stdout. |
| `skills` | Emit the bundled agent skills into a directory you name. |
| `hook` | Claude Code integration: decide a `PreToolUse` hook's allow/deny. |
| `concerns` | Print the concerns catalogue. |
| `verification` | Print the verification catalogue: how a record's claim would be checked. |
| `qmd` | Report on semantic search, and install it. |
| `jev` | Report on the Jev ranking model `search` calls. |
| `review` | Open, mark and check a stored search's review. |
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
openrecord map --for specs                → the five types
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
`decisions/api/` with its `INDEX.md`. Every write stamps `components.json`'s integrity `hash` (see
[decision-record.md](decision-record.md#hash) for the formula), and refuses instead of writing — with
`json-hash-missing` or `json-hash-mismatch`, file untouched — when the existing `components.json`
cannot be trusted: a hand-edit, such as adding a path directly in the file, invalidates it the same way
editing a record's body outside the tool invalidates its `body-hash`.

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

Like `component add`, it refuses before touching the file when `components.json` cannot be trusted —
`json-hash-missing` or `json-hash-mismatch`, file untouched. Restore the file (`git checkout`, typically)
rather than asking the tool to re-stamp over a hand-edit.

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

## `actor`

The actor axis: who can start a `flow` or a `process` from outside the system, and who any record may
mention in its text. The system itself is never an actor — it is who responds.

**Declaring is never required on its own.** A store with no `actors.json` yet is a normal state. It
only becomes a problem the moment a `flow` or a `process` needs to name who starts it, or a body
mentions `@actor:` something that does not exist — both report the usual finding, naming `actor add`.

### `actor add`

```
openrecord actor add administrator --kind human --description "Manages users and content." --alias admin
```

`--kind` is one of `human`, `external-system`, `time`, and `--description` is required — the same
principle as a component's description: an actor nobody described does not tell a reader what fits it.
`--alias`, repeatable, is optional: words prose tends to name this actor with, used only to drive the
"unmarked mention?" warning below.

Creates `actors.json` if it is absent, with the format version and the same integrity `hash`
`components.json` carries (see [decision-record.md](decision-record.md#hash)); refuses instead of
writing — file untouched — when an existing `actors.json` cannot be trusted, the same way
`component add` refuses over a tampered `components.json`. Refused, too, when the id is already
declared, or an alias collides with one already declared on a different actor.

### `actor edit`

```
openrecord actor edit administrator --description "Manages users, content and billing." --alias admin
```

Replaces `--description` and/or the whole `--alias` list (passing `--alias` one or more times replaces
every existing alias, never appends) — the id itself is immutable. `--kind` may also be changed, but is
refused when the change would break a `flow` or a `process` that already names this actor in
`initiated-by`: a `human` actor turning into an `external-system` would silently invalidate every flow
that depends on being initiated by a person. The refusal lists exactly which file(s) would break, and
changes nothing.

### `actor remove`

```
openrecord actor remove administrator
```

Refuses while the actor still appears in some `initiated-by`, or in some `@actor:` mention anywhere in
the store, and lists where — the same principle `component remove` already follows for a component.
Deleting nothing but silently orphaning every record that still names the actor is the worst thing this
command could do.

Like `component add`/`component remove`, every `actor` command refuses before touching the file when
`actors.json` cannot be trusted — `json-hash-missing` or `json-hash-mismatch`, file untouched.

### `initiated-by`

A `flow` or a `process` names who starts it in its frontmatter, as a list of actor ids:

```yaml
initiated-by: [visitor]            # flow: sign up
initiated-by: [payment-provider]   # process: payment webhook
```

**Required on `flow` and `process`, forbidden everywhere else** — `lifecycle`, `rule`, `quality` and
every decision. A `flow` only accepts a `human` actor; a `process` only accepts `external-system` or
`time` — the system is never the one that starts its own process, something outside it is. `--trigger`
has no flag of its own: `## Trigger` in the body keeps describing what happened, in prose; `initiated-by`
says only who.

### Mentions: `@actor:<id>`

Any record — a spec of any type, or a decision — may mention an actor anywhere in its body:

```markdown
## Main flow
1. @actor:visitor submits an email address and a password.
2. The system creates the account.
3. Support is notified if @actor:administrator needs to review it.
```

The pattern is `@` at the start of a line or right after whitespace or an opening bracket/quote,
followed by `actor:` and an id of lowercase letters, digits and hyphens — so a sentence-closing `:`
never becomes part of it, and an email address (`soporte@empresa.com`) never matches, since its `@`
sits right after a letter.

Every actor named in `initiated-by` must be mentioned at least once this way in the same body —
naming who starts something and never saying so in the steps that follow is exactly the gap this
convention closes.

### Validations

Every control above has a stable code, listed with the rest of `validate`'s findings in the codes
table below (`actor-duplicate` through `actors-unreadable`).

The unmarked-mention warning is deliberately narrow: it catches the actor's own words written in the
open, never a synonym nobody declared (*"the buyer"*, with no such alias). That wider case is for
semantic review (`reconcile`, `audit`), which resolves it by adding the alias — the mechanical check
widens with use, never ahead of it.

---

## `check` {#check}

The checks registry (task 09): `checks.json` maps **what verifies a decision or a spec** to **the
command that actually runs it** — a tool, installed and configured once, covering many records at a
time. It never runs anything itself; it only indexes commands and the scope each one is responsible
for. See [decision-record.md](decision-record.md#checks) for the reasoning behind the shape below, and
[verification.md](verification.md#where-it-lands) for how this registry's two cases map onto the wider
`on-change`/`pre-merge`/`pre-release`/`production`/`manual` vocabulary.

**Three levels, each with its own job**, and `checks.json` is only the middle one:

| Piece | Says | Lives |
| --- | --- | --- |
| A tool decision | Why this tool | The store, in prose — typically a `delivery` concern |
| `checks.json` | How it runs, over what | Next to `components.json`, machine-readable |
| Annotations in the code | Which rule or test covers one record | The code itself (a later task) |

### `check add`

```
openrecord check add arch-layers \
  --command "go-arch-lint check" \
  --justified-by decisions/api/delivery/go-arch-lint.md \
  --enforces decisions/api/structure
```

`--command` and `--justified-by` are required, and the command fails without either. `--enforces` and
`--config` are each optional and repeatable. `--needs` is optional and single-valued: the id of a
sandbox (`sandbox.json`, [below](#sandbox)) that has to be running before `--command` can produce
anything — typically an end-to-end or an integration test:

```
openrecord check add e2e-api \
  --command "go test ./e2e/..." \
  --justified-by decisions/api/delivery/testing-strategy.md \
  --needs api
```

It must name a sandbox already declared in `sandbox.json`; a nonexistent one is
`check-needs-not-found`. Declaring `--needs` only records that this check depends on that sandbox —
`check add` shells out to nothing at all. `check run` ([below](#check-run)) is what actually brings that
sandbox up, once, for every check that needs it, exercises nothing itself, and tears it back down again.

**`--justified-by`** is the one decision that justifies running this tool at all, so no entry floats
with no origin — the same principle `serves` and `depends-on` already follow for a decision's own
relations. It must name an existing record, and that record must be a decision: a path into `specs/`,
or one that does not resolve to a record at all, is refused.

**`--enforces`** is what the check is responsible for, which is also its scope: either a whole record
path, or a **group coordinate** — `decisions/api/structure`, exactly the kind `map` and `validate --for`
already resolve. A path ending `.md` is read as a record; anything else is read as a coordinate and
walked with the identical mechanism `validate` itself uses to recurse one, never a parallel resolver
invented for this command. Each entry must resolve to something: a record that does not exist, a
coordinate the store does not know, or a group with nothing filed in it yet, are all refused. **A check
with no `--enforces` at all is a hygiene check** — it is accepted, and enforces no record. It is not the
same weight as one that does: see "Hygiene checks weigh less" below.

**`--config`** is the tool's own configuration files, repository-relative, repeatable — each one must
already exist in the repository.

Creates `checks.json` if it is absent — with the format version in it — adds the entry, and stamps the
same integrity `hash` every other JSON file under `.openrecord/` carries (see
[decision-record.md](decision-record.md#hash)). Refuses instead of writing — `json-hash-missing` or
`json-hash-mismatch`, file untouched — when an existing `checks.json` cannot be trusted, the same way
`component add` and `actor add` refuse over a tampered file. Refused, too, when the `id` is already
declared.

### `check remove`

```
openrecord check remove arch-layers
```

Removes the entry. **There is no `check edit`**, deliberately: unlike a component or an actor, nothing
else in the store ever references a check by its `id` — no record names one — so remove-then-add is
every bit as cheap as an edit would be, and there is no "still referenced" case for an edit to protect
against the way `component remove`/`actor remove` do. Like every `check` command, it refuses before
touching the file when `checks.json` cannot be trusted.

### `check run` {#check-run}

Task 13b: the binary actually running what `check add` only ever declared. `impact`/`impact --files`
report which checks apply; `check run` is what completes `{files}`, runs the command, and turns the
result into an exit code.

```
openrecord check run --files src/api/handlers/upload.go
openrecord check run arch-layers
openrecord check run e2e-api --files src/api/handlers/upload.go
```

**`--files <file...>`** selects every declared check whose `enforces` covers a record governing those
files — the identical file → components → records chain `impact --files` resolves (see
[`#impact`](#impact)), never a second resolver. **An id** (positional) selects that one check instead;
the two combine — `check run ID --files a b` runs exactly that check, substituting `{files}` with those
files — but at least one of them is required. Giving neither, or giving an id whose command contains
`{files}` with no `--files` at all, is a usage error: `{files}` is never completed with nothing, because
that would run over the whole project and a "passed" would claim something that was never run.

Every match's `{files}` is replaced with the files actually requested, each shell-quoted and
space-separated — `--files` is read the same way `impact --files` reads it (space-separated, repeated,
or comma-separated, freely mixed). A check with no `{files}` in its command just runs, whether or not
`--files` was given.

**By default every selected check runs** — the ones with `{files}` and the ones without, the "before
you're done" sweep. **`--per-change`** narrows that to only the ones whose command contains `{files}` —
the "while you're editing" loop. Leaving the flag out is always the safer default: forgetting it never
skips something that mattered, it only costs more time.

Every check runs through `sh -c`, with the repository root as its working directory, and its output is
discarded — not captured, not streamed anywhere; see "No logs or stored output yet", below. A check
with a declared `needs` brings that sandbox up with the same lifecycle `sandbox run` ([below](#sandbox-run))
uses (`setup → start → ready`, no `exercise`), once for every selected check that needs it, runs them,
and tears it down again — always, even when one of them fails. If the sandbox never comes up, every
check that needed it is reported `failed` without running at all, and the sandbox's own steps are
reported under `sandboxes`.

Validated first, over the whole store, exactly like `impact`: an error anywhere refuses before anything
runs, reporting the same findings `impact` would. Output:

```json
{
  "files": { "src/api/handlers/upload.go": ["api"] },
  "checks": [
    { "id": "e2e-api", "command": "go test ./e2e/... -run TestUpload", "result": "passed", "seconds": 4.2, "needs": "api" },
    { "id": "lint", "command": "golangci-lint run 'src/api/handlers/upload.go'", "result": "failed", "seconds": 0.8 }
  ],
  "sandboxes": [
    { "id": "api", "steps": [
      { "step": "setup", "command": "docker compose up -d", "result": "ok", "seconds": 1.1 },
      { "step": "ready", "http": "http://localhost:8080/health", "result": "ok", "seconds": 0.3 },
      { "step": "stop", "command": "docker compose down -v", "result": "ok", "seconds": 0.4 } ] }
  ]
}
```

`files` is present only with `--files`; `sandboxes` only when at least one was brought up. `checks` is
sorted by id. **The exit code is the verdict** — `0` only when every selected check passed, `1`
otherwise — so a script or an agent never has to parse the body to know whether it is done. A failed
check reports only `"result": "failed"`, with no output: see "No logs or stored output yet" below for
why, and what changes once that lands.

### Why there is no `stage` field

A check runs either on just the files a change touched, or over the whole project — and that is read
off `command` itself, never a separate field:

| The command… | Means | Example |
| --- | --- | --- |
| Contains the literal `{files}` | Runs on just the touched files: cheap, after every change — `on-change` | `golangci-lint run {files}` |
| Does not contain `{files}` | Runs over the whole project: closing a task, or whenever asked — `pre-merge` | `go-arch-lint check`, `go test ./...` |

`{files}` is a **classification `check add`/`check remove` only ever report, never execute** — declaring
or removing an entry never shells out. `check run` ([below](#check-run)) is what actually completes
`{files}` and runs a check's command; see [verification.md](verification.md#where-it-lands) for how
these two values map onto that document's wider vocabulary of `on-change` and `pre-merge`.

### What never belongs in `checks.json`

**Only what a command can verify deterministically.** Manual review and anything statistical or
needing a real environment — load, SLOs, a person looking — are never registered here: a `quality` spec
records that kind of verification in its own `VERIFIED-IN` line, and a decision or a functional spec
records it in its own prose. `check add` has no flag that would let either in, and this is intentional,
not an oversight — see [decision-record.md](decision-record.md#checks) for the fuller reasoning.

### Hygiene checks weigh less

A check with no `--enforces` enforces no record — code health, dead code, duplication. It is accepted,
but it does not carry the same weight as a check that does enforce something: a hygiene check that
counted the same as one backed by a decision would reward optimizing its own metric instead of actually
meeting what was decided.

### No logs or stored output yet

`check run` keeps nothing on disk and hands back no id: a result is `passed` or `failed`, nothing more
— no output on success, and no output on failure either. A follow-up task adds a per-invocation log
folder under `.openrecord/logs/<id>/` (ignored by git, last N kept) and a command that reads one back by
id; until then, a failing check's own output has to be reproduced by running its `command` by hand.

Neither a check's own command nor a sandbox's `setup`/`start`/`exercise`/`stop` carry a timeout yet —
only `ready` does (`ready.timeout`, defaulting to `60` seconds): without one, a sandbox that never comes
up would hang `check run`/`sandbox run` forever. Timeouts for everything else are a separate, later
task.

### Validations

Every control above has a stable code, listed with the rest of `validate`'s findings in the codes table
below (`check-duplicate` through `check-config-not-found`, plus `check-needs-not-found`). `validate`
reports the same findings against `checks.json` as it already sits on disk — a `justified-by` whose
decision disappeared or stopped being a decision, an `enforces` group emptied out, a `config` file that
no longer exists, or a `needs` sandbox that disappeared are all breakage, not just an add-time refusal,
and `config`/`needs` existence is re-checked at `validate` time for exactly that reason: a config file
or a sandbox going missing later is no safer to leave unnoticed than a decision disappearing later is.

---

## `sandbox` {#sandbox}

Task 11's registry: `sandbox.json` declares **how to bring up an environment to observe the system
running**, never a thing the binary itself brings up, exercises or tears down — it only records the
commands a person or an agent (or a `checks.json` entry through `needs`) runs. See
[decision-record.md](decision-record.md#sandbox) for why this registry is named `sandbox` and not
`observability` or `runtime`.

A test can pass while the real output of the system shows something is wrong — a swallowed error, a
recovered panic, a retry storm, a repeated or slow query, a timeout. No exit code catches that; it
takes watching the system run, under real traffic, and reading what it produces. `sandbox.json` is
what makes that reviewable: three uses, any number of sandboxes, each with its own `description` so a
reader (or an agent) picks the right one without guessing:

| Use | Field |
| --- | --- |
| The environment a `checks.json` check needs running (an e2e, an integration test) | `needs` on the check |
| A load test approximating a `quality` scenario's threshold, locally | `approximates` |
| Reviewing a decision against real output, with volume | `observes` |

### `sandbox add`

```
openrecord sandbox add api \
  --description "API with its database, to run the e2e suite and watch it work." \
  --component api \
  --setup "docker compose up -d" \
  --ready-http http://localhost:8080/health --ready-timeout 60 \
  --exercise "./scripts/smoke-api.sh" \
  --logs-source "docker compose logs api" --logs-format json \
  --stop "docker compose down -v"
```

`--description` is required — the same principle as every other registry's description: a sandbox
nobody described does not tell a reader what it is for. Required too: exactly one of `--ready-http`
(a URL that answers once it is up) or `--ready-log` (a line to wait for in its output); `--logs-source`,
where its output is read; and `--stop`, how to tear it down. At least one of `--setup` or `--start` is
required — nothing brings a sandbox up otherwise.

**`--ready-timeout` and `--logs-format` are optional**, defaulting to `60` (seconds) and `text`: left
out, the command writes the default into `sandbox.json` explicitly rather than leaving the field
absent, so the file always states what applies and a later reader never has to know the default by
heart. `ready.timeout` is always a whole number of seconds — never a duration string like `60s` — and
given, it has to be a positive integer: zero, a negative number, and anything that is not an integer at
all are all `sandbox-ready-timeout-invalid`. `--logs-format`, given, has to be one of `json`/`text`
(`sandbox-logs-format-invalid` otherwise).

`--component` (repeatable) must each already be declared in `components.json`. `--setup` and
`--exercise` are repeatable commands; `--start` is single-valued, for when `--setup` does not already
bring everything up on its own (`docker compose up` commonly does).

**`--exercise`** is optional and deliberately not a check: whether it "passes" is never judged, only
the output it produces. A sandbox just brought up barely writes anything worth reading; `--exercise`
is what makes it do real work — a smoke script, a load test — so there is something to look at. Keep
it a fixed command: a new failure is then attributable to the change, not to a different path taken
run to run. When a sandbox comes up because a `checks.json` check names it with `needs`, that check's
own command already generates traffic and `--exercise` can be left out.

**`--approximates`** (repeatable) links a load sandbox to the `quality` scenario it approximates:
`specs/quality/<name>.md#<anchor>`, the anchor required (unlike `qualifies`, a sandbox approximates
one scenario at a time, never a whole spec). The target must exist, must itself be a `quality` spec,
and the anchor must be one of its scenarios — `sandbox-approximates-not-found`,
`sandbox-approximates-not-quality`, `sandbox-approximates-anchor-not-found`. It **detects a
regression against the previous run on the same machine** (2s before, 6s now) and catches gross
problems (N+1 queries, a missing index, a leak) — it never **certifies** the scenario's exact
threshold: there is no real network, the data is small, and the machine competes for CPU with
everything else running on it. See [decision-record.md](decision-record.md#sandbox) and
[capability-spec.md](capability-spec.md#VERIFIED-IN) for how this clears `quality-unverified`.

**`--observes`** (repeatable) names decisions this sandbox's output is judged against — a path to a
decision, never a spec and never an anchor. It must exist and must be a decision —
`sandbox-observes-not-found`, `sandbox-observes-not-decision`. Use it for a rule that only shows up
under real volume ("no query takes more than 50ms with the test dataset"); a rule a command can check
deterministically belongs in `checks.json` instead, never here.

Creates `sandbox.json` if it is absent, with the format version and the same integrity `hash` every
other `.openrecord/` JSON file carries (see [decision-record.md](decision-record.md#hash)); refuses
instead of writing — file untouched — when an existing `sandbox.json` cannot be trusted, the same way
`check add` refuses over a tampered `checks.json`. Refused, too, when the id is already declared.

### `sandbox edit`

```
openrecord sandbox edit api --setup "docker compose -f compose.v2.yaml up -d"
```

Replaces whatever flag is given, on top of the existing entry — the id itself is immutable, and every
other field may change. A repeatable flag given at all (one or more times) **replaces the whole list**,
never appends to it — passing `--setup` once replaces every existing setup command. `--ready-http` and
`--ready-log` are handled together: giving either one replaces the whole ready condition and clears the
other side, so switching from one to the other never has to fight the "exactly one" rule `sandbox add`
enforces. The same validation `sandbox add` runs applies to the edited entry before it lands.

### `sandbox remove`

```
openrecord sandbox remove api
```

Refuses while some `checks.json` entry's `needs` still names it, and lists which — the same principle
`actor remove`/`component remove` already follow for what still references them. Like every `sandbox`
command, it refuses before touching the file when `sandbox.json` cannot be trusted.

### `sandbox run` {#sandbox-run}

Task 13b: the binary actually bringing a sandbox up, exercising it, and tearing it down — `sandbox
add`/`sandbox edit` only ever declare how to.

```
openrecord sandbox run api
openrecord sandbox run --files src/api/handlers/upload.go
```

**An id** (positional) runs that one sandbox. **`--files <file...>`** instead resolves every sandbox
that `observes` a decision governing those files, `approximates` a quality anchor of a spec governing
them, or is `needs`-ed by a check whose `enforces` covers a governing record — the same file →
components → records chain `impact --files`/`check run --files` resolve. **Never** a sandbox's own
`components` field: that would bring up a sandbox with no actual relationship to what is being changed.
The two are mutually exclusive — an id or `--files`, never neither, never both.

**Lifecycle, in order: `setup` → `start` → `ready` → `exercise` → `stop`.** `ready` polls `ready.http`
until it answers `2xx`, up to `ready.timeout` seconds (`ready.log` — waiting for a line in `start`'s own
output — is not implemented yet; a sandbox declaring it fails this step every time until a later task
adds it, a deliberate, documented gap rather than a half-implemented attempt). `logs` is **not run** in
this task (there is nowhere to put what it would capture — see "No logs or stored output yet" under
[`#check`](#check)); the cycle this command runs is `setup → start → ready → exercise → stop`. Once any
step fails, every later one before `stop` is reported `skipped` rather than attempted — **`stop` always
actually runs**, so nothing is left up or listening because an earlier step failed.

Every step runs through `sh -c`, with the repository root as its working directory, output discarded —
the same execution `check run` uses. Validated first, over the whole store, exactly like `impact`.

Output, one entry per sandbox:

```json
{
  "id": "api",
  "because": {
    "observes": [ { "path": "decisions/api/uploads/size-limit.md", "status": "accepted", "description": "..." } ],
    "approximates": [ { "anchor": "upload-speed", "measure": "p95 < 10s" } ],
    "needed_by": ["e2e-api"]
  },
  "steps": [
    { "step": "setup", "command": "docker compose up -d", "result": "ok", "seconds": 1.1 },
    { "step": "ready", "http": "http://localhost:8080/health", "result": "ok", "seconds": 0.3 },
    { "step": "exercise", "command": "k6 run upload.js", "result": "ok", "seconds": 9.8 },
    { "step": "stop", "command": "docker compose down -v", "result": "ok", "seconds": 0.4 }
  ]
}
```

`because` only ever carries the keys that apply (`observes`, `approximates`, `needed_by`); with an id,
it is computed the same way but against the **whole store**, since an explicit id already says which
sandbox and there is no file list left to scope it by. With `--files`, the top level wraps every
selected sandbox under `sandboxes`, alongside `files` — the same file→components map `impact --files`
reports:

```json
{ "files": { "src/api/handlers/upload.go": ["api"] }, "sandboxes": [ { "id": "api", "because": {...}, "steps": [...] } ] }
```

**The exit code says whether the cycle completed — `setup` through `stop` with nothing failing — never
whether the logs or the exercised output "look fine".** Reading a sandbox's output against what it
`observes`, or a load number against its `MEASURE`, stays judgment, and judgment is not this command's
job.

### Validations

Every control above has a stable code, listed with the rest of `validate`'s findings in the codes table
below (`sandbox-duplicate` through `sandbox-observes-not-decision`). `validate` reports the same
findings against `sandbox.json` as it already sits on disk.

---

## `compliance` {#compliance}

Task 14's registry: did the code a record governs actually get checked against it, and does it still
hold? Nothing else in the store remembers this — without it, any automation implementing records would
have to reverify the whole store on every run, which stops scaling the moment a project has more than a
handful of accepted records.

**It is a shortcut for not reverifying what is already known to hold, never the source of truth.** The
truth is always comparing the real code against the real record; `compliance` only remembers the last
time that comparison was made, against which exact version of the record, and what it found. A record
with no compliance file is not "unimplemented" — it is **unknown**, and the first thing any flow working
through it does is find out, never assume either way. A brand-new project adopting openrecord starts with
every record unknown, and that is the normal state, not a backlog of failures.

### Where it lives

```
.openrecord/compliance/
  specs/flow/upload-file.json
  decisions/api/data/presigned-uploads.json
```

One JSON file per record, at the record's own store-relative path with `.md` replaced by `.json` —
mirroring the store's own layout so two records never contend for the same file. Written only by the
binary (issue 03), and **committed together with the code that earned the event**: if a change never
reaches the main branch, its compliance file never claims anything either, because it never existed on a
branch anybody merged.

### Content

```json
{
  "version": "1",
  "events": [
    { "event": "adopted", "body-hash": "50d858e0...", "date": "2026-09-10", "verified-with": ["audit"] },
    { "event": "blocked", "body-hash": "9a8b7c6d...", "date": "2026-10-01",
      "blocked-by": [ { "kind": "pending", "record": "decisions/api/uploads/virus-scan.md" } ] },
    { "event": "implemented", "body-hash": "9a8b7c6d...", "date": "2026-10-04", "verified-with": ["checks", "scenarios"] }
  ],
  "hash": "..."
}
```

An **array of events**, kept whole rather than collapsed to the last one, so the history reads without
having to go dig it out of git. `hash` is the same integrity stamp every other JSON file under
`.openrecord/` carries (see [decision-record.md](decision-record.md#hash)); `version` is the store format.

**Four events**, each carrying `body-hash` (the record's body-hash *at the moment this was logged*) and
`date` (`YYYY-MM-DD`) — both filled by the binary, never given on the command line:

| Event | Means |
| --- | --- |
| `implemented` | Code was written or changed to satisfy the record, and verification passed. |
| `adopted` | No compliance file existed yet, the code already satisfied the record, and this records that without any code change. |
| `verified` | Re-checked with no code change (typically after a `possibly_stale` warning) and still holds. |
| `blocked` | Work stopped on a gap or a contradiction. |

There is no `reason` field, no `by`/committer field, and no `commit` field: a commit cannot record its own
hash, and git already answers "which commit" whenever that matters (see `possibly_stale`, below) — storing
it a second time here would only ever be able to drift from the truth.

**`implemented`/`adopted`/`verified` require `verified-with`**: a non-empty subset of `checks`,
`scenarios`, `sandbox`, `audit` — the four parts of "done" (task 18) — with no value repeated. `verified`
means "the definition of done was actually applied and held," never "the checks passed": a record with no
checks at all is still verifiable, by audit.

**`blocked` requires `blocked-by`**, a non-empty structured list, replacing a free-text `reason`:

```json
"blocked-by": [
  { "kind": "pending",     "record": "decisions/api/uploads/virus-scan.md" },
  { "kind": "contradicts", "record": "decisions/api/data/presigned-uploads.md", "note": "..." },
  { "kind": "missing",     "note": "nothing decides which error wins: size or format" }
]
```

`pending` (blocked on a decision that exists but is not yet accepted) and `contradicts` (blocked because an
existing record says something incompatible) each require `record`, naming a record that exists anywhere
in the store — not only a decision, since what is blocking may itself be a spec. `missing` (blocked on a
decision nobody has written down yet) requires `note` instead, and names no record. `note` is optional on
the other two kinds.

### Compliance state: computed, never stored

`compliance status` (below) never reads a stored "state" field — there is none. The state is always
recomputed from the event history and the record's **current** body-hash, so it can never go stale on its
own the way a cached field could:

| State | How it is computed |
| --- | --- |
| `unknown` | No compliance file, or one with no events. |
| `up_to_date` | The latest fulfilled event (`implemented`/`adopted`/`verified`) has the record's current body-hash, and is not superseded by a later `blocked` on that same body-hash. |
| `blocked` | The latest event is `blocked`, and its body-hash still matches the record's current one. |
| `outdated` | There is a fulfilled event, but the latest relevant one's body-hash no longer matches — the record was edited since. |

A `blocked` event whose body-hash no longer matches the record's current one no longer counts as blocked
at all (the record changed since): the state falls back to whatever the latest *fulfilled* event says,
`outdated` if there is one, `unknown` if there is none.

### `possibly_stale`

A flag on top of any state except `unknown` (an unknown record has nothing to compare against yet).
It means **"this changed and nobody looked again"** — the code governing a record changed since the last
time this registry recorded a look at it, and the record itself did not change. It is never a verdict: if
the record still holds, log `verified` and the flag clears; if it does not, the gap goes into the plan like
any other.

**Triggered by any change, committed or uncommitted, with no threshold**, in a file whose **most specific**
declared component (`components.json`'s own `OwnerOf` — the longest matching path prefix, never "every
component that contains it" the way `impact --files`'s `files` map reads it) is the record's own
component:

- A decision's component is the folder it is filed under (closed by containment, same as everywhere else).
- A spec's components are the ones its frontmatter names; it is `possibly_stale` if *any* changed file's
  most specific owner is one of them.

This is deliberately narrower than `impact --files`. With `root` declared as `.` (the repository root),
"every component whose path contains the file" would mark every `root` record on every single change
anywhere in the repository, and the warning would stop meaning anything. `impact` answers "what do I have
to respect from this file," where `root` counts; `possibly_stale` answers "whose code is this," where it
should not — a change made from inside a narrower component (say, `api`) never marks `root`, only `api`.
What this gives up — a `root` rule quietly violated from within a subcomponent — is exactly what `root`'s
own checks (`check run`) catch instead.

**Based on git, never on dates**: clocks differ between machines, and a rebase rewrites commit dates
without changing any actual content.

| Case | How it is read |
| --- | --- |
| Committed since the last look | `git diff <reference>..HEAD -- <changed paths>`, filtered by owner |
| Not committed at all (modified or new) | `git status --porcelain --untracked-files=all`, filtered by owner |

**Reference point: the last commit that touched that record's own compliance file**
(`git log -1 --format=%H -- .openrecord/compliance/<record path>.json`) — never a date, and never the
event's own commit, which it cannot record (see "no `commit` field," above). The task's own code lands in
the same commit as its `verified`/`implemented` event, so that commit's own changes never trigger the
warning; only what changes *after* it does. A compliance file that has never been committed yet (freshly
logged, nothing committed since) has no commit to diff from — the committed half of the check is skipped
for that record, nothing more; its uncommitted half still runs unconditionally, since that comparison is
never relative to a commit in the first place.

**Not a git repository at all:** `possibly_stale` is reported empty for every record, always — there is no
committed/uncommitted distinction to read without git, and refusing the whole report over one missing
flag would throw away everything else `compliance status` can still say truthfully.

### `compliance status`

```
openrecord compliance status
openrecord compliance status --files src/api/handlers/upload.go
```

Reports every record's state, or — with `--files`, read the same way `impact --files`/`check run --files`
already read it (space-separated, repeated, or comma-separated, freely mixed) — only the records governing
those files, via the identical file → components → records chain. Validated first, over the whole store,
exactly like `impact`: any error anywhere refuses rather than reporting against a store that cannot be
trusted.

```json
{
  "files": { "src/api/handlers/upload.go": ["api"] },
  "unknown": ["specs/flow/export-report.md"],
  "outdated": ["decisions/api/data/presigned-uploads.md"],
  "blocked": [
    { "path": "specs/flow/bulk-delete.md",
      "blocked_by": [ { "kind": "pending", "record": "decisions/api/uploads/virus-scan.md" } ] }
  ],
  "possibly_stale": ["specs/flow/upload-file.md"],
  "up_to_date": 12
}
```

`files` is present only with `--files`. `unknown`/`outdated`/`possibly_stale` are sorted path lists;
`blocked` is a sorted list of `{path, blocked_by}`, carrying the exact `blocked-by` entries of the event
that made it blocked; `up_to_date` is a count, not a list — it is the bucket every record not worth
singling out falls into, and the one openrecord expects to dominate in a healthy project.

### `compliance log`

```
openrecord compliance log specs/flow/upload-file.md --event implemented --verified-with checks,scenarios
openrecord compliance log decisions/api/data/presigned-uploads.md --event adopted --verified-with audit
openrecord compliance log specs/flow/bulk-delete.md --event blocked \
  --blocked-by pending:decisions/api/uploads/virus-scan.md \
  --blocked-by "missing:nothing decides which error wins, size or format"
```

The one way to write this registry outside the implementation flow itself (issue 03 forbids a hand edit) —
and the exact command that flow (issues 15/18) calls at the end of every task.

**`--event`** is required: one of `implemented`, `adopted`, `verified`, `blocked`. **`--verified-with`**
(repeatable, or comma-separated) is required for the first three and forbidden for `blocked`.
**`--blocked-by`** (repeatable) is required for `blocked` and forbidden otherwise.

**`--blocked-by`'s syntax** is one self-contained flag value per entry, `kind:value`, so several entries
never have to coordinate across separate flags:

| Form | Meaning |
| --- | --- |
| `--blocked-by pending:decisions/x.md` | Blocked on `decisions/x.md`, not yet accepted. |
| `--blocked-by contradicts:decisions/y.md` | Blocked: `decisions/y.md` says something incompatible. |
| `--blocked-by contradicts:decisions/y.md:a free-text note` | Same, with an optional note after a second colon. |
| `--blocked-by "missing:nothing decides which error wins: size or format"` | Blocked: nobody has written this decision down; everything after the first colon is the note, verbatim, including any further colons. |

Only the **first** colon ever separates the kind from the rest — a `missing` note (or a `contradicts`
note after its own second colon) is free text and may contain colons, commas or anything else without
being torn apart.

The binary fills `body-hash` (the record's current one) and `date`; it does not read them from the command
line, and it does **not** require `check run` to have passed first — most records have no checks at all,
and a record's compliance is backed by whichever of `checks`/`scenarios`/`sandbox`/`audit` actually ran,
never only by the one this binary itself can execute. It also does not record who ran the command: see "no
`by` field," above.

Validated first, over the whole store, like `compliance status`. Refuses — file untouched — when an
existing compliance file's own `hash` cannot be trusted (`json-hash-missing`/`json-hash-mismatch`, the same
way `check add`/`sandbox add` refuse over a tampered `checks.json`/`sandbox.json`), when `--event` is not
one of the four, when the required flag for that event is empty or missing, when a `blocked-by` entry's
kind is unknown or its required field (`record`/`note`) is absent, or when a `pending`/`contradicts`
`blocked-by` names a record that does not exist. On success:

```json
{
  "record": "specs/flow/bulk-delete.md",
  "event": { "event": "blocked", "body_hash": "9a8b7c6d...", "date": "2026-10-20",
             "blocked_by": [ { "kind": "pending", "record": "decisions/api/uploads/virus-scan.md" } ] },
  "state": "blocked"
}
```

### Validations

Every control above has a stable code, listed with the rest of `validate`'s findings in the codes table
below (`compliance-unreadable` through `compliance-orphan`). A compliance file whose record no longer
exists in the store (renamed or removed) is `compliance-orphan`, a **warning**: it governs nothing any
more, but deleting it is left to whoever maintains the store, never done automatically — the same
principle `component remove`/`sandbox remove` already follow for what still references something.

---

## `run` {#run}

Task 15's core: a run's own artifacts. **A run is not a session** — it is a unit of work (one scope, one
plan, one set of tasks, eventually one PR). It can span several sessions, which is the whole point of
writing it down: a long implementation flow (issue 18) can be cut short and picked up later without
redoing what is already done, and one session may start several runs. A run is never a record: it does
not describe the system, it is the work of one particular pass over it.

**What is built now, and what waits on issue 18** (its only consumer today):

| Now | With issue 18 |
| --- | --- |
| `.openrecord/runs/<id>/` with `run.json`, `plan.json`, `tasks.json`, `gaps/` | `reviews/`, `findings/`, `report.md`, a per-task retry limit, retention |
| `run start --scope` computing a plan over the current checkout | A worktree of its own (issue 20) |
| `run task done`: runs checks, refuses if they fail, writes nothing to `compliance` yet | `run task done` writing a `compliance log` event |

### Where it lives

```
.openrecord/runs/                 ← ignored by git, its own .gitignore
  20261006T143012Z-a3f9c1/        ← one run
    run.json                      ← id, scope, the commit the store was at, state
    plan.json                     ← the plan `run start` computed, once
    tasks.json                    ← tasks' own DEFINITION only — id, title, records, depends_on —
                                     plus a history of earlier plans; never a task's state or files
    tasks/<task-id>.json          ← that task's own EXECUTION: state, when it started and its
                                     snapshot, and — once done — what it found; never written from a
                                     caller's own input, only by `run task start`/`run task done`/a gap
    gaps/<gap-id>.json            ← one file per gap
```

**A task's definition and its execution are two separate files, deliberately.** `run tasks set`
validates and saves a replan by touching `tasks.json` alone; `run task start`/`run task done` and a gap
touch only that one task's own file under `tasks/`, never `tasks.json` — so the two things a replan
needs to decide (what a task covers, and what is already done) never contend for the same write.

Every file carries the same integrity `hash` every other JSON file under `.openrecord/` carries (see
[decision-record.md](decision-record.md#hash)). Any `run` command that reads a file whose hash no
longer matches refuses with a finding rather than working from it — the file itself is left untouched.
**`validate` does not walk `runs/`**: these are never records, and the review of these files lives in
the commands below, not in `validate`.

**No worktree yet.** A run works on the current checkout; an isolated worktree per run is a separate
mechanism, deferred. **No retention yet.** Nothing under `runs/` is ever deleted automatically — decide
later, once there is more than one run's worth of experience to decide from.

### `run start`

```
openrecord run start --scope decisions/api/structure specs/flow/checkout.md
```

`--scope` is **required** for now — one or more store paths, space-separated, repeated, or
comma-separated, exactly like `--files` (impact.go's `extractFiles`). Each one is either a group
(every record under it) or a single record; the plan is the union, deduplicated. **Any path that does
not exist fails the whole call, before anything is written.** Scope resolution is its own step
(`resolveScope`) precisely so a later version can compute a scope on its own when `--scope` is omitted
(a roadmap command, issue 21) without touching anything else here.

Validated first, over the whole store, exactly like `impact`/`compliance status`: a plan built on top
of a store with an error anywhere is not trustworthy.

Writes, in order, only once everything above has succeeded:

- **`run.json`**: `id` (a timestamp plus a random suffix, the same shape `search`'s own ids use),
  `scope` exactly as requested, `commit` (the repository's `HEAD` at the moment the run started, or
  `""` when the repository is not a git repository at all — reported here as the decision, since the
  task left it to be made), `state` (`open` — closing a run waits on a later task), and `created`
  (`YYYY-MM-DD`).
- **`plan.json`**: `records` — the union of every record under the scope, each carrying `path`,
  `status`, `description`, `body_hash`, `compliance` (the exact state `compliance status` computes —
  `unknown`/`up_to_date`/`outdated`/`blocked`), `possibly_stale`, `verified_by` (the same check objects
  `impact` reports) and `scenarios` (the same shape `impact` reports, omitted when the record has no
  anchors). `outside_scope` — records **outside** the scope that an in-scope record depends on (a
  decision serving an in-scope spec, and the full `depends-on` chain upward of in-scope decisions and
  of those serving decisions), kept only when not known to be up to date right now: `compliance` is
  anything other than `up_to_date`, **or** it is `up_to_date` but `possibly_stale` (a decision this
  brief left open, made here: a `possibly_stale` dependency is exactly the kind of thing worth a second
  look before building on it from outside its own scope). Each entry carries `path`, `status`,
  `description`, `compliance` and `needed_by` (the in-scope paths that reach it). **Informative only —
  `run start` never refuses over it.**

`compliance`/`possibly_stale` are computed by the exact same function `compliance status` uses
(`complianceSnapshot`, internal/cli/compliance.go) — never a second implementation that could answer
differently for the same record.

Output: the written `run.json` and `plan.json`, as JSON.

### `run status`

```
openrecord run status 20261006T143012Z-a3f9c1
```

Reports `run.json`, each task's definition merged with its current execution (`tasks.items`: `id`,
`title`, `records`, `state`, and — once `done` — `files`), a task-count summary (`tasks.by_state`), and
`drift` against the plan — **it never updates `plan.json`**, because only a person decides whether to
keep going against a plan that has moved, or start a new run:

- `changed` — in-scope records whose current body-hash no longer matches `plan.json`'s own snapshot.
- `added` — records now under the scope paths that are not in the plan.
- `removed` — plan records that no longer exist.

### `run list`

```
openrecord run list
```

Every run under `.openrecord/runs/`, with its `id`, `state`, `created` and `scope` — cheap: one listing
of `runs/` plus one read per run, no store validation and no plan or task files opened. It exists so a
caller (a skill, a person) can find an open run without knowing `.openrecord/runs/` is where they live.

### `run tasks set` — planning and replanning

```
openrecord run tasks set 20261006T143012Z-a3f9c1 --file tasks.json
openrecord run tasks set 20261006T143012Z-a3f9c1          # reads the list from stdin
```

Reads the **whole** task list as a JSON array, from `--file` or stdin: `[{"id", "title", "records",
"depends_on"}, ...]` — a task's **definition** only, which is all `tasks.json` ever holds. **No `state`
field is accepted** — proposing a plan and marking it done are two different acts, and this command is
only ever the first. One rejected with everything else is reported as `task-state-forbidden`.

The binary — never the caller — enforces, refusing (nothing written) on the first violation, reading
each existing task's own **execution** file (`tasks/<task-id>.json`) to decide which of the rules below
apply, never a field inside `tasks.json` itself:

- **Coverage**: every plan record whose `compliance` is not `up_to_date` (or is `up_to_date` but
  `possibly_stale`) is covered by some task's `records`.
- **Containment**: every task's `records` are records that are actually in this run's plan.
- **Immutability of `done`**: a task whose execution already says `done` must be present in the new
  list, identical in `id`, `title`, `records` and `depends_on` — it can be neither removed nor
  modified. Every other existing task's definition may simply be replaced by whatever the new list
  gives it — only `done` is protected. Its execution file is never touched by this command either way.
- **`depends_on`** names tasks that exist in the same list, with no self-dependency and no cycle.
- **Unique, valid ids.**

A task's execution is untouched by `set` in every case — a new task simply has none yet (implicitly
`pending`), and an existing `in_progress`/`blocked` one keeps exactly the execution it already had.

**Each successful `set` keeps the previous version**, appended to `tasks.json`'s own `history` array
with the date it was superseded — a replan never silently loses what came before it.

### `run task start`

```
openrecord run task start 20261006T143012Z-a3f9c1 task-02
```

Moves a `pending` task (a task with no execution file yet reads as `pending` — nothing has to exist on
disk for a task that was never started) to `in_progress`. Refuses when any of its `depends_on` tasks is
not `done`, or when another task in the same run is already `in_progress` — **tasks inside one run run
in series**: they share the current checkout, and two at once would step on each other.

It writes a brand new `tasks/<task-id>.json` — state `in_progress`, and a **snapshot**: every file git
considers changed right now — tracked modified/staged, or untracked, excluding `.openrecord/` (the same
signal `compliance status`'s own `possibly_stale` reads) — each with a SHA-256 content fingerprint,
under `started.snapshot`. Hashed like every other run artifact, and the one thing that lets `run task
done` (below) infer `{files}` on its own, rather than asking for it. Starting a task again after a gap
put it back to `pending` discards whatever execution it had before — the new snapshot is the only one
that matters from here on.

### `run task done`

```
openrecord run task done 20261006T143012Z-a3f9c1 task-02
```

**Takes no `--files` flag at all** — a stray one is a usage error, like any other unexpected argument.
The files a task touched are never given by the caller; the binary classifies them itself, against the
snapshot `run task start` recorded and git's own history, into three lists:

| List | Rule |
| --- | --- |
| `created` | Exists now, was **not** in the snapshot, and is **not tracked at `HEAD`** either — genuinely new. |
| `modified` | Exists now, and its content differs from the snapshot's own fingerprint when the snapshot had one for it; or, when it did not, differs from what is committed at `HEAD`. |
| `deleted` | No longer exists, but was either in the snapshot or is tracked at `HEAD` — something worth reporting lost. |

A file in neither the snapshot nor currently flagged by git as changed is never considered at all —
nothing suggests it needs a second look. **`{files}` is completed with `modified` ∪ `created` only —
`deleted` is reported but never passed to a check**, since there is nothing left to point one at.

The task must be `in_progress`. Runs, with the **exact same engine `check run` uses** (`executeChecks`,
shared rather than reimplemented — internal/cli/run.go), the union of: every check whose command takes
`{files}`, completed with `modified ∪ created`, and every check whose `enforces` covers any of the
task's own `records` — the latter run **even when nothing changed at all**. A check that takes
`{files}` when nothing changed has nothing to run and nothing to lint: it is **skipped**, reported with
`"result": "skipped"` and a `reason`, rather than refused or run against an empty substitution (which
would report a vacuous pass).

All non-skipped, selected checks passing writes `tasks/<task-id>.json`'s own `done` — `date`, the three
file lists, and the check results — and moves `state` to `done`; both are visible afterward in `run
status`. Any failure writes nothing at all: the task stays `in_progress`, its snapshot untouched, so a
retry diffs against the exact same starting point, and exits non-zero with the results that did run. A
task whose records have no checks, with nothing changed, has nothing to run and passes.

**This does not write to `.openrecord/compliance/` yet** — see the table at the top of this section for
why, and what changes with issue 18.

### Gaps

A gap is a question only a person can answer, with the evidence that raised it.

```
openrecord run gap add 20261006T143012Z-a3f9c1 \
  --id size-vs-format --question "Which error wins when a file is both too large and the wrong format?" \
  --kind missing --evidence specs/flow/upload-file.md#reject-oversized-upload \
  --evidence src/api/handlers/upload.go:41 --blocks task-03
openrecord run gaps
openrecord run gap resolve 20261006T143012Z-a3f9c1 size-vs-format --record decisions/api/uploads/error-precedence.md
```

**`run gap add`**: `--id` (required, validated the same way a run or task id is — no path separators,
no `..`) names the file (`gaps/<id>.json`); refuses if one with that id already exists.
`--question` is required. `--kind` is one of the same three values `compliance log --blocked-by`
already declares — `missing` (nobody has written this decision down: `--record` must be **absent**),
`pending` (blocked on a decision that exists but is not accepted yet: `--record` required and must
exist), `contradicts` (an existing record says something incompatible: `--record` required and must
exist). `--evidence` is required, one or more refs, each either a record path, a `record#anchor` (the
anchor must exist on that record), or a `file:line` (the file must exist); any ref that resolves to
none of the three refuses the call. `--blocks` names existing, not-yet-`done` tasks in this run; each
one's own **execution** moves to `blocked` (`tasks.json`, the definition, is never touched by this
command).

**`run gaps`** lists every **open** gap across every run, each carrying which run it belongs to — the
one command a caller (the definition skill, issue 17) needs: it never has to know `runs/<id>/gaps/`
exists at all.

**`run gap resolve`** requires `--record`, naming a record that exists and is `accepted`. The gap moves
to `resolved`, recording that record; every task it blocked has its own execution moved back to
`pending`, unless another still-open gap also blocks it.

### Validations

| Code | Severity | What it catches |
| --- | --- | --- |
| `run-not-found` | — | A run id that is malformed or names no run. |
| `run-unreadable` | error | A run artifact exists but cannot be parsed, or declares an unknown format. |
| `scope-not-found` | — | A `--scope` value that does not resolve to a record or a group in the store. |
| `task-state-forbidden` | error | `run tasks set`'s input carries a `state` field, or `tasks.json` itself holds one outside `pending`/`in_progress`/`done`/`blocked`. |
| `task-coverage-missing` | error | A plan record needing work is covered by no task. |
| `task-record-not-in-plan` | error | A task names a record that is not in this run's plan. |
| `task-done-immutable` | error | A `done` task was removed or its `id`/`title`/`records`/`depends_on` changed. |
| `task-depends-on-not-found` | error | `depends_on` names a task that is not in the list. |
| `task-depends-on-cycle` | error | A task depends on itself, or `depends_on` edges form a cycle. |
| `task-duplicate` | error | A task id declared more than once in one `run tasks set` call. |
| `task-not-found` | — | A task id that names no task in the run. |
| `task-not-pending` | — | `run task start` on a task that is not `pending`. |
| `task-dependency-not-done` | — | `run task start` while a `depends_on` task is not `done`. |
| `task-in-progress-conflict` | — | `run task start` while another task in the run is already `in_progress`. |
| `task-not-in-progress` | — | `run task done` on a task that is not `in_progress`. |
| `task-snapshot-missing` | error | `run task done` found no snapshot for the task — `run task start` was never run for it, or its snapshot was removed by hand. |
| `gap-not-found` | — | A gap id that names no gap in the run. |
| `gap-kind-invalid` | error | `--kind` is not `missing`, `pending`, or `contradicts`. |
| `gap-record-missing` | error | `--record` absent for `pending`/`contradicts`, or given for `missing`. |
| `gap-record-not-found` | error | `--record` does not resolve to a record. |
| `gap-record-not-accepted` | error | `run gap resolve --record` names a record that is not `accepted`. |
| `gap-evidence-invalid` | error | An `--evidence` ref resolves to neither a record, a `record#anchor`, nor a `file:line`. |
| `gap-blocks-not-found` | error | `--blocks` names a task that is not in the run, or is already `done`. |
| `gap-duplicate` | error | `run gap add --id` names a gap that already exists in this run. |

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
| One of the five spec types | The binary — the types are the format, not a project's choice | `shipped` |
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
component declared), and the body against the type's template. Each spec type requires its own
structural section — `flow` requires `## Main flow`; `lifecycle` requires `## States and transitions`;
`process` requires `## Trigger` and `## Main flow`; `rule` and `quality` require none of their own —
plus branch anchors pointing at steps that exist and transition lines in the fixed shape. Every other
type section (`## Branches`, `## Edge cases`, `## Errors facing the actor`, `## Rule`) stays allowed but
optional; a `quality` spec allows none at all beyond the core (see
[capability-spec.md](capability-spec.md#the-quality-type)).

**`--qualifies`**, repeatable, is a `quality` spec's only type-specific flag: a functional spec's path,
optionally followed by `#anchor` (`specs/flow/upload-file.md#successful-upload`). It is validated the
same way `--components` is — against what already exists — with its own codes: `qualifies-not-found`
(the target spec, or the anchor inside it, does not exist), `qualifies-not-functional` (the target is
itself a `quality` spec), and `qualifies-not-quality` (`--qualifies` was used on a spec that is not
`quality`).

**`--serves`** and **`--depends-on`**, both repeatable, are a decision's only two frontmatter flags —
each a whole record path, never `#anchor` — and refused on a spec. `--serves` names the spec, functional
or quality, this decision exists because of; `--depends-on` names another decision this one only makes
sense while that one stays `accepted`. Both are validated against the store, with their own codes:
`serves-not-found` / `depends-on-not-found` (the target does not exist), `serves-not-spec` (the target is
not a spec), `depends-on-not-decision` (the target is not a decision), `depends-on-pending` (the target
decision is `pending`), and `depends-on-cycle` (following `depends-on` edges from this record leads back
to it, directly or through others). See
[decision-record.md](decision-record.md#relations) for what each field means.

**If anything fails, nothing is written.** The command returns the same `code` and `severity` findings
`validate` returns, so there is one vocabulary for what is wrong with a record regardless of when you
find out.

An existing file is replaced without a flag. The caller always sends complete content, so a
confirmation flag would be one the agent sets every time. If a record already sits at that path and its
body no longer matches its stamped `body-hash`, the write refuses with `body-hash-mismatch` and leaves
the existing file untouched, exactly like `record edit` below — writing over a hand-edited body would
launder it into a fresh, matching hash instead of ever reporting the tamper.

**A successful write on a spec also reports its `anchors`** — every scenario and, for a `lifecycle`,
every transition, in document order, as the slug a future reference uses (see
[capability-spec.md](capability-spec.md#anchors) for the rule):

```json
{
  "written": "specs/flow/user-signup.md",
  "warnings": [],
  "anchors": ["password-too-short", "email-already-registered"]
}
```

A decision has no anchorable part, so the field is left out rather than reported empty — the two read
differently: "this record has none" against "this kind of record never has any".

**A successful write on a decision reports its `dependents` instead, and a successful write on a spec
also reports its `served_by`** — the inverse of `depends-on` and `serves` respectively, computed fresh
from the store rather than written anywhere, so it can never go stale:

```json
{
  "written": "decisions/api/data/postgres.md",
  "warnings": [],
  "dependents": ["decisions/api/data/atlas-migrations.md", "decisions/api/data/pgx.md"]
}
```

`dependents` lists every decision that depends on this one, directly or transitively; `served_by` lists
every decision whose `serves` names this spec. Each is reported even when empty — the one that does not
apply to the kind of record just written is left out instead, the same split `anchors` already follows.
Neither blocks the write: they are informational, there so whoever just changed a decision can see what
else may need a second look, never acted on automatically.

### `record edit`

Replaces one section, addressed by its heading.

```
openrecord record edit decisions/api/security/rate-limiting.md \
  --section "## Alternatives" \
  --body-file ./new-alternatives.md
```

The binary swaps that section, revalidates the whole file, and writes only if it still holds — including
against two anchors colliding, even when the colliding section is the one left untouched.

Before any of that, it checks whether the record it is about to edit can be trusted: if the body
already on disk no longer matches its stamped `body-hash`, the edit refuses with `body-hash-mismatch`
and the file stays exactly as it was. This runs before the new section is even applied, because
recomputing `body-hash` from whatever is on disk — as the old behaviour did — would silently re-stamp a
hand-edit into a hash that matches again, which defeats the entire mechanism.

**Addressed by section, not by line.** Line numbers are the fragile way to do this: a caller working
from a stale view of the file patches the wrong place, and the result can still pass validation. Here
the sections are known and fixed, so the caller says what it means — *change the alternatives* —
instead of translating that into coordinates that may have moved.

Its successful reply carries `anchors` and `served_by` on a spec, and `dependents` on a decision, exactly
like `record write`.

---

## `validate`

Checks a coordinate is well-formed, recursively — the whole store by default, or anything under a
coordinate.

```
openrecord validate                        → the whole store
openrecord validate --for decisions/api    → api and everything below it
``` It never judges whether a record is *honest* or still true of the
code — that is reading, not linting.

This walk covers `components.json`, `actors.json`, `checks.json`, `sandbox.json` and `compliance/`
(task 14 — see [`#compliance`](#compliance)), records and indexes — everything that sits next to the
store proper. Files under `runs/` are a deliberate exception: they are working data, gitignored, and
not part of the coordinate this walk covers by default, so an old or corrupt run never blocks a
`validate` over records that have nothing to do with it. The binary still stamps and can check their
`hash` (see [decision-record.md](decision-record.md#hash)), just through the execution's own commands
rather than through `validate`.

Every run also scans the repository's text for `@record:<path>[#<anchor>]` annotations (task 10 — see
[decision-record.md](decision-record.md#annotations) for the full convention) and reports the broken
ones: a path that does not name a record, or an anchor the target record does not have. An annotation
lives outside the store coordinate this walk covers, so it is scoped differently from everything else
here — a plain `validate` reports every broken annotation in the repository, while `validate --for
decisions/api` reports only the ones whose target falls inside `decisions/api`. A scenario with no
annotation naming it at all is never reported: that is pending work, not breakage.

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
| `duplicate-anchor` | error | Two scenarios, two transitions, or a scenario and a transition, that produce the same anchor. |
| `malformed-scenario` | error | A scenario missing GIVEN/WHEN/THEN, or — on a `quality` spec — MEASURE. |
| `empty-anchor` | error | A scenario name, or a state in a transition, with no letters or digits — so it has no anchor at all. |
| `verified-in-invalid` | error | A `VERIFIED-IN` value outside `pre-release`, `production`, `manual`. |
| `verified-in-not-quality` | error | A `VERIFIED-IN` line on a scenario that is not in a `quality` spec. |
| `qualifies-not-found` | error | `qualifies` names a spec file that does not exist. |
| `qualifies-anchor-not-found` | error | `qualifies` names an anchor that is not among the target spec's anchors. |
| `qualifies-not-functional` | error | `qualifies` points at another `quality` spec instead of a functional one. |
| `qualifies-not-quality` | error | `qualifies` is declared on a spec that is not `quality`. |
| `serves-not-found` | error | `serves` names a record that does not exist. |
| `serves-not-spec` | error | `serves` names something other than a spec. |
| `depends-on-not-found` | error | `depends-on` names a record that does not exist. |
| `depends-on-not-decision` | error | `depends-on` names something other than a decision. |
| `depends-on-pending` | error | `depends-on` names a decision that is `pending`, not `accepted`. |
| `depends-on-cycle` | error | Following `depends-on` edges from this record leads back to it, directly or through others. |
| `invalid-frontmatter` | error | Frontmatter absent or malformed. |
| `body-hash-missing` | error | A record's frontmatter has no `body-hash` field. |
| `body-hash-mismatch` | error | A `body-hash` is well-formed but does not match the body — the body drifted outside the tool. |
| `json-hash-missing` | error | A JSON file the binary writes inside `.openrecord/` (`components.json` today) has no `hash` field. |
| `json-hash-mismatch` | error | A JSON file's `hash` does not match its content, or the file is not byte-for-byte what the binary writes — it was edited outside the tool. See [decision-record.md](decision-record.md#hash) for the formula. |
| `index-missing-description` | error | An `INDEX.md` with no `title` or no `description`. |
| `component-path-missing` | warning | A declared directory that is not in the repository — silent drift. |
| `unreachable-state` / `dead-end-state` | warning | A state with no way in, or no way out. A design bug nobody sees today. |
| `concern-too-flat` | warning | Many loose records in one concern: a subgroup is missing. |
| `quality-unverified` | warning | A `quality` scenario with no `VERIFIED-IN` line, and no check or sandbox covering it either — nothing says it is verified anywhere. |
| `actor-duplicate` | error | An id declared more than once in `actors.json`. |
| `actor-alias-duplicate` | error | An alias claimed by two different actors. |
| `actor-invalid-kind` | error | A `kind` outside `human`, `external-system`, `time`. |
| `initiated-by-missing` | error | A `flow` or a `process` with no `initiated-by`. |
| `initiated-by-forbidden` | error | `initiated-by` declared on `lifecycle`, `rule`, `quality` or a decision. |
| `initiated-by-duplicate` | error | The same actor listed twice in one `initiated-by`. |
| `initiated-by-unknown-actor` | error | An id in `initiated-by` not declared in `actors.json`. |
| `initiated-by-wrong-kind` | error | A non-`human` actor initiating a `flow`, or a `human` one initiating a `process`. |
| `initiated-by-not-mentioned` | error | An `initiated-by` actor never written as `@actor:<id>` in the body. |
| `actor-mention-unknown` | error | `@actor:<id>` naming an actor `actors.json` does not declare. |
| `unmarked-actor-mention` | warning | A registered actor's id (hyphens also matching a space) or alias, written in the open without `@actor:`. |
| `actors-undeclared` | error | Something needs an actor and `actors.json` does not exist yet. |
| `actors-unreadable` | error | `actors.json` exists but cannot be parsed. |
| `check-duplicate` | error | An `id` declared more than once in `checks.json`. |
| `check-without-command` | error | A check with no `command`. |
| `check-justified-by-missing` | error | A check with no `justified-by` at all. |
| `check-justified-by-not-found` | error | `justified-by` names a record that does not exist. |
| `check-justified-by-not-decision` | error | `justified-by` names a record that is not a decision. |
| `check-enforces-not-found` | error | `enforces` names a record or a coordinate that does not exist. |
| `check-enforces-empty-group` | error | `enforces` names a group coordinate with nothing filed in it. |
| `check-config-not-found` | error | A `config` file that does not exist in the repository. |
| `check-needs-not-found` | error | `needs` names a sandbox that is not declared in `sandbox.json`. |
| `checks-undeclared` | error | Something needs a check and `checks.json` does not exist yet. |
| `checks-unreadable` | error | `checks.json` exists but cannot be parsed. |
| `annotation-record-not-found` | error | An `@record:<path>` annotation (task 10) names a path that is not a record in the store. |
| `annotation-anchor-not-found` | error | An `@record:<path>#<anchor>` annotation names an anchor the target record does not have. |
| `sandbox-duplicate` | error | An `id` declared more than once in `sandbox.json`. |
| `sandbox-without-description` | error | A sandbox with no `description`. |
| `sandbox-without-setup-or-start` | error | A sandbox with neither `setup` nor `start`: nothing brings it up. |
| `sandbox-ready-missing` | error | A sandbox with neither `ready.http` nor `ready.log`. |
| `sandbox-ready-ambiguous` | error | A sandbox declaring both `ready.http` and `ready.log`. |
| `sandbox-ready-timeout-invalid` | error | A sandbox's `ready.timeout` is present but is not a positive whole number of seconds. |
| `sandbox-logs-missing` | error | A sandbox with no `logs.source`. |
| `sandbox-logs-format-invalid` | error | A sandbox's `logs.format` is present but is not one of `json`/`text`. |
| `sandbox-stop-missing` | error | A sandbox with no `stop`. |
| `sandbox-undeclared-component` | error | A sandbox's `components` names one not declared in `components.json`. |
| `sandbox-approximates-not-found` | error | `approximates` names a spec, or an anchor on it, that does not exist — or carries no `#anchor` at all. |
| `sandbox-approximates-not-quality` | error | `approximates` points at a spec that is not `quality`. |
| `sandbox-approximates-anchor-not-found` | error | `approximates` names an anchor that is not among the target quality spec's scenarios. |
| `sandbox-observes-not-found` | error | `observes` names a record that does not exist. |
| `sandbox-observes-not-decision` | error | `observes` names a record that is not a decision. |
| `sandbox-undeclared` | error | Something needs a sandbox and `sandbox.json` does not exist yet. |
| `sandbox-unreadable` | error | `sandbox.json` exists but cannot be parsed. |
| `compliance-unreadable` | error | A compliance file (task 14) exists but cannot be parsed, or declares an unknown format. |
| `compliance-event-invalid` | error | An event is not one of `implemented`, `adopted`, `verified`, `blocked`. |
| `compliance-verified-with-missing` | error | An `implemented`/`adopted`/`verified` event has no `verified-with`. |
| `compliance-verified-with-invalid` | error | A `verified-with` value is not one of `checks`, `scenarios`, `sandbox`, `audit`, or is repeated. |
| `compliance-blocked-by-missing` | error | A `blocked` event has no `blocked-by` entries. |
| `compliance-blocked-by-kind-invalid` | error | A `blocked-by` entry's `kind` is not `pending`, `contradicts`, or `missing`. |
| `compliance-blocked-by-record-missing` | error | A `pending`/`contradicts` entry has no `record`, or a `missing` entry names one. |
| `compliance-blocked-by-note-missing` | error | A `missing` entry has no `note`. |
| `compliance-blocked-by-not-found` | error | A `pending`/`contradicts` `blocked-by` names a record that does not exist. |
| `compliance-orphan` | warning | A compliance file whose record no longer exists in the store. |

### Exit code

Non-zero when any finding is an `error`. The classification is for the agent; the exit code is for the
pipeline, and the two do not get in each other's way — `validate` works as a CI gate and a pre-commit
hook without a second mode.

---

## `impact`

`impact` has two modes, taking different input and answering different questions, over the same engine:

- **Record mode** (`openrecord impact <record>`) — before changing or adding a record, what else is
  connected to it: what initiates it, what qualifies it, what serves and depends on it, what verifies
  it, and how covered it is.
- **Files mode** (`openrecord impact --files <file...>`) — before changing code, which decisions and
  specs govern the file(s) you are about to touch, and how they are checked.

Both follow the links the store already declared and validated — `components`, `actors`, `qualifies`,
`serves`, `depends-on`, `enforces`, `needs`, `@record:` annotations, `approximates`, `VERIFIED-IN` —
never by searching text. What either mode returns is complete because it **follows relations**, not
because a search happened to match. Neither mode runs anything: reading checks off the store and
actually executing them is a separate concern, for a command of its own.

### Record mode

```
openrecord impact decisions/api/data/postgres.md
```

```json
{
  "record": {
    "path": "decisions/api/data/postgres.md",
    "status": "accepted",
    "description": "Account data is stored in Postgres."
  },
  "serves": [],
  "depends_on": [],
  "dependents": [
    {
      "path": "decisions/api/data/pgx.md",
      "status": "accepted",
      "description": "Database access goes through pgx, never database/sql.",
      "dependents": [
        {
          "path": "decisions/api/data/atlas-migrations.md",
          "status": "accepted",
          "description": "Every schema change is an Atlas migration, applied in order.",
          "dependents": []
        }
      ]
    }
  ],
  "verified_by": [],
  "annotated_in": []
}
```

### Every record listed carries only `path`, `status` and `description`

Never the body. The consumers (`define`, changing a decision, `implement`) are an agent that has to
choose what to open next — the `description` is what lets it skip opening everything, and `status`
says what is `pending`: undecided, so it cannot be contradicted or broken, because nobody settled it
yet.

### Validate runs first, over the whole store

A neighbourhood built on top of a store with an error anywhere is not trustworthy — the error may well
be a decision this very record is served by. So **both modes** run `check.Store` (the same engine
`validate` runs) over the **whole** store before building anything, not only the record's or the
files' own corner of it:

| Situation | What `impact` does |
| --- | --- |
| Record mode: the argument does not name a record in the store | Usage error (`record-not-found`), exit 2 |
| Either mode: `validate` finds an `error` anywhere in the store | Refuses: exit 1, the same `findings` shape `validate` emits, no neighbourhood |
| Either mode: `validate` finds only `warning`s (`quality-unverified`, `unmarked-actor-mention`) | Does not block: the neighbourhood builds normally |
| Either mode: `checks.json` / `sandbox.json` / `actors.json` does not exist | A normal state, not an error — the corresponding section is just empty |

A refused run reports only the raw argument(s) and the findings, never a partial neighbourhood — record
mode under `"record"`, files mode under `"files"`:

```json
{
  "record": "specs/flow/sign-up.md",
  "findings": [
    { "code": "body-hash-mismatch", "severity": "error",
      "path": "decisions/api/data/pgx.md", "message": "..." }
  ]
}
```

### Depth: one hop, except `depends-on`

Every link stops at one hop — `qualified_by`, `qualifies`, `served_by`/`serves`, `verified_by`,
`annotated_in`, `scenarios` — because a second hop already drags in a large, unfocused slice of the
store. The one exception is `depends-on`, followed as a **full chain**, in **both directions**, in
separate fields: `depends_on` (what the record rests on, upward) and `dependents` (what rests on the
record, downward — what has to be re-evaluated if it changes). `validate` forbids a cycle in
`depends-on`, so the chain always terminates. Inside `served_by`, each serving decision carries its own
`depends_on`/`dependents` chains — the one place a chain nests inside something other than the record
`impact` was asked about.

### Sections, by record type (record mode)

Every section has the same name and shape everywhere it appears; one that does not apply to the
record's type is **omitted entirely** (the key is absent), never reported empty — an applicable section
with nothing in it is an empty array or object instead.

| Record | Sections |
| --- | --- |
| Functional spec (`flow`, `rule`, `lifecycle`, `process`) | `initiated_by` (only `flow`/`process`) · `qualified_by` (each entry carrying its own `verification`) · `served_by` (+ `depends_on`/`dependents` of each serving decision) · `verified_by` · `annotated_in` · `scenarios` |
| `quality` spec | `qualifies` · `served_by` (+ chains) · `verified_by` · `annotated_in` · `scenarios` · `verification` (its own scenarios) |
| Decision | `serves` · `depends_on` · `dependents` · `verified_by` · `annotated_in` |

**`initiated_by`** is every actor in `initiated-by`, resolved against `actors.json` — `{id, kind,
description}`. A `lifecycle`, a `rule`, a `quality` spec and a decision never declare `initiated-by`, so
the key is absent on them.

**`qualified_by`** is every `quality` spec whose `qualifies` names this functional spec — the inverse of
`qualifies`, computed fresh rather than written anywhere — each paired with which of this spec's anchors
it qualifies (an empty list means the whole spec) and, since a functional spec reports no `verification`
of its own, that quality spec's own `verification`, anchor by anchor — see below.

**`qualifies`**, on a `quality` spec, is its own declared targets: the functional spec(s) it
characterizes, each with the one anchor it names, when it names one.

**`served_by`** is every decision whose `serves` names this spec — functional or `quality` alike — each
carrying its own `depends_on`/`dependents` chains.

**`serves`/`depends_on`/`dependents`**, on a decision, are its own three relational fields: the spec(s)
it exists because of, and the full `depends-on` chain in both directions.

**`verified_by`** is every check from `checks.json` whose `enforces` covers this record — whole file, or
a group coordinate containing it — as `{id, command}` plus `needs` (a sandbox id) when the check
declares one. The sandbox itself is never expanded; `needs` only names it.

**`annotated_in`** is every `@record:` annotation naming this record whole, with no `#anchor` — the only
kind a decision or an anchor-less spec can have — from the same engine `validate`'s own annotation scan
uses:

```json
"annotated_in": ["tests/signup.spec.ts:1"]
```

**`scenarios`** (present only on a record with at least one anchor) is a map from each anchor to every
`@record:#anchor` annotation naming it, `[]` meaning no annotated test yet:

```json
"scenarios": {
  "account-is-created": ["tests/signup.spec.ts:2"]
}
```

An annotation only **declares** what a test or a rule intends to verify — it is never evidence that the
test passed, or even that it still exists.

**`verification`** is a map from each of a `quality` spec's own scenario anchors to its verification
state: `{checks, sandboxes, verified_in}` — `checks` is every check (full `{id, command, needs?}`
objects) enforcing that `quality` spec whole, `sandboxes` is every sandbox whose `approximates` names
that exact anchor, and `verified_in` is the scenario's own `VERIFIED-IN` value, when it declares one.
All three empty or absent is the same state `quality-unverified` warns about: nobody has said how the
scenario is verified at all. On a `quality` spec itself it is a top-level section; on a functional spec
it is nested inside each `qualified_by` entry instead, because it only ever makes sense attached to the
quality spec it describes:

```json
"qualified_by": [
  { "path": "specs/quality/sign-up-speed.md", "status": "accepted",
    "description": "Sign-up stays fast under load.", "qualifies": [],
    "verification": {
      "fast-enough": { "checks": [], "sandboxes": ["load-sandbox"] }
    } }
]
```

### Files mode

```
openrecord impact --files src/api/handlers/upload.go src/web/upload/form.tsx
```

`--files` takes one or more files: every argument after it, up to the next flag, is its own file —
`--files a.go b.go c.go`, not just the first. Repeating the flag (`--files a.go --files b.go`) or
comma-separating one occurrence (`--files a.go,b.go`) work the same way; pick whichever reads better.

```json
{
  "files": {
    "src/api/handlers/upload.go": ["api", "root"],
    "src/web/upload/form.tsx": ["root", "web"]
  },
  "decisions": {
    "root": [
      { "path": "decisions/root/structure/go.md", "status": "accepted",
        "description": "The service is written in Go.",
        "verified_by": [ { "id": "lint-go", "command": "golangci-lint run {files}" } ],
        "annotated_in": [] }
    ]
  },
  "specs": [
    { "path": "specs/flow/upload-file.md", "status": "accepted",
      "description": "A visitor uploads a file through the API and sees it in the web UI.",
      "components": ["api", "web"],
      "verified_by": [ { "id": "e2e-upload", "command": "go test ./e2e/upload/..." } ],
      "annotated_in": [],
      "scenarios": {
        "upload-succeeds": ["tests/upload.spec.ts:1"],
        "format-not-allowed": []
      } }
  ]
}
```

**`files`** maps every requested path — normalised the same way `component owners` normalises one — to
every component whose declared path **contains** it, not only the most specific: a component declared
as `.` (root) covers every path by the same mechanism a narrower one covers its own, so both appear at
once rather than one hiding the other. `[]` means the path is outside every declared component, never
an error; a path that does not exist on disk yet still resolves, because resolution is by path, not by
what is already written.

**`decisions`** groups every decision filed under a component that owns at least one requested file, by
that component's id — a decision belongs to exactly one component, the folder it is filed under, so
there is nothing to deduplicate across keys. Each decision carries `path`, `status`, `description`,
`verified_by` and `annotated_in` — never `depends_on`/`dependents`/`serves`, which are relations between
decisions, not what a block of files has to respect; ask record mode for those.

**`specs`** is every spec naming at least one of those components, plus — one hop further — every
`quality` spec whose `qualifies` names a spec already included that way, even when that `quality` spec's
own `components` names none of them: a quality spec is rarely filed under the same component as what it
qualifies, and the hop is what keeps it from being missed. Each spec appears exactly once, with its own
`components`, `verified_by`, `annotated_in` and (when it has anchors) `scenarios`; a `quality` spec also
carries `qualifies` (its own declared targets, as written) and `verification` (its own scenarios' states,
the same shape record mode nests inside `qualified_by`).

Nothing is repeated except a check's own object, when the same check verifies more than one of the
decisions or specs returned.

### `--only`: trimming the sections returned

`--only <names>` (comma-separated) keeps just the sections named, in both modes. The identity key
(`record` or `files`) always stays, whether or not `--only` names it — it is never something a caller
opts into. A name that is not a valid section for that mode is a usage error, not a silent no-op:

```
openrecord impact specs/flow/sign-up.md --only verified_by,annotated_in
openrecord impact --files src/api/handlers/upload.go --only specs
```

### Which mode to use

Reach for **record mode** when you have a record in hand and need its neighbourhood — writing or
reviewing a decision or a spec, checking what a status change would ripple into. Reach for **files
mode** when you have code in hand and need to know what governs it before you touch it — the walk
`openrecord-consult` runs before an edit. Neither mode runs a check, completes a `{files}` placeholder,
or starts a sandbox; reading what applies and actually verifying it are different jobs, and running
checks belongs to a command of its own.

### What `impact` never follows

Only **declared, validated** links — never "same folder" proximity. Two decisions filed side by side in
the same concern are not a relation, and judging whether they conflict is semantic work for an agent
(`openrecord-reconcile`, which already gets them from `openrecord map --for <folder>`), not something a
deterministic command can decide.

### Exit codes

| Exit | Meaning |
| --- | --- |
| `0` | The neighbourhood was built — possibly with warnings earlier in `validate`'s own pass. |
| `1` | `validate` found an `error` somewhere in the store, or the arguments were invalid in some other way (a bad `--only` name, `--files` combined with a record path, or neither given); the findings or the usage error are reported, no neighbourhood. |
| `2` | Record mode only: the argument does not name a record in the store at all — a usage error, same as an unknown command. |

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

`--for` is **required** — there is no unscoped search. A bare `decisions` coordinate — the decisions
root, with no component named — is rejected too: a decision governs exactly one component, so `grep`
needs one named. `openrecord map --for decisions` lists the declared components.

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

## `search`

The command that actually finds a record: every record under scope is scored against a context by Jev,
a ranking model, and a fixed rule decides what gets served. The result is stored, so it can be reviewed
afterward.

```
openrecord search --for decisions/api \
  --literal "rate limit" \
  --semantic "rate limiting policy" \
  --context "adding a per-user rate limit to the signup endpoint"

openrecord search --for decisions/api --for decisions/cli \
  --literal "rate limit" --semantic "rate limiting" --context "..." \
  --omit decisions/api/security/rate-limits/at-the-gateway.md
```

`--for` is **required**, and repeatable — scope is the union of every value given, and there is no
unscoped search. A bare `decisions` coordinate is rejected too, for the same reason `grep` rejects it: a
decision governs exactly one component, and `openrecord map --for decisions` is what lists the ones
declared. `--literal` is **required and repeatable**, 1 to 10 non-blank terms, never split on commas.
`--semantic` and `--context` are each **required**, one value. `--omit` may be repeated, each occurrence
naming one scoped path exactly; a path containing a comma is kept whole, never split. No positional
argument is accepted.

**`qmd` and `OPENROUTER_API_KEY` are both required.** There is no degraded mode: a missing key, an
absent or unusable `qmd`, a project that has never run `openrecord qmd index` (or one missing a
collection this scope needs), or a failing Jev request each fail the whole command before anything is
served or stored, naming the fix (`openrecord qmd install`, `openrecord qmd index`,
`OPENROUTER_API_KEY`, or `openrecord jev status`). The not-indexed case fails before a Jev request is
ever built — Jev sees nothing either way.

### The serving rule

Scope is every record under the union of `--for`, indexes excluded, minus `--omit`. Jev scores each one,
from its title, description and `--context` — record bodies are never sent. The served set is the union
of: the top 3 by score, every record scoring above 0.2, every record with a literal hit, and every
record with a semantic hit. `records` is ordered by score descending, ties by path ascending.
`discarded` is every other scoped record, in path order — never gone, just not ranked highly and not
otherwise hit.

```json
{
  "id": "20261004T153012Z-a1b2c3",
  "model": "typesafe/jev-1.13-20260917",
  "records": [
    {
      "path": "decisions/api/security/rate-limits/at-the-gateway.md",
      "literal": { "terms": ["rate limit"], "line": 12, "text": "...", "hits": 6 }
    },
    { "path": "decisions/api/security/throttling.md" }
  ],
  "discarded": ["decisions/api/data/queries.md"],
  "omitted": 0
}
```

A record carries `literal` and/or `semantic` only when that pass hit it directly; a record served on
ranking alone carries neither, and no score ever appears in the output. `literal.hits` is a real count
of matching lines; a `semantic` hit's `line`/`text` come from qmd's own snippet.

### Every search is stored

Each successful search creates one stored search under `.openrecord/.searches/`, addressable by its
`id`: the inputs, the model, every served record's full content at search time, the discarded paths, and
timestamps. It is git-ignored, invisible to `validate`/`map`/`grep`/every search scope, and persists
until a person deletes it. See `review` below for working through it.

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
openrecord skills --emit .claude/skills/ --dry-run
openrecord skills --emit .claude/skills/ --guard
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

### One variant

`search` requires qmd now, so every bundled skill — `openrecord-setup-search` included — applies to
every project unconditionally. There is no flag to choose a smaller variant, and nothing is stripped: an
emitted skill is byte-identical to its embedded source.

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

### `--guard`: writes to `.openrecord/` stay behind the binary

Everything inside `.openrecord/` is only trustworthy because the binary validates before it writes and
refuses to write at all when something fails. If anyone — person or agent — can edit those files
directly with an editor, that guarantee stops meaning anything. `--guard` installs a mechanical barrier
against that, **for Claude Code specifically** — the only agent where every layer this needs is
documented. Two actors, two different defences:

| Against | Mechanism | What it guarantees |
| --- | --- | --- |
| An AI agent (Claude Code) | Permissions + process sandbox + a `PreToolUse` hook | A real block, enforced by the operating system |
| A person editing by hand | `body-hash` (records) and `hash` (JSON files, e.g. `components.json`), checked by `validate` | Detection after the fact, never a block |

Both are necessary, and neither substitutes for the other: a person with direct filesystem access can
always open a file in an editor, but `validate` (in a pre-commit hook or in CI) will notice. An agent,
by contrast, can genuinely be stopped before it writes at all.

`--guard` merges three layers into the project's `.claude/settings.json`:

1. **`permissions.deny`** — `Edit(/.openrecord/**)` and `Edit(/.claude/**)`. An `Edit` rule already covers
   every built-in tool that edits a file by path (`Write` and `NotebookEdit` included), and no permission
   mode skips a deny rule. The leading `/` anchors each pattern at the project root regardless of the
   session's current working directory — a bare `.openrecord/**` would instead anchor at wherever the
   session happened to start, which is not always the project root (see [Claude Code's permissions
   reference](https://code.claude.com/docs/en/permissions#read-and-edit)).
2. **`sandbox.*`** — the process sandbox, which blocks a write at the operating-system level, including
   from a subprocess a hand-written script launches:
   - `sandbox.enabled: true`, `sandbox.failIfUnavailable: true` — if the sandbox cannot start (a missing
     dependency, an unsupported platform), Claude Code refuses to start with this configuration rather
     than silently running unguarded.
   - `sandbox.allowUnsandboxedCommands: false` — the agent cannot ask to retry a blocked command outside
     the sandbox.
   - `sandbox.filesystem.denyWrite: [".openrecord/**", ".claude/**"]` — every write under the store, and
     under the agent's own configuration, is denied.
   - `sandbox.excludedCommands: ["openrecord *"]` — the one command allowed to write there: the binary
     with its arguments, never a wider pattern. Entries use the `Bash(...)` rule syntax, where a pattern
     without a wildcard is an exact match, so bare `"openrecord"` would only cover the binary run with no
     arguments and every real write would fail. A call leaves the sandbox only when every command in it
     matches, and a redirect, a `cd` or a `$(...)` keeps it sandboxed, so `openrecord version; python3
     ...` cannot ride along. This entry is what lets the real binary still write both `.openrecord/**`
     and `.claude/**` (e.g. `skills --emit ... --guard` writing `.claude/settings.json` itself).
   - `permissions.allow: ["Bash(openrecord *)"]` — an excluded command goes through the regular
     permission flow, so without this every `openrecord` call would ask for confirmation.
3. **`hooks.PreToolUse`** — a hook entry that calls `openrecord hook check` (below) for every tool call.
   It is not the barrier — layers 1 and 2 already are — it only gives the agent (and whoever reads the
   transcript afterward) the matching `openrecord` command instead of a bare rejection.

The merge **preserves whatever is already in `.claude/settings.json`**: an unrelated top-level key, an
unrelated entry already sitting in an array this touches. Running the emit twice produces byte-identical
output — every merge only ever appends a missing entry in a fixed position, never reorders what was
already there. The report lists exactly what was added (`created`), already present (`unchanged`), or
corrected against a conflicting value (`updated`) — the three structural `sandbox` booleans above are
pinned to the values this protection needs even if the project's settings disagreed.

**`git` is not excluded from the sandbox.** Restoring a file or switching a branch that touches
`.openrecord/` is a human action, not the agent's: `git checkout -- .openrecord/components.json` run by
the agent itself is blocked by the same sandbox rule as anything else, by design. A person runs that
command themselves, outside the agent's own tool calls.

**Linux and WSL2 need `bubblewrap` and `socat` installed** (`sudo apt install bubblewrap socat`, or your
distribution's equivalent) for the sandbox to start at all. With `sandbox.failIfUnavailable: true`,
Claude Code refuses to start with this configuration if either is missing, rather than running without
the sandbox layer silently. macOS needs neither — it uses the built-in Seatbelt framework.

Two bits of what the original task sketched for this guard turned out to use different Claude Code key
names than it assumed, verified against the current documentation (`https://code.claude.com/docs/en/*`,
checked 2026-10-13): `sandbox.filesystem.denyWrite`, `sandbox.excludedCommands`,
`sandbox.allowUnsandboxedCommands` and `sandbox.failIfUnavailable` were already exactly right. The one
correction is the `permissions.deny` path form — `/.openrecord/**` (one leading slash, anchored at the
project root) rather than a bare `.openrecord/**` (anchored at the session's current directory instead).

Two blocks the original task described as part of this guard are **out of scope here**: blocking an edit
to a check's own configuration file, and blocking `record write`/`record edit` while an implementation run
is in progress. `checks.json` exists now and names which files are a check's configuration (`cli.md#check`),
but knowing whether either block should actually fire still depends on a run being active, which depends
on a run's `run.json` — tracked by issue 15, and not required for this guard to be useful on its own.

---

## `hook`

```
openrecord hook check
```

The command behind the `PreToolUse` hook `--guard` installs. It reads one Claude Code tool call as JSON
on stdin and prints Claude Code's own decision JSON on stdout — never the barrier itself (the permissions
and sandbox layers above are), only the message: which `openrecord` command should have been used
instead.

**Input** is whatever Claude Code's `PreToolUse` hook sends on stdin; this command reads `tool_name` and
`tool_input` and ignores the rest (`session_id`, `cwd`, `transcript_path`...).

**Output**, on exit 0, is the current PreToolUse decision shape:

```json
{
  "hookSpecificOutput": {
    "hookEventName": "PreToolUse",
    "permissionDecision": "deny",
    "permissionDecisionReason": "`.openrecord/components.json` is written only by `openrecord component add` / `openrecord component remove`."
  }
}
```

or, allowing:

```json
{ "hookSpecificOutput": { "hookEventName": "PreToolUse", "permissionDecision": "allow" } }
```

**Decisions:**

| Tool call | Decision | Points at |
| --- | --- | --- |
| `Edit`/`Write`/`NotebookEdit` on `.openrecord/components.json` | deny | `component add` / `component remove` |
| `Edit`/`Write`/`NotebookEdit` on `.openrecord/actors.json` | deny | `actor add` / `actor edit` / `actor remove` |
| `Edit`/`Write`/`NotebookEdit` on `.openrecord/checks.json` | deny | `check add` / `check remove` |
| `Edit`/`Write`/`NotebookEdit` on `.openrecord/sandbox.json` | deny | `sandbox add` / `sandbox edit` / `sandbox remove` |
| `Edit`/`Write`/`NotebookEdit` on a record under `decisions/` or `specs/` | deny | `record write` / `record edit` |
| `Edit`/`Write`/`NotebookEdit` on an `INDEX.md` | deny | `level add` (or `component add` for a component's own index) |
| `Edit`/`Write`/`NotebookEdit` on anything else under `.openrecord/` | deny | the matching command, generically |
| `Edit`/`Write`/`NotebookEdit` under `.claude/` | deny | `skills --emit ... --guard` re-installs it; it is never hand-edited |
| `Edit`/`Write`/`NotebookEdit` anywhere else | allow | — |
| `Read`/`Glob`/`Grep`, or any tool this build does not recognize | allow | — |
| `Bash` whose command text looks like a direct write into `.openrecord/` or `.claude/` (a redirect, `tee`, `sed -i`, `cp`/`mv`/`rm`, `git checkout`/`git restore`) | deny, with an explanation | — |
| `Bash` that never mentions `.openrecord/` or `.claude/` at all, anywhere in the command text | allow | — |

**The `Bash` check is a best-effort message, not a security boundary.** Unlike the path-based tools
above, there is no sandboxed-by-construction signal this command can see into for an arbitrary shell
command — the process sandbox is what actually blocks the write, regardless of what this command says.
The pattern match exists so a blocked (or merely probing) agent reads an explanation instead of silence,
and it does not try to be airtight against a command deliberately written to avoid it.

**The scan always runs over the full command text, not just its first word.** There is no "starts with
`openrecord`, so allow the rest" shortcut: a chained command (`openrecord status && sed -i ...
.openrecord/components.json`, joined with `&&`, `||`, `;`, a pipe, or command substitution) puts a
harmless `openrecord` call first and the actual write second, and only scanning the whole string catches
it. `openrecord status && cat foo` still allows, since nothing in that string matches a deny pattern.

**Fails open on anything it does not recognize** — an unrecognized tool name, an unrecognized
`tool_input` shape, `Read`/`Glob`/`Grep` — because the one failure mode this command must never cause is
blocking an unrelated, legitimate tool call. The one case that fails **loud**, not open, is stdin that
does not parse as JSON at all: that is this command being miscalled, not a tool call to judge, so it
exits non-zero with a clear error rather than silently allowing or crashing.

---

## `concerns`

```
openrecord concerns
openrecord concerns --json
```

Prints the catalogue — ten concerns and, under each, the places projects usually have a decision
worth recording.

**This is how the catalogue is reached, and the only way.** It ships inside the binary and never enters
a project, so a skill or an instruction that named a file path would be pointing at something that does
not exist where it is read. Everything a consumer needs comes through a command.

The document is printed **verbatim** from the embedded copy rather than rebuilt from parsed entries:
the document is the deliverable, and anything that reassembled it would drift from what ships.

`--json` returns the concern names with their descriptions, for a caller that wants to pick one rather
than read. The topics stay out of it — they name nothing and exist to be read.

---

## `verification`

```
openrecord verification
openrecord verification KEY
```

Prints the verification catalogue — the other document of this kind, alongside `concerns.md`. Where
that one is a tunnel of questions for *what was decided*, this one is a tunnel of questions for *how
would anyone know the code still does it*: for every functional spec aspect, every `quality` attribute,
and every `concerns.md` topic, what the record has to say to be verifiable, a typical mechanism, where
that kind of check lands, and its class — `full-check`, `partial-check+audit`, or `manual-only`.

Like the concerns catalogue, **it ships inside the binary and never enters a project**, and it names no
tool to pick: every tool mentioned in it is an example, never a choice.

With no argument, the whole document is printed **verbatim** from the embedded copy, the same reasoning
`concerns` follows: the document is the deliverable, and rebuilding it from parsed entries risks
drifting from what actually ships.

With a `KEY`, only that entry is printed — again verbatim, sliced out of the document rather than
reassembled. A key is `<spec-type>/<aspect>` for a functional spec (`flow/main-flow`,
`lifecycle/transitions`...), a bare attribute for a `quality` spec (`latency`, `accessibility`...), or
`<concern>/<topic>` for a decision (`security/rate-limiting`...) — the same topic names `concerns.md`
already uses. An unknown key is a usage error that lists the valid keys of the gate it looks closest to,
or every key if none looks close.

---

## `.openrecord/.env`

Every command merges one project-scoped environment before it runs: `.openrecord/.env`, under the
process environment, under a small set of paths openrecord pins itself and that neither layer can
move. Nothing here is openrecord's own configuration — it is how `qmd` and Jev are configured per
project, read once per invocation and merged the same way every time.

### Grammar

```
# a comment, and the blank line above are both ignored
A="x y"
B='z'
C=$HOME
```

`KEY=VALUE`, one per line. A key matches `[A-Za-z_][A-Za-z0-9_]*`; a `#`-prefixed or blank line is
skipped. One matching `'…'`/`"…"` pair is stripped from the value — nothing else is: there is no
expansion and no `export`, so `C=$HOME` above is the four literal characters `$HOME`, not the shell
variable. A missing file, or a missing `.openrecord/` entirely, counts as empty, never an error.

Any other line — including `export K=v`, or a key declared twice — fails the command that read it: a
`usage` finding naming the file and the exact line, exit code 2, before that command does anything
else.

### Precedence

Highest first: the paths openrecord pins itself (`QMD_CONFIG_DIR`, `INDEX_PATH` — see `qmd` below,
never overridable), then the process environment, then `.env`. Each key reaches a subprocess exactly
once, at whichever layer governs it — `.env` can set a model or a credential, but neither it nor the
process environment can ever move where this project's own index lives.

### Where a value came from

`qmd status` and `jev status` report, per key, whether it currently comes from `env`, `dotenv`, or is
`unset` — never the value itself. A pinned path is never reported this way: it is never something the
project or its environment chose.

```
openrecord qmd status
→ "sources": {"QMD_EMBED_MODEL": "dotenv", "QMD_OPENAI_API_KEY": "env", "QMD_RERANK_URL": "unset", ...}
```

### Never committed

`.openrecord/.gitignore` gains a `/.env` line the first time any command runs against a declared store
(one whose components file already exists) — a hand-made `.env`, including one left by an older
openrecord, is protected from that point on. `.openrecord/.qmd/` ignores itself completely the moment
it is created. Neither ever shows up in `git status`, with or without `--untracked-files=all`; every
other file under `.openrecord/` still does.

---

## `qmd`

Semantic search is a separate project and openrecord does not own it. These two subcommands are how
openrecord reports on it and installs it; `search` (above) is what actually executes it.

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

**`index`** registers and embeds this project's own collections — one per declared component plus
specs — into a qmd index this project owns outright, under `.openrecord/.qmd/`, never the global one. A
re-run is incremental: it only adds a collection that is not registered yet, and leaves an existing one
and its pinned model untouched. `--rebuild` deletes `.openrecord/.qmd/` first and rebuilds every
collection from scratch, which is the supported way to pick up a different embedding model.

```
openrecord qmd index
openrecord qmd index --rebuild
```

`search` requires a project that has already run this at least once; without it, `search` fails naming
`openrecord qmd index` rather than silently searching nothing.

### Why a pinned release and not the latest one

The install URL names one release's asset — `.../download/v<version>/tobilu-qmd-<version>.tgz` — rather
than `releases/latest/download/`, which would install whatever qmd published most recently. Following
latest asks four things of qmd, and three of them are already true: an asset whose name does not carry
the version, so the URL survives a release (the fork publishes `qmd.tgz` next to the versioned one); a
release GitHub itself counts as latest, which one marked prerelease is not, and a `-mate.N` suffix makes
that easy to get wrong; and a tarball npm installs without running a build, which is why the URL points
at a tarball and not at the git repository — npm runs a git dependency's `prepare`, and the build dies
where there is no compiler.

The fourth is the one not on offer: a stable contract. Unpinned, every release reaches every machine
without anyone reading it first, and openrecord depends on `status`, `--version` and the collection
commands keeping their shape. The pin is also what `status` compares against — `pinned_version` is a
claim about which qmd this openrecord was built for, and there is no such claim left to make if the
answer is always whichever one is newest.

---

## `jev`

Jev is the model `search` runs at query time to rank every scoped record against the task. This
subcommand only reports on it — `search` is what actually calls it.

```
openrecord jev status
```

**`status`** reports whether `OPENROUTER_API_KEY` is set (never its value), whether the endpoint
answered a minimal request, and the pinned model. Without a key it sends no request at all — there is
no request a missing key could plausibly send. It exits non-zero unless the key is present and the
endpoint is reachable.

---

## `review`

A `search` stores what it served and discarded. `review` is how that stored search gets worked through:
opening a record, giving it a verdict, and checking whether every served record has one yet.

```
openrecord review open ID PATH
openrecord review mark ID PATH --verdict governs|contradicts|unrelated
openrecord review status ID
```

**`open`** prints one record from the stored search and marks it opened. A served record prints the
snapshot taken at search time; a discarded record prints the current file instead and stays discarded —
opening it never promotes it into the served list.

**`mark`** stores a verdict for a record that was opened earlier. Marking a record that was never opened
fails rather than silently opening it first, so a reviewer cannot record a verdict for something they
never actually read. The mark that gives the last served record a verdict is what completes the search;
marking a discarded record never does.

**`status`** is read-only: it reports which served (and reviewed-discarded) records have a verdict, and
whether the search is complete. It exits non-zero while any served record still lacks one.

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
