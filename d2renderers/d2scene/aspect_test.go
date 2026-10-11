package d2scene

import (
	"math"
	"strings"
	"testing"
)

func TestAspectRatioMatrix(t *testing.T) {
	source := Box{X: 10, Y: 20, Width: 100, Height: 100}
	destination := Box{X: 7, Y: 11, Width: 200, Height: 100}
	tests := []struct {
		name   string
		aspect AspectRatio
		want   Matrix
	}{
		{name: "stretch", aspect: AspectRatio{Align: AlignNone, Fit: AspectSlice}, want: Matrix{A: 2, D: 1, E: -13, F: -9}},
		{name: "meet min", aspect: AspectRatio{Align: AlignXMinYMin, Fit: AspectMeet}, want: Matrix{A: 1, D: 1, E: -3, F: -9}},
		{name: "meet mid", aspect: AspectRatio{Align: AlignXMidYMid, Fit: AspectMeet}, want: Matrix{A: 1, D: 1, E: 47, F: -9}},
		{name: "meet max", aspect: AspectRatio{Align: AlignXMaxYMax, Fit: AspectMeet}, want: Matrix{A: 1, D: 1, E: 97, F: -9}},
		{name: "slice mid", aspect: AspectRatio{Align: AlignXMidYMid, Fit: AspectSlice}, want: Matrix{A: 2, D: 2, E: -13, F: -79}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := AspectRatioMatrix(source, destination, test.aspect)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("AspectRatioMatrix() = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestAspectRatioMatrixZeroDestinationAndInvalidDomain(t *testing.T) {
	matrix, err := AspectRatioMatrix(Box{X: 4, Y: 5, Width: 2, Height: 3}, Box{X: 7, Y: 8}, AspectRatio{Align: AlignXMidYMid})
	if err != nil {
		t.Fatal(err)
	}
	if matrix != (Matrix{E: 7, F: 8}) {
		t.Fatalf("zero destination transform = %+v", matrix)
	}

	tests := []struct {
		name        string
		source      Box
		destination Box
		aspect      AspectRatio
		want        string
	}{
		{name: "empty source", source: Box{Height: 1}, destination: Box{Width: 1, Height: 1}, want: "source box"},
		{name: "negative destination", source: Box{Width: 1, Height: 1}, destination: Box{Width: -1, Height: 1}, want: "destination box"},
		{name: "invalid align", source: Box{Width: 1, Height: 1}, destination: Box{Width: 1, Height: 1}, aspect: AspectRatio{Align: AspectAlign(255)}, want: "invalid aspect-ratio"},
		{name: "invalid fit", source: Box{Width: 1, Height: 1}, destination: Box{Width: 1, Height: 1}, aspect: AspectRatio{Fit: AspectFit(255)}, want: "invalid aspect-ratio"},
		{name: "overflow", source: Box{Width: math.SmallestNonzeroFloat64, Height: 1}, destination: Box{Width: math.MaxFloat64, Height: 1}, want: "non-finite"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := AspectRatioMatrix(test.source, test.destination, test.aspect)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("AspectRatioMatrix() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestAspectRatioMatrixRoundsViewportCancellation(t *testing.T) {
	// These fitted label dimensions exposed different ARM64/amd64 SVG
	// transforms. Pin the separately rounded result; an FMA residual must not
	// survive just because a viewport almost fills its destination.
	for _, tc := range []struct {
		name                string
		source, destination Box
		want                Matrix
	}{
		{"code", Box{Width: 199, Height: 57}, Box{Width: 1087, Height: 243}, Matrix{A: 4.2631578947368425, D: 4.2631578947368425, E: 119.31578947368416, F: -1.4210854715202004e-14}},
		{"markdown", Box{Width: 257, Height: 51}, Box{Width: 1579, Height: 313}, Matrix{A: 6.137254901960785, D: 6.137254901960785, E: 0.8627450980391131}},
		{"multiline", Box{Width: 459, Height: 48}, Box{Width: 1445, Height: 151}, Matrix{A: 3.1458333333333335, D: 3.1458333333333335, E: 0.53125}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := AspectRatioMatrix(tc.source, tc.destination, AspectRatio{Align: AlignXMidYMid})
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("viewport retained a CPU-dependent rounding residual: %+v, want %+v", got, tc.want)
			}
		})
	}
	// Exercise cancellation in the source-origin translation separately from
	// fitting. The explicit FMA proves this fixture catches contraction even
	// when the regression itself runs on a CPU without fused instructions.
	epsilon := math.Ldexp(1, -27)
	scale, origin := 1+epsilon, 1-epsilon
	if math.FMA(-origin, scale, 1) == 0 {
		t.Fatal("fixture no longer distinguishes fused source-origin translation")
	}
	for _, align := range []AspectAlign{AlignNone, AlignXMidYMid} {
		got, err := AspectRatioMatrix(Box{X: origin, Y: origin, Width: 1, Height: 1}, Box{X: 1, Y: 1, Width: scale, Height: scale}, AspectRatio{Align: align})
		if err != nil {
			t.Fatal(err)
		}
		if got != (Matrix{A: scale, D: scale}) {
			t.Fatalf("source-origin translation retained an FMA residual: %+v", got)
		}
	}
}

func TestAspectRatioMatrixPreservesTinySourceOrigins(t *testing.T) {
	got, err := AspectRatioMatrix(Box{X: -4e-13, Y: 4e-13, Width: 1e-12, Height: 1e-12}, Box{Width: 100, Height: 100}, AspectRatio{Align: AlignNone})
	if err != nil {
		t.Fatal(err)
	}
	if got != (Matrix{A: 1e14, D: 1e14, E: 40, F: -40}) {
		t.Fatalf("tiny authored source origin was quantized before scaling: %+v", got)
	}
}
