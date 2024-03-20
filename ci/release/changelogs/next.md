#### Features 🚀

- `style.underline` works on connections [#1836](https://github.com/terrastruct/d2/pull/1836)

#### Improvements 🧹

- Boards no longer inherit `label` fields from parents [#1838](https://github.com/terrastruct/d2/pull/1838)
- Prevents `near` targeting a child of a special object like grid cells, which wasn't doing anything [#1851](https://github.com/terrastruct/d2/pull/1851)

#### Bugfixes ⛑️

- Fixes styles in connections not overriding styles set by globs [#1857](https://github.com/terrastruct/d2/pull/1857)
- Fixes `null` being set on a nested shape not working in certain cases when connections also pointed to that shape [#1830](https://github.com/terrastruct/d2/pull/1830)
- Fixes edge case of bad import syntax crashing using d2 as a library [#1829](https://github.com/terrastruct/d2/pull/1829)
- Fixes `style.fill` not applying to markdown [#1872](https://github.com/terrastruct/d2/pull/1872)
