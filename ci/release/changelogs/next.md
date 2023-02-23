#### Features 🚀

- Dark theme support! See [https://d2lang.com/tour/themes](https://d2lang.com/tour/themes).[#613](https://github.com/terrastruct/d2/pull/613)
- Many non-Latin languages (e.g. Chinese, Japanese, Korean) are usable now that multi-byte characters are measured correctly. [#817](https://github.com/terrastruct/d2/pull/817)
- Dimensions can be set on containers (layout engine dependent). [#845](https://github.com/terrastruct/d2/pull/845)

#### Improvements 🧹

- Cleaner watch mode logs without timestamps. [#830](https://github.com/terrastruct/d2/pull/830)
- Remove duplicate success logs in watch mode. [#830](https://github.com/terrastruct/d2/pull/830)
- CLI reports when a feature is incompatible with layout engine, instead of silently ignoring. [#845](https://github.com/terrastruct/d2/pull/845)
- `near` key set to direct parent or ancestor throws an appropriate error message. [#851](https://github.com/terrastruct/d2/pull/851)
- Dimensions and positions are able to be set from API. [#853](https://github.com/terrastruct/d2/pull/853)

#### Bugfixes ⛑️

- Fixes edge case where layouts with dagre show a connection from the bottom side of shapes being slightly disconnected from the shape. [#820](https://github.com/terrastruct/d2/pull/820)
- Bounding boxes weren't accounting for icons placed on the boundaries. [#879](https://github.com/terrastruct/d2/pull/879)
- Fixes rare compiler bug when using underscores in edges to create objects across containers. [#824](https://github.com/terrastruct/d2/pull/824)
- Fixes rare possibility of rendered connections being hidden or cut off. [#828](https://github.com/terrastruct/d2/pull/828)
- Creating nested children within `sql_table` and `class` shapes are now prevented (caused confusion when accidentally done). [#834](https://github.com/terrastruct/d2/pull/834)
- Fixes graph deserialization bug. [#837](https://github.com/terrastruct/d2/pull/837)
- `steps` with non-map fields could cause panics. [#783](https://github.com/terrastruct/d2/pull/783)
