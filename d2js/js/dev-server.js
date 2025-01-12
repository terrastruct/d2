const MIME_TYPES = {
  ".html": "text/html",
  ".js": "text/javascript",
  ".mjs": "text/javascript",
  ".css": "text/css",
  ".wasm": "application/wasm",
  ".svg": "image/svg+xml",
};

const server = Bun.serve({
  port: 3000,
  async fetch(request) {
    const url = new URL(request.url);
    let path = url.pathname;

    // Serve index page by default
    if (path === "/") {
      path = "/examples/basic.html";
    }

    // Handle attempts to access files in src
    if (path.startsWith("/src/")) {
      const wasmFile = path.includes("wasm_exec.js") || path.includes("d2.wasm");
      if (wasmFile) {
        path = path.replace("/src/", "/wasm/");
      }
    }

    try {
      const filePath = path.slice(1);
      const file = Bun.file(filePath);
      const exists = await file.exists();

      if (!exists) {
        return new Response(`File not found: ${path}`, { status: 404 });
      }

      // Get file extension and corresponding MIME type
      const ext = "." + filePath.split(".").pop();
      const mimeType = MIME_TYPES[ext] || "application/octet-stream";

      return new Response(file, {
        headers: {
          "Content-Type": mimeType,
          "Access-Control-Allow-Origin": "*",
          "Cross-Origin-Opener-Policy": "same-origin",
          "Cross-Origin-Embedder-Policy": "require-corp",
        },
      });
    } catch (err) {
      console.error(`Error serving ${path}:`, err);
      return new Response(`Server error: ${err.message}`, { status: 500 });
    }
  },
});

console.log(`Server running at http://localhost:3000`);
