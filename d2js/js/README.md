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

## Usage

D2.js uses webworkers to call a WASM file.

```javascript
// Same for Node or browser
import { D2 } from '@terrastruct/d2';
// Or using a CDN
// import { D2 } from 'https://esm.sh/@terrastruct/d2';

const d2 = new D2();

const result = await d2.compile('x -> y');
const svg = await d2.render(result.diagram);
```

## API Reference

### `new D2()`
Creates a new D2 instance.

### `compile(input: string, options?: CompileOptions): Promise<CompileResult>`
Compiles D2 markup into an intermediate representation.

Options:
- `layout`: Layout engine to use ('dagre' | 'elk') [default: 'dagre']
- `sketch`: Enable sketch mode [default: false]

### `render(diagram: Diagram, options?: RenderOptions): Promise<string>`
Renders a compiled diagram to SVG.

## Development

D2.js uses Bun, so install this first.

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

## Contributing

Contributions are welcome!

## License

This project is licensed under the Mozilla Public License Version 2.0.
