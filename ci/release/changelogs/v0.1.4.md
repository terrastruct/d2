This release marks the introduction of interactive diagrams. Namely, `tooltip` and `link` can now be set, which allows you to hover to see more or click to go to an external link. This small change enables many possibilities, including richer integrations like internal wiki's that can be linked together through diagrams. An icon will indicate that a shape has a tooltip that can be hovered over for more information, or a link.

<img width="509" alt="Screen Shot 2022-12-30 at 6 47 45 PM" src="https://user-images.githubusercontent.com/3120367/210122771-b38003e7-2881-4708-a875-8066c465c16c.png">

Since interactive features obviously won't work on static export formats like PNG, they will be included automatically in an appendix when exporting to those formats, like so:

![tooltip](https://user-images.githubusercontent.com/3120367/210122793-582d3fc7-8e09-46f1-bb78-5dcc6cf1de55.png)

This release also gives more power to configure layouts. `width` and `height` are D2 keywords which previouslly only worked on images, but now work on any non-containers. Additionally, all the layout engines have configurations exposed. D2 sets sensible defaults to each layout engine without any input, so this is meant to be an advanced feature for users who want that extra control.

Happy new years!

#### Features 🚀

- Tooltips can be set on shapes. See [https://d2lang.com/tour/interactive](https://d2lang.com/tour/interactive). [#548](https://github.com/terrastruct/d2/pull/548)
- Links can be set on shapes. See [https://d2lang.com/tour/interactive](https://d2lang.com/tour/interactive). [#548](https://github.com/terrastruct/d2/pull/548)
- The `width` and `height` attributes are no longer restricted to images and can be applied to non-container shapes. [#498](https://github.com/terrastruct/d2/pull/498)
- Layout engine options are exposed and configurable. See individual layout pages on [https://d2lang.com/tour/layouts](https://d2lang.com/tour/layouts) for list of configurations. [#563](https://github.com/terrastruct/d2/pull/563)

#### Improvements 🧹

- Watch mode renders fit to screen. [#560](https://github.com/terrastruct/d2/pull/560)

#### Bugfixes ⛑️

- Fixes rendering `class` and `table` with empty headers. [#498](https://github.com/terrastruct/d2/pull/498)
- Fixes rendering of `sql_table` with no columns. [#553](https://github.com/terrastruct/d2/pull/553)
- Diagram bounding boxes account for stroke widths. [#574](https://github.com/terrastruct/d2/pull/574)
- Restricts where `near` key constant values can be used, with good error messages, instead of erroring (e.g. setting `near: top-center` on a container would cause bad layouts or error). [#538](https://github.com/terrastruct/d2/pull/538)
- Fixes panic when images with empty labels are rendered with ELK. [#555](https://github.com/terrastruct/d2/pull/555)

#### Breaking changes

- For usages of D2 as a library, `d2dagrelayout.Layout` and `d2elklayout.Layout` now accept a third parameter for options. If you would like to keep the defaults, please change your code to call `dagrelayout.DefaultLayout` and `d2elklayout.DefaultLayout` respectively.

