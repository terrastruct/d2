# Changelog

All notable changes to only the d2.js package will be documented in this file. **Does not
include changes to the main d2 project.**

## Next

- Add `D2.dispose()` for terminating the backing worker
- Fix concurrent calls sharing a D2 instance
- Fix completions after Unicode characters
- Deprecate the raw `d2.getELKGraph` and `d2.getObjOrder` compatibility exports. They remain callable for one release and emit one migration warning per export.
- Fix TypeScript signatures
- Fix theme-overrides not applying
- Fix grids in ELK layouts
- Significantly reduce bundle size
- Support D2 0.7.1

## [0.1.22]
### March 20, 2025

- Support `d2-config`. Support additional options. [#2343](https://github.com/terrastruct/d2/pull/2343)
  - `themeID`
  - `darkThemeID`
  - `center`
  - `pad`
  - `scale`
  - `forceAppendix`
  - `target`
  - `animateInterval`
  - `salt`
  - `noXMLTag`
- Support relative imports. Improve elk error handling [#2382](https://github.com/terrastruct/d2/pull/2382)
- Support fonts (`fontRegular`, `fontItalic`, `fontBold`, `fontSemiBold`) [#2384](https://github.com/terrastruct/d2/pull/2384)
- Add TypeScript signatures

## [0.1.21]
### January 12, 2025

First public release
