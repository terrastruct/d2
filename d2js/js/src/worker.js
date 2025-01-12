const isNode = typeof process !== "undefined" && process.release?.name === "node";
let currentPort;
let wasm;
let d2;

async function initWasm(wasmBinary) {
  const go = new Go();
  const result = await WebAssembly.instantiate(wasmBinary, go.importObject);
  go.run(result.instance);
  return isNode ? global.d2 : self.d2;
}

function setupMessageHandler(port) {
  currentPort = port;
  if (isNode) {
    port.on("message", handleMessage);
  } else {
    port.onmessage = (e) => handleMessage(e.data);
  }
}

async function handleMessage(e) {
  const { type, data } = e;

  switch (type) {
    case "init":
      try {
        if (isNode) {
          eval(data.wasmExecContent);
        }
        d2 = await initWasm(data.wasm);
        currentPort.postMessage({ type: "ready" });
      } catch (err) {
        currentPort.postMessage({
          type: "error",
          error: err.message,
        });
      }
      break;

    case "compile":
      try {
        const result = await d2.compile(JSON.stringify(data));
        const response = JSON.parse(result);
        if (response.error) {
          throw new Error(response.error.message);
        }
        currentPort.postMessage({
          type: "result",
          data: response.data,
        });
      } catch (err) {
        currentPort.postMessage({
          type: "error",
          error: err.message,
        });
      }
      break;

    case "render":
      try {
        const result = await d2.render(JSON.stringify(data));
        const response = JSON.parse(result);
        if (response.error) {
          throw new Error(response.error.message);
        }
        currentPort.postMessage({
          type: "result",
          data: atob(response.data),
        });
      } catch (err) {
        currentPort.postMessage({
          type: "error",
          error: err.message,
        });
      }
      break;
  }
}

async function init() {
  if (isNode) {
    const { parentPort } = await import("node:worker_threads");
    setupMessageHandler(parentPort);
  } else {
    setupMessageHandler(self);
  }
}

init().catch((err) => {
  console.error("Initialization error:", err);
});
