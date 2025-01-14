import { wasmBinary, wasmExecJs } from "./wasm-loader.browser.js";

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
  let response = await fetch(new URL("./worker.js", import.meta.url));
  if (!response.ok)
    throw new Error(
      `Failed to load worker.js: ${response.status} ${response.statusText}`
    );
  let workerScript = await response.text();

  let blob = new Blob([wasmExecJs, workerScript], {
    type: "text/javascript;charset=utf-8",
  });

  const worker = new Worker(URL.createObjectURL(blob), {
    type: "module",
  });
  return worker;
}
