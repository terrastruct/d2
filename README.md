<div align="center">
  <h1>
    <img src="./docs/assets/logo.svg" alt="D2" />
  </h1>
  <p>A modern DSL that turns text into diagrams.</p>

[Language docs](https://d2-lang.com) | [Cheat sheet](./docs/assets/cheat_sheet.pdf)

[![ci](https://github.com/terrastruct/d2/actions/workflows/ci.yml/badge.svg)](https://github.com/terrastruct/d2/actions/workflows/ci.yml)
[![release](https://img.shields.io/github/v/release/terrastruct/d2)](https://github.com/terrastruct/d2/releases)
[![discord](https://img.shields.io/discord/976899413542830181?label=discord)](https://discord.gg/h9VFkAKTsT)
![twitter](https://img.shields.io/twitter/follow/terrastruct?style=social)
[![license](https://img.shields.io/github/license/terrastruct/d2?color=9cf)](./LICENSE.txt)
[![godoc](https://pkg.go.dev/badge/oss.terrastruct.com/d2.svg)](https://pkg.go.dev/oss.terrastruct.com/d2)

<img src="./docs/assets/cli.gif" alt="D2 CLI" />

</div>

# Table of Contents

<!-- toc -->

- [Quickstart (CLI)](#quickstart-cli)
  * [MacOS](#macos)
  * [Linux/Windows](#linuxwindows)
- [Quickstart (library)](#quickstart-library)
- [Themes](#themes)
- [Fonts](#fonts)
- [Export file types](#export-file-types)
- [Language tooling](#language-tooling)
- [Layout engine](#layout-engine)
- [Comparison](#comparison)
- [Contributing](#contributing)
- [License](#license)
- [Dependencies](#dependencies)
- [Related](#related)
  * [VSCode extension](#vscode-extension)
  * [Vim extension](#vim-extension)
  * [Misc](#misc)

<!-- tocstop -->

## Quickstart (CLI)
The most convenient way to use D2 is to just run it as a CLI executable to
produce SVGs from `.d2` files.

```sh
go install oss.terrastruct.com/d2

echo 'x -> y -> z' > in.d2
d2 --watch in.d2 out.svg
```

A browser window will open with `out.svg` and live-reload on changes to `in.d2`.

### MacOS

Homebrew package coming soon.

### Linux/Windows

We have precompiled binaries on the [releases](https://github.com/terrastruct/d2/releases)
page. D2 will be added to OS-respective package managers soon.


## Quickstart (library)
In addition to being a runnable CLI tool, D2 can also be used to produce diagrams from
Go programs.

```go
import (
	"github.com/terrastruct/d2/d2compiler"
	"github.com/terrastruct/d2/d2exporter"
	"github.com/terrastruct/d2/d2layouts/d2dagrelayout"
	"github.com/terrastruct/d2/d2renderers/textmeasure"
	"github.com/terrastruct/d2/d2themes/d2themescatalog"
)

func main() {
  graph, err := d2compiler.Compile("", strings.NewReader("x -> y"), &d2compiler.CompileOptions{ UTF16: true })
  ruler, err := textmeasure.NewRuler()
  err = graph.SetDimensions(nil, ruler)
  err = d2dagrelayout.Layout(ctx, graph)
  diagram, err := d2exporter.Export(ctx, graph, d2themescatalog.NeutralDefault)
  ioutil.WriteFile(filepath.Join("out.svg"), d2svg.Render(*diagram), 0600)
}
```

D2 is built to be hackable -- the language has an API built on top of it to make edits
programmatically.

```go
import (
  "github.com/terrastruct/d2/d2oracle"
  "github.com/terrastruct/d2/d2format"
)

// ...modifying the diagram `x -> y` from above
// Create a shape with the ID, "meow"
graph, err = d2oracle.Create(graph, "meow")
// Style the shape green
graph, err = d2oracle.Set(graph, "meow.style.fill", "green")
// Create a shape with the ID, "cat"
graph, err = d2oracle.Create(graph, "cat")
// Move the shape "meow" inside the container "cat"
graph, err = d2oracle.Move(graph, "meow", "cat.meow")
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

## Layout engine

D2 currently uses the open-source library [dagre](https://github.com/dagrejs/dagre) as its
default layout engine. D2 includes a wrapper around dagre to work around one of its
biggest limitations -- the inability to make container-to-container edges.

Dagre was chosen due to its popularity in other tools, but D2 intends to integrate with a
variety of layout engines, e.g. `dot`, as well as single-purpose layout types like
sequence diagrams. You can choose whichever layout engine you like and works best for the
diagram you're making.

Terrastruct has created a proprietary layout engine called
[TALA](https://terrastruct.com/tala). It has been designed specifically for software
architecture diagrams, though it's good for other domains too. TALA has many advantages
over other layout engines, the biggest being that it isn't constrained to hierarchies, or
any single type like "radial" or "tree" (as almost all layout engines are). For more
information and to download & try TALA, see
[https://github.com/terrastruct/TALA](https://github.com/terrastruct/TALA).

## Comparison

For a comparison against other popular text-to-diagram tools, see
[https://text-to-diagram.com](https://text-to-diagram.com).

## Contributing

Contributions are welcome! See [./docs/CONTRIBUTING.md](./docs/CONTRIBUTING.md).

## License

Copyright © 2022 Terrastruct, Inc. Open-source licensed under the Mozilla Public License
2.0.

## Dependencies

D2 is light on third-party dependencies in the source code. Note that these are bundled
with D2, you do not have to separately install anything.

| Dependency                                                                                                                          | What it does                              |
| -----------                                                                                                                         | -----------                               |
| [slog](https://cdr.dev/slog)                                                                                                        | logging (deprecating it is a TODO)        |
| [goldmark](https://github.com/yuin/goldmark), [goquery](https://github.com/PuerkitoBio/goquery)                                     | Markdown rendering                        |
| [chroma](https://github.com/alecthomas/chroma)                                                                                      | syntax highlighting code snippets         |
| [pflag](https://github.com/spf13/pflag), [fsnotify](https://github.com/fsnotify/fsnotify), [websocket](https://nhooyr.io/websocket) | CLI functions                             |
| [v8go](https://rogchap.com/v8go)                                                                                                    | Run Javascript (e.g. Dagre layout engine) |
| [gonum](https://gonum.org/v1/plot)                                                                                                  | Bezier curve stuff (rendering)            |

The rest are helpers we've open-sourced. E.g. [diff](https://oss.terrastruct.com/diff) for
our testing framework.

## Related

### VSCode extension

[https://github.com/terrastruct/d2-vscode](https://github.com/terrastruct/d2-vscode)

### Vim extension

[https://github.com/terrastruct/d2-vim](https://github.com/terrastruct/d2-vim)

### Misc

- [https://github.com/terrastruct/d2-docs](https://github.com/terrastruct/d2-docs)
- [https://github.com/terrastruct/text-to-diagram-com](https://github.com/terrastruct/text-to-diagram-com)
