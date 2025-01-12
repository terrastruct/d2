import { build } from "bun";
import { copyFile, mkdir } from "node:fs/promises";
import { join } from "node:path";

await mkdir("./dist/esm", { recursive: true });
await mkdir("./dist/cjs", { recursive: true });

const commonConfig = {
  target: "node",
  splitting: false,
  sourcemap: "external",
  minify: true,
  naming: {
    entry: "[dir]/[name].js",
    chunk: "[name]-[hash].js",
    asset: "[name]-[hash][ext]",
  },
};

async function buildAndCopy(format) {
  const outdir = `./dist/${format}`;

  await build({
    ...commonConfig,
    entrypoints: ["./src/index.js", "./src/worker.js", "./src/platform.js"],
    outdir,
    format,
  });

  await copyFile("./wasm/d2.wasm", join(outdir, "d2.wasm"));
  await copyFile("./wasm/wasm_exec.js", join(outdir, "wasm_exec.js"));
}

await buildAndCopy("esm");
await buildAndCopy("cjs");
