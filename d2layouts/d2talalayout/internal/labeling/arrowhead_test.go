package labeling

import (
	"testing"

	"github.com/d2lang/d2/lib/geo"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
)

func TestPositionArrowheadLabelOwnsRenderedRecord(t *testing.T) {
	edge := layoutgraph.NewEdge(nil, nil)
	edge.Points = geo.Route{geo.NewPoint(0, 0), geo.NewPoint(100, 0)}
	edge.SourceArrowhead = layoutgraph.TriangleArrowhead
	edge.TargetArrowhead = layoutgraph.TriangleArrowhead
	edge.SourceArrowheadLabel = &layoutgraph.Label{Text: "source", Width: 30, Height: 12}
	edge.TargetArrowheadLabel = &layoutgraph.Label{Text: "target", Width: 40, Height: 14}

	for _, test := range []struct {
		name     string
		isTarget bool
		text     string
		width    float64
		height   float64
		want     *geo.Point
	}{
		{name: "source", text: "source", width: 30, height: 12, want: geo.NewPoint(5, -20)},
		{name: "target", isTarget: true, text: "target", width: 40, height: 14, want: geo.NewPoint(55, -22)},
	} {
		t.Run(test.name, func(t *testing.T) {
			positioned := PositionArrowheadLabel(edge, test.isTarget, edge.Points)
			if positioned == nil {
				t.Fatal("positioned label is nil")
			}
			if positioned.Edge != edge || positioned.IsTarget != test.isTarget || positioned.Text != test.text {
				t.Fatalf("positioned metadata = %#v", positioned)
			}
			if positioned.Width != test.width || positioned.Height != test.height {
				t.Fatalf("positioned dimensions = %v x %v, want %v x %v", positioned.Width, positioned.Height, test.width, test.height)
			}
			if !positioned.TopLeft.Equals(test.want) {
				t.Fatalf("top-left = %v, want %v", positioned.TopLeft, test.want)
			}
		})
	}

	if positioned := PositionArrowheadLabel(layoutgraph.NewEdge(nil, nil), false, edge.Points); positioned != nil {
		t.Fatalf("unlabeled endpoint produced %#v", positioned)
	}
	if positioned := PositionArrowheadLabel(nil, false, nil); positioned != nil {
		t.Fatalf("nil edge produced %#v", positioned)
	}
}
