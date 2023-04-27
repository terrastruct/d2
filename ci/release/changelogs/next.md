#### Features 🚀

- `class` field now accepts arrays. See [docs](TODO). [#1256](https://github.com/terrastruct/d2/pull/1256)
- Pill shape is implemented with rectangles of large border radius. Thanks @Poivey ! [#1006](https://github.com/terrastruct/d2/pull/1006)

#### Improvements 🧹

- ELK self loops get distributed around the object instead of stacking [#1232](https://github.com/terrastruct/d2/pull/1232)
- ELK preserves order of objects in cycles [#1235](https://github.com/terrastruct/d2/pull/1235)

#### Bugfixes ⛑️

- Fixes an issue with markdown labels that are empty when rendered [#1223](https://github.com/terrastruct/d2/issues/1223)
- ELK self loops always have enough space for long labels [#1232](https://github.com/terrastruct/d2/pull/1232)
- Fixes panic when setting `shape` to be `class` or `sql_table` within a class [#1251](https://github.com/terrastruct/d2/pull/1251)
