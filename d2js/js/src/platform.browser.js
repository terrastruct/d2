import { wasmBinary, wasmExecJs } from "./wasm-loader.browser.js";
import workerScript from "./worker.js" with { type: "text" };

// For the browser version, we build the wasm files into a file (wasm-loader.browser.js)
// and loading a file just reads the text, so there's no external dependency calls
export async function loadFile(path) {
  if (path === "./d2.wasm") {
    return wasmBinary.buffer;
  }
  if (path === "./wasm_exec.js") {
    return new TextEncoder().encode(wasmExecJs).buffer;
  }
  return null;
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
