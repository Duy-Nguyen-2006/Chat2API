---
name: codemod-authoring
description: "Author or extend Chat2API codemods with jssg and ast-grep. Examples: \"Write a codemod for X\", \"Add a transform\", \"Create a migration script\""
---

# Authoring Chat2API Codemods

## Layout

```
.codemod/
├── workflow.yaml           # Scan scope, language, entry script
├── codemod.yaml            # Package manifest
├── scripts/
│   ├── codemod.ts          # Production entry (no-op until you add transforms)
│   └── examples/           # Reference transforms with tests
└── tests/fixtures/         # Input/expected pairs for jssg test
```

## Workflow

```
1. Edit scripts/codemod.ts (or import from scripts/transforms/)
2. Match workflow.yaml include/exclude to intended file scope
3. npm run codemod:test     → fixture tests pass
4. npm run codemod:validate → workflow valid
5. npm run codemod:dry-run  → review diffs on real sources
```

## Writing transforms

Use ast-grep patterns in `scripts/codemod.ts`:

```typescript
import type { Codemod } from "codemod:ast-grep";
import type TS from "codemod:ast-grep/langs/typescript";
import type TSX from "codemod:ast-grep/langs/tsx";

const codemod: Codemod<TS | TSX> = async (root) => {
  const rootNode = root.root();
  const nodes = rootNode.findAll({
    rule: { pattern: "OLD_PATTERN" },
  });
  const edits = nodes.map((node) => node.replace("NEW_PATTERN"));
  return rootNode.commitEdits(edits);
};

export default codemod;
```

Language notes for this repo:

- Use **TSX** for `src/renderer/**/*.tsx` (React + JSX)
- Use **TS** for `src/main`, `src/preload`, `src/shared`, and plain `.ts` files
- The workflow currently sets `language: typescript`; split workflows if you need separate TS/TSX passes

## Testing

1. Add `tests/fixtures/input.*` and `tests/fixtures/expected.*`
2. Point `package.json` test script at your transform file
3. Run `npm run codemod:test`

See `scripts/examples/var-to-const.ts` for a minimal working example.

## Publishing (optional)

```bash
cd .codemod
npx codemod@latest login
npx codemod@latest publish
```