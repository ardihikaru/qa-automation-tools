#!/usr/bin/env bash
set -euo pipefail

mode="${1:---staged}"
if [[ "$mode" != "--staged" ]]; then
  echo "Usage: $0 [--staged]" >&2
  exit 2
fi

if ! git rev-parse --git-dir >/dev/null 2>&1; then
  echo "Not inside a Git repository." >&2
  exit 2
fi

invalid=0

while IFS= read -r -d '' path; do
  reason=""

  if [[ "$path" =~ [\<\>\:\"\\\|\?\*] ]]; then
    reason="contains Windows-invalid character (< > : \" / \\ | ? *)"
  elif [[ "$path" =~ [\.\ ]$ ]]; then
    reason="ends with a dot or space"
  else
    IFS='/' read -r -a parts <<< "$path"
    for part in "${parts[@]}"; do
      lower="$(printf '%s' "$part" | tr '[:upper:]' '[:lower:]')"
      if [[ "$lower" =~ ^(con|prn|aux|nul|com[1-9]|lpt[1-9])(\..*)?$ ]]; then
        reason="uses reserved Windows filename component: $part"
        break
      fi
    done
  fi

  if [[ -n "$reason" ]]; then
    invalid=1
    printf '  - %s (%s)\n' "$path" "$reason" >&2
  fi
done < <(git diff --cached --name-only -z --diff-filter=ACMR)

if [[ "$invalid" -ne 0 ]]; then
  cat >&2 <<'EOF'
Commit blocked: staged paths include filenames invalid on Windows.
Rename/remove the paths above, then re-stage and commit again.
EOF
  exit 1
fi

exit 0
