import { build } from "bun";
import { copyFile, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { join, resolve } from "node:path";

const __dirname = new URL(".", import.meta.url).pathname;
const ROOT_DIR = resolve(__dirname);
const SRC_DIR = resolve(ROOT_DIR, "src");

await rm("./dist", { recursive: true, force: true });
await mkdir("./dist/browser", { recursive: true });
await mkdir("./dist/node-esm", { recursive: true });
await mkdir("./dist/node-cjs", { recursive: true });

const wasmBinary = await readFile("./wasm/d2.wasm");
const wasmExecJs = await readFile("./wasm/wasm_exec.js", "utf8");

await writeFile(
  join(SRC_DIR, "wasm-loader.browser.js"),
  `export const wasmBinary = Uint8Array.from(atob("${Buffer.from(wasmBinary).toString(
    "base64"
  )}"), c => c.charCodeAt(0));
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

  const platformPath = join(SRC_DIR, "platform.js");
  await writeFile(platformPath, platformContent);

  const workerSource =
    platform === "node"
      ? join(SRC_DIR, "worker.node.js")
      : join(SRC_DIR, "worker.browser.js");

  const workerTarget = join(SRC_DIR, "worker.js");
  const workerContent = await readFile(workerSource, "utf8");
  await writeFile(workerTarget, workerContent);
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
