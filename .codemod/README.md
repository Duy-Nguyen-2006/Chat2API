# chat2api-codemods

Local Codemod workflow for the Chat2API repository.

## Quick start

From the repository root:

```bash
npm run codemod:validate
npm run codemod:test
npm run codemod:dry-run   # preview — no writes
npm run codemod:run       # apply transforms
```

## Scope

The workflow scans:

- `src/**/*.{ts,tsx}`
- `tests/**/*.{ts,tsx,mjs}`

Excludes: `node_modules`, `out`, `dist`, `build`, `.codemod`, `.gitnexus`.

## Development

Edit `scripts/codemod.ts` to add transforms. See `scripts/examples/var-to-const.ts` for a tested example.

```bash
npm test                  # fixture tests (from .codemod/)
npm run validate          # validate workflow.yaml
npm run check-types       # TypeScript check
```

## Publishing (optional)

```bash
npx codemod@latest login
npx codemod@latest publish
```

## License

GPL-3.0