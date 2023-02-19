#### Features 🚀

- Many non-Latin languages (e.g. Chinese, Japanese, Korean) are usable now that multi-byte characters are measured correctly. [#817](https://github.com/terrastruct/d2/pull/817)
- Fix duplicate success logs in watch mode. [830](https://github.com/terrastruct/d2/pull/830)

#### Improvements 🧹

- Cleaner watch mode logs without timestamps. [830](https://github.com/terrastruct/d2/pull/830)

#### Bugfixes ⛑️

- Fixes edge case where layouts with dagre show a connection from the bottom side of shapes being slightly disconnected from the shape. [#820](https://github.com/terrastruct/d2/pull/820)
- Fixes rare compiler bug when using underscores in edges to create objects across containers. [#824](https://github.com/terrastruct/d2/pull/824)
- Fixes rare possibility of rendered connections being hidden or cut off. [#828](https://github.com/terrastruct/d2/pull/828)
- Creating nested children within `sql_table` and `class` shapes are now prevented (caused confusion when accidentally done). [#834](https://github.com/terrastruct/d2/pull/834)
- Fixes graph deserialization bug. [#837](https://github.com/terrastruct/d2/pull/837)