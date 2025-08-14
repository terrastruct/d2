//go:build js && wasm

package d2dagrelayout

import (
	_ "embed"
	"fmt"

	"oss.terrastruct.com/d2/lib/compression"
)

//go:embed dagre.js.br
var dagreJSBr []byte

var dagreJS string

func init() {
	// Decompress Dagre JS for WASM builds
	var err error
	dagreJS, err = compression.DecompressBrotli(dagreJSBr)
	if err != nil {
		panic(fmt.Sprintf("Failed to decompress Dagre JS: %v", err))
	}
}

