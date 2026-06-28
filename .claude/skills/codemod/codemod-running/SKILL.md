---
name: codemod-running
description: "Run Chat2API codemods safely with dry-run, validation, and post-run checks. Examples: \"Run the codemod\", \"Apply this migration\", \"Dry-run codemod on src\""
---

# Running Codemods in Chat2API

## When to Use

- Bulk AST rewrites across `src/` or `tests/`
- Repeatable migrations (API renames, import path updates, pattern replacements)
- Tasks where manual find-and-replace is risky

Prefer **GitNexus `rename`** for symbol renames that must follow the call graph. Use codemods for syntactic or structural patterns.

## Pre-flight (required)

```
1. impact({target: "symbolName", direction: "upstream"})  → GitNexus blast radius
2. npm run codemod:validate                               → workflow schema check
3. npm run codemod:dry-run                                → preview diffs, no writes
4. Review dry-run output; confirm only expected files change
```

> If GitNexus index is stale → `node .gitnexus/run.cjs analyze`

## Apply

```bash
npm run codemod:run
```

Run from the repository root. The workflow targets `src/**/*.{ts,tsx}` and `tests/**/*.{ts,tsx,mjs}`.

## Post-run (required)

```
1. detect_changes()                    → GitNexus scope check
2. npm run codemod:test                → codemod fixture tests still pass
3. Run affected test suites             → e.g. tests/tool-calling/, tests/request-logs/
4. npm run build                       → if renderer/main code changed
```

## Commands

| Command | Purpose |
|---------|---------|
| `npm run codemod:validate` | Validate `.codemod/workflow.yaml` |
| `npm run codemod:test` | Run codemod fixture tests |
| `npm run codemod:dry-run` | Preview changes without writing |
| `npm run codemod:run` | Apply transforms to the repo |

## Registry codemods

Search and run published packages when a local workflow is not needed:

```bash
npx codemod@latest search <query>
npx codemod@latest run <package> --target . --dry-run
```