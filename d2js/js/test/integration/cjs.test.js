import { expect, test, describe } from "bun:test";

describe("D2 CJS Integration", () => {
  test("can require and use CJS build", async () => {
    const { D2 } = require("../../dist/node-cjs/index.js");
    const d2 = new D2();
    const result = await d2.compile("x -> y");
    expect(result.diagram).toBeDefined();
    await d2.worker.terminate();
  }, 20000);
});
