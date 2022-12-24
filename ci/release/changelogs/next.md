![sketch](https://user-images.githubusercontent.com/3120367/209235066-d8ad6b3c-d19b-491d-b014-407f3c47407f.png)


#### Features 🚀

- `sketch` flag renders the diagram to look like it was sketched by hand. [#492](https://github.com/terrastruct/d2/pull/492)

#### Improvements 🧹

- Improved label placements for shapes with images to avoid overlapping container labels. [#474](https://github.com/terrastruct/d2/pull/474)
- Sequence diagram edge group labels have more reasonable padding. [#512](https://github.com/terrastruct/d2/pull/512)

#### Bugfixes ⛑️

- `d2 fmt` only rewrites if it has changes, instead of always rewriting. [#470](https://github.com/terrastruct/d2/pull/470)
- Fixed an issue where text could overflow in sql_table shapes. [#458](https://github.com/terrastruct/d2/pull/458)
- Fixed an issue with elk layouts accounting for edge labels as if they were placed on the side of the edge. [#483](https://github.com/terrastruct/d2/pull/483)
- Fixed an issue where dagre layouts may not have enough spacing for all edge labels. [#484](https://github.com/terrastruct/d2/pull/484)
- Icons with query parameters are now being escaped to valid SVG XML. [#438](https://github.com/terrastruct/d2/issues/438)
- Fixed connections being clipped if they were at the very top or left edges of the diagram. [#493](https://github.com/terrastruct/d2/pull/493)
- Fixed edge case where style being defined in same scope as sql_table caused compiler to skip compiling sql_table. [#506](https://github.com/terrastruct/d2/issues/506)
