![animated connection example](https://user-images.githubusercontent.com/3120367/213055161-e6f1918b-150c-4beb-b61c-3ea05cc29f00.svg)


#### Features 🚀

- `animated` keyword implemented for connections. [#652](https://github.com/terrastruct/d2/pull/652)

#### Improvements 🧹

- ELK layouts tuned to have better defaults. [#627](https://github.com/terrastruct/d2/pull/627)
- Code snippets of unrecognized languages will render (just without syntax highlighting). [#650](https://github.com/terrastruct/d2/pull/650)
- Adds sketched versions of arrowheads. [#656](https://github.com/terrastruct/d2/pull/656)

#### Bugfixes ⛑️

- Fixes arrowheads sometimes appearing broken in dagre layouts. [#649](https://github.com/terrastruct/d2/pull/649)
- Fixes attributes being ignored for `sql_table` to `sql_table` connections. [#658](https://github.com/terrastruct/d2/pull/658)
- Fixes tooltip/link attributes being ignored for `sql_table` and `class`. [#658](https://github.com/terrastruct/d2/pull/658)
- Fixes arrowheads sometimes appearing broken with sketch on. [#656](https://github.com/terrastruct/d2/pull/656)
- Bounding box was not accounting for dimensions added by `multiple` and `3d` keywords, which made them look cut off with 0 padding. [#684](https://github.com/terrastruct/d2/pull/684), [#685](https://github.com/terrastruct/d2/pull/685)
- Fixes code snippets not being tall enough with leading newlines. [#664](https://github.com/terrastruct/d2/pull/664)
- Opacity was not being applied to labels of shapes (and other edge cases). [#677](https://github.com/terrastruct/d2/pull/677)
- Icon URLs that needed escaping (e.g. with ampersands) are handled correctly by CLI. [#666](https://github.com/terrastruct/d2/pull/666)
- Fixes markdown shapes being slightly too short for their text in some cases. [#665](https://github.com/terrastruct/d2/pull/665)
- Fixes self-connections inside layouts when using ELK. [#676](https://github.com/terrastruct/d2/pull/676)
- Fixes panic when the only diagram object has `near` set to a constant. [#687](https://github.com/terrastruct/d2/pull/687)
