#### Features 🚀

#### Improvements 🧹

- Use shape specific sizing for grid containers [#1294](https://github.com/terrastruct/d2/pull/1294)
- Grid diagrams now support nested shapes or grid diagrams [#1309](https://github.com/terrastruct/d2/pull/1309)
- Grid diagrams will now also use `grid-gap`, `vertical-gap`, and `horizontal-gap` for padding [#1309](https://github.com/terrastruct/d2/pull/1309)
- Watch mode browser uses an error favicon to easily indicate compiler errors. Thanks @sinyo-matu ! [#1240](https://github.com/terrastruct/d2/pull/1240)
- Improves grid layout performance when there are many similarly sized shapes. [#1315](https://github.com/terrastruct/d2/pull/1315)
- Connections and labels now are adjusted for shapes with `3d` or `multiple`. [#1340](https://github.com/terrastruct/d2/pull/1340)
- The autoformatter moves board declarations to the bottom of its scope. [#1424](https://github.com/terrastruct/d2/pull/1424)

#### Bugfixes ⛑️

- Shadow is cut off when `--pad` is 0. Thank you @LeonardsonCC ! [#1326](https://github.com/terrastruct/d2/pull/1326)
- Fixes grid layout overwriting label placements for nested objects. [#1345](https://github.com/terrastruct/d2/pull/1345)
- Fixes fonts not rendering correctly on certain platforms. Thanks @mikeday for identifying the solution. [#1356](https://github.com/terrastruct/d2/pull/1356)
- Fixes folders not rendering in animations (`--animate-interval`) [#1357](https://github.com/terrastruct/d2/pull/1357)
- Fixes panic using reserved keywords as containers [#1358](https://github.com/terrastruct/d2/pull/1358)
- When multiple classes are applied changing different attributes of arrowheads, they are
  all applied instead of only the last one [#1362](https://github.com/terrastruct/d2/pull/1362)
- Prevent empty block strings [#1364](https://github.com/terrastruct/d2/pull/1364)
