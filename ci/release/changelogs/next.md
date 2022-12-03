#### Features 🚀

- Sequence diagrams are now supported. See [docs](https://d2lang.com/tour/sequence-diagrams) for more.
  [#99](https://github.com/terrastruct/d2/issues/99)
- Formatting of d2 scripts is supported on the CLI with the `fmt` subcommand.
  [#292](https://github.com/terrastruct/d2/pull/292)
- Latex is now supported. See [docs](https://d2lang.com/tour/text) for more.
  [#229](https://github.com/terrastruct/d2/pull/229)
- `direction` keyword is now supported to specify `up`, `down`, `right`, `left` layouts. See
  [docs](https://d2lang.com/tour/layouts) for more.
  [#251](https://github.com/terrastruct/d2/pull/251)
- Self-referencing connections are now valid. E.g. `x -> x`.
  [#273](https://github.com/terrastruct/d2/pull/273)
- Arrowhead labels are now supported. [#182](https://github.com/terrastruct/d2/pull/182)
- `stroke-dash` on shapes is now supported. [#188](https://github.com/terrastruct/d2/issues/188)
- `font-color` is now supported on shapes and connections. [#215](https://github.com/terrastruct/d2/pull/215)
- `font-size` is now supported on shapes and connections. [#250](https://github.com/terrastruct/d2/pull/250)
- Querying shapes and connections by ID is now supported in renders. [#218](https://github.com/terrastruct/d2/pull/218)
- [install.sh](./install.sh) now accepts `-d` as an alias for `--dry-run`.
  [#266](https://github.com/terrastruct/d2/pull/266)
- `-b/--bundle` flag to `d2` now works and bundles all image assets directly as base64
  data urls. [#278](https://github.com/terrastruct/d2/pull/278)

#### Improvements 🧹

- Local images can now be included, e.g. `icon: ./my_img.png`.
  [#146](https://github.com/terrastruct/d2/issues/146)
- ELK layout engine now defaults to top-down to be consistent with dagre.
  [#251](https://github.com/terrastruct/d2/pull/251)
- [install.sh](./install.sh) prints the dry run message more visibly.
  [#266](https://github.com/terrastruct/d2/pull/266)
- `d2` now lives in the root folder of the repository instead of as a subcommand.
  So you can run `go install oss.terrastruct.com/d2@latest` to install from source
  now.
  [#290](https://github.com/terrastruct/d2/pull/290)
- `BROWSER=0` now works to disable opening a browser on `--watch`.
  [#311](https://github.com/terrastruct/d2/pull/311)

#### Bugfixes ⛑️

- 3D style was missing border and other styles for its top and right faces.
  [#187](https://github.com/terrastruct/d2/pull/187)
- System dark mode was incorrectly applying to markdown in renders.
  [#159](https://github.com/terrastruct/d2/issues/159)
- Fixes markdown newlines created with a trailing double space or backslash.
  [#214](https://github.com/terrastruct/d2/pull/214)
- Fixes images not loading in PNG exports.
  [#224](https://github.com/terrastruct/d2/pull/224)
- Avoid logging benign file watching errors.
  [#293](https://github.com/terrastruct/d2/pull/293)
- `$BROWSER` now works to open a custom browser correctly.
  For example, to open Firefox on macOS: `BROWSER='open -aFirefox'`
  [#311](https://github.com/terrastruct/d2/pull/311)
- Fixes numbered IDs being wrongly positioned in `dagre`
  [#321](https://github.com/terrastruct/d2/issues/321). Thank you @pleshevskiy for the
  report.
