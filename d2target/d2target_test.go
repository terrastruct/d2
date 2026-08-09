package d2target

import (
	"bytes"
	"net/url"
	"testing"
)

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
