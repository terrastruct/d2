import { wasmBinary, wasmExecJs } from "./wasm-loader.browser.js";

export async function loadFile(path) {
  console.log("loading " + path);
  if (path === "./d2.wasm") {
    return wasmBinary.buffer;
  }
  if (path === "./wasm_exec.js") {
    return new TextEncoder().encode(wasmExecJs).buffer;
  }
  throw new Error(`Unexpected file request: ${path}`);
}

export async function createWorker() {
  // Combine wasmExecJs with worker script
  const workerResponse = await fetch(new URL("./worker.js", import.meta.url));
  if (!workerResponse.ok) {
    throw new Error(
      `Failed to load worker.js: ${workerResponse.status} ${workerResponse.statusText}`
    );
  }
  const workerJs = await workerResponse.text();

  const blob = new Blob(["(() => {", wasmExecJs, "})();", workerJs], {
    type: "application/javascript",
  });

  return new Worker(URL.createObjectURL(blob));
}
