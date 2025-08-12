# D2.js

[![npm version](https://badge.fury.io/js/%40terrastruct%2Fd2.svg)](https://www.npmjs.com/package/@terrastruct/d2)
[![License: MPL-2.0](https://img.shields.io/badge/License-MPL_2.0-brightgreen.svg)](https://mozilla.org/MPL/2.0/)

D2.js is a JavaScript wrapper around D2, the modern diagram scripting language. It enables running D2 directly in browsers and Node environments through WebAssembly.

## Features

- 🌐 **Universal** - Works in both browser and Node environments
- 🚀 **Modern** - Built with ESM modules, with CJS fallback
- 🔄 **Isomorphic** - Same API everywhere
- ⚡ **Fast** - Powered by WebAssembly for near-native performance
- 📦 **Lightweight** - Minimal wrapper around the core D2 engine

## Installation

```bash
# npm
npm install @terrastruct/d2

# yarn
yarn add @terrastruct/d2

# pnpm
pnpm add @terrastruct/d2

# bun
bun add @terrastruct/d2
```

### Nightly

Use the `@nightly` tag to get the version that is built by daily CI on the master branch.

For example,

```bash
yarn add @terrastruct/d2@nightly
```

A demo using the nightly build is hosted [here](https://alixander-d2js.web.val.run/).

## Usage

D2.js uses webworkers to call a WASM file.

### Basic Usage

```javascript
// Same for Node or browser
import { D2 } from '@terrastruct/d2';
// Or using a CDN
// import { D2 } from 'https://esm.sh/@terrastruct/d2';

const d2 = new D2();

const result = await d2.compile('x -> y');
const svg = await d2.render(result.diagram, result.renderOptions);
```

Configuring render options (see [CompileOptions](#compileoptions) for all available options):

```javascript
import { D2 } from '@terrastruct/d2';

const d2 = new D2();

const result = await d2.compile('x -> y', {
    sketch: true,
});
const svg = await d2.render(result.diagram, result.renderOptions);
```

### Imports

In order to support [imports](https://d2lang.com/tour/imports), a mapping of D2 file paths to their content can be passed to the compiler.

```javascript
import { D2 } from '@terrastruct/d2';

const d2 = new D2();

const fs = {
  "project.d2": "a: @import",
  "import.d2": "x: {shape: circle}",
}

const result = await d2.compile({
    fs,
    inputPath: "project.d2",
    options: {
        sketch: true
    }
});
const svg = await d2.render(result.diagram, result.renderOptions);
```

## API Reference

### `new D2()`

Creates a new D2 instance.

### `compile(input: string | CompileRequest, options?: CompileOptions): Promise<CompileResult>`

Compiles D2 markup into an intermediate representation. It compile options are provided in both `input` and `options`, the latter will take precedence.

### `render(diagram: Diagram, options?: RenderOptions): Promise<string>`

Renders a compiled diagram to SVG.

### `CompileOptions`

All [RenderOptions](#renderoptions) properties in addition to:

- `layout`: Layout engine to use ('dagre' | 'elk') [default: 'dagre']
- `fontRegular` A byte array containing .ttf file to use for the regular font. If none provided, Source Sans Pro Regular is used.
- `fontItalic` A byte array containing .ttf file to use for the italic font. If none provided, Source Sans Pro Italic is used.
- `fontBold` A byte array containing .ttf file to use for the bold font. If none provided, Source Sans Pro Bold is used.
- `fontSemibold` A byte array containing .ttf file to use for the semibold font. If none provided, Source Sans Pro Semibold is used.

### `RenderOptions`

- `sketch`: Enable sketch mode [default: false]
- `themeID`: Theme ID to use [default: 0]
- `darkThemeID`: Theme ID to use when client is in dark mode
- `center`: Center the SVG in the containing viewbox [default: false]
- `pad`: Pixels padded around the rendered diagram [default: 100]
- `scale`: Scale the output. E.g., 0.5 to halve the default size. The default will render SVG's that will fit to screen. Setting to 1 turns off SVG fitting to screen.
- `forceAppendix`: Adds an appendix for tooltips and links [default: false]
- `target`: Target board/s to render. If target ends with '*', it will be rendered with all of its scenarios, steps, and layers. Otherwise, only the target board will be rendered. E.g. `target: 'layers.x.*'` to render layer 'x' with all of its children. Pass '*' to render all scenarios, steps, and layers. By default, only the root board is rendered. Multi-board outputs are currently only supported for animated SVGs and so `animateInterval` must be set to a value greater than 0 when targeting multiple boards.
- `animateInterval`: If given, multiple boards are packaged as 1 SVG which transitions through each board at the interval (in milliseconds).
- `salt`: Add a salt value to ensure the output uses unique IDs. This is useful when generating multiple identical diagrams to be included in the same HTML doc, so that duplicate IDs do not cause invalid HTML. The salt value is a string that will be appended to IDs in the output.
- `noXMLTag`: Omit XML tag `(<?xml ...?>)` from output SVG files. Useful when generating SVGs for direct HTML embedding.

### `CompileRequest`

- `fs`: A mapping of D2 file paths to their content
- `inputPath`: The path of the entry D2 file [default: index]
- `options`: The [CompileOptions](#compileoptions) to pass to the compiler

### `CompileResult`

- `diagram`: `Diagram`: Compiled D2 diagram
- `options`: `RenderOptions`: Render options merged with configuration set in diagram
- `fs`
- `graph`

## Development

D2.js uses Bun, so install this first.

For optimal WASM file size, also install binaryen:
```bash
# macOS
brew install binaryen

# Ubuntu/Debian
sudo apt-get install binaryen
```

### Building from source

```bash
git clone https://github.com/terrastruct/d2.git
cd d2/d2js/js
./make.sh all
```

If you change the main D2 source code, you should regenerate the WASM file:
```bash
./make.sh build
```

### Running the dev server

You can browse the examples by running the dev server:

```bash
./make.sh dev
```

Visit `http://localhost:3000` to see the example page.

### Publishing

TODO stable release publishing.

Nightly builds are automated by CI by running:

```bash
PUBLISH=1 ./make.sh build
```

## Contributing

Contributions are welcome!

## License

This project is licensed under the Mozilla Public License Version 2.0.
