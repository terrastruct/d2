#### Features 🚀

#### Improvements 🧹

- Opacity 0 shapes no longer have a label mask which made any segment of connections going through them lower opacity [#1940](https://github.com/terrastruct/d2/pull/1940)
- Bidirectional connections are now animated in opposite directions rather than one direction [#1939](https://github.com/terrastruct/d2/pull/1939)
- Animated connections in sketch mode are now generated in normal mode to fix imperfections in the path [#1944](https://github.com/terrastruct/d2/pull/1944)

#### Bugfixes ⛑️

- Local relative icons are relative to the d2 file instead of CLI invoke path [#1924](https://github.com/terrastruct/d2/pull/1924)
- Custom label positions weren't being read when the width was smaller than the label [#1928](https://github.com/terrastruct/d2/pull/1928)
- Using `shape: circle` for arrowheads no longer removes all arrowheads along path in sketch mode [#1942](https://github.com/terrastruct/d2/pull/1942)
