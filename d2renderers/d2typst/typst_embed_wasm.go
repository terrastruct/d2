//go:build js && wasm

package d2typst

import (
	_ "embed"
	"fmt"

	"oss.terrastruct.com/d2/lib/jsrunner"
)

//go:embed typst.bundle.js
var typstJS string

//go:embed setup.js
var setupJS string

// render executes typst.ts in browser JavaScript runtime to convert Typst markup to SVG
// This implementation is used for WASM builds (browser environment)
func render(s string) (string, error) {
	runner := jsrunner.NewJSRunner()

	// Load typst.ts bundle (includes compiler and renderer interfaces)
	if _, err := runner.RunString(typstJS); err != nil {
		return "", fmt.Errorf("failed to load typst.ts: %w", err)
	}

	// Initialize typst.ts with WASM module locations
	if _, err := runner.RunString(setupJS); err != nil {
		return "", fmt.Errorf("failed to initialize typst.ts: %w", err)
	}

	// Call $typst.svg() to render Typst markup to SVG
	// Note: This is async in the browser, but jsrunner handles promises
	renderCode := fmt.Sprintf(`
		(async () => {
			try {
				const svg = await $typst.svg({ mainContent: %q });
				return svg;
			} catch (error) {
				throw new Error('Typst rendering failed: ' + error.message);
			}
		})()
	`, s)

	val, err := runner.RunString(renderCode)
	if err != nil {
		return "", fmt.Errorf("typst rendering failed: %w", err)
	}

	// Wait for promise to resolve
	result, err := runner.WaitPromise(nil, val)
	if err != nil {
		return "", fmt.Errorf("typst promise failed: %w", err)
	}

	svg, ok := result.(string)
	if !ok {
		return "", fmt.Errorf("typst returned non-string result: %T", result)
	}

	return svg, nil
}
