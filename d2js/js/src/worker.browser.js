let currentPort;
let d2;
let elk;

function loadScript(content) {
  const func = new Function(content);
  func.call(globalThis);
}

export function setupMessageHandler(isNode, port, initWasm) {
  currentPort = port;

  const handleMessage = async (e) => {
    const { type, data } = e;

    switch (type) {
      case "init":
        try {
          if (isNode) {
            loadScript(data.wasmExecContent);
            loadScript(data.elkContent);
          }
          d2 = await initWasm(data.wasm);
          elk = new ELK();
          currentPort.postMessage({ type: "ready" });
        } catch (err) {
          currentPort.postMessage({ type: "error", error: err.message });
        }
        break;

      case "compile":
        try {
          // We use Go to get the intermediate ELK graph
          // Then natively run elk layout
          // This is due to elk.layout being an async function, which a
          // single-threaded WASM call cannot complete without giving control back
          // So we compute it, store it here, then during elk layout, instead
          // of computing again, we use this variable (and unset it for next call)
          // If the layout option has not been set, we generate the elk layout now
          // anyway to support `layout-engine: elk` in d2-config vars
          if (data.options.layout === "elk" || data.options.layout == null) {
            const elkGraph = await d2.getELKGraph(JSON.stringify(data));
            const response = JSON.parse(elkGraph);
            if (response.error) throw new Error(response.error.message);
            const elkGraph2 = response.data;
            const layout = await elk.layout(elkGraph2);
            globalThis.elkResult = layout;
          }
          const result = await d2.compile(JSON.stringify(data));
          const response = JSON.parse(result);
          if (response.error) throw new Error(response.error.message);
          currentPort.postMessage({ type: "result", data: response.data });
        } catch (err) {
          currentPort.postMessage({ type: "error", error: err.message });
        }
        break;

      case "render":
        try {
          const result = await d2.render(JSON.stringify(data));
          const response = JSON.parse(result);
          if (response.error) throw new Error(response.error.message);
          const decoded = new TextDecoder().decode(
            Uint8Array.from(atob(response.data), (c) => c.charCodeAt(0))
          );
          currentPort.postMessage({ type: "result", data: decoded });
        } catch (err) {
          currentPort.postMessage({ type: "error", error: err.message });
        }
        break;
    }
  };

  if (isNode) {
    port.on("message", handleMessage);
  } else {
    port.onmessage = (e) => handleMessage(e.data);
  }
}

async function initWasmBrowser(wasmBinary) {
  const go = new Go();
  const result = await WebAssembly.instantiate(wasmBinary, go.importObject);
  go.run(result.instance);
  return self.d2;
}

setupMessageHandler(false, self, initWasmBrowser);
