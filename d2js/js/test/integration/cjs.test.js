import { describe, expect, test } from "bun:test";

describe("D2 CJS Integration", () => {
    test("can require main entry point without error", () => {
    expect(() => {
      const module = require("../../dist/node-cjs/index.js");
      expect(module).toBeDefined();
      expect(module.D2).toBeDefined();
      expect(typeof module.D2).toBe("function");
    }).not.toThrow();
  });

  test("worker module file exists", () => {
    const fs = require("fs");
    const path = require("path");
    const workerPath = path.resolve(__dirname, "../../dist/node-cjs/worker.js");
    expect(fs.existsSync(workerPath)).toBe(true);
  });

  test("exported D2 class is constructable", () => {
    const { D2 } = require("../../dist/node-cjs/index.js");
    expect(() => new D2()).not.toThrow();
  });

  test("module exports match expected structure", () => {
    const module = require("../../dist/node-cjs/index.js");
    expect(module).toHaveProperty("D2");
    expect(typeof module.D2).toBe("function");
  });

  test("can access both named and default exports", () => {
    const module = require("../../dist/node-cjs/index.js");
    const { D2 } = require("../../dist/node-cjs/index.js");
    
    expect(module.D2).toBe(D2);
    expect(module.D2).toBeDefined();
  });
  
  test("can compile a diagram", async () => {
    const { D2 } = require("../../dist/node-cjs/index.js");
    const d2 = new D2();
    const result = await d2.compile("x -> y");
    expect(result.diagram).toBeDefined();
    await d2.worker.terminate();
  }, 20000);
});
