package packing

import (
	"context"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/shape"
)

func TestPackCompactsRoutedRectangularContainerWhenEndpointStaysOnOriginalSide(t *testing.T) {
	graph := layoutgraph.NewGraph()
	container := layoutgraph.NewNode(1, 700, 600)
	container.SetShape(shape.SQUARE_TYPE)
	container.TopLeft = geo.NewPoint(20, 30)
	desiredWidth := 700.
	container.DesiredWidth = &desiredWidth
	first := layoutgraph.NewNode(2, 20, 20)
	first.TopLeft = geo.NewPoint(80, 90)
	moving := layoutgraph.NewNode(3, 20, 20)
	moving.TopLeft = geo.NewPoint(40, 130)
	external := layoutgraph.NewNode(4, 20, 20)
	external.TopLeft = geo.NewPoint(80, -100)
	graph.AddNewNodeToContainer(nil, container)
	graph.AddNewNodeToContainer(container, first)
	graph.AddNewNodeToContainer(container, moving)
	graph.AddNewNodeToContainer(nil, external)
	edge := graph.Connect(container, external)
	edge.Points = []*geo.Point{geo.NewPoint(50, 30), geo.NewPoint(50, -80)}
	descendantExit := graph.Connect(first, external)
	descendantExit.Points = []*geo.Point{geo.NewPoint(90, 90), geo.NewPoint(90, -80)}
	originalTopLeft := container.TopLeft.Copy()
	originalWidth, originalHeight := container.Width, container.Height
	originalRoute := captureExactRouteTest(edge)
	originalDescendantExit := captureExactRouteTest(descendantExit)

	if err := Pack(context.Background(), graph, container); err != nil {
		t.Fatal(err)
	}
	if !container.TopLeft.Equals(originalTopLeft) {
		t.Fatalf("routed container top-left changed from %v to %v", originalTopLeft, container.TopLeft)
	}
	if container.Width != originalWidth || container.Height >= originalHeight {
		t.Fatalf(
			"routed container did not compact only opposite its attached top side from %vx%v: got %vx%v; children at %v and %v",
			originalWidth, originalHeight, container.Width, container.Height, first.TopLeft, moving.TopLeft,
		)
	}
	if edge.Points[0].Y != container.TopLeft.Y || edge.Points[0].X < container.TopLeft.X || edge.Points[0].X > container.TopLeft.X+container.Width {
		t.Fatalf("route endpoint %v detached from compacted container top side %#v", edge.Points[0], container.Box)
	}
	originalRoute.assertRestored(t)
	originalDescendantExit.assertRestored(t)
}

func TestPackPreservesContainerWhenDescendantRouteLeavesCandidate(t *testing.T) {
	graph := layoutgraph.NewGraph()
	container := layoutgraph.NewNode(1, 700, 600)
	container.SetShape(shape.SQUARE_TYPE)
	container.TopLeft = geo.NewPoint(20, 30)
	desiredWidth := 700.
	container.DesiredWidth = &desiredWidth
	first := layoutgraph.NewNode(2, 20, 20)
	first.TopLeft = geo.NewPoint(40, 50)
	moving := layoutgraph.NewNode(3, 20, 20)
	moving.TopLeft = geo.NewPoint(40, 130)
	graph.AddNewNodeToContainer(nil, container)
	graph.AddNewNodeToContainer(container, first)
	graph.AddNewNodeToContainer(container, moving)
	loop := graph.Connect(moving, moving)
	loop.Points = []*geo.Point{
		geo.NewPoint(45, 150),
		geo.NewPoint(45, 550),
		geo.NewPoint(55, 550),
		geo.NewPoint(55, 150),
	}
	external := layoutgraph.NewNode(4, 20, 20)
	external.TopLeft = geo.NewPoint(40, -100)
	graph.AddNewNodeToContainer(nil, external)
	edge := graph.Connect(container, external)
	edge.Points = []*geo.Point{geo.NewPoint(50, 30), geo.NewPoint(50, -80)}
	originalTopLeft := container.TopLeft.Copy()
	originalWidth, originalHeight := container.Width, container.Height

	if err := Pack(context.Background(), graph, container); err != nil {
		t.Fatal(err)
	}
	if !container.TopLeft.Equals(originalTopLeft) || container.Width != originalWidth || container.Height != originalHeight {
		t.Fatalf(
			"container with a leaky descendant route changed from (%v, %vx%v) to (%v, %vx%v)",
			originalTopLeft, originalWidth, originalHeight, container.TopLeft, container.Width, container.Height,
		)
	}
}

func TestPackPreservesRoutedRectangularContainerWhenShrinkChangesAttachedSide(t *testing.T) {
	graph := layoutgraph.NewGraph()
	container := layoutgraph.NewNode(1, 700, 600)
	container.SetShape(shape.SQUARE_TYPE)
	container.TopLeft = geo.NewPoint(20, 30)
	first := layoutgraph.NewNode(2, 20, 20)
	first.TopLeft = geo.NewPoint(40, 50)
	moving := layoutgraph.NewNode(3, 20, 20)
	moving.TopLeft = geo.NewPoint(40, 130)
	external := layoutgraph.NewNode(4, 20, 20)
	external.TopLeft = geo.NewPoint(190, -100)
	graph.AddNewNodeToContainer(nil, container)
	graph.AddNewNodeToContainer(container, first)
	graph.AddNewNodeToContainer(container, moving)
	graph.AddNewNodeToContainer(nil, external)
	edge := graph.Connect(container, external)
	edge.Points = []*geo.Point{geo.NewPoint(200, 30), geo.NewPoint(200, -80)}
	originalTopLeft := container.TopLeft.Copy()
	originalWidth, originalHeight := container.Width, container.Height

	if err := Pack(context.Background(), graph, container); err != nil {
		t.Fatal(err)
	}
	if !container.TopLeft.Equals(originalTopLeft) || container.Width != originalWidth || container.Height != originalHeight {
		t.Fatalf(
			"routed container box changed from (%v, %vx%v) to (%v, %vx%v)",
			originalTopLeft, originalWidth, originalHeight, container.TopLeft, container.Width, container.Height,
		)
	}
}

func TestPackRejectsRoutedContainerWhenRootIncidenceIsMissing(t *testing.T) {
	graph := layoutgraph.NewGraph()
	container := layoutgraph.NewNode(1, 700, 600)
	container.SetShape(shape.SQUARE_TYPE)
	container.TopLeft = geo.NewPoint(20, 30)
	first := layoutgraph.NewNode(2, 20, 20)
	first.TopLeft = geo.NewPoint(40, 50)
	moving := layoutgraph.NewNode(3, 20, 20)
	moving.TopLeft = geo.NewPoint(40, 130)
	external := layoutgraph.NewNode(4, 20, 20)
	external.TopLeft = geo.NewPoint(690, -100)
	graph.AddNewNodeToContainer(nil, container)
	graph.AddNewNodeToContainer(container, first)
	graph.AddNewNodeToContainer(container, moving)
	graph.AddNewNodeToContainer(nil, external)
	edge := graph.Connect(container, external)
	edge.Points = []*geo.Point{geo.NewPoint(700, 30), geo.NewPoint(700, -80)}
	container.Edges = nil
	originalTopLeft := container.TopLeft
	originalTopLeftValue := *container.TopLeft
	originalWidth, originalHeight := container.Width, container.Height
	originalRoute := captureExactRouteTest(edge)

	err := Pack(context.Background(), graph, container)
	if err == nil || !strings.Contains(err.Error(), "edge inventory") {
		t.Fatalf("Pack error = %v, want root edge-inventory rejection", err)
	}
	if container.TopLeft != originalTopLeft || *container.TopLeft != originalTopLeftValue ||
		container.Width != originalWidth || container.Height != originalHeight {
		t.Fatalf(
			"rejected container changed from (%v, %vx%v) to (%v, %vx%v)",
			originalTopLeftValue, originalWidth, originalHeight, container.TopLeft, container.Width, container.Height,
		)
	}
	originalRoute.assertRestored(t)
}

func TestPackRejectsDescendantEdgeInventoryMismatch(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*layoutgraph.Graph, *layoutgraph.Node, *layoutgraph.Edge)
	}{
		{
			name: "route omitted from graph inventory",
			configure: func(graph *layoutgraph.Graph, _ *layoutgraph.Node, _ *layoutgraph.Edge) {
				graph.Edges = nil
			},
		},
		{
			name: "route omitted from descendant inventory",
			configure: func(_ *layoutgraph.Graph, descendant *layoutgraph.Node, _ *layoutgraph.Edge) {
				descendant.Edges = nil
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			graph := layoutgraph.NewGraph()
			container := layoutgraph.NewNode(1, 700, 600)
			container.SetShape(shape.SQUARE_TYPE)
			container.TopLeft = geo.NewPoint(20, 30)
			desiredWidth := 700.
			container.DesiredWidth = &desiredWidth
			first := layoutgraph.NewNode(2, 20, 20)
			first.TopLeft = geo.NewPoint(40, 50)
			moving := layoutgraph.NewNode(3, 20, 20)
			moving.TopLeft = geo.NewPoint(40, 130)
			graph.AddNewNodeToContainer(nil, container)
			graph.AddNewNodeToContainer(container, first)
			graph.AddNewNodeToContainer(container, moving)
			loop := graph.Connect(moving, moving)
			loop.Points = []*geo.Point{
				geo.NewPoint(45, 150), geo.NewPoint(45, 550),
				geo.NewPoint(55, 550), geo.NewPoint(55, 150),
			}
			test.configure(graph, moving, loop)

			containerTopLeft := container.TopLeft
			containerTopLeftValue := *container.TopLeft
			containerWidth, containerHeight := container.Width, container.Height
			movingTopLeft := moving.TopLeft
			movingTopLeftValue := *moving.TopLeft
			route := captureExactRouteTest(loop)

			err := Pack(context.Background(), graph, container)
			if err == nil || !strings.Contains(err.Error(), "edge inventory") {
				t.Fatalf("Pack error = %v, want descendant edge-inventory rejection", err)
			}
			if container.TopLeft != containerTopLeft || *container.TopLeft != containerTopLeftValue ||
				container.Width != containerWidth || container.Height != containerHeight ||
				moving.TopLeft != movingTopLeft || *moving.TopLeft != movingTopLeftValue {
				t.Fatal("descendant edge-inventory rejection changed node geometry")
			}
			route.assertRestored(t)
		})
	}
}

func TestPackRejectsPartiallyRoutedGraph(t *testing.T) {
	graph := layoutgraph.NewGraph()
	container := layoutgraph.NewNode(1, 700, 600)
	container.SetShape(shape.SQUARE_TYPE)
	container.TopLeft = geo.NewPoint(20, 30)
	fixed := layoutgraph.NewNode(2, 20, 20)
	fixed.TopLeft = geo.NewPoint(80, 80)
	fixed.FixedTopLeft = fixed.TopLeft.Copy()
	moving := layoutgraph.NewNode(3, 20, 20)
	moving.TopLeft = geo.NewPoint(80, 160)
	external := layoutgraph.NewNode(4, 20, 20)
	external.TopLeft = geo.NewPoint(900, 40)
	other := layoutgraph.NewNode(5, 20, 20)
	other.TopLeft = geo.NewPoint(900, 140)
	graph.AddNewNodeToContainer(nil, container)
	graph.AddNewNodeToContainer(container, fixed)
	graph.AddNewNodeToContainer(container, moving)
	graph.AddNewNodeToContainer(nil, external)
	graph.AddNewNodeToContainer(nil, other)
	routed := graph.Connect(container, external)
	routed.Points = []*geo.Point{geo.NewPoint(720, 50), geo.NewPoint(900, 50)}
	graph.Connect(external, other)

	containerTopLeft := container.TopLeft
	containerTopLeftValue := *container.TopLeft
	containerWidth, containerHeight := container.Width, container.Height
	route := captureExactRouteTest(routed)

	err := Pack(context.Background(), graph, container)
	if err == nil || !strings.Contains(err.Error(), "partially routed") {
		t.Fatalf("Pack error = %v, want partial-route rejection", err)
	}
	if container.TopLeft != containerTopLeft || *container.TopLeft != containerTopLeftValue ||
		container.Width != containerWidth || container.Height != containerHeight {
		t.Fatal("partial-route rejection changed container geometry")
	}
	route.assertRestored(t)
}

func TestPackRejectsMalformedRoute(t *testing.T) {
	tests := []struct {
		name   string
		points []*geo.Point
		want   string
	}{
		{name: "one point", points: []*geo.Point{geo.NewPoint(10, 5)}, want: "incomplete route"},
		{name: "nil point", points: []*geo.Point{geo.NewPoint(10, 5), nil}, want: "nil route point"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			graph := layoutgraph.NewGraph()
			from := layoutgraph.NewNode(1, 10, 10)
			from.TopLeft = geo.NewPoint(0, 0)
			to := layoutgraph.NewNode(2, 10, 10)
			to.TopLeft = geo.NewPoint(100, 0)
			graph.AddNewNodeToContainer(nil, from)
			graph.AddNewNodeToContainer(nil, to)
			edge := graph.Connect(from, to)
			edge.Points = test.points
			fromTopLeft := from.TopLeft
			fromTopLeftValue := *from.TopLeft

			err := Pack(context.Background(), graph, nil)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Pack error = %v, want %q", err, test.want)
			}
			if from.TopLeft != fromTopLeft || *from.TopLeft != fromTopLeftValue {
				t.Fatal("malformed-route rejection changed node geometry")
			}
		})
	}
}

func TestPackRejectsNilEdge(t *testing.T) {
	graph := layoutgraph.NewGraph()
	node := layoutgraph.NewNode(1, 10, 10)
	node.TopLeft = geo.NewPoint(0, 0)
	graph.AddNewNodeToContainer(nil, node)
	graph.Edges = append(graph.Edges, nil)
	topLeft := node.TopLeft
	topLeftValue := *node.TopLeft

	err := Pack(context.Background(), graph, nil)
	if err == nil || !strings.Contains(err.Error(), "nil edge") {
		t.Fatalf("Pack error = %v, want nil-edge rejection", err)
	}
	if node.TopLeft != topLeft || *node.TopLeft != topLeftValue {
		t.Fatal("nil-edge rejection changed node geometry")
	}
}

func TestPackPreservesFixedRoutedContainerOrigin(t *testing.T) {
	newGraph := func(fixContainer bool) (*layoutgraph.Graph, *layoutgraph.Node) {
		graph := layoutgraph.NewGraph()
		container := layoutgraph.NewNode(1, 300, 300)
		container.SetShape(shape.SQUARE_TYPE)
		container.TopLeft = geo.NewPoint(0, 0)
		desiredWidth := 300.
		container.DesiredWidth = &desiredWidth
		if fixContainer {
			container.FixedTopLeft = container.TopLeft.Copy()
		}
		anchor := layoutgraph.NewNode(2, 20, 20)
		anchor.TopLeft = geo.NewPoint(100, 220)
		anchor.FixedTopLeft = geo.NewPoint(40, 120)
		moving := layoutgraph.NewNode(3, 20, 20)
		moving.TopLeft = geo.NewPoint(100, 140)
		external := layoutgraph.NewNode(4, 20, 20)
		external.TopLeft = geo.NewPoint(90, 400)
		graph.AddNewNodeToContainer(nil, container)
		graph.AddNewNodeToContainer(container, anchor)
		graph.AddNewNodeToContainer(container, moving)
		graph.AddNewNodeToContainer(nil, external)
		edge := graph.Connect(container, external)
		edge.Points = []*geo.Point{geo.NewPoint(100, 300), geo.NewPoint(100, 400)}
		return graph, container
	}

	unlockedGraph, unlocked := newGraph(false)
	if err := Pack(context.Background(), unlockedGraph, unlocked); err != nil {
		t.Fatal(err)
	}
	if unlocked.TopLeft.X != 0 || unlocked.TopLeft.Y <= 0 || unlocked.Width != 300 ||
		unlocked.TopLeft.Y+unlocked.Height != 300 {
		t.Fatalf("unlocked control did not propose an otherwise-safe bottom-anchored shrink: %#v", unlocked.Box)
	}

	fixedGraph, fixed := newGraph(true)
	originalTopLeft := fixed.TopLeft.Copy()
	originalWidth, originalHeight := fixed.Width, fixed.Height
	if err := Pack(context.Background(), fixedGraph, fixed); err != nil {
		t.Fatal(err)
	}
	if !fixed.TopLeft.Equals(originalTopLeft) || fixed.Width != originalWidth || fixed.Height != originalHeight {
		t.Fatalf(
			"fixed routed container changed from (%v, %vx%v) to (%v, %vx%v)",
			originalTopLeft, originalWidth, originalHeight, fixed.TopLeft, fixed.Width, fixed.Height,
		)
	}
}

func TestPackPreservesRoutedCurvedContainerBox(t *testing.T) {
	graph := layoutgraph.NewGraph()
	container := layoutgraph.NewNode(1, 700, 600)
	container.SetShape(shape.OVAL_TYPE)
	container.TopLeft = geo.NewPoint(20, 30)
	desiredWidth := 700.
	container.DesiredWidth = &desiredWidth
	first := layoutgraph.NewNode(2, 20, 20)
	first.TopLeft = geo.NewPoint(40, 50)
	moving := layoutgraph.NewNode(3, 20, 20)
	moving.TopLeft = geo.NewPoint(40, 130)
	external := layoutgraph.NewNode(4, 20, 20)
	external.TopLeft = geo.NewPoint(360, -100)
	graph.AddNewNodeToContainer(nil, container)
	graph.AddNewNodeToContainer(container, first)
	graph.AddNewNodeToContainer(container, moving)
	graph.AddNewNodeToContainer(nil, external)
	edge := graph.Connect(container, external)
	edge.Points = []*geo.Point{geo.NewPoint(370, 30), geo.NewPoint(370, -80)}
	originalTopLeft := container.TopLeft.Copy()
	originalWidth, originalHeight := container.Width, container.Height

	if err := Pack(context.Background(), graph, container); err != nil {
		t.Fatal(err)
	}
	if !container.TopLeft.Equals(originalTopLeft) || container.Width != originalWidth || container.Height != originalHeight {
		t.Fatalf(
			"routed curved container box changed from (%v, %vx%v) to (%v, %vx%v)",
			originalTopLeft, originalWidth, originalHeight, container.TopLeft, container.Width, container.Height,
		)
	}
}

func TestPackPreservesRoutedSelfLoopContainerBox(t *testing.T) {
	graph := layoutgraph.NewGraph()
	container := layoutgraph.NewNode(1, 700, 600)
	container.SetShape(shape.SQUARE_TYPE)
	container.TopLeft = geo.NewPoint(20, 30)
	desiredWidth := 700.
	container.DesiredWidth = &desiredWidth
	first := layoutgraph.NewNode(2, 20, 20)
	first.TopLeft = geo.NewPoint(40, 50)
	moving := layoutgraph.NewNode(3, 20, 20)
	moving.TopLeft = geo.NewPoint(40, 130)
	graph.AddNewNodeToContainer(nil, container)
	graph.AddNewNodeToContainer(container, first)
	graph.AddNewNodeToContainer(container, moving)
	edge := graph.Connect(container, container)
	edge.Points = []*geo.Point{
		geo.NewPoint(50, 30),
		geo.NewPoint(50, -20),
		geo.NewPoint(100, -20),
		geo.NewPoint(100, 30),
	}
	originalTopLeft := container.TopLeft.Copy()
	originalWidth, originalHeight := container.Width, container.Height

	if err := Pack(context.Background(), graph, container); err != nil {
		t.Fatal(err)
	}
	if !container.TopLeft.Equals(originalTopLeft) || container.Width != originalWidth || container.Height != originalHeight {
		t.Fatalf(
			"routed self-loop container box changed from (%v, %vx%v) to (%v, %vx%v)",
			originalTopLeft, originalWidth, originalHeight, container.TopLeft, container.Width, container.Height,
		)
	}
}

func TestPackDoesNotMoveRoutedChildAttachedToContainer(t *testing.T) {
	graph := layoutgraph.NewGraph()
	container := layoutgraph.NewNode(1, 300, 300)
	container.SetShape(shape.SQUARE_TYPE)
	container.TopLeft = geo.NewPoint(0, 0)
	connected := layoutgraph.NewNode(2, 20, 20)
	connected.TopLeft = geo.NewPoint(220, 140)
	disconnected := layoutgraph.NewNode(3, 20, 20)
	disconnected.TopLeft = geo.NewPoint(70, 70)
	graph.AddNewNodeToContainer(nil, container)
	graph.AddNewNodeToContainer(container, connected)
	graph.AddNewNodeToContainer(container, disconnected)
	edge := graph.Connect(container, connected)
	edge.Points = []*geo.Point{geo.NewPoint(300, 150), geo.NewPoint(240, 150)}
	originalChildTopLeft := connected.TopLeft.Copy()
	originalRoute := append([]*geo.Point(nil), edge.Points...)
	originalRouteValues := make([]geo.Point, len(edge.Points))
	for index, point := range edge.Points {
		originalRouteValues[index] = *point
	}

	if err := Pack(context.Background(), graph, container); err != nil {
		t.Fatal(err)
	}
	if !connected.TopLeft.Equals(originalChildTopLeft) {
		t.Fatalf("routed child moved from %v to %v", originalChildTopLeft, connected.TopLeft)
	}
	for index := range originalRoute {
		if edge.Points[index] != originalRoute[index] || *edge.Points[index] != originalRouteValues[index] {
			t.Fatalf("container route point %d changed from %p %v to %p %v", index, originalRoute[index], originalRouteValues[index], edge.Points[index], edge.Points[index])
		}
	}
}
