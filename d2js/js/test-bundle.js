// test-bundle.js
import { build } from "bun";
import { mkdir, writeFile } from "node:fs/promises";
import { join } from "node:path";

// Ensure output directory exists
await mkdir("./test-dist", { recursive: true });

// First, write a temporary platform.js that uses browser code
const platformContent = `export * from "./platform.browser.js";`;
await writeFile("./src/platform.js", platformContent);

console.log("Building main bundle...");
const result = await build({
  entrypoints: ["./src/index.js"],
  outdir: "./test-dist",
  format: "esm",
  target: "browser",
  platform: "browser",
  minify: true,
});

if (!result.success) {
  console.error("Main bundle build failed:", result.logs);
  process.exit(1);
}

console.log("Building worker bundle...");
const workerResult = await build({
  entrypoints: ["./src/worker.js"],
  outdir: "./test-dist",
  format: "esm",
  target: "browser",
  platform: "browser",
  minify: true,
});

if (!workerResult.success) {
  console.error("Worker bundle build failed:", workerResult.logs);
  process.exit(1);
}

console.log("Builds complete");

// Create a simple server to serve the bundles
const server = Bun.serve({
  port: 3001,
  async fetch(req) {
    const url = new URL(req.url);

    try {
      // Serve main bundle
      if (url.pathname === "/d2.mjs") {
        const file = await Bun.file("./test-dist/index.js").text();
        return new Response(file, {
          headers: {
            "Content-Type": "application/javascript",
            "Access-Control-Allow-Origin": "*",
          },
        });
      }

      // Serve worker bundle
      if (url.pathname === "/worker.js") {
        const file = await Bun.file("./test-dist/worker.js").text();
        return new Response(file, {
          headers: {
            "Content-Type": "application/javascript",
            "Access-Control-Allow-Origin": "*",
          },
        });
      }

      // Serve test page
      if (url.pathname === "/") {
        return new Response(
          `
          <!DOCTYPE html>
          <html>
          <head>
            <title>D2 Test</title>
            <script type="module">
              import { D2 } from 'http://localhost:3001/d2.mjs';

              async function init() {
                try {
                  console.log("Creating D2");
                  const d2 = new D2();
                  console.log("D2 created:", d2);

                  await d2.ready;
                  console.log("D2 ready");

                  const result = await d2.compile("x -> y");
                  console.log("Compile result:", result);

                  const svg = await d2.render(result.diagram);
                  console.log("Render result:", svg);
                  document.getElementById('output').innerHTML = svg;
                } catch (error) {
                  console.error("Error:", error);
                }
              }

              init();
            </script>
          </head>
          <body>
            <div id="output"></div>
          </body>
          </html>
        `,
          {
            headers: {
              "Content-Type": "text/html",
              "Content-Security-Policy":
                "script-src 'unsafe-inline' 'wasm-unsafe-eval' http://localhost:3001; " +
                "worker-src 'self' blob:; " +
                "script-src-elem 'unsafe-inline' http://localhost:3001 blob:",
            },
          }
        );
      }

      return new Response("Not found", { status: 404 });
    } catch (error) {
      console.error(`Error serving ${url.pathname}:`, error);
      return new Response("Server Error", { status: 500 });
    }
  },
});

console.log(`Test server running at http://localhost:${server.port}/`);
