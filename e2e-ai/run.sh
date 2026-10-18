#!/usr/bin/env bash
# e2e-ai/run.sh — drive one AI flow scenario end to end, as the persistent
# orlab lab user, headless.
#
#   e2e-ai/run.sh <scenario.md> [--record]
#
# What it does, in order:
#   1. parses the scenario file (## Lab precondition / Prompts / Checked
#      criteria / Judged criteria / Failure signals)
#   2. creates a fresh run dir under ~orlab/runs/<UTC timestamp>-<slug>
#   3. runs the lab precondition as orlab, then `openrecord skills --emit`
#   4. sends each prompt to one Claude Code session (first via `claude -p`,
#      later ones via `--resume <session_id>`), headless, full transcript
#   5. runs every `check` block as orlab, with $ANSWER_N / $FINAL_ANSWER /
#      $TRANSCRIPT / $SNAPSHOT_N exported
#   6. judges the Judged criteria + Failure signals with a second, separate
#      `claude -p` run, as orlab, in an empty directory
#   7. prints a per-criterion report and exits 0 only if everything passed
#   8. with --record, appends one dated line to the scenario's Verdict log
#
# Never hand-edits anything under the lab project's .openrecord/ — the lab
# precondition and every prompt only ever use `openrecord` itself.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd "$HERE/.." && pwd)"
LAB_USER=orlab
ORLAB_HOME="/home/$LAB_USER"

usage() {
  echo "usage: e2e-ai/run.sh <scenario.md> [--record]" >&2
  exit 1
}

[ $# -ge 1 ] || usage
scenario_arg="$1"; shift
record=0
for arg in "$@"; do
  case "$arg" in
    --record) record=1 ;;
    *) usage ;;
  esac
done

name="$scenario_arg"
case "$name" in
  *.md) : ;;
  *) name="$name.md" ;;
esac
case "$name" in
  /*) scenario_path="$name" ;;
  *)
    if [ -f "$name" ]; then
      scenario_path="$(cd "$(dirname "$name")" && pwd)/$(basename "$name")"
    else
      scenario_path="$REPO/e2e-ai/scenarios/$name"
    fi
    ;;
esac
[ -f "$scenario_path" ] || { echo "error: scenario not found: $scenario_path" >&2; exit 1; }
scenario_name="$(basename "$scenario_path" .md)"

echo "==> preflight: orlab reachable"
if ! sudo -n -u "$LAB_USER" true 2>/dev/null; then
  echo "error: 'sudo -n -u $LAB_USER true' failed. Run: sudo bash e2e-ai/lab/setup.sh" >&2
  exit 1
fi

echo "==> preflight: orlab Claude Code login"
auth_status="$(sudo -n -iu "$LAB_USER" bash -lc 'claude auth status' 2>&1 || true)"
if ! grep -q '"loggedIn": *true' <<<"$auth_status"; then
  cat >&2 <<EOF
error: orlab's Claude Code session is not logged in. This is the one manual
step that cannot be scripted (see e2e-ai/README.md):

    sudo -iu orlab claude
    /login

Observed:
$auth_status
EOF
  exit 1
fi
claude_version="$(sudo -n -iu "$LAB_USER" bash -lc 'claude --version' 2>&1 || echo unknown)"
echo "    orlab logged in, claude $claude_version"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

echo "==> parsing $scenario_path"
python3 - "$scenario_path" "$work" <<'PY'
import os
import re
import sys

path, work = sys.argv[1], sys.argv[2]
text = open(path, encoding="utf-8").read()


def blocks(tag):
    return re.findall(r"```" + tag + r"\n(.*?)\n```", text, re.S)


setup = blocks("setup")
if len(setup) != 1:
    sys.exit(f"error: expected exactly one ```setup``` block, found {len(setup)}")
open(os.path.join(work, "setup.sh"), "w", encoding="utf-8").write(setup[0] + "\n")

prompts = blocks("prompt")
if not prompts:
    sys.exit("error: expected at least one ```prompt``` block")
os.makedirs(os.path.join(work, "prompts"), exist_ok=True)
for i, p in enumerate(prompts, 1):
    open(os.path.join(work, "prompts", f"{i}.txt"), "w", encoding="utf-8").write(p.strip() + "\n")

checks = blocks("check")
os.makedirs(os.path.join(work, "checks"), exist_ok=True)
meta = []
for i, c in enumerate(checks, 1):
    first_line = c.split("\n", 1)[0].strip()
    m = re.match(r"#\s*(C\d+)\s*:\s*(.*)", first_line)
    if not m:
        sys.exit(f"error: check block {i} must start with '# CN: description', got: {first_line!r}")
    cid, desc = m.group(1), m.group(2).strip()
    filename = f"{i:02d}-{cid}.sh"
    open(os.path.join(work, "checks", filename), "w", encoding="utf-8").write(c + "\n")
    meta.append((filename, cid, desc))
with open(os.path.join(work, "checks_meta.tsv"), "w", encoding="utf-8") as f:
    for filename, cid, desc in meta:
        f.write(f"{filename}\t{cid}\t{desc}\n")


def section(heading):
    m = re.search(r"^## " + re.escape(heading) + r"\s*\n(.*?)(?=\n## |\Z)", text, re.S | re.M)
    return m.group(1).strip() if m else ""


open(os.path.join(work, "judged.txt"), "w", encoding="utf-8").write(section("Judged criteria"))
open(os.path.join(work, "failure_signals.txt"), "w", encoding="utf-8").write(section("Failure signals"))
PY

num_prompts="$(ls "$work/prompts" 2>/dev/null | wc -l | tr -d ' ')"
num_checks="$(ls "$work/checks" 2>/dev/null | wc -l | tr -d ' ')"
echo "    $num_prompts prompt(s), $num_checks check(s)"

run_id="$(date -u +%Y%m%dT%H%M%SZ)-$scenario_name"
rundir="$ORLAB_HOME/runs/$run_id"
# The project under test lives in its own subdirectory, and everything the
# runner keeps (transcripts, snapshots, checks, judge input) stays beside it,
# never inside it: the binary scans the project's files for @record:
# annotations, so a transcript quoting an example would read as the project's.
projdir="$rundir/project"
artifacts_dir="$REPO/e2e-ai/.runs/$run_id"
echo "==> run id: $run_id"
sudo -n -u "$LAB_USER" mkdir -p "$projdir"
mkdir -p "$artifacts_dir"

echo "==> lab precondition"
sudo -n -u "$LAB_USER" tee "$rundir/.setup.sh" >/dev/null < "$work/setup.sh"
sudo -n -iu "$LAB_USER" bash -lc "cd '$projdir' && set -euo pipefail && bash '$rundir/.setup.sh'"

echo "==> openrecord skills --emit"
sudo -n -iu "$LAB_USER" bash -lc "cd '$projdir' && mkdir -p .claude/skills && openrecord skills --emit .claude/skills/"

snapshot() {
  local n="$1"
  sudo -n -u "$LAB_USER" mkdir -p "$rundir/snapshots/$n"
  sudo -n -u "$LAB_USER" rsync -a "$projdir/" "$rundir/snapshots/$n/"
}

echo "==> snapshot 0 (before any prompt)"
snapshot 0

# claude flags shared by every prompt in this session. acceptEdits auto-accepts
# Edit/Write so a hand-edit (the thing scenarios check for) can actually happen
# instead of silently permission-denying into a false pass; --allowedTools
# pre-approves Bash too, since openrecord itself only runs through it. Nothing
# more permissive than this was needed: no scenario here needs Task, WebFetch,
# or any tool outside this short list.
CLAUDE_FLAGS=(-p --output-format stream-json --verbose --permission-mode acceptEdits --allowedTools "Bash Edit Write Read Glob Grep")

quote_args() {
  local out="" a qa
  for a in "$@"; do
    printf -v qa '%q' "$a"
    out+="$qa "
  done
  printf '%s' "$out"
}

session_id=""
for i in $(seq 1 "$num_prompts"); do
  echo "==> prompt $i"
  args=("${CLAUDE_FLAGS[@]}")
  if [ "$i" -gt 1 ]; then
    args+=(--resume "$session_id")
  fi
  quoted="$(quote_args "${args[@]}")"
  stream_out="$rundir/prompt-$i.stream.json"
  stderr_out="$rundir/prompt-$i.stderr.log"

  if ! sudo -n -iu "$LAB_USER" bash -lc "cd '$projdir' && claude $quoted > '$stream_out' 2> '$stderr_out'" < "$work/prompts/$i.txt"; then
    echo "error: claude invocation failed for prompt $i (environment/sudo failure, not a model result) — see $stderr_out" >&2
    sudo -n -u "$LAB_USER" cat "$stderr_out" >&2 || true
    exit 1
  fi

  session_file="$rundir/prompt-$i.session_id"
  answer_file="$rundir/prompt-$i.answer.txt"
  denials_file="$rundir/prompt-$i.denials.txt"
  if ! sudo -n -u "$LAB_USER" python3 - "$stream_out" "$session_file" "$answer_file" "$denials_file" <<'PY'
import json
import sys

stream_path, session_path, answer_path, denials_path = sys.argv[1:5]
session_id = None
answer = None
is_error = True
denials = []
with open(stream_path, encoding="utf-8") as f:
    for line in f:
        line = line.strip()
        if not line:
            continue
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        sid = event.get("session_id")
        if sid:
            session_id = sid
        if event.get("type") == "result":
            answer = event.get("result", "")
            is_error = bool(event.get("is_error", False))
            denials = event.get("permission_denials") or []

open(session_path, "w", encoding="utf-8").write(session_id or "")
open(answer_path, "w", encoding="utf-8").write(answer or "")
open(denials_path, "w", encoding="utf-8").write(json.dumps(denials))
sys.exit(1 if (is_error or not session_id) else 0)
PY
  then
    echo "error: prompt $i did not complete successfully — see $stream_out / $stderr_out" >&2
    echo "    last result: $(sudo -n -u "$LAB_USER" cat "$answer_file" 2>/dev/null)" >&2
    exit 1
  fi
  session_id="$(sudo -n -u "$LAB_USER" cat "$session_file")"
  denials="$(sudo -n -u "$LAB_USER" cat "$denials_file")"
  if [ "$denials" != "[]" ]; then
    echo "    warning: permission_denials for prompt $i: $denials" >&2
  fi

  echo "==> snapshot $i (after prompt $i)"
  snapshot "$i"
done

echo "==> building transcript"
sudo -n -iu "$LAB_USER" bash -lc "cd '$rundir' && cat \$(seq -f 'prompt-%g.stream.json' 1 $num_prompts) > transcript.ndjson"

echo "==> exporting check environment"
sudo -n -u "$LAB_USER" python3 - "$rundir" "$num_prompts" <<'PY'
import os
import shlex
import sys

rundir, num_prompts = sys.argv[1], int(sys.argv[2])
lines = []
last_answer = ""
for i in range(1, num_prompts + 1):
    p = os.path.join(rundir, f"prompt-{i}.answer.txt")
    text = open(p, encoding="utf-8").read() if os.path.exists(p) else ""
    last_answer = text
    lines.append(f"export ANSWER_{i}={shlex.quote(text)}")
lines.append(f"export FINAL_ANSWER={shlex.quote(last_answer)}")
lines.append(f"export TRANSCRIPT={shlex.quote(os.path.join(rundir, 'transcript.ndjson'))}")
for i in range(0, num_prompts + 1):
    d = os.path.join(rundir, "snapshots", str(i))
    if os.path.isdir(d):
        lines.append(f"export SNAPSHOT_{i}={shlex.quote(d)}")
with open(os.path.join(rundir, ".checks-env.sh"), "w", encoding="utf-8") as f:
    f.write("\n".join(lines) + "\n")
PY

echo "==> running checks"
sudo -n -u "$LAB_USER" mkdir -p "$rundir/checks"
: > "$work/check_results.tsv"
while IFS=$'\t' read -r filename cid desc; do
  [ -n "$filename" ] || continue
  sudo -n -u "$LAB_USER" tee "$rundir/checks/$filename" >/dev/null < "$work/checks/$filename"
  echo "    $cid: $desc"
  if sudo -n -iu "$LAB_USER" bash -lc "cd '$projdir' && set -a && . '$rundir/.checks-env.sh' && set +a && bash '$rundir/checks/$filename'" \
      > "$artifacts_dir/check-$cid.stdout.log" 2> "$artifacts_dir/check-$cid.stderr.log"; then
    echo "        PASS"
    printf '%s\tPASS\t%s\n' "$cid" "$desc" >> "$work/check_results.tsv"
  else
    echo "        FAIL"
    printf '%s\tFAIL\t%s\n' "$cid" "$desc" >> "$work/check_results.tsv"
  fi
done < "$work/checks_meta.tsv"

echo "==> judging"
sudo -n -u "$LAB_USER" tee "$rundir/judged.txt" >/dev/null < "$work/judged.txt"
sudo -n -u "$LAB_USER" tee "$rundir/failure_signals.txt" >/dev/null < "$work/failure_signals.txt"
sudo -n -u "$LAB_USER" tee "$rundir/check_results.tsv" >/dev/null < "$work/check_results.tsv"

sudo -n -u "$LAB_USER" python3 - "$rundir" <<'PY'
import json
import os
import sys

rundir = sys.argv[1]
transcript_path = os.path.join(rundir, "transcript.ndjson")


def trunc(value, n=500):
    s = value if isinstance(value, str) else json.dumps(value)
    return s if len(s) <= n else s[:n] + "...[truncated]"


digest_lines = []
with open(transcript_path, encoding="utf-8") as f:
    for line in f:
        line = line.strip()
        if not line:
            continue
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        etype = event.get("type")
        if etype == "assistant":
            for block in (event.get("message") or {}).get("content") or []:
                if block.get("type") == "tool_use":
                    digest_lines.append(f"TOOL_USE {block.get('name')}: {trunc(block.get('input'))}")
                elif block.get("type") == "text" and block.get("text"):
                    digest_lines.append(f"ASSISTANT_TEXT: {trunc(block.get('text'))}")
        elif etype == "user":
            for block in (event.get("message") or {}).get("content") or []:
                if isinstance(block, dict) and block.get("type") == "tool_result":
                    digest_lines.append(f"TOOL_RESULT: {trunc(block.get('content'))}")
        elif etype == "result":
            # Never truncated: the final answer is what most judged criteria
            # are about, and a cut one reads as an omission to the judge.
            digest_lines.append(f"FINAL_ANSWER: {event.get('result', '')}")

digest = "\n".join(digest_lines)
judged = open(os.path.join(rundir, "judged.txt"), encoding="utf-8").read()
failure_signals = open(os.path.join(rundir, "failure_signals.txt"), encoding="utf-8").read()

checks_text_lines = []
checks_path = os.path.join(rundir, "check_results.tsv")
if os.path.exists(checks_path):
    for row in open(checks_path, encoding="utf-8"):
        row = row.rstrip("\n")
        if not row:
            continue
        parts = row.split("\t", 2)
        cid, result, desc = (parts + ["", "", ""])[:3]
        checks_text_lines.append(f"{cid}: {result} — {desc}")
checks_text = "\n".join(checks_text_lines)

prompt = f"""You are judging a transcript of an AI coding agent using a CLI tool called `openrecord`, against prose criteria written in advance. You will get a digest of the transcript (tool calls with their inputs, truncated tool results, assistant text, and the final answer of each turn), the criteria already checked by a script (for context only, do not re-judge them), the judged criteria (numbered, J1 J2 ...), and failure signals that must not appear regardless of how the judged criteria read.

Respond with ONLY strict JSON, no prose before or after it, matching exactly this shape:
{{"criteria":[{{"id":"J1","pass":true,"evidence":"..."}}],"failure_signals_seen":["..."]}}

Judge every numbered criterion independently, with concrete evidence paraphrased or quoted from the digest below — never a generic impression. List in failure_signals_seen only signals you find actual evidence for in the digest; otherwise return an empty array.

## Judged criteria
{judged}

## Failure signals
{failure_signals}

## Already-checked criteria (context only)
{checks_text}

## Transcript digest
{digest}
"""

open(os.path.join(rundir, "judge-input.txt"), "w", encoding="utf-8").write(prompt)
PY

judge_tmp="$(sudo -n -u "$LAB_USER" mktemp -d)"
judge_schema='{"type":"object","properties":{"criteria":{"type":"array","items":{"type":"object","properties":{"id":{"type":"string"},"pass":{"type":"boolean"},"evidence":{"type":"string"}},"required":["id","pass","evidence"]}},"failure_signals_seen":{"type":"array","items":{"type":"string"}}},"required":["criteria","failure_signals_seen"]}'
judge_args=(-p --output-format stream-json --verbose --tools "" --json-schema "$judge_schema")
judge_quoted="$(quote_args "${judge_args[@]}")"

if ! sudo -n -iu "$LAB_USER" bash -lc "cd '$judge_tmp' && claude $judge_quoted < '$rundir/judge-input.txt' > '$rundir/judge.stream.json' 2> '$rundir/judge.stderr.log'"; then
  echo "error: judge invocation failed (environment/sudo failure) — see $rundir/judge.stderr.log" >&2
  sudo -n -u "$LAB_USER" rm -rf "$judge_tmp"
  exit 1
fi
sudo -n -u "$LAB_USER" rm -rf "$judge_tmp"

if ! sudo -n -u "$LAB_USER" python3 - "$rundir/judge.stream.json" "$rundir/judge-result.json" <<'PY'
import json
import re
import sys

stream_path, out_path = sys.argv[1], sys.argv[2]
result_text = None
with open(stream_path, encoding="utf-8") as f:
    for line in f:
        line = line.strip()
        if not line:
            continue
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        if event.get("type") == "result":
            result_text = event.get("result", "")

if result_text is None:
    sys.exit("error: judge produced no result event")

parsed = None
try:
    parsed = json.loads(result_text)
except json.JSONDecodeError:
    m = re.search(r"\{.*\}", result_text, re.S)
    if m:
        try:
            parsed = json.loads(m.group(0))
        except json.JSONDecodeError:
            parsed = None

if parsed is None:
    with open(out_path, "w", encoding="utf-8") as f:
        json.dump({"criteria": [], "failure_signals_seen": [], "_parse_error": True, "_raw": result_text}, f)
    sys.exit(1)

with open(out_path, "w", encoding="utf-8") as f:
    json.dump(parsed, f)
PY
then
  echo "error: could not parse judge output as the required JSON shape — see $artifacts_dir/judge-result.json" >&2
fi

echo "==> copying artifacts into $artifacts_dir"
for i in $(seq 1 "$num_prompts"); do
  for suffix in stream.json stderr.log answer.txt; do
    sudo -n -u "$LAB_USER" cat "$rundir/prompt-$i.$suffix" > "$artifacts_dir/prompt-$i.$suffix" 2>/dev/null || true
  done
done
for f in transcript.ndjson judge-input.txt judge.stream.json judge.stderr.log judge-result.json check_results.tsv; do
  sudo -n -u "$LAB_USER" cat "$rundir/$f" > "$artifacts_dir/$f" 2>/dev/null || true
done
cp "$work/check_results.tsv" "$artifacts_dir/check_results.tsv" 2>/dev/null || true
echo "$run_id" > "$artifacts_dir/run_id.txt"

judge_json="$(cat "$artifacts_dir/judge-result.json" 2>/dev/null || echo '{"criteria":[],"failure_signals_seen":[]}')"

report_file="$artifacts_dir/report.txt"
if ! python3 - "$judge_json" "$work/check_results.tsv" "$run_id" "$claude_version" > "$report_file" <<'PY'
import json
import sys

judge_json, checks_path, run_id, claude_version = sys.argv[1:5]
try:
    data = json.loads(judge_json)
except json.JSONDecodeError:
    data = {"criteria": [], "failure_signals_seen": []}

lines = []
overall_pass = True
checked_total = checked_pass = 0
with open(checks_path, encoding="utf-8") as f:
    for row in f:
        row = row.rstrip("\n")
        if not row:
            continue
        parts = row.split("\t", 2)
        cid, result, desc = (parts + ["", "", ""])[:3]
        checked_total += 1
        if result == "PASS":
            checked_pass += 1
        else:
            overall_pass = False
        lines.append(f"{cid}: {result} — {desc}")

judged_total = judged_pass = 0
for c in data.get("criteria", []):
    judged_total += 1
    status = "PASS" if c.get("pass") else "FAIL"
    if c.get("pass"):
        judged_pass += 1
    else:
        overall_pass = False
    lines.append(f"{c.get('id')}: {status} — {c.get('evidence', '')}")

signals = data.get("failure_signals_seen") or []
if signals:
    overall_pass = False
    lines.append("failure signals seen: " + ", ".join(signals))
else:
    lines.append("failure signals seen: none")

if data.get("_parse_error"):
    overall_pass = False
    lines.append("judge output did not parse as the required JSON shape")

lines.append(f"checked: {checked_pass}/{checked_total}  judged: {judged_pass}/{judged_total}")
lines.append(f"run id: {run_id}  claude: {claude_version}")
lines.append("OVERALL: " + ("PASS" if overall_pass else "FAIL"))
print("\n".join(lines))
sys.exit(0 if overall_pass else 1)
PY
then
  overall_rc=1
else
  overall_rc=0
fi

cat "$report_file"

if [ "$record" -eq 1 ]; then
  overall_word="PASS"
  [ "$overall_rc" -eq 0 ] || overall_word="FAIL"
  counts_line="$(grep -E '^checked: ' "$report_file" || true)"
  date_str="$(date -u +%Y-%m-%d)"
  verdict_line="- $date_str: $overall_word — $counts_line. Run $run_id. $claude_version as orlab."
  printf '\n%s\n' "$verdict_line" >> "$scenario_path"
  echo "==> recorded verdict in $scenario_path"
fi

exit "$overall_rc"
