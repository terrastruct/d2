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

  test("theme overrides work correctly", async () => {
    const d2 = new D2();

    // B1 controls arrow/border color
    const resultOverridden = await d2.compile(`
  vars: {
    d2-config: {
      theme-overrides: {
        B1: "#000000"
      }
    }
  }

  a -> b
  `);

    expect(resultOverridden.renderOptions.themeOverrides).toBeDefined();
    expect(resultOverridden.renderOptions.themeOverrides.b1).toBe("#000000");

    const svgOverridden = await d2.render(
      resultOverridden.diagram,
      resultOverridden.renderOptions
    );

    expect(svgOverridden).toContain("fill:#000000");
    await d2.worker.terminate();
  }, 20000);

  test("grid layout with elk engine matches go rendering", async () => {
    const d2 = new D2();
    const source = `vars: {
    d2-config: {
      layout-engine: elk
    }
  }

  group: "" {
    grid-rows: 1
    grid-gap: 0
    1
    2
    3
  }`;
    const result = await d2.compile(source);
    expect(result.diagram).toBeDefined();

    const svg = await d2.render(result.diagram);
    expect(svg).toContain("<svg");
    expect(svg).toContain("</svg>");

    const groupMatch = svg.match(
      /<g class="Z3JvdXA="[^>]*>[\s\S]*?<rect[^>]*width="([^"]*)"[^>]*height="([^"]*)"[^>]*>/
    );
    expect(groupMatch).not.toBeNull();
    const groupWidth = parseFloat(groupMatch[1]);
    const groupHeight = parseFloat(groupMatch[2]);

    const element1Match = svg.match(
      /<g class="Z3JvdXAuMQ=="[^>]*>[\s\S]*?<rect[^>]*width="([^"]*)"[^>]*height="([^"]*)"[^>]*>/
    );
    expect(element1Match).not.toBeNull();
    const element1Width = parseFloat(element1Match[1]);
    const element1Height = parseFloat(element1Match[2]);

    expect(groupHeight).toBe(element1Height);

    // Verify the grid elements are rendered correctly with elk layout
    expect(svg).toContain("Z3JvdXA="); // "group" base64 encoded
    expect(svg).toContain(">1</text>"); // Should contain the "1" element
    expect(svg).toContain(">2</text>"); // Should contain the "2" element
    expect(svg).toContain(">3</text>"); // Should contain the "3" element
    await d2.worker.terminate();
  }, 20000);

  test("layout engine switching works (dagre -> elk)", async () => {
    const d2 = new D2();

    // Test diagram
    const testDiagram = `a -> b
  a -> c
  b -> d
  c -> d`;

    // First compile with dagre using d2-config
    const dagreSource = `${testDiagram}

  vars: {
    d2-config: {
      layout-engine: dagre
    }
  }`;

    const dagreResult = await d2.compile(dagreSource);
    expect(dagreResult.diagram).toBeDefined();

    // Then compile with elk using d2-config
    const elkSource = `${testDiagram}

  vars: {
    d2-config: {
      layout-engine: elk
    }
  }`;

    const elkResult = await d2.compile(elkSource);
    expect(elkResult.diagram).toBeDefined();

    // Both should render successfully
    const dagreSvg = await d2.render(dagreResult.diagram);
    expect(dagreSvg).toContain("<svg");
    expect(dagreSvg).toContain("</svg>");

    const elkSvg = await d2.render(elkResult.diagram);
    expect(elkSvg).toContain("<svg");
    expect(elkSvg).toContain("</svg>");

    await d2.worker.terminate();
  }, 30000);

  test("layout engine switching works (elk -> dagre -> elk)", async () => {
    const d2 = new D2();

    // Test diagram
    const testDiagram = `a -> b
  a -> c
  b -> d
  c -> d`;

    // Start with ELK
    const elkSource1 = `${testDiagram}

  vars: {
    d2-config: {
      layout-engine: elk
    }
  }`;

    const elkResult1 = await d2.compile(elkSource1);
    expect(elkResult1.diagram).toBeDefined();

    // Switch to Dagre
    const dagreSource = `${testDiagram}

  vars: {
    d2-config: {
      layout-engine: dagre
    }
  }`;

    const dagreResult = await d2.compile(dagreSource);
    expect(dagreResult.diagram).toBeDefined();

    // Switch back to ELK (this should trigger the panic without the fix)
    const elkSource2 = `${testDiagram}

  vars: {
    d2-config: {
      layout-engine: elk
    }
  }`;

    const elkResult2 = await d2.compile(elkSource2);
    expect(elkResult2.diagram).toBeDefined();

    // All should render successfully
    const elkSvg1 = await d2.render(elkResult1.diagram);
    expect(elkSvg1).toContain("<svg");
    expect(elkSvg1).toContain("</svg>");

    const dagreSvg = await d2.render(dagreResult.diagram);
    expect(dagreSvg).toContain("<svg");
    expect(dagreSvg).toContain("</svg>");

    const elkSvg2 = await d2.render(elkResult2.diagram);
    expect(elkSvg2).toContain("<svg");
    expect(elkSvg2).toContain("</svg>");

    await d2.worker.terminate();
  }, 30000);

  test("version returns a string", async () => {
    const d2 = new D2();
    const version = await d2.version();
    expect(version).toBeDefined();
    expect(typeof version).toBe("string");
    expect(version).toContain("v");
    await d2.worker.terminate();
  }, 20000);

  test("jsVersion returns d2js version string", async () => {
    const d2 = new D2();
    const jsVersion = await d2.jsVersion();
    expect(jsVersion).toBeDefined();
    expect(typeof jsVersion).toBe("string");
    expect(jsVersion.length).toBeGreaterThan(0);
    await d2.worker.terminate();
  }, 20000);

  test("ASCII render works", async () => {
    const d2 = new D2();
    const result = await d2.compile("x -> y");
    const ascii = await d2.render(result.diagram, { ascii: true });
    expect(ascii).toBeDefined();
    expect(typeof ascii).toBe("string");
    expect(ascii).toContain("x");
    expect(ascii).toContain("y");
    // ASCII art uses box drawing characters for connections
    expect(ascii).toContain("┌") ||
      expect(ascii).toContain("└") ||
      expect(ascii).toContain("│");
    await d2.worker.terminate();
  }, 20000);

  test("ASCII render with multiple shapes works", async () => {
    const d2 = new D2();
    const result = await d2.compile(`
      a: {shape: rectangle}
      b: {shape: circle} 
      c: {shape: diamond}
      a -> b
      b -> c
    `);
    const ascii = await d2.render(result.diagram, { ascii: true });
    expect(ascii).toBeDefined();
    expect(typeof ascii).toBe("string");
    expect(ascii).toContain("a");
    expect(ascii).toContain("b");
    expect(ascii).toContain("c");
    await d2.worker.terminate();
  }, 20000);

  test("ASCII mode options work correctly", async () => {
    const d2 = new D2();
    const result = await d2.compile("x -> y");

    // Test extended mode (default)
    const asciiExtended = await d2.render(result.diagram, {
      ascii: true,
      asciiMode: "extended",
    });
    expect(asciiExtended).toBeDefined();
    expect(typeof asciiExtended).toBe("string");
    expect(asciiExtended).toMatch(/[┌┐└┘│─]/); // Should contain Unicode box chars

    // Test standard mode
    const asciiStandard = await d2.render(result.diagram, {
      ascii: true,
      asciiMode: "standard",
    });
    expect(asciiStandard).toBeDefined();
    expect(typeof asciiStandard).toBe("string");
    expect(asciiStandard).not.toMatch(/[┌┐└┘│─]/); // Should not contain Unicode box chars
    expect(asciiStandard).toMatch(/[+\-|]/); // Should contain basic ASCII chars

    // Modes should produce different outputs
    expect(asciiExtended).not.toBe(asciiStandard);

    await d2.worker.terminate();
  }, 20000);
});
