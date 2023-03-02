#### Features 🚀

- `border-radius` is now supported on connections (ELK and TALA only, since Dagre uses curves). [#913](https://github.com/terrastruct/d2/pull/913)

#### Improvements 🧹

- PDF exports now support external links on shapes [#891](https://github.com/terrastruct/d2/issues/891)
- SVGs are fit to top left by default to avoid issues with zooming. [#954](https://github.com/terrastruct/d2/pull/954)
- Person shapes now have labels below them and don't need to expand as much. [#960](https://github.com/terrastruct/d2/pull/960)

#### Bugfixes ⛑️

- Fixes a regression where PNG backgrounds could be cut off in the appendix. [#941](https://github.com/terrastruct/d2/pull/941)
- Fixes zooming not working in watch mode. [#944](https://github.com/terrastruct/d2/pull/944)
- [API] Fixes `DeleteIDDeltas` giving duplicate deltas in rare cases. [#957](https://github.com/terrastruct/d2/pull/957)
