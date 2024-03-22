# D2 as a Javascript library

D2 is runnable as a Javascript library, on both the client and server side. This means you
can run D2 entirely on the browser.

This is achieved by a JS wrapper around a WASM file.

## Install

### NPM

```sh
npm install @terrastruct/d2
```

### Yarn

```sh
yarn add @terrastruct/d2
```

## Build

```sh
GOOS=js GOARCH=wasm go build -ldflags='-s -w' -trimpath -o main.wasm ./d2js
```

## API

todo
