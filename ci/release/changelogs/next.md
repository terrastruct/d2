#### Features 🚀

- ASCII renders. Output `txt` for d2 to render diagrams as ASCII art [#2572](https://github.com/terrastruct/d2/pull/2572)
- `cross` arrowhead shape is available [#2190](https://github.com/terrastruct/d2/pull/2190)
- `style.underline` support for class fields and methods [#2544](https://github.com/terrastruct/d2/pull/2544)
- markdown, latex, and code can be used as edge labels [#2545](https://github.com/terrastruct/d2/pull/2545)
- border-x label positioning functionality [#2549](https://github.com/terrastruct/d2/pull/2549)
- tooltips with `near` set always show even without hover [#2564](https://github.com/terrastruct/d2/pull/2564)
- CLI supports customizing monospace fonts with `--font-mono`, `--font-mono-bold`, `--font-mono-italic`, and `--font-mono-semibold` flags [#2590](https://github.com/terrastruct/d2/pull/2590)

#### Improvements 🧹

- labels on scenario/step boards can be set with primary value (like layers) [#2579](https://github.com/terrastruct/d2/pull/2579)
- autoformatter preserves order of boards [#2580](https://github.com/terrastruct/d2/pull/2580)
- rename "Legend" with a title/label of your choosing (especially useful for non-English diagrams) [#2582](https://github.com/terrastruct/d2/pull/2582)

#### Bugfixes ⛑️

- actors in sequence diagrams with labels and icons together no longer overlap, position keywords now work too [#2548](https://github.com/terrastruct/d2/pull/2548)
- fix double glob behavior in scenarios (wasn't propagating correctly) [#2557](https://github.com/terrastruct/d2/pull/2557)
- fix diagram bounding box not accounting for legend in some cases [#2584](https://github.com/terrastruct/d2/pull/2584)

---

For the latest d2.js changes, see separate [changelog](https://github.com/terrastruct/d2/blob/master/d2js/js/CHANGELOG.md).
