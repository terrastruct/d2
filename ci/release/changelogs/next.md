#### Features 🚀

- Latex is now supported. See [docs](https://d2lang.com/tour/text) for more.
  [#229](https://github.com/terrastruct/d2/pull/229)
- `direction` keyword is now supported to specify `up`, `down`, `right`, `left` layouts. See
  [docs](https://d2lang.com/tour/layouts) for more.
  [#251](https://github.com/terrastruct/d2/pull/251)
- Arrowhead labels are now supported. [#182](https://github.com/terrastruct/d2/pull/182)
- `stroke-dash` on shapes is now supported. [#188](https://github.com/terrastruct/d2/issues/188)
- `font-color` is now supported on shapes and connections. [#215](https://github.com/terrastruct/d2/pull/215)
- Querying shapes and connections by ID is now supported in renders. [#218](https://github.com/terrastruct/d2/pull/218)
- [install.sh](./install.sh) now accepts `-d` as an alias for `--dry-run`.
  [#266](https://github.com/terrastruct/d2/pull/266)

#### Improvements 🔧

- ELK layout engine now defaults to top-down to be consistent with dagre.
  [#251](https://github.com/terrastruct/d2/pull/251)
- [install.sh](./install.sh) prints the dry run message more visibly.
  [#266](https://github.com/terrastruct/d2/pull/266)

#### Bugfixes 🔴

- 3D style was missing border and other styles for its top and right faces.
  [#187](https://github.com/terrastruct/d2/pull/187)
- System dark mode was incorrectly applying to markdown in renders.
  [#159](https://github.com/terrastruct/d2/issues/159)
- Fixes markdown newlines created with a trailing double space or backslash.
  [#214](https://github.com/terrastruct/d2/pull/214)
