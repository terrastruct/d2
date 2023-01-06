#### Features 🚀

- Crow foot notation is now supported. [#578](https://github.com/terrastruct/d2/pull/578)
- Exported SVGs also fit to screen on open. [#601](https://github.com/terrastruct/d2/pull/601)

#### Improvements 🧹

#### Bugfixes ⛑️

- Restricts where `near` key constant values can be used, with good error messages, instead of erroring (e.g. setting `near: top-center` on a container would cause bad layouts or error). [#538](https://github.com/terrastruct/d2/pull/538)
- Fixes an error during ELK layout when images had empty labels. [#555](https://github.com/terrastruct/d2/pull/555)
- Fixes rendering classes and tables with empty headers. [#498](https://github.com/terrastruct/d2/pull/498)
- Fixes rendering sql tables with no columns. [#553](https://github.com/terrastruct/d2/pull/553)
- Appendix seperator line no longer added to PNG export when appendix doesn't exist. [#582](https://github.com/terrastruct/d2/pull/582)
- Watch mode only fits to screen on initial load. [#601](https://github.com/terrastruct/d2/pull/601)
- Dimensions (`width`/`height`) were incorrectly giving compiler errors when applied on a shape with style. [#614](https://github.com/terrastruct/d2/pull/614)
- Fixes routing between sql table columns if the column name is the prefix of the table name [#615](https://github.com/terrastruct/d2/pull/615)
