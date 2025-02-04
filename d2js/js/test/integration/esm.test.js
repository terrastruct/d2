import { expect, test, describe } from "bun:test";
import { D2 } from "../../dist/node-esm/index.js";

describe("D2 ESM Integration", () => {
  test("can import and use ESM build", async () => {
    const d2 = new D2();
    const result = await d2.compile("x -> y");
    expect(result.diagram).toBeDefined();
    await d2.worker.terminate();
  }, 20000);
});
