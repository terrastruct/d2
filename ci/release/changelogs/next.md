#### Features 🚀

- sequence diagrams: add opt-in `shape: sequence-diagram` with explicit message and actor groups, spacing controls, span labels, actor repetition, and message numbering; `shape: sequence_diagram` retains the original behavior. See the [v2 guide](../../../docs/sequence-diagrams.md).
- exports: render PNG, GIF, PDF, and PPTX with the built-in renderer

#### Improvements 🧹

- renders: render Markdown labels as native SVG instead of HTML `foreignObject` content

#### Bugfixes ⛑️

- sequence diagrams: keep synthetic lifeline endpoint IDs stable across architectures
- exports: honor `D2_TIMEOUT` during PNG and GIF rendering
- compiler: keep recursive globs out of class and variable definitions and report class reference cycles instead of overflowing the stack
- renders: decode gzip, Brotli, and deflate remote images before embedding them

---

For the latest d2.js changes, see separate [changelog](https://github.com/d2lang/d2/blob/master/d2js/js/CHANGELOG.md).
