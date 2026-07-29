//go:build js && wasm

package d2svg

import (
	_ "embed"
	"fmt"

	"oss.terrastruct.com/d2/lib/compression"
)

//go:embed paper.txt.br
var paperBr []byte

var paper string

func init() {
	// Decompress paper texture for WASM builds
	var err error
	paper, err = compression.DecompressBrotli(paperBr)
	if err != nil {
		panic(fmt.Sprintf("Failed to decompress paper texture: %v", err))
	}
}
