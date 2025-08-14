//go:build !js || !wasm

package d2svg

import (
	_ "embed"
)

//go:embed paper.txt
var paper string