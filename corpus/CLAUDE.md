# corpus — local law

- SYNTHETIC ONLY (threat-model D16). Nothing derived from a real run
  enters this directory, ever, including "just this one anonymized
  example". The pre-commit secret scan enforces; do not bypass it.
- The annotated trace (docs/trace-annotated.md) is scenario 0; the
  generator must reproduce its chain head exactly (WP-10).
- Golden state hashes here are ratchet material: regenerating them
  requires human sign-off in the PR description (/vector-add rules).
