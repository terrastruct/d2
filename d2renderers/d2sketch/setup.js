const root = {
  ownerDocument: {
    createElementNS: (ns, tagName) => {
      const children = [];
      const attrs = {};
      const style = {};
      return {
        style,
        tagName,
        attrs,
        setAttribute: (key, value) => (attrs[key] = value),
        appendChild: (node) => children.push(node),
        children,
      };
    },
  },
};
const rc = rough.svg(root, { seed: 1 });
let node;

if (typeof globalThis !== "undefined") {
  globalThis.rc = rc;
}
