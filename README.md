<div align="center">
  <img src="./docs/assets/banner.png" alt="D2" />
  <h2>
    A modern diagram scripting language that turns text to diagrams.
  </h2>

[Language docs](https://d2lang.com) | [Cheat sheet](./docs/assets/cheat_sheet.pdf)

[![ci](https://github.com/terrastruct/d2/actions/workflows/ci.yml/badge.svg)](https://github.com/terrastruct/d2/actions/workflows/ci.yml)
[![release](https://img.shields.io/github/v/release/terrastruct/d2)](https://github.com/terrastruct/d2/releases)
[![discord](https://img.shields.io/discord/1039184639652265985?label=discord)](https://discord.gg/NF6X8K4eDq)
[![twitter](https://img.shields.io/twitter/follow/terrastruct?style=social)](https://twitter.com/terrastruct)
[![license](https://img.shields.io/github/license/terrastruct/d2?color=9cf)](./LICENSE.txt)

<img src="./docs/assets/cli.gif" alt="D2 CLI" />

</div>

# Table of Contents

<!-- toc -->

- [What does D2 look like?](#what-does-d2-look-like)
- [Quickstart](#quickstart)
- [Install](#install)
- [D2 as a library](#d2-as-a-library)
- [Themes](#themes)
- [Fonts](#fonts)
- [Export file types](#export-file-types)
- [Language tooling](#language-tooling)
- [Plugins](#plugins)
- [Comparison](#comparison)
- [Contributing](#contributing)
- [License](#license)
- [Related](#related)
  * [VSCode extension](#vscode-extension)
  * [Vim extension](#vim-extension)
  * [Misc](#misc)
- [FAQ](#faq)

<!-- tocstop -->

# What does D2 look like?

```d2
# Actors
hans: Hans Niemann

defendants: {
  mc: Magnus Carlsen
  playmagnus: Play Magnus Group
  chesscom: Chess.com
  naka: Hikaru Nakamura

  mc -> playmagnus: Owns majority
  playmagnus <-> chesscom: Merger talks
  chesscom -> naka: Sponsoring
}

# Accusations
hans -> defendants: 'sueing for $100M'

# Offense
defendants.naka -> hans: Accused of cheating on his stream
defendants.mc -> hans: Lost then withdrew with accusations
defendants.chesscom -> hans: 72 page report of cheating
```

> There is syntax highlighting with the editor plugins linked below.

<img src="./docs/assets/syntax.png" alt="D2 render example" />

## Quickstart

The most convenient way to use D2 is to just run it as a CLI executable to
produce SVGs from `.d2` files.

```sh
# First, install D2
curl -fsSL https://d2lang.com/install.sh | sh -s --

echo 'x -> y -> z' > in.d2
d2 --watch in.d2 out.svg
```

A browser window will open with `out.svg` and live-reload on changes to `in.d2`.

## Install

The easiest way to install is with our install script:

```sh
curl -fsSL https://d2lang.com/install.sh | sh -s --
```

To uninstall:

```sh
curl -fsSL https://d2lang.com/install.sh | sh -s -- --uninstall
```

For detailed installation docs, with alternative methods and examples for each OS, see
[./docs/INSTALL.md](./docs/INSTALL.md).

## D2 as a library

In addition to being a runnable CLI tool, D2 can also be used to produce diagrams from
Go programs.

```go
import (
  "context"
  "io/ioutil"
  "path/filepath"
  "strings"

  "oss.terrastruct.com/d2/d2compiler"
  "oss.terrastruct.com/d2/d2exporter"
  "oss.terrastruct.com/d2/d2layouts/d2dagrelayout"
  "oss.terrastruct.com/d2/d2renderers/d2svg"
  "oss.terrastruct.com/d2/d2renderers/textmeasure"
  "oss.terrastruct.com/d2/d2themes/d2themescatalog"
)

func main() {
  graph, _ := d2compiler.Compile("", strings.NewReader("x -> y"), &d2compiler.CompileOptions{UTF16: true})
  ruler, _ := textmeasure.NewRuler()
  graph.SetDimensions(nil, ruler)
  d2dagrelayout.Layout(context.Background(), graph)
  diagram, _ := d2exporter.Export(context.Background(), graph, d2themescatalog.NeutralDefault.ID)
  out, _ := d2svg.Render(diagram)
  ioutil.WriteFile(filepath.Join("out.svg"), out, 0600)
}
```

D2 is built to be hackable -- the language has an API built on top of it to make edits
programmatically. Modifying the above diagram:

```go
import (
  "oss.terrastruct.com/d2/d2renderers/textmeasure"
  "oss.terrastruct.com/d2/d2themes/d2themescatalog"
)

// Create a shape with the ID, "meow"
graph, _, _ = d2oracle.Create(graph, "meow")
// Style the shape green
color := "green"
graph, _ = d2oracle.Set(graph, "meow.style.fill", nil, &color)
// Create a shape with the ID, "cat"
graph, _, _ = d2oracle.Create(graph, "cat")
// Move the shape "meow" inside the container "cat"
graph, _ = d2oracle.Move(graph, "meow", "cat.meow")
// Prints formatted D2 code
println(d2format.Format(graph.AST))
```

This makes it easy to build functionality on top of D2. Terrastruct uses the above API to
implement editing of D2 from mouse actions in a visual interface.

## Themes

D2 includes a variety of official themes to style your diagrams beautifully right out of
the box. See [./d2themes](./d2themes) to browse the available themes and make or
contribute your own creation.

## Fonts

D2 ships with "Source Sans Pro" as the font in renders. If you wish to use a different
one, please see [./d2renderers/d2fonts](./d2renderers/d2fonts).

## Export file types

D2 currently supports SVG exports. More coming soon.

## Language tooling

D2 is designed with language tooling in mind. D2's parser can parse multiple errors from a
broken program, has an autoformatter, syntax highlighting, and we have plans for LSP's and
more. Good language tooling is necessary for creating and maintaining large diagrams.

The extensions for VSCode and Vim can be found in the [Related](#related) section.

## Plugins

D2 is designed to be extensible and composable. The plugin system allows you to
change out layout engines and customize the rendering pipeline. Plugins can either be
bundled with the build or separately installed as a standalone binary.

**Layout engines**:

- [dagre](https://github.com/dagrejs/dagre) (default, bundled): A fast, directed graph
  layout engine that produces layered/hierarchical layouts. Based on Graphviz's DOT
  algorithm.
- [ELK](https://github.com/kieler/elkjs) (bundled): A directed graph layout engine
  particularly suited for node-link diagrams with an inherent direction and ports.
- [TALA](https://github.com/terrastruct/TALA) (binary): Novel layout engine designed
  specifically for software architecture diagrams. Requires separate install, visit the
  Github page for more.

D2 intends to integrate with a variety of layout engines, e.g. `dot`, as well as
single-purpose layout types like sequence diagrams. You can choose whichever layout engine
you like and works best for the diagram you're making.

## Comparison

For a comparison against other popular text-to-diagram tools, see
[https://text-to-diagram.com](https://text-to-diagram.com).

## Contributing

Contributions are welcome! See [./docs/CONTRIBUTING.md](./docs/CONTRIBUTING.md).

## License

Open sourced under the Mozilla Public License 2.0. See [./LICENSE.txt](./LICENSE.txt).

## Related

### VSCode extension

[https://github.com/terrastruct/d2-vscode](https://github.com/terrastruct/d2-vscode)

### Vim extension

[https://github.com/terrastruct/d2-vim](https://github.com/terrastruct/d2-vim)

### Language docs

[https://github.com/terrastruct/d2-docs](https://github.com/terrastruct/d2-docs)

### Misc

- [https://github.com/terrastruct/text-to-diagram-site](https://github.com/terrastruct/text-to-diagram-site)

## FAQ

- Does D2 collect telemetry?
  - No, D2 does not use an internet connection after installation, except to check for
    version updates from Github periodically.
- Does D2 need a browser to run?
  - No, D2 can run entirely server-side.
- I have a question or need help.
  - The best way to get help is to ask on [D2 Discord](https://discord.gg/NF6X8K4eDq)
- I have a feature request, proposal, or bug report.
  - Please open up a Github Issue.
- I have a private inquiry.
  - Please reach out at [hi@d2lang.com](hi@d2lang.com).
