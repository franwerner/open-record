# The concerns catalogue

Ten concerns, and the sixty places projects usually have a decision worth recording.

This document ships with the binary and **never enters a project**. It creates nothing: a concern
folder exists because someone had a record to file in it.

A project with its own `quality` concern is not broken by this revision. A project's concern never
depends on the catalogue — a project that needs a concern the catalogue does not have simply invents
it, as the next section says. Dropping `quality` here only means that name no longer comes with a
default description; records already filed under a project's own `decisions/<component>/quality/`
keep working exactly as before.

**Why `quality` is gone.** This catalogue used to ship an eleventh concern, `quality`, and it is now
dissolved rather than reformulated. The name collided with the `quality` *spec* type introduced
alongside it — one vocabulary cannot carry the same word for two different meanings, the same reason
this catalogue's `data-lifecycle` concern is not called `lifecycle`. Its three topics did not share one
fate: `nfr-performance` is retired outright, because the objective it used to ask about is now a
`quality` spec and the mechanism was already asked about elsewhere (`runtime/caching`,
`data/data-access`, `runtime/concurrency-async`, `delivery/deployment-topology`) — what is traded away
to meet the objective is the `## Consequences` of that decision, not a topic of its own. `scalability`
moved to [`runtime/scaling`](#runtime). `i18n-l10n` moved to [`domain-logic/i18n-l10n`](#domain-logic)
and was broadened to cover formats, time zones and currencies, not only text.

## The two levels do different jobs

**The ten concerns are the folder vocabulary.** This is what a writer picks from when filing a
record, and it is what `level add` reads to fill in a new folder's index. A project that needs a
concern the catalogue does not have simply invents it.

**The topics name nothing.** They are the discovery checklist — so an agent facing an
empty component knows what to *ask* (*did anyone decide something about rate limiting? about
caching?*) rather than staring at a blank folder. Subgroups are not taken from this list; a subgroup is
named for whatever topic its cluster shares.

> **Format.** Each concern is a heading, followed by a blockquote that is its default index
> description — `level add` reads exactly that, and nothing else on the page. The description is
> generic by construction, since it has to hold for any project, so overriding it with something that
> says when to descend *in this project* is usually the right move.
>
> Where a concern sits next to another one it could be confused with, an *italic line* under it says
> what belongs to the neighbour instead. The same line is also used to say what belongs to a spec
> instead of a decision — a measurable target to a `quality` spec, actor-facing behaviour to a
> functional spec. That is for a reader choosing where something goes; it is not part of the
> description and never reaches an index.

---

## structure

> How the codebase is organised, and what is allowed to depend on what, inside the code and on the
> outside world. Descend here if you touch layering, module boundaries, folder shape, the languages and
> runtimes in play, or the conventions the code holds to.

- **architecture-style** — Whether the system commits to a named shape (layered, hexagonal, vertical
  slices, none in particular) and the framework that gives that shape its concrete form (React, Next,
  Gin, and so on), and what that commitment actually forbids.
- **code-conventions** — The naming and idiom rules the project holds to beyond what a formatter can
  enforce: how things are named, which constructs are avoided, what "consistent" means here.
- **folder-structure** — How directories are cut: by feature, by layer, by type. This one is decided
  once and silently governs every file added afterwards.
- **inter-layer-communication** — How the pieces talk to each other: direct calls, events, a mediator,
  injected ports.
- **layers-and-dependencies** — Which layers exist and which may depend on which. The direction of
  dependency is the decision; the layer names are just its vocabulary.
- **code-health** — The thresholds held for complexity, duplication, dead code and coverage, or the
  decision to hold none.
- **language-and-runtime** — What languages and runtimes the system runs on, and why those.
- **dependency-policy** — What external dependencies may be added, under what criteria — licences
  included — and who decides. Extends "what may depend on what" to the outside world.
- **shared-code** — What is shared between parts of the system (internal libraries, components, a
  design system) and how it evolves.
- **code-generation** — What code is generated, from what (contracts, schemas, SQL), and whether it is
  committed or regenerated on every build.

## domain-logic

> How the business itself is modelled in code: where the consistency boundaries are, where invariants
> are enforced, and what the domain's words mean here. Descend here if you touch an aggregate, an
> invariant, the vocabulary, or how locale-dependent values are represented.

*Not here: what the system does when a rule fires — that is observable, so it is a capability spec. How
the code is organised into layers and folders is `structure`. This concern is about the shape of the
business, not the shape of the codebase; a perfectly layered project can still have its aggregates cut
in the wrong places.*

- **aggregates** — What is changed together and what is not: the consistency boundaries, and which
  things have an identity of their own versus only existing inside something else.
- **invariants** — Where a business invariant is enforced, and what is not allowed to bypass it. The
  rule itself is a spec; *where it lives* is the decision.
- **domain-vocabulary** — The words the business uses, and how far they are allowed to reach into the
  code before being translated.
- **i18n-l10n** — How text, formats, time zones and currencies are handled for display, and how far
  that reaches into the domain. *Which languages are supported is behaviour: a functional spec.*

## runtime

> How the system behaves while it is running: deferred work, failure, concurrency, where state lives,
> and how it grows. Descend here if you touch jobs, retries, caching, error propagation, state, or
> scaling.

- **background-jobs** — How work that does not happen while the actor waits runs: a queue, a scheduler,
  in-process — this includes a worker running in a browser as much as one running on a server. *What
  happens to a job that fails, as the actor experiences it, is an edge case of the `process` that
  queued it.*
- **caching** — What is cached, where it lives, and — the part that is actually decided — how it is
  invalidated. *How stale a value an actor may see is a `quality` spec.*
- **concurrency-async** — How concurrent work is expressed and coordinated. *What the system promises
  an actor about ordering is a `rule` spec.*
- **error-handling** — How errors are modelled, how they propagate, and where they are translated
  between a dependency's vocabulary and the system's own.
- **resilience** — Timeouts, retries, backoff, circuit breaking. *What the system does for the actor
  when something it depends on is slow or gone is functional; how much availability is committed to is
  a `quality` spec.*
- **state-management** — Where state lives, who owns it, and how it is shared or synchronized. Holds
  for a UI (a store, server-synced state) as much as for a backend (sessions, stateless services).
- **scaling** — How the system is expected to grow, along which axis (horizontal, vertical, sharding),
  and what that rules out. *How much load it holds, with a threshold, is a `quality` spec.*

## data

> How the system stores and reads data that outlives a single request or call — on a server or on a
> client. Descend here if you touch queries, the storage model, transactions, schema change, local
> storage, or how a client's calls to an API are scoped.

- **data-access** — Which layer talks to the data source — a query against a database, a client's call
  to an API, a write to local storage — and how transactions are scoped.
- **data-modeling** — How the domain is represented in storage, how schema change is handled once there
  is data to migrate, and how values that tolerate no imprecision are represented (a dated amount, a
  currency).
- **data-stores** — What kinds of storage exist (relational, document, files, a search index, a cache)
  and why each.
- **tenancy** — How data belonging to different customers or organizations is isolated, or the decision
  that it does not apply.

## data-lifecycle

> What happens to data over time, after it is written. Descend here if you touch retention, deletion,
> backups, or archival.

*Not here: how data is shaped and read — that is `data`. This concern starts where that one stops: not
what a record looks like, but how long it lives and what happens at the end. The name is
`data-lifecycle` rather than `lifecycle` because a spec type already carries that word for something
else — an entity's state machine — and a vocabulary an agent picks from cannot have one word meaning
two things.*

- **retention** — How retention is enforced, once a period is settled. *How long each kind of data is
  kept, when that is promised to a user or required by a regulation, is a `rule` spec.*
- **deletion** — What "deleted" means mechanically: gone, flagged, or cascaded. *What a user is
  promised about deletion is behaviour: a functional spec.*
- **backup-and-restore** — What is backed up, how often, and — the part usually left undecided — how a
  restore is proven to work before it is needed. *RPO and RTO are `quality` specs.*
- **archival** — How data leaves the hot path without being lost, and what still reaches it afterwards.

## contracts

> What the system promises to anything outside it, and how those promises change. Descend here if you
> touch a public surface: an API, a CLI, published events, a package, or the URLs a user can bookmark
> or share.

The consumer of an API, a CLI, an event or a library is an actor: what it sees is behaviour. What stays
here as a decision is **the cross-cutting conventions and the compatibility policy**, not what each
endpoint or command does.

- **api-contract** — Conventions shared across the surface (error shape, pagination, style) and the
  policy for versioning and breaking changes. *What each endpoint does is a `flow` spec.*
- **cli-contract** — Conventions for flags, output format, exit codes, and stability across versions.
  *What each command does is a spec.*
- **event-contract** — Conventions and compatibility rules for published events — the hardest contract
  to change, because you cannot see who is reading. *When each event is published is part of the
  `process` or `flow` that emits it.*
- **library-contract** — What a published package exposes, what is deliberately internal, and the
  compatibility promise attached.

## integration

> What the system consumes from outside, and how it keeps a foreign shape from spreading inwards.
> Descend here if you call a third party, receive a webhook, or read from a queue you do not own.

*Not here: what this system exposes — that is `contracts`, and the direction is what separates them.
Timeouts and retries against a slow dependency are `runtime`; this concern is about the boundary
itself, not about surviving it.*

- **third-party-clients** — How an external provider is called, and where its API is confined so that
  swapping it does not reach the domain.
- **anti-corruption** — Where a foreign vocabulary is translated into this system's own, and what is
  deliberately not translated.
- **inbound-webhooks** — How events from outside are received and verified as genuine. *That receiving
  the same one twice has no effect is an edge case of the `process` it feeds; the decision here is the
  idempotency mechanism.*
- **messaging** — Which queues or brokers are consumed, and what is actually relied on about ordering
  and delivery.

## security

> How the system establishes who someone is, what they may do, and how it protects what it holds.
> Descend here if you touch authentication, permissions, secrets, input handling, or data protection.

- **auth** — How identity is established and carried: sessions, tokens, an external provider, and where
  the boundary of trust sits.
- **authorization** — How permissions are modelled and — the part that matters — *where* they are
  enforced. *Who may do what is behaviour: specs with their own actors.*
- **cors** — Which origins may talk to the system, and why those.
- **dependency-scanning** — How risk in third-party code is detected, and what happens when something
  is found.
- **input-validation** — Where input is validated, against what, and what the system assumes once it
  is past that boundary. *What input is rejected, and what the actor receives instead, is a spec.*
- **rate-limiting** — Where a limit is applied and how it is computed. *What a caller is told when they
  hit it is a spec.*
- **secrets-management** — How secrets are stored, injected and rotated, what is deliberately never in
  the repository, and how a secret committed by mistake is detected.
- **data-protection** — How sensitive data is protected: encryption at rest and in transit, personal
  data, and the audit trail kept over actions taken on it.

## observability

> How the system is understood from outside while it runs, on a server and on a client. Descend here
> if you touch logs, metrics, traces, health, or how errors and performance are observed in the client.

- **health-checks** — What "healthy" means for this system, and who is asking.
- **logging** — What is logged, at what level, in what shape — and what is deliberately never logged.
- **metrics** — What is measured, and what question each measurement is there to answer. *The targets
  (SLOs) are `quality` specs; this is about what is measured and to answer what.*
- **tracing** — How a single piece of work is followed across boundaries.

## delivery

> How the system is built, configured, tested and shipped. Descend here if you touch CI, configuration,
> deployment, wiring, versioning, environments, or the testing approach.

- **arch-enforcement** — How architectural rules are kept from drifting: import boundaries, lint rules,
  whatever makes a violation fail rather than accumulate.
- **ci-quality-gates** — What blocks a merge, and what is only reported.
- **configuration** — How configuration is supplied and layered, and what happens when it is missing or
  invalid.
- **dependency-injection** — How dependencies are wired: a container, manual composition, or something
  in between.
- **deployment-topology** — What runs where, as what, and how the pieces find each other.
- **documentation** — What is documented, where it lives, and what is deliberately left to the code.
- **feature-flags** — How behaviour is toggled, who may toggle it, and how a flag is retired — the part
  nobody decides until flags have piled up.
- **testing-strategy** — What is tested at which level, and what is deliberately not tested.
- **build** — How the system is built: the build tool, the bundler, a monorepo or separate repos.
- **release** — How the system is versioned and published: semver, channels, a changelog.
- **environments** — What environments exist, what tells them apart, and how each is created
  (infrastructure as code or otherwise).
