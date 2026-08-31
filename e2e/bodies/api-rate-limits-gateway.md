## Context

Every endpoint needs protecting from a client that calls it too often, and the first instinct is to add
the check where the abuse shows up — inside the handler that is being hammered. The service already sits
behind a gateway that sees every request, which makes the same protection available one layer earlier,
before any application code runs.

The pressure that forced the choice was a client hitting three endpoints in a loop. Each handler saw a
rate it considered acceptable; the total was not.

## Decision

Rate limiting is applied at the gateway, not in each handler.

The limit is computed per API key across the whole surface, so a client's budget is one number rather
than one per endpoint. A handler never checks a limit and never has a reason to know one exists.

What the actor meets when they cross it — the status code, the header telling them when to retry — is
behaviour and is fixed by the usage-limits rule, not here. This record fixes the placement and the
reason for it.

## Alternatives

- **A check inside each handler.** Rejected: a per-handler limit cannot see a client's total across
  endpoints, which is the case that caused the problem. It also puts the same block of code in every
  handler, where it drifts.
- **A middleware inside the service, before the router.** Rejected: closer to right, but it still only
  sees traffic that reached this service, so it cannot protect the service from the load itself.
- **Limits per endpoint, tuned individually.** Rejected as premature: it is the answer to a problem
  nobody has yet, and it makes the budget impossible for a client to reason about.

## Consequences

The gateway becomes a place where behaviour lives, so a change to the limit is a deployment of
infrastructure rather than of this service, and the two are released on different cadences.

A local development run has no gateway in front of it, so the limit does not exist there. Anything that
needs to observe the limit has to be tested against a deployed environment — that cost is accepted, and
it is the reason the usage-limits rule states the numbers rather than leaving them to be read off a
configuration file.
