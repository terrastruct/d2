//go:build js && wasm

package d2svg

import (
	"bytes"
	_ "embed"
	"fmt"
	"io"

	"github.com/andybalholm/brotli"
)

//go:embed paper.txt.br
var paperBr []byte

var paper string

func init() {
	// Decompress paper texture for WASM builds
	var err error
	paper, err = decompressBrotli(paperBr)
	if err != nil {
		panic(fmt.Sprintf("Failed to decompress paper texture: %v", err))
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