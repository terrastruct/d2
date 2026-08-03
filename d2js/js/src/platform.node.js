let nodeModules = null;

// Injected at runtime by the node bundle banners in build.js.
const moduleDir = __D2_NODE_MODULE_DIR__;

async function loadNodeModules() {
  if (!nodeModules) {
    nodeModules = {
      fs: await import("node:fs/promises"),
      path: await import("node:path"),
      worker: await import("node:worker_threads"),
    };
  }
  return nodeModules;
}

export async function loadFile(path) {
  const modules = await loadNodeModules();
  const readFile = modules.fs.readFile;
  const { join } = modules.path;

  try {
    return await readFile(join(moduleDir, path));
  } catch (err) {
    if (err.code === "ENOENT") {
      return await readFile(join(moduleDir, "../../../wasm", path.replace("./", "")));
    }
    throw err;
  }
}

export async function createWorker() {
  const modules = await loadNodeModules();
  const { Worker } = modules.worker;
  const { join } = modules.path;
  const workerPath = join(moduleDir, "worker.js");
  return new Worker(workerPath);
}
