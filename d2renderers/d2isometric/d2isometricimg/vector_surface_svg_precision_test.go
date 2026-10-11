package d2isometricimg

import (
	"bytes"
	"context"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2target"
)

func TestNativeSVGMatrixPreservesAuthoredPrecision(t *testing.T) {
	for _, input := range []d2scene.Matrix{
		{A: 1e-100, B: -2e-100, C: 3e-100, D: 4e-100, E: 1e300, F: -1e300},
		{A: 1, D: 1, E: 4e-13, F: -4e-13},
		{A: 1.0000000000001, D: .9999999999999, E: math.Nextafter(1, 2)},
	} {
		serialized := nativeSVGMatrix(input)
		output := parseNativeSVGTestMatrix(t, serialized)
		if input != output {
			t.Fatalf("authored transform changed: %+v != %+v", input, output)
		}
		if again := nativeSVGMatrix(output); again != serialized {
			t.Fatalf("matrix serialization is not idempotent: %s != %s", serialized, again)
		}
	}
	input := d2scene.Matrix{A: 1, D: 1, E: math.Copysign(0, -1)}
	if got := nativeSVGMatrix(input); got != "matrix(1 0 0 1 0 0)" {
		t.Fatalf("negative zero survived serialization: %s", got)
	}
}

func TestNativeSVGPreservesMagnifiedAuthoredIconTranslation(t *testing.T) {
	// The child's tiny translation becomes 40 pixels in this 100-pixel icon
	// after its parent scale. Rounding all matrix translations as if they were
	// generated viewport residuals moves the red rectangle across the face.
	const source = `<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100" viewBox="0 0 100 100"><g transform="scale(1e14)"><rect width="5e-13" height="1e-12" fill="#ff0000" transform="translate(4e-13 0)"/></g></svg>`
	d := d2target.NewDiagram()
	s := *d2target.BaseShape()
	s.ID, s.Type, s.Width, s.Height = "artwork", d2target.ShapeImage, 100, 100
	s.Fill, s.Stroke, s.StrokeWidth = "transparent", "none", 0
	s.Icon = iconData(t, "image/svg+xml", []byte(source))
	d.Shapes = []d2target.Shape{s}
	data, err := Render(context.Background(), d, &Options{Format: SVG, Width: 500, Height: 400, FitContent: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, transform := range []string{"matrix(1e+14 0 0 1e+14 0 0)", "matrix(1 0 0 1 4e-13 0)"} {
		if !bytes.Contains(data, []byte(transform)) {
			t.Fatalf("authored SVG transform was lost in public image rendering: %s", transform)
		}
	}
}

func parseNativeSVGTestMatrix(t *testing.T, value string) d2scene.Matrix {
	t.Helper()
	parts := strings.Fields(strings.TrimSuffix(strings.TrimPrefix(value, "matrix("), ")"))
	if len(parts) != 6 {
		t.Fatalf("invalid serialized matrix: %s", value)
	}
	var values [6]float64
	for i, part := range parts {
		parsed, err := strconv.ParseFloat(part, 64)
		if err != nil {
			t.Fatal(err)
		}
		values[i] = parsed
	}
	return d2scene.Matrix{A: values[0], B: values[1], C: values[2], D: values[3], E: values[4], F: values[5]}
}
