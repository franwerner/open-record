## Context

The suite grew faster than the product and started costing more than it caught: a change to one screen
broke forty tests, none of which were about that screen. The question was not whether to test but at
which level, and what is deliberately left untested.

## Decision

Tests are written at the boundary the change would be noticed at.

Behaviour a user observes is tested through the surface that offers it — a request in, a response out,
with the providers stubbed at the adapter boundary and nothing else faked. Logic with real branching of
its own is tested directly, without going through a surface. Everything in between is not tested twice.

What is deliberately not tested: the wiring itself, and anything whose failure a running process would
report immediately. A test that a module exports what it exports catches a class of bug that does not
happen.

A test asserts an outcome, never a call. A test that checks something was invoked passes after a
refactor that broke the feature, which is the failure mode this rule exists to prevent.

## Alternatives

- **A pyramid with a target ratio per level.** Rejected: the ratio becomes the goal, and tests get
  written at whichever level is under quota rather than where the risk is.
- **End-to-end only, through the browser.** Rejected: it catches the most and reports it the least
  precisely, and the run time makes people stop running it.
- **Mocking every dependency at every level.** Rejected: it is what produced the forty broken tests. A
  mock encodes today's call shape, so every refactor becomes a test rewrite.

## Consequences

Some code is covered only through the surface above it, so a failure there points at the surface and the
cause has to be found by reading. That is accepted: the alternative is a second test that pins the
current arrangement of code in place.

The rule about outcomes over calls is the one that needs enforcing in review, because a call assertion
is easier to write and looks the same in a diff.
