import assert from "node:assert/strict";
import test from "node:test";
import { D2 } from "@d2lang/d2";

test("installed ESM package compiles and renders", async () => {
  const d2 = new D2();
  try {
    const result = await d2.compile("x -> y");
    assert.ok(result.diagram);
    const svg = await d2.render(result.diagram);
    assert.match(svg, /<svg\b/);
    assert.match(svg, /<\/svg>/);
  } finally {
    await d2.dispose();
  }
});
