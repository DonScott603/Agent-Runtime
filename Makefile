# Run before declaring any work package done (CLAUDE.md working rules).
.PHONY: conformance test

conformance:
	sh scripts/conformance.sh

test:
	go test ./...
