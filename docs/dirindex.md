# Directory Index

<!-- toc -->

- [Structure](#structure)
  * [d2](#d2)
  * [d2ast](#d2ast)

<!-- tocstop -->

This is largely a TODO, waiting for things to stablize.

## Structure

### [d2](./d2.go)

`d2` hooks up the other packages together into a minimal `Compile` function that
implements the flow shown in the above diagram.

### [d2ast](d2ast)
- [d2ast](d2ast)
  - `d2ast` implements the types used to represent D2 AST language nodes.
- [d2parser](d2parser)
  - `d2parser` implements a recursive descent parser that parses D2 text into `d2ast`.
    It keeps track of node ranges finely and is robustly capable of handling bad input.
- [d2compiler](d2compiler)
  - `d2compiler` takes the AST from `d2parser` and compiles its keys and values into a
    `d2graph`. It can handle incomplete ASTs.
- [d2graph](d2graph)
  - `d2graph` implements the complete representation of a D2 graph with nodes, edges and
    reserved keywords. Classes, styles, scenarios and more are planned.
  - It maintains information for layout and exporting but also for the language tooling.
    See `d2oracle` below.
- [d2layouts](d2layouts)
  - `d2layouts` contains the autolayout algorithms available in D2 via `$D2_LAYOUT=...` e.g. `$D2_LAYOUT=dagre`.
  - [d2talalayout](d2layouts/d2talalayout)
    - `d2talalayout` lays out the `d2graph` using the Terrastruct autolayout algorithm //
      TODO
  - [d2dagrelayout](d2layouts/d2dagrelayout)
    - `d2dagrelayout` lays out the `d2graph` using the Dagre graph layout algorithm.
    - See https://github.com/dagrejs/dagre
    - Layouts from Dagre can serve as a good comparison against our own algorithm.
      To layout graphs with dagre in dev you'd do: `D2_LAYOUT=dagre ./ci/dev.sh`
- [d2exporter](d2exporter)
  - `d2exporter` takes `d2graph` and derives a target diagram, applying themes along the way
- [d2target](d2target)
  - `d2target` contains the structure of the target diagram.

- [d2format](d2format)
  - `d2format` implements the autoformatter.
- [d2oracle](d2oracle)
  - `d2oracle` implements the D2 language tooling. Currently it only contains an API
    for richly editing a `d2graph` and its ASTs. Eventually it will support all LSP
    functionality like find all references, renaming etc.
  - Bidirectional navigation will also be implemented here.
- [d2chaos](d2chaos)
  - `d2chaos` implements a test for the D2 flow with randomly generated D2 files. Its main
    purpose is to ensure nothing panics or errors out. It's currently disabled due to a
    nefarious panic from autolayout.
- [d2transpilers](d2transpilers)
  - `d2transpilers` is a planned directory to hold our transpilers from other diagramming
    languages like `.dot`, `.uml` and `.mmd` to D2.
- [d2renderers](d2renderers)
  - `d2renderers` is a planned directory to hold renderers that render `d2target` into images, e.g. SVG
- [d2themes](d2themes)
  - `d2themes` is the package that makes D2 diagrams pretty
  - `d2themescatalog` is the list of available themes. New themes should be added here.

- [lib](lib)
  - `lib` contains utility libraries.
  - [assert](lib/assert)
    - `assert` contains test assertion helpers.
  - [diff](lib/diff)
    - `diff` contains string and file generation functions. Used in tests to print the
      differences between expected and actual output.
  - [env](lib/env)
    - `env` contains environment variable helpers.
  - [geo](lib/geo)
    - `geo` provides geometry related structures and logic for points, line segments and
      more.
