//go:build js && wasm

package d2dagrelayout

import (
	"bytes"
	_ "embed"
	"fmt"
	"io"

	"github.com/andybalholm/brotli"
)

//go:embed dagre.js.br
var dagreJSBr []byte

var dagreJS string

func init() {
	// Decompress Dagre JS for WASM builds
	var err error
	dagreJS, err = decompressBrotli(dagreJSBr)
	if err != nil {
		panic(fmt.Sprintf("Failed to decompress Dagre JS: %v", err))
	}
}

// decompressBrotli decompresses brotli compressed data
func decompressBrotli(compressed []byte) (string, error) {
	reader := brotli.NewReader(bytes.NewReader(compressed))

	decompressed, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("failed to decompress: %w", err)
	}

	return string(decompressed), nil
}
