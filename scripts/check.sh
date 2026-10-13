#!/usr/bin/env bash
# Local CI entry point. No GitHub Actions involved — this is the same check a
# developer runs before a commit (`fast`) and the same check that proves a
# task finished (`ai`).
#
#   scripts/check.sh fast                 gofmt -l, go vet, go test — seconds
#   scripts/check.sh ai [scenario ...]     fast, then every e2e-ai scenario
#                                          (default: all e2e-ai/scenarios/[0-9]*.md)
#   scripts/check.sh ai 01-anchors --record   same, but records the verdict
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd "$HERE/.." && pwd)"
cd "$REPO"

usage() {
  cat >&2 <<'EOF'
usage:
  scripts/check.sh fast
  scripts/check.sh ai [scenario ...] [--record]
EOF
  exit 1
}

run_fast() {
  echo "==> gofmt -l ."
  local unformatted
  unformatted="$(gofmt -l .)"
  if [ -n "$unformatted" ]; then
    printf 'not gofmt-ed:\n%s\n' "$unformatted" >&2
    return 1
  fi
  echo "    clean"

  echo "==> go vet ./..."
  go vet ./...

  echo "==> go test ./..."
  go test ./...
}

run_ai() {
  local record=0
  local scenarios=()
  for arg in "$@"; do
    if [ "$arg" = "--record" ]; then
      record=1
    else
      scenarios+=("$arg")
    fi
  done

  echo "==> scripts/check.sh fast (precondition for ai)"
  run_fast

  echo "==> e2e-ai/lab/refresh.sh"
  "$REPO/e2e-ai/lab/refresh.sh"

  if [ "${#scenarios[@]}" -eq 0 ]; then
    local f
    for f in "$REPO"/e2e-ai/scenarios/[0-9]*.md; do
      [ -e "$f" ] || continue
      scenarios+=("$(basename "$f")")
    done
  fi

  if [ "${#scenarios[@]}" -eq 0 ]; then
    echo "no AI scenarios found under e2e-ai/scenarios/" >&2
    return 1
  fi

  local record_flag=()
  if [ "$record" -eq 1 ]; then
    record_flag=(--record)
  fi

  local failures=0
  local name
  for name in "${scenarios[@]}"; do
    local path="$name"
    case "$path" in
      */*) : ;;
      *) path="$REPO/e2e-ai/scenarios/$name" ;;
    esac
    case "$path" in
      *.md) : ;;
      *) path="$path.md" ;;
    esac

    echo "==> e2e-ai/run.sh $path ${record_flag[*]:-}"
    if ! "$REPO/e2e-ai/run.sh" "$path" "${record_flag[@]}"; then
      echo "    FAIL: $path" >&2
      failures=$((failures + 1))
    fi
  done

  if [ "$failures" -gt 0 ]; then
    echo "==> $failures scenario(s) failed" >&2
    return 1
  fi
  echo "==> all scenarios passed"
}

[ $# -ge 1 ] || usage

cmd="$1"
shift
case "$cmd" in
  fast)
    [ $# -eq 0 ] || usage
    run_fast
    ;;
  ai)
    run_ai "$@"
    ;;
  *)
    usage
    ;;
esac
