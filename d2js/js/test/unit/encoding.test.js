import { expect, test, describe } from "bun:test";
import { D2 } from "../../dist/node-esm/index.js";

describe("D2 Encoding Tests", () => {
  test("encode basic script works", async () => {
    const d2 = new D2();
    const script = "x -> y";
    const encoded = await d2.encode(script);
    expect(encoded).toBeDefined();
    expect(typeof encoded).toBe("string");
    expect(encoded.length).toBeGreaterThan(0);
    await d2.worker.terminate();
  }, 20000);

  test("decode encoded script works", async () => {
    const d2 = new D2();
    const script = "x -> y";
    const encoded = await d2.encode(script);
    const decoded = await d2.decode(encoded);
    expect(decoded).toBe(script);
    await d2.worker.terminate();
  }, 20000);

  test("encode and decode complex script works", async () => {
    const d2 = new D2();
    const script = `network: {
  cell tower: {
    satellites: {
      shape: stored_data
      style.multiple: true
    }
    transmitter
    satellites -> transmitter: send
  }
  online portal: {
    ui: {shape: hexagon}
  }
}
user: {shape: person}
user -> network.cell tower: make call`;

    const encoded = await d2.encode(script);
    expect(encoded).toBeDefined();
    expect(typeof encoded).toBe("string");

    const decoded = await d2.decode(encoded);
    expect(decoded).toBe(script);
    await d2.worker.terminate();
  }, 20000);

  test("encode and decode unicode characters works", async () => {
    const d2 = new D2();
    const script = "こんにちは -> ♒️";

    const encoded = await d2.encode(script);
    expect(encoded).toBeDefined();

    const decoded = await d2.decode(encoded);
    expect(decoded).toBe(script);
    await d2.worker.terminate();
  }, 20000);

  test("decode handles invalid input correctly", async () => {
    const d2 = new D2();
    try {
      await d2.decode("invalid-base64-string");
      throw new Error("Should have thrown decode error");
    } catch (err) {
      expect(err).toBeDefined();
      expect(err.message).not.toContain("Should have thrown decode error");
    }
    await d2.worker.terminate();
  }, 20000);

  test("encode empty string works", async () => {
    const d2 = new D2();
    const script = "";

    const encoded = await d2.encode(script);
    expect(encoded).toBeDefined();

    const decoded = await d2.decode(encoded);
    expect(decoded).toBe(script);
    await d2.worker.terminate();
  }, 20000);
});
