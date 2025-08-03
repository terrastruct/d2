import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";

const __dirname = dirname(fileURLToPath(import.meta.url));
export async function getWasmBinary() {
  return readFile(resolve(__dirname, "./d2.wasm"));
}
