<div align="center">
  <img src="./docs/assets/banner.png" alt="D2" />
  <h2>
    A modern diagram scripting language that turns text to diagrams.
  </h2>


[Docs](https://d2lang.com) | [Cheat sheet](./docs/assets/cheat_sheet.pdf) | [Comparisons](https://text-to-diagram.com) | [Playground](https://play.d2lang.com)

[![ci](https://github.com/terrastruct/d2/actions/workflows/ci.yml/badge.svg)](https://github.com/terrastruct/d2/actions/workflows/ci.yml)
[![daily](https://github.com/terrastruct/d2/actions/workflows/daily.yml/badge.svg)](https://github.com/terrastruct/d2/actions/workflows/daily.yml)
[![release](https://img.shields.io/github/v/release/terrastruct/d2)](https://github.com/terrastruct/d2/releases)
[![discord](https://img.shields.io/discord/1039184639652265985?label=discord)](https://discord.gg/NF6X8K4eDq)
[![twitter](https://img.shields.io/twitter/follow/terrastruct?style=social)](https://twitter.com/terrastruct)
[![license](https://img.shields.io/github/license/terrastruct/d2?color=9cf)](./LICENSE.txt)

<a href="https://play.d2lang.com">
<img src="./docs/assets/playground_button.png" alt="D2 Playground button" width="200" />
</a>

https://user-images.githubusercontent.com/3120367/206125010-bd1fea8e-248a-43e7-8f85-0bbfca0c6e2a.mp4

</div>

# Table of Contents

<!-- toc -->
- <a href="#what-does-d2-look-like" id="toc-what-does-d2-look-like">What does D2 look like?</a>
- <a href="#quickstart" id="toc-quickstart">Quickstart</a>
- <a href="#install" id="toc-install">Install</a>
- <a href="#d2-as-a-library" id="toc-d2-as-a-library">D2 as a library</a>
- <a href="#themes" id="toc-themes">Themes</a>
- <a href="#fonts" id="toc-fonts">Fonts</a>
- <a href="#export-file-types" id="toc-export-file-types">Export file types</a>
- <a href="#language-tooling" id="toc-language-tooling">Language tooling</a>
- <a href="#plugins" id="toc-plugins">Plugins</a>
- <a href="#comparison" id="toc-comparison">Comparison</a>
- <a href="#contributing" id="toc-contributing">Contributing</a>
- <a href="#license" id="toc-license">License</a>
- <a href="#related" id="toc-related">Related</a>
  - <a href="#official-plugins" id="toc-official-plugins">Official plugins</a>
  - <a href="#community-plugins" id="toc-community-plugins">Community plugins</a>
  - <a href="#misc" id="toc-misc">Misc</a>
- <a href="#faq" id="toc-faq">FAQ</a>
- <a href="#open-source-projects-documenting-with-d2" id="toc-open-source-projects-documenting-with-d2">Open-source projects documenting with D2</a>

## What does D2 look like?

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

> Rendered with the TALA layout engine.

> For more examples, see [./docs/examples](./docs/examples).

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

You can run the install script with `--dry-run` to see the commands that will be used
to install without executing them.

Or if you have Go installed you can install from source though you won't get the manpage:

```sh
go install oss.terrastruct.com/d2@latest
```

You can also install a release from source which will include manpages.
See [./docs/INSTALL.md#source-release](./docs/INSTALL.md#source-release).

To uninstall with the install script:

```sh
curl -fsSL https://d2lang.com/install.sh | sh -s -- --uninstall
```

For detailed installation docs, see [./docs/INSTALL.md](./docs/INSTALL.md).
We demonstrate alternative methods and examples for each OS.

As well, the functioning of the install script is described in detail to alleviate any
concern of its use. We recommend using your OS's package manager directly instead for
improved security but the install script is by no means insecure.

## D2 as a library

In addition to being a runnable CLI tool, D2 can also be used to produce diagrams from
Go programs.

For examples, see [./docs/examples/lib](./docs/examples/lib). This [blog
post](https://terrastruct.com/blog/post/generate-diagrams-programmatically/) also demos a
complete, runnable example of using D2 as a library for a real-world use case.

## Themes

D2 includes a variety of official themes to style your diagrams beautifully right out of
the box. See [./d2themes](./d2themes) to browse the available themes and make or
contribute your own creation.

## Fonts

D2 ships with "Source Sans Pro" as the font in renders. If you wish to use a different
one, please see [./d2renderers/d2fonts](./d2renderers/d2fonts).

## Export file types

D2 currently supports SVG and PNG exports. More coming soon.

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

We are constantly working on new plugins, integrations, extensions. Contributions are
welcome in any official or community plugins. If you have somewhere in your workflow that
you want to use D2, feel free to open a discussion. We have limited bandwidth and usually
choose the most high-demand ones to work on. If you make something cool with D2 yourself,
let us know and we'll be happy to include it here!

### Official plugins

- **VSCode extension**: [https://github.com/terrastruct/d2-vscode](https://github.com/terrastruct/d2-vscode)
- **Vim extension**: [https://github.com/terrastruct/d2-vim](https://github.com/terrastruct/d2-vim)
- **Obsidian plugin**: [https://github.com/terrastruct/d2-obsidian](https://github.com/terrastruct/d2-obsidian)
- **Slack app**: [https://d2lang.com/tour/slack](https://d2lang.com/tour/slack)
- **Discord plugin**: [https://d2lang.com/tour/discord](https://d2lang.com/tour/discord)

### Community plugins

- **Tree-sitter grammar**: [https://github.com/pleshevskiy/tree-sitter-d2](https://github.com/pleshevskiy/tree-sitter-d2)
- **Emacs major mode**: [https://github.com/andorsk/d2-mode](https://github.com/andorsk/d2-mode)
- **Goldmark extension**: [https://github.com/FurqanSoftware/goldmark-d2](https://github.com/FurqanSoftware/goldmark-d2)
- **Telegram bot**: [https://github.com/meinside/telegram-d2-bot](https://github.com/meinside/telegram-d2-bot)
- **Postgres importer**: [https://github.com/zekenie/d2-erd-from-postgres](https://github.com/zekenie/d2-erd-from-postgres)
- **Structurizr to D2 exporter**: [https://github.com/goto1134/structurizr-d2-exporter](https://github.com/goto1134/structurizr-d2-exporter)
- **MdBook preprocessor**: [https://github.com/danieleades/mdbook-d2](https://github.com/danieleades/mdbook-d2)
- **ROS2 D2 Exporter**: [https://github.com/Greenroom-Robotics/ros-d2](https://github.com/Greenroom-Robotics/ros-d2)
- **D2 org-mode support**: [https://github.com/xcapaldi/ob-d2](https://github.com/xcapaldi/ob-d2)
- **Python D2 diagram builder**: [https://github.com/MrBlenny/py-d2](https://github.com/MrBlenny/py-d2)
- **Clojure D2 transpiler**: [https://github.com/judepayne/dictim](https://github.com/judepayne/dictim)
- **JavaScript D2 diagram builder**: [https://github.com/Kreshnik/d2lang-js](https://github.com/Kreshnik/d2lang-js)
- **Maven plugin**: [https://github.com/andrinmeier/unofficial-d2lang-confluence-plugin](https://github.com/andrinmeier/unofficial-d2lang-confluence-plugin)
- **Confluence plugin**: [https://github.com/andrinmeier/unofficial-d2lang-confluence-plugin](https://github.com/andrinmeier/unofficial-d2lang-confluence-plugin)
- **CIL (C#, Visual Basic, F#, C++ CLR) to D2**: [https://github.com/HugoVG/AppDiagram](https://github.com/HugoVG/AppDiagram)

### Misc

- **Comparison site**: [https://github.com/terrastruct/text-to-diagram-site](https://github.com/terrastruct/text-to-diagram-site)
- **Playground**: [https://github.com/terrastruct/d2-playground](https://github.com/terrastruct/d2-playground)
- **Language docs**: [https://github.com/terrastruct/d2-docs](https://github.com/terrastruct/d2-docs)
- **Hosted icons**: [https://icons.terrastruct.com](https://icons.terrastruct.com)

## FAQ

- Does D2 collect telemetry?
  - No, D2 does not use an internet connection after installation, except to check for
    version updates from Github periodically.
- Does D2 need a browser to run?
  - No, D2 can run entirely server-side.
- What's coming in the next release?
  - See [./ci/release/changelogs/next.md](./ci/release/changelogs/next.md).
- I have a question or need help.
  - The best way to get help is to ask on [D2 Discord](https://discord.gg/NF6X8K4eDq)
- I have a feature request, proposal, or bug report.
  - Please open up a Github Issue.
- I have a private inquiry.
  - Please reach out at [hi@d2lang.com](hi@d2lang.com).

## Open-source projects documenting with D2

Do you have or see an open-source project with `.d2` files? Please submit a PR adding to
this list (ordered by star count, desc).

- [Block Protocol](https://github.com/blockprotocol/blockprotocol) - The Block Protocol is
  an open standard for building and using data-driven blocks.
- [Ivy Wallet](https://github.com/Ivy-Apps/ivy-wallet) - Ivy Wallet is an open-source
  money manager app for Android.
- [Learn EVM Attacks](https://github.com/coinspect/learn-evm-attacks) - Learn & Contribute
  on previously exploited vulnerabilities across several EVM projects.
- [BYCEPS](https://github.com/byceps/byceps) - BYCEPS is a self-hosted web platform to run
  LAN parties.
- [Re:Earth](https://github.com/reearth/reearth-web) - A free, open and highly extensible
  WebGIS platform.
- [Terraform OCI VSCode Server](https://github.com/timoa/terraform-oci-vscode-server) -
  Terraform project that deploys VSCode Server on Oracle Cloud Infrastructure.
