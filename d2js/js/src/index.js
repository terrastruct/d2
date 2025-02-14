import { createWorker, loadFile } from "./platform.js";

export class D2 {
  constructor() {
    this.ready = this.init();
  }

  setupMessageHandler() {
    const isNode = typeof window === "undefined";
    return new Promise((resolve, reject) => {
      if (isNode) {
        this.worker.on("message", (data) => {
          if (data.type === "ready") resolve();
          if (data.type === "error") reject(new Error(data.error));
          if (data.type === "result" && this.currentResolve) {
            this.currentResolve(data.data);
          }
          if (data.type === "error" && this.currentReject) {
            this.currentReject(new Error(data.error));
          }
        });
      } else {
        this.worker.onmessage = (e) => {
          if (e.data.type === "ready") resolve();
          if (e.data.type === "error") reject(new Error(e.data.error));
          if (e.data.type === "result" && this.currentResolve) {
            this.currentResolve(e.data.data);
          }
          if (e.data.type === "error" && this.currentReject) {
            this.currentReject(new Error(e.data.error));
          }
        };
      }
    });
  }

  async init() {
    this.worker = await createWorker();

    const elkContent = await loadFile("./elk.js");
    const wasmExecContent = await loadFile("./wasm_exec.js");
    const wasmBinary = await loadFile("./d2.wasm");

    const isNode = typeof window === "undefined";
    const messageHandler = this.setupMessageHandler();

    if (isNode) {
      this.worker.on("error", (error) => {
        console.error("Worker (node) encountered an error:", error.message || error);
      });
    } else {
      this.worker.onerror = (error) => {
        console.error("Worker encountered an error:", error.message || error);
      };
    }

    this.worker.postMessage({
      type: "init",
      data: {
        wasm: wasmBinary,
        wasmExecContent: isNode ? wasmExecContent.toString() : null,
        elkContent: isNode ? elkContent.toString() : null,
        wasmExecUrl: isNode
          ? null
          : URL.createObjectURL(
              new Blob([wasmExecContent], { type: "application/javascript" })
            ),
      },
    });

    return messageHandler;
  }

  async sendMessage(type, data) {
    await this.ready;
    return new Promise((resolve, reject) => {
      this.currentResolve = resolve;
      this.currentReject = reject;
      this.worker.postMessage({ type, data });
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
}
