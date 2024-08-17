#### Features 🚀

- Vars: Variable definitions can now refer to other variables in the current scope [#2052](https://github.com/terrastruct/d2/pull/2052)

#### Improvements 🧹

- Sequence diagram: edge groups account for edge label heights [#2038](https://github.com/terrastruct/d2/pull/2038)
- Sequence diagram: self-referential edges account for edge label heights [#2040](https://github.com/terrastruct/d2/pull/2040)
- Sequence diagram: The spacing between self-referential edges and regular edges is uniform [#2043](https://github.com/terrastruct/d2/pull/2043)
- Compiler: Error on multi-line labels in `sql_table` shapes [#2057](https://github.com/terrastruct/d2/pull/2057)
- Sequence diagram: Image shape actors can use spans and notes [#2056](https://github.com/terrastruct/d2/issues/2056)

#### Bugfixes ⛑️

- Sequence diagram: multi-line edge labels no longer can collide with other elements [#2049](https://github.com/terrastruct/d2/pull/2049)
- Sequence diagram: long self-referential edge labels no longer can collide neighboring actors (or its own) lifeline edges [#2050](https://github.com/terrastruct/d2/pull/2050)
- Globs: An edge case was fixed where globs used in edges were creating nodes when it shouldn't have [#2051](https://github.com/terrastruct/d2/pull/2051)
- Render: Multi-line class labels/headers are rendered correctly [#2057](https://github.com/terrastruct/d2/pull/2057)
- CLI: Watch mode uses correct backlinks (`_` usages) [#2058](https://github.com/terrastruct/d2/pull/2058)
