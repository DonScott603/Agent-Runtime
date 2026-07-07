# Devkit integration map

Merge into the repo root as-is; paths are final:

  docs/vectors/           golden vectors (normative; see README inside)
  docs/trace-annotated.md worked run, real chain (corpus scenario 0)
  docs/workplan.md        Stage-1 packages WP-01..WP-17
  docs/glossary.md        naming map (concept -> Go identifier)
  docs/errors.md          error taxonomy (S4-frozen wire codes)
  stubs/kernel/types.go   types-first commit -> move to kernel/ in WP-01
  kernel/*/CLAUDE.md      path-local law (loaded on entry to each dir)
  broker/CLAUDE.md, vault/CLAUDE.md, corpus/CLAUDE.md    same
  .claude/settings.json   PreToolUse frozen-path guard
  .claude/hooks/          guard script (exit 2 blocks; verify hook
                          semantics against code.claude.com/docs/en/hooks)
  .claude/skills/         /conformance, /new-adr, /vector-add
  .claude/agents/         security-reviewer subagent
  scripts/pre-commit      link into .git/hooks/pre-commit

First session after git init: WP-01 (kernel/canon) with /conformance
wired to the canon vectors. Everything else follows docs/workplan.md.
