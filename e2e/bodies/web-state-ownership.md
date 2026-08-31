## Context

The browser application shows data that lives on the server and data that exists only while someone is
looking at a screen — which row is expanded, what is typed but not submitted. Early on both were kept
the same way, and the result was a cache nobody could reason about: a screen would show a stale total
because a local update had been written into the same place the server's answer lived.

## Decision

Server data is never held as local state. It is fetched, cached by the fetching layer, and invalidated
by the mutation that changed it. What a screen owns is only what would be lost by closing it.

The consequence that does the work: a component never writes to the place server data is read from. A
change is sent, the fetch is invalidated, and the new value arrives the same way the old one did — so
what is on screen after a change is what the server actually holds, not what the client predicted.

## Alternatives

- **One store for everything.** Rejected: it is what produced the stale totals. The two kinds of data
  have different lifetimes and different owners, and one place makes that impossible to see.
- **Optimistic local writes into the cache.** Rejected for now, and this is the closest call: it is
  faster to look at, but it makes the screen show a prediction, and every failed prediction needs a
  rollback path that is exercised only when something is already going wrong.
- **No cache at all — fetch on every render.** Rejected: it turns navigation into a sequence of empty
  screens.

## Consequences

A change looks slower than it could, because the value on screen waits for the round trip rather than
updating immediately. That is accepted deliberately, and it is the reason the waiting state is specified
as behaviour rather than left to each screen.

Anyone adding a screen has to decide, per piece of data, which side it falls on. The question is easy —
would closing the tab lose it — but it has to be asked every time.
