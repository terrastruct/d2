package d2cycle_test

import (
	"context"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"oss.terrastruct.com/d2/d2compiler"
	"oss.terrastruct.com/d2/d2layouts/d2cycle"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/log"
)

// borderDistance returns the distance from p to the border of box (0 if p is
// exactly on the border).
func borderDistance(box *geo.Box, p *geo.Point) float64 {
	x, y := box.TopLeft.X, box.TopLeft.Y
	r, b := x+box.Width, y+box.Height
	dx := math.Max(math.Max(x-p.X, p.X-r), 0)
	dy := math.Max(math.Max(y-p.Y, p.Y-b), 0)
	if dx > 0 || dy > 0 {
		// outside: distance to the box
		return math.Hypot(dx, dy)
	}
	// inside: distance to the closest side
	return math.Min(
		math.Min(p.X-x, r-p.X),
		math.Min(p.Y-y, b-p.Y),
	)
}

func TestCycleLayout(t *testing.T) {
	input := `
shape: cycle
a -> b -> c -> d -> a
`
	g, _, err := d2compiler.Compile("", strings.NewReader(input), nil)
	assert.Nil(t, err)
	for _, obj := range g.Root.ChildrenArray {
		obj.Box = geo.NewBox(nil, 100, 60)
	}

	ctx := log.WithTB(context.Background(), t)
	err = d2cycle.Layout(ctx, g, nil)
	assert.Nil(t, err)

	// nodes are centered on a common circle around the origin
	radii := make([]float64, len(g.Root.ChildrenArray))
	for i, obj := range g.Root.ChildrenArray {
		center := obj.Center()
		radii[i] = math.Hypot(center.X, center.Y)
	}
	for i := 1; i < len(radii); i++ {
		// TopLeft is snapped to integers so allow 1px of tolerance
		assert.InDelta(t, radii[0], radii[i], 1.0)
	}

	for _, edge := range g.Edges {
		assert.True(t, edge.IsCurve)
		// cubic Bézier route: [P0, C1, C2, P1, ...]
		assert.Equal(t, 1, len(edge.Route)%3)

		// the route starts and ends exactly on the shape borders
		start := edge.Route[0]
		end := edge.Route[len(edge.Route)-1]
		assert.Less(t, borderDistance(edge.Src.Box, start), 0.01)
		assert.Less(t, borderDistance(edge.Dst.Box, end), 0.01)

		// every anchor point lies on the same circle around the origin (the
		// arc stays perfectly circular)
		arcRadius := math.Hypot(start.X, start.Y)
		for i := 3; i < len(edge.Route); i += 3 {
			p := edge.Route[i]
			assert.InDelta(t, arcRadius, math.Hypot(p.X, p.Y), 0.01)
		}
	}
}

func TestCycleLayoutNonRectangular(t *testing.T) {
	input := `
shape: cycle
a: {shape: circle}
b: {shape: hexagon}
c
a -> b -> c -> a
`
	g, _, err := d2compiler.Compile("", strings.NewReader(input), nil)
	assert.Nil(t, err)
	for _, obj := range g.Root.ChildrenArray {
		obj.Box = geo.NewBox(nil, 80, 80)
	}

	ctx := log.WithTB(context.Background(), t)
	err = d2cycle.Layout(ctx, g, nil)
	assert.Nil(t, err)

	a, has := g.Root.HasChild([]string{"a"})
	assert.True(t, has)

	for _, edge := range g.Edges {
		assert.True(t, edge.IsCurve)
		start := edge.Route[0]
		end := edge.Route[len(edge.Route)-1]

		// arcs at a circle shape stop on the visible circle perimeter, not
		// its bounding box
		if edge.Src == a {
			center := a.Center()
			assert.InDelta(t, a.Width/2, math.Hypot(start.X-center.X, start.Y-center.Y), 0.5)
		}
		if edge.Dst == a {
			center := a.Center()
			assert.InDelta(t, a.Width/2, math.Hypot(end.X-center.X, end.Y-center.Y), 0.5)
		}
	}
}

func TestCycleLayoutSingleNode(t *testing.T) {
	// a single node must not produce an infinite radius
	input := `
shape: cycle
a
`
	g, _, err := d2compiler.Compile("", strings.NewReader(input), nil)
	assert.Nil(t, err)
	for _, obj := range g.Root.ChildrenArray {
		obj.Box = geo.NewBox(nil, 100, 60)
	}

	ctx := log.WithTB(context.Background(), t)
	err = d2cycle.Layout(ctx, g, nil)
	assert.Nil(t, err)

	obj := g.Root.ChildrenArray[0]
	assert.False(t, math.IsInf(obj.TopLeft.X, 0))
	assert.False(t, math.IsInf(obj.TopLeft.Y, 0))
}
