package d2typst

import (
	"encoding/xml"
	"strings"
	"testing"
)

func TestRender(t *testing.T) {
	// Skip if typst CLI not available (for CI without typst installed)
	// WASM builds will use different implementation

	txts := []string{
		`Hello, Typst!`,
		`$a + b = c$`,
		`#set page(width: 10cm, height: auto)
$integral x dif x = x^2 / 2$`,
		`*Bold* and _italic_`,
	}
	for _, txt := range txts {
		svg, err := Render(txt)
		if err != nil {
			// Skip test if typst CLI not found (expected in some CI environments)
			if strings.Contains(err.Error(), "typst CLI not found") {
				t.Skip("typst CLI not available, skipping test")
				return
			}
			t.Fatal(err)
		}
		// Verify output is valid SVG
		var xmlParsed interface{}
		if err := xml.Unmarshal([]byte(svg), &xmlParsed); err != nil {
			t.Fatalf("invalid SVG: %v\nSVG output: %s", err, svg)
		}
		// Verify output contains SVG tag
		if !strings.Contains(svg, "<svg") {
			t.Fatalf("output does not contain SVG tag: %s", svg)
		}
	}
}

func TestMeasure(t *testing.T) {
	txt := `Hello, Typst!`
	width, height, err := Measure(txt)
	if err != nil {
		// Skip test if typst CLI not found
		if strings.Contains(err.Error(), "typst CLI not found") {
			t.Skip("typst CLI not available, skipping test")
			return
		}
		t.Fatal(err)
	}

	// Verify dimensions are reasonable
	if width <= 0 || height <= 0 {
		t.Fatalf("invalid dimensions: width=%d, height=%d", width, height)
	}

	// Basic sanity check: text should be wider than it is tall
	if width < height {
		t.Logf("Warning: width (%d) < height (%d), may indicate parsing issue", width, height)
	}
}

func TestRenderMultiline(t *testing.T) {
	txt := `#set page(width: 10cm, height: auto)
Line 1

Line 2`
	svg, err := Render(txt)
	if err != nil {
		if strings.Contains(err.Error(), "typst CLI not found") {
			t.Skip("typst CLI not available, skipping test")
			return
		}
		t.Fatal(err)
	}

	var xmlParsed interface{}
	if err := xml.Unmarshal([]byte(svg), &xmlParsed); err != nil {
		t.Fatalf("invalid SVG for multiline text: %v", err)
	}
}
