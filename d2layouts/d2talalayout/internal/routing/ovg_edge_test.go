package routing

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/d2lang/d2/lib/geo"
)

func TestIsVertical(t *testing.T) {
	vertical := NewOVGEdge(
		NewOVGNode(geo.NewPoint(5, 5)),
		NewOVGNode(geo.NewPoint(5, 15)),
	)
	assert.True(t, vertical.isVertical())

	horizontal := NewOVGEdge(
		NewOVGNode(geo.NewPoint(5, 15)),
		NewOVGNode(geo.NewPoint(35, 15)),
	)
	assert.False(t, horizontal.isVertical())

	diagonal := NewOVGEdge(
		NewOVGNode(geo.NewPoint(5, 15)),
		NewOVGNode(geo.NewPoint(35, 55)),
	)
	assert.False(t, diagonal.isVertical())
}
