package labelgeom

import (
	"testing"

	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/geo"
)

func TestArrowheadTopLeftPreservesHistoricalTruncation(t *testing.T) {
	route := geo.Route{geo.NewPoint(0, 0), geo.NewPoint(100, 0)}
	got := ArrowheadTopLeft(route, false, "triangle", "none", 30.9, 12.9)
	want := geo.NewPoint(5, -20)
	if !got.Equals(want) {
		t.Fatalf("top-left = %v, want %v", got, want)
	}
}

func TestArrowheadTopLeftMatchesRendererGeometry(t *testing.T) {
	tests := []struct {
		name                     string
		route                    geo.Route
		isTarget                 bool
		sourceArrow, targetArrow string
		width, height            float64
	}{
		{
			name:        "horizontal source",
			route:       geo.Route{geo.NewPoint(0, 0), geo.NewPoint(100, 0)},
			sourceArrow: "triangle",
			targetArrow: "none",
			width:       30.9,
			height:      12.9,
		},
		{
			name:        "horizontal target",
			route:       geo.Route{geo.NewPoint(0, 0), geo.NewPoint(100, 0)},
			isTarget:    true,
			sourceArrow: "none",
			targetArrow: "diamond",
			width:       40,
			height:      14,
		},
		{
			name:        "vertical target",
			route:       geo.Route{geo.NewPoint(0, 0), geo.NewPoint(0, 100)},
			isTarget:    true,
			sourceArrow: "circle",
			targetArrow: "triangle",
			width:       21.25,
			height:      11.75,
		},
		{
			name: "multi-segment target",
			route: geo.Route{
				geo.NewPoint(0, 0),
				geo.NewPoint(45, 0),
				geo.NewPoint(45, 70),
			},
			isTarget:    true,
			sourceArrow: "none",
			targetArrow: "cf-many-required",
			width:       33.8,
			height:      15.2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := ArrowheadTopLeft(
				test.route,
				test.isTarget,
				test.sourceArrow,
				test.targetArrow,
				test.width,
				test.height,
			)
			want := rendererArrowheadTopLeft(
				test.route,
				test.isTarget,
				test.sourceArrow,
				test.targetArrow,
				test.width,
				test.height,
			)
			if !got.Equals(want) {
				t.Fatalf("top-left = %v, renderer = %v", got, want)
			}
		})
	}
}

func rendererArrowheadTopLeft(
	route geo.Route,
	isTarget bool,
	sourceArrowhead, targetArrowhead string,
	width, height float64,
) *geo.Point {
	connection := d2target.BaseConnection()
	connection.Route = route
	connection.SrcArrow = d2target.Arrowhead(sourceArrowhead)
	connection.DstArrow = d2target.Arrowhead(targetArrowhead)
	text := &d2target.Text{LabelWidth: int(width), LabelHeight: int(height)}
	if isTarget {
		connection.DstLabel = text
	} else {
		connection.SrcLabel = text
	}
	return connection.GetArrowheadLabelPosition(isTarget)
}
