package d2cycle_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"oss.terrastruct.com/d2/d2ast"
	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2layouts"
	"oss.terrastruct.com/d2/d2layouts/d2cycle"
	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/log"
)

func TestLayoutOrdersObjectsAroundCycle(t *testing.T) {
	g := d2graph.NewGraph()
	g.Root.Shape = d2graph.Scalar{Value: d2target.ShapeCycle}
	g.Root.Box = &geo.Box{}

	a := addCycleNode(g, "a")
	b := addCycleNode(g, "b")
	c := addCycleNode(g, "c")
	d := addCycleNode(g, "d")
	g.Edges = []*d2graph.Edge{{Src: a, Dst: c}}

	ctx := log.WithTB(context.Background(), t)
	require.NoError(t, d2cycle.Layout(ctx, g))

	assert.Equal(t, geo.NewPoint(100, 0), a.TopLeft)
	assert.Equal(t, geo.NewPoint(200, 100), b.TopLeft)
	assert.Equal(t, geo.NewPoint(100, 200), c.TopLeft)
	assert.Equal(t, geo.NewPoint(0, 100), d.TopLeft)
	assert.Equal(t, 420., g.Root.Width)
	assert.Equal(t, 420., g.Root.Height)
	require.Len(t, g.Edges[0].Route, 2)
}

func TestLayoutNestedCycle(t *testing.T) {
	g := d2graph.NewGraph()

	cycle := g.Root.EnsureChild([]d2ast.String{d2ast.FlatUnquotedString("cluster")})
	cycle.Shape = d2graph.Scalar{Value: d2target.ShapeCycle}
	cycle.Box = &geo.Box{}

	a := addCycleChild(g, cycle, "a")
	b := addCycleChild(g, cycle, "b")
	c := addCycleChild(g, cycle, "c")
	d := addCycleChild(g, cycle, "d")
	g.Edges = []*d2graph.Edge{{Src: a, Dst: c}}

	ctx := log.WithTB(context.Background(), t)
	err := d2layouts.LayoutNested(ctx, g, d2layouts.NestedGraphInfo(g.Root), func(ctx context.Context, g *d2graph.Graph) error {
		for _, obj := range g.Objects {
			if obj.TopLeft == nil {
				obj.TopLeft = geo.NewPoint(0, 0)
			}
		}
		return nil
	}, d2layouts.DefaultRouter)
	require.NoError(t, err)

	assert.Equal(t, 420., cycle.Width)
	assert.Equal(t, 420., cycle.Height)
	assert.Equal(t, geo.NewPoint(160, 60), a.TopLeft)
	assert.Equal(t, geo.NewPoint(260, 160), b.TopLeft)
	assert.Equal(t, geo.NewPoint(160, 260), c.TopLeft)
	assert.Equal(t, geo.NewPoint(60, 160), d.TopLeft)
	require.Len(t, g.Edges[0].Route, 2)
}

func addCycleNode(g *d2graph.Graph, id string) *d2graph.Object {
	obj := g.Root.EnsureChild([]d2ast.String{d2ast.FlatUnquotedString(id)})
	obj.Box = geo.NewBox(nil, 100, 100)
	obj.Width = 100
	obj.Height = 100
	return obj
}

func addCycleChild(g *d2graph.Graph, parent *d2graph.Object, id string) *d2graph.Object {
	obj := parent.EnsureChild([]d2ast.String{d2ast.FlatUnquotedString(id)})
	obj.Box = geo.NewBox(nil, 100, 100)
	obj.Width = 100
	obj.Height = 100
	return obj
}
