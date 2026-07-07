#!/bin/sh
# PreToolUse guard for frozen paths. Reads the tool-call JSON on stdin,
# extracts file_path, blocks edits to contract files unless the session
# explicitly opted in via ALLOW_FROZEN=1 (which itself signals that a
# human decision was made). Exit 2 blocks the tool call and returns
# stderr to Claude (verify semantics against code.claude.com/docs/en/hooks).
INPUT=$(cat)
FP=$(printf '%s' "$INPUT" | grep -o '"file_path"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | sed 's/.*:[[:space:]]*"//;s/"$//')
[ -z "$FP" ] && exit 0
case "$FP" in
  *docs/rfc-*.md|*docs/vectors/*.json|*kernel/canon/*)
    if [ "$ALLOW_FROZEN" = "1" ]; then exit 0; fi
    echo "BLOCKED: $FP is a frozen contract (CLAUDE.md rule 5 / golden-file law)." >&2
    echo "If this change is genuinely intended: draft an ADR, get owner sign-off," >&2
    echo "then re-run with ALLOW_FROZEN=1 and cite the ADR in the commit." >&2
    exit 2 ;;
esac
exit 0
