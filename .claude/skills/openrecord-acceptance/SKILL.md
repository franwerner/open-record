---
name: openrecord-acceptance
description: "Trigger: task finished, acceptance test, end-to-end, lab, prove the feature works. Write and run the task's acceptance plan in the orlab lab before calling it done."
license: Apache-2.0
metadata:
  author: franwerner
  version: "3.0"
---

# Prove a task works, in the lab

A task is not done because the code compiles and its unit tests pass. It is done when a real,
locally-built `openrecord` behaves the way the task decided it would, both for the deterministic
things a Go test can assert exact values for, and for the parts only a real Claude Code session
exercises — the things an actual agent, following the shipped skills, actually does when asked.

This repo splits that proof across two layers: deterministic Go tests under `e2e/*_test.go`, and
prose AI flow scenarios under `e2e-ai/scenarios/*.md`, run against the persistent `orlab` lab user.
This skill writes the second layer and runs both.

## Activation Contract

Trigger on: a task's scenarios are being defined (write scenarios now, before implementing); a task
or feature is reported finished and needs proving before being called done; someone asks for an
acceptance test, an end-to-end check, or "does this actually work"; a reference to "the lab,"
"orlab," or this skill by name. Do not trigger for ordinary unit/integration test runs
(`go test ./...`) alone — those are a precondition, not a substitute, and this skill does not
replace them.

## Hard Rules

- Scenarios are written when the task's scenarios are defined — before implementing — not
  reconstructed afterward. A deterministic scenario becomes a Go test in `e2e/<name>_test.go`; a
  non-deterministic one becomes `e2e-ai/scenarios/<NN-slug>.md` from `e2e-ai/scenarios/TEMPLATE.md`:
  a `setup` block, one or more `prompt` blocks, a `check` block per machine-checkable criterion, a
  numbered prose "Judged criteria" list, and a prose "Failure signals" section.
- A task is not accepted until **both** halves pass: `go test ./...` and every AI scenario's checked
  *and* judged criteria. Either one failing means the task is not done yet.
- Assert **exact values** everywhere a check can — a code, an anchor, a JSON field's content, a file
  diff. Never settle for "exit code was zero" or "something came back."
- Never hand-edit the lab project's `.openrecord/` store. Every write goes through `openrecord`
  itself, run as `orlab`.
- Never run a scenario's non-deterministic half by driving a terminal by hand — `scripts/check.sh ai`
  does it headlessly end to end (lab precondition, prompts, checks, judge), with a saved transcript
  for every run. Running against a stale binary proves nothing about the current code, which is why
  that same command always rebuilds and reinstalls for `orlab` first.
- Judge strictly against a scenario's own written criteria — never generic "did it seem reasonable."
  The checked criteria are verified by the runner's script half, against the lab project's files and
  `openrecord`'s own output; the judged criteria are verified by a separate judge model call, fed the
  transcript and the checked-criteria results. A judge verdict can itself be wrong — on a FAIL, read
  the saved transcript under `e2e-ai/.runs/<run id>/` before concluding anything; report the evidence,
  don't just trust the verdict.
- If the lab is missing (`sudo -n -u orlab true` fails) or `orlab`'s Claude Code session is logged
  out (`scripts/check.sh ai` reports this itself), tell the human to run `sudo bash
  e2e-ai/lab/setup.sh` if the user doesn't exist yet, then do the one manual step nothing can script
  (`sudo -iu orlab claude` and `/login`), and **STOP**. Do not try to work around a missing lab.

## Decision Gates

- Defining scenarios for a task now, before implementing? Write the deterministic ones as
  `e2e/<name>_test.go` and the AI ones as `e2e-ai/scenarios/<NN-slug>.md`, then stop — there is
  nothing to run yet.
- Task reported finished? Does `e2e-ai/scenarios/<NN-slug>.md` already exist for it? If so, read it
  alongside the task doc rather than starting from a blank template.
- Lab present and logged in? `scripts/check.sh ai <scenario>` checks both itself and fails fast with
  the exact manual step if not — if it does, stop per the Hard Rule above, don't retry around it.
- Did `scripts/check.sh fast` pass? It always runs first as part of `ai` and aborts immediately if
  not — do not drive a lab session against code that does not even pass its own tests.

## Execution Steps

When a task's scenarios are defined (before implementing):
1. Write the deterministic checks as a new `e2e/<name>_test.go`, using the existing e2e harness
   (`e2e/README.md`, `e2e/harness_test.go`, `e2e/fixture_test.go`).
2. Write the non-deterministic checks as `e2e-ai/scenarios/<NN-slug>.md`, from
   `e2e-ai/scenarios/TEMPLATE.md` — lab precondition, prompts, one `check` block per checked
   criterion, numbered judged criteria, failure signals.

When the task is reported finished:
1. Run `scripts/check.sh ai <NN-slug> --record`. It runs `scripts/check.sh fast` first and aborts
   immediately if that fails; then rebuilds and reinstalls for `orlab`; then drives the scenario
   headlessly end to end — lab precondition, every prompt in one resumed Claude Code session, every
   `check` block, and a separate judge call for the judged criteria — and appends one dated line to
   the scenario's Verdict log.
2. Read the printed per-criterion report (C1..Cn, J1..Jn). On any FAIL, read the saved transcript and
   judge output under `e2e-ai/.runs/<run id>/` before concluding anything — a judge FAIL can itself be
   wrong; report evidence, not just the verdict.
3. Report pass/fail to the human, with the evidence that backs it. The task is accepted only when
   `scripts/check.sh fast` passed **and** every scenario's checked and judged criteria passed.

## Output Contract

Report: which scenario(s) ran, the run id, the fast-check result, the per-criterion report (checked
and judged), and the final pass/fail — with evidence for any FAIL, read from the saved run artifacts,
not assumed from the verdict alone.

## References

- `e2e-ai/README.md`, `e2e-ai/lab/setup.sh`, `e2e-ai/lab/refresh.sh` — the lab itself.
- `e2e-ai/run.sh`, `scripts/check.sh` — the headless runner and its local-CI entry point.
- `e2e-ai/scenarios/TEMPLATE.md`, `e2e-ai/scenarios/<NN-slug>.md` — one scenario file per task.
- `e2e-ai/.runs/<run id>/` — saved transcripts, per-prompt answers, check results, judge output.
- `e2e/README.md`, `e2e/harness_test.go`, `e2e/fixture_test.go` — the deterministic e2e harness.
- `e2e/asuser/` — an older, throwaway-user (`ordtest`) pattern this lab is modeled on but does not
  reuse: that one recreates its user on every run and never persists a Claude Code login; this lab
  deliberately keeps `orlab` and its login around across many separate task runs.
