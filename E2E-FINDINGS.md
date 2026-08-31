# openrecord end-to-end findings

Run as a first-time consumer on openrecord's own repository. Nothing under `internal/`, `cmd/`, `docs/`
or `skills/` was modified; nothing was committed or pushed. Everything written landed in `.openrecord/`,
`.claude/skills/` and this file.

Environment: Linux (WSL2), `openrecord v0.1.0` (commit `847a465`, store format `1`), `qmd` — see §5.

**Summary.** The five skills are readable and the format is coherent; the store filled up without me
having to guess at the format. What did not hold: two reproducible defects in `openrecord diagram`, a
`map` listing that advertises a coordinate `map` itself rejects, a `validate` that reports clean for a
coordinate that does not exist, a claim in `openrecord-consult` about grep hits that is false for almost
every hit, a `setup-record-search` skill that names no registration command at all, and a `.gitignore`
in this repo that excludes the store the format says git must carry.

---

## 0. Install

### What ran

```
curl -fsSL https://raw.githubusercontent.com/franwerner/open-record/master/scripts/install.sh | WITH_QMD=yes bash
```

```
==> downloading openrecord v0.1.0 (linux/amd64)
==> installed /home/ifran/.local/bin/openrecord
==> qmd is already installed
{ "version": "0.1.0", "commit": "847a465...", "date": "2026-08-31T15:19:05Z", "store_format": "1" }
```

`openrecord version` and `openrecord skills --emit .claude/skills/ --with-qmd` both worked first time
(5 skills created). The binary install half of this is clean.

### Finding I-1 — `WITH_QMD=yes` silently accepted a qmd that cannot run

The installer's idempotence check is `command -v qmd`. On this machine that resolved to a pre-existing
`qmd 2.5.3` at `~/.bun/bin/qmd` whose native SQLite bindings are missing, so it prints
`==> qmd is already installed` and installs nothing — including not the version openrecord pins
(`v2.8.3-mate.4`, `internal/qmd/qmd.go:31` and `scripts/install.sh`).

The result is a qmd that answers `--version` and dies on every real command:

```
$ qmd --version
qmd 2.5.3                                    # exit 0

$ qmd collection list
Error: Could not locate the bindings file. Tried:
 → .../better-sqlite3/build/better_sqlite3.node
 ...
Node.js v22.23.2                             # exit 1
```

`command -v qmd` cannot distinguish *present* from *working* and cannot distinguish *some version* from
*the pinned version*. A user who passes `WITH_QMD=yes` is entitled to assume they got the qmd openrecord
was tested against; here they did not, and nothing said so.

### Finding I-2 — `openrecord qmd status` reports a broken qmd as healthy

```
$ openrecord qmd status
{
  "installed": true,
  "path": "/home/ifran/.bun/bin/qmd",
  "version": "qmd 2.5.3",
  ...
}
```

`qmd.Check()` (`internal/qmd/qmd.go:41`) does `LookPath` plus `--version`, and `--version` is the one
command that does not open the database. So the health check is green for a qmd on which every
subsequent instruction in `setup-record-search` fails. The package comment justifies the leniency
("a tool that does not answer is still installed — the version is a nicety"), but the case that actually
occurs is the opposite one: it answers, and works for nothing.

### Finding I-3 — `openrecord qmd install` cannot repair or upgrade

The documented remedy is `openrecord qmd install`. It short-circuits on the same check:

```
$ openrecord qmd install
{ "installed": true, "note": "already installed; nothing to do",
  "path": "/home/ifran/.bun/bin/qmd", "version": "qmd 2.5.3" }        # exit 0
```

It does not compare `2.5.3` against the pinned `2.8.3-mate.4`, and it has no force/reinstall flag. From
a broken or outdated qmd there is no path forward through openrecord's own commands. I had to install
the pinned tarball by hand into a private prefix to continue:

```
npm install -g --prefix <scratch> https://github.com/franwerner/qmd/releases/download/v2.8.3-mate.4/tobilu-qmd-2.8.3-mate.4.tgz
```

Related, and worse in the general case: `npm install -g` writes to the npm prefix, but the broken qmd
was earlier on `PATH` (`~/.bun/bin` at position 33, `~/.npm-global/bin` at 207). Even had the install
run, the broken binary would still have won. Nothing warns about this.

### Finding I-4 — this repository gitignores the store

```
$ git check-ignore -v .openrecord/components.json
.gitignore:8:.openrecord/	.openrecord/components.json
```

`.gitignore` carries `.openrecord/` with the comment *"A store this repository might grow for its own
records."* That contradicts the format as documented:

- `setup-record-search/SKILL.md` — *"A store is versioned and shared; a key in it is a key published."*
- `docs/decision-record.md` on the absent `date` field — *"git knows when the file was created and last
  touched, and cannot drift"*; on the absent `superseded` status — *"you edit the record in place. Git
  carries the history."*

The header design depends on git carrying the store. Here git is configured not to. After writing 13
records, `git status --short` showed nothing for `.openrecord/` — a first-time user following the
skills in this repo would commit and lose the whole store without a single message.

---

## 1. openrecord-bootstrap

**Did every command exist and behave as described?** The two commands the skill names —
`openrecord component add` and `openrecord concerns` — exist and behave exactly as written. `concerns`
prints eleven concerns and 49 topics (counted; the header's claim is accurate).

**Did it leave me able to act?** For components, yes. Beyond components, no — see B-1.

### What I did

Declared five surfaces, cut by reason-to-change rather than by package:

```
openrecord component add cli    --path cmd/openrecord --path internal/cli --title "CLI" --description "..."
openrecord component add format --path internal/store --path internal/check --path internal/frontmatter \
                                --path internal/finding --path internal/catalogue --title "Record format" ...
openrecord component add skills --path skills --path internal/emit --title "Agent skills" ...
openrecord component add search --path internal/qmd --title "Semantic search" ...
openrecord component add root   --path . --title "Repository" ...
```

Then walked the catalogue per surface and wrote **9 decision records and 4 capability specs**, all real
and grounded in this repo's code and its package comments (inventory in §6). `openrecord validate`
returns `{"findings": []}`.

### Finding B-1 — the skill stops one step short of writing anything

The skill names `component add` and `concerns`, then says *"Write records as you go"* and *"Pick the
type from what the thing is"* — and never names `level add` or `record write`. It shows no command for
either, no `--body-file`, and no `--components`. Coming from this skill alone I could declare surfaces
and then had nowhere to go; I recovered the write path from `--help` and from `openrecord-capture`,
which does document it.

Concretely, following bootstrap in order, the first write fails:

```
$ openrecord record write decisions/api/runtime/x.md --title T --description D --status accepted --body-file body.md
{"code":"usage","severity":"error",
 "message":"decisions/api/runtime does not exist; create it with `openrecord level add decisions/api/runtime`"}
```

The error is excellent — it names the missing step and the exact command. But bootstrap should not have
let me arrive there, and its behaviour half ("Behaviour is a different interview") is the part with no
commands at all.

### Finding B-2 — spec types are a fixed set the binary knows, yet `level add` refuses to default them

The four spec types are a constant in the binary (`store.SpecTypes`) and each has a documented meaning
in `docs/capability-spec.md`. `level add` still refuses to create one:

```
$ openrecord level add specs/flow
{"code":"usage","severity":"error",
 "message":"\"flow\" is not in the catalogue, so level add needs --title and --description;
            the catalogue has contracts, data, data-lifecycle, delivery, domain-logic, integration,
            observability, quality, runtime, security, structure"}
```

The message is actively misleading: it lists the *decision* concerns as if `flow` were a failed attempt
at one, when `flow` is a fixed spec type on a different axis. So every project invents its own wording
for the same four fixed types, and those descriptions are what `map` shows an agent deciding where to
descend — the one place the format most wants a shared vocabulary.

### Finding B-3 — the interview premise has no fallback for a headless run

The skill is explicitly interview-driven: *"An empty store gets filled by asking, not by inferring"*,
*"Do not propose names"*, *"each one is confirmed by the user before it is written"*. In this run there
was no human to interview; the brief made me both tester and owner, and I sourced the *what* from the
code and the *why* from the authors' own package comments.

That is a legitimate reading, but the skill does not acknowledge the case at all, and it is the case an
agent will hit constantly (a CI run, a batch, a non-interactive session). It also sits oddly next to
`openrecord-mine`, which does exactly the inferring bootstrap forbids and hands back candidates. The
boundary between "ask, never infer" and "infer, then have it confirmed" is never drawn.

### Finding B-4 — nothing points at the `--components` flag

`record write --help` prints:

```
Usage:
  openrecord record write PATH --title TITLE --description TEXT --status STATUS --body-file FILE
```

`--components` is missing, yet it is **mandatory for every spec** and the write fails without it:

```
$ openrecord record write specs/flow/z.md --title T --description D --status accepted --body-file spec.md
{"findings":[{"code":"empty-components","severity":"error","path":"specs/flow/z.md",
 "message":"a spec must declare the components it reaches: ..."}],"written":null}
```

Neither the help text nor bootstrap mentions it. The failure is at least loud and self-explanatory, so
this is recoverable — but only after a failed write.

---

## 2. openrecord-consult

**File chosen:** `internal/cli/diagram.go` — a file I would genuinely change, because I found two
defects in it (§D below).

**Did every command exist and behave as described?** `component owners`, `map --for` and `grep --for`
all exist and work. Two of the skill's claims about them are false — C-1 and C-2.

### The walk, as the skill prescribes

```
$ openrecord component owners internal/cli/diagram.go
{"map":"decisions/cli","owner":"cli","path":"internal/cli/diagram.go"}

$ openrecord map --for decisions/cli
  [group] decisions/cli/contracts   "What the system promises to anything outside it..."
  [group] decisions/cli/structure   "How the codebase is organised..."

$ openrecord map --for decisions/cli/contracts
  [record] decisions/cli/contracts/machine-readable-output.md  [accepted]

$ openrecord map --for decisions/cli/structure
  [record] decisions/cli/structure/command-tree.md             [accepted]

$ openrecord grep "diagram"                 → {"matches": []}
$ openrecord grep "Mermaid"                 → {"matches": []}
$ openrecord grep "state machine"           → 2 hits (one record, one INDEX)
```

### What I found — the skill's own report shape

```
Governs this work:

- decisions/cli/contracts/machine-readable-output.md               [accepted]
  Constrains you: whatever diagram emits, the failure path stays JSON-on-stdout with a coded
  finding, and the exit status must keep separating a bad invocation from a real finding.
  Surfaced by: descent (cli/contracts)

- decisions/format/delivery/well-formed-not-true.md                [accepted]
  Constrains you: the structural checks exist to catch what a reader would not — a branch anchored
  to a step that does not exist. A diagram fix must not weaken that; `diagram` refuses to render a
  spec that does not hold together, and that stays.
  Surfaced by: grep "state machine"

- specs/lifecycle/emitted-file.md, specs/flow/*                    [accepted]
  Constrains you: these are the inputs the renderer has to keep rendering correctly.
  Surfaced by: manual descent through specs/ — see C-2, nothing routes a file to its specs.

Looked at, does not apply:
- decisions/cli/structure/command-tree.md — about dispatch shape; diagram is already a leaf command.
- decisions/format/structure, decisions/format/data — layout and header rules; the renderer reads a
  parsed record and touches neither.
- decisions/search, decisions/skills, decisions/root — different surfaces entirely.
```

### Finding C-1 — "a grep hit path *is* a map coordinate" is false for almost every hit

The skill states:

> A hit comes back as a path, and that path **is** a `map` coordinate — so a search leaves you standing
> at the right level to keep descending.

`grep` returns *file* paths — records and `INDEX.md` files. Feeding one straight to `map`, which is
exactly what the sentence invites, fails:

```
$ openrecord grep "state machine"
{"matches":[{"path":"decisions/format/delivery/well-formed-not-true.md", ...},
            {"path":"specs/lifecycle/INDEX.md", ...}]}

$ openrecord map --for decisions/format/delivery/well-formed-not-true.md
{"code":"invalid-coordinate","severity":"error",
 "path":"decisions/format/delivery/well-formed-not-true",
 "message":"decisions/format/delivery/well-formed-not-true does not exist"}        # exit 1

$ openrecord map --for specs/lifecycle/INDEX.md
{"code":"invalid-coordinate","severity":"error","path":"specs/lifecycle/INDEX",
 "message":"specs/lifecycle/INDEX does not exist"}                                  # exit 1
```

The caller has to strip the last segment, and the skill never says so. The error compounds it: `.md` is
silently stripped first, so the message points at a phantom subgroup (`.../well-formed-not-true`) rather
than saying *that is a record — open it, do not descend into it*. A deeper decision hit produces a
different message again (`"is 4 levels below decisions"`), so the same mistake reads three ways.

### Finding C-2 — nothing routes a repository path to the specs that cover it

The walk is decisions-only. `component owners` answers with a single `decisions/<component>` coordinate
and stops. Then step 4 says *"Specs are behaviour, decisions are construction"* without ever giving a
step for reaching the specs.

The data to do it exists — `map --for specs/flow` returns each spec's `components` list — but no command
crosses a path to it, and the skill names none. In practice I enumerated `specs/flow`, `specs/rule` and
`specs/lifecycle` by hand and filtered by eye. For a store with dozens of flows that does not scale, and
it is precisely the enumeration the skill says search cannot substitute for.

### Finding C-3 — `map` advertises a group that `map` then rejects

```
$ openrecord map --for specs
{"for":"specs","entries":[
  {"kind":"group","path":"specs/flow","title":"Flow","description":"An operation an actor triggers..."},
  {"kind":"group","path":"specs/rule", ...},
  {"kind":"group","path":"specs/lifecycle", ...},
  {"kind":"group","path":"specs/process","title":"process","description":""}
]}

$ openrecord map --for specs/process
{"code":"invalid-coordinate","severity":"error","path":"specs/process",
 "message":"specs/process does not exist"}                                          # exit 1
```

I never created `specs/process`; there is no such directory. `map` synthesises the missing spec types
from the fixed list, gives them a lowercase title and an **empty description**, and marks them
`kind: group` — the value the skill defines as *"descend"*. Following that exact instruction errors out.

This breaks the skill's central promise about descent: *"Each level returns the `title` and
`description`... The descriptions arrive in the response; you never open an index yourself."* Here the
description is empty, so there is nothing to decide on. And `validate` calls the store clean
(`{"findings": []}`), so nothing flags it either.

It also sits badly against B-2: creating the level is refused unless you invent a description, but the
listing fabricates the entry with none.

### Finding C-4 — `grep` returns one match per line, not per record

```
$ openrecord grep "version" --for decisions | (paths)
decisions/format/data/frontmatter-carries-four-things.md   ×3
decisions/root/delivery/single-binary-piped-installer.md   ×4
decisions/search/integration/optional-and-never-inspected.md ×4
...
```

13 matches, 5 records. The skill says the steps *"produce **candidates**, deduplicated by path"* — but
the deduplication is the caller's job and nothing says so. An agent that counts hits to gauge coverage
over-counts by 2–4×.

---

## 3. openrecord-capture

**Did every command exist and behave as described?** Yes. This is the skill with the fewest problems:
`grep`, `map`, `level add`, `record write` and `record edit` all exist, and their invocations in the
skill are correct and complete (it is the only skill that shows `--body-file`).

**Did it leave me able to act?** Yes.

### What I did

Work in scope: populating this store and running the consult walk. Applying the skill's two questions:

1. *Did this change what someone using the product observes?* No. Nothing shipped.
2. *Did this settle a choice between alternatives about how it is built?* Once.

The one thing that survived was the **component decomposition of this repository** — a real choice
(one surface per Go package / a single surface / five surfaces cut by reason-to-change), with a real
cost accepted, that governs the coordinate of every record written from now on.

```
Written:
- created   decisions/root/structure/five-surfaces.md
            Because: the cut was a choice with three rejected alternatives, and it fixes the first
            segment of every future coordinate and the partitioning of the search index.

Not recorded:
- installing the binary and emitting the skills — mechanical, settled nothing.
- the concern levels created along the way — the catalogue chose their descriptions; no choice was made.
- the consult walk over diagram.go — it read; it decided nothing.
- the two defects found in `openrecord diagram` — bugs, not decisions. Reported here, §D.
```

### Finding CAP-1 — "the store maintains itself" collides with the other two skills

Capture says plainly: *"Nobody approves a record. The store maintains itself. You do not present a list
and wait for someone to approve it — you resolve what should be written and you write it."*

Bootstrap says the opposite about the same store: *"each one is confirmed by the user before it is
written — not in a batch at the end, and not silently because they answered the question."* Mine says
*"Nothing is materialised without that confirmation."*

The distinction is presumably *who supplied the why* — capture writes reasoning from work just done,
bootstrap and mine handle reasoning that came from elsewhere. That is coherent, but no skill states it,
and each reads as a general rule about writing records. An agent that loads capture alone will write
without asking; one that loads bootstrap alone will never write without asking; and the descriptions
overlap enough that which one loads is not fully determined.

### Finding CAP-2 — the write path reports fewer findings than the skill implies

*"The binary validates both halves and writes nothing if anything fails."* True for errors. But a
successful write only reports findings about **that record**; findings about the store as a whole never
appear. Verified:

```
# 5 loose records in one concern — validate warns, write does not
$ openrecord record write decisions/api/runtime/x5.md ... → {"warnings":[],"written":"..."}
$ openrecord validate → {"findings":[{"code":"concern-too-flat","severity":"warning",
    "path":"decisions/api/runtime","message":"5 records sit loose here; ..."}]}

# a declared surface pointing at a deleted directory — same asymmetry
$ rm -rf src && openrecord record write decisions/api/runtime/x6.md ... → {"warnings":[]}
$ openrecord validate → {"findings":[{"code":"component-path-missing", ...}, ...]}
```

`check.Record` runs on write; `flatConcerns`, `Components.Validate` and `orphanComponentFolders` run
only in `check.Store`. That is a defensible split, but a `"warnings": []` on write reads as *the store is
clean* and it does not mean that. The skill never tells you to run `validate` afterwards.

This one bit me directly: two scenarios I had written into `specs/flow/write-a-record.md` and
`specs/rule/nothing-partial-lands.md` asserted a flat-concern warning on the write path. Testing them
falsified both, and I corrected the records with `record edit`. Findings about the record itself *do*
surface on write, which is what the corrected scenarios now say:

```
$ openrecord record write specs/lifecycle/w.md ... (a state nothing leaves)
{"warnings":[{"code":"dead-end-state","severity":"warning", ...},
             {"code":"unreachable-state","severity":"warning", ...}],
 "written":"specs/lifecycle/w.md"}                                                  # exit 0
```

### Finding CAP-3 — a rejected `record edit` reports itself under `"written"`

```
$ openrecord record edit decisions/api/runtime/x.md --section "## Decision" --body-file bad.md
{"findings":[{"code":"unexpected-section", ...}],
 "path":"decisions/api/runtime/x.md",
 "written":null}                                                                     # exit 1
```

A successful edit returns `{"edited": ..., "section": ...}`; a failed one returns `{"written": null}`.
A caller keying on `edited` sees neither success nor an explicit failure of the thing it asked for.
Cosmetic, but it is a machine contract.

---

## 4. openrecord-mine

**Did every command exist and behave as described?** Yes — the skill names only `map --for` and
`grep --for` for the "do not mine what is already there" check, and both work.

**Did it leave me able to act?** Yes. This skill is self-consistent: it returns candidates and writes
nothing, so there is no write path to get wrong. **I wrote none of the candidates below into the store.**

### Coverage check first, as the skill requires

```
$ openrecord grep "<topic>" --for decisions     # dependenc / library / internal / test / gofmt /
                                                # lint / pinned / logging / telemetry / embed
```

`library`, `gofmt`, `logging`, `telemetry` → no matches. The rest matched existing records and are
excluded below as already covered.

### Candidates, strongest evidence first

Each is anchored to what it was mined from. **For every one of them the *why* is not recorded**, except
where a source comment states it — noted explicitly, because a comment written by the author is
evidence, not a reconstruction.

**1. The module takes no third-party dependencies at all.** *(evidence: strong — absence, held
everywhere)*
`go.mod` has no `require` block; every import across 5,424 lines of Go is stdlib or internal. Command
dispatch, flag handling, the frontmatter parser, the Mermaid emitter and the JSON contract are all
hand-rolled where a well-known library existed. Consistency at this scale is intent.
Surface `root`, concern `structure`. Why not recorded at the project level — though
`internal/frontmatter/frontmatter.go:1-8` states it for *that* package: the format is "a handful of
known keys", narrowness makes rejecting everything else safe, and "a silently misparsed status is worse
than a parse error". Whether that reasoning was meant to generalise is not stated.

**2. Nothing is importable: the whole program lives under `internal/`.** *(evidence: strong —
enforcement)*
`cmd/openrecord/main.go` is 11 lines; all eight packages are under `internal/`; the only root package is
the `go:embed` holder. Go's `internal/` is compiler-enforced, so this is a decision with its enforcement
attached: openrecord is a program, not a library. Surface `root`, concern `contracts`
(`library-contract`, as an absence). Why not recorded.

**3. Quality gates: vet, test, and gofmt-clean, with tests re-run at release.** *(evidence: strong —
configuration)*
`.github/workflows/ci.yml` runs `go vet ./...`, `go test ./...`, and fails the build on any
non-gofmt-ed file via a hand-written shell block rather than a linter action.
`.goreleaser.yml` re-runs `go mod tidy` and `go test ./...` as `before.hooks`, so a release cannot ship
from a red tree. Surface `root`, concern `delivery` (`ci-quality-gates`). Why not recorded — in
particular, why no linter beyond `vet`.

**4. The release toolchain is pinned rather than floating.** *(evidence: strong — configuration, **and
the why is in the code**)*
`.github/workflows/release.yml` pins `goreleaser-action` to `~> v2.18` with the comment: *"Pinned rather
than `~> v2`: the archive config uses `formats`, which older v2 releases reject, so a floating version
would break the release on a machine that resolved to one of them."* That is a context, a rejected
alternative and a consequence — a complete decision, already written, just not in the store. Surface
`root`, concern `delivery`.

**5. Tests are white-box and colocated.** *(evidence: medium-strong — repetition)*
All 12 test files declare the package under test (`package cli`, not `package cli_test`), across 7 of the
8 internal packages. `internal/finding` and `cmd/openrecord` have none — both are near-trivial, so the absence
looks deliberate rather than a gap. Surface `root`, concern `delivery` (`testing-strategy`). Why not
recorded.

**6. No logging, metrics or telemetry anywhere.** *(evidence: medium — absence)*
No `log` import, no counters, no phone-home. The only output any code path produces is the JSON
contract on stdout and a coded finding on stderr. Surface `root` or `cli`, concern `observability`.
Weaker than the others: for a short-lived CLI this may be a default nobody chose rather than a decision,
which is exactly the case the skill says not to manufacture a record from. **Confirm before writing.**

### Already covered — not proposed, per "do not mine what is already there"

- Assets compiled into the binary (`assets.go`, with the `go:embed` cannot-reach-upwards reason in the
  comment) → covered by `decisions/root/delivery/single-binary-piped-installer.md`.
- The pinned qmd tarball and why not a git URL (`internal/qmd/qmd.go:21-31`) → covered by
  `decisions/search/integration/optional-and-never-inspected.md`.
- The finding vocabulary and the severity/code split → covered by
  `decisions/format/runtime/one-finding-vocabulary.md`.

### What I left out, and why

Behaviour mining (route/command handlers → `flow`) was not run to completion: the four specs already in
the store cover the emit flow, the write flow, the atomicity rule and the emitted-file lifecycle, but
`map`, `grep`, `validate`, `component owners`, `diagram`, `level add` and `qmd status` each have
observable behaviour with no spec. That is roughly seven more `flow` candidates. Stopping here is a
judgement about review capacity, not about coverage — stating it because the skill is right that silent
truncation reads as completeness.

### Finding M-1 — the skill has no position on a why written in a source comment

*"You can read **what** was chosen by reading the code. You usually cannot recover **why**"*, and
*"do not compose a plausible rationale"*. Both correct. But this codebase writes its reasoning into
package and inline comments, and candidates 1 and 4 above have a genuine, author-written why sitting
right next to the evidence.

Quoting it is neither recovering it from behaviour nor inventing it — it is a person's stated reason in
a place the store does not index. The skill gives no guidance, so an agent must either discard real
rationale to obey the letter of the rule, or improvise. Worth an explicit sentence, since a codebase
with good comments is the one where mining is most productive.

### Finding M-2 — no way to check "is this already covered" by meaning

*"Check the store before proposing anything"* with `map` and `grep`. `grep` is literal, so this check
is exactly as weak as the consult skill says literal search is: a candidate phrased differently from an
existing record passes the check and becomes a duplicate. This skill has no `<!-- qmd -->` passage at
all, so even with semantic search installed it never suggests using it — although the duplicate check is
the place where meaning-based search matters most.

Confirmed against the emitted output: only `openrecord-consult/SKILL.md` differs between
`--with-qmd` and plain emission.

```
$ openrecord skills --emit .claude/skills/ --dry-run          # (was emitted --with-qmd)
"changes":[ ... {"path":"openrecord-consult/SKILL.md","action":"updated"},
                {"path":"setup-record-search/SKILL.md","action":"removed", ...}]
"counts":{"removed":1,"unchanged":3,"updated":1}
```

`openrecord-bootstrap`, `openrecord-capture` and `openrecord-mine` are byte-identical either way.

---

## 5. setup-record-search

**Did every command the skill names exist?** The skill names exactly one qmd command — `qmd query` —
and it exists. **It names no command for the thing it is for.**

**Did it leave me able to act?** No. See S-1.

### Finding S-1 — the skill that registers the stores names no registration command

The title is *"Register a project's openrecord stores with qmd"*. The body specifies collection names,
a mask (`**/*.md`), that `INDEX.md` must be included, and that paths must be absolute — a complete
specification of *what* to register and nothing about *how*. There is no `qmd collection add`, no
indexing step, no embedding step.

I had to discover the CLI myself:

```
$ qmd collection add
Usage: qmd collection add <path> [--name NAME] [--mask GLOB]
```

Two consequences beyond the inconvenience. `qmd collection add` **indexes immediately** — so
registration and first indexing are one step, not the two the skill implies. And `--mask` exists but
there is no exclusion flag, which matters because the same machine's other openrecord-shaped store uses
`Ignore: **/INDEX.md` — the exact opposite of what this skill mandates. An agent that does the natural
thing and copies the neighbouring collection's shape violates the skill silently.

`openrecord qmd status` is the one thing that does help, and it is good:

```
$ openrecord qmd status
{"installed":true, "path":"...", "version":"qmd 2.8.3-mate.4 (e5171c6)",
 "project":"openrecord-e2e",
 "collections_needed":["openrecord-e2e-decisions-cli","openrecord-e2e-decisions-format",
   "openrecord-e2e-decisions-root","openrecord-e2e-decisions-search",
   "openrecord-e2e-decisions-skills","openrecord-e2e-specs"]}
```

The list is derived correctly from `components.json`, the project prefix is applied, and it is directly
consumable. But the skill never mentions this command, which is the one openrecord actually ships for
this job.

### What I did

Registered all six collections against the pinned qmd (installed by hand — see I-3):

```
$ qmd collection add /home/ifran/proyectos/openrecord-e2e/.openrecord/decisions/cli \
      --name openrecord-e2e-decisions-cli --mask '**/*.md'
Indexed: 5 new, 0 updated, 0 unchanged, 0 removed
✓ Collection 'openrecord-e2e-decisions-cli' created successfully
```

All six created; file counts include the `INDEX.md` files, as the skill requires (cli = 1 component
index + 2 concern indexes + 2 records = 5). Registration itself went cleanly.

### Finding S-2 — the skill's own safety rule cannot be executed with the commands it names

> Without a working provider, indexing fails **halfway** and leaves the index partly built... If the
> provider is not configured, say so and stop rather than starting.

Correct advice, and not actionable. There is no preflight command named, and the failure is invisible
until *after* registration, because `qmd collection add` succeeds on the lexical index and only the
later embedding step touches the provider:

```
$ qmd embed -c openrecord-e2e-decisions-cli
Failed to get embedding dimensions from first chunk

$ qmd doctor
[openai] embed failed: https://openrouter.ai/api/v1/embeddings -> 401:
  {"error":{"message":"API key expired.","code":401, ...}}
⚠ embedding freshness: 32 active documents need embeddings
```

By the time I could tell the provider was broken, all six collections were registered and lexically
indexed — the "partly built" state the skill warns against, reached by following the skill. The rule
needs a check that runs first; `qmd doctor` is the command that would serve, and the skill does not
mention it.

### Finding S-3 — a search that failed and a search that found nothing are the same output

This is the one that matters. With the provider dead, the semantic path returns a clean, confident
empty result:

```
$ qmd query "how are errors shaped at the boundary" -c openrecord-e2e-decisions-format
Warning: 32 documents (16%) need embeddings. Run 'qmd embed' for better results.
Expanding query...[openai] generate failed: ... 401: "API key expired."
Searching 1 queries...
Embedding 1 query...[openai] embedBatch failed: ... 401: "API key expired."
No results found.

$ qmd vsearch "what stops a half-written record from landing" -c openrecord-e2e-specs
... 401 ... No results found.
```

`No results found.` — the same sentence a genuine miss produces, with exit 0. The 401s are on the way
past, easy to miss, and absent entirely from any structured output. The consult skill's rule *"No search
proves an absence"* is exactly right, and this is the sharpest possible illustration: here the search
did not merely fail to prove an absence, it did not run at all and said nothing that a caller keying on
the result could distinguish.

For the record, the query in the first command is verbatim the skill's own example, against the surface
whose records are precisely about error shape at the boundary. It is the case that should have worked
best.

### Did a semantic search return anything useful?

**No — the semantic half could not run in this environment** (expired OpenRouter key in
`~/.config/qmd/env`, an environment problem, not openrecord's). I did not force a local embedding model
instead: the user's shared index at `~/.cache/qmd/index.sqlite` holds 1,405 vectors from a
1536-dimension model, and writing 768-dimension vectors into it would have degraded their existing
searches to prove a point about mine.

What does work without a provider is qmd's lexical half, and on these collections it returns genuinely
useful hits:

```
$ qmd search "errors" -c openrecord-e2e-decisions-format
qmd://openrecord-e2e-decisions-format/runtime/INDEX.md #51f157        Score: 64%
  description: "How the system behaves while it is running: ... error propagation."
qmd://openrecord-e2e-decisions-format/runtime/one-finding-vocabulary.md:29 #f09906   Score: 56%
  - **Errors carried as plain messages.** Rejected: ...

$ qmd search "manifest emitted skills" -c openrecord-e2e-decisions-skills -c openrecord-e2e-specs
qmd://openrecord-e2e-decisions-skills/delivery/emitted-with-a-manifest.md:2 #ee7882  Score: 92%
```

Two of the skill's design claims hold up here. Multi-collection queries do merge, so breadth really is
a flag. And including `INDEX.md` earns its keep — the top hit for a broad term was an index, which is
the *"descend here"* signal the skill predicted.

### Finding S-4 — qmd results are not openrecord coordinates, and consult says they are

`openrecord-consult` claims of its three search paths: *"because they all return paths, and a path is a
coordinate, nothing has to be translated between them."*

qmd returns `qmd://openrecord-e2e-decisions-format/runtime/one-finding-vocabulary.md:29`. Reaching the
openrecord coordinate `decisions/format/runtime/one-finding-vocabulary.md` requires stripping the
scheme, splitting the collection name on `-decisions-`, discarding the project prefix, re-joining the
component, and dropping the line number. That is five transformations, on the one path shape where the
skill promises none — and the collection-name split is ambiguous for any project whose name contains
`-decisions-`.

### Finding S-5 — an unprefixed skill name collides in a shared skills directory

Four emitted skills are prefixed `openrecord-*`. The fifth is `setup-record-search`, generic and
unowned. This machine already had a differently-behaving skill of that exact name; after emitting, both
were live in the same session with the same trigger surface. Whichever wins, the loser's behaviour is
silently unavailable. The other four have no such problem, and the fix is the naming convention the
tool already uses for them.

---

## D. Two defects in `openrecord diagram`

Found while consulting `internal/cli/diagram.go`. Reported, not fixed — this repo's source is out of
scope for this run.

### D-1 — the state diagram emits invalid Mermaid (doubled quotes)

```
$ openrecord diagram specs/lifecycle/emitted-file.md
stateDiagram-v2
    state ""written by us"" as written_by_us
    state ""edited locally"" as edited_locally
    state ""no longer ours"" as no_longer_ours
    absent --> written_by_us: "an emit ships this file and the directory does not have it"
    ...
```

`state ""written by us"" as written_by_us` is a syntax error; Mermaid expects `state "written by us" as
written_by_us`. Every `lifecycle` spec with a multi-word state name — which is every realistic one —
produces a diagram that will not render.

Cause, at `internal/cli/diagram.go:150`:

```go
declared = append(declared, fmt.Sprintf("    state \"%s\" as %s\n", mermaidText(name), id))
```

`mermaidText` (line 193) already wraps its argument in quotes, so the format string's quotes are the
second pair. The transition labels on the following lines have the same double-wrap, rendering the
quotes literally inside each label.

`openrecord validate` reports the store clean, because the diagram is a view and is never checked.

### D-2 — the flowchart silently truncates every wrapped step and branch

```
$ openrecord diagram specs/flow/emit-agent-skills.md
    S3["The system works out, per file, what would happen: created, unchanged, updated, overwritten, removed"]
    B2["it is replaced, and reported as"]
    B4["it is deleted, and"]
    B6["it is left on disk and"]
```

The source lines are:

```markdown
3. The system works out, per file, what would happen: created, unchanged, updated, overwritten, removed
   or kept.
- **[3] A file this build still ships was edited locally** → it is replaced, and reported as
  overwritten with a note saying these files are generated.
```

`diagramSections` (line 73) accumulates lines individually and `diagramStep` / `diagramBranch` match
single lines only, so every continuation line is dropped. `B4` renders as *"it is deleted, and"* — a
node whose text has lost its meaning entirely, with no marker that anything was cut.

This is not an exotic input. Every markdown file in this repository, including all of `docs/`, wraps at
about 100 columns, so any spec written in the house style produces a lying diagram. The docs state the
cost of diagrams being a view rather than stored — *"opening a spec in a markdown viewer shows no
diagram — you run the command"* — but the assumption behind that trade is that the command is faithful.

Also cosmetic: branch node ids are indexed by *line* rather than by branch (`B1, B2, B4, B6, B8`).

---

## E. Two more tool findings

### E-1 — `validate` reports clean for a coordinate that does not exist

```
$ openrecord validate --for decisions/nope
{"for":"decisions/nope","findings":[]}                                              # exit 0

$ openrecord map --for decisions/nope
{"code":"invalid-coordinate","severity":"error","path":"decisions/nope",
 "message":"decisions/nope does not exist"}                                          # exit 1
```

Same coordinate, two answers. A validation scoped to a mistyped or renamed component returns a green
`findings: []` — exactly what a CI step or an agent keys on to conclude the store is fine. `validate`
does reject a coordinate that is not a store at all (`nonsense/xx` → `invalid-coordinate`, exit 1), so
the strictness is there; it just stops one level too early.

### E-2 — everything else adversarial held

For balance, the checks that behaved correctly, all verified in a scratch repository:

| Attempt | Result |
| --- | --- |
| Write into a level that does not exist | Refused, message names `level add` and the exact coordinate |
| Body missing a required section | Refused; nothing on disk afterwards |
| Body with an unexpected section | Refused; the message lists the four allowed sections |
| Failing edit over a valid record | Refused; file byte-for-byte identical (sha256 compared) |
| Spec naming an undeclared component | Refused, `undeclared-component` |
| Spec with no components at all | Refused, `empty-components` |
| `--status proposed` | Refused, "the only values are accepted and pending" |
| `component remove` while records reference it | Refused; names the 1 file holding it; nothing deleted |
| Branch anchored past the last step | Refused, names how many steps exist |
| `skills --emit --dry-run` | Plan reported; directory hash unchanged |
| Coordinate with `..` or a leading `/` | Refused as an attempt, not cleaned up |

The atomicity guarantee is real. No operation in this run left a partial state.

---

## 6. Usability of the JSON output

**Directly usable, in most cases.** Stable key names, `kind: group | record` is the right discriminator
for a descent loop, findings carry a machine `code` alongside the human message, and `--help` is text
while everything else is JSON, so nothing has to be sniffed. Findings sort stably. Exit codes really do
separate 2 (bad invocation) from 1 (a finding), and I relied on that throughout.

Where it fell short, all reported above:

- `validate` returns exit 0 and `findings: []` for a coordinate that does not exist (E-1) — the single
  most dangerous shape, because it is the one a pipeline trusts.
- `map` emits `kind: group` entries that `map` itself rejects, with an empty `description` (C-3).
- `grep` emits per-line matches, not per-record (C-4).
- A failed `record edit` reports `written: null` rather than `edited: null` (CAP-3).
- `qmd status` reports `installed: true` for a qmd that cannot open its database (I-2).
- Nothing in the JSON distinguishes a qmd search that failed from one that found nothing (S-3).

One nice touch worth naming: `component owners` returns `"owner": null` plus the declared ids rather
than defaulting, which is exactly what a scope check needs, and `skills --emit` returns a `note` field
explaining a qmd/skills mismatch instead of silently changing what it emits.

---

## 7. What is in the store now

`openrecord validate` → `{"findings": []}`. 5 components, 9 decisions, 4 specs, all real.

```
decisions/cli/contracts/machine-readable-output.md      JSON on stdout, coded finding, exit 2 for misuse
decisions/cli/structure/command-tree.md                 a command runs or holds subcommands, never both
decisions/format/structure/two-stores-one-layout.md     two trees; the path carries every axis
decisions/format/runtime/one-finding-vocabulary.md      one finding vocabulary; severity vs code split
decisions/format/delivery/well-formed-not-true.md       checking stops at well-formed (deliberate absence)
decisions/format/data/frontmatter-carries-four-things.md  no date, no repeated axes, no relations
decisions/skills/delivery/emitted-with-a-manifest.md    emit manifest; replace vs keep
decisions/search/integration/optional-and-never-inspected.md  qmd optional; its state never read
decisions/root/delivery/single-binary-piped-installer.md  one static binary, piped installer
decisions/root/structure/five-surfaces.md               the component cut (written by openrecord-capture)

specs/flow/emit-agent-skills.md          [skills, cli]
specs/flow/write-a-record.md             [cli, format]
specs/rule/nothing-partial-lands.md      [cli, format, skills]
specs/lifecycle/emitted-file.md          [skills]
```

No `process` spec: nothing in this repository is triggered by an event rather than by an actor. Recorded
here rather than filed as an empty level, since the format is explicit that an absent section reads as
"does not apply" and an empty one reads as "nobody wrote this yet".

Two scenarios in the specs were written wrong and corrected during the run (CAP-2). The rest were
verified against the tool before being left in place.

## 8. Environment left behind

- `~/.local/bin/openrecord` — installed by the brief's command.
- `.claude/skills/` — five emitted skills plus `.openrecord-emitted.json`.
- `.openrecord/` — the store. Gitignored by this repo (I-4), so untracked.
- Six qmd collections registered in the shared index at `~/.cache/qmd/index.sqlite`, lexically indexed,
  **without vectors** (S-2). Remove with `qmd collection remove <name>` if unwanted.
- The pinned qmd installed to a scratch prefix, not on the default `PATH`. The pre-existing broken
  `~/.bun/bin/qmd 2.5.3` was left exactly as it was.
- Nothing committed, nothing pushed, no file under `internal/`, `cmd/`, `docs/` or `skills/` modified.
