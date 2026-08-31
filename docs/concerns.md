# The concerns catalogue

Eleven concerns, and the forty-nine places projects usually have a decision worth recording.

This document ships with the binary and **never enters a project**. It creates nothing: a concern
folder exists because someone had a record to file in it.

## The two levels do different jobs

**The eleven concerns are the folder vocabulary.** This is what a writer picks from when filing a
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
> what belongs to the neighbour instead. That is for a reader choosing between the two; it is not part
> of the description and never reaches an index.

---

## structure

> How the codebase is organised, and what is allowed to depend on what. Descend here if you touch
> layering, module boundaries, folder shape, or the conventions the code holds to.

- **architecture-style** — Whether the system commits to a named shape (layered, hexagonal, vertical
  slices, none in particular) and what that commitment actually forbids.
- **code-conventions** — The naming and idiom rules the project holds to beyond what a formatter can
  enforce: how things are named, which constructs are avoided, what "consistent" means here.
- **folder-structure** — How directories are cut: by feature, by layer, by type. This one is decided
  once and silently governs every file added afterwards.
- **inter-layer-communication** — How the pieces talk to each other: direct calls, events, a mediator,
  injected ports.
- **layers-and-dependencies** — Which layers exist and which may depend on which. The direction of
  dependency is the decision; the layer names are just its vocabulary.

## domain-logic

> How the business itself is modelled in code: where the consistency boundaries are, where invariants
> are enforced, and what the domain's words mean here. Descend here if you touch an aggregate, an
> invariant, or the vocabulary.

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

## runtime

> How the system behaves while it is running: deferred work, failure, concurrency, and what it keeps in
> memory. Descend here if you touch jobs, retries, caching or error propagation.

- **background-jobs** — How work that does not happen in the request runs: a queue, a scheduler,
  in-process. And what happens to a job that fails.
- **caching** — What is cached, where it lives, and — the part that is actually decided — how it is
  invalidated.
- **concurrency-async** — How concurrent work is expressed and coordinated, and what the system
  promises about ordering.
- **error-handling** — How errors are modelled, how they propagate, and where they are translated
  between a dependency's vocabulary and the system's own.
- **resilience** — Timeouts, retries, backoff, circuit breaking. What the system does when something it
  depends on is slow or gone.

## data

> How the system stores and reads persistent data. Descend here if you touch queries, the storage
> model, transactions, or schema change.

- **data-access** — Which layer owns queries, whether anything sits between the domain and the
  database, and how transactions are scoped.
- **data-modeling** — How the domain is represented in storage, and how schema change is handled once
  there is data to migrate.

## data-lifecycle

> What happens to data over time, after it is written. Descend here if you touch retention, deletion,
> backups, or archival.

*Not here: how data is shaped and read — that is `data`. This concern starts where that one stops: not
what a record looks like, but how long it lives and what happens at the end. The name is
`data-lifecycle` rather than `lifecycle` because a spec type already carries that word for something
else — an entity's state machine — and a vocabulary an agent picks from cannot have one word meaning
two things.*

- **retention** — How long each kind of data is kept, and what decides that: a policy, a regulation, or
  nobody having thought about it.
- **deletion** — What "deleted" means: gone, flagged, or cascaded — and what a user is promised about
  it.
- **backup-and-restore** — What is backed up, how often, and — the part usually left undecided — how a
  restore is proven to work before it is needed.
- **archival** — How data leaves the hot path without being lost, and what still reaches it afterwards.

## contracts

> What the system promises to anything outside it, and how those promises change. Descend here if you
> touch a public surface: an API, a CLI, published events, a package.

- **api-contract** — The shape of the HTTP or RPC surface: versioning, error shape, pagination, what
  counts as a breaking change.
- **cli-contract** — The command surface and what it promises across versions: flag stability, output
  format, exit codes.
- **event-contract** — The shape of published events and their compatibility rules — the hardest
  contract to change, because you cannot see who is reading.
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
- **inbound-webhooks** — How events from outside are received, verified as genuine, and made safe to
  receive twice — because they will be.
- **messaging** — Which queues or brokers are consumed, and what is actually relied on about ordering
  and delivery.

## security

> How the system establishes who someone is, what they may do, and how it protects what it holds.
> Descend here if you touch authentication, permissions, secrets, or input handling.

- **auth** — How identity is established and carried: sessions, tokens, an external provider, and where
  the boundary of trust sits.
- **authorization** — How permissions are modelled and — the part that matters — *where* they are
  enforced.
- **cors** — Which origins may talk to the system, and why those.
- **dependency-scanning** — How risk in third-party code is detected, and what happens when something
  is found.
- **input-validation** — Where input is validated, against what, and what the system assumes once it is
  past that boundary.
- **rate-limiting** — Where limits are applied, how they are computed, and what a caller is told when
  they hit one.
- **secrets-management** — How secrets are stored, injected and rotated, and what is deliberately never
  in the repository.

## observability

> How the system is understood from outside while it runs. Descend here if you touch logs, metrics,
> traces, or health.

- **health-checks** — What "healthy" means for this system, and who is asking.
- **logging** — What is logged, at what level, in what shape — and what is deliberately never logged.
- **metrics** — What is measured, and what question each measurement is there to answer.
- **tracing** — How a single piece of work is followed across boundaries.

## delivery

> How the system is built, configured, tested and shipped. Descend here if you touch CI, configuration,
> deployment, wiring, or the testing approach.

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

## quality

> The properties the system holds to beyond being correct. Descend here if you touch performance
> targets, growth, or locale-dependent behaviour.

- **i18n-l10n** — How text, formats and locale-dependent behaviour are handled, and how far that
  reaches into the domain.
- **nfr-performance** — The performance targets that are actually committed to, and what is traded away
  to meet them.
- **scalability** — How the system is expected to grow, along which axis, and what that rules out.
