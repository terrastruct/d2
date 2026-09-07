package routing

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

func TestRouteOpposingColinear(t *testing.T) {
	route := &Route{
		GEdge: &layoutgraph.Edge{
			TargetArrowhead: layoutgraph.TriangleArrowhead,
		},
		OVGNodes: []*OVGNode{
			NewOVGNode(geo.NewPoint(0, 0)),
			NewOVGNode(geo.NewPoint(100, 0)),
			NewOVGNode(geo.NewPoint(100, 100)),
			NewOVGNode(geo.NewPoint(200, 100)),
		},
	}

	// Test horizontal
	from, to := NewOVGNode(geo.NewPoint(0, 0)), NewOVGNode(geo.NewPoint(100, 0))
	assert.False(t, route.isOpposingColinear(from, to))
	assert.True(t, route.isOpposingColinear(to, from))

	// Test vertical
	from, to = NewOVGNode(geo.NewPoint(100, 0)), NewOVGNode(geo.NewPoint(100, 100))
	assert.False(t, route.isOpposingColinear(from, to))
	assert.True(t, route.isOpposingColinear(to, from))

	// Test touch
	from, to = NewOVGNode(geo.NewPoint(190, 0)), NewOVGNode(geo.NewPoint(100, 0))
	assert.False(t, route.isOpposingColinear(from, to))
	assert.False(t, route.isOpposingColinear(to, from))

	// Test partial
	from, to = NewOVGNode(geo.NewPoint(90, 0)), NewOVGNode(geo.NewPoint(120, 0))
	assert.False(t, route.isOpposingColinear(from, to))
	assert.True(t, route.isOpposingColinear(to, from))

	// Test non intersecting
	from, to = NewOVGNode(geo.NewPoint(101, 0)), NewOVGNode(geo.NewPoint(200, 0))
	assert.False(t, route.isOpposingColinear(from, to))
	assert.False(t, route.isOpposingColinear(to, from))

	// Test non colinear
	from, to = NewOVGNode(geo.NewPoint(0, 0)), NewOVGNode(geo.NewPoint(100, 100))
	assert.False(t, route.isOpposingColinear(from, to))
	assert.False(t, route.isOpposingColinear(to, from))
	from, to = NewOVGNode(geo.NewPoint(120, 100)), NewOVGNode(geo.NewPoint(300, 100))
	assert.False(t, route.isOpposingColinear(from, to))
	assert.False(t, route.isOpposingColinear(to, from))
}
