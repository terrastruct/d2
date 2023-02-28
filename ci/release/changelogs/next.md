`style` keywords now apply at the root level, letting you style the diagram background and frame like so:

![chilly](https://user-images.githubusercontent.com/3120367/221755385-22e9078e-a8db-418d-81e4-282c8b33f1d7.svg)

(also showcases a little 3d hexagon, newly supported thanks to our newest contributor @JettChenT !)

PDF is also now supported as an export format:

[demo.pdf](https://github.com/terrastruct/d2/files/10846644/demo.pdf)

#### Features 🚀

- PDF exports. See [docs](https://d2lang.com/tour/exports#pdf). [#120](https://github.com/terrastruct/d2/issues/120)
- Diagram background and frame can be added and styled. See [docs](https://d2lang.com/tour/style#root). [#910](https://github.com/terrastruct/d2/pull/910)
- `3d` works on `hexagon` shapes. [#869](https://github.com/terrastruct/d2/issues/869)
- The arm64 docker container supports rendering diagrams to PNGs. [#917](https://github.com/terrastruct/d2/pull/917)

#### Improvements 🧹

- `near` key set to sequence diagram children get an appropriate error message. [#899](https://github.com/terrastruct/d2/pull/899)
- `class` and `sql_table` shape respect `font-color` styling as header font color. [#899](https://github.com/terrastruct/d2/pull/899)
- SVG fits to screen by default in both watch mode and as a standalone SVG (this time with just CSS, no JS). [#725](https://github.com/terrastruct/d2/pull/725)
- Only chromium is installed when rendering png diagrams instead of also installing webkit and firefox. [#835](https://github.com/terrastruct/d2/issues/835)
- Multiboard output is now self-contained and less confusing. See [#923](https://github.com/terrastruct/d2/pull/923)

#### Bugfixes ⛑️

- Error reported when no actors are declared in sequence diagram. [#886](https://github.com/terrastruct/d2/pull/886)
- Fixes img bundling on image shapes. [#889](https://github.com/terrastruct/d2/issues/889)
- `class` shape as sequence diagram actors had wrong colors. [#899](https://github.com/terrastruct/d2/pull/899)
- Fixes regression in last release where some hex codes were not working. [#922](https://github.com/terrastruct/d2/pull/922)
