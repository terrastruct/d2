package routing

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

func TestPreferLaunchingVertically(t *testing.T) {
	t.Parallel()

	graph := layoutgraph.NewGraph()

	a := layoutgraph.NewNode(1, 5, 5)
	b := layoutgraph.NewNode(2, 5, 5)
	a.TopLeft = geo.NewPoint(10, 10)
	b.TopLeft = geo.NewPoint(20, 20)
	graph.AddNode(a)
	graph.AddNode(b)

	_, strong := preferLaunchingVertically(a, b, geo.TopLeft)
	assert.Equal(t, false, strong)

	graph.Connect(a, b)

	c := layoutgraph.NewNode(3, 5, 5)
	c.TopLeft = geo.NewPoint(10, 20)
	graph.AddNode(c)

	graph.Connect(a, c)

	pref, strong := preferLaunchingVertically(a, b, geo.TopLeft)
	assert.Equal(t, false, pref)
	assert.Equal(t, true, strong)

	c.TopLeft = geo.NewPoint(20, 10)
	pref, strong = preferLaunchingVertically(a, b, geo.TopLeft)
	assert.Equal(t, true, pref)
	assert.Equal(t, true, strong)
}

func TestFillPath(t *testing.T) {
	t.Parallel()

	graph := layoutgraph.NewGraph()

	a := layoutgraph.NewNode(1, 4, 4)
	b := layoutgraph.NewNode(2, 4, 4)
	a.TopLeft = geo.NewPoint(10, 10)
	b.TopLeft = geo.NewPoint(20, 20)
	graph.AddNode(a)
	graph.AddNode(b)

	ovg := NewOVG(graph.Nodes)

	aCenter := ovg.AddNode(NewOVGNode(geo.NewPoint(12, 12)))
	aPort := ovg.AddNode(NewOVGNode(geo.NewPoint(12, 14)))

	inPathNode := ovg.AddNode(NewOVGNode(geo.NewPoint(12, 16)))
	notInPathNode := ovg.AddNode(NewOVGNode(geo.NewPoint(10, 16)))

	bPort := ovg.AddNode(NewOVGNode(geo.NewPoint(20, 22)))
	bCenter := ovg.AddNode(NewOVGNode(geo.NewPoint(22, 22)))

	anchor := ovg.AddNode(NewOVGNode(geo.NewPoint(12, 22)))

	ovg.Connect(aCenter, aPort)
	ovg.Connect(aPort, inPathNode)
	ovg.Connect(aPort, notInPathNode)
	ovg.Connect(inPathNode, anchor)
	ovg.Connect(anchor, bPort)
	ovg.Connect(bPort, bCenter)

	guard, err := newRouteSearchWorkGuard(t.Context(), Default, maxRouteSearchWorkUnits)
	if err != nil {
		t.Fatal(err)
	}
	router := &ovgEdgeRouter{work: guard}
	got, ok, err := router.fillPathGuarded(aPort, anchor)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, true, ok)
	exp := []*OVGNode{aPort, inPathNode}
	assert.Equal(t, len(exp), len(got))
	for i := 0; i < len(exp); i++ {
		assert.Equal(t, exp[i].X, got[i].X)
		assert.Equal(t, exp[i].Y, got[i].Y)
	}
}

func TestSlingshot(t *testing.T) {
	t.Parallel()

	graph := layoutgraph.NewGraph()

	a := layoutgraph.NewNode(1, 40, 40)
	b := layoutgraph.NewNode(2, 40, 40)
	a.TopLeft = geo.NewPoint(0, 0)
	b.TopLeft = geo.NewPoint(100, 100)
	graph.AddNode(a)
	graph.AddNode(b)
	graph.Connect(a, b)

	ovg, err := buildOVGFromGraph(context.Background(), graph, nil)
	if err != nil {
		t.Fatal(err)
	}
	router, _ := newOVGEdgeRouter(context.Background(), Default, ovg, graph, []*Route{}, graph.Edges)

	// Works in isolation
	ctx := withTestLogger(context.Background(), t)
	_, distance, err := router.slingshot(ctx, graph.Edges[0])
	assert.Nil(t, err)
	assert.Greater(t, distance, 0.0)

	// Works when a node blocks one side
	c := layoutgraph.NewNode(3, 10, 40)
	c.TopLeft = geo.NewPoint(80, 0)
	graph.AddNode(c)

	ovg, err = buildOVGFromGraph(context.Background(), graph, nil)
	if err != nil {
		t.Fatal(err)
	}
	router, _ = newOVGEdgeRouter(context.Background(), Default, ovg, graph, []*Route{}, graph.Edges)

	_, distance2, err := router.slingshot(ctx, graph.Edges[0])
	assert.Nil(t, err)
	assert.Equal(t, distance, distance2)

	// Goes to s-shape when both sides are blocked
	d := layoutgraph.NewNode(4, 10, 40)
	d.TopLeft = geo.NewPoint(0, 80)
	graph.AddNode(d)

	ovg, err = buildOVGFromGraph(context.Background(), graph, nil)
	if err != nil {
		t.Fatal(err)
	}
	router, _ = newOVGEdgeRouter(context.Background(), Default, ovg, graph, []*Route{}, graph.Edges)

	_, distance3, err := router.slingshot(ctx, graph.Edges[0])
	assert.Nil(t, err)
	// s-shapes cost more
	assert.Greater(t, distance3, distance)
	assert.Greater(t, distance3, distance2)

	// block all slingshot routes
	e := layoutgraph.NewNode(5, 1000, 10)
	e.TopLeft = geo.NewPoint(0, 20)
	graph.AddNode(e)

	ovg, err = buildOVGFromGraph(context.Background(), graph, nil)
	if err != nil {
		t.Fatal(err)
	}
	router, _ = newOVGEdgeRouter(context.Background(), Default, ovg, graph, []*Route{}, graph.Edges)

	_, distance4, err := router.slingshot(ctx, graph.Edges[0])
	assert.Nil(t, err)
	assert.Equal(t, math.Inf(1), distance4)
}
