#### Features 🚀

- Tooltips can be set on shapes. See [https://d2lang.com/tour/tooltips](https://d2lang.com/tour/interactive). [#548](https://github.com/terrastruct/d2/pull/548)
- Links can be set on shapes. See [https://d2lang.com/tour/tooltips](https://d2lang.com/tour/interactive). [#548](https://github.com/terrastruct/d2/pull/548)
- The `width` and `height` attributes are no longer restricted to images and can be applied to non-container shapes. [#498](https://github.com/terrastruct/d2/pull/498)

#### Improvements 🧹

#### Bugfixes ⛑️

- Restricts where `near` key constant values can be used, with good error messages, instead of erroring (e.g. setting `near: top-center` on a container would cause bad layouts or error). [#538](https://github.com/terrastruct/d2/pull/538)
- Fixes an error during ELK layout when images had empty labels. [#555](https://github.com/terrastruct/d2/pull/555)
- Fixes rendering classes and tables with empty headers. [#498](https://github.com/terrastruct/d2/pull/498)
- Fixes rendering sql tables with no columns. [#553](https://github.com/terrastruct/d2/pull/553)
