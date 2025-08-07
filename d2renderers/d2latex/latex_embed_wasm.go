//go:build js && wasm

package d2latex

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"fmt"
	"io"
)

//go:embed polyfills.js
var polyfillsJS string

//go:embed setup.js
var setupJS string

//go:embed mathjax.js.gz
var mathjaxJSGz []byte

var mathjaxJS string

func init() {
	// Decompress MathJax for WASM builds
	var err error
	mathjaxJS, err = decompressGzip(mathjaxJSGz)
	if err != nil {
		panic(fmt.Sprintf("Failed to decompress MathJax: %v", err))
	}
}

// decompressGzip decompresses gzipped data
func decompressGzip(compressed []byte) (string, error) {
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return "", fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer reader.Close()

	decompressed, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("failed to decompress: %w", err)
	}

	return string(decompressed), nil
}