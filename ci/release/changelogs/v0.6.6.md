#### Features 🚀

- Glob inverse filters are implemented (e.g. `*: {!&shape: circle; style.fill: red}` to turn all non-circles red) [#2008](https://github.com/terrastruct/d2/pull/2008)
- Globs can be used in glob filter values, including checking for existence (e.g. `*: {&link: *; style.fill: red}` to turn all objects with a link red) [#2009](https://github.com/terrastruct/d2/pull/2009)

#### Improvements 🧹

- Opacity 0 shapes no longer have a label mask which made any segment of connections going through them lower opacity [#1940](https://github.com/terrastruct/d2/pull/1940)
- Bidirectional connections are now animated in opposite directions rather than one direction [#1939](https://github.com/terrastruct/d2/pull/1939)

#### Bugfixes ⛑️

- Local relative icons are relative to the d2 file instead of CLI invoke path [#1924](https://github.com/terrastruct/d2/pull/1924)
- Custom label positions weren't being read when the width was smaller than the label [#1928](https://github.com/terrastruct/d2/pull/1928)
- Using `shape: circle` for arrowheads no longer removes all arrowheads along path in sketch mode [#1942](https://github.com/terrastruct/d2/pull/1942)
- Globs to null connections work [#1965](https://github.com/terrastruct/d2/pull/1965)
- Edge globs setting styles inherit correctly in child boards [#1967](https://github.com/terrastruct/d2/pull/1967)
- Board links imported with spread imports work [#1972](https://github.com/terrastruct/d2/pull/1972)
- Fix importing a file with nested boards [#1998](https://github.com/terrastruct/d2/pull/1998)
- Fix importing a file with underscores in links [#1999](https://github.com/terrastruct/d2/pull/1999)
- Replace a panic with an error message resulting from invalid `link` usage [#2011](https://github.com/terrastruct/d2/pull/2011)
- Fix globs not applying to scenarios on keys that were applied in earlier scenarios [#2021](https://github.com/terrastruct/d2/pull/2021)
- Fix edge case of invalid SVG from code blocks [#2031](https://github.com/terrastruct/d2/pull/2031)
