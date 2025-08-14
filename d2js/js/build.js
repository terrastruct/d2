import { build } from "bun";
import { copyFile, mkdir, writeFile, readFile, rm } from "node:fs/promises";
import { join, resolve } from "node:path";
import { brotliCompressSync } from "node:zlib";

const __dirname = new URL(".", import.meta.url).pathname;
const ROOT_DIR = resolve(__dirname);
const SRC_DIR = resolve(ROOT_DIR, "src");

await rm("./dist", { recursive: true, force: true });
await mkdir("./dist/browser", { recursive: true });
await mkdir("./dist/node-esm", { recursive: true });
await mkdir("./dist/node-cjs", { recursive: true });

const wasmBinary = await readFile("./wasm/d2.wasm");
const wasmExecJs = await readFile("./wasm/wasm_exec.js", "utf8");

const compressedWasm = brotliCompressSync(wasmBinary);
console.log(
  `WASM compression: ${(wasmBinary.length / 1024 / 1024).toFixed(2)}MB → ${(
    compressedWasm.length /
    1024 /
    1024
  ).toFixed(2)}MB`
);

// Store compressed WASM as base64 and include brotli decoder in the loader
// Don't decompress immediately - let the consumer decompress when needed
const brotliDecoder = await readFile("./vendor/decode.min.js", "utf8");

await writeFile(
  join(SRC_DIR, "wasm-loader.browser.js"),
  `${brotliDecoder}

export const wasmBinaryCompressed = "${Buffer.from(compressedWasm).toString("base64")}";
export function getWasmBinary() {
  const compressedBytes = Uint8Array.from(atob(wasmBinaryCompressed), c => c.charCodeAt(0));
  return BrotliDecode(compressedBytes);
}
export const wasmExecJs = ${JSON.stringify(wasmExecJs)};`
);

const commonConfig = {
  minify: true,
};

async function buildDynamicFiles(platform) {
  const platformContent =
    platform === "node"
      ? `export * from "./platform.node.js";`
      : `export * from "./platform.browser.js";`;

  await writeFile(join(SRC_DIR, "platform.js"), platformContent);

  if (platform === "node") {
    const workerContent = await readFile(join(SRC_DIR, "worker.node.js"), "utf8");
    await writeFile(join(SRC_DIR, "worker.js"), workerContent);
  } else {
    // For browser, prepend the ELK variables to worker.browser.js
    // since the worker runs in a blob and can't use ES6 imports
    const elkJs = await readFile(
      resolve(ROOT_DIR, "../../d2layouts/d2elklayout/elk.js"),
      "utf8"
    );
    const setupJs = await readFile(
      resolve(ROOT_DIR, "../../d2layouts/d2elklayout/setup.js"),
      "utf8"
    );

    // Compress elk.js and setup.js
    const elkJsCompressed = brotliCompressSync(new TextEncoder().encode(elkJs));
    const setupJsCompressed = brotliCompressSync(new TextEncoder().encode(setupJs));

    console.log(
      `ELK compression: ${(elkJs.length / 1024 / 1024).toFixed(2)}MB → ${(
        elkJsCompressed.length /
        1024 /
        1024
      ).toFixed(2)}MB`
    );

    const workerBase = await readFile(join(SRC_DIR, "worker.browser.js"), "utf8");

    // Bundle brotli decoder directly into the worker
    const brotliDecoder = await readFile(resolve(ROOT_DIR, "vendor/decode.min.js"), "utf8");

    const elkVars = `${brotliDecoder}
const elkJsCompressed = "${Buffer.from(elkJsCompressed).toString("base64")}";
const setupJsCompressed = "${Buffer.from(setupJsCompressed).toString("base64")}";
const elkJs = new TextDecoder().decode(BrotliDecode(Uint8Array.from(atob(elkJsCompressed), c => c.charCodeAt(0))));
const setupJs = new TextDecoder().decode(BrotliDecode(Uint8Array.from(atob(setupJsCompressed), c => c.charCodeAt(0))));
`;
    await writeFile(join(SRC_DIR, "worker.js"), elkVars + workerBase);
  }
}

async function buildAndCopy(buildType) {
  const configs = {
    browser: {
      outdir: resolve(ROOT_DIR, "dist/browser"),
      splitting: false,
      format: "esm",
      target: "browser",
      platform: "browser",
      entrypoints: [resolve(SRC_DIR, "index.js")],
    },
    "node-esm": {
      outdir: resolve(ROOT_DIR, "dist/node-esm"),
      splitting: true,
      format: "esm",
      target: "node",
      platform: "node",
      entrypoints: [resolve(SRC_DIR, "index.js"), resolve(SRC_DIR, "worker.js")],
    },
    "node-cjs": {
      outdir: resolve(ROOT_DIR, "dist/node-cjs"),
      splitting: false,
      format: "cjs",
      target: "node",
      platform: "node",
      entrypoints: [resolve(SRC_DIR, "index.js"), resolve(SRC_DIR, "worker.js")],
    },
  };

  const config = configs[buildType];
  await buildDynamicFiles(config.platform);

  const result = await build({
    ...commonConfig,
    ...config,
  });

  if (!result.outputs || result.outputs.length === 0) {
    throw new Error(
      `No outputs generated for ${buildType} build. Result: ${JSON.stringify(result)}`
    );
  }

  if (buildType !== "browser") {
    await copyFile(resolve(ROOT_DIR, "wasm/d2.wasm"), join(config.outdir, "d2.wasm"));
    await copyFile(
      resolve(ROOT_DIR, "wasm/wasm_exec.js"),
      join(config.outdir, "wasm_exec.js")
    );
    // Copy ELK library files from d2elklayout
    await copyFile(
      resolve(ROOT_DIR, "../../d2layouts/d2elklayout/elk.js"),
      join(config.outdir, "elk.js")
    );
    await copyFile(
      resolve(ROOT_DIR, "../../d2layouts/d2elklayout/setup.js"),
      join(config.outdir, "setup.js")
    );
  }
}

try {
  await buildAndCopy("browser");
  await buildAndCopy("node-esm");
  await buildAndCopy("node-cjs");
} catch (error) {
  console.error("Build failed:", error);
  process.exit(1);
}
