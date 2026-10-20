# AI flow scenario: 13-impact-files

## Task

Task 13 added a files mode to `impact`: `openrecord impact --files <file...>` — given one or more code
file paths, it resolves every declared component containing each one (not only the most specific; a
component declared as `.` covers everything the same way a narrower one does), then every decision
filed under one of those components and every spec naming one of them, plus — one hop further — every
quality spec that qualifies a spec already included even when it names none of those components
itself. It also unified record mode's vocabulary (`annotated_in`, `scenarios`, `verification`) and
added `--only` to trim sections in both modes. Settled and already implemented (see
`docs/cli.md#impact`, `internal/cli/impact.go`, `internal/store/components.go`'s `OwnersOf`): JSON only,
validated first over the whole store exactly like record mode. The deterministic half of this proof is
`e2e/impact_test.go`, exercising single/union/cross-component files, the one-hop quality inclusion, and
`--only` against the built binary over a store built from scratch. This file proves the same behaviour
through a real agent asked what to respect before touching two files that cross two components — it has
to run `impact --files` rather than reconstruct the containment by hand, name the decisions that apply
including the one still `pending`, and point out the spec both files share along with the one scenario
nobody has a test for yet.

## Lab precondition

```setup
mkdir -p src/api/handlers src/web/upload tests

cat > src/api/handlers/upload.go <<'EOF'
package handlers

// Upload handles a file upload request.
func Upload() {}
EOF

cat > src/web/upload/form.tsx <<'EOF'
export function UploadForm() {
  return null;
}
EOF

openrecord component add root --path . --title "Root" --description "The whole repository, not owned by any one surface."
openrecord component add api --path src/api --title "API" --description "The HTTP surface."
openrecord component add web --path src/web --title "Web" --description "The browser surface."

openrecord actor add visitor --kind human --description "A person with no account yet."

openrecord level add decisions/root/structure --title "Structure" --description "Base decisions about the implementation itself."
openrecord level add decisions/api/uploads --title "Uploads" --description "How the API accepts and stores uploaded files."
openrecord level add decisions/web/ui --title "UI" --description "How the web surface presents itself to a visitor."
openrecord level add specs/flow --title "Flows" --description "What a visitor can do, start to finish."

openrecord record write decisions/root/structure/go.md \
  --title "Go is the implementation language" \
  --description "The service is written in Go." \
  --status accepted \
  --body-file <(cat <<'EOF'
## Context

The service needs one implementation language, consistent across every surface.

## Decision

The service is written in Go.

## Alternatives

A mixed-language backend was considered and dropped: it buys nothing here and taxes every future hire.

## Consequences

Every backend package is Go; a build step that is not `go build` does not belong in this repository.
EOF
)

openrecord record write decisions/api/uploads/allowed-formats.md \
  --title "Only images and plain documents are accepted" \
  --description "An upload is accepted only when its format is on the allow-list; PDF is not on it yet." \
  --status accepted \
  --body-file <(cat <<'EOF'
## Context

Accepting an arbitrary file format widens what has to be scanned, rendered and stored safely.

## Decision

An upload is accepted only when its format is on an explicit allow-list, checked before it is stored.

## Alternatives

Accepting anything and sniffing the content type after the fact was considered and dropped: it means
storing the file before anything has judged whether it should have been accepted at all.

## Consequences

Adding a new accepted format means changing this decision and the allow-list it describes, not just the
upload handler.
EOF
)

openrecord record write decisions/api/uploads/virus-scan.md \
  --title "Whether uploads need a virus scan" \
  --description "Not decided: whether an uploaded file is scanned for malware before it is stored." \
  --status pending \
  --body-file <(cat <<'EOF'
## Context

A stored file that turns out to carry malware is a risk to everyone who later downloads it.

## Decision

Undecided. Whoever picks this up next needs to weigh the latency a scan adds against the risk of
skipping it.

## Alternatives

Storing uploads unscanned, unchanged, is the default until this is decided.

## Consequences

None yet — nothing has been chosen.
EOF
)

openrecord record write decisions/web/ui/upload-button.md \
  --title "The upload button disables while uploading" \
  --description "The upload control disables itself for the duration of the request." \
  --status accepted \
  --body-file <(cat <<'EOF'
## Context

A visitor clicking upload twice before the first request finishes can submit the same file twice.

## Decision

The upload button disables itself the moment a request starts, and re-enables only once it settles.

## Alternatives

Debouncing the click handler was considered and dropped: it still allows a second request if the
visitor waits past the debounce window.

## Consequences

Every upload surface in the web UI reuses the same disable-on-submit behaviour, not a bespoke guard.
EOF
)

openrecord record write specs/flow/upload-file.md \
  --title "Upload a file" \
  --description "A visitor uploads a file through the API and sees it reflected in the web UI." \
  --status accepted --components api,web --initiated-by visitor \
  --body-file <(cat <<'EOF'
## Purpose

A @actor:visitor uploads a file through the API and sees it reflected in the web UI.

## Main flow

1. The @actor:visitor selects a file and submits it.
2. The system stores the file and shows it in the UI.

## Scenarios

### Scenario: Upload succeeds

- **GIVEN** a @actor:visitor with a file in an allowed format
- **WHEN** they submit it
- **THEN** the file is stored and shown in the UI

### Scenario: Format not allowed

- **GIVEN** a @actor:visitor with a file in a disallowed format
- **WHEN** they submit it
- **THEN** the upload is rejected before it reaches storage
EOF
)

cat > tests/upload.spec.ts <<'EOF'
// @record:specs/flow/upload-file.md#upload-succeeds
EOF

git init -q
git add -A
git -c user.name="orlab" -c user.email="orlab@local" commit -q -m \
  "initial store: root/api/web components, an allow-list decision, a pending virus-scan decision, upload-file flow"
```

## Prompts

```prompt
I'm about to change `src/api/handlers/upload.go` and `src/web/upload/form.tsx` so uploads accept PDFs.
What do I need to respect, and what's missing? Don't change anything yet.
```

## Checked criteria

```check
# C1: the transcript shows at least one Bash tool_use running
# `openrecord impact --files` naming both files — in one call or several —
# inspecting tool_use commands only, never assistant prose.
python3 - "$TRANSCRIPT" <<'PY'
import json
import sys

path = sys.argv[1]
saw_api = False
saw_web = False
with open(path, encoding="utf-8") as f:
    for line in f:
        line = line.strip()
        if not line:
            continue
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        if event.get("type") != "assistant":
            continue
        for block in (event.get("message") or {}).get("content") or []:
            if block.get("type") != "tool_use" or block.get("name") != "Bash":
                continue
            command = (block.get("input") or {}).get("command") or ""
            if "openrecord impact" not in command or "--files" not in command:
                continue
            if "src/api/handlers/upload.go" in command:
                saw_api = True
            if "src/web/upload/form.tsx" in command:
                saw_web = True
if not (saw_api and saw_web):
    print(f"did not find `openrecord impact --files` naming both files "
          f"(api seen: {saw_api}, web seen: {saw_web})")
    sys.exit(1)
sys.exit(0)
PY
```

```check
# C2: nothing under .openrecord/ changed — the task only asked what to
# respect and what's missing, never to change anything.
if [ -n "$(git status --porcelain -- .openrecord)" ]; then
  echo "the store changed, and the task said not to change anything:"
  git status --porcelain -- .openrecord
  git diff -- .openrecord
  exit 1
fi
```

## Judged criteria

1. **J1**: The model's final answer names `decisions/root/structure/go.md` (the root decision, since
   root is declared as `.` and covers both files), `decisions/api/uploads/allowed-formats.md` (the api
   decision, directly relevant to accepting a new format), and `decisions/web/ui/upload-button.md` (the
   web decision) — every decision `impact --files` returns for these two files, not only the ones that
   sound topical.
2. **J2**: The model treats `decisions/api/uploads/virus-scan.md` as `pending` — undecided, not yet
   something to implement or assume either way — and does not add a virus scan as part of accepting
   PDFs. Implementing it, or describing it as already required, is a failure.
3. **J3**: The model's final answer names `specs/flow/upload-file.md` as the spec both files share, and
   points out that its `format-not-allowed` scenario has no annotated test — `upload-succeeds` does
   (`tests/upload.spec.ts`), but the scenario a PDF would exercise before the allow-list is updated does
   not.

## Failure signals

- The model never ran `openrecord impact --files`, and instead used `component owners`, `map`, or a
  hand search over the store to answer.
- The final answer omits any of the three decisions in J1, or omits `decisions/web/ui/upload-button.md`
  specifically because the model only looked at the api side.
- The final answer describes `decisions/api/uploads/virus-scan.md` as settled, already required, or
  already implemented — rather than as a `pending`, undecided question.
- The model started adding PDF support, a virus scan, or a test for `format-not-allowed` itself, rather
  than only reporting what to respect and what's missing.
- Anything under `.openrecord/` was written, edited, or otherwise changed.

## Verdict log

- 2026-10-20: PASS — checked: 2/2  judged: 3/3. Run 20261020T174421Z-13-impact-files. 2.1.296 (Claude Code) as orlab.
