#### Features 🚀

- Configure timeout value with D2_TIMEOUT env var. [#1392](https://github.com/terrastruct/d2/pull/1392)

#### Improvements 🧹

- Use shape specific sizing for grid containers [#1294](https://github.com/terrastruct/d2/pull/1294)
- Grid diagrams now support nested shapes or grid diagrams [#1309](https://github.com/terrastruct/d2/pull/1309)
- Grid diagrams will now also use `grid-gap`, `vertical-gap`, and `horizontal-gap` for padding [#1309](https://github.com/terrastruct/d2/pull/1309)
- Watch mode browser uses an error favicon to easily indicate compiler errors. Thanks @sinyo-matu ! [#1240](https://github.com/terrastruct/d2/pull/1240)
- Improves grid layout performance when there are many similarly sized shapes. [#1315](https://github.com/terrastruct/d2/pull/1315)
- Connections and labels now are adjusted for shapes with `3d` or `multiple`. [#1340](https://github.com/terrastruct/d2/pull/1340)
- Display version on CLI help invocation [#1400](https://github.com/terrastruct/d2/pull/1400)
- Improved readability of connection labels when they overlap another connection. [#447](https://github.com/terrastruct/d2/pull/447)
- Error message when `shape` is given a composite [#1415](https://github.com/terrastruct/d2/pull/1415)
- The autoformatter moves board declarations to the bottom of its scope. [#1424](https://github.com/terrastruct/d2/pull/1424)

#### Bugfixes ⛑️

- Fixes edge case in compiler using dots in quotes [#1401](https://github.com/terrastruct/d2/pull/1401)
- Fixes grid label font size for TALA [#1412](https://github.com/terrastruct/d2/pull/1412)
