//go:build !js && !wasm

package d2typst

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// render executes typst CLI to convert Typst markup to SVG
// This implementation is used for native builds (non-WASM)
func render(s string) (string, error) {
	// Check if typst CLI is available
	if _, err := exec.LookPath("typst"); err != nil {
		return "", fmt.Errorf("typst CLI not found in PATH. Please install typst from https://typst.app/docs/installation/")
	}

	// Create command: typst compile --format svg - -
	// First "-" reads from stdin, second "-" writes to stdout
	cmd := exec.Command("typst", "compile", "--format", "svg", "-", "-")

	// Provide Typst markup via stdin
	cmd.Stdin = strings.NewReader(s)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute command
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("typst compilation failed: %w\nstderr: %s", err, stderr.String())
	}

	svg := stdout.String()

	// Validate output is SVG
	if !strings.Contains(svg, "<svg") {
		return "", fmt.Errorf("typst produced invalid SVG output: %s", svg)
	}

	return svg, nil
}
