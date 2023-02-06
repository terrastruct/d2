#### Features 🚀

- `double-border` keyword implemented. [#565](https://github.com/terrastruct/d2/pull/565)
- The [Dockerfile](./docs/INSTALL.md#docker) now supports rendering PNGs [#594](https://github.com/terrastruct/d2/issues/594)

  - There was a minor breaking change as part of this where the default working directory of the Dockerfile is now `/home/debian/src` instead of `/root/src` to allow UID remapping with [`fixuid`](https://github.com/boxboat/fixuid).

- `d2 fmt` accepts multiple files to be formatted [#718](https://github.com/terrastruct/d2/issues/718)

- `font-size` works for `sql_table` and `class` shapes [#769](https://github.com/terrastruct/d2/issues/769)

- You can now use the reserved keywords `layers`/`scenarios`/`steps` to define diagrams
  with multiple levels of abstractions. [#714](https://github.com/terrastruct/d2/pull/714)
  Docs to come soon
  - [#416](https://github.com/terrastruct/d2/issues/416) was also fixed so you can no
    longer use keywords intended for use under `style` outside and vice versa. e.g.
    `obj.style.shape` and `obj.double-border` are now illegal. The correct uses are
    `obj.shape` and `obj.style.double-border`.
  - Many other minor compiler bugs were fixed.

#### Improvements 🧹

- Code snippets use bold and italic font styles as determined by highlighter [#710](https://github.com/terrastruct/d2/issues/710), [#741](https://github.com/terrastruct/d2/issues/741)
- Reduces default padding of shapes. [#702](https://github.com/terrastruct/d2/pull/702)
- Ensures labels fit inside shapes with shape-specific inner bounding boxes. [#702](https://github.com/terrastruct/d2/pull/702)
- Improves package shape dimensions with short height. [#702](https://github.com/terrastruct/d2/pull/702)
- Keeps person shape from becoming too distorted. [#702](https://github.com/terrastruct/d2/pull/702)
- Ensures shapes with icons have enough padding for their labels. [#702](https://github.com/terrastruct/d2/pull/702)
- `--force-appendix` flag adds an appendix to SVG outputs with tooltips or links. [#761](https://github.com/terrastruct/d2/pull/761)
- `d2 themes` subcommand to list themes. [#760](https://github.com/terrastruct/d2/pull/760)

#### Bugfixes ⛑️

- Fixes groups overlapping in sequence diagrams when they end in a self loop. [#728](https://github.com/terrastruct/d2/pull/728)
- Fixes dimensions of unlabeled squares or circles with only a set width or height. [#702](https://github.com/terrastruct/d2/pull/702)
- Fixes scaling of actor shapes in sequence diagrams. [#702](https://github.com/terrastruct/d2/pull/702)
- Images can now be set to sizes smaller than 128x128. [#702](https://github.com/terrastruct/d2/pull/702)
- Fixes class height when there are no rows. [#756](https://github.com/terrastruct/d2/pull/756)
