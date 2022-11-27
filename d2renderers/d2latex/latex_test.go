package d2latex

import (
	"encoding/xml"
	"testing"
)

func TestSVG(t *testing.T) {
	svg, err := SVG("$$a + b = c$$")
	if err != nil {
		t.Fatal(err)
	}
	var xmlParsed interface{}
	if err := xml.Unmarshal([]byte(svg), &xmlParsed); err != nil {
		t.Fatalf("invalid SVG: %v", err)
	}
}
