Many have asked how to get the diagram to look like the one on D2's [cheat sheet](https://d2lang.com/tour/cheat-sheet). With this release, now you can! See [https://d2lang.com/tour/themes](https://d2lang.com/tour/themes) for more.

![sketch](https://user-images.githubusercontent.com/3120367/209235066-d8ad6b3c-d19b-491d-b014-407f3c47407f.png)

The Slack app for D2 has now hit production, so if you're looking for the quickest way to express a visual model without interrupting the conversation flow, go to [http://d2lang.com/tour/slack](http://d2lang.com/tour/slack) to install.

Hope everyone is enjoying the holidays this week!

#### Features 🚀

- `sketch` flag renders the diagram to look like it was sketched by hand. [#492](https://github.com/terrastruct/d2/pull/492)
- `near` now takes constants like `top-center`, particularly useful for diagram titles. See [docs](https://d2lang.com/tour/text#near-a-constant) for more. [#525](https://github.com/terrastruct/d2/pull/525)

#### Improvements 🧹

- Improved label placements for shapes with images and icons to avoid overlapping labels. [#474](https://github.com/terrastruct/d2/pull/474)
- Themes are applied to `sql_table` and `class` shapes. [#521](https://github.com/terrastruct/d2/pull/521)
- `class` shapes use monospaced font. [#521](https://github.com/terrastruct/d2/pull/521)
- Sequence diagram edge group labels have more reasonable padding. [#512](https://github.com/terrastruct/d2/pull/512)
- ELK layout engine preserves order of nodes. [#282](https://github.com/terrastruct/d2/issues/282)
- Markdown headings set font-family explicitly, so that external stylesheets with more specific targeting don't override it. [#525](https://github.com/terrastruct/d2/pull/525)

#### Bugfixes ⛑️

- `d2 fmt` only rewrites if it has changes, instead of always rewriting. [#470](https://github.com/terrastruct/d2/pull/470)
- Text no longer overflows in `sql_table` shapes. [#458](https://github.com/terrastruct/d2/pull/458)
- ELK connection labels are now given the appropriate dimensions. [#483](https://github.com/terrastruct/d2/pull/483)
- Dagre connection lengths make room for longer labels. [#484](https://github.com/terrastruct/d2/pull/484)
- Icons with query parameters are escaped to valid SVG XML. [#438](https://github.com/terrastruct/d2/issues/438)
- Connections at the boundaries no longer get part of its stroke clipped. [#493](https://github.com/terrastruct/d2/pull/493)
- Fixes edge case where `style` being defined in same scope as `sql_table` causes compiler to skip compiling `sql_table`. [#506](https://github.com/terrastruct/d2/issues/506)
- Fixes panic passing a non-string value to `constraint`. [#248](https://github.com/terrastruct/d2/issues/248)
