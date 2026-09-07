package routing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/loops"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
)

func TestNoLoops(t *testing.T) {
	g := layoutgraph.NewGraph()
	n1 := g.AddNode(layoutgraph.NewNode(1, 100, 100))
	n1.TopLeft = geo.NewPoint(0, 0)
	n2 := g.AddNode(layoutgraph.NewNode(2, 100, 100))
	n2.TopLeft = geo.NewPoint(200, 200)

	g.Connect(n1, n2)

	edges := loops.Route(n1)
	assert.Equal(t, 0, len(edges))
}

func TestRouteLoop(t *testing.T) {
	g := layoutgraph.NewGraph()
	n1 := g.AddNode(layoutgraph.NewNode(1, 100, 100))
	n1.TopLeft = geo.NewPoint(0, 0)
	g.Connect(n1, n1)

	ovg, err := buildOVGFromGraph(context.Background(), g, nil)
	if err != nil {
		t.Fatal(err)
	}
	edges := loops.Route(n1)
	assert.Equal(t, 1, len(edges))

	route := makeLoopRoute(edges[0], ovg)
	assert.Equal(t, g.Edges[0], route.GEdge)
	assert.Equal(t, 7, len(route.OVGNodes))

	leftPortIndex := n1.PortIndices(geo.Left)[0]
	topPortIndex := n1.PortIndices(geo.Top)[0]
	assert.Equal(t, *ovg.Ports[n1][leftPortIndex].Point, route.FromPort)
	assert.Equal(t, *ovg.Ports[n1][topPortIndex].Point, route.ToPort)
}

func TestRouteLoopsWithDifferentArrowheads(t *testing.T) {
	g := layoutgraph.NewGraph()
	n1 := g.AddNode(layoutgraph.NewNode(1, 100, 100))
	n1.TopLeft = geo.NewPoint(0, 0)
	e1 := g.Connect(n1, n1)
	e1.SourceArrowhead = layoutgraph.NoArrowhead
	e1.TargetArrowhead = layoutgraph.NoArrowhead

	e2 := g.Connect(n1, n1)
	e2.SourceArrowhead = layoutgraph.TriangleArrowhead
	e2.TargetArrowhead = layoutgraph.TriangleArrowhead

	e3 := g.Connect(n1, n1)
	e3.SourceArrowhead = layoutgraph.TriangleArrowhead
	e3.TargetArrowhead = layoutgraph.NoArrowhead

	ovg, err := buildOVGFromGraph(context.Background(), g, nil)
	if err != nil {
		t.Fatal(err)
	}
	edges := loops.Route(n1)

	route := makeLoopRoute(edges[0], ovg)
	leftPortIndex := n1.PortIndices(geo.Left)[0]
	topPortIndex := n1.PortIndices(geo.Top)[0]
	assert.Equal(t, *ovg.Ports[n1][leftPortIndex].Point, route.FromPort)
	assert.Equal(t, *ovg.Ports[n1][topPortIndex].Point, route.ToPort)

	route = makeLoopRoute(edges[1], ovg)
	topPortIndex = n1.PortIndices(geo.Top)[2]
	rightPortIndex := n1.PortIndices(geo.Right)[0]
	assert.Equal(t, *ovg.Ports[n1][topPortIndex].Point, route.FromPort)
	assert.Equal(t, *ovg.Ports[n1][rightPortIndex].Point, route.ToPort)

	route = makeLoopRoute(edges[2], ovg)
	bottomPortIndex := n1.PortIndices(geo.Bottom)[0]
	leftPortIndex = n1.PortIndices(geo.Left)[2]
	assert.Equal(t, *ovg.Ports[n1][bottomPortIndex].Point, route.FromPort)
	assert.Equal(t, *ovg.Ports[n1][leftPortIndex].Point, route.ToPort)
}

func TestRouteLoopsSortedByArea(t *testing.T) {
	g := layoutgraph.NewGraph()
	n1 := g.AddNode(layoutgraph.NewNode(1, 100, 100))
	n1.TopLeft = geo.NewPoint(0, 0)
	e1 := g.Connect(n1, n1)
	e1.SourceArrowhead = layoutgraph.NoArrowhead
	e1.TargetArrowhead = layoutgraph.NoArrowhead

	e2 := g.Connect(n1, n1)
	e2.SourceArrowhead = layoutgraph.TriangleArrowhead
	e2.TargetArrowhead = layoutgraph.TriangleArrowhead

	e3 := g.Connect(n1, n1)
	e3.SourceArrowhead = layoutgraph.TriangleArrowhead
	e3.TargetArrowhead = layoutgraph.NoArrowhead

	e4 := g.Connect(n1, n1)
	e4.SourceArrowhead = layoutgraph.TriangleArrowhead
	e4.TargetArrowhead = layoutgraph.NoArrowhead

	e5 := g.Connect(n1, n1)
	e5.SourceArrowhead = layoutgraph.TriangleArrowhead
	e5.TargetArrowhead = layoutgraph.TriangleArrowhead
	e5.Label = &layoutgraph.Label{
		Position: label.OutsideBottomRight,
		Width:    100,
		Height:   100,
	}

	loops.Route(n1)

	e4TL, e4BR := e4.BoundingBox()
	e4Area := (e4BR.X - e4TL.X) * (e4BR.Y - e4TL.Y)
	e2TL, e2BR := e2.BoundingBox()
	e2Area := (e2BR.X - e2TL.X) * (e2BR.Y - e2TL.Y)
	assert.Greater(t, e4Area, e2Area)
}
