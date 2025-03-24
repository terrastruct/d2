#### Features 🚀

- Icons: connections can include icons [#12](https://github.com/terrastruct/d2/issues/12)
- Syntax: `suspend`/`unsuspend` to define models and instantiate them [#2394](https://github.com/terrastruct/d2/pull/2394)
- Globs: support for filtering edges based on properties of endpoint nodes (e.g., `&src.style.fill: blue`) [#2395](https://github.com/terrastruct/d2/pull/2395)
- Render:
  - markdown, latex, and code can be used as object labels [#2204](https://github.com/terrastruct/d2/pull/2204)
  - `shape: c4-person` to render a person shape like what the C4 model prescribes [#2397](https://github.com/terrastruct/d2/pull/2397)
- Diagram legends are implemented [#2416](https://github.com/terrastruct/d2/pull/2416)

#### Improvements 🧹

- CLI:
    - Support `validate` command. [#2415](https://github.com/terrastruct/d2/pull/2415)
    - Watch mode ignores backup files (e.g. files created by certain editors like Helix). [#2131](https://github.com/terrastruct/d2/issues/2131)
- Compiler:
    - `link`s can be set to root path, e.g. `/xyz`. [#2357](https://github.com/terrastruct/d2/issues/2357)
- Render:
    - horizontal padding added for connection labels [#2461](https://github.com/terrastruct/d2/pull/2461)

#### Bugfixes ⛑️

- Compiler:
  - fixes panic when `sql_shape` shape value had mixed casing [#2349](https://github.com/terrastruct/d2/pull/2349)
  - fixes panic when importing from a file with spread substitutions in `vars` [#2427](https://github.com/terrastruct/d2/pull/2427)
  - fixes support for `center` in `d2-config` [#2360](https://github.com/terrastruct/d2/pull/2360)
  - fixes panic when comment lines appear in arrays [#2378](https://github.com/terrastruct/d2/pull/2378)
  - fixes inconsistencies when objects were double quoted [#2390](https://github.com/terrastruct/d2/pull/2390)
  - fixes globs not applying to spread substitutions [#2426](https://github.com/terrastruct/d2/issues/2426)
  - fixes panic when classes were mixed with layers incorrectly [#2448](https://github.com/terrastruct/d2/pull/2448)
- CLI: fetch and render remote images of mimetype octet-stream correctly [#2370](https://github.com/terrastruct/d2/pull/2370)
- Composition: spread importing scenarios/steps was not inheriting correctly [#2460](https://github.com/terrastruct/d2/pull/2460)
- Markdown: fixes nested var substitutions not working [#2456](https://github.com/terrastruct/d2/pull/2456)
- d2js: handle unicode characters [#2393](https://github.com/terrastruct/d2/pull/2393)

---

For the latest d2.js changes, see separate [changelog](https://github.com/terrastruct/d2/blob/master/d2js/js/CHANGELOG.md).
