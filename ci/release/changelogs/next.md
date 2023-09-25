#### Features 🚀

- UTF-16 files are automatically detected and supported [#1525](https://github.com/terrastruct/d2/pull/1525)
- Grid diagrams can now have simple edges between cells [#1586](https://github.com/terrastruct/d2/pull/1586)

#### Improvements 🧹

- Globs are lazily-evaluated [#1552](https://github.com/terrastruct/d2/pull/1552)
- Latex now includes Mathjax's ASM extension for more powerful Latex functionality [#1544](https://github.com/terrastruct/d2/pull/1544)
- `font-color` works on Markdown [#1546](https://github.com/terrastruct/d2/pull/1546)
- `font-color` works on arrowheads [#1582](https://github.com/terrastruct/d2/pull/1582)
- CLI failure message includes input path [#1617](https://github.com/terrastruct/d2/pull/1617)

#### Bugfixes ⛑️

- Fixes `d2 fmt` to format all files passed as arguments rather than first non-formatted only [#1523](https://github.com/terrastruct/d2/issues/1523)
- Fixes Markdown cropping last element in mixed-element blocks (e.g. em and strong) [#1543](https://github.com/terrastruct/d2/issues/1543)
- Fixes missing compile error for non-blockstring empty labels [#1590](https://github.com/terrastruct/d2/issues/1590)
- Fixes multiple constant nears overlapping in some cases [#1591](https://github.com/terrastruct/d2/issues/1591)
- Fixes error with an empty nested grid [#1594](https://github.com/terrastruct/d2/issues/1594)
- Fixes incorrect `d2fmt` with variable substitution mid-string [#1611](https://github.com/terrastruct/d2/issues/1611)
- Fixes dagre error with child named id [#1610](https://github.com/terrastruct/d2/issues/1610)
- Fixes font-size missing from rendered code shape [#1614](https://github.com/terrastruct/d2/issues/1614)
