import { setupMessageHandler } from "./worker.shared.js";

async function initWasmBrowser(wasmBinary) {
  const go = new Go();
  const result = await WebAssembly.instantiate(wasmBinary, go.importObject);
  go.run(result.instance);
  return self.d2;
}

setupMessageHandler(self, initWasmBrowser);
