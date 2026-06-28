import type { Codemod } from "codemod:ast-grep";
import type JS from "codemod:ast-grep/langs/javascript";

const codemod: Codemod<JS> = async (root) => {
  const rootNode = root.root();

  const nodes = rootNode.findAll({
    rule: {
      pattern: "var $VAR = $VALUE;",
    },
  });

  const edits = nodes.map((node) => {
    const varName = node.getMatch("VAR")?.text();
    const value = node.getMatch("VALUE")?.text();
    return node.replace(`const ${varName} = ${value};`);
  });

  return rootNode.commitEdits(edits);
};

export default codemod;