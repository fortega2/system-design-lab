#!/usr/bin/env bash
# PreToolUse hook: show what a git commit will include and make Claude confirm
# it by re-running the same commit. Blocks staging and committing in one call,
# where the list can't be known in advance.
set -euo pipefail

cmd=$(jq -r '.tool_input.command // empty')
grep -qE '(^|[;&|[:space:]])git( -C [^ ]+)? commit\b' <<<"$cmd" || exit 0

if grep -qE '(^|[;&|[:space:]])git( -C [^ ]+)? add\b' <<<"$cmd"; then
  echo "Blocked: run 'git add' and 'git commit' in separate calls, so the files to commit can be reviewed first." >&2
  exit 2
fi

root="${CLAUDE_PROJECT_DIR:-$(pwd)}"
cd "$root"

if grep -qE 'git( -C [^ ]+)? commit\b.*( -a\b| -[a-zA-Z]*a[a-zA-Z]*\b| --all\b)' <<<"$cmd"; then
  files=$(git diff --name-status HEAD)
else
  files=$(git diff --cached --name-status)
fi
untracked=$(git ls-files --others --exclude-standard)

state="$(git rev-parse --git-dir)/claude-commit-review"
sum=$(printf '%s' "$files" | sha256sum | cut -d' ' -f1)

if [[ -f "$state" && "$(cat "$state")" == "$sum" ]]; then
  rm -f "$state"
  exit 0
fi
echo "$sum" >"$state"

{
  echo "Review before committing. This commit will include:"
  echo "${files:-(nothing staged)}" | sed 's/^/  /'
  if [[ -n "$untracked" ]]; then
    echo "Untracked (not included):"
    echo "$untracked" | sed 's/^/  /'
  fi
  echo "If every file belongs in this commit, run the commit again; otherwise fix the index first."
} >&2
exit 2
