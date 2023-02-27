#### Features 🚀

- PDF exports are now supported. [#120](https://github.com/terrastruct/d2/issues/120)
- Diagram background can be styled. [#910](https://github.com/terrastruct/d2/issues/910)
- 3D Hexagons are now supported. [#869](https://github.com/terrastruct/d2/issues/869)
- The arm64 docker container now supports rendering diagrams to pngs. [#917](https://github.com/terrastruct/d2/pull/917)

#### Improvements 🧹

- `near` key set to sequence diagram children get an appropriate error message. [#899](https://github.com/terrastruct/d2/issues/899)
- `class` and `sql_table` shape respect `font-color` styling as header font color. [#899](https://github.com/terrastruct/d2/issues/899)
- SVG fits to screen by default in both watch mode and as a standalone SVG (this time with just CSS, no JS). [#725](https://github.com/terrastruct/d2/issues/725)
- Only chromium is installed when rendering png diagrams instead of also installing webkit and firefox. [#835](https://github.com/terrastruct/d2/issues/835)

#### Bugfixes ⛑️

- Error reported when no actors are declared in sequence diagram. [#886](https://github.com/terrastruct/d2/pull/886)
- Fixed img bundling on image shapes. [#889](https://github.com/terrastruct/d2/issues/889)
- `class` shape as sequence diagram actors had wrong colors. [#899](https://github.com/terrastruct/d2/issues/899)
