D2 0.6.2 makes grid diagrams significantly more powerful. Namely, connections can now be made from grid elements to other grid elements. This enables diagrams like the following.

![vector-grid](https://raw.githubusercontent.com/terrastruct/d2/af841720b48864f5240ae4fb9cb0cb87bcfef038/docs/examples/vector-grid/vector-grid.svg)

> [Source code](https://github.com/terrastruct/d2/blob/master/docs/examples/vector-grid/vector-grid.d2)

> [Playground link](https://play.d2lang.com/?script=xFVNb9swDL37VxDqTgVs-CMfjTAMaJt5p12GYXfFVhOjthTIzNKuyH8fLEu27DpYGxTYoaj5KJHUI_mSlayueU3hxQPg1R6f20-Akm14SYEQbdX4XHLrAngoypICKibqPVNcoMFrVPKR-8cixx2FUKMnr_1D_oQ2wgeEy8pDjVxdFjFn9Y7CbABSIFf3i3WyTskg0VYVuQ3cfPuZLA-VqClEbyXquCuQv-FNSh4HmZQ8vifNewg8ed5vpkzjS_6A_ohQm0_4dfGH5_CLZyhVm11PDbUt0FB4HeiagrYUcrVOv6bpLRk6LdObkmWP0-_UwJbtbbm6jkBwpijIA9ZFzn2Ue7-p2dMnSBgsYmI_o3n7qYc5Ckyp2tK4sFAzj_3J-PVJEgarqIs7uyGekwM--yDA_9Kf0g0struzTFYXMrmM03ma_lcmn3GCyerjmKxGTJ48z3lO7HlNnd12mPhKHq1ojVO2oBn7KArDJuEPJnJZwXeGqniCb1xwxVB2bTKnk7Bla8ebXlKIlmG_ccGDFKjbSCEe4KZXt8u7u_vU2dGJRrX4RqqcK1-xvDg0LQvtwzcSUVZnXqs31VUk422gltSIwqcXd6FPGo6n4WQKPjWRcol-JgWyQvSj_KqzU4J4bvNyiRTWEmGvZH7IsFOv8RgBZIXKGn0jpDsEUO_YnlPj61GH_57jMf9DCXTaHfeI7XgHncz_qcUxXcq4sHvbMVfpAbuQvPYyhZ9MbTlCXlRc1IUUxtHRcW6ZnYIGkjBzIUcUAK67PQl7qJt-BxtOehqli3RFRt7X096ufER6I4kdY-UaUWKUQVvLmeuaO8Y8cYw4ci-tlo5r5l6K3BrCuXtp4bqWboT5yjFuEjLs9IT2dD8C_17SwY-Fs6UTeDKJT05bo6R9BY2gGO0Mzuhfc6HXnGAczwDe3wAAAP__&sketch=1&)
>
> Credit: this diagram is based off of a manually-drawn one from a [blog post](https://github.com/terrastruct/d2/issues/1299)

In addition, another significant feature is that using the ELK layout engine will now route SQL diagrams to their exact columns.

<img width="576" alt="Screen Shot 2023-12-06 at 3 26 39 PM" src="https://github.com/terrastruct/d2/assets/3120367/5b153748-22ed-4bad-bd05-70dfd6e7d3e0">

> [Playground link](https://play.d2lang.com/?script=pI9BroMwDAX3PsW7wOcAWfyrIJdYYDUJaWxaIcTdKyrYV-rS0jzNeJjNlywWsBFgE1cJsEfqnW9JCNAYoMWxDXMxb6zFA2rTzG3t77Lux0xT0iJmH5SAfLDSrjOxeb_UyC4xwDWLOedKO9FJ_ubP81OPmTctIwEv0XHyL-zX992Zgb__q906jfQOAAD__w%3D%3D&sketch=1&layout=elk&)

**Note** that all previous playground links will be broken given the encoding change. The encoding before 0.6.2 used the keyword set as compression dictionary, but it no longer does, so this will be the last time playground links break.

#### Features 🚀

- ELK routes `sql_table` edges to the exact columns (ty @landmaj) [#1681](https://github.com/terrastruct/d2/pull/1681)
- Unfilled triangle arrowhead is available. [#1711](https://github.com/terrastruct/d2/pull/1711)
- Grid containers customize label positions. [#1715](https://github.com/terrastruct/d2/pull/1715)
- A single board from a multi-board diagram can be rendered with `--target` flag. [#1725](https://github.com/terrastruct/d2/pull/1725)

#### Improvements 🧹

- Grid cells can contain nested edges [#1629](https://github.com/terrastruct/d2/pull/1629)
- Edges can go across constant `near`s, sequence diagrams, and grids, including nested ones. [#1631](https://github.com/terrastruct/d2/pull/1631)
- All vars defined in a scope are accessible everywhere in that scope, i.e., an object can use a var defined after itself. [#1695](https://github.com/terrastruct/d2/pull/1695)
- Encoding API switches to standard zlib encoding so that decoding doesn't depend on source. [#1709](https://github.com/terrastruct/d2/pull/1709)
- `currentcolor` is accepted as a color option to inherit parent colors. (ty @hboomsma) [#1700](https://github.com/terrastruct/d2/pull/1700)
- Grid containers can be sized with `width`/`height` even when using a layout plugin without that feature. [#1731](https://github.com/terrastruct/d2/pull/1731)
- Watch mode watches for changes in both the input file and imported files [#1720](https://github.com/terrastruct/d2/pull/1720)

#### Bugfixes ⛑️

- Fixes a bug calculating grid height with only grid-rows and different horizontal-gap and vertical-gap values. [#1646](https://github.com/terrastruct/d2/pull/1646)
- Grid layout accounts for each cell's outside labels and icons [#1624](https://github.com/terrastruct/d2/pull/1624)
- Grid layout accounts for labels wider or taller than the shape and fixes default label positions for image grid cells. [#1670](https://github.com/terrastruct/d2/pull/1670)
- Fixes a panic with a spread substitution in a glob map [#1643](https://github.com/terrastruct/d2/pull/1643)
- Fixes use of `null` in `sql_table` constraints (ty @landmaj) [#1660](https://github.com/terrastruct/d2/pull/1660)
- Fixes ELK growing shapes with width/height set [#1679](https://github.com/terrastruct/d2/pull/1679)
- Adds a compiler error when accidentally using an arrowhead on a shape [#1686](https://github.com/terrastruct/d2/pull/1686)
- Correctly reports errors from invalid values set by globs. [#1691](https://github.com/terrastruct/d2/pull/1691)
- Fixes panic when spread substitution referenced a nonexistant var. [#1695](https://github.com/terrastruct/d2/pull/1695)
- Fixes incorrect appendix icon numbering. [#1704](https://github.com/terrastruct/d2/pull/1704)
- Fixes crash when using `--watch` and navigating to an invalid board path [#1693](https://github.com/terrastruct/d2/pull/1693)
- Fixes edge case where nested edge globs were creating excess shapes [#1713](https://github.com/terrastruct/d2/pull/1713)
- Fixes a panic with a connection to a grid cell that is a container in TALA [#1729](https://github.com/terrastruct/d2/pull/1729)
- Fixes incorrect grid cell positioning when the grid has a shape set and fixes content sometimes escaping circle shapes. [#1734](https://github.com/terrastruct/d2/pull/1734)
- Fixes content sometimes escaping cloud shapes. [#1736](https://github.com/terrastruct/d2/pull/1736)
- Fixes panic using a glob filter (e.g. `&a`) outside globs. [#1748](https://github.com/terrastruct/d2/pull/1748)
- Fixes glob keys with import values (e.g. `user*: @lib/user`). [#1755](https://github.com/terrastruct/d2/pull/1755)
