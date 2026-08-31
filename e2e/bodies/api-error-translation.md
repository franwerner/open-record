## Context

The service calls a payment provider and a mail provider. Both raise their own errors, in their own
vocabulary, with their own idea of what is retryable. Left alone, those errors travel: a provider's
class name ends up in a catch block three layers away, and swapping the provider becomes a change to
code that has nothing to do with payments.

## Decision

Business errors are their own hierarchy, and a provider's errors are translated at the adapter boundary.

Nothing outside an adapter ever sees a foreign error type. The adapter decides what a provider failure
means in this system's terms — declined, temporarily unavailable, misconfigured — and raises that. Which
provider failure maps to which meaning is part of the adapter, and changing providers is a change to
that mapping and nothing else.

What a client receives for each of those meanings is a public contract and is fixed by the API
versioning record, not here.

## Alternatives

- **Letting provider errors propagate and catching them at the edge.** Rejected: it works until there
  are two providers, and then the edge has to know both vocabularies. It also means every layer in
  between declares a dependency it does not use.
- **One generic error with a message.** Rejected: it makes the message the contract, so nothing can
  decide whether to retry without reading prose.
- **Translating at the edge rather than at the adapter.** Rejected: the knowledge of what a provider
  failure means belongs next to the call that produced it, and the edge is where that context is
  already gone.

## Consequences

Every adapter carries a mapping that has to be kept current with its provider, and a provider failure
nobody mapped arrives as the unclassified case rather than as something specific. That is deliberate:
an unmapped failure is visible as a gap instead of being silently absorbed into the nearest category.

The hierarchy is one more thing to learn before writing an adapter, and it is the first thing to explain
to someone adding one.
