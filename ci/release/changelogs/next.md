#### Features 🚀

- `double-border` keyword implemented. [#565](https://github.com/terrastruct/d2/pull/565)
- The [Dockerfile](./docs/INSTALL.md#docker) now supports rendering PNGs [#594](https://github.com/terrastruct/d2/issues/594)
  - There was a minor breaking change as part of this where the default working directory of the Dockerfile is now `/home/debian/src` instead of `/root/src` to allow UID remapping with [`fixuid`](https://github.com/boxboat/fixuid).

- `d2 fmt` accepts multiple files to be formatted [#718](https://github.com/terrastruct/d2/issues/718)

- You can now use the reserved keywords `layers`/`scenarios`/`steps` to define diagrams
  with multiple levels of abstractions. [#714](https://github.com/terrastruct/d2/pull/714)
  Docs to come soon
  - [#416](https://github.com/terrastruct/d2/issues/416) was also fixed so you can no
    longer use keywords intended for use under `style` outside and vice versa. e.g.
    `obj.style.shape` and `obj.double-border` are now illegal. The correct uses are
    `obj.shape` and `obj.style.double-border`.

#### Improvements 🧹

#### Bugfixes ⛑️

- Fixes groups overlapping in sequence diagrams when they end in a self loop. [#728](https://github.com/terrastruct/d2/pull/728)
