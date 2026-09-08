package routing

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

func TestDistanceToBoundary(t *testing.T) {
	a := NewOVGNode(geo.NewPoint(0, 0))
	b := layoutgraph.NewNode(1, 1, 1)
	b.TopLeft = geo.NewPoint(6, 0)
	assert.Equal(t, 6.0, a.distanceToBoundary(b))

	a.Point = geo.NewPoint(10, 10)
	b.TopLeft = geo.NewPoint(5, 6)
	assert.Equal(t, 5.0, a.distanceToBoundary(b))
}

func TestAddEdge(t *testing.T) {
	n1 := NewOVGNode(geo.NewPoint(0, 0))
	n2 := NewOVGNode(geo.NewPoint(1, 1))
	e := NewOVGEdge(n1, n2)

	n1.addEdge(e)

	assert.Equal(t, 1, len(n1.Edges))
	assert.Equal(t, 0, len(n2.Edges))
}

func TestAdjacent(t *testing.T) {
	n1 := NewOVGNode(geo.NewPoint(0, 0))
	n2 := NewOVGNode(geo.NewPoint(1, 1))
	e := NewOVGEdge(n1, n2)

	assert.Equal(t, n2, n1.adjacent(e))
	assert.Equal(t, n1, n2.adjacent(e))
}

func TestPortDirectionsForObstacleWithoutOwnerAreUnrestricted(t *testing.T) {
	n := NewOVGNode(geo.NewPoint(0, 0))
	owner := layoutgraph.NewNode(1, 10, 10)

	directions := n.portDirectionsForObstacle(owner)
	assert.Zero(t, directions)
	assert.True(t, directions.any(func(direction geo.Orientation) bool {
		return direction == geo.NONE
	}))
}
