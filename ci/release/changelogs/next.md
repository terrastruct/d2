D2 v0.6 introduces variable substitutions. The only major language feature left that were in the intial language design is globs, then we'll stamp it 1.0!

Layout capability also takes a subtle but important step forward: you can now customize the position of labels and icons.

#### Features 🚀

- Variables and substitutions are implemented. See [docs](https://d2lang.com/tour/vars). [#1473](https://github.com/terrastruct/d2/pull/1473)
- Configure timeout value with D2_TIMEOUT env var [#1392](https://github.com/terrastruct/d2/pull/1392)
- Scale renders and disable fit to screen with `--scale` flag [#1413](https://github.com/terrastruct/d2/pull/1413)
- `null` keyword can be used to un-declare. See [docs](https://d2lang.com/tour/overrides#null) [#1446](https://github.com/terrastruct/d2/pull/1446)
- Develop multi-board diagrams in watch mode (links to layers/scenarios/steps work in `--watch`) [#1503](https://github.com/terrastruct/d2/pull/1503)
- Glob patterns have been implemented. See [docs](https://d2lang.com/tour/globs). [#1479](https://github.com/terrastruct/d2/pull/1479)
- Ampersand filters have been implemented. See [docs](https://d2lang.com/tour/filters). [#1509](https://github.com/terrastruct/d2/pull/1509)

#### Improvements 🧹

- Display version on CLI help invocation [#1400](https://github.com/terrastruct/d2/pull/1400)
- Improved readability of connection labels when they overlap another connection [#447](https://github.com/terrastruct/d2/pull/447)
- Error message when `shape` is given a composite [#1415](https://github.com/terrastruct/d2/pull/1415)
- Improved rendering and text measurement for code shapes [#1425](https://github.com/terrastruct/d2/pull/1425)
- The autoformatter moves board declarations to the bottom of its scope [#1424](https://github.com/terrastruct/d2/pull/1424)
- All font styles in sketch mode use a consistent font-family [#1463](https://github.com/terrastruct/d2/pull/1463)
- Tooltip and link icons are positioned on shape border [#1466](https://github.com/terrastruct/d2/pull/1466)
- Tooltip and link icons are always rendered over shapes [#1467](https://github.com/terrastruct/d2/pull/1467)
- Boards with no objects are considered folders [#1504](https://github.com/terrastruct/d2/pull/1504)
- `DEBUG` environment variable ignored if set incorrectly [#1505](https://github.com/terrastruct/d2/pull/1505)

#### Bugfixes ⛑️

- Fixes edge case in compiler using dots in quotes [#1401](https://github.com/terrastruct/d2/pull/1401)
- Fixes grid label font size for TALA [#1412](https://github.com/terrastruct/d2/pull/1412)
- Fixes person shape label positioning with `multiple` or `3d` [#1478](https://github.com/terrastruct/d2/pull/1478)
