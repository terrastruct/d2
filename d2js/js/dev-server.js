const fs = require("fs/promises");
const path = require("path");

const MIME_TYPES = {
  ".html": "text/html",
  ".js": "text/javascript",
  ".mjs": "text/javascript",
  ".css": "text/css",
  ".wasm": "application/wasm",
  ".svg": "image/svg+xml",
};

async function isPortAvailable(port) {
  try {
    const server = Bun.serve({
      port,
      fetch() {
        return new Response("test");
      },
    });
    server.stop();
    return true;
  } catch (err) {
    return false;
  }
}

async function findAvailablePort(startPort = 3000) {
  let port = startPort;
  while (!(await isPortAvailable(port))) {
    port++;
  }
  return port;
}

const port = await findAvailablePort(3000);

const server = Bun.serve({
  port,
  async fetch(request) {
    const url = new URL(request.url);
    let filePath = url.pathname.slice(1); // Remove leading "/"

    if (filePath === "") {
      filePath = "examples/";
    }

    try {
      const fullPath = path.join(process.cwd(), filePath);
      const stats = await fs.stat(fullPath);

      if (stats.isDirectory()) {
        const entries = await fs.readdir(fullPath);
        const links = await Promise.all(
          entries.map(async (entry) => {
            const entryPath = path.join(fullPath, entry);
            const isDir = (await fs.stat(entryPath)).isDirectory();
            const slash = isDir ? "/" : "";
            return `<li><a href="${filePath}${entry}${slash}">${entry}${slash}</a></li>`;
          })
        );

        const html = `
          <html>
            <body>
              <h1>Examples</h1>
              <ul>
                ${links.join("")}
              </ul>
            </body>
          </html>
        `;
        return new Response(html, {
          headers: { "Content-Type": "text/html" },
        });
      } else {
        const ext = path.extname(filePath);
        const mimeType = MIME_TYPES[ext] || "application/octet-stream";

        const file = Bun.file(filePath);
        return new Response(file, {
          headers: {
            "Content-Type": mimeType,
            "Access-Control-Allow-Origin": "*",
            "Cross-Origin-Opener-Policy": "same-origin",
            "Cross-Origin-Embedder-Policy": "require-corp",
          },
        });
      }
    } catch (err) {
      console.error(`Error serving ${filePath}:`, err);
      return new Response(`File not found: ${filePath}`, { status: 404 });
    }
  },
});

console.log(`Server running at http://localhost:${port}`);
