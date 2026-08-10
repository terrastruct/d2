package color

import (
	"encoding/xml"
	"strings"
	"testing"
)

func TestOneStopGradientToSVG(t *testing.T) {
	tests := []string{
		"linear-gradient(red)",
		"radial-gradient(red)",
	}

	for _, cssGradient := range tests {
		t.Run(cssGradient, func(t *testing.T) {
			gradient, err := ParseGradient(cssGradient)
			if err != nil {
				t.Fatalf("ParseGradient() error = %v", err)
			}

			svg := GradientToSVG(gradient)
			if strings.Contains(svg, "NaN") || strings.Contains(svg, "Inf") {
				t.Fatalf("GradientToSVG() produced a non-finite offset:\n%s", svg)
			}

			var parsed struct {
				Stops []struct {
					Offset string `xml:"offset,attr"`
				} `xml:"stop"`
			}
			if err := xml.Unmarshal([]byte(svg), &parsed); err != nil {
				t.Fatalf("GradientToSVG() produced invalid XML: %v\n%s", err, svg)
			}
			if len(parsed.Stops) != 1 {
				t.Fatalf("GradientToSVG() produced %d stops, want 1", len(parsed.Stops))
			}
			if got, want := parsed.Stops[0].Offset, "0.00%"; got != want {
				t.Fatalf("stop offset = %q, want %q", got, want)
			}
		})
	}
}
