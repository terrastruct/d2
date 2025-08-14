//go:build js && wasm

package d2latex

import (
	_ "embed"
	"fmt"

	"oss.terrastruct.com/d2/lib/compression"
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
	mathjaxJS, err = compression.DecompressBrotli(mathjaxJSBr)
	if err != nil {
		panic(fmt.Sprintf("Failed to decompress MathJax: %v", err))
	}
}

