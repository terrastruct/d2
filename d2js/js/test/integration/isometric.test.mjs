import assert from "node:assert/strict";
import { createRequire } from "node:module";
import { describe, test } from "node:test";
import { D2 as ESMD2 } from "@d2lang/d2";

const require = createRequire(import.meta.url);
const { D2: CJSD2 } = require("@d2lang/d2");

for (const [format, D2] of [
  ["ESM", ESMD2],
  ["CJS", CJSD2],
]) {
  describe(`installed ${format} package isometric integration`, () => {
    test("source configuration survives the worker roundtrip", async () => {
      const d2 = new D2();
      try {
        const source =
          "vars: {d2-config: {isometric: true}}\nsystem: {api -> database}";
        const result = await d2.compile(source);
        assert.equal(result.renderOptions.isometric, true);
        const original = JSON.stringify(result.diagram);
        const svg = await d2.render(result.diagram, result.renderOptions);
        assert.match(svg, /d2-isometric/);
        assert.match(svg, /<path\b/);
        assert.equal(JSON.stringify(result.diagram), original);

        const flat = await d2.render(result.diagram, {
          ...result.renderOptions,
          isometric: false,
        });
        assert.match(flat, /<svg\b/);
        assert.doesNotMatch(flat, /d2-isometric/);
      } finally {
        await d2.dispose();
      }
    });

    test("explicit compile options override the source mode", async () => {
      const d2 = new D2();
      try {
        const enabled = await d2.compile("a -> b", { isometric: true });
        assert.equal(enabled.renderOptions.isometric, true);
        assert.match(
          await d2.render(enabled.diagram, enabled.renderOptions),
          /d2-isometric/
        );
        const disabled = await d2.compile(
          "vars: {d2-config: {isometric: true}}\na -> b",
          { isometric: false }
        );
        assert.equal(disabled.renderOptions.isometric, false);
        assert.doesNotMatch(
          await d2.render(disabled.diagram, disabled.renderOptions),
          /d2-isometric/
        );
      } finally {
        await d2.dispose();
      }
    });
  });
}
