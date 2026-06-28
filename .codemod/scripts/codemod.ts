import type { Codemod } from "codemod:ast-grep";
import type TS from "codemod:ast-grep/langs/typescript";
import type TSX from "codemod:ast-grep/langs/tsx";

// Chat2API production codemod entrypoint.
// Add AST transforms here, or split into scripts/transforms/ and import them.
// See scripts/examples/var-to-const.ts for a working example with tests.
type Target = TS | TSX;

const codemod: Codemod<Target> = async (root) => {
  const rootNode = root.root();

  // Example: register transforms and collect edits, then commit once.
  // const nodes = rootNode.findAll({ rule: { pattern: "..." } });
  // const edits = nodes.map((node) => node.replace("..."));
  // return rootNode.commitEdits(edits);

  return rootNode.commitEdits([]);
};

export default codemod;