#### Features 🚀

- Vars: vars in markdown blocks are substituted [#2218](https://github.com/terrastruct/d2/pull/2218)
- Markdown: Github-flavored tables work in `md` blocks [#2221](https://github.com/terrastruct/d2/pull/2221)

#### Improvements 🧹

- Composition: links pointing to own board are purged [#2203](https://github.com/terrastruct/d2/pull/2203)
- Syntax: reserved keywords must be unquoted [#2231](https://github.com/terrastruct/d2/pull/2231)
- Latex: Backslashes in Latex blocks do not escape [#2232](https://github.com/terrastruct/d2/pull/2232)
  - This is a breaking change. Previously Latex blocks required escaping the backslash. So
    for older D2 versions, you should remove the excess backslashes.

#### Bugfixes ⛑️

- Imports: fixes using substitutions in `icon` values [#2207](https://github.com/terrastruct/d2/pull/2207)
- Markdown: fixes ampersands in URLs in markdown [#2219](https://github.com/terrastruct/d2/pull/2219)
