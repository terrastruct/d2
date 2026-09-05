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
