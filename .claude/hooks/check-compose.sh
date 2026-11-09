#!/usr/bin/env bash
# PostToolUse hook: validate lab conventions in compose.yaml files.
set -euo pipefail

file=$(jq -r '.tool_input.file_path // empty')
[[ "$(basename -- "$file")" == "compose.yaml" && -f "$file" ]] || exit 0

root="${CLAUDE_PROJECT_DIR:-$(pwd)}"
dir=$(realpath --relative-to="$root" "$(dirname -- "$file")")
errors=()

if grep -qE '^\s*restart:\s*' "$file" && grep -E '^\s*restart:' "$file" | grep -qvE 'restart:\s*"?no"?\s*$'; then
  errors+=("no restart policy allowed (stack starts only via make; crashes must stay visible)")
fi

if grep -nE '^\s*-\s*"?[0-9]+:[0-9]+' "$file" >/dev/null; then
  errors+=("published ports must bind to 127.0.0.1 (\"127.0.0.1:HOST:CONTAINER\")")
fi

if [[ "$dir" != "observability" ]]; then
  if ! grep -qE '^name:\s*lab-' "$file"; then
    errors+=("phase compose project must be named lab-... (top-level 'name: lab-<phase>')")
  fi
  if grep -qE '^\s*-\s*"?\.\.?/[^:]+:' "$file" &&
    ! grep -E '^CONFIG_DIRS\s*:?=' "$root/Makefile" | grep -qw -- "$dir"; then
    errors+=("$dir bind-mounts files: add it to CONFIG_DIRS in the Makefile (umask 077)")
  fi
fi

if ((${#errors[@]})); then
  echo "$file violates lab conventions:" >&2
  printf ' - %s\n' "${errors[@]}" >&2
  exit 2
fi
exit 0
