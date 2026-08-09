#### Features 🚀

- exports: gif exports work with `animate: true` keyword [#2663](https://github.com/d2lang/d2/pull/2663)

#### Improvements 🧹

- maintenance: update the Go toolchain to 1.26.5 and refresh Go, d2.js, CI, and release dependencies
- renders: update syntax highlighting and migrate archived font and PDF dependencies to maintained replacements
- d2dagre: replace the embedded JavaScript runtime with the native Go Dagro port without changing layout output
- d2sketch: replace the embedded Rough.js runtime with the native Go rough-go port without changing sketch output
- d2elk: replace the embedded JavaScript runtime with the native Go elk-go port, preserving layouts apart from sub-pixel floating-point rounding
- d2latex: replace the embedded MathJax JavaScript runtime with the native Go mathjax-go port without changing rendered formulas
- performance: native Go engine migrations substantially reduce median end-to-end D2-to-SVG conversion time in Apple M4 benchmarks:
  - Dagre is 7–9× faster
  - ELK is 40–53× faster
  - LaTeX is 21.6× faster on a four-formula diagram
- performance: remove superlinear compiler and layout bottlenecks in large globs, repeated imports, nested diagrams, compound Dagre graphs, bend-heavy ELK layouts, and style-only scenario/step boards
- d2ascii:
  - sql_table and uml class shapes are supported [#2623](https://github.com/d2lang/d2/pull/2623)
  - newlines are handled [#2626](https://github.com/d2lang/d2/pull/2626)
  - empty left columns are cropped [#2626](https://github.com/d2lang/d2/pull/2626)
- exports:
  - Chromium download through CLI for PNG exports is prompted [#2655](https://github.com/d2lang/d2/pull/2655)
  - `animate-interval` is no longer required, defaults to 1000ms for gifs [#2663](https://github.com/d2lang/d2/pull/2663)
- renders:
  - remote images are fetched more reliably [#2659](https://github.com/d2lang/d2/pull/2659)

#### Bugfixes ⛑️

- exports: pptx follows standards more closely, addressing warnings from some Powerpoint software [#2645](https://github.com/d2lang/d2/pull/2645)
- d2sequence: fix edge case of invalid sequence diagrams [#2660](https://github.com/d2lang/d2/pull/2660)
- d2svg: Text may overflow legend bounds when monospace font is used [#2674](https://github.com/d2lang/d2/pull/2674)

---

For the latest d2.js changes, see separate [changelog](https://github.com/d2lang/d2/blob/master/d2js/js/CHANGELOG.md).
