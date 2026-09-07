import { describe, expect, test } from "bun:test";
import { D2 } from "../../dist/node-esm/index.js";
import packageJson from "../../package.json" assert { type: "json" };

class FakeNodeWorker {
  constructor() {
    this.handlers = new Map();
    this.messages = [];
    this.terminateCalls = 0;
  }

  on(type, handler) {
    this.handlers.set(type, handler);
  }

  postMessage(message) {
    this.messages.push(message);
  }

  emitMessage(message) {
    this.handlers.get("message")(message);
  }

  terminate() {
    this.terminateCalls++;
    return Promise.resolve(0);
  }
}

class FakeBrowserWorker {
  constructor() {
    this.messages = [];
    this.terminateCalls = 0;
  }

  postMessage(message) {
    this.messages.push(message);
  }

  emitMessage(message) {
    this.onmessage({ data: message });
  }

  terminate() {
    this.terminateCalls++;
  }
}

function createD2(isNode) {
  const worker = isNode ? new FakeNodeWorker() : new FakeBrowserWorker();
  const d2 = new (class extends D2 {
    init() {
      this.worker = worker;
      const ready = this.setupMessageHandler(isNode);
      worker.emitMessage({ type: "ready" });
      return ready;
    }
  })();
  return { d2, worker };
}

async function waitForRequests(worker, count) {
  while (worker.messages.length < count) {
    await Promise.resolve();
  }
}

for (const [name, isNode] of [
  ["Node", true],
  ["browser", false],
]) {
  describe(`${name} worker request correlation`, () => {
    test("resolves overlapping requests that complete out of order", async () => {
      const { d2, worker } = createD2(isNode);

      const first = d2.compile("first");
      const second = d2.version();
      await waitForRequests(worker, 2);

      const [firstRequest, secondRequest] = worker.messages;
      expect(firstRequest.id).not.toBe(secondRequest.id);
      expect(firstRequest.type).toBe("compile");
      expect(secondRequest.type).toBe("version");

      worker.emitMessage({
        id: secondRequest.id,
        type: "result",
        data: "second result",
      });
      worker.emitMessage({
        id: firstRequest.id,
        type: "result",
        data: "first result",
      });

      expect(await first).toBe("first result");
      expect(await second).toBe("second result");
      expect(d2.pendingRequests.size).toBe(0);
    });

    test("rejects only the request whose out-of-order response is an error", async () => {
      const { d2, worker } = createD2(isNode);

      const first = d2.render({ name: "first" });
      const second = d2.decode("invalid");
      const secondResult = second.then(
        () => ({ resolved: true }),
        (error) => ({ error })
      );
      await waitForRequests(worker, 2);

      const [firstRequest, secondRequest] = worker.messages;
      worker.emitMessage({
        id: secondRequest.id,
        type: "error",
        error: "second failed",
      });
      worker.emitMessage({
        id: firstRequest.id,
        type: "result",
        data: "first result",
      });

      expect(await first).toBe("first result");
      expect((await secondResult).error.message).toBe("second failed");
      expect(d2.pendingRequests.size).toBe(0);
    });

    test("cleans up a request when posting it fails", async () => {
      const { d2, worker } = createD2(isNode);
      worker.postMessage = () => {
        throw new Error("post failed");
      };

      await expect(d2.version()).rejects.toThrow("post failed");
      expect(d2.pendingRequests.size).toBe(0);
    });

    test("disposes the worker exactly once", async () => {
      const { d2, worker } = createD2(isNode);

      await Promise.all([d2.dispose(), d2.dispose()]);

      expect(worker.terminateCalls).toBe(1);
      expect(d2.worker).toBeUndefined();
      await expect(d2.version()).rejects.toThrow("D2 instance has been disposed");
    });

    test("rejects pending requests when disposed", async () => {
      const { d2, worker } = createD2(isNode);
      const result = d2.compile("pending").then(
        () => ({ resolved: true }),
        (error) => ({ error })
      );
      await waitForRequests(worker, 1);

      await d2.dispose();

      expect((await result).error.message).toBe("D2 instance has been disposed");
      expect(d2.pendingRequests.size).toBe(0);
      expect(worker.terminateCalls).toBe(1);
    });
  });
}

test("correlates every operation through the real worker", async () => {
  const d2 = new D2();
  const base = await d2.compile("base");
  const encodedBase = await d2.encode("decoded result");

  const [compiled, rendered, encoded, decoded, version, jsVersion] = await Promise.all([
    d2.compile("compiled-result"),
    d2.render(base.diagram),
    d2.encode("encoded result"),
    d2.decode(encodedBase),
    d2.version(),
    d2.jsVersion(),
  ]);

  expect(compiled.diagram.shapes[0].id).toBe("compiled-result");
  expect(rendered).toContain("base");
  expect(await d2.decode(encoded)).toBe("encoded result");
  expect(decoded).toBe("decoded result");
  expect(version).toMatch(/^(v\d+\.\d+\.\d+(?:-[\w.-]+)?|[0-9a-f]{7,40})$/);
  expect(jsVersion).toBe(packageJson.version);
  await d2.dispose();
}, 20000);
