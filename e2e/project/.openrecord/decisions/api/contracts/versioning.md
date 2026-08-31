---
title: The version is in the URL, and a version is additive
description: The API version is carried in the URL; inside a version only additive change ships, and a consumer keeps working until that version is retired.
status: accepted
---

## Context

The API has external consumers who deploy on their own schedule, so a change that breaks them cannot be
coordinated by asking. Something had to fix what counts as breaking, and how a consumer finds out.

## Decision

The version is carried in the URL, and a version is additive until it is retired.

Inside a version, adding a field or a new optional parameter is not a breaking change and ships without
ceremony. Removing a field, narrowing what a parameter accepts, or changing the meaning of a value
requires a new version. A consumer pinned to a version keeps working until that version is retired, and
retirement is announced rather than deployed.

Errors are part of the contract: the code a client matches on is stable within a version, and the
message attached to it is not.

## Alternatives

- **A version header instead of the URL.** Rejected: it is invisible in a log, a browser and a bug
  report, which are the three places a version is actually needed. The URL is uglier and always there.
- **Rolling changes with no versions, coordinated by announcement.** Rejected: it only works when you
  can reach every consumer, and the point of a public API is that you cannot.
- **A version per endpoint.** Rejected: it makes the matrix of what works with what unbounded, and no
  consumer can state which version they are on.

## Consequences

Two versions run at once for as long as a retirement takes, so a change to shared behaviour has to be
correct under both. That is the cost that buys consumers a schedule of their own.

Because errors are contractual, a new error code is an additive change and a renamed one is not — which
makes naming an error a decision rather than a detail.
