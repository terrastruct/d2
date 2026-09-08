#### Features 🚀

- TALA is now open source and bundled with D2, including the JavaScript/WASM package. Select it with `d2 --layout=tala`, `D2_LAYOUT=tala`, or `vars.d2-config.layout-engine: tala`. No separate plugin installation or license key is required.
- tala: configure layout attempts with `--tala-seeds`, `D2_TALA_SEEDS`, or `vars.d2-config.data.tala-seeds`.
- exports: render PNG, GIF, PDF, and PPTX with the built-in renderer

#### Improvements 🧹

- performance: reduce repeated work in Dagre ranking, text measurement, SVG preparation, and PNG encoding
- performance: reduce repeated work in selective imports, dynamic grid layout, and PNG connection rendering
- exports: reduce PNG memory usage and support images above the previous 67-megapixel limit
- performance: SVG rendering is approximately 10× faster across the E2E corpus. Real-world diagrams compile, lay out, and render approximately 3× faster at the median.
- renders: SVG exports are approximately 24% smaller across the E2E corpus and 18% smaller across real-world fixtures, with unchanged appearance.
- renders: render Markdown labels as native SVG instead of HTML `foreignObject` content

#### Bugfixes ⛑️

- install: reject the obsolete `--tala` installer flag with migration guidance before installing, instead of incorrectly claiming that older D2 releases bundle TALA
- sequence diagrams: keep synthetic lifeline endpoint IDs stable across architectures
- exports: honor `D2_TIMEOUT` during PNG and GIF rendering
- compiler: keep recursive globs out of class and variable definitions and report class reference cycles instead of overflowing the stack
- renders: decode gzip, Brotli, and deflate remote images before embedding them

---

For the latest d2.js changes, see separate [changelog](https://github.com/d2lang/d2/blob/master/d2js/js/CHANGELOG.md).
