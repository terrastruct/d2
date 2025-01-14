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

  // Create global Go without IIFE in module context
  let blob = new Blob(
    [
      // First establish Go in global scope
      wasmExecJs,
      // Then the module code
      workerScript,
    ],
    {
      type: "text/javascript;charset=utf-8",
    }
  );

  console.log("about to create worker");
  const worker = new Worker(URL.createObjectURL(blob), {
    type: "module",
  });
  console.log("worker", worker);

  // Add error handler to see initialization errors
  worker.onerror = (error) => {
    console.error("Worker initialization error:", {
      message: error.message,
      filename: error.filename,
      lineno: error.lineno,
      error: error.error,
    });
  };

  return worker;
}
