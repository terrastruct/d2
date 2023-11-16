The globs feature underwent a major rewrite and is now almost finalized.

### Before

Previously, globs would evaluate once on all the shapes and connections declared above it. So if you wanted to set everything red, you had to add the line at the bottom.

```d2
x
y

*.style.fill: red
```

### Now

```d2
*.style.fill: red

x
y
```

We still have one more release in 0.6 series to add filters to globs, so stay tuned.

You might also be interested to know that grid cells can now have connections between them! Source code for this diagram [here](https://github.com/terrastruct/d2/blob/master/e2etests/testdata/files/simple_grid_edges.d2).

![267854495-bc0a5456-3618-4d46-84db-f211ffb5246a](https://github.com/terrastruct/d2/assets/3120367/bb7b01a5-5473-401d-baf7-9faf2e7cfbe8)


#### Features 🚀

- UTF-16 files are automatically detected and supported [#1525](https://github.com/terrastruct/d2/pull/1525)
- Grid diagrams can now have simple connections between top-level cells [#1586](https://github.com/terrastruct/d2/pull/1586)

#### Improvements 🧹

- Globs are lazily-evaluated [#1552](https://github.com/terrastruct/d2/pull/1552)
- Latex blocks includes Mathjax's ASM extension [#1544](https://github.com/terrastruct/d2/pull/1544)
- `font-color` works on Markdown [#1546](https://github.com/terrastruct/d2/pull/1546)
- `font-color` works on arrowheads [#1582](https://github.com/terrastruct/d2/pull/1582)
- CLI failure message includes input path [#1617](https://github.com/terrastruct/d2/pull/1617)

#### Bugfixes ⛑️

- `d2 fmt` formats all files passed as arguments rather than just the first non-formatted (thank you @maxbrunet) [#1523](https://github.com/terrastruct/d2/issues/1523)
- Fixes Markdown cropping last element in mixed-element blocks (e.g. em and strong) [#1543](https://github.com/terrastruct/d2/issues/1543)
- Adds compiler error for non-blockstring empty labels [#1590](https://github.com/terrastruct/d2/issues/1590)
- Prevents multiple constant nears overlapping in some cases [#1591](https://github.com/terrastruct/d2/issues/1591)
- Fixes crash from empty nested grid [#1594](https://github.com/terrastruct/d2/issues/1594)
- `d2fmt` with variable substitution mid-string is formatted correctly [#1611](https://github.com/terrastruct/d2/issues/1611)
- Fixes certain shape IDs not working with dagre [#1610](https://github.com/terrastruct/d2/issues/1610)
- Fixes font-size adjustments missing from rendered code shape [#1614](https://github.com/terrastruct/d2/issues/1614)
