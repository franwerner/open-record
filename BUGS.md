# BUGS — what to resolve

Derived from the end-to-end run recorded in [E2E-FINDINGS.md](E2E-FINDINGS.md). Every item was
reproduced against `openrecord v0.1.0` (commit `847a465`); the finding id in brackets points at the
section of that file with the full command and output.

Ordered by severity within each group. Each item names the fix **and** the test that has to exist for
it, because most of these survived precisely because the existing test asserts something weaker than
the property that matters.

---

## A. Wrong output — the tool produces something that is silently incorrect

- [x] **`diagram` emits invalid Mermaid for any multi-word state** `[D-1]`
      `internal/cli/diagram.go:150` formats `state \"%s\" as %s` with `mermaidText(name)`, which already
      returns a quoted string — so the output is `state ""written by us"" as written_by_us`, which
      Mermaid rejects. The same double-wrap makes transition labels render their quotes literally
      (`--> x: "trigger"`).
      *Fix:* pass the raw escaped text, not `mermaidText`, into the `state ... as ...` line and into the
      label, or make `mermaidText` return unquoted content and quote at each call site — one or the
      other, not both.
      *Why it escaped:* every state in `TestDiagramRendersEachType` is a single word, so `mermaidID`
      returns the name unchanged, `id != name` is false, and the buggy line never executes.

- [x] **`diagram` silently truncates every wrapped step and branch** `[D-2]`
      `diagramSections` (`internal/cli/diagram.go:73`) accumulates lines one by one and `diagramStep` /
      `diagramBranch` are single-line regexes, so a continuation line is dropped with no marker. A
      branch renders as `B4["it is deleted, and"]`.
      *Fix:* join a step or branch with its continuation lines (a following line that is indented and
      does not itself start a new step/branch/heading) before matching.
      *Why it matters here:* every markdown file in this repository wraps at ~100 columns, so a spec
      written in the house style produces a diagram that lies. `validate` cannot catch it — a diagram is
      a view and is never checked.

- [x] **Branch node ids are indexed by line, not by branch** `[D-2, cosmetic]`
      `internal/cli/diagram.go:130` uses the loop index over *lines*, producing `B1, B2, B4, B6, B8`.
      Harmless to Mermaid, confusing to a reader diffing two renders.

---

## B. False green — a check reports clean when it has not checked

- [x] **`validate --for` accepts a coordinate that does not exist** `[E-1]`
      ```
      $ openrecord validate --for decisions/nope
      {"for":"decisions/nope","findings":[]}          # exit 0
      $ openrecord map --for decisions/nope
      {"code":"invalid-coordinate", ...}               # exit 1
      ```
      Same coordinate, two answers. A validation scoped to a mistyped or renamed component returns the
      exact shape a CI step or an agent treats as "the store is fine". `validate` already rejects a
      coordinate that is not a store at all, so the strictness exists — it stops one level too early.
      *Fix:* resolve the coordinate to a directory before walking, and fail with `invalid-coordinate`
      the way `map` does.

- [x] **`map` advertises a group that `map` itself rejects** `[C-3]`
      `map --for specs` lists all four fixed spec types including ones with no directory, giving them a
      lowercase title and an **empty description**, marked `kind: group` — the value that means
      "descend". Following it errors:
      ```
      $ openrecord map --for specs/process
      {"code":"invalid-coordinate","message":"specs/process does not exist"}    # exit 1
      ```
      This breaks the contract `openrecord-consult` is built on: *"Each level returns the `title` and
      `description` ... you never open an index yourself."* With an empty description there is nothing
      to decide on.
      *Fix:* pick one and hold it — either a synthesised type is navigable (`map` returns an empty
      listing for it) or it is not listed as a group. If it stays listed, it needs a description, which
      means shipping the four type descriptions with the binary — see item C-1 below, they are the same
      fix.

- [x] **`qmd status` reports a qmd that cannot open its database as healthy** `[I-2]`
      `qmd.Check()` (`internal/qmd/qmd.go:41`) does `LookPath` + `--version`, and `--version` is the one
      command that does not touch the database. A qmd with broken native bindings answers it and fails
      everything else, and openrecord says `"installed": true`.
      *Fix:* probe with a command that opens the store (`qmd status`) and report a third state —
      present but not usable — rather than folding it into `installed: true`.

- [x] **`qmd install` cannot repair or upgrade** `[I-3]`
      Short-circuits on the same presence check, never compares the found version against the pinned
      `InstallSource`, and has no force flag. From a broken or outdated qmd there is no path forward
      through openrecord's own commands.
      *Fix:* compare against the pinned version, install when it is older or unusable, and add
      `--force`. Warn when the resulting binary would still be shadowed by an earlier `PATH` entry.

- [x] **The installer's `WITH_QMD=yes` is satisfied by any `qmd` on `PATH`** `[I-1]`
      `scripts/install.sh` `install_qmd()` tests `command -v qmd` and prints `qmd is already installed`.
      A user who asked for qmd is entitled to the version openrecord pins; here they can silently get
      neither that version nor a working one.
      *Fix:* same check as above, shared with the binary so the two cannot drift.

---

## C. Machine contract — JSON and help that a caller cannot rely on

- [x] **`record write --help` omits `--components`** `[B-4]`
      It is mandatory for every spec and the write fails without it, but neither the usage line nor
      `openrecord-bootstrap` mentions it. `--dry-run` is likewise absent from `skills --help`.
      *Fix:* print the flag set under `Usage:` for commands that have one.

- [x] **A rejected `record edit` reports itself under `"written"`** `[CAP-3]`
      Success returns `{"edited": ..., "section": ...}`; failure returns `{"written": null, "path": ...,
      "findings": [...]}`. A caller keying on `edited` sees neither success nor an explicit failure.
      *Fix:* have `writeRejected` take the verb, or emit both keys with the inapplicable one null.

- [x] **`grep` returns one match per line, not per record** `[C-4]`
      13 matches over 5 records for a common term. `openrecord-consult` says the steps *"produce
      candidates, deduplicated by path"* — nothing does that deduplication and nothing says the caller
      must.
      *Fix:* either group matches by path in the report, or state in the skill that the caller
      deduplicates.

- [x] **`level add` refuses to default the four fixed spec types** `[B-2]`
      ```
      $ openrecord level add specs/flow
      {"code":"usage","message":"\"flow\" is not in the catalogue, so level add needs --title and
       --description; the catalogue has contracts, data, data-lifecycle, ..."}
      ```
      The message lists the *decision* concerns as if `flow` were a failed attempt at one, when it is a
      fixed spec type on a different axis. The binary knows the four types (`store.SpecTypes`) and
      `docs/capability-spec.md` defines each one, yet every project has to invent its own wording — and
      those descriptions are what `map` shows an agent deciding where to descend.
      *Fix:* ship a description per spec type alongside the concerns catalogue and default from it, the
      way a concern defaults. This also resolves the empty description in item B-2 above.

---

## D. Skills — text that does not match the tool

- [x] **`openrecord-consult`: "a grep hit path *is* a map coordinate" is false** `[C-1]`
      `grep` returns file paths; feeding one to `map` fails. The caller must strip the last segment and
      the skill never says so. The error compounds it: `.md` is stripped first, so the message points at
      a phantom subgroup (`decisions/.../well-formed-not-true does not exist`) instead of saying *that
      is a record — open it, do not descend into it*.
      *Fix (skill):* say that a hit's **parent** is the coordinate and the hit itself is the file to
      open. *Fix (tool):* when a coordinate resolves to an existing `.md` file, say so.

- [x] **`openrecord-consult`: nothing routes a repository path to the specs that cover it** `[C-2]`
      `component owners` answers with one `decisions/<component>` coordinate and stops; the skill then
      says *"Specs are behaviour, decisions are construction"* without a step for reaching them. The
      data exists — `map --for specs/flow` returns each spec's `components` — but no command crosses a
      path to it. In this run I enumerated all three spec types by hand.
      *Fix:* have `component owners` also return the specs naming that component, or add a step to the
      skill that enumerates `specs/*` and filters on `components`.

- [x] **`setup-record-search` names no registration command** `[S-1]`
      The skill specifies collection names, the mask, that `INDEX.md` is included and that paths are
      absolute — and never names `qmd collection add`, the indexing step or the embedding step. It also
      never mentions `openrecord qmd status`, which is the command openrecord ships for exactly this and
      whose `collections_needed` output is directly usable.
      *Fix:* add the commands. Note in passing that `qmd collection add` indexes immediately, so
      registration and first indexing are one step, not the two the skill implies.

- [x] **`setup-record-search`: its own safety rule is not executable** `[S-2]`
      *"If the provider is not configured, say so and stop rather than starting"* — but there is no
      preflight named, and the failure only appears at `qmd embed`, after all collections are registered
      and lexically indexed. Following the skill produces exactly the half-built state it warns against.
      *Fix:* name `qmd doctor` (or an equivalent) as the first step, before any `collection add`.

- [x] **A failed semantic search is indistinguishable from an empty one** `[S-3]` — **both halves done.
      See section J for the second one, which was fixed in qmd.**
      With an expired provider key, `qmd query` prints `No results found.` and exits 0; the 401s scroll
      past and appear in no structured output. `openrecord-consult`'s rule *"No search proves an
      absence"* is right, and this is the sharpest case: the search did not run at all and said nothing
      a caller could act on.
      *Fix (skill):* **done** — `openrecord-consult` and `openrecord-setup-search` now require
      establishing that the semantic path ran before reporting a miss.
      *Fix (tool):* **done, in qmd** — see section J. `openrecord qmd status` reports `usable`, which
      is a different question and never caught this on its own: the probe runs `qmd status`, which
      reads the local index and never touches the provider.

- [x] **`qmd://` results are not openrecord coordinates, and `openrecord-consult` says they are** `[S-4]`
      *"they all return paths, and a path is a coordinate, nothing has to be translated between them."*
      Turning `qmd://openrecord-e2e-decisions-format/runtime/one-finding-vocabulary.md:29` into
      `decisions/format/runtime/one-finding-vocabulary.md` takes five transformations, and splitting the
      collection name on `-decisions-` is ambiguous for any project whose name contains that string.
      *Fix:* document the translation in the skill, or have openrecord provide it.

- [x] **`openrecord-bootstrap` stops one step before writing anything** `[B-1]`
      Names `component add` and `concerns`; never names `level add`, `record write`, `--body-file` or
      `--components`. Its behaviour half ("Behaviour is a different interview") has no commands at all.
      *Fix:* add the write path, or point explicitly at `openrecord-capture`, which documents it.

- [x] **The three skills contradict each other on who confirms a record** `[CAP-1]`
      `openrecord-capture`: *"The store maintains itself ... you resolve what should be written and you
      write it."* `openrecord-bootstrap`: *"each one is confirmed by the user before it is written."*
      `openrecord-mine`: *"Nothing is materialised without that confirmation."* The distinction is
      presumably who supplied the *why*, which is coherent — but no skill states it, and each reads as a
      general rule.
      *Fix:* state the rule once, in each of the three.

- [x] **`openrecord-capture` does not say that a clean write is not a clean store** `[CAP-2]`
      `check.Record` runs on write; `flatConcerns`, `Components.Validate` and `orphanComponentFolders`
      run only in `check.Store`. So `"warnings": []` on a write says nothing about the store, and the
      skill never tells you to run `validate` afterwards. Two scenarios I wrote into the specs during
      this run asserted the opposite and had to be corrected.
      *Fix:* one sentence in the skill, and consider naming the split in the JSON (e.g. `"warnings"`
      scoped to the record).

- [x] **`openrecord-mine` has no position on a *why* written in a source comment** `[M-1]`
      *"You usually cannot recover why"* and *"do not compose a plausible rationale"* are both right, but
      this codebase writes its reasoning into package comments — `internal/qmd/qmd.go:21-31` and
      `.github/workflows/release.yml` each contain a complete context/alternative/consequence. Quoting a
      person's stated reason is neither recovery from behaviour nor invention, and the skill leaves an
      agent to improvise.
      *Fix:* say that an author's written rationale is evidence, and must be quoted rather than
      paraphrased.

- [x] **`openrecord-mine` never mentions semantic search** `[M-2]`
      Its "do not mine what is already there" check uses only literal `grep`, so a candidate phrased
      differently from an existing record passes and becomes a duplicate — the exact weakness
      `openrecord-consult` warns about. The skill has no `qmd:` block at all, confirmed: emitting with
      and without `--with-qmd` changes only `openrecord-consult/SKILL.md`.
      *Fix:* add a `qmd:` passage to the duplicate check.

- [x] **`setup-record-search` is the one skill without the `openrecord-` prefix** `[S-5]`
      The other four are namespaced. This machine already had a differently-behaving skill of that exact
      name; after emitting, both were live with the same trigger surface, and whichever wins the other
      is silently unavailable.
      *Fix:* rename to `openrecord-setup-search`. `emit` already removes a file this build no longer
      ships, so a rename cleans up after itself for anyone who re-emits.

---

## E. Repository configuration

- [x] **`.gitignore` excludes `.openrecord/`** `[I-4]`
      ```
      $ git check-ignore -v .openrecord/components.json
      .gitignore:8:.openrecord/	.openrecord/components.json
      ```
      Comment: *"A store this repository might grow for its own records."* This contradicts the format
      as documented — `setup-record-search`: *"A store is versioned and shared"*; `docs/decision-record.md`
      on the absent `date` field: *"git knows when the file was created and last touched"*, and on the
      absent `superseded` status: *"you edit the record in place. Git carries the history."* The header
      design depends on git carrying the store, and here git is configured not to. After 13 records,
      `git status` showed nothing.
      *Fix:* remove the entry, or replace it with a comment explaining why this repository is the
      exception.

---

## F. Missing deterministic tests

Existing coverage is broad — 12 test files, ~60 test functions, and the atomicity guarantee held under
every adversarial probe in the run. The gaps below are not "untested areas"; they are cases where **a
test exists and asserts something weaker than the property that matters**, which is why each of the bugs
above shipped green.

### F.1 Tests that assert too little today — tighten these first

- [x] **`internal/cli/commands_test.go:225` — `TestGrepSeparatesIndexHitsFromRecordHits`**
      It checks `store.ParseCoordinate(match.Path)` returns no error, and calls that "a hit path is a map
      coordinate". `ParseCoordinate` validates *syntax only* — `decisions/api/security/gateway` is three
      segments, the legal maximum, so it parses while `map --for` on it fails at runtime. The test
      asserts the skill's claim with the one check that cannot falsify it.
      *Add:* assert the round trip that is actually claimed — for a record hit, `map --for
      path.Dir(hit)` must exit 0 and list that record; for an index hit, `map --for path.Dir(hit)` must
      exit 0. Keep the syntax check as well.

- [x] **`internal/store/store_test.go:190` — `TestLevelListsDeclaredComponentsAndFixedTypes`**
      Asserts only `len(entries) == len(SpecTypes)`. It locks in the behaviour that produces the
      unnavigable `specs/process` entry without checking any consequence of it.
      *Add:* assert every entry has a non-empty `Title` and `Description`, and that every entry marked
      `EntryGroup` is one `Level()` accepts.

- [x] **`internal/cli/diagram_test.go:40` — `TestDiagramRendersEachType`, lifecycle case**
      Uses `pending`, `active`, `expired` — all single words, so `mermaidID` is an identity and the
      `state "..." as ...` line is never emitted. The buggy branch has no coverage at all.
      *Add:* a state name with spaces, asserting the emitted line is exactly
      `state "written by us" as written_by_us` — one pair of quotes.

- [x] **`internal/cli/diagram_test.go:24` — `TestDiagramRendersEachType`, flow case**
      Every step and branch in the fixture is a single physical line, so the truncation path is never
      exercised.
      *Add:* a step and a branch that wrap across two lines, asserting the node text contains the
      continuation's last word.

### F.2 Deterministic cases with no test at all

**`internal/cli` — coordinate resolution**

- [x] `TestValidateRejectsACoordinateThatDoesNotExist` — `validate --for decisions/nope` must exit
      non-zero with `invalid-coordinate`, matching `map`. Today it exits 0 with `findings: []`.
      Table-drive it over the same inputs as `TestMapRejectsABadCoordinate`, plus the well-formed
      absent case, and assert `map` and `validate` agree on every row.
- [x] `TestACoordinateNamingARecordSaysSo` — `map --for decisions/a/b/record.md` must report that the
      target is a record to open, not `does not exist` about a stripped path.
- [x] `TestGrepAndMapAgreeOnEveryHit` — property-style: for a fixture store, every `grep` hit's parent
      resolves under `map`. This is the invariant `openrecord-consult` sells.

**`internal/cli` — the JSON contract**

- [x] `TestRejectedEditReportsItselfAsAnEdit` — the failure envelope must not key the result under
      `written` when the command was `edit`.
- [x] `TestWarningsOnWriteAreScopedToTheRecord` — write into a level holding five loose records and one
      with a declared surface pointing at a deleted directory; assert `warnings` is empty **and** that
      `validate` reports both. This pins the split deliberately instead of leaving it implicit, and it
      is the behaviour that falsified two of the specs written during the run.
- [x] `TestHelpNamesEveryFlagACommandRequires` — walk the command tree and assert each command's help
      mentions every flag whose absence causes a `usage` failure. Catches the missing `--components`
      and `--dry-run` mechanically, and keeps catching them.

**`internal/cli` — emit**

- [x] `TestSkillsDryRunTouchesNothing` — `emit_test.go:140` covers `Plan` at the package level, but the
      CLI `--dry-run` path (`skills.go:65`, the early return before `Apply`) has no test. Assert the
      report is identical to the real run's and that the directory hash is unchanged.
- [x] `TestSkillsReportsAQmdMismatch` — `qmdMismatch` has four branches and no test. Assert each of the
      three notes fires for its condition and that the emitted bytes do **not** change with what happens
      to be installed, which is the property the comment says it is protecting.

**`internal/qmd`**

- [x] `TestCheckDistinguishesPresentFromUsable` — a stub `qmd` that answers `--version` and fails
      everything else must not be reported as plainly installed.
- [x] `TestInstallComparesAgainstThePinnedVersion` — an older present version must not short-circuit.
      Both need `Check`/`Install` to take an injectable runner; today they call `exec` directly, which
      is itself why they are untested.

**`internal/cli` — level defaults**

- [x] `TestLevelAddDefaultsTheFourSpecTypes` — `level add specs/flow` with no flags must succeed and
      take its description from a shipped source, the way a concern does. Fails today by design; it is
      the test for the fix.
- [x] `TestSpecTypeDescriptionsAreShipped` — every entry in `store.SpecTypes` has a description in the
      binary. Mirrors `TestCatalogueParses`, and is what keeps `map` from ever emitting an empty one.

**`internal/cli/paths_test.go` — extend the existing skill audit**

`TestSkillsOnlyInvokeCommandsThatExist` already parses commands out of the skill prose and checks they
resolve. Two cheap extensions would have caught three of the skill findings above:

- [x] `TestSkillsNameEveryRequiredFlag` — for each command invocation found in a skill, assert the flags
      it shows are enough for that command to succeed. Catches `openrecord-bootstrap` never showing
      `--components`.
- [x] `TestEverySkillNameIsNamespaced` — assert every emitted directory starts with `openrecord-`.
      Catches `setup-record-search`.
- [x] `TestSkillsInvokeQmdCommandsThatExist` — the `invocation` regex at `paths_test.go:59` is anchored
      to `openrecord `, so no `qmd ...` line in any skill is checked by anything. Add a pinned list of
      the qmd commands the skills rely on and assert against it. This is a weaker guard than the
      openrecord one (the list is hand-maintained rather than walked from a command tree), but it is what
      would have surfaced `setup-record-search` naming only `qmd query` while its whole subject is
      `qmd collection add`.

**`internal/check`**

- [x] `TestFlatConcernIsNotRaisedByARecordCheck` — assert explicitly that `check.Record` never returns
      `concern-too-flat`, `component-path-missing` or `orphan-component-folder`. The split is currently
      only implied by which function calls which.

---

## G. Verified as correct — do not regress

Recorded so a fix in one area does not quietly cost one of these. All reproduced in a scratch repository
during the run:

- A refused write leaves no file behind; a refused edit leaves the original byte-for-byte identical.
- `component remove` is blocked by both records filed under the surface and specs naming it, and names
  the files holding it.
- A spec with an undeclared component, with no components, or with `--status proposed` is refused, each
  with its own code.
- A branch anchored past the last step is refused, and `diagram` refuses to render that spec at all.
- A coordinate containing `..` or a leading `/` is refused as an attempt rather than cleaned up.
- `skills --emit --dry-run` leaves the target directory hash unchanged.
- Exit statuses really do separate 2 (bad invocation) from 1 (a finding); findings sort stably, so two
  runs over one store are byte-identical.
- `component owners` returns `"owner": null` plus the declared ids rather than defaulting to something.

---

## H. Found while re-verifying — closed

Not part of the 45 above. Surfaced by re-running the brief's walk against the fixed binary, and by the
doc guard added in F.

- [x] **`docs/cli.md` described behaviour the fixes had changed.** It still claimed a `grep` hit "**is**
      a `map` coordinate", documented `component owners` without the `specs` half, `level add` without
      the spec-type defaults or the subgroup case, `grep` without per-file entries, `qmd status` without
      `usable`/`pinned_version`, and `qmd install` without `--force`. Shipped in the release archive,
      so it is the account a person reads.
- [x] **`openrecord version` was undocumented.** Not in the command table, no section. Caught
      immediately by `TestEveryCommandIsDocumented`, which is the point of adding it.
- [x] **`TestEveryCommandIsDocumented` / `TestDocumentedFlagsExist`** — the guards. The first fails on a
      command nobody documented; the second on a flag the documentation shows and the binary does not
      accept.

## I. Found while re-verifying — closed

- [x] **The linking mechanism was still named in the documentation after being removed from the
      format.**

      `docs/decision-record.md` justified dropping the `related` / `supersedes` frontmatter fields with
      *"wikilinks in the body already do this"*, carried a section titled **Records do not link to each
      other**, and claimed `validate` checked "wikilinks resolving" — with no wikilink handling
      anywhere in the code. `docs/capability-spec.md` carried the same section, pointing back at it.

      **Settled: linking was removed from the format, and it is not named anywhere.** Both sections are
      gone, along with the anchor into them. The relations bullet now states its own reason — a list of
      other records goes stale unnoticed and breaks when one is renamed — and says what to do instead:
      the prose names the other record in words, and there are already two ways to reach it. The
      `validate` sentence lists what it actually checks.

      No guard was added. A test asserting that a removed concept stays unmentioned keeps the concept
      alive in the codebase, which is the thing being removed.

---

## J. Closed in qmd — a dead search provider used to report success

`openrecord qmd status` reported `usable: true` for a qmd whose embedding provider was unreachable, and
every search then returned an empty result no caller could distinguish from a genuine miss.

Measured against `qmd 2.8.3-mate.4`, with a deliberately invalid key:

```
openrecord qmd status        → installed: true, usable: true, note: null
qmd status                   → exit 0
qmd doctor                   → exit 0     (it printed the 401, and still exited 0)
qmd vsearch …                → exit 0     "No results found."
qmd embed …                  → exit 0     (embedded nothing)
qmd vsearch … --format json  → []         valid JSON, no error field
```

With a working key the same JSON is a list of five results. **The two states were identical in every
machine-readable output qmd produced**, and `doctor --json` / `status --json` are ignored — `--format`
exists only on the search commands. Detecting it from openrecord would have meant matching qmd's human
prose on stderr for `401` or `embed failed`.

### Why it was not fixed here

`decisions/search/integration/optional-and-never-inspected.md` settles that openrecord never reads the
search tool's own state: *"which collections it has registered lives in its configuration, in its
format, and reading that would break the day it changes."* Scraping its error prose is the same
coupling in a worse place — prose changes more freely than configuration. Three of the four ways
forward contradicted that record; the fourth put the fix where the defect is.

### What was done

Fixed in qmd and released as **`2.8.3-mate.5`**, which this repository now pins.

The root defect was that `null` meant two things in the OpenAI-compatible backend — *the provider said
there is nothing* and *the provider was never asked* — so a caller could not tell them apart, and by the
top of a search the difference was gone. Failures are now recorded as well as logged; nothing in the
pipeline changed behaviour, and the command at the top asks once whether the work it is about to report
on could be done.

```
                             qmd 2.8.3-mate.4   qmd 2.8.3-mate.5
vsearch, bad key                   exit 0            exit 1
vsearch --format json, bad key     exit 0            exit 1
doctor, bad key                    exit 0            exit 1
vsearch, good key                  exit 0            exit 0
search that genuinely finds
  nothing, good key                exit 0            exit 0
```

`doctor` gained a `search provider` check that reports three states rather than two — failed, answered,
or never exercised by the checks, the last claiming nothing rather than reporting healthy. Searches say
`Search did not run.` instead of `No results found.`, and `min_score` is untouched: results existed and
were filtered, so that search did run.

openrecord needs no output parsing to ask the question — the exit status answers it.
