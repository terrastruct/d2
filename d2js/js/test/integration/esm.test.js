import { describe, expect, test } from "bun:test";

describe("D2 ESM Integration", () => {
    test("can import main entry point without error", async () => {
    const module = await import("../../dist/node-esm/index.js");
    expect(module).toBeDefined();
    expect(module.D2).toBeDefined();
    expect(typeof module.D2).toBe("function");
  });

  test("worker module file exists", () => {
    const fs = require("fs");
    const path = require("path");
    const workerPath = path.resolve(__dirname, "../../dist/node-esm/worker.js");
    expect(fs.existsSync(workerPath)).toBe(true);
  });

  test("exported D2 class is constructable", () => {
    return import("../../dist/node-esm/index.js").then(({ D2 }) => {
      expect(() => new D2()).not.toThrow();
    });
  });

  test("can import all exports from package.json exports field", async () => {
    const mainModule = await import("../../index.d.ts");
    expect(mainModule).toBeDefined();
  });
  
  test("can import and use ESM build", async () => {
    const { D2 } = await import("../../dist/node-esm/index.js");
    const d2 = new D2();
    const result = await d2.compile("x -> y");
    expect(result.diagram).toBeDefined();
    await d2.worker.terminate();
  }, 20000);
});
