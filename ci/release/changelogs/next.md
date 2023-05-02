This release improves on the features introduced in 0.4, with `class` keyword now accepting multiple class values with an array, and grid diagrams becoming faster and more robust. 

Multiple classes example:

<img src="https://user-images.githubusercontent.com/3120367/235749202-aa85830e-8f4a-4a2c-be16-599302919122.svg" style="width: 600px" />

```d2
classes: {
  base: {
    style: {
      stroke-dash: 2
      border-radius: 5
      font: mono
      text-transform: uppercase
    }
  }
  error: {
    style.fill: "#e07d7d"
    style.stroke: "#a60c0c"
    style.font-color: white
  }
  success: {
    style.fill: "#86f499"
    style.stroke: "#017f07"
    style.font-color: black
  }
}

server-1.class: [base; error]
server-2.class: [base; success]

```

#### Features 🚀

- `class` field now accepts arrays. See [docs](https://d2lang.com/tour/classes/#multiple-classes). [#1256](https://github.com/terrastruct/d2/pull/1256)
- Pill shape is implemented with rectangles of large border radius. Thanks @Poivey ! [#1006](https://github.com/terrastruct/d2/pull/1006)

#### Improvements 🧹

- ELK self loops get distributed around the object instead of stacking [#1232](https://github.com/terrastruct/d2/pull/1232)
- ELK preserves order of objects in cycles [#1235](https://github.com/terrastruct/d2/pull/1235)
- Improper usages of `class` and `style` get error messages [#1254](https://github.com/terrastruct/d2/pull/1254)
- Improves scaling of object widths/heights in grid diagrams [#1263](https://github.com/terrastruct/d2/pull/1263)
- Enhance Markdown parsing error message by appending link to docs [#1269](https://github.com/terrastruct/d2/pull/1269)

#### Bugfixes ⛑️

- Fixes an issue with markdown labels that are empty when rendered [#1223](https://github.com/terrastruct/d2/issues/1223)
- ELK self loops always have enough space for long labels [#1232](https://github.com/terrastruct/d2/pull/1232)
- Fixes panic when setting `shape` to be `class` or `sql_table` within a class [#1251](https://github.com/terrastruct/d2/pull/1251)
- Fixes rare panic exporting to gifs [#1257](https://github.com/terrastruct/d2/pull/1257)
- Fixes bad performance in large grid diagrams [#1263](https://github.com/terrastruct/d2/pull/1263)
- Fixes bug in ELK when container has ID "root" [#1268](https://github.com/terrastruct/d2/pull/1268)
- Fixes edge case panic with invalid CLI arguments [#1271](https://github.com/terrastruct/d2/pull/1271)
