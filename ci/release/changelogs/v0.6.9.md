#### Features 🚀

- Animations: `style.animated: true` is supported on shapes [#2250](https://github.com/terrastruct/d2/pull/2250)
- Connections now support `link` [#1955](https://github.com/terrastruct/d2/pull/1955)
- Vars: vars in markdown blocks are substituted [#2218](https://github.com/terrastruct/d2/pull/2218)
- Markdown: Github-flavored tables work in `md` blocks [#2221](https://github.com/terrastruct/d2/pull/2221)
- Render: adds box arrowheads [#2227](https://github.com/terrastruct/d2/issues/2227)
- `d2 fmt` now supports a `--check` flag [#2253](https://github.com/terrastruct/d2/pull/2253)
- CLI: PNG output to stdout is supported using `--stdout-format png -` [#2291](https://github.com/terrastruct/d2/pull/2291)
- Globs: `&connected` and `&leaf` filters are implemented [#2299](https://github.com/terrastruct/d2/pull/2299)
- CLI: add --no-xml-tag for direct HTML embedding [#2302](https://github.com/terrastruct/d2/pull/2302)
- CLI: `play` cmd added for opening d2 input in online playground [#2242](https://github.com/terrastruct/d2/pull/2242)

#### Improvements 🧹

- Composition: links pointing to own board are purged [#2203](https://github.com/terrastruct/d2/pull/2203)
- Syntax: reserved keywords must be unquoted [#2231](https://github.com/terrastruct/d2/pull/2231)
- Latex: Backslashes in Latex blocks do not escape [#2232](https://github.com/terrastruct/d2/pull/2232)
  - This is a breaking change. Previously Latex blocks required escaping the backslash. So
    for older D2 versions, you should remove the excess backslashes.
- Links: non-http url scheme links are supported (e.g. `x.link: vscode://file/`) [#2237](https://github.com/terrastruct/d2/issues/2237)
- Compiler: reserved keywords with missing values error instead of silently doing nothing [#2251](https://github.com/terrastruct/d2/pull/2251)
- Render: SVG outputs conform to stricter HTML standards, e.g. no duplicate ids [#2273](https://github.com/terrastruct/d2/issues/2273)
- Themes: theme names are consistently cased [#2322](https://github.com/terrastruct/d2/pull/2322)
- Nears: constant nears avoid collision with edge routes [#2327](https://github.com/terrastruct/d2/pull/2327)

#### Bugfixes ⛑️

- Imports: fixes using substitutions in `icon` values [#2207](https://github.com/terrastruct/d2/pull/2207)
- Markdown: fixes ampersands in URLs in markdown [#2219](https://github.com/terrastruct/d2/pull/2219)
- Globs: fixes edge case where globs with imported boards would create empty boards [#2247](https://github.com/terrastruct/d2/pull/2247)
- Sequence diagrams: fixes alignment of notes when self messages are above it [#2264](https://github.com/terrastruct/d2/pull/2264)
- Null: fixes `null`ing a connection with absolute syntax [#2318](https://github.com/terrastruct/d2/issues/2318)
- Gradients: works with connection fills [#2326](https://github.com/terrastruct/d2/pull/2326)
- Latex: fixes backslashes doubling on successive parses [#2328](https://github.com/terrastruct/d2/pull/2328)
