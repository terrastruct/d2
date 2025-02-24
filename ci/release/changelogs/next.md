#### Features 🚀

- Icons: connections can include icons [#12](https://github.com/terrastruct/d2/issues/12)

#### Improvements 🧹

- d2js: Support `d2-config`. Support additional options: [#2343](https://github.com/terrastruct/d2/pull/2343)
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

#### Bugfixes ⛑️

- Compiler:
  - fixes panic when `sql_shape` shape value had mixed casing [#2349](https://github.com/terrastruct/d2/pull/2349)
  - fixes support for `center` in `d2-config` [#2360](https://github.com/terrastruct/d2/pull/2360)
