//go:build js && wasm

package d2latex

import (
	"bytes"
	_ "embed"
	"fmt"
	"io"

	"github.com/andybalholm/brotli"
)

//go:embed polyfills.js
var polyfillsJS string

//go:embed setup.js
var setupJS string

//go:embed mathjax.js.br
var mathjaxJSBr []byte

var mathjaxJS string

func init() {
	// Decompress MathJax for WASM builds
	var err error
	mathjaxJS, err = decompressBrotli(mathjaxJSBr)
	if err != nil {
		panic(fmt.Sprintf("Failed to decompress MathJax: %v", err))
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
