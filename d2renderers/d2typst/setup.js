// Initialize typst.ts compiler and renderer
// The typst.bundle.js provides $typst global with svg() method

// Set initialization options to load WASM modules from CDN
if (typeof $typst !== "undefined") {
  // Configure compiler WASM module location
  $typst.setCompilerInitOptions({
    getModule: () =>
      "https://cdn.jsdelivr.net/npm/@myriaddreamin/typst-ts-web-compiler@0.7.0-rc2/pkg/typst_ts_web_compiler_bg.wasm",
  });

  // Configure renderer WASM module location
  $typst.setRendererInitOptions({
    getModule: () =>
      "https://cdn.jsdelivr.net/npm/@myriaddreamin/typst-ts-renderer@0.7.0-rc2/pkg/typst_ts_renderer_bg.wasm",
  });

  // Expose to globalThis for access from Go WASM
  if (typeof globalThis !== "undefined") {
    globalThis.$typst = $typst;
  }
}
