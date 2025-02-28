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

  test("import works", async () => {
    const d2 = new D2();
    const fs = {
      index: "a: @import",
      "import.d2": "x: {shape: circle}",
    };
    const result = await d2.compile({ fs });
    expect(result.diagram).toBeDefined();
    await d2.worker.terminate();
  }, 20000);

  test("relative import works", async () => {
    const d2 = new D2();
    const fs = {
      "folder/index.d2": "a: @../import",
      "import.d2": "x: {shape: circle}",
    };
    const inputPath = "folder/index.d2";
    const result = await d2.compile({ fs, inputPath });
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

  test("d2-config read correctly", async () => {
    const d2 = new D2();
    const result = await d2.compile(
      `
vars: {
  d2-config: {
    theme-id: 4
    dark-theme-id: 200
    pad: 10
    center: true
    sketch: true
    layout-engine: elk
  }
}
x -> y
`
    );
    expect(result.renderOptions.sketch).toBe(true);
    expect(result.renderOptions.themeID).toBe(4);
    expect(result.renderOptions.darkThemeID).toBe(200);
    expect(result.renderOptions.center).toBe(true);
    expect(result.renderOptions.pad).toBe(10);
    await d2.worker.terminate();
  }, 20000);

  test("render options take priority", async () => {
    const d2 = new D2();
    const result = await d2.compile(
      `
vars: {
  d2-config: {
    theme-id: 4
    dark-theme-id: 200
    pad: 10
    center: true
    sketch: true
    layout-engine: elk
  }
}
x -> y
`,
      {
        sketch: false,
        themeID: 100,
        darkThemeID: 300,
        center: false,
        pad: 0,
        layout: "dagre",
      }
    );
    expect(result.renderOptions.sketch).toBe(false);
    expect(result.renderOptions.themeID).toBe(100);
    expect(result.renderOptions.darkThemeID).toBe(300);
    expect(result.renderOptions.center).toBe(false);
    expect(result.renderOptions.pad).toBe(0);
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

  test("center render works", async () => {
    const d2 = new D2();
    const result = await d2.compile("x -> y", { center: true });
    const svg = await d2.render(result.diagram, { center: true });
    expect(svg).toContain("<svg");
    expect(svg).toContain("</svg>");
    expect(svg).toContain("xMidYMid meet");
    await d2.worker.terminate();
  }, 20000);

  test("no XML tag works", async () => {
    const d2 = new D2();
    const result = await d2.compile("x -> y");
    const svg = await d2.render(result.diagram, { noXMLTag: true });
    expect(svg).not.toContain('<?xml version="1.0"');
    await d2.worker.terminate();
  }, 20000);

  test("force appendix works", async () => {
    const d2 = new D2();
    const result = await d2.compile("x: {tooltip: x appendix}", { forceAppendix: true });
    const svg = await d2.render(result.diagram, { forceAppendix: true });
    expect(svg).toContain("<svg");
    expect(svg).toContain("</svg>");
    expect(svg).toContain('class="appendix"');
    await d2.worker.terminate();
  }, 20000);

  test("animated multi-board works", async () => {
    const d2 = new D2();
    const source = `
x -> y
layers: {
  numbers: {
    1 -> 2
  }
}
`;
    const options = { target: "*", animateInterval: 1000 };
    const result = await d2.compile(source, options);
    const svg = await d2.render(result.diagram, result.renderOptions);
    expect(svg).toContain("<svg");
    expect(svg).toContain("</svg>");
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

  test("unicode characters work", async () => {
    const d2 = new D2();
    const result = await d2.compile("こんにちは -> ♒️");
    const svg = await d2.render(result.diagram);
    expect(svg).toContain("<svg");
    expect(svg).toContain("</svg>");
    expect(svg).toContain("こんにちは");
    expect(svg).toContain("♒️");
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

  test("handles unanimated multi-board error correctly", async () => {
    const d2 = new D2();
    const source = `
x -> y
layers: {
  numbers: {
    1 -> 2
  }
}
`;
    const result = await d2.compile(source);
    try {
      await d2.render(result.diagram, { target: "*" });
      throw new Error("Should have thrown compile error");
    } catch (err) {
      expect(err).toBeDefined();
      expect(err.message).not.toContain("Should have thrown compile error");
    }
    await d2.worker.terminate();
  }, 20000);

  test("handles invalid imports correctly", async () => {
    const d2 = new D2();
    const fs = {
      "folder/index.d2": "a: @../invalid",
      "import.d2": "x: {shape: circle}",
    };
    const inputPath = "folder/index.d2";
    try {
      await d2.compile({ fs, inputPath });
      throw new Error("Should have thrown compile error");
    } catch (err) {
      expect(err).toBeDefined();
      expect(err.message).not.toContain("Should have thrown compile error");
    }
    await d2.worker.terminate();
  }, 20000);
});
