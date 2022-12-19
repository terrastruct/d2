#### Features 🚀

- Diagram padding can now can be configured in the CLI (default 100px). [https://github.com/terrastruct/d2/pull/431](https://github.com/terrastruct/d2/pull/431)
- Connection label backgrounds can now be set with the `style.fill` keyword. [https://github.com/terrastruct/d2/pull/452](https://github.com/terrastruct/d2/pull/452)
- Add official Docker image. See [./docs/INSTALL.md#docker](./docs/INSTALL.md#docker). [#76](https://github.com/terrastruct/d2/issues/76)
- Add `.msi` installer for convenient installation on Windows. [#379](https://github.com/terrastruct/d2/issues/379)

#### Improvements 🧹

- `d2 fmt` now preserves leading comment spacing. [#400](https://github.com/terrastruct/d2/issues/400)

#### Bugfixes ⛑️

- Fixed crash when sequence diagrams had no messages. [https://github.com/terrastruct/d2/pull/427](https://github.com/terrastruct/d2/pull/427)
- Fixed `constraint` keyword setting label. [https://github.com/terrastruct/d2/issues/415](https://github.com/terrastruct/d2/issues/415)
- Fixed serialization affecting binary plugins (TALA). [https://github.com/terrastruct/d2/pull/426](https://github.com/terrastruct/d2/pull/426)
- Fixed connections in elk layouts not going all the way to shape borders. [https://github.com/terrastruct/d2/pull/459](https://github.com/terrastruct/d2/pull/459)
- Fixed a connection rendering bug that could happen in firefox when there were no connection labels. [https://github.com/terrastruct/d2/pull/453](https://github.com/terrastruct/d2/pull/453)
- Fixed a crash when external connection IDs were prefixes of a sequence diagram ID. [https://github.com/terrastruct/d2/pull/462](https://github.com/terrastruct/d2/pull/462)
