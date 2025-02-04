import { readFile } from "fs/promises";
import { fileURLToPath } from "url";
import { dirname, resolve } from "path";

const __dirname = dirname(fileURLToPath(import.meta.url));
export async function getWasmBinary() {
  return readFile(resolve(__dirname, "./d2.wasm"));
}
