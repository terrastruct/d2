package layoutgraph

import (
	"testing"

	"github.com/d2lang/d2/lib/geo"
)

func TestCountSegmentCrossingsEndpointContract(t *testing.T) {
	tests := []struct {
		name     string
		segments []CrossingSegment
		want     int64
	}{
		{
			name: "ordinary crossing",
			segments: []CrossingSegment{
				{Start: geo.Point{X: 0, Y: 0}, End: geo.Point{X: 2, Y: 2}},
				{Start: geo.Point{X: 0, Y: 2}, End: geo.Point{X: 2, Y: 0}},
			},
			want: 1,
		},
		{
			name: "equal starts",
			segments: []CrossingSegment{
				{Start: geo.Point{X: 0, Y: 0}, End: geo.Point{X: 2, Y: 2}},
				{Start: geo.Point{X: 0, Y: 0}, End: geo.Point{X: 2, Y: 0}},
			},
		},
		{
			name: "equal ends",
			segments: []CrossingSegment{
				{Start: geo.Point{X: 0, Y: 0}, End: geo.Point{X: 2, Y: 2}},
				{Start: geo.Point{X: 0, Y: 2}, End: geo.Point{X: 2, Y: 2}},
			},
		},
		{
			name: "end touches start",
			segments: []CrossingSegment{
				{Start: geo.Point{X: 0, Y: 0}, End: geo.Point{X: 2, Y: 2}},
				{Start: geo.Point{X: 2, Y: 2}, End: geo.Point{X: 4, Y: 0}},
			},
			want: 1,
		},
		{
			name: "parallel overlap",
			segments: []CrossingSegment{
				{Start: geo.Point{X: 0, Y: 0}, End: geo.Point{X: 2, Y: 0}},
				{Start: geo.Point{X: 1, Y: 0}, End: geo.Point{X: 3, Y: 0}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got int64 = CountSegmentCrossings(tt.segments)
			if got != tt.want {
				t.Fatalf("crossing count = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCountSegmentCrossingsContextWrapsCancellation(t *testing.T) {
	crossings, err := CountSegmentCrossingsContext(canceledContext(), []CrossingSegment{
		{Start: geo.Point{X: 0, Y: 0}, End: geo.Point{X: 2, Y: 2}},
		{Start: geo.Point{X: 0, Y: 2}, End: geo.Point{X: 2, Y: 0}},
	})
	if crossings != 0 {
		t.Fatalf("crossing count = %d, want 0", crossings)
	}
	requireCanceledAt(t, err, "EdgeLength")
}
