Multi-board D2 compositions now get 2 more useful formats to export to: Powerpoint and GIFs.

## Powerpoint example

Make sure you click present, and click around the links and navigations!

- Download: [wcc.pptx](https://github.com/terrastruct/d2/files/11256733/wcc.pptx)
- Google Slides: [https://docs.google.com/presentation/d/18rRh4izu3k_43On8PXtVYdqRxmoQJd4y/view](https://docs.google.com/presentation/d/18rRh4izu3k_43On8PXtVYdqRxmoQJd4y/view)
- Source code: [https://github.com/terrastruct/d2/blob/master/docs/examples/wcc/wcc.d2](https://github.com/terrastruct/d2/blob/master/docs/examples/wcc/wcc.d2)

## GIF example

This is the same diagram as one we shared when we announced animated SVGs, but in GIF form.

![animated](https://user-images.githubusercontent.com/3120367/232637553-dd35e076-dfb4-4910-958d-d57ec382f792.gif)

#### Features 🚀

- Export diagrams to `.pptx` (PowerPoint)[#1139](https://github.com/terrastruct/d2/pull/1139)
- Customize gap size in grid diagrams with `grid-gap`, `vertical-gap`, or `horizontal-gap` [#1178](https://github.com/terrastruct/d2/issues/1178)
- New dark theme "Dark Terrastruct Flagship" with the theme ID of `201` [#1150](https://github.com/terrastruct/d2/issues/1150)

#### Improvements 🧹

- `font-size` works with Markdown text [#1191](https://github.com/terrastruct/d2/issues/1191)
- SVG outputs for internal links use relative paths instead of absolute [#1197](https://github.com/terrastruct/d2/pull/1197)
- Improves arrowhead label positioning [#1207](https://github.com/terrastruct/d2/pull/1207)

#### Bugfixes ⛑️

- Fixes grid layouts not applying on objects with a constant near [#1173](https://github.com/terrastruct/d2/issues/1173)
- Fixes Markdown header rendering in Firefox and Safari [#1177](https://github.com/terrastruct/d2/issues/1177)
- Fixes a grid layout panic when there are fewer objects than rows/columns [#1189](https://github.com/terrastruct/d2/issues/1189)
- Fixes an issue where PNG exports that were too large were crashing [#1093](https://github.com/terrastruct/d2/issues/1093)
