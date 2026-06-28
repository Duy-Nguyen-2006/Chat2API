---
name: codemod-guide
description: "Codemod CLI and Chat2API workflow reference. Examples: \"What codemod tools are available?\", \"How do I use codemod here?\", \"codemod vs GitNexus rename\""
---

# Codemod Guide — Chat2API

## Tool choice

| Task | Tool |
|------|------|
| Rename function/class across call graph | GitNexus `rename` |
| Blast radius before any edit | GitNexus `impact` |
| Bulk pattern rewrite (imports, APIs, syntax) | Codemod `jssg` / local workflow |
| Published community migrations | `npx codemod run <package>` |

## CLI reference

```bash
# Local workflow (this repo)
npm run codemod:validate
npm run codemod:test
npm run codemod:dry-run
npm run codemod:run

# Low-level jssg
npx codemod@latest jssg run -l typescript .codemod/scripts/codemod.ts --target src/
npx codemod@latest jssg test -l javascript .codemod/scripts/examples/var-to-const.ts

# Registry
npx codemod@latest search <query>
npx codemod@latest run <package> --target . --dry-run
```

## Workflow file

`.codemod/workflow.yaml` defines:

- `js_file` — transform entrypoint
- `include` / `exclude` — file globs relative to `--target`
- `language` — ast-grep parser (`typescript`, `tsx`, `javascript`)

## Agent skills

| Task | Skill |
|------|-------|
| Run migrations safely | `.claude/skills/codemod/codemod-running/SKILL.md` |
| Write new transforms | `.claude/skills/codemod/codemod-authoring/SKILL.md` |
| CLI and tool selection | `.claude/skills/codemod/codemod-guide/SKILL.md` |

## Docs

- https://go.codemod.com/docs
- https://codemod.com/blog/jssg