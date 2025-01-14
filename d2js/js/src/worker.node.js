import { parentPort } from "node:worker_threads";
import { setupMessageHandler } from "./worker.shared.js";

async function initWasmNode(wasmBinary) {
  const go = new Go();
  const result = await WebAssembly.instantiate(wasmBinary, go.importObject);
  go.run(result.instance);
  return global.d2;
}

setupMessageHandler(true, parentPort, initWasmNode);
