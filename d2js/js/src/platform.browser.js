import { wasmBinary, wasmExecJs } from "./wasm-loader.browser.js";
import workerScript from "./worker.js" with { type: "text" };

export async function loadFile(path) {
  if (path === "./d2.wasm") {
    return wasmBinary.buffer;
  }
  if (path === "./wasm_exec.js") {
    return new TextEncoder().encode(wasmExecJs).buffer;
  }
  throw new Error(`Unexpected file request: ${path}`);
}

export async function createWorker() {
  let blob = new Blob([wasmExecJs, workerScript], {
    type: "text/javascript;charset=utf-8",
  });

  const worker = new Worker(URL.createObjectURL(blob), {
    type: "module",
  });
  return worker;
}
