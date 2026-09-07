import { describe, expect, test } from "bun:test";
import { D2 } from "../../dist/node-esm/index.js";

// Inspect requests assembled by the shipped API without starting a WASM worker.
function compileRequest(...args) {
  return D2.prototype.compile.call(
    { sendMessage: async (type, data) => ({ type, data }) },
    ...args
  );
}

describe("compile request contract", () => {
  test.each(["dagre", "elk", "tala"])(
    "accepts flat %s options for string input",
    async (layout) => {
      expect(await compileRequest("x -> y", { layout })).toEqual({
        type: "compile",
        data: { fs: { index: "x -> y" }, options: { layout } },
      });
    }
  );

  test("accepts object input without options and preserves its input path", async () => {
    const input = { fs: { "main.d2": "x -> y" }, inputPath: "main.d2" };
    expect(await compileRequest(input)).toEqual({
      type: "compile",
      data: { ...input, options: {} },
    });
    expect(input).not.toHaveProperty("options");
  });

  test("merges object-input options over second-argument defaults without mutation", async () => {
    const inputOptions = Object.freeze({
      layout: "tala",
      sketch: false,
      pad: 0,
    });
    const defaults = Object.freeze({
      layout: "elk",
      sketch: true,
      pad: 100,
      center: true,
    });
    const input = Object.freeze({
      fs: { index: "x -> y" },
      options: inputOptions,
    });

    const result = await compileRequest(input, defaults);
    expect(result).toEqual({
      type: "compile",
      data: {
        fs: input.fs,
        options: { layout: "tala", sketch: false, pad: 0, center: true },
      },
    });
    expect(result.data.options).not.toBe(inputOptions);
    expect(result.data.options).not.toBe(defaults);
  });
});
