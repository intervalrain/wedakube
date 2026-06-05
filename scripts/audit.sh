#!/usr/bin/env bash
# Audit working tree + full git history for likely-sensitive strings.
# Exit 0 = clean; exit 1 = leak found.
#
# Working-tree scan respects .gitignore (only files git would commit).
# History scan covers commit messages AND patches (additions and removals)
# because GitHub keeps both in the public record.
#
# Patterns:
#   - built-in shape-only (safe to commit): API-key shape, UUID shape
#   - optional .audit-patterns file (gitignored): one regex per line for
#     org-specific names without committing them into this repo
#
# Allowlist: well-known placeholder UUIDs are ignored (00000000-..., 12345678-...).
set -eo pipefail
cd "$(git rev-parse --show-toplevel)"

BUILTIN='\bAK[A-Z0-9]{30,}\b|\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b'
if [ -f .audit-patterns ]; then
  EXTRA=$(grep -vE '^\s*(#|$)' .audit-patterns | paste -sd'|' -)
  [ -n "$EXTRA" ] && PATTERN="$BUILTIN|$EXTRA" || PATTERN="$BUILTIN"
else
  PATTERN="$BUILTIN"
fi

# Placeholder UUIDs / values that show up in docs/examples and are NOT secrets.
ALLOWLIST='00000000-0000-0000-0000-[0-9a-f]+|12345678-1234-1234-1234-[0-9a-f]+|<TENANT_ID>|<API_KEY>'

filter() { grep -vE "$ALLOWLIST" || true; }
leak=0

echo "─── working tree (tracked + untracked-not-ignored) ───"
if matches=$(git ls-files --cached --others --exclude-standard -z 2>/dev/null \
               | xargs -0 grep -nHEi "$PATTERN" 2>/dev/null | filter); \
   [ -n "$matches" ]; then
  printf '%s\n' "$matches"; leak=1
else
  echo "  clean"
fi

echo "─── commit messages ───"
if matches=$(git log --all --format='%h %B' 2>/dev/null | grep -nEi "$PATTERN" | filter); \
   [ -n "$matches" ]; then
  printf '%s\n' "$matches" | head -20; leak=1
else
  echo "  clean"
fi

echo "─── patches across history ───"
if matches=$(git log --all -p 2>/dev/null | grep -nEi "$PATTERN" | filter); \
   [ -n "$matches" ]; then
  printf '%s\n' "$matches" | head -20; leak=1
else
  echo "  clean"
fi

if [ "$leak" = "1" ]; then
  echo
  echo "✗ leaks detected. Sanitize before pushing." >&2
  echo "  - working tree:  edit files, drop sensitive values to env vars / placeholders" >&2
  echo "  - history:       git filter-repo --replace-text <file> (and --replace-message)" >&2
  exit 1
fi

echo "✓ safe to push"
