---
title: Whether a key can have its own quota
description: Whether an individual API key can carry a quota of its own is deliberately undecided; the single shared limit governs until someone settles it.
status: pending
body-hash: d0b1f55a4fa1db1d919b5349ac0af8bbb3f631f56f0fbfc72eca01f160524621
---

## Context

The limit today is one number for every client. Two customers have asked for a higher ceiling, and one
integration partner genuinely needs a different one — their traffic is bursty by nature rather than by
carelessness.

Raising the single number for everyone was rejected out of hand: it would remove the protection for the
clients that do not need the headroom.

## Decision

**Undecided, deliberately.** Nobody has settled how a per-key quota would work, and this record exists
so the next person can tell that from nobody having looked.

Two shapes were sketched and neither was chosen: a quota attached to the key itself, which puts customer
data in the gateway's configuration; and a tier on the account that the gateway reads at request time,
which adds a lookup to the hot path of every request.

The question is open until someone decides which cost is the acceptable one.

## Alternatives

Not applicable yet — a pending record settles nothing, so there is no rejected alternative to describe.
The two shapes above are sketches, not candidates that were weighed and dropped.

## Consequences

Until this is settled, the single limit fixed by the gateway placement governs every client, and the
partner's bursts are absorbed by their own retry logic.

Nothing is verified against this record. Work that needs a per-key quota to exist is blocked on the
decision, not on the implementation, and saying so is the point of writing this down.
