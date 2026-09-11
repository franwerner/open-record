---
title: The service authenticates nothing itself
description: Identity is established at the gateway and taken as given inward; the service performs no authentication of its own, and this absence is decided rather than forgotten.
status: accepted
body-hash: 1f72333ad35e36efc2b47afdea650fb3b5cd67f8540180be9064573993d4ff2f
---

## Context

Every few months someone opens the service, looks for where a request is authenticated, finds nothing,
and assumes it was forgotten. The next step is usually a pull request adding a check — which is the
outcome this record exists to prevent.

## Decision

The service performs no authentication of its own. Identity is established at the gateway, which
verifies the caller and passes the established identity inward; anything reaching the service has
already been authenticated, and the service treats that as given.

This was looked at deliberately and the conclusion is that there is nothing here to decide beyond where
the boundary of trust sits — it sits at the gateway. That is a decision, and it governs the code that
comes next just as much as a mechanism would.

Authorisation is a different question and is not settled by this record. What a caller is allowed to do,
once we know who they are, is enforced in the service.

## Alternatives

- **Verifying the credential again in the service.** Rejected: two places that decide who someone is
  will eventually disagree, and the failure is that a request is accepted by one and refused by the
  other for reasons neither logs usefully.
- **No gateway, authentication in the service.** Rejected: the gateway is where rate limiting already
  lives, and it is the only component that sees traffic before it costs anything.

## Consequences

The service is only safe behind the gateway. Exposing it directly — including by port-forwarding it in a
development cluster — exposes it with no authentication at all, and nothing in the code will say so.
That is the cost of the boundary being outside the repository.

This record is here so that the absence reads as decided rather than as forgotten. Without it a reader
cannot tell "nobody looked at this" from "someone looked and it does not apply", and those call for
opposite behaviour.
