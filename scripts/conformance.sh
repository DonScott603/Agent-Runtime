#!/bin/sh
# The conformance gate (versioning.md §5; .claude/skills/conformance).
# Order: vectors + determinism (go test), fold ratchet, secret scan —
# plus gofmt/vet hygiene. Single source of truth: `make conformance`
# delegates here, and the script runs standalone on hosts without make.
set -u
cd "$(dirname "$0")/.."
fail=0

echo "== gofmt =="
UNFMT=$(gofmt -l . || true)
if [ -n "$UNFMT" ]; then
  echo "gofmt: FAIL — needs formatting:"; echo "$UNFMT"; fail=1
else
  echo "gofmt: clean"
fi

echo "== go vet =="
if go vet ./...; then echo "vet: clean"; else fail=1; fi

echo "== vectors + determinism (go test ./...) =="
if go test ./...; then echo "suites: green"; else fail=1; fi

echo "== fold ratchet =="
# Generation 0 freezes with the corpus generator (WP-10/WP-17).
echo "ratchet: PENDING — no frozen corpora yet (lands WP-10/WP-17)"

echo "== secret/PII scan (threat-model D16) =="
if grep -rinE 'BEGIN (RSA|EC|OPENSSH) PRIVATE KEY|api[_-]?key|password|@gmail\.com|@outlook\.com' corpus/ docs/vectors/; then
  echo "secret-scan: FAIL — purge the material and audit how it entered"; fail=1
else
  echo "secret-scan: clean"
fi

echo ""
if [ "$fail" -eq 0 ]; then
  echo "CONFORMANCE: GREEN"
else
  echo "CONFORMANCE: RED"
  exit 1
fi
