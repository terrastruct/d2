#### Features 🚀

- Vars: Variable definitions can now refer to other variables in the current scope [#2052](https://github.com/terrastruct/d2/pull/2052)
- Composition: Imported boards can use underscores to reference boards beyond its own
  scope (e.g. to a sibling board at the scope its imported to) [#2075](https://github.com/terrastruct/d2/pull/2075)
- Autoformat: Reserved keywords are formatted to be lowercase [#2098](https://github.com/terrastruct/d2/pull/2098)
- Misc: characters in the unicode range for Latin-1 and geometric shapes are measured more accurately [#2100](https://github.com/terrastruct/d2/pull/2100)

#### Improvements 🧹

- Sequence diagram: edge groups account for edge label heights [#2038](https://github.com/terrastruct/d2/pull/2038)
- Sequence diagram: self-referential edges account for edge label heights [#2040](https://github.com/terrastruct/d2/pull/2040)
- Sequence diagram: The spacing between self-referential edges and regular edges is uniform [#2043](https://github.com/terrastruct/d2/pull/2043)
- Compiler: Error on multi-line labels in `sql_table` shapes [#2057](https://github.com/terrastruct/d2/pull/2057)
- Sequence diagram: Image shape actors can use spans and notes [#2056](https://github.com/terrastruct/d2/issues/2056)
- Globs: Filters work with default values (e.g. `&opacity: 1` will capture everything without opacity explicitly set) [#2090](https://github.com/terrastruct/d2/pull/2090)
- Render: connection label fills have a bit of padding and border-radius for better aesthetics [#2094](https://github.com/terrastruct/d2/pull/2094)
- Sequence diagram: the padding between message labels and message endpoints are slightly increased [#2096](https://github.com/terrastruct/d2/pull/2096)

#### Bugfixes ⛑️

- Sequence diagram: multi-line edge labels no longer can collide with other elements [#2049](https://github.com/terrastruct/d2/pull/2049)
- Sequence diagram: long self-referential edge labels no longer can collide neighboring actors (or its own) lifeline edges [#2050](https://github.com/terrastruct/d2/pull/2050)
- Sequence diagram: fixes layout when sequence diagrams are in children boards (e.g. a layer) [#1692](https://github.com/terrastruct/d2/issues/1692)
- Globs: An edge case was fixed where globs used in edges were creating nodes when it shouldn't have [#2051](https://github.com/terrastruct/d2/pull/2051)
- Render: Multi-line class labels/headers are rendered correctly [#2057](https://github.com/terrastruct/d2/pull/2057)
- CLI: Watch mode uses correct backlinks (`_` usages) [#2058](https://github.com/terrastruct/d2/pull/2058)
- Vars: Spread variables are inserted in place instead of appending to end of scope [#2062](https://github.com/terrastruct/d2/pull/2062)
- Imports: fix local icon imports from files that are imported [#2066](https://github.com/terrastruct/d2/pull/2066)
- CLI: fixes edge case of watch mode links to nested board that had more nested boards not working [#2070](https://github.com/terrastruct/d2/pull/2070)
- CLI: fixes theme flag not being passed to GIF outputs [#2071](https://github.com/terrastruct/d2/pull/2071)
- CLI: fixes scale flag not being passed to animated SVG outputs [#2071](https://github.com/terrastruct/d2/pull/2071)
- CLI: pptx exports use theme flags correctly [#2099](https://github.com/terrastruct/d2/pull/2099)
- Imports: importing files with url links is fixed [#2105](https://github.com/terrastruct/d2/pull/2105)
- Vars: substitutions consistently appear in the specified order [#2116](https://github.com/terrastruct/d2/pull/2116)
- Composition: linking to invalid boards no longer produces an invalid link [#2118](https://github.com/terrastruct/d2/pull/2118)
