//go:build !js || !wasm

package d2latex

import (
	_ "embed"
)

//go:embed polyfills.js
var polyfillsJS string

//go:embed setup.js
var setupJS string

//go:embed mathjax.js
var mathjaxJS string
