import { expect, test, describe } from "bun:test";
import { D2 } from "../../dist/node-esm/index.js";

describe("D2 Unit Tests", () => {
  test("basic compilation works", async () => {
    const d2 = new D2();
    const result = await d2.compile("x -> y");
    expect(result.diagram).toBeDefined();
    await d2.worker.terminate();
  }, 20000);

  test("elk layout works", async () => {
    const d2 = new D2();
    const result = await d2.compile("x -> y", { layout: "elk" });
    expect(result.diagram).toBeDefined();
    await d2.worker.terminate();
  }, 20000);

  test("render works", async () => {
    const d2 = new D2();
    const result = await d2.compile("x -> y");
    const svg = await d2.render(result.diagram);
    expect(svg).toContain("<svg");
    expect(svg).toContain("</svg>");
    await d2.worker.terminate();
  }, 20000);

  test("multiple renders works", async () => {
    const d2 = new D2();
    const result = await d2.compile("x -> y");
    const svg = await d2.render(result.diagram);
    expect(svg).toContain("<svg");
    expect(svg).toContain("</svg>");
    const result2 = await d2.compile("x -> y");
    const svg2 = await d2.render(result2.diagram);
    expect(svg).toEqual(svg2);
    await d2.worker.terminate();
  }, 20000);

  test("sketch render works", async () => {
    const d2 = new D2();
    const result = await d2.compile("x -> y", { sketch: true });
    const svg = await d2.render(result.diagram, { sketch: true });
    expect(svg).toContain("<svg");
    expect(svg).toContain("</svg>");
    expect(svg).toContain("sketch-overlay");
    await d2.worker.terminate();
  }, 20000);

  test("latex works", async () => {
    const d2 = new D2();
    const result = await d2.compile("x: |latex \\frac{f(x+h)-f(x)}{h} |");
    const svg = await d2.render(result.diagram);
    expect(svg).toContain("<svg");
    expect(svg).toContain("</svg>");
    await d2.worker.terminate();
  }, 20000);

  test("handles syntax errors correctly", async () => {
    const d2 = new D2();
    try {
      await d2.compile("invalid -> -> syntax");
      throw new Error("Should have thrown syntax error");
    } catch (err) {
      expect(err).toBeDefined();
      expect(err.message).not.toContain("Should have thrown syntax error");
    }
    await d2.worker.terminate();
  }, 20000);
});
