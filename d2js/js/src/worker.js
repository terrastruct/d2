import { parentPort } from "node:worker_threads";
import { readFileSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

let currentPort;
let d2;

function loadScript(content) {
  const func = new Function(content);
  func.call(globalThis);
}

function loadELK() {
  if (typeof globalThis.ELK === "undefined") {
    try {
      const __dirname = dirname(fileURLToPath(import.meta.url));
      const elkJS = readFileSync(join(__dirname, "elk.js"), "utf8");
      const setupJS = readFileSync(join(__dirname, "setup.js"), "utf8");

      loadScript(elkJS);
      loadScript(setupJS);
    } catch (err) {
      console.error("Failed to load ELK library:", err);
      throw err;
    }
  }
}

export function setupMessageHandler(isNode, port, initWasm) {
  currentPort = port;

  const handleMessage = async (e) => {
    const { type, data } = e;

    switch (type) {
      case "init":
        try {
          if (isNode) {
            loadScript(data.wasmExecContent);
          }
          loadELK();
          d2 = await initWasm(data.wasm);
          currentPort.postMessage({ type: "ready" });
        } catch (err) {
          currentPort.postMessage({ type: "error", error: err.message });
        }
        break;

      case "compile":
        try {
          const result = await d2.compile(JSON.stringify(data));
          const response = JSON.parse(result);
          if (response.error) throw new Error(response.error.message);
          currentPort.postMessage({ type: "result", data: response.data });
        } catch (err) {
          currentPort.postMessage({ type: "error", error: err.message });
        }
        break;

      case "render":
        try {
          const result = await d2.render(JSON.stringify(data));
          const response = JSON.parse(result);
          if (response.error) throw new Error(response.error.message);
          const decoded = new TextDecoder().decode(
            Uint8Array.from(atob(response.data), (c) => c.charCodeAt(0))
          );
          currentPort.postMessage({ type: "result", data: decoded });
        } catch (err) {
          currentPort.postMessage({ type: "error", error: err.message });
        }
        break;

      case "encode":
        try {
          const result = d2.encode(data);
          const response = JSON.parse(result);
          if (response.error) throw new Error(response.error.message);
          currentPort.postMessage({ type: "result", data: response.data.result });
        } catch (err) {
          currentPort.postMessage({ type: "error", error: err.message });
        }
        break;

      case "decode":
        try {
          const result = d2.decode(data);
          const response = JSON.parse(result);
          if (response.error) throw new Error(response.error.message);
          currentPort.postMessage({ type: "result", data: response.data.result });
        } catch (err) {
          currentPort.postMessage({ type: "error", error: err.message });
        }
        break;

      case "version":
        try {
          const result = d2.version();
          const response = JSON.parse(result);
          if (response.error) throw new Error(response.error.message);
          currentPort.postMessage({ type: "result", data: response.data });
        } catch (err) {
          currentPort.postMessage({ type: "error", error: err.message });
        }
        break;
    }
  };

  if (isNode) {
    port.on("message", handleMessage);
  } else {
    port.onmessage = (e) => handleMessage(e.data);
  }
}

async function initWasmNode(wasmBinary) {
  const go = new Go();
  const result = await WebAssembly.instantiate(wasmBinary, go.importObject);
  go.run(result.instance);
  return global.d2;
}

setupMessageHandler(true, parentPort, initWasmNode);
