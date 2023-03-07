ELK layout has been much improved by increasing node dimensions to make room for nice even padding around ports:
![elk](https://user-images.githubusercontent.com/3120367/223520168-96dad4c4-3f7f-4c7f-a6d5-5115a6f04e5b.png)

Do you use ELK more than dagre? We're considerng switching d2's default layout engine to ELK, so please chime in to this poll if you have an opinion! https://github.com/terrastruct/d2/discussions/990

#### Improvements 🧹

- ELK nodes with > 1 connection grow to ensure padding around ports [#981](https://github.com/terrastruct/d2/pull/981)
- Using a style keyword incorrectly in connections returns clear error message [#989](https://github.com/terrastruct/d2/pull/989)
- Unsemantic Markdown returns clear error message [#994](https://github.com/terrastruct/d2/pull/994)

#### Bugfixes ⛑️

- Accept absolute paths again on the CLI (regression from previous release). [#979](https://github.com/terrastruct/d2/pull/979)
- Fixes some rare undefined behavior using capitalized reserved keywords [#978](https://github.com/terrastruct/d2/pull/978)
- Fixes an error rendering when links contained `&` characters [#988](https://github.com/terrastruct/d2/pull/988)
