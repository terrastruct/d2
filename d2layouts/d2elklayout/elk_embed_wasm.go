//go:build js && wasm

package d2elklayout

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"fmt"
	"io"
)

//go:embed elk.js.gz
var elkJSGz []byte

var elkJS string

func init() {
	// Decompress ELK.js for WASM builds
	var err error
	elkJS, err = decompressGzip(elkJSGz)
	if err != nil {
		panic(fmt.Sprintf("Failed to decompress elk.js: %v", err))
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
