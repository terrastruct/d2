#### Features 🚀

- exports: gif exports work with `animate: true` keyword [#2663](https://github.com/d2lang/d2/pull/2663)

#### Improvements 🧹

- plugins: make render post-processing optional and deprecate it for removal after one protocol compatibility cycle; legacy external `postprocess` commands remain supported during the transition
- api: deprecate legacy layout-feature constants, raw-WASM ELK/object-order bridges, unused public wrappers, and test-only comparison, validation, and logging helpers; compatibility entry points remain callable for one release while in-repository callers use supported or internal replacements
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
- performance: remove superlinear compiler and layout bottlenecks in large globs, repeated imports, nested diagrams, compound Dagre graphs, bend-heavy ELK layouts, and style-only scenario/step boards [#2827](https://github.com/d2lang/d2/pull/2827). Median Apple M4 single-CPU synthetic stress benchmarks for these targeted cliffs, rather than corpus-wide conversion averages, improve by:
  - leading glob applied to 1,000 fields: 1.59s to 12ms (~133× faster)
  - 100 references into the same 100-field import file: 54ms to 12ms (~4.6× faster)
  - 4,000 flat fields: 67ms to 7ms (~9.6× faster)
  - 3,200 distinct edges: 200ms to 19ms (~10.6× faster)
  - 32-node base with 8 style-only scenarios and 8 steps (17 boards total): 40ms to 7.3ms (~5.5× faster)
  - 340-object nested D2 Dagre layout: 1.30s to 0.88s (~32% faster)
  - Dagro depth-100 compound layout: 3.60s to 0.90s (~4× faster)
  - 250-node, 1,000-edge ELK layout: 337ms to 238ms (~30% faster)
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

- d2svg: render one-stop gradients with finite SVG offsets
- compiler: make suffix globs match only names with the requested suffix
- d2svg: preserve connection links on LaTeX, Markdown, and code labels
- exports: pptx follows standards more closely, addressing warnings from some Powerpoint software [#2645](https://github.com/d2lang/d2/pull/2645)
- d2sequence: fix edge case of invalid sequence diagrams [#2660](https://github.com/d2lang/d2/pull/2660)
- d2svg: Text may overflow legend bounds when monospace font is used [#2674](https://github.com/d2lang/d2/pull/2674)

---

For the latest d2.js changes, see separate [changelog](https://github.com/d2lang/d2/blob/master/d2js/js/CHANGELOG.md).
