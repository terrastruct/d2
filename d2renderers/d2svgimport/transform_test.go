package d2svgimport

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2scene"
)

var generousTransformLimits = transformLimits{MaxBytes: 1 << 20, MaxFunctions: 1000}

func TestParseTransformCompleteGrammarAndSourceOrder(t *testing.T) {
	t.Parallel()

	got, _, err := parseTransformWithCount(context.Background(), "icon.svg", "translate(10 20),scale(2,-3) rotate(90) skewX(45) skewY(-45) matrix(1 2 3 4 5 6)", generousTransformLimits)
	if err != nil {
		t.Fatal(err)
	}
	want := d2scene.Translate(10, 20).
		Mul(d2scene.Scale(2, -3)).
		Mul(d2scene.Rotate(math.Pi / 2)).
		Mul(d2scene.SkewX(math.Pi / 4)).
		Mul(d2scene.SkewY(-math.Pi / 4)).
		Mul(d2scene.Matrix{A: 1, B: 2, C: 3, D: 4, E: 5, F: 6})
	assertMatrixNear(t, got, want, 1e-12)

	// SVG applies the right-most point operation first: scale then translate.
	point := d2scene.Point{X: 1, Y: 2}
	ordered, _, err := parseTransformWithCount(context.Background(), "order", "translate(10 20) scale(2)", generousTransformLimits)
	if err != nil {
		t.Fatal(err)
	}
	if got := ordered.Point(point); got != (d2scene.Point{X: 12, Y: 24}) {
		t.Fatalf("source-order point = %+v, want (12,24)", got)
	}
}

func TestParseTransformDefaultsAndRotateAround(t *testing.T) {
	t.Parallel()

	tests := []struct {
		value string
		want  d2scene.Matrix
	}{
		{"", d2scene.Identity()},
		{" \n\t", d2scene.Identity()},
		{"translate(7)", d2scene.Translate(7, 0)},
		{"scale(3)", d2scene.Scale(3, 3)},
		{"rotate(180, 4, 5)", d2scene.RotateAround(math.Pi, 4, 5)},
		{"matrix(.5,-.25,1e1,2E-1,3,4)", d2scene.Matrix{A: .5, B: -.25, C: 10, D: .2, E: 3, F: 4}},
	}
	for _, test := range tests {
		got, _, err := parseTransformWithCount(context.Background(), "defaults", test.value, generousTransformLimits)
		if err != nil {
			t.Fatalf("ParseTransform(%q): %v", test.value, err)
		}
		assertMatrixNear(t, got, test.want, 1e-12)
	}
}

func TestParseTransformRejectsMalformedAndUnboundedValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		value  string
		limits transformLimits
		want   string
	}{
		{"unknown", "warp(1)", generousTransformLimits, "unsupported"},
		{"case sensitive", "SCALE(2)", generousTransformLimits, "unsupported"},
		{"missing open", "scale 2)", generousTransformLimits, "missing"},
		{"missing close", "scale(2", generousTransformLimits, "unterminated"},
		{"empty first", "scale(,2)", generousTransformLimits, "empty first"},
		{"empty middle", "scale(2,,3)", generousTransformLimits, "empty argument"},
		{"empty last", "scale(2,)", generousTransformLimits, "empty argument"},
		{"unseparated argument", "translate(1.2.3)", generousTransformLimits, "must be separated"},
		{"unseparated functions", "scale(2)rotate(3)", generousTransformLimits, "must be separated"},
		{"wrong matrix arity", "matrix(1 2 3)", generousTransformLimits, "expected 6"},
		{"wrong rotate arity", "rotate(1 2)", generousTransformLimits, "expected 1 or 3"},
		{"bad exponent", "translate(1e)", generousTransformLimits, "exponent"},
		{"overflow", "translate(1e999)", generousTransformLimits, "finite"},
		{"trailing comma", "scale(2),", generousTransformLimits, "empty function"},
		{"function limit", "scale(1) scale(2)", transformLimits{MaxBytes: 100, MaxFunctions: 1}, "function count"},
		{"argument limit", "matrix(1 2 3 4 5 6 7)", generousTransformLimits, "more than 6"},
		{"byte limit", "scale(1)", transformLimits{MaxBytes: 3, MaxFunctions: 1}, "exceeding limit"},
		{"zero limit", "", transformLimits{MaxBytes: 1}, "limits must be positive"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, _, err := parseTransformWithCount(context.Background(), "safe.svg", test.value, test.limits)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ParseTransform() = %+v, %v; want %q", got, err, test.want)
			}
			if got != (d2scene.Matrix{}) {
				t.Fatalf("failed parse returned partial transform: %+v", got)
			}
		})
	}
}

func TestParseTransformCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, _, err := parseTransformWithCount(ctx, "cancelled.svg", "scale(2)", generousTransformLimits)
	if got != (d2scene.Matrix{}) || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled ParseTransform() = %+v, %v", got, err)
	}
}

func FuzzParseTransform(f *testing.F) {
	for _, seed := range []string{"", "translate(1 2) scale(3)", "rotate(45 10 20)", "matrix(1,0,0,1,2,3)", "scale(,,)"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, value string) {
		if len(value) > 4096 {
			t.Skip()
		}
		matrix, _, err := parseTransformWithCount(context.Background(), "fuzz.svg", value, transformLimits{MaxBytes: 4096, MaxFunctions: 128})
		if err == nil && !matrix.IsFinite() {
			t.Fatalf("successful parse returned non-finite matrix: %+v", matrix)
		}
	})
}

func assertMatrixNear(t *testing.T, got, want d2scene.Matrix, tolerance float64) {
	t.Helper()
	gotValues := [...]float64{got.A, got.B, got.C, got.D, got.E, got.F}
	wantValues := [...]float64{want.A, want.B, want.C, want.D, want.E, want.F}
	for index := range gotValues {
		if math.Abs(gotValues[index]-wantValues[index]) > tolerance {
			t.Fatalf("matrix = %+v, want %+v (component %d)", got, want, index)
		}
	}
}
