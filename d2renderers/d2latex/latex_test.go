package d2latex

import (
	"encoding/xml"
	"testing"
)

func TestRender(t *testing.T) {
	txts := []string{
		`a + b = c`,
		`\\frac{1}{2}`,
	}
	for _, txt := range txts {
		svg, err := Render(txt)
		if err != nil {
			t.Fatal(err)
		}
		var xmlParsed interface{}
		if err := xml.Unmarshal([]byte(svg), &xmlParsed); err != nil {
			t.Fatalf("invalid SVG: %v", err)
		}
	}
}

func TestRenderError(t *testing.T) {
	_, err := Render(`\frac{1}{2}`)
	if err == nil {
		t.Fatal("expected to error on invalid latex syntax")
	}
}
