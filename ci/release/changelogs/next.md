#### Features 🚀

- Render: SVG files render in non-browser contexts (e.g. Inkscape, LaTeX) [#2147](https://github.com/terrastruct/d2/pull/2147)

#### Improvements 🧹

- Lib: removes a dependency on external slog that was causing troubles with installation [#2137](https://github.com/terrastruct/d2/pull/2137)
- CLI: attempts writing to path atomically, falling back to non-atomic if failed [#2141](https://github.com/terrastruct/d2/pull/2141)
- Export: pptx has "created at" metadata removed, so successive runs yield the same result [#2169](https://github.com/terrastruct/d2/pull/2160)
- Formatter: empty board keywords (e.g. `layers`) are removed [#2178](https://github.com/terrastruct/d2/pull/2178)
- Render: circle containers are tighter fitting [#2183](https://github.com/terrastruct/d2/pull/2183)
- Render: a tooltip or link by itself will not expand width of shape [#2183](https://github.com/terrastruct/d2/pull/2183)

#### Bugfixes ⛑️

- Render: fixes edge case of a 3d shape with outside label being cut off [#2132](https://github.com/terrastruct/d2/pull/2132)
- Composition: labels for boards set with shorthand `x: y` was not applied [#2182](https://github.com/terrastruct/d2/pull/2182)
