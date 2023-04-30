#### Features 🚀

- `class` field now accepts arrays. See [docs](TODO). [#1256](https://github.com/terrastruct/d2/pull/1256)
- Pill shape is implemented with rectangles of large border radius. Thanks @Poivey ! [#1006](https://github.com/terrastruct/d2/pull/1006)

#### Improvements 🧹

- ELK self loops get distributed around the object instead of stacking [#1232](https://github.com/terrastruct/d2/pull/1232)
- ELK preserves order of objects in cycles [#1235](https://github.com/terrastruct/d2/pull/1235)
- Improper usages of `class` and `style` get error messages [#1254](https://github.com/terrastruct/d2/pull/1254)
- Improves scaling of object widths/heights in grid diagrams [#1263](https://github.com/terrastruct/d2/pull/1263)

#### Bugfixes ⛑️

- Fixes an issue with markdown labels that are empty when rendered [#1223](https://github.com/terrastruct/d2/issues/1223)
- ELK self loops always have enough space for long labels [#1232](https://github.com/terrastruct/d2/pull/1232)
- Fixes panic when setting `shape` to be `class` or `sql_table` within a class [#1251](https://github.com/terrastruct/d2/pull/1251)
- Fixes rare panic exporting to gifs [#1257](https://github.com/terrastruct/d2/pull/1257)
- Fixes bad performance in large grid diagrams [#1263](https://github.com/terrastruct/d2/pull/1263)
- Fixes bug in ELK when container has ID "root" [#1268](https://github.com/terrastruct/d2/pull/1268)
