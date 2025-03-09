#### Features 🚀

- Icons: connections can include icons [#12](https://github.com/terrastruct/d2/issues/12)
- Syntax: `suspend`/`unsuspend` to define models and instantiate them [#2394](https://github.com/terrastruct/d2/pull/2394)
- Globs: support for filtering edges based on properties of endpoint nodes (e.g., `&src.style.fill: blue`) [#2395](https://github.com/terrastruct/d2/pull/2395)
- Render:
  - markdown, latex, and code can be used as object labels [#2204](https://github.com/terrastruct/d2/pull/2204)
  - `shape: c4-person` to render a person shape like what the C4 model prescribes [#2397](https://github.com/terrastruct/d2/pull/2397)

#### Improvements 🧹

- d2js:
  - Support `d2-config`. Support additional options: [#2343](https://github.com/terrastruct/d2/pull/2343)
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
  - Support relative imports. Improve elk error handling: [#2382](https://github.com/terrastruct/d2/pull/2382)
  - Support fonts (`fontRegular`, `fontItalic`, `fontBold`, `fontSemiBold`): [#2384](https://github.com/terrastruct/d2/pull/2384)

- d2cli:
    - Support `validate` command. [#2415](https://github.com/terrastruct/d2/pull/2415)

#### Bugfixes ⛑️

- Compiler:
  - fixes panic when `sql_shape` shape value had mixed casing [#2349](https://github.com/terrastruct/d2/pull/2349)
  - fixes support for `center` in `d2-config` [#2360](https://github.com/terrastruct/d2/pull/2360)
  - fixes panic when comment lines appear in arrays [#2378](https://github.com/terrastruct/d2/pull/2378)
  - fixes inconsistencies when objects were double quoted [#2390](https://github.com/terrastruct/d2/pull/2390)
- CLI: fetch and render remote images of mimetype octet-stream correctly [#2370](https://github.com/terrastruct/d2/pull/2370)
- d2js: handle unicode characters [#2393](https://github.com/terrastruct/d2/pull/2393)
