#### Features 🚀

- `style.font: mono` to use a monospaced font for the text/label [#1010](https://github.com/terrastruct/d2/pull/1010)
- `border-radius` is supported for both `class` and `sql_table` shapes. [#982](https://github.com/terrastruct/d2/pull/982)
- Implements `style.fill-pattern`. Currently only value supported is `dots`. [#1024](https://github.com/terrastruct/d2/pull/1024)

#### Improvements 🧹

- `dagre` layouts that have a connection where one endpoint is a container is much improved. [#1011](https://github.com/terrastruct/d2/pull/1011)
- `elk` layouts favor balance over straight edges. [#1028](https://github.com/terrastruct/d2/pull/1028)
- `elk` layouts have nicer margins between node boundaries and edges. [#1028](https://github.com/terrastruct/d2/pull/1028)
- `sketch` draws connections with less roughness, which especially improves look of corner bends in ELK. [#1014](https://github.com/terrastruct/d2/pull/1014)
- CSS in SVGs are diagram-specific, which means you can embed multiple D2 diagrams on a web page without fear of style conflicts. [#1016](https://github.com/terrastruct/d2/pull/1016)

#### Bugfixes ⛑️

- Fixes `d2` erroring on malformed user paths (`fdopendir` error). [util-go#10](https://github.com/terrastruct/util-go/pull/10)
- Arrowhead labels being set without maps wasn't being picked up. [#1015](https://github.com/terrastruct/d2/pull/1015)
