package packing

import (
	"context"
	"math"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/shape"
)

func routedContainerTestGuard(t *testing.T) *limits.WorkGuard {
	t.Helper()
	guard, err := newWorkGuard(context.Background(), 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	return guard
}

func routedContainerTestGraph(shapeType string) (*layoutgraph.Graph, *layoutgraph.Node, *layoutgraph.Node) {
	graph := layoutgraph.NewGraph()
	container := layoutgraph.NewNode(1, 300, 300)
	container.SetShape(shapeType)
	container.TopLeft = geo.NewPoint(0, 0)
	child := layoutgraph.NewNode(2, 20, 20)
	child.TopLeft = geo.NewPoint(100, 100)
	graph.AddNewNodeToContainer(nil, container)
	graph.AddNewNodeToContainer(container, child)
	return graph, container, child
}

func routedContainerTestDecision(t *testing.T, graph *layoutgraph.Graph, container *layoutgraph.Node, original *geo.Box) (routedContainerBoxDecision, error) {
	t.Helper()
	var incidentEdges []*layoutgraph.Edge
	for _, edge := range graph.Edges {
		if edge != nil && (edge.From == container || edge.To == container) {
			incidentEdges = append(incidentEdges, edge)
		}
	}
	return binPackCanUseRoutedContainerBox(graph, container, original, incidentEdges, routedContainerTestGuard(t))
}

func TestRoutedContainerDefaultEdgeBorderRadiusMatchesRenderer(t *testing.T) {
	if got, want := routedContainerDefaultEdgeBorderRadius, d2target.BaseConnection().BorderRadius; got != want {
		t.Fatalf("routed-container default edge border radius = %v, renderer default = %v", got, want)
	}
}

func TestBinPackCanUseRoutedContainerBoxForSameSideEndpoint(t *testing.T) {
	graph, container, _ := routedContainerTestGraph(shape.SQUARE_TYPE)
	container.FixedTopLeft = container.TopLeft.Copy()
	external := layoutgraph.NewNode(3, 20, 20)
	external.TopLeft = geo.NewPoint(90, -100)
	graph.AddNewNodeToContainer(nil, external)
	edge := graph.Connect(container, external)
	edge.Points = []*geo.Point{geo.NewPoint(100, 0), geo.NewPoint(100, -80)}
	original := geo.Box{TopLeft: geo.NewPoint(0, 0), Width: 300, Height: 300}
	container.Width = 300
	container.Height = 180

	decision, err := routedContainerTestDecision(t, graph, container, &original)
	if err != nil {
		t.Fatal(err)
	}
	if decision != routedContainerUseProposedBox {
		t.Fatal("endpoint on an unchanged side was rejected for opposite-side compaction")
	}
}

func TestBinPackCanUseRoutedContainerBoxRejectsEndpointBecomingNewCorner(t *testing.T) {
	graph, container, _ := routedContainerTestGraph(shape.SQUARE_TYPE)
	external := layoutgraph.NewNode(3, 20, 20)
	external.TopLeft = geo.NewPoint(170, -100)
	graph.AddNewNodeToContainer(nil, external)
	edge := graph.Connect(container, external)
	edge.Points = []*geo.Point{geo.NewPoint(180, 0), geo.NewPoint(180, -80)}
	original := geo.Box{TopLeft: geo.NewPoint(0, 0), Width: 300, Height: 300}
	container.Width = 180
	container.Height = 180

	decision, err := routedContainerTestDecision(t, graph, container, &original)
	if err != nil {
		t.Fatal(err)
	}
	if decision != routedContainerKeepOriginalBox {
		t.Fatal("attached top side was shortened until its endpoint became a new rounded corner")
	}
}

func TestBinPackCanUseRoutedContainerBoxRejectsSelfLoops(t *testing.T) {
	graph, container, _ := routedContainerTestGraph(shape.SQUARE_TYPE)
	edge := graph.Connect(container, container)
	edge.Points = []*geo.Point{
		geo.NewPoint(100, 0),
		geo.NewPoint(100, -50),
		geo.NewPoint(150, -50),
		geo.NewPoint(150, 0),
	}
	original := geo.Box{TopLeft: geo.NewPoint(0, 0), Width: 300, Height: 300}
	container.Width = 300
	container.Height = 180

	decision, err := routedContainerTestDecision(t, graph, container, &original)
	if err != nil {
		t.Fatal(err)
	}
	if decision != routedContainerKeepOriginalBox {
		t.Fatal("self-loop was accepted for routed container compaction")
	}
}

func TestBinPackCanUseRoutedContainerBoxRejectsDescendantRoute(t *testing.T) {
	graph, container, child := routedContainerTestGraph(shape.SQUARE_TYPE)
	edge := graph.Connect(container, child)
	edge.Points = []*geo.Point{
		geo.NewPoint(150, 0),
		geo.NewPoint(150, 250),
		geo.NewPoint(120, 250),
		geo.NewPoint(120, 110),
	}
	original := geo.Box{TopLeft: geo.NewPoint(0, 0), Width: 300, Height: 300}
	container.Width = 300
	container.Height = 180

	decision, err := routedContainerTestDecision(t, graph, container, &original)
	if err != nil {
		t.Fatal(err)
	}
	if decision != routedContainerKeepOriginalBox {
		t.Fatal("descendant route was accepted even though its body leaves the proposed box")
	}
}

func TestBinPackCanUseRoutedContainerBoxDefersDescendantOnlyRouteToSideConstraints(t *testing.T) {
	graph, container, child := routedContainerTestGraph(shape.SQUARE_TYPE)
	edge := graph.Connect(child, child)
	edge.Points = []*geo.Point{
		geo.NewPoint(100, 100),
		geo.NewPoint(100, 150),
		geo.NewPoint(120, 150),
		geo.NewPoint(120, 100),
	}
	original := geo.Box{TopLeft: geo.NewPoint(0, 0), Width: 300, Height: 300}
	container.Width = 300
	container.Height = 180

	decision, err := routedContainerTestDecision(t, graph, container, &original)
	if err != nil {
		t.Fatal(err)
	}
	if decision != routedContainerDeferToSideConstraints {
		t.Fatal("descendant-only route did not defer to ordinary side constraints")
	}
}

func TestBinPackCanUseRoutedContainerBoxPreservesFixedOrigin(t *testing.T) {
	graph, container, _ := routedContainerTestGraph(shape.SQUARE_TYPE)
	container.FixedTopLeft = container.TopLeft.Copy()
	external := layoutgraph.NewNode(3, 20, 20)
	external.TopLeft = geo.NewPoint(90, 400)
	graph.AddNewNodeToContainer(nil, external)
	edge := graph.Connect(container, external)
	edge.Points = []*geo.Point{geo.NewPoint(100, 300), geo.NewPoint(100, 400)}
	original := geo.Box{TopLeft: geo.NewPoint(0, 0), Width: 300, Height: 300}
	container.TopLeft.Y = 20
	container.Height = 280

	decision, err := routedContainerTestDecision(t, graph, container, &original)
	if err != nil {
		t.Fatal(err)
	}
	if decision != routedContainerKeepOriginalBox {
		t.Fatal("routed fixed container accepted a translated proposed box")
	}
}

func TestBinPackCanUseRoutedContainerBoxForDescendantRouteBodies(t *testing.T) {
	zeroRadius := "0"
	invalidRadius := "not-a-radius"
	negativeRadius := "-1"
	nonfiniteRadius := "NaN"
	oversizedRadius := strings.Repeat("1", routedContainerMaxBorderRadiusTextBytes+1)
	tests := []struct {
		name       string
		points     []*geo.Point
		toExternal bool
		isCurve    bool
		radius     *string
		want       bool
	}{
		{
			name: "contained internal route",
			points: []*geo.Point{
				geo.NewPoint(100, 100), geo.NewPoint(100, 150),
				geo.NewPoint(120, 150), geo.NewPoint(120, 100),
			},
			want: true,
		},
		{
			name: "single exit through unchanged side",
			points: []*geo.Point{
				geo.NewPoint(100, 100), geo.NewPoint(100, -50),
			},
			toExternal: true,
			want:       true,
		},
		{
			name: "crosses moved side",
			points: []*geo.Point{
				geo.NewPoint(100, 100), geo.NewPoint(100, 350),
			},
			toExternal: true,
			want:       false,
		},
		{
			name: "internal route reenters from removed strip",
			points: []*geo.Point{
				geo.NewPoint(100, 100), geo.NewPoint(100, 250),
				geo.NewPoint(120, 250), geo.NewPoint(120, 100),
			},
			want: false,
		},
		{
			name: "curved route",
			points: []*geo.Point{
				geo.NewPoint(100, 100), geo.NewPoint(100, 150),
				geo.NewPoint(120, 150), geo.NewPoint(120, 100),
			},
			isCurve: true,
			want:    false,
		},
		{
			name: "default rounded corner straddles boundary",
			points: []*geo.Point{
				geo.NewPoint(100, 20), geo.NewPoint(110, -5), geo.NewPoint(140, -35),
			},
			toExternal: true,
			want:       false,
		},
		{
			name: "default rounded short segment",
			points: []*geo.Point{
				geo.NewPoint(100, 100), geo.NewPoint(100, 130),
				geo.NewPoint(110, 130), geo.NewPoint(110, 100),
			},
			want: false,
		},
		{
			name: "explicit square corner crossing",
			points: []*geo.Point{
				geo.NewPoint(100, 20), geo.NewPoint(110, -5), geo.NewPoint(140, -35),
			},
			toExternal: true,
			radius:     &zeroRadius,
			want:       true,
		},
		{
			name: "invalid corner radius",
			points: []*geo.Point{
				geo.NewPoint(100, 100), geo.NewPoint(100, -50),
			},
			toExternal: true,
			radius:     &invalidRadius,
			want:       false,
		},
		{
			name: "negative corner radius",
			points: []*geo.Point{
				geo.NewPoint(100, 100), geo.NewPoint(100, -50),
			},
			toExternal: true,
			radius:     &negativeRadius,
			want:       false,
		},
		{
			name: "nonfinite corner radius",
			points: []*geo.Point{
				geo.NewPoint(100, 100), geo.NewPoint(100, -50),
			},
			toExternal: true,
			radius:     &nonfiniteRadius,
			want:       false,
		},
		{
			name: "oversized corner radius",
			points: []*geo.Point{
				geo.NewPoint(100, 100), geo.NewPoint(100, -50),
			},
			toExternal: true,
			radius:     &oversizedRadius,
			want:       false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			graph, container, child := routedContainerTestGraph(shape.SQUARE_TYPE)
			other := child
			if test.toExternal {
				other = layoutgraph.NewNode(3, 20, 20)
				other.TopLeft = geo.NewPoint(90, -100)
				graph.AddNewNodeToContainer(nil, other)
			}
			edge := graph.Connect(child, other)
			edge.Points = test.points
			edge.IsCurve = test.isCurve
			if test.radius != nil {
				edge.Style.BorderRadius = &layoutgraph.StyleScalar{Value: *test.radius}
			}
			rootExternal := layoutgraph.NewNode(4, 20, 20)
			rootExternal.TopLeft = geo.NewPoint(190, -100)
			graph.AddNewNodeToContainer(nil, rootExternal)
			rootEdge := graph.Connect(container, rootExternal)
			rootEdge.Points = []*geo.Point{geo.NewPoint(200, 0), geo.NewPoint(200, -80)}
			original := geo.Box{TopLeft: geo.NewPoint(0, 0), Width: 300, Height: 300}
			container.Width = 300
			container.Height = 180

			decision, err := routedContainerTestDecision(t, graph, container, &original)
			if err != nil {
				t.Fatal(err)
			}
			want := routedContainerKeepOriginalBox
			if test.want {
				want = routedContainerUseProposedBox
			}
			if decision != want {
				t.Fatalf("routed container decision = %v, want %v", decision, want)
			}
		})
	}
}

func TestRoutedContainerSegmentStaysInsideShrink(t *testing.T) {
	original := geo.Box{TopLeft: geo.NewPoint(0, 0), Width: 300, Height: 300}
	proposed := geo.Box{TopLeft: geo.NewPoint(0, 0), Width: 300, Height: 180}
	tests := []struct {
		name       string
		start, end *geo.Point
		want       bool
	}{
		{name: "inside candidate", start: geo.NewPoint(100, 50), end: geo.NewPoint(100, 150), want: true},
		{name: "outside original", start: geo.NewPoint(100, -100), end: geo.NewPoint(200, -50), want: true},
		{name: "exit unchanged side", start: geo.NewPoint(100, 100), end: geo.NewPoint(100, -50), want: true},
		{name: "cross removed strip", start: geo.NewPoint(100, 100), end: geo.NewPoint(100, 350), want: false},
		{name: "tangent at removed corner", start: geo.NewPoint(-10, 310), end: geo.NewPoint(10, 290), want: false},
		{name: "zero length", start: geo.NewPoint(100, 100), end: geo.NewPoint(100, 100), want: false},
		{name: "nonfinite", start: geo.NewPoint(100, 100), end: geo.NewPoint(100, math.Inf(1)), want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := routedContainerSegmentStaysInsideShrink(
				&original, &proposed, test.start, test.end, routedContainerTestGuard(t),
			)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("segment preservation = %v, want %v", got, test.want)
			}
		})
	}
}

func TestBinPackCanUseRoutedContainerBoxFailsClosedForUnsafeShapes(t *testing.T) {
	tests := []struct {
		name      string
		shapeType string
		configure func(*layoutgraph.Node)
	}{
		{name: "curved", shapeType: shape.OVAL_TYPE},
		{name: "3d", shapeType: shape.SQUARE_TYPE, configure: func(node *layoutgraph.Node) { node.Is3D = true }},
		{name: "multiple", shapeType: shape.SQUARE_TYPE, configure: func(node *layoutgraph.Node) { node.IsMultiple = true }},
		{name: "non-container shape", shapeType: shape.IMAGE_TYPE},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			graph, container, _ := routedContainerTestGraph(test.shapeType)
			if test.configure != nil {
				test.configure(container)
			}
			external := layoutgraph.NewNode(3, 20, 20)
			external.TopLeft = geo.NewPoint(90, -100)
			graph.AddNewNodeToContainer(nil, external)
			edge := graph.Connect(container, external)
			edge.Points = []*geo.Point{geo.NewPoint(100, 0), geo.NewPoint(100, -80)}
			original := geo.Box{TopLeft: geo.NewPoint(0, 0), Width: 300, Height: 300}
			container.Width = 300
			container.Height = 180

			decision, err := routedContainerTestDecision(t, graph, container, &original)
			if err != nil {
				t.Fatal(err)
			}
			if decision != routedContainerKeepOriginalBox {
				t.Fatal("unsafe shape accepted routed container compaction")
			}
		})
	}
}
