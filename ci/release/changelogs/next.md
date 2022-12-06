When we launched D2 to open-source 2 weeks ago, we left the sprint after it a completely
blank slate. Because, while we do have long-term goals, we wanted to make the first
post-launch update focused 100% on addressing the biggest pain points that came up. Thank
you so much to everyone who asked for or complained about something. Every item in this
release was something that was posted on the D2 Discord, a GitHub issue/discussion, a
comment on social media, or an email.

On top of the listed changes to core D2, we have been building out integrations, starting
with [Obsidian](https://github.com/terrastruct/d2-obsidian). Work has also begun on the
API and Playground.

If you want a fast way to check out what a change looks like, we put screenshots in the
PRs when it's a visual change.

#### Features 🚀

- Sequence diagrams are now supported, experimentally. See
  [docs](https://d2lang.com/tour/sequence-diagrams).
  [#99](https://github.com/terrastruct/d2/issues/99)
- Formatting of d2 scripts is supported on the CLI with the `fmt` subcommand. See `man d2`
  or `d2 --help`. [#292](https://github.com/terrastruct/d2/pull/292)
- Latex is now supported. See [docs](https://d2lang.com/tour/text) for how to use.
  [#229](https://github.com/terrastruct/d2/pull/229)
- `direction` keyword is now supported to specify `up`, `down`, `right`, `left` layouts.
  See [docs](https://d2lang.com/tour/layouts) for more.
  [#251](https://github.com/terrastruct/d2/pull/251)
- Self-referencing connections are now valid. E.g. `x -> x`. Render will vary based on
  layout engine. [#273](https://github.com/terrastruct/d2/pull/273)
- Arrowhead labels are now supported. [#182](https://github.com/terrastruct/d2/pull/182)
- Support for `stroke-dash` on shapes.
  [#188](https://github.com/terrastruct/d2/issues/188)
- Support for `font-color` on shapes and connections.
  [#215](https://github.com/terrastruct/d2/pull/215)
- Support for `font-size` on shapes and connections.
  [#250](https://github.com/terrastruct/d2/pull/250)
- HTML IDs are now added in the SVG output. You can use this to query shapes and
  connections by ID post-render. [#218](https://github.com/terrastruct/d2/pull/218)
- `-b/--bundle` flag to `d2` bundles all image assets directly as base64 data urls.
  [#278](https://github.com/terrastruct/d2/pull/278)
- [install.sh](./install.sh) now accepts `-d` as an alias for `--dry-run`.
  [#266](https://github.com/terrastruct/d2/pull/266)

#### Improvements 🧹

- Local images can now be used for values to the `icon` keyword, e.g. `icon:
  ./my_img.png`. [#146](https://github.com/terrastruct/d2/issues/146)
- Connection labels no longer overlap other connections.
  [#332](https://github.com/terrastruct/d2/pull/332)
- ELK layout engine now defaults to top-down to be consistent with dagre.
  [#251](https://github.com/terrastruct/d2/pull/251)
- Container default font styling is no longer bold. Everything used to look too bold.
  [#358](https://github.com/terrastruct/d2/pull/358)
- `BROWSER=0` will disable opening a browser on `--watch`.
  [#311](https://github.com/terrastruct/d2/pull/311)
- [install.sh](./install.sh) prints the dry run message more visibly.
  [#266](https://github.com/terrastruct/d2/pull/266)
- `d2` now lives in the root folder of the repository instead of as a subcommand. So you
  can now run `go install oss.terrastruct.com/d2@latest` to install from source.
  [#290](https://github.com/terrastruct/d2/pull/290)

#### Bugfixes ⛑️

- 3D style was missing border and other styles for its top and right faces.
  [#187](https://github.com/terrastruct/d2/pull/187)
- System dark mode was incorrectly applying to Markdown in renders.
  [#159](https://github.com/terrastruct/d2/issues/159)
- Fixes Markdown newlines created with a trailing double space or backslash.
  [#214](https://github.com/terrastruct/d2/pull/214)
- Fixes images not loading in PNG exports.
  [#224](https://github.com/terrastruct/d2/pull/224)
- Fixes label and icon overlapping each other in dagre and ELK layouts.
  [#343](https://github.com/terrastruct/d2/pull/343)
- No longer log benign file-watching errors.
  [#293](https://github.com/terrastruct/d2/pull/293)
- `$BROWSER` now works to open a custom browser correctly. For example, to open Firefox on
  macOS: `BROWSER='open -a Firefox'` [#311](https://github.com/terrastruct/d2/pull/311)
- Fixes numbered IDs being wrongly positioned in `dagre`
  [#321](https://github.com/terrastruct/d2/issues/321).
