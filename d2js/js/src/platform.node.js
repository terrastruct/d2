let nodeModules = null;

async function loadNodeModules() {
  if (!nodeModules) {
    nodeModules = {
      fs: await import("fs/promises"),
      path: await import("path"),
      url: await import("url"),
      worker: await import("worker_threads"),
    };
  }
  return nodeModules;
}

export async function loadFile(path) {
  const modules = await loadNodeModules();
  const readFile = modules.fs.readFile;
  const { join, dirname } = modules.path;
  const { fileURLToPath } = modules.url;
  const __dirname = dirname(fileURLToPath(import.meta.url));

  try {
    return await readFile(join(__dirname, path));
  } catch (err) {
    if (err.code === "ENOENT") {
      return await readFile(join(__dirname, "../../../wasm", path.replace("./", "")));
    }
    throw err;
  }
}

export async function createWorker() {
  const modules = await loadNodeModules();
  const { Worker } = modules.worker;
  const { join, dirname } = modules.path;
  const { fileURLToPath } = modules.url;
  const __dirname = dirname(fileURLToPath(import.meta.url));
  const workerPath = join(__dirname, "worker.js");
  return new Worker(workerPath);
}
