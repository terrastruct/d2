Here's what a D2 diagram looks like in 0.2 vs 0.1:

Much more legible right? This upgrade trims a lot of the excess whitespace present before and makes diagrams more compact. We've also combed through each shape to improve their label positions, paddings, and aspect ratios at different sizes.

There's also been a major compiler rewrite. It's fixed many minor compiler bugs, but most importantly, it implements multi-board diagrams. Stay tuned for more as we write docs and make this accessible in the next release!

#### Features 🚀

- `double-border` keyword implemented. [#565](https://github.com/terrastruct/d2/pull/565)
- The [Dockerfile](./docs/INSTALL.md#docker) now supports rendering PNGs [#594](https://github.com/terrastruct/d2/issues/594)
  - There was a minor breaking change as part of this where the default working directory of the Dockerfile is now `/home/debian/src` instead of `/root/src` to allow UID remapping with [`fixuid`](https://github.com/boxboat/fixuid).
- `d2 fmt` accepts multiple files to be formatted [#718](https://github.com/terrastruct/d2/issues/718)
- `font-size` works for `sql_table` and `class` shapes [#769](https://github.com/terrastruct/d2/issues/769)
- You can now use the reserved keywords `layers`/`scenarios`/`steps` to define diagrams with multiple levels of abstractions. Coming soon. [#714](https://github.com/terrastruct/d2/pull/714)

#### Improvements 🧹

- Reduces default padding of shapes. [#702](https://github.com/terrastruct/d2/pull/702)
- Ensures labels fit inside shapes with shape-specific inner bounding boxes. [#702](https://github.com/terrastruct/d2/pull/702)
- dagre container labels changed positions to outside the shape. Many previously obscured container labels are now legible. [#788](https://github.com/terrastruct/d2/pull/788)
- Code snippets use bold and italic font styles as determined by highlighter [#710](https://github.com/terrastruct/d2/issues/710), [#741](https://github.com/terrastruct/d2/issues/741)
- Improves package shape dimensions with short height. [#702](https://github.com/terrastruct/d2/pull/702)
- Sequence diagrams are rendered more compacted, both vertically and horizontally. [#796](https://github.com/terrastruct/d2/pull/796)
- Keeps person shape from becoming too distorted. [#702](https://github.com/terrastruct/d2/pull/702)
- Keeps oval shape from becoming too thin. [#807](https://github.com/terrastruct/d2/pull/807)
- Ensures shapes with icons have enough padding for their labels. [#702](https://github.com/terrastruct/d2/pull/702)
- `--force-appendix` flag adds an appendix to SVG outputs with tooltips or links. [#761](https://github.com/terrastruct/d2/pull/761)
- `d2 themes` subcommand to list themes. [#760](https://github.com/terrastruct/d2/pull/760)
- `sql_table` header left-aligned with column [#769](https://github.com/terrastruct/d2/pull/769)
- Sequence diagram edge group labels are clearer [#782](https://github.com/terrastruct/d2/pull/782)

#### Bugfixes ⛑️

- Fixes groups overlapping in sequence diagrams when they end in a self loop. [#728](https://github.com/terrastruct/d2/pull/728)
- Fixes dimensions of unlabeled squares or circles with only a set width or height. [#702](https://github.com/terrastruct/d2/pull/702)
- Fixes scaling of actor shapes in sequence diagrams. [#702](https://github.com/terrastruct/d2/pull/702)
- Sequence diagram note ordering was sometimes wrong. [#796](https://github.com/terrastruct/d2/pull/796)
- Images can now be set to sizes smaller than 128x128. [#702](https://github.com/terrastruct/d2/pull/702)
- Tooltips with ampersand would result in invalid SVGs. [#798](https://github.com/terrastruct/d2/pull/798)
- Fixes class height when there are no rows. [#756](https://github.com/terrastruct/d2/pull/756)
- Border radius was not firefox-compatible. [#799](https://github.com/terrastruct/d2/pull/799)

#### Breaking changes

- You can no longer use keywords intended for use under `style` outside and vice versa. e.g. `obj.style.shape` and `obj.double-border` are now illegal. The correct usages have always been `obj.shape` and `obj.style.double-border`; it just wasn't enforced until now.
