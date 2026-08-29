import { createWorker, loadFile } from "./platform.js";

export class D2 {
  constructor() {
    this.nextRequestId = 0;
    this.pendingRequests = new Map();
    this.disposed = false;
    this.disposePromise = null;
    this.ready = this.init();
  }

  rejectPendingRequests(error) {
    for (const request of this.pendingRequests.values()) {
      request.reject(error);
    }
    this.pendingRequests.clear();
  }

  setupMessageHandler(isNode = typeof window === "undefined") {
    return new Promise((resolve, reject) => {
      let isReady = false;

      const handleWorkerError = (error) => {
        const workerError =
          error instanceof Error
            ? error
            : new Error(error?.message || "D2 worker encountered an error");
        if (!isReady) reject(workerError);
        this.rejectPendingRequests(workerError);
        console.error(
          `Worker${isNode ? " (node)" : ""} encountered an error:`,
          workerError.message
        );
      };

      const handleMessage = (data) => {
        if (data.type === "ready") {
          isReady = true;
          resolve();
          return;
        }

        if (data.type !== "result" && data.type !== "error") return;

        if (data.id === undefined) {
          if (data.type === "error") {
            const error = new Error(data.error);
            if (!isReady) reject(error);
            this.rejectPendingRequests(error);
          }
          return;
        }

        const request = this.pendingRequests.get(data.id);
        if (!request) return;

        this.pendingRequests.delete(data.id);
        if (data.type === "result") {
          request.resolve(data.data);
        } else {
          request.reject(new Error(data.error));
        }
      };

      if (isNode) {
        this.worker.on("message", handleMessage);
        this.worker.on("error", handleWorkerError);
      } else {
        this.worker.onmessage = (e) => handleMessage(e.data);
        this.worker.onerror = handleWorkerError;
      }
    });
  }

  async init() {
    this.worker = await createWorker();

    const isNode = typeof window === "undefined";
    const wasmExecContent = isNode ? await loadFile("./wasm_exec.js") : null;
    const wasmBinary = await loadFile("./d2.wasm");

    const messageHandler = this.setupMessageHandler();

    this.worker.postMessage({
      type: "init",
      data: {
        wasm: wasmBinary,
        wasmExecContent: isNode ? wasmExecContent.toString() : null,
      },
    });

    return messageHandler;
  }

  async sendMessage(type, data) {
    if (this.disposed) {
      throw new Error("D2 instance has been disposed");
    }
    await this.ready;
    if (this.disposed) {
      throw new Error("D2 instance has been disposed");
    }
    return new Promise((resolve, reject) => {
      const id = this.nextRequestId++;
      this.pendingRequests.set(id, { resolve, reject });
      try {
        this.worker.postMessage({ id, type, data });
      } catch (error) {
        this.pendingRequests.delete(id);
        reject(error);
      }
    });
  }

  async compile(input, options = {}) {
    const request =
      typeof input === "string"
        ? { fs: { index: input }, options }
        : { ...input, options: { ...options, ...input.options } };
    return this.sendMessage("compile", request);
  }

  async render(diagram, options = {}) {
    return this.sendMessage("render", { diagram, options });
  }

  async encode(script) {
    return this.sendMessage("encode", script);
  }

  async decode(encoded) {
    return this.sendMessage("decode", encoded);
  }

  async version() {
    return this.sendMessage("version");
  }

  async jsVersion() {
    return this.sendMessage("jsVersion");
  }

  async dispose() {
    if (this.disposePromise) return this.disposePromise;

    this.disposed = true;
    this.rejectPendingRequests(new Error("D2 instance has been disposed"));
    this.disposePromise = (async () => {
      try {
        await this.ready;
      } catch {
        // Initialization errors are already exposed through ready. The worker,
        // if one was created, still needs to be terminated.
      }
      const worker = this.worker;
      this.worker = undefined;
      if (worker) await worker.terminate();
    })();
    return this.disposePromise;
  }
}
