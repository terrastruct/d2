#### Features 🚀

- `sketch` flag renders the diagram to look like it was sketched by hand. [#492](https://github.com/terrastruct/d2/pull/492)

#### Improvements 🧹

- Improved label placements for shapes with images to avoid overlapping container labels. [#474](https://github.com/terrastruct/d2/pull/474)

#### Bugfixes ⛑️

- `d2 fmt` only rewrites if it has changes, instead of always rewriting. [#470](https://github.com/terrastruct/d2/pull/470)
- Fixed an issue where text could overflow in sql_table shapes. [#458](https://github.com/terrastruct/d2/pull/458)
- Fixed an issue with elk layouts accounting for edge labels as if they were placed on the side of the edge. [#483](https://github.com/terrastruct/d2/pull/483)
- Fixed an issue where dagre layouts may not have enough spacing for all edge labels. [#484](https://github.com/terrastruct/d2/pull/484)
- Fixed connections being clipped if they were at the very top or left edges of the diagram. [#493](https://github.com/terrastruct/d2/pull/493)
