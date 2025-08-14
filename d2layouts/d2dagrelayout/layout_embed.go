//go:build !js || !wasm

package d2dagrelayout

import (
	_ "embed"
)

//go:embed dagre.js
var dagreJS string