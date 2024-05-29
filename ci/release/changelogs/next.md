#### Features 🚀

- Vars: vars in markdown blocks are substituted [#2218](https://github.com/terrastruct/d2/pull/2218)
- Markdown: Github-flavored tables work in `md` blocks [#2221](https://github.com/terrastruct/d2/pull/2221)

#### Improvements 🧹

<<<<<<< HEAD
- Composition: links pointing to own board are purged [#2203](https://github.com/terrastruct/d2/pull/2203)
- Syntax: reserved keywords must be unquoted [#2231](https://github.com/terrastruct/d2/pull/2231)
- Latex: Backslashes in Latex blocks do not escape [#2232](https://github.com/terrastruct/d2/pull/2232)
  - This is a breaking change. Previously Latex blocks required escaping the backslash. So
    for older D2 versions, you should remove the excess backslashes.
- Links: non-http url scheme links are supported (e.g. `x.link: vscode://file/`) [#2237](https://github.com/terrastruct/d2/issues/2237)
||||||| parent of 703bcd388 (update next.md)
- Sequence diagram: edge groups account for edge label heights [#2038](https://github.com/terrastruct/d2/pull/2038)
- Sequence diagram: self-referential edges account for edge label heights [#2040](https://github.com/terrastruct/d2/pull/2040)
- Sequence diagram: The spacing between self-referential edges and regular edges is uniform [#2043](https://github.com/terrastruct/d2/pull/2043)
- Compiler: Error on multi-line labels in `sql_table` shapes [#2057](https://github.com/terrastruct/d2/pull/2057)
- Sequence diagram: Image shape actors can use spans and notes [#2056](https://github.com/terrastruct/d2/issues/2056)
- Globs: Filters work with default values (e.g. `&opacity: 1` will capture everything without opacity explicitly set) [#2090](https://github.com/terrastruct/d2/pull/2090)
- Render: connection label fills have a bit of padding and border-radius for better aesthetics [#2094](https://github.com/terrastruct/d2/pull/2094)
- Sequence diagram: the padding between message labels and message endpoints are slightly increased [#2096](https://github.com/terrastruct/d2/pull/2096)
=======
- Sequence diagram: edge groups account for edge label heights [#2038](https://github.com/terrastruct/d2/pull/2038)
- Sequence diagram: self-referential edges account for edge label heights [#2040](https://github.com/terrastruct/d2/pull/2040)
- Sequence diagram: The spacing between self-referential edges and regular edges is uniform [#2043](https://github.com/terrastruct/d2/pull/2043)
- Compiler: Error on multi-line labels in `sql_table` shapes [#2057](https://github.com/terrastruct/d2/pull/2057)
- Sequence diagram: Image shape actors can use spans and notes [#2056](https://github.com/terrastruct/d2/issues/2056)
- Globs: Filters work with default values (e.g. `&opacity: 1` will capture everything without opacity explicitly set) [#2090](https://github.com/terrastruct/d2/pull/2090)
- Render: connection label fills have a bit of padding and border-radius for better aesthetics [#2094](https://github.com/terrastruct/d2/pull/2094)
- Sequence diagram: the padding between message labels and message endpoints are slightly increased [#2096](https://github.com/terrastruct/d2/pull/2096)
- Opacity 0 shapes no longer have a label mask which made any segment of connections going through them lower opacity [#1940](https://github.com/terrastruct/d2/pull/1940)
- Bidirectional connections are now animated in opposite directions rather than one direction [#1939](https://github.com/terrastruct/d2/pull/1939)
- Connections now support `link` in SVGs [#1955](https://github.com/terrastruct/d2/pull/1955)
>>>>>>> 703bcd388 (update next.md)

#### Bugfixes ⛑️

- Imports: fixes using substitutions in `icon` values [#2207](https://github.com/terrastruct/d2/pull/2207)
- Markdown: fixes ampersands in URLs in markdown [#2219](https://github.com/terrastruct/d2/pull/2219)
- Globs: fixes edge case where globs with imported boards would create empty boards [#2247](https://github.com/terrastruct/d2/pull/2247)
