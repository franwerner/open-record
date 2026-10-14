# AI flow scenario: NN-slug

## Task

[Reference the task document (path) and summarize the settled decisions/implementation this
scenario is proving — in your own words, not a full quote. Name exactly which behaviors were
decided and where that's recorded (code paths, docs, decision/spec records). The deterministic half
of this task's proof lives in `e2e/<name>_test.go` — point at it here, do not duplicate it.]

## Lab precondition

ONE `setup` block, run as `orlab` inside a fresh, completely empty run directory. Only `openrecord`
commands (plus `mkdir` for path components the content needs) — it must create the openrecord store
from zero. Never a hand-edit of `.openrecord/`.

```setup
mkdir -p src/example && touch src/example/.gitkeep
openrecord component add example --path src/example --title "Example" --description "..."
openrecord level add specs/example --title "Example specs" --description "..."
openrecord record write specs/example/thing.md \
  --title "Thing" --description "..." --status accepted --components example \
  --body-file <(cat <<'EOF'
...
EOF
)
```

## Prompts

One or more `prompt` blocks, sent in order within ONE Claude session (the first via `claude -p`,
later ones via `--resume <session_id>`).

If the scenario expects the agent to write a record, add a last prompt confirming it ("Looks right — go
ahead and write it."). The shipped skills ask the person to confirm before writing, so an agent may
stop at a proposal; nobody answers in a headless run, and the scenario would fail on a run where the
agent did exactly what its skills say. An agent that already wrote it just says so.

```prompt
[...]
```

```prompt
[...]
```

## Checked criteria

One `check` block per criterion. First line is a `# CN: description` comment. Bash, run as `orlab`
in the run directory, after all prompts have run. Exit 0 = pass.

The runner exports `$FINAL_ANSWER` (last prompt's final answer text), `$ANSWER_1`, `$ANSWER_2`, ...
(per-prompt final answers), and `$TRANSCRIPT` (path to the full transcript, every prompt's
stream-json events concatenated in order) for these to use. It also exports `$SNAPSHOT_0`,
`$SNAPSHOT_1`, ... — each a directory holding a full copy of the run directory as it stood right
after that prompt finished (`$SNAPSHOT_0` is right after the lab precondition, before any prompt) —
so a check can assert a file was untouched by prompt N by diffing `$SNAPSHOT_<N-1>/path` against the
live `path`.

```check
# C1: [description]
[bash, exits 0 on pass]
```

```check
# C2: [description]
[bash, exits 0 on pass]
```

## Judged criteria

Numbered prose, sent to the judge model alongside the transcript digest and check results.

1. **J1**: [...]
2. **J2**: [...]

## Failure signals

Prose. Things that must NOT have happened, regardless of whether J1/J2 above read as satisfied.

- [...]

## Verdict log

- YYYY-MM-DD: PASS/FAIL — <note, run id, claude version>
