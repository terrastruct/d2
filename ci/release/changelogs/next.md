#### Features 🚀

#### Improvements 🧹

- Grid cells can now contain nested edges [#1629](https://github.com/terrastruct/d2/pull/1629)
- Edges can now go across constant nears, sequence diagrams, and grids including nested ones. [#1631](https://github.com/terrastruct/d2/pull/1631)

#### Bugfixes ⛑️

- Fixes a bug calculating grid height with only grid-rows and different horizontal-gap and vertical-gap values. [#1646](https://github.com/terrastruct/d2/pull/1646)
- Grid layout now accounts for each cell's outside labels and icons [#1624](https://github.com/terrastruct/d2/pull/1624)
- Grid layout now accounts for labels wider or taller than the shape and fixes default label positions for image grid cells. [#1670](https://github.com/terrastruct/d2/pull/1670)
- Fixes a panic with a spread substitution in a glob map [#1643](https://github.com/terrastruct/d2/pull/1643)
- Fixes use of `null` in `sql_table` constraints (ty @landmaj) [#1660](https://github.com/terrastruct/d2/pull/1660)
- Fixes elk growing shapes with width/height set [#1679](https://github.com/terrastruct/d2/pull/1679)
- Adds a compiler error when accidentally using an arrowhead on a shape [#1686](https://github.com/terrastruct/d2/pull/1686)
- Correctly reports errors from invalid values set by globs. [#1691](https://github.com/terrastruct/d2/pull/1691)
