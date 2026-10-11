package d2isometricimg

import (
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2scene"
)

func TestNativeSVGMatrixIgnoresRichViewportResiduals(t *testing.T) {
	// Actual ARM64 and amd64 rich-label viewport pairs. Cancellation when
	// centering a label can perturb both a real translation and a nominal zero;
	// LaTeX also exposes these errors in translations smaller than one unit.
	for _, tc := range []struct {
		name string
		a, b d2scene.Matrix
	}{
		{"code", d2scene.Matrix{A: 4.2631578947368425, D: 4.2631578947368425, E: 119.31578947368418, F: -1.0658141036401504e-14}, d2scene.Matrix{A: 4.2631578947368425, D: 4.2631578947368425, E: 119.31578947368419, F: -1.4210854715202004e-14}},
		{"markdown", d2scene.Matrix{A: 6.137254901960785, D: 6.137254901960784, E: 0.8627450980391643, F: -1.021405182655144e-14}, d2scene.Matrix{A: 6.137254901960785, D: 6.137254901960784, E: 0.8627450980391131}},
		{"latex", d2scene.Matrix{A: 0.018097111502225906, D: 0.018097111502225906, E: 0.0038678952272470928, F: 15.996036856817478}, d2scene.Matrix{A: 0.018097111502225906, D: 0.018097111502225906, E: 0.003867895227244844, F: 15.996036856817478}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.a == tc.b {
				t.Fatal("fixture must retain distinct CPU rounding residuals")
			}
			a, b := nativeSVGMatrix(tc.a), nativeSVGMatrix(tc.b)
			if a != b {
				t.Fatalf("equivalent rich viewports differ: %s != %s", a, b)
			}
			parsed := parseNativeSVGTestMatrix(t, a)
			if again := nativeSVGMatrix(parsed); again != a {
				t.Fatalf("matrix precision is not idempotent: %s != %s", a, again)
			}
			for _, point := range []d2scene.Point{{}, {X: 100000, Y: 100000}} {
				before, after := tc.a.Point(point), parsed.Point(point)
				if math.Hypot(before.X-after.X, before.Y-after.Y) > 1e-6 {
					t.Fatal("matrix serialization moved label geometry by more than one millionth of a unit")
				}
			}
		})
	}
}

func TestNativeSVGMatrixPreservesSmallScalesAndLargeTranslations(t *testing.T) {
	input := d2scene.Matrix{A: 1e-100, B: -2e-100, C: 3e-100, D: 4e-100, E: 1e300, F: -1e300}
	output := parseNativeSVGTestMatrix(t, nativeSVGMatrix(input))
	if input != output {
		t.Fatalf("authored transform collapsed or overflowed: %+v != %+v", input, output)
	}
	for _, zero := range []float64{0, math.Copysign(0, -1), 1e-14, -1e-14} {
		input := d2scene.Matrix{A: 1, D: 1, E: zero, F: -zero}
		if got := nativeSVGMatrix(input); got != "matrix(1 0 0 1 0 0)" {
			t.Fatalf("translation residual or negative zero survived serialization: %s", got)
		}
	}
	input = d2scene.Matrix{A: 1, D: 1, E: 5e-8, F: -5e-8}
	if got := parseNativeSVGTestMatrix(t, nativeSVGMatrix(input)); got != input {
		t.Fatalf("small authored translation disappeared: %+v", got)
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
