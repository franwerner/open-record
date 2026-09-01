# Testing

What has to be exercised before a release, what already runs on its own, and what only a person can
check.

> Status of this document: settled.

## How to read it

Every box says who checks it:

| Mark | Meaning |
| --- | --- |
| **[auto]** | A test asserts it. Named, so you can run that one. `go test ./...` covers all of them. |
| **[manual]** | Nobody checks it. Somebody has to, and the box says what "passing" looks like. |

A **[manual]** box is not a lesser box. Most of them are here because the thing they check cannot be
automated honestly — it needs a machine that has never seen openrecord, another operating system, a
network that fails, or a person reading prose and deciding whether it is true.

Every finding in `BUGS.md` came from walking this list. Several of them were things a full green suite
said nothing about.

---

## 1. Install

The installer is the one path every user takes and the one thing a green test suite cannot reach: it
downloads a published release onto a machine that has nothing.

### 1.1 The basic path

- [ ] **[manual]** On a machine with no openrecord and no qmd, `curl … | WITH_QMD=no bash` installs the
      binary into `~/.local/bin`, prints the version, and exits 0.
- [ ] **[manual]** `openrecord version` reports the tag that was just released and `store_format: 1`.
- [ ] **[manual]** The install directory is not on `PATH` → the script says so, in one line, and still
      exits 0.
- [ ] **[manual]** `VERSION=vX.Y.Z` installs that release rather than the newest.
- [ ] **[manual]** `INSTALL_DIR=/somewhere/else` puts it there.
- [ ] **[manual]** An unsupported OS or architecture fails with a message naming the releases page,
      not with a tar error.
- [ ] **[manual]** A tag that has no asset for this platform fails saying which asset was missing.

### 1.2 With qmd, in every state qmd can already be in

The check is `qmd_is_usable`, which runs a command that opens the index — **not** `command -v`. A qmd
whose native bindings are missing answers `--version` and dies on everything else, and presence alone
reported that install as fine.

- [ ] **[manual]** **No qmd at all** → installs it, and `openrecord qmd status` afterwards says
      `usable: true` with the pinned version.
- [ ] **[manual]** **qmd present, usable, pinned version** → `qmd is already installed (…)`, installs
      nothing.
- [ ] **[manual]** **qmd present, usable, some other version** → says it is not the pinned one and
      points at `openrecord qmd install --force`. **Does not replace it.** An installer that silently
      downgrades a working tool is worse than one that says something.
- [ ] **[manual]** **qmd present but does not run** → says so and reinstalls over it.
- [ ] **[manual]** **npm absent** → warns, tells you to retry with `openrecord qmd install`, and
      **openrecord is still installed and the script exits 0**. Semantic search is optional; its
      absence is never fatal.
- [ ] **[manual]** **npm present but with no prefix configured** → `npm install -g` hits `EACCES`
      against the system prefix. Same as above: warn, keep going, exit 0. This is the common case on a
      brand-new user account.

### 1.3 The question it asks

- [ ] **[manual]** Piped with `WITH_QMD` unset and no terminal → takes **no** without asking. A piped
      install must never block waiting for an answer.
- [ ] **[manual]** Run from a terminal with `WITH_QMD` unset → asks on `/dev/tty`, and answering `n`
      leaves openrecord installed. It reads the terminal rather than stdin because stdin is the script
      itself.
- [ ] **[manual]** `WITH_QMD=yes` and `WITH_QMD=no` skip the question entirely.

### 1.4 Installing qmd afterwards

The path a user takes when they said no the first time, or when their qmd broke.

- [ ] **[auto]** `TestQmdInstallDoesNotShortCircuitOnABrokenInstall` — a qmd that answers `--version`
      and fails everything else is not reported as "nothing to do".
- [ ] **[auto]** `TestQmdInstallReportsAVersionMismatchAndStops` — a usable, non-pinned qmd is reported
      and left alone; `--force` proceeds.
- [ ] **[manual]** With **no** qmd → installs, then reports the path and version it ended up with (not
      the ones the probe found beforehand).
- [ ] **[manual]** With npm absent → the failure names npm, rather than surfacing whatever npm prints
      when it is not there.
- [ ] **[manual]** After a successful install, the reply says to re-emit skills. **Do that and check
      it reconciles** — see 5.3.
- [ ] **[manual]** The new binary is the one that gets used. `npm install -g` writes to the npm prefix;
      if a different qmd sits earlier on `PATH`, the install "succeeds" and changes nothing that runs.

### 1.5 The pin

- [ ] **[auto]** `TestTheInstallScriptPinsTheSameVersion` — `internal/qmd/qmd.go` and
      `scripts/install.sh` name the same qmd release. Nothing else would notice them drifting: the
      script would install one version while the binary reported a mismatch against the other.
- [ ] **[manual]** After bumping the pin, the tarball URL it composes actually resolves. A pin pointing
      at a release that does not exist breaks every install, and no test can see it.

---

## 2. qmd status: present, usable, and pinned are three questions

- [ ] **[auto]** `TestQmdStatusSeparatesPresentFromUsable` — broken, working and absent are three
      distinct reports.
- [ ] **[auto]** `TestQmdStatusNamesThePinnedVersion` — the report says which release openrecord was
      built against, so a caller can tell they are running a different one.
- [ ] **[auto]** `TestQmdStatusNamesTheCollectionsWithoutQmd` — `collections_needed` is derived from
      `components.json` and is answered whether or not qmd is installed.
- [ ] **[manual]** With no components declared, the report says the collections are not known rather
      than returning an empty list. Empty and unknown are different answers.
- [ ] **[manual]** `status` never claims anything about **whether those collections are registered**.
      That lives in qmd's configuration, in qmd's format, and reading it would break the day it changes.

---

## 3. The store

### 3.1 Writing

- [ ] **[auto]** `TestWarningsOnWriteAreScopedToTheRecord` — a write reports findings about **that
      record** only. A level grown flat, or a surface pointing at a missing directory, come from
      `validate` and from nothing else. `"warnings": []` on a write does not mean the store is clean.
- [ ] **[manual]** The body written to disk is **byte-for-byte the file that was passed**. Read it
      back and compare. A `--body-file` that could not be written — a stale file owned by another user
      under `/tmp` is the real case — is read anyway, and a record lands whose body has nothing to do
      with its description. Nothing catches that.
- [ ] **[manual]** A record's `description` states the decision, not the area it covers. This is the
      property the whole format leans on and the only one no test can judge.

### 3.2 Refusals, and that nothing partial lands

- [ ] **[manual]** A decision body missing one of its four sections → refused, **and no file exists
      afterwards**. Check the directory, not just the exit code.
- [ ] **[manual]** A spec with no `--components` → refused. With an undeclared component → refused.
- [ ] **[manual]** `--status` anything but `accepted` or `pending` → refused.
- [ ] **[manual]** A branch anchored to a step the flow does not have → refused, naming how many steps
      there are.
- [ ] **[manual]** An edit that breaks the record → refused, **and the file is byte-for-byte what it
      was**. Hash it before and after.
- [ ] **[auto]** `TestARejectedEditReportsItselfAsAnEdit` — a failed edit reports under `edited`, not
      under `written`. `"written": null` and an absent key decode identically, so assert on the raw
      keys.
- [ ] **[manual]** `component remove` while anything references the surface → refused, naming what
      holds it, and **nothing is deleted**.

### 3.3 Levels

- [ ] **[auto]** `TestSpecTypesTakeTheirDescriptionFromTheBinary` — the four spec types describe
      themselves; no project invents wording for them.
- [ ] **[auto]** `TestASubgroupStillHasToBeDescribed` — a subgroup has no default and the refusal says
      so, rather than listing the concerns, which are a different axis.
- [ ] **[manual]** A concern the catalogue knows reports `source: catalogue`; a spec type reports
      `shipped`; anything else reports `given`. The reply says which, so a caller can tell prose it
      supplied from prose it was given.

---

## 4. Navigation

- [ ] **[auto]** `TestMapAndValidateAgreeOnEveryCoordinate` — the two commands take the same `--for`,
      so a coordinate one accepts is one the other accepts. Where they disagreed, `validate` reported a
      clean store for a component somebody had renamed.
- [ ] **[auto]** `TestValidateRefusesACoordinateThatDoesNotExist`
- [ ] **[auto]** `TestEveryGroupMapAdvertisesCanBeDescendedInto` — walks the whole tree and opens
      everything marked `kind: group`. Picking the cases by hand is how an unnavigable spec type
      survived.
- [ ] **[auto]** `TestASpecTypeWithNothingInItIsStillNavigable`
- [ ] **[auto]** `TestACoordinateNamingARecordSaysSo` — feeding a search hit straight back to `map` is
      the likeliest mistake there is, and the message names the level holding it.
- [ ] **[auto]** `TestGrepReportsEachRecordOnce` — one entry per file with a `hits` count, not one per
      line.
- [ ] **[auto]** `TestEveryGrepHitCanBeActedOn` — every hit's parent is a coordinate `map` opens.
- [ ] **[auto]** `TestOwnersRoutesToTheSpecsThatCoverAPath` — both halves of what governs a file: the
      decisions coordinate, and every capability naming that surface.
- [ ] **[auto]** `TestOwnersReportsAnUnclaimedPathWithoutGuessing`

---

## 5. Skills

### 5.1 What ships

- [ ] **[auto]** `TestEverySkillNameIsNamespaced` — an unprefixed name collides silently in a shared
      skills directory, and the loser's behaviour just stops being available.
- [ ] **[auto]** `TestSkillsOnlyInvokeCommandsThatExist` — no skill tells an agent to run a command the
      binary does not have.
- [ ] **[auto]** `TestSkillsInvokeQmdCommandsThatExist` — same for `qmd`, against a hand-maintained
      list, because this binary cannot ask qmd what it supports.
- [ ] **[auto]** `TestSkillsShowTheFlagsTheirExamplesNeed` — an example that omits a mandatory flag is
      an example an agent will follow into a failure.
- [ ] **[auto]** `TestSkillsNameNoPathOfThisRepository` — a skill naming `internal/…` sends an agent
      looking for a directory that exists here and nowhere else. It does not distinguish an example
      from an instruction, and it should not.

### 5.2 That the prose is true

None of this is automatable. It is also where every skill finding came from.

- [ ] **[manual]** Every claim a skill makes about the tool is **checked against the tool**, not
      remembered. `openrecord-consult` claimed a grep hit is a `map` coordinate; it is a file.
- [ ] **[manual]** A skill that names a workflow names **every command that workflow needs**.
      `openrecord-setup-search` specified collection names, masks and paths, and named no registration
      command at all.
- [ ] **[manual]** A rule a skill states can actually be carried out **in the order the skill gives**.
      Its own "check the provider before registering" was not executable: the failure only appears
      after registering.
- [ ] **[manual]** Where two skills state the same rule, they state it the same way. Three of them gave
      three different answers on who confirms a record.

### 5.3 Emitting

- [ ] **[auto]** `TestDryRunReportsThePlanAndTouchesNothing` — the plan matches what a real run does,
      and the directory is unchanged.
- [ ] **[auto]** `TestEmitReportsAQmdMismatch` — all four cases: asked for qmd with none installed,
      qmd installed but emitted without, neither, and previously emitted with it.
- [ ] **[auto]** `TestWhatIsEmittedDoesNotDependOnWhatIsInstalled` — the flag decides what is emitted,
      never what happens to be on the machine. Otherwise the same command produces different files on
      different machines.
- [ ] **[manual]** The **file lifecycle**, which needs a second build to see properly:
      - a still-shipped file with local edits → **overwritten**, and the note says these are generated
      - a dropped file that is untouched → **removed**
      - a dropped file with local edits → **kept**, and it is never reclaimed by a later emit
      - a file no emit ever wrote → never touched, never reported
- [ ] **[manual]** After installing qmd later, re-emitting with `--with-qmd` turns the stripped
      passages back into `updated` files and brings `openrecord-setup-search` along.

### 5.4 That the fixes reach the user

The skills are compiled into the binary. Correct prose in `skills/` proves nothing about what a user
gets.

- [ ] **[manual]** Emit from the **built binary**, into a project, and read the file on disk. Assert
      the change you made is in the emitted copy — not in the source.

---

## 6. Diagrams

- [ ] **[auto]** `TestLifecycleDiagramIsValidMermaid` — a multi-word state emits one pair of quotes.
      Two make mermaid refuse the whole diagram.
- [ ] **[auto]** `TestFlowDiagramKeepsWrappedProse` — every markdown file here wraps at about a hundred
      columns, so a step spanning two lines is the normal case, not an exotic one.
- [ ] **[auto]** `TestFlowDiagramNumbersBranchesConsecutively`
- [ ] **[auto]** `TestProcessDiagramStartsFromItsTrigger`
- [ ] **[auto]** `TestRuleHasNoDiagram`, `TestDiagramRefusesADecision`
- [ ] **[manual]** **Paste the output into a mermaid renderer.** Nothing in this repository parses
      mermaid, so a diagram that is subtly invalid passes every test and fails at the only moment that
      matters.

---

## 7. Semantic search

Needs qmd, an embedding provider, and credentials. None of it runs in CI.

### 7.1 Setting it up from nothing

- [ ] **[manual]** A fresh qmd is **not configured for a hosted provider**: it starts on local models
      it has not downloaded, and the first embed fails with *"Failed to get embedding dimensions from
      first chunk"*. Either `qmd pull`, or point it at an API.
- [ ] **[manual]** The **index config beats the environment**, and `collection add` writes the local
      defaults into it. A model set only by environment variable stops taking effect the moment the
      first collection is registered — silently. Set it in the file, before registering.
- [ ] **[manual]** Credentials live in the environment, never in a file that gets committed.
- [ ] **[manual]** Collections are registered under the names `openrecord qmd status` gives, with
      absolute paths and `**/*.md`, and the `INDEX.md` files are **included** — a hit on one means
      *descend here*.
- [ ] **[manual]** `qmd embed`, then `qmd status` shows **`Pending: 0`**. A run that ends with
      documents pending is the half-built state: searches then answer over a fraction of the store and
      nothing announces the gap.

### 7.2 That it answers, and answers the right thing

- [ ] **[manual]** Ask something whose wording appears **nowhere** in the store. `openrecord grep` for
      the same words returns nothing; the semantic search returns the record that answers it. Assert
      **which record came back**, not that something did — a search that returns anything at all will
      pass a weaker check over a store full of the wrong content.
- [ ] **[manual]** A query scoped to one component returns that component's records and not another's.
      That isolation is the reason decisions are one collection per component.
- [ ] **[manual]** Several `-c` flags merge results: breadth is a flag, not a second search.
- [ ] **[manual]** A `qmd://` result is **not** an openrecord coordinate. Translating it back — drop the
      scheme, read the surface out of the collection name, drop the line number — lands on a real file
      whose parent `map` opens.

### 7.3 When the provider is gone

The failure that matters most, because it looks like an ordinary answer.

- [ ] **[manual]** With an invalid or expired key: `qmd vsearch` **exits 1** and says
      `Search did not run.` — not `No results found.`
- [ ] **[manual]** Same with `--format json`: **exit 1**. The JSON body carries no error field, so the
      exit status is the only machine-readable signal there is.
- [ ] **[manual]** `qmd doctor` **exits 1** and names what the provider said.
- [ ] **[manual]** A search that genuinely finds nothing, with a working provider, still **exits 0**.
      This is the one that must not break: it is what separates the two states.
- [ ] **[manual]** `qmd doctor` on an index with nothing embedded says the provider was **not
      exercised** rather than reporting it healthy. Do not stop on that: on a fresh index it is the
      honest answer, and the embed is what proves the provider.

---

## 8. End to end, as somebody who has never seen it

The three levels below find different things. The third found what the first two could not.

### 8.1 A throwaway HOME

- [ ] **[manual]** `env -i`, a `HOME` of its own, a `PATH` of its own. Catches anything that leans on
      the developer's shell.

### 8.2 A real Unix user

- [ ] **[manual]** `useradd`, then run everything as them. Catches what a fake `HOME` cannot: npm with
      no prefix, file permissions, `/tmp` collisions with another user's leftovers, and a `PATH` that
      genuinely starts empty.

`e2e/` in this repository drives the built binary against a realistic project and is the automated
half of this. It is not a substitute for the two above: it runs as whoever is developing.

### 8.3 An agent, given nothing but the skills

- [ ] **[manual]** Point a fresh agent at a project with the skills emitted and a task in a user's
      words — **naming no command**. Then read what it produced.

This is the only test that reads the skills as a consumer rather than as their author. It found that
mined records named file paths in their prose, which the format forbids, which `validate` cannot see,
and which every other test on this page passed straight over.

Judge the **content**, not the exit code: are the records true, is the reasoning quoted rather than
invented, is an absent section absent rather than filled with "None recorded."

---

## 9. Before a release

- [ ] **[auto]** `go vet ./...`, `go test ./...`, and `gofmt -l .` clean. Run them **unfiltered** — a
      `| head` has hidden a real failure here before.
- [ ] **[auto]** `TestEveryCommandIsDocumented`, `TestDocumentedFlagsExist` — `docs/cli.md` ships in the
      release archive, so it is what a person reads.
- [ ] **[auto]** `./e2e/seed.sh --check` — the checked-in fixture is still what the commands produce.
- [ ] **[manual]** Every commit since the last tag builds and passes on its own, if history matters to
      you. `git worktree add` per commit is the cheap way.
- [ ] **[manual]** Section 1 of this document, against the **published** artifact, after the tag lands.
      Until a release exists, `latest` serves the previous one and every new install gets it.

---

## What nothing here covers

Named so that a green run is not mistaken for more than it is.

- **macOS and Windows.** Every box above was walked on Linux. The build targets darwin and windows and
  nobody has run them.
- **arm64.** Same.
- **The interactive install prompt.** It reads `/dev/tty`, which is exactly what a test harness does
  not have.
- **Local embedding models.** Everything in section 7 was exercised against a hosted API. The local
  path — `qmd pull`, GGUF models, CPU or GPU — is untested here.
- **A rendered diagram.** Mermaid is never parsed by anything in this repository.
- **Concurrency.** Two processes writing one store, or one qmd index shared by several projects.
- **Whether a record is still true of the code.** By design, and stated in the format: `validate`
  checks well-formed and never still-true. Judging that is reading, and it is nobody's automated job.
