# e2e-ai — AI flow scenarios

Prose scenarios, run against a real headless Claude Code session — as the persistent `orlab` lab
user — against this repo's shipped skills, and judged in two parts: a script checks whatever is
machine-verifiable, a separate judge model call checks the rest. They exist because some things only
a real agent exercises: whether it reaches for `openrecord record edit` instead of hand-editing a
file, whether it reports a tool's own finding rather than inventing one, whether it answers from what
the tool actually said. None of that is machine-checkable the way a Go test is.

This is one of three testing layers:

- unit — `internal/**/*_test.go`, run with `go test`.
- deterministic e2e — `e2e/*_test.go`, the binary driven against a copy of `e2e/project`, run with
  `go test`.
- AI flow scenarios (this directory) — prose, run by an agent through the `openrecord-acceptance`
  skill, in the `orlab` lab.

## When scenarios are written

With the task's scenarios — when the task's behavior is decided, before implementing it — not after
the code is written. The same moment a task's deterministic checks become a new `e2e/<name>_test.go`
is the moment its non-deterministic checks become a new `e2e-ai/scenarios/<NN-slug>.md`.

## How to run

```
scripts/check.sh ai [NN-slug ...] [--record]
```

Runs `scripts/check.sh fast` first and aborts immediately if that fails, then `e2e-ai/lab/refresh.sh`
(rebuild + reinstall for `orlab`), then `e2e-ai/run.sh` for each named scenario (default: every
`e2e-ai/scenarios/[0-9]*.md`, sorted). `e2e-ai/run.sh` establishes the scenario's lab precondition
using only `openrecord` commands, drives one headless Claude Code session as `orlab` through every
`prompt` block in order (first via `claude -p`, later ones via `--resume <session_id>`), runs every
`check` block against the result, judges the prose criteria with a second, separate `claude -p` call
in an empty directory, and prints a per-criterion report. `--record` appends one dated line to the
scenario's Verdict log.

This is also what the `openrecord-acceptance` skill (`.claude/skills/openrecord-acceptance/SKILL.md`)
runs before calling a task done.

## One-time lab setup

```
sudo bash e2e-ai/lab/setup.sh
```

Then, once, by hand (this cannot be scripted):

```
sudo -iu orlab claude
/login
```

Unlike `e2e/asuser/` (an older pattern that recreates a throwaway user on every single run), `orlab`
and its project persist across many separate task runs — its Claude Code login is set up once and
never needs repeating.

## Layout

```
e2e-ai/
  README.md          this file
  run.sh             the headless runner: one scenario, start to finish
  lab/
    setup.sh          one-time, sudo, idempotent, never destroys an existing orlab home
    refresh.sh        developer convenience: rebuild + reinstall for orlab, no sudo on itself
  scenarios/
    TEMPLATE.md       the section skeleton for a new scenario file
    <NN-slug>.md       one file per task: lab precondition, prompts, checked criteria, judged
                       criteria, failure signals, verdict log
  .runs/
    <run id>/          gitignored — per-run transcripts, answers, check results, judge output
```

A scenario file never has a "deterministic" section — that half lives in `e2e/<name>_test.go`
instead, and `scripts/check.sh ai` runs `scripts/check.sh fast` (which runs `go test ./...`) as the
precondition before touching the lab.

## Scenario file format

- `## Lab precondition`: one ` ```setup ``` ` bash block, run as `orlab` in a fresh, empty run
  directory — only `openrecord` commands (plus `mkdir` for path components), building the store from
  zero.
- `## Prompts`: one or more ` ```prompt ``` ` blocks, sent in order within one Claude session.
- `## Checked criteria`: one ` ```check ``` ` block per criterion, first line `# CN: description`,
  bash run as `orlab` after every prompt has run, exit 0 = pass. The runner exports `$FINAL_ANSWER`,
  `$ANSWER_1`, `$ANSWER_2`, ..., `$TRANSCRIPT` (the full session transcript), and `$SNAPSHOT_0`,
  `$SNAPSHOT_1`, ... (a full copy of the run directory right after each prompt, for before/after
  diffs).
- `## Judged criteria`: numbered prose (J1, J2, ...), judged by a separate model call.
- `## Failure signals`: prose things that must not have happened, checked alongside the judged
  criteria regardless of how those read.
- `## Verdict log`: append-only, one line per `--record` run.
