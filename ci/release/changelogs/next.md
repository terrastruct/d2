Customizations and layouts take a big leap forward with this release! Put together, these improvements make beautiful diagrams like these possible:

![mono](https://user-images.githubusercontent.com/3120367/225767298-73b6466c-c245-4df9-b9fd-9e2e4c7910c2.png)
> [Playground link](https://play.d2lang.com/?script=rJLRasMwDEXf_RX6gYWNvnmwXxkiEampaxlLXVdK_n3IidekHfRlT8GxonvvuUmkZy4HD1cH0FOMoHymMp8BBJViDErS3gDUhB5EudDwOaBiuzBHXePlQcuJ6tXk6kMLJjkGK9DdbYeXj_W1B6E0_ONMdcDJYkPmohhbnlPwcF0i7ekbR05T-8CyQS7ckwjfmCgXHOkBSH-Jwdg-p7Gsv-HuVp4twla4-1XMe04EkUdxk3MnaUUtDjIV4eQAarUe3navbc62Ll1365qPeCDoMcaHqQ2tzjBhb35mwRpu9at62JkU5gBC5evZqiFIjni5m7dgc4og6uZT6ybjSO9_Qp2ca0JbbrbyJuB-AgAA__8%3D&theme=300&sketch=0&layout=elk&)

#### Features 🚀

- New class of special themes, starting with `Terminal`, and `Terminal Grayscale`. See [docs](https://d2lang.com/tour/themes/#special-themes). [#1040](https://github.com/terrastruct/d2/pull/1040), [#1041](https://github.com/terrastruct/d2/pull/1041)
- `style.font: mono` to use a monospaced font for the text/label. See [docs](https://d2lang.com/tour/style/#font). [#1010](https://github.com/terrastruct/d2/pull/1010)
- `border-radius` is supported for both `class` and `sql_table` shapes. Thanks to second-time contributor @donglixiaoche ! [#982](https://github.com/terrastruct/d2/pull/982)
- Implements `style.fill-pattern`. See [docs](https://d2lang.com/tour/style#fill-pattern). [#1024](https://github.com/terrastruct/d2/pull/1024), [#1041](https://github.com/terrastruct/d2/pull/1041)

#### Improvements 🧹

- `dagre` layouts that have a connection where one endpoint is a container is much improved. [#1011](https://github.com/terrastruct/d2/pull/1011)
- `elk` layouts have less bends in the routes. [#1033](https://github.com/terrastruct/d2/pull/1033)
- `elk` layouts center nodes better. [#1028](https://github.com/terrastruct/d2/pull/1028)
- `elk` layouts have nicer margins between node boundaries and edges. [#1028](https://github.com/terrastruct/d2/pull/1028)
- `elk` layouts container contents are centered within. [#1038](https://github.com/terrastruct/d2/pull/1038)
- `elk` layouts container dimensions fit label. [#1038](https://github.com/terrastruct/d2/pull/1038)
- `sketch` draws connections with less roughness, which especially improves look of corner bends in ELK. [#1014](https://github.com/terrastruct/d2/pull/1014)
- CSS in SVGs are diagram-specific, which means you can embed multiple D2 diagrams on a web page without fear of style conflicts. [#1016](https://github.com/terrastruct/d2/pull/1016)

#### Bugfixes ⛑️

- Fixes `d2` erroring on malformed user paths (`fdopendir` error). [util-go#10](https://github.com/terrastruct/util-go/pull/10)
- Arrowhead labels being set without maps wasn't being picked up. [#1015](https://github.com/terrastruct/d2/pull/1015)
- Fixes a `dagre` layout error with connections to a container shape with a blockstring label. [#1032](https://github.com/terrastruct/d2/pull/1032)
