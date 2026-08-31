---
title: Readable by default, JSON on request
description: Commands print for a person by default and switch wholly to JSON with a flag; exit status separates success, failure and a malformed invocation.
status: accepted
---

## Context

The operator command line is run by people at a terminal and by scripts in a deployment pipeline. Those
two want opposite things from the same command: a person wants a table they can read, a script wants
something it can parse without matching on prose.

## Decision

Human-readable output is the default, and `--json` switches the whole command to a single JSON object.
No command mixes the two, and no command emits JSON with a human line above it.

Exit status is the part scripts actually depend on, so it is fixed: zero when the command did what it
was asked, non-zero when it did not, and a distinct status for a malformed invocation, so a pipeline can
tell its own bug from a real failure of the operation.

Anything a script needs is in the JSON. A field is never available only in the human output, because
that would make the readable form the contract by accident.

## Alternatives

- **JSON always.** Rejected: an operator reading output at three in the morning is the case this tool
  exists for, and making them pipe through a formatter to read one number is hostile.
- **A machine format that is also readable — tab-separated columns.** Rejected: it looks parseable and
  is not, because a value containing the separator is not hypothetical, and the failure is silent.
- **Guessing from whether standard output is a terminal.** Rejected: it makes the same command behave
  differently depending on where it runs, which is exactly what breaks in a pipeline nobody tested
  interactively.

## Consequences

Every command has to render twice, and a new field has to be added in both places — the JSON shape and
the human view — or one of them is quietly incomplete.

Because the exit status is contractual, changing what a status means is a breaking change even when no
text output moved, and it is the kind of change that will not show up in a diff of the readable output.
