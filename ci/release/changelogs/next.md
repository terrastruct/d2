#### Features 🚀

- `double-border` keyword implemented. [#565](https://github.com/terrastruct/d2/pull/565)
- The [Dockerfile](./docs/INSTALL.md#docker) now supports rendering PNGs [#594](https://github.com/terrastruct/d2/issues/594)

  - There was a minor breaking change as part of this where the default working directory of the Dockerfile is now `/home/debian/src` instead of `/root/src` to allow UID remapping with [`fixuid`](https://github.com/boxboat/fixuid).

- `d2 fmt` accepts multiple files to be formatted [#718](https://github.com/terrastruct/d2/issues/718)

#### Improvements 🧹

- Reduces default padding of shapes. [#702](https://github.com/terrastruct/d2/pull/702)
- Ensures labels fit inside shapes with shape-specific inner bounding boxes. [#702](https://github.com/terrastruct/d2/pull/702)
- Improves package shape dimensions with short height. [#702](https://github.com/terrastruct/d2/pull/702)
- Keeps person shape from becoming too distorted. [#702](https://github.com/terrastruct/d2/pull/702)
- Ensures shapes with icons have enough padding for their labels. [#702](https://github.com/terrastruct/d2/pull/702)

#### Bugfixes ⛑️

- Fixes groups overlapping in sequence diagrams when they end in a self loop. [#728](https://github.com/terrastruct/d2/pull/728)
- Fixed dimensions of unlabeled squares or circles with only a set width or height. [#702](https://github.com/terrastruct/d2/pull/702)
- Fixed scaling of actor shapes in sequence diagrams. [#702](https://github.com/terrastruct/d2/pull/702)
- Code snippets use bold and italic font styles as determined by highlighter [#710](https://github.com/terrastruct/d2/issues/710), [#741](https://github.com/terrastruct/d2/issues/741)
