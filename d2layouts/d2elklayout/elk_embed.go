//go:build !js || !wasm

package d2elklayout

import (
	_ "embed"
)

//go:embed elk.js
var elkJS string
