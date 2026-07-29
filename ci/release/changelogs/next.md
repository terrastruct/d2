#### Features 🚀

- exports: gif exports work with `animate: true` keyword [#2663](https://github.com/terrastruct/d2/pull/2663)

#### Improvements 🧹

- d2ascii:
  - sql_table and uml class shapes are supported [#2623](https://github.com/terrastruct/d2/pull/2623)
  - newlines are handled [#2626](https://github.com/terrastruct/d2/pull/2626)
  - empty left columns are cropped [#2626](https://github.com/terrastruct/d2/pull/2626)
- exports:
  - Chromium download through CLI for PNG exports is prompted [#2655](https://github.com/terrastruct/d2/pull/2655)
  - `animate-interval` is no longer required, defaults to 1000ms for gifs [#2663](https://github.com/terrastruct/d2/pull/2663)
- renders:
  - remote images are fetched more reliably [#2659](https://github.com/terrastruct/d2/pull/2659)

#### Bugfixes ⛑️

- exports: pptx follows standards more closely, addressing warnings from some Powerpoint software [#2645](https://github.com/terrastruct/d2/pull/2645)
- d2sequence: fix edge case of invalid sequence diagrams [#2660](https://github.com/terrastruct/d2/pull/2660)
- d2svg: Text may overflow legend bounds when monospace font is used [#2674](https://github.com/terrastruct/d2/pull/2674)

---

For the latest d2.js changes, see separate [changelog](https://github.com/terrastruct/d2/blob/master/d2js/js/CHANGELOG.md).
