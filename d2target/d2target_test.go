package d2target

import (
	"bytes"
	"net/url"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/lib/textmeasure"
)

func TestCalculateTooltipBoundsUsesDiagramFonts(t *testing.T) {
	fontFamily := d2fonts.HandDrawn
	monoFontFamily := d2fonts.SourceCodePro
	shape := Shape{
		Pos:             Point{X: 100, Y: 200},
		Width:           120,
		Height:          60,
		Tooltip:         "**wide handwritten tooltip**",
		TooltipPosition: "top-left",
		Text:            Text{FontFamily: "DEFAULT"},
	}

	minX, minY, maxX, maxY := calculateTooltipBounds(shape, &fontFamily, &monoFontFamily)
	ruler, err := textmeasure.NewRuler()
	if err != nil {
		t.Fatal(err)
	}
	width, height, err := textmeasure.MeasureMarkdown(shape.Tooltip, ruler, &fontFamily, &monoFontFamily, d2fonts.FONT_SIZE_M)
	if err != nil {
		t.Fatal(err)
	}
	if minX != shape.Pos.X || maxX-minX != width+20 {
		t.Fatalf("tooltip horizontal bounds = (%d,%d), want x=%d width=%d", minX, maxX, shape.Pos.X, width+20)
	}
	wantY := shape.Pos.Y - (height + 20) - 10
	if minY != wantY || maxY-minY != height+20 {
		t.Fatalf("tooltip vertical bounds = (%d,%d), want y=%d height=%d", minY, maxY, wantY, height+20)
	}
}

func TestHashIDStableAcrossURLFieldOrder(t *testing.T) {
	icon := &url.URL{
		Scheme:      "https",
		Opaque:      "opaque",
		User:        url.UserPassword("user", "password"),
		Host:        "example.com",
		Path:        "/icon.svg",
		RawPath:     "/icon%2Esvg",
		OmitHost:    true,
		ForceQuery:  true,
		RawQuery:    "x=1",
		Fragment:    "fragment",
		RawFragment: "frag%6Dent",
	}
	d := Diagram{
		Shapes: []Shape{{
			ID:   "shape",
			Icon: icon,
			Text: Text{Label: `quoted "icon":{"Scheme":"fake"}`},
		}},
		Connections: []Connection{{ID: "connection", Icon: icon}},
		Root:        Shape{ID: "root", Icon: icon},
		Layers: []*Diagram{{
			Shapes: []Shape{{ID: "layer-shape", Icon: icon}},
		}},
		Scenarios: []*Diagram{{
			Connections: []Connection{{ID: "scenario-connection", Icon: icon}},
		}},
		Steps: []*Diagram{{
			Root: Shape{ID: "step-root", Icon: icon},
		}},
	}

	b, err := d.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	legacyURLTail := []byte(`"Path":"/icon.svg","RawPath":"/icon%2Esvg","OmitHost":true,"ForceQuery":true,"RawQuery":"x=1","Fragment":"fragment","RawFragment":"frag%6Dent"`)
	if got, want := bytes.Count(b, legacyURLTail), 6; got != want {
		t.Fatalf("legacy URL field sequence count = %d, want %d", got, want)
	}

	got, err := d.HashID(nil)
	if err != nil {
		t.Fatal(err)
	}
	// This is the hash produced by Go 1.25.12 before net/url.URL fields were
	// reorganized in Go 1.26.
	const want = "d2-887368360"
	if got != want {
		t.Fatalf("HashID = %q, want %q", got, want)
	}
}
