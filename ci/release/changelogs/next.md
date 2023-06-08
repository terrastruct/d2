#### Features 🚀

- D2 files have the ability to import from other D2 files. See [docs](https://d2lang.com/tour/imports). [#1371](https://github.com/terrastruct/d2/pull/1371)

#### Improvements 🧹

- Use shape specific sizing for grid containers [#1294](https://github.com/terrastruct/d2/pull/1294)
- Grid diagrams now support nested shapes or grid diagrams [#1309](https://github.com/terrastruct/d2/pull/1309)
- Grid diagrams will now also use `grid-gap`, `vertical-gap`, and `horizontal-gap` for padding [#1309](https://github.com/terrastruct/d2/pull/1309)
- Watch mode browser uses an error favicon to easily indicate compiler errors. Thanks @sinyo-matu ! [#1240](https://github.com/terrastruct/d2/pull/1240)
- Improves grid layout performance when there are many similarly sized shapes. [#1315](https://github.com/terrastruct/d2/pull/1315)
- Connections and labels now are adjusted for shapes with `3d` or `multiple`. [#1340](https://github.com/terrastruct/d2/pull/1340)
- `sql_table` now alternatively takes an array of constraints instead of being limited to a single one. Thanks @satoqz ! [#1245](https://github.com/terrastruct/d2/pull/1245)
- Constraints in `sql_table` render even if they have no matching abbreviation [#1372](https://github.com/terrastruct/d2/pull/1372)
- Constraints in `sql_table` sheds their excessive letter-spacing and is padded from the end consistently [#1372](https://github.com/terrastruct/d2/pull/1372)
- Duplicate image URLs in icons are only fetched once [#1373](https://github.com/terrastruct/d2/pull/1373)
- In watch mode, images are cached by default across compiles. Can be disabled with flag `--img-cache=0`. [#1373](https://github.com/terrastruct/d2/pull/1373)
- Common invalid array separator `,` usage in class arrays returns a helpful error message [#1376](https://github.com/terrastruct/d2/pull/1376)
- Invalid `constraint` usage is met with an error message, preventing a common mistake of omitting `shape: sql_table` [#1379](https://github.com/terrastruct/d2/pull/1379)

#### Bugfixes ⛑️

- Shadow is cut off when `--pad` is 0. Thank you @LeonardsonCC ! [#1326](https://github.com/terrastruct/d2/pull/1326)
- Fixes grid layout overwriting label placements for nested objects. [#1345](https://github.com/terrastruct/d2/pull/1345)
- Fixes fonts not rendering correctly on certain platforms. Thanks @mikeday for identifying the solution. [#1356](https://github.com/terrastruct/d2/pull/1356)
- Fixes folders not rendering in animations (`--animate-interval`) [#1357](https://github.com/terrastruct/d2/pull/1357)
- Fixes panic using reserved keywords as containers [#1358](https://github.com/terrastruct/d2/pull/1358)
- When multiple classes are applied changing different attributes of arrowheads, they are
  all applied instead of only the last one [#1362](https://github.com/terrastruct/d2/pull/1362)
- Prevent empty block strings [#1364](https://github.com/terrastruct/d2/pull/1364)
- Fixes dagre mis-aligning a nested shape's connection. [#1370](https://github.com/terrastruct/d2/pull/1370)
