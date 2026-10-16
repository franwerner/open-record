# The verification catalogue

A second document of the same kind as [`concerns.md`](concerns.md): it ships with the binary and
**never enters a project**. Where that one is a tunnel of questions for *what was decided*, this one is
a tunnel of questions for *how would anyone know the code still does it*.

Every record — a functional spec, a `quality` spec, a decision — makes a claim. Writing one without
asking how it would be checked is how a spec ends up saying "fast" with no threshold, or a decision
ends up saying "hexagonal architecture" with no rule that could ever fail if someone broke it. This
catalogue exists to be walked through at the moment a record is written, so the claim comes out
verifiable from the start — and to be walked through against a real store, so the records that are
missing become visible instead of silently assumed.

**It names no tool to use.** Every tool named here — `go test`, `k6`, `go-arch-lint`, `axe` — is an
example, never a choice. Picking one is a decision a person makes about a real project; this document
is read, not executed.

## Three gates

A record being defined is always one of three kinds, and that is what decides which gate applies:

| Gate | Keyed by | Example |
| --- | --- | --- |
| 1 — Functional specs | the spec type and the aspect being verified | `lifecycle/transitions` → a test per valid and per forbidden transition |
| 2 — `quality` specs | the quality attribute | `latency` → a load test with percentiles, landing `pre-release` |
| 3 — Decisions | the `concerns.md` topic | `security/rate-limiting` → an integration test of the limit, landing `pre-merge` |

**Gate 3 carries no `Question` field.** The question for each topic already lives in `concerns.md`;
repeating it here would duplicate it across sixty entries. Everything else about an entry — what it
binds to, what the record has to say, the mechanism, where it lands, its class — is still here.

> **Format.** Every entry is a self-contained block: a `### <key>` heading followed by its fields as a
> bullet list, bolded the same way a scenario's `GIVEN`/`WHEN`/`THEN` is in a [capability
> spec](capability-spec.md). `openrecord verification <key>` slices a block out of this file byte for
> byte — it is never rebuilt from parsed fields, for the same reason `openrecord concerns` prints
> `concerns.md` verbatim: the document is the deliverable, and reassembling it risks drifting from what
> actually ships. A line inside the catalogue's own prose, outside any `### ` block, is read by a
> person, never sliced.

## Where it lands

Five values, and every entry carries exactly one. They are **the same vocabulary everywhere a landing
is said** — `VERIFIED-IN` on a `quality` spec ([capability-spec.md](capability-spec.md#VERIFIED-IN))
only ever uses three of them, and the checks registry (task 09) will deduce two of them from whether a
command's `checks.json` entry carries `{files}` rather than naming a fifth value of its own:

| Value | Means | Checks registry (task 09) |
| --- | --- | --- |
| `on-change` | A check run on the files a change touched | A `checks.json` command with `{files}` |
| `pre-merge` | A check over the whole project, before it integrates | A `checks.json` command with no `{files}` |
| `pre-release` | An automated measurement against a non-real environment, before users reach the change | Not a check at all today — a load test, Lighthouse on a preview, a staging scan, a periodic dependency audit. A sandbox (task 11) running locally, `approximates`-linked, is one way to run that load test on a developer's own machine — it detects a regression and catches gross problems, but it never certifies the number that landing `pre-release` would usually imply: see [decision-record.md](decision-record.md#sandbox) |
| `production` | Measured against the real system, with real users | An SLO or a field metric; not a check `checks.json` runs |
| `manual` | A person looks, by hand | Never a check: `checks.json` only holds what code can verify. A `quality` spec says it with `VERIFIED-IN manual`; a decision or a functional spec says how it is reviewed in its own prose |

This is also the old draft's five stages renamed, not a new idea: `edit` became `on-change`; `pr` and
`main` both became `pre-merge`, because both are a whole-project gate before code is on the trunk;
`release` and `scheduled` both became `pre-release`, because both measure against something that is not
yet, or not only, production; `production` kept its name; `human` became `manual`.

## Three classes

Every entry also carries exactly one of three classes — what a flow that implements a task actually
*does* with the entry, reported **separately** in every case, so nothing reads as "it passed" when a
person judged it, or as "nobody checked" when a tool actually ran:

| Class | Means | How it was assigned below |
| --- | --- | --- |
| `full-check` | A tool fails outright if the record's claim is violated | The mechanism is deterministic or statistical with a clear pass/fail, and nothing in it names a person |
| `partial-check+audit` | The mechanism catches the mechanical part; a broader review judges the rest | The mechanism's own description says `+ audit`, `+ review`, or pairs an automatic half with a human one |
| `manual-only` | No mechanism can fail reliably on its own | The mechanism is a person, full stop (usability is the one case in this whole catalogue) |

## Gate 1 — Functional specs, by spec type and aspect

The unit being verified is the **scenario**. Whether that test runs unit, integration, or end to end is
`delivery/testing-strategy`'s call for a given project, never this catalogue's.

### flow/main-flow
- **Question** Does the happy path produce the promised result?
- **Binds to** a `flow` spec
- **To be verifiable, the record says** numbered steps, each one an observable action, plus a scenario
  that walks all of them
- **Mechanism** an end-to-end test of the flow's scenario
- **Where it lands** pre-merge
- **Class** full-check
- **Examples** go test, Vitest, Playwright

### flow/branches
- **Question** Does every branch lead to the result it names?
- **Binds to** a `flow` spec
- **To be verifiable, the record says** each branch anchored to its step (`**[n] condition**`), with its
  own scenario
- **Mechanism** one test per branch scenario
- **Where it lands** pre-merge
- **Class** full-check

### flow/edge-cases
- **Question** Do the limits behave the way the spec says they do?
- **Binds to** a `flow` spec
- **To be verifiable, the record says** the limit with a concrete value ("a file over 50 MB")
- **Mechanism** one test per edge case; property-based when the limit is a range rather than a single
  value
- **Where it lands** pre-merge
- **Class** full-check
- **Examples** rapid, fast-check

### flow/errors
- **Question** Does the actor get exactly what was promised when this fails?
- **Binds to** a `flow` spec
- **To be verifiable, the record says** the observable error — a code, a message, a state — never the
  internal exception that produced it
- **Mechanism** a test that forces the failure and asserts on the response
- **Where it lands** pre-merge
- **Class** full-check

### process/trigger
- **Question** Does the event trigger the process, and only that event?
- **Binds to** a `process` spec
- **To be verifiable, the record says** `## Trigger` naming the concrete event, plus `initiated-by`
- **Mechanism** an integration test that emits the event
- **Where it lands** pre-merge
- **Class** full-check
- **Examples** Testcontainers

### process/repetition
- **Question** Is receiving the same event twice safe?
- **Binds to** a `process` spec
- **To be verifiable, the record says** the duplicate-delivery edge case, with its result ("it is
  ignored")
- **Mechanism** a test that delivers the event twice
- **Where it lands** pre-merge
- **Class** full-check

### lifecycle/transitions
- **Question** Do only the declared transitions ever happen?
- **Binds to** a `lifecycle` spec
- **To be verifiable, the record says** one transition per line, each with its trigger
- **Mechanism** one test per valid transition and one per forbidden one
- **Where it lands** pre-merge
- **Class** full-check

### lifecycle/machine-shape
- **Question** Is there an unreachable state, or one with no way out?
- **Binds to** a `lifecycle` spec
- **To be verifiable, the record says** a complete `## States and transitions` section
- **Mechanism** `openrecord validate` (`unreachable-state`, `dead-end-state`)
- **Where it lands** on-change
- **Class** full-check
- **Examples** already shipped — this is the one entry in the whole catalogue the binary already
  enforces

### rule
- **Question** Does the rule hold for every valid input, not only the ones someone thought to try?
- **Binds to** a `rule` spec
- **To be verifiable, the record says** the rule stated with concrete values, in `## Rule`
- **Mechanism** a property-based test over the invariant
- **Where it lands** pre-merge
- **Class** full-check
- **Examples** rapid, fast-check

### all/coverage
- **Question** Does every scenario have a test?
- **Binds to** any functional spec
- **To be verifiable, the record says** named scenarios — each one is an anchor, which is what makes it
  something a test can point back at
- **Mechanism** cross-referencing a test's `@record:` annotation against the spec's own anchors
- **Where it lands** pre-merge
- **Class** full-check
- **Examples** the openrecord binary itself, once that cross-reference ships

### all/test-quality
- **Question** Do the tests actually catch a real bug, or only run the code without asserting anything
  that matters?
- **Binds to** any functional spec
- **To be verifiable, the record says** nothing beyond the scenario itself — this entry is about the
  tests that cover it, not about anything the record adds
- **Mechanism** mutation testing over the code the spec covers, run as a slow pass over the whole
  project rather than the files a change touched. That shape is also why it lands at `pre-merge` and not
  `on-change`: `on-change` means a command built around `{files}`, and a project-wide mutation run is
  not one. It cannot borrow a `VERIFIED-IN` line either, because that line only exists on a `quality`
  spec — a functional spec has nowhere else to say where this lands, so this entry is where it has to be
  said
- **Where it lands** pre-merge
- **Class** partial-check+audit — the score is mechanical, but which surviving mutant is actually
  dangerous is a judgement call
- **Examples** go-mutesting, Stryker

## Gate 2 — Quality specs, by attribute

A `quality` scenario's shape is `GIVEN`/`WHEN`/`THEN` plus `MEASURE` (and, often, `VERIFIED-IN`). What
blocks a merge has to be deterministic; almost everything statistical here runs at `pre-release`, not
`pre-merge`, for exactly that reason.

### latency
- **Question** How long can this operation take, measured how, under what load?
- **Binds to** a `quality` spec
- **To be verifiable, the record says** the operation, a percentile, a threshold, and the load it was
  measured under
- **Mechanism** a load test measuring percentiles
- **Where it lands** pre-release
- **Class** full-check
- **Examples** k6, vegeta

### throughput
- **Question** How much load does this sustain?
- **Binds to** a `quality` spec
- **To be verifiable, the record says** operations per second, a duration, and an error-rate threshold
- **Mechanism** a sustained load test
- **Where it lands** pre-release
- **Class** full-check
- **Examples** k6

### availability
- **Question** What fraction of the time is this available?
- **Binds to** a `quality` spec
- **To be verifiable, the record says** a percentage, a window, and what counts as being down
- **Mechanism** an SLO measured through observability
- **Where it lands** production
- **Class** full-check
- **Examples** Prometheus, Grafana

### durability
- **Question** Can a confirmed piece of data be lost?
- **Binds to** a `quality` spec
- **To be verifiable, the record says** which operations must never be lost, and under which failure
- **Mechanism** a test with an injected failure
- **Where it lands** pre-release
- **Class** full-check
- **Examples** Toxiproxy, Chaos Mesh

### recovery-point
- **Question** How much data can a disaster lose — the RPO?
- **Binds to** a `quality` spec
- **To be verifiable, the record says** the RPO, with a concrete value
- **Mechanism** a restore drill, usually with a person confirming the data that came back is within the
  stated bound
- **Where it lands** pre-release
- **Class** partial-check+audit
- **Examples** project-specific scripts

### recovery-time
- **Question** How long can a disaster take to recover from — the RTO?
- **Binds to** a `quality` spec
- **To be verifiable, the record says** the RTO, with a concrete value
- **Mechanism** a restore drill, usually with a person confirming the system was back within the stated
  bound. Kept as its own entry rather than folded into `recovery-point`: an amount of data and a span of
  time are different claims, each with its own threshold, even though the same drill usually proves both
- **Where it lands** pre-release
- **Class** partial-check+audit
- **Examples** project-specific scripts

### resilience
- **Question** What happens when a dependency fails or goes slow?
- **Binds to** a `quality` spec
- **To be verifiable, the record says** the dependency, the kind of failure, and the expected result
- **Mechanism** injected failures
- **Where it lands** pre-release
- **Class** full-check
- **Examples** Toxiproxy

### resource-usage
- **Question** How much memory or CPU can this use?
- **Binds to** a `quality` spec
- **To be verifiable, the record says** the operation, the resource, and a threshold
- **Mechanism** a benchmark; deterministic when the threshold is generous, statistical when it is tight
- **Where it lands** pre-merge
- **Class** full-check
- **Examples** `go test -bench -benchmem`

### artifact-size
- **Question** How large can the binary or the bundle be?
- **Binds to** a `quality` spec
- **To be verifiable, the record says** the artifact and its maximum size
- **Mechanism** measuring the artifact
- **Where it lands** pre-merge
- **Class** full-check
- **Examples** size-limit

### startup-time
- **Question** How long until this is ready?
- **Binds to** a `quality` spec
- **To be verifiable, the record says** the event it starts from, the signal it ends at, and a threshold
- **Mechanism** repeated measurement
- **Where it lands** pre-release
- **Class** full-check
- **Examples** project-specific scripts

### web-performance
- **Question** How long until a page is usable?
- **Binds to** a `quality` spec
- **To be verifiable, the record says** the page, a metric (LCP, INP, CLS), and a threshold
- **Mechanism** a lab measurement before release, corroborated by field data once it is live
- **Where it lands** pre-release
- **Class** full-check
- **Examples** Lighthouse, Web Vitals

### data-freshness
- **Question** How old can a piece of data be, from what the user sees?
- **Binds to** a `quality` spec
- **To be verifiable, the record says** the data and its maximum age
- **Mechanism** a test of the invalidation path, corroborated by a production measurement
- **Where it lands** pre-merge
- **Class** full-check

### accessibility
- **Question** Does this meet its accessibility bar?
- **Binds to** a `quality` spec
- **To be verifiable, the record says** a level (for example WCAG AA) and the scope it applies to
- **Mechanism** an automated scan, plus a manual review for what the scan cannot see
- **Where it lands** pre-merge
- **Class** partial-check+audit
- **Examples** axe

### usability
- **Question** Who confirms this is understood and used the way it is meant to be, and against what?
- **Binds to** a `quality` spec
- **To be verifiable, the record says** who reviews, what criterion they use, and what task they check
  against
- **Mechanism** a review, or a test with real users — the one attribute in this whole catalogue with no
  automatic mechanism at all
- **Where it lands** manual
- **Class** manual-only

### compatibility
- **Question** On which browsers, systems, or versions does this have to work?
- **Binds to** a `quality` spec
- **To be verifiable, the record says** the matrix of supported platforms
- **Mechanism** tests run across that matrix
- **Where it lands** pre-merge
- **Class** full-check
- **Examples** Playwright, a CI build matrix

### security-exposure
- **Question** How long can a known vulnerability stay open?
- **Binds to** a `quality` spec
- **To be verifiable, the record says** a severity and a maximum time to fix
- **Mechanism** a continuous scan
- **Where it lands** pre-release
- **Class** full-check
- **Examples** govulncheck, osv-scanner

## Gate 3 — Decisions, by concern topic

One entry per topic of the revised [`concerns.md`](concerns.md) — exactly sixty, grouped the same way
that document groups them. Many decisions have no automatic mechanism at all, and the entry says so
plainly: its verification stays `manual`, or a `partial-check+audit` class, rather than a silent gap
nobody can see.

**structure**

### structure/architecture-style
- **Binds to** the decision at `structure/architecture-style`
- **To be verifiable, the record says** the style, and exactly what it forbids
- **Mechanism** dependency rules, plus an audit for what a rule cannot phrase mechanically
- **Where it lands** pre-merge
- **Class** partial-check+audit
- **Examples** go-arch-lint, ArchUnit

### structure/code-conventions
- **Binds to** the decision at `structure/code-conventions`
- **To be verifiable, the record says** the rules as forbidden or required constructs
- **Mechanism** a pattern linter
- **Where it lands** on-change
- **Class** full-check
- **Examples** golangci-lint, ESLint, semgrep

### structure/folder-structure
- **Binds to** the decision at `structure/folder-structure`
- **To be verifiable, the record says** where each kind of file goes
- **Mechanism** an allowed-paths rule
- **Where it lands** pre-merge
- **Class** full-check
- **Examples** a project-specific script, ls-lint

### structure/inter-layer-communication
- **Binds to** the decision at `structure/inter-layer-communication`
- **To be verifiable, the record says** the mechanism the parts use to talk to each other
- **Mechanism** import rules, plus an audit for anything a rule cannot phrase mechanically
- **Where it lands** pre-merge
- **Class** partial-check+audit
- **Examples** dependency-cruiser

### structure/layers-and-dependencies
- **Binds to** the decision at `structure/layers-and-dependencies`
- **To be verifiable, the record says** the layers by name, and the direction allowed between them
- **Mechanism** an architecture linter
- **Where it lands** pre-merge
- **Class** full-check
- **Examples** go-arch-lint, dependency-cruiser, import-linter

### structure/code-health
- **Binds to** the decision at `structure/code-health`
- **To be verifiable, the record says** the thresholds, with values
- **Mechanism** measuring against the threshold
- **Where it lands** on-change
- **Class** full-check
- **Examples** gocyclo, gocognit, jscpd, deadcode, knip

### structure/language-and-runtime
- **Binds to** the decision at `structure/language-and-runtime`
- **To be verifiable, the record says** the language and the minimum version
- **Mechanism** a pinned version, checked
- **Where it lands** pre-merge
- **Class** full-check
- **Examples** `go.mod`, `.nvmrc`, mise

### structure/dependency-policy
- **Binds to** the decision at `structure/dependency-policy`
- **To be verifiable, the record says** the criterion — licences included — and the allowed list, if one
  exists
- **Mechanism** diffing dependencies against the policy, plus a licence check
- **Where it lands** pre-merge
- **Class** full-check
- **Examples** a script over `go.mod` / `package.json`, go-licenses, license-checker

### structure/shared-code
- **Binds to** the decision at `structure/shared-code`
- **To be verifiable, the record says** what is shared, and where it lives
- **Mechanism** an import rule toward the shared code
- **Where it lands** pre-merge
- **Class** full-check
- **Examples** dependency-cruiser

### structure/code-generation
- **Binds to** the decision at `structure/code-generation`
- **To be verifiable, the record says** what is generated, from what, and whether it is committed
- **Mechanism** regenerating and diffing — a clean diff is the pass
- **Where it lands** pre-merge
- **Class** full-check
- **Examples** `go generate` + `git diff --exit-code`

**domain-logic**

### domain-logic/aggregates
- **Binds to** the decision at `domain-logic/aggregates`
- **To be verifiable, the record says** what changes together, and what does not
- **Mechanism** access rules between packages, plus an audit for what a rule cannot phrase mechanically
- **Where it lands** pre-merge
- **Class** partial-check+audit

### domain-logic/invariants
- **Binds to** the decision at `domain-logic/invariants`
- **To be verifiable, the record says** where the invariant is enforced
- **Mechanism** a pattern rule that nothing may bypass, plus the `rule` spec's own property-based test
- **Where it lands** pre-merge
- **Class** full-check
- **Examples** semgrep

### domain-logic/domain-vocabulary
- **Binds to** the decision at `domain-logic/domain-vocabulary`
- **To be verifiable, the record says** the chosen terms, and the synonyms forbidden alongside them
  ("customer", never "client")
- **Mechanism** an identifier linter forbidding the synonyms, plus a review that the vocabulary itself
  is the right one
- **Where it lands** pre-merge
- **Class** partial-check+audit
- **Examples** semgrep, a regular expression over identifiers

### domain-logic/i18n-l10n
- **Binds to** the decision at `domain-logic/i18n-l10n`
- **To be verifiable, the record says** how text, formats, time zones, and currencies are handled
- **Mechanism** a check for untranslated text, plus format tests
- **Where it lands** pre-merge
- **Class** full-check
- **Examples** i18n linters

**runtime**

### runtime/background-jobs
- **Binds to** the decision at `runtime/background-jobs`
- **To be verifiable, the record says** the mechanism, and what happens to a job that fails
- **Mechanism** an integration test of the failed job
- **Where it lands** pre-merge
- **Class** full-check
- **Examples** Testcontainers

### runtime/caching
- **Binds to** the decision at `runtime/caching`
- **To be verifiable, the record says** what is cached, where, and how it is invalidated
- **Mechanism** a test of the invalidation path
- **Where it lands** pre-merge
- **Class** full-check

### runtime/concurrency-async
- **Binds to** the decision at `runtime/concurrency-async`
- **To be verifiable, the record says** how concurrent work is coordinated
- **Mechanism** a race detector
- **Where it lands** pre-merge
- **Class** full-check
- **Examples** `go test -race`

### runtime/error-handling
- **Binds to** the decision at `runtime/error-handling`
- **To be verifiable, the record says** how errors are modelled and translated
- **Mechanism** a pattern rule, plus an audit for what a rule cannot phrase mechanically
- **Where it lands** pre-merge
- **Class** partial-check+audit
- **Examples** semgrep, errorlint

### runtime/resilience
- **Binds to** the decision at `runtime/resilience`
- **To be verifiable, the record says** timeouts, retries, and circuit-breaking, with values
- **Mechanism** injected failures
- **Where it lands** pre-release
- **Class** full-check
- **Examples** Toxiproxy

### runtime/state-management
- **Binds to** the decision at `runtime/state-management`
- **To be verifiable, the record says** where state lives, and who owns it
- **Mechanism** import rules, plus an audit for what a rule cannot phrase mechanically
- **Where it lands** pre-merge
- **Class** partial-check+audit

### runtime/scaling
- **Binds to** the decision at `runtime/scaling`
- **To be verifiable, the record says** the growth axis, and what it rules out
- **Mechanism** a pattern rule, plus a load test
- **Where it lands** pre-merge
- **Class** full-check
- **Examples** semgrep, k6

**data**

### data/data-access
- **Binds to** the decision at `data/data-access`
- **To be verifiable, the record says** which layer talks to the data source
- **Mechanism** an architecture linter — nobody else may import the driver
- **Where it lands** pre-merge
- **Class** full-check
- **Examples** go-arch-lint

### data/data-modeling
- **Binds to** the decision at `data/data-modeling`
- **To be verifiable, the record says** how the domain is represented in storage, and how schema change
  is handled
- **Mechanism** a migration linter
- **Where it lands** pre-merge
- **Class** full-check
- **Examples** squawk, atlas lint

### data/data-stores
- **Binds to** the decision at `data/data-stores`
- **To be verifiable, the record says** which kinds of storage exist
- **Mechanism** a diff of dependencies and infrastructure
- **Where it lands** pre-merge
- **Class** full-check
- **Examples** a project-specific script

### data/tenancy
- **Binds to** the decision at `data/tenancy`
- **To be verifiable, the record says** how different customers' data is isolated, or that it
  deliberately is not
- **Mechanism** a cross-tenant isolation test, plus an audit for what a test cannot phrase mechanically
- **Where it lands** pre-merge
- **Class** partial-check+audit

**data-lifecycle**

### data-lifecycle/retention
- **Binds to** the decision at `data-lifecycle/retention`
- **To be verifiable, the record says** how the retention period is enforced
- **Mechanism** a test of the cleanup job
- **Where it lands** pre-merge
- **Class** full-check

### data-lifecycle/deletion
- **Binds to** the decision at `data-lifecycle/deletion`
- **To be verifiable, the record says** whether deletion is physical, logical, or cascaded
- **Mechanism** a deletion test
- **Where it lands** pre-merge
- **Class** full-check

### data-lifecycle/backup-and-restore
- **Binds to** the decision at `data-lifecycle/backup-and-restore`
- **To be verifiable, the record says** the mechanism, and the frequency
- **Mechanism** a restore drill, usually with a person confirming the restore actually worked
- **Where it lands** pre-release
- **Class** partial-check+audit
- **Examples** project-specific scripts

### data-lifecycle/archival
- **Binds to** the decision at `data-lifecycle/archival`
- **To be verifiable, the record says** what leaves the hot path, and how it is reached afterwards
- **Mechanism** an archival test
- **Where it lands** pre-merge
- **Class** full-check

**contracts**

### contracts/api-contract
- **Binds to** the decision at `contracts/api-contract`
- **To be verifiable, the record says** the shared conventions, and the breaking-change policy
- **Mechanism** a schema linter, plus a breaking-change detector
- **Where it lands** pre-merge
- **Class** full-check
- **Examples** Spectral, oasdiff, `buf breaking`

### contracts/cli-contract
- **Binds to** the decision at `contracts/cli-contract`
- **To be verifiable, the record says** the conventions for flags, output, and exit codes
- **Mechanism** golden-file tests over the output
- **Where it lands** pre-merge
- **Class** full-check

### contracts/event-contract
- **Binds to** the decision at `contracts/event-contract`
- **To be verifiable, the record says** the conventions, and the compatibility rules for events
- **Mechanism** a schema registry with a compatibility check
- **Where it lands** pre-merge
- **Class** full-check
- **Examples** buf, a schema registry

### contracts/library-contract
- **Binds to** the decision at `contracts/library-contract`
- **To be verifiable, the record says** what is exposed, and the compatibility promise attached
- **Mechanism** a detector of changes to the public API
- **Where it lands** pre-merge
- **Class** full-check
- **Examples** apidiff, api-extractor

**integration**

### integration/third-party-clients
- **Binds to** the decision at `integration/third-party-clients`
- **To be verifiable, the record says** where each provider's API is confined
- **Mechanism** an architecture linter — the SDK is only imported from its own adapter
- **Where it lands** pre-merge
- **Class** full-check
- **Examples** go-arch-lint

### integration/anti-corruption
- **Binds to** the decision at `integration/anti-corruption`
- **To be verifiable, the record says** where the foreign vocabulary is translated
- **Mechanism** import rules, plus an audit for what a rule cannot phrase mechanically
- **Where it lands** pre-merge
- **Class** partial-check+audit

### integration/inbound-webhooks
- **Binds to** the decision at `integration/inbound-webhooks`
- **To be verifiable, the record says** how an inbound event is verified as genuine
- **Mechanism** a test with an invalid signature, plus a duplicate-delivery test
- **Where it lands** pre-merge
- **Class** full-check

### integration/messaging
- **Binds to** the decision at `integration/messaging`
- **To be verifiable, the record says** what is actually relied on about ordering and delivery
- **Mechanism** contract tests against the broker
- **Where it lands** pre-merge
- **Class** full-check
- **Examples** Pact, Testcontainers

**security**

### security/auth
- **Binds to** the decision at `security/auth`
- **To be verifiable, the record says** how identity is established and carried
- **Mechanism** SAST, plus a test of unauthenticated routes
- **Where it lands** pre-merge
- **Class** full-check
- **Examples** gosec, semgrep

### security/authorization
- **Binds to** the decision at `security/authorization`
- **To be verifiable, the record says** where permissions are enforced
- **Mechanism** a pattern rule (every handler goes through the check), plus one test per actor
- **Where it lands** pre-merge
- **Class** full-check
- **Examples** semgrep

### security/cors
- **Binds to** the decision at `security/cors`
- **To be verifiable, the record says** which origins are allowed
- **Mechanism** a configuration test
- **Where it lands** pre-merge
- **Class** full-check

### security/dependency-scanning
- **Binds to** the decision at `security/dependency-scanning`
- **To be verifiable, the record says** how risk in third-party code is detected, and what happens when
  something is found
- **Mechanism** a vulnerability scan of the code, the images, and the SBOM
- **Where it lands** pre-merge
- **Class** full-check
- **Examples** govulncheck, osv-scanner, `npm audit`, trivy, syft

### security/input-validation
- **Binds to** the decision at `security/input-validation`
- **To be verifiable, the record says** where input is validated, and against what
- **Mechanism** a pattern rule, plus fuzzing
- **Where it lands** pre-merge
- **Class** full-check
- **Examples** semgrep, `go test -fuzz`

### security/rate-limiting
- **Binds to** the decision at `security/rate-limiting`
- **To be verifiable, the record says** where the limit is applied, and how it is computed
- **Mechanism** an integration test of the limit
- **Where it lands** pre-merge
- **Class** full-check

### security/secrets-management
- **Binds to** the decision at `security/secrets-management`
- **To be verifiable, the record says** how secrets are stored, and how a committed one is detected
- **Mechanism** secret scanning
- **Where it lands** on-change
- **Class** full-check
- **Examples** gitleaks, trufflehog

### security/data-protection
- **Binds to** the decision at `security/data-protection`
- **To be verifiable, the record says** encryption, personal data, and the audit trail kept over it
- **Mechanism** SAST, plus a test that sensitive data never appears in logs
- **Where it lands** pre-merge
- **Class** full-check
- **Examples** gosec, semgrep

**observability**

### observability/health-checks
- **Binds to** the decision at `observability/health-checks`
- **To be verifiable, the record says** what "healthy" means
- **Mechanism** a test of the health endpoint or signal
- **Where it lands** pre-merge
- **Class** full-check

### observability/logging
- **Binds to** the decision at `observability/logging`
- **To be verifiable, the record says** the shape, the levels, and what is never logged
- **Mechanism** a pattern rule — a structured logger, and no sensitive data
- **Where it lands** pre-merge
- **Class** full-check
- **Examples** semgrep

### observability/metrics
- **Binds to** the decision at `observability/metrics`
- **To be verifiable, the record says** what is measured, and to answer what
- **Mechanism** a test that the metric exists
- **Where it lands** pre-merge
- **Class** full-check

### observability/tracing
- **Binds to** the decision at `observability/tracing`
- **To be verifiable, the record says** how a piece of work is followed across boundaries
- **Mechanism** a context-propagation test
- **Where it lands** pre-merge
- **Class** full-check

**delivery**

### delivery/arch-enforcement
- **Binds to** the decision at `delivery/arch-enforcement`
- **To be verifiable, the record says** which structural rules block a merge
- **Mechanism** the `structure` checks this same catalogue already names, registered to run at
  `pre-merge`
- **Where it lands** pre-merge
- **Class** full-check

### delivery/ci-quality-gates
- **Binds to** the decision at `delivery/ci-quality-gates`
- **To be verifiable, the record says** what blocks a merge, and what only reports
- **Mechanism** the project's own `checks.json` (task 09): every check there runs before a merge, the
  ones taking `{files}` also on each change; what only reports, and does not block, stays out of it
- **Where it lands** pre-merge
- **Class** full-check

### delivery/configuration
- **Binds to** the decision at `delivery/configuration`
- **To be verifiable, the record says** how configuration is supplied, and what happens if it is
  missing
- **Mechanism** a startup test with invalid configuration
- **Where it lands** pre-merge
- **Class** full-check

### delivery/dependency-injection
- **Binds to** the decision at `delivery/dependency-injection`
- **To be verifiable, the record says** the mechanism, and where the composition root lives
- **Mechanism** a rule that concrete implementations are only instantiated at the composition root, no
  unapproved container library, regenerating with no diff when a generator is involved, plus an audit of
  the finer judgement calls a rule cannot phrase mechanically
- **Where it lands** pre-merge
- **Class** partial-check+audit
- **Examples** semgrep, wire, `git diff --exit-code`

### delivery/deployment-topology
- **Binds to** the decision at `delivery/deployment-topology`
- **To be verifiable, the record says** what runs where, and as what
- **Mechanism** an infrastructure linter
- **Where it lands** pre-merge
- **Class** full-check
- **Examples** hadolint, kubeconform, tflint

### delivery/documentation
- **Binds to** the decision at `delivery/documentation`
- **To be verifiable, the record says** what is documented, and where it lives
- **Mechanism** a link and existence check
- **Where it lands** pre-merge
- **Class** full-check
- **Examples** lychee

### delivery/feature-flags
- **Binds to** the decision at `delivery/feature-flags`
- **To be verifiable, the record says** who may toggle a flag, and how it is retired
- **Mechanism** a detector of stale flags
- **Where it lands** pre-release
- **Class** full-check
- **Examples** a project-specific script

### delivery/testing-strategy
- **Binds to** the decision at `delivery/testing-strategy`
- **To be verifiable, the record says** what is tested at each level
- **Mechanism** scenario coverage (`all/coverage` above), plus mutation testing (`all/test-quality`
  above)
- **Where it lands** pre-merge
- **Class** partial-check+audit

### delivery/build
- **Binds to** the decision at `delivery/build`
- **To be verifiable, the record says** the build tool, and how the build is structured
- **Mechanism** a reproducible build in CI
- **Where it lands** pre-merge
- **Class** full-check

### delivery/release
- **Binds to** the decision at `delivery/release`
- **To be verifiable, the record says** how the system is versioned and published
- **Mechanism** validating the version and the changelog
- **Where it lands** pre-release
- **Class** full-check
- **Examples** semantic-release, goreleaser

### delivery/environments
- **Binds to** the decision at `delivery/environments`
- **To be verifiable, the record says** what environments exist, and how each is created
- **Mechanism** an infrastructure linter, plus a plan
- **Where it lands** pre-merge
- **Class** full-check
- **Examples** tflint, `terraform plan`

## What no mechanism proves

True for all three gates, and worth stating once rather than on every entry above it would otherwise
weaken:

- **The absence of bugs.** A test shows that errors exist; it never shows that none do.
- **That a spec is correct.** Every mechanism above checks the code against the spec. Nothing checks the
  spec against what was actually needed.
- **The absence of vulnerabilities.** A scan finds the vulnerabilities it knows to look for.
