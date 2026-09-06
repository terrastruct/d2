package d2scene

import (
	"image/color"
	"testing"
)

func TestTextRunCarriesFontAndDecoration(t *testing.T) {
	run := TextRun{
		Text:      "link",
		Font:      Font{Family: "D2 Sans", Style: "italic", Weight: 600, Size: 14, Asset: "font"},
		Fallbacks: []AssetID{"fallback"},
		Underline: true,
		Strike:    true,
		Fill:      SolidPaint{Color: color.NRGBA{A: 255}},
		Ink:       NewBounds(1, 2, 11, 14),
	}
	bounds, err := PrimitiveBounds(run, Identity())
	if err != nil {
		t.Fatal(err)
	}
	assertBounds(t, bounds, 1, 2, 11, 14)
}
