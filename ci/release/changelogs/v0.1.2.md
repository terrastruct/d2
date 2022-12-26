D2 now has an official playground site: [https://play.d2lang.com](https://play.d2lang.com). It loads and runs fast, works on all the major browsers and has been tested on desktop and mobile on a variety of devices. It's the easiest way to get started with D2 and share diagrams. The playground is all open source ([https://github.com/terrastruct/d2-playground](https://github.com/terrastruct/d2-playground)). We'd love to hear your feedback and feature requests.

Windows users, the install experience just got a whole lot better. Making D2 accessible and easy to use continues to be a priority for us. With this release, we added an MSI installer for Windows, so that installs are just a few clicks. An official Docker image has also been added.

#### Features 🚀

- Diagram padding can be configured in the CLI (default 100px). [https://github.com/terrastruct/d2/pull/431](https://github.com/terrastruct/d2/pull/431)
- Connection label backgrounds can be set with the `style.fill` keyword. [https://github.com/terrastruct/d2/pull/452](https://github.com/terrastruct/d2/pull/452)
- Adds official Docker image. See [./docs/INSTALL.md#docker](./docs/INSTALL.md#docker). [#76](https://github.com/terrastruct/d2/issues/76)
- Adds `.msi` installer for convenient installation on Windows. [#379](https://github.com/terrastruct/d2/issues/379)

#### Improvements 🧹

- `d2 fmt` preserves leading comment spacing. [#400](https://github.com/terrastruct/d2/issues/400)
- `stroke` and `fill` keywords work for Markdown text. [https://github.com/terrastruct/d2/pull/460](https://github.com/terrastruct/d2/pull/460)
- PNG export resolution increased by 2x to not be blurry exporting on retina displays. [https://github.com/terrastruct/d2/pull/445](https://github.com/terrastruct/d2/pull/445)

#### Bugfixes ⛑️

- Fixes crash when sequence diagrams has no messages. [https://github.com/terrastruct/d2/pull/427](https://github.com/terrastruct/d2/pull/427)
- Fixes `constraint` keyword setting label. [https://github.com/terrastruct/d2/issues/415](https://github.com/terrastruct/d2/issues/415)
- Fixes serialization affecting binary plugins (TALA). [https://github.com/terrastruct/d2/pull/426](https://github.com/terrastruct/d2/pull/426)
- Fixes connections in ELK layouts not going all the way to shape borders. [https://github.com/terrastruct/d2/pull/459](https://github.com/terrastruct/d2/pull/459)
- Fixes a connection rendering bug that could happen in Firefox when there were no connection labels. [https://github.com/terrastruct/d2/pull/453](https://github.com/terrastruct/d2/pull/453)
- Fixes a crash when external connection IDs were prefixes of a sequence diagram ID. [https://github.com/terrastruct/d2/pull/462](https://github.com/terrastruct/d2/pull/462)
