//go:build js && wasm

package d2elklayout

import (
	_ "embed"
)

// ELK is loaded in JS instances in the JS environments natively, not through WASM
var elkJS string
