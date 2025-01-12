export async function loadFile(path) {
  if (typeof window === "undefined") {
    const fs = await import("node:fs/promises");
    const { fileURLToPath } = await import("node:url");
    const { join, dirname } = await import("node:path");
    const __dirname = dirname(fileURLToPath(import.meta.url));

    try {
      return await fs.readFile(join(__dirname, path));
    } catch (err) {
      if (err.code === "ENOENT") {
        return await fs.readFile(join(__dirname, "../wasm", path.replace("./", "")));
      }
      throw err;
    }
  }
  try {
    const response = await fetch(new URL(path, import.meta.url));
    return await response.arrayBuffer();
  } catch {
    const response = await fetch(
      new URL(`../wasm/${path.replace("./", "")}`, import.meta.url)
    );
    return await response.arrayBuffer();
  }
}

export async function createWorker() {
  if (typeof window === "undefined") {
    const { Worker } = await import("node:worker_threads");
    const { fileURLToPath } = await import("node:url");
    const { join, dirname } = await import("node:path");
    const __dirname = dirname(fileURLToPath(import.meta.url));
    return new Worker(join(__dirname, "worker.js"));
  }
  return new window.Worker(new URL("./worker.js", import.meta.url));
}
