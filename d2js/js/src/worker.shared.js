let currentPort;
let d2;

export function setupMessageHandler(port, initWasm) {
  currentPort = port;

  const handleMessage = async (e) => {
    const { type, data } = e;

    switch (type) {
      case "init":
        try {
          d2 = await initWasm(data.wasm);
          currentPort.postMessage({ type: "ready" });
        } catch (err) {
          currentPort.postMessage({ type: "error", error: err.message });
        }
        break;

      case "compile":
        try {
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
          currentPort.postMessage({ type: "result", data: atob(response.data) });
        } catch (err) {
          currentPort.postMessage({ type: "error", error: err.message });
        }
        break;
    }
  };

  if (typeof process !== "undefined" && process.release?.name === "node") {
    port.on("message", handleMessage);
  } else {
    port.onmessage = (e) => handleMessage(e.data);
  }
}
