package routing

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/d2lang/d2/lib/geo"
)

func (set *ovgEdgeSet) intersectsWith(edge *OVGEdge) bool {
	intersects, _ := set.intersectsWithChecked(edge, nil)
	return intersects
}

func (set *ovgEdgeSet) overlappingEdges(edge *OVGEdge) []OVGEdge {
	overlaps, _ := set.overlappingEdgesChecked(edge, nil)
	return overlaps
}

func TestOvgEdgeSetAdd(t *testing.T) {
	set := newOvgEdgeSet()

	vertical := NewOVGEdge(NewOVGNode(geo.NewPoint(5, 5)), NewOVGNode(geo.NewPoint(5, 15)))
	set.add(vertical)
	assert.Equal(t, 1, len(set.verticalEdges))
	assert.Equal(t, 0, len(set.horizontalEdges))
	if set.verticalEdges[5][0] != *vertical {
		t.Fatal("Expected vertical edge to exist in edge set")
	}

	horizontal := NewOVGEdge(NewOVGNode(geo.NewPoint(5, 5)), NewOVGNode(geo.NewPoint(15, 5)))
	set.add(horizontal)
	assert.Equal(t, 1, len(set.horizontalEdges))
	assert.Equal(t, 1, len(set.verticalEdges))
	if set.horizontalEdges[5][0] != *horizontal {
		t.Fatal("Expected horizontal edge to exist in edge set")
	}

	diagonal := NewOVGEdge(NewOVGNode(geo.NewPoint(5, 10)), NewOVGNode(geo.NewPoint(15, 5)))
	set.add(diagonal)
	assert.Equal(t, 1, len(set.horizontalEdges))
	assert.Equal(t, 1, len(set.verticalEdges))
}

func TestOvgEdgeSetIntersectsWith(t *testing.T) {
	set := newOvgEdgeSet()

	v1 := NewOVGEdge(NewOVGNode(geo.NewPoint(5, 5)), NewOVGNode(geo.NewPoint(5, 15)))
	set.add(v1)
	v2 := NewOVGEdge(NewOVGNode(geo.NewPoint(5, 25)), NewOVGNode(geo.NewPoint(5, 39)))
	set.add(v2)
	v3 := NewOVGEdge(NewOVGNode(geo.NewPoint(7, 5)), NewOVGNode(geo.NewPoint(7, 15)))
	set.add(v3)

	h1 := NewOVGEdge(NewOVGNode(geo.NewPoint(5, 5)), NewOVGNode(geo.NewPoint(7, 5)))
	set.add(h1)
	h2 := NewOVGEdge(NewOVGNode(geo.NewPoint(20, 25)), NewOVGNode(geo.NewPoint(30, 25)))
	set.add(h2)
	h3 := NewOVGEdge(NewOVGNode(geo.NewPoint(0, 40)), NewOVGNode(geo.NewPoint(50, 40)))
	set.add(h3)

	assert.False(t, set.intersectsWith(v1)) // does not consider h1 because they share point (5, 5)
	assert.False(t, set.intersectsWith(h3)) // does not intersect with itself
	assert.False(t, set.intersectsWith(h1)) // does not consider v1 because they share point (5, 5)

	h3Reverse := NewOVGEdge(NewOVGNode(geo.NewPoint(50, 40)), NewOVGNode(geo.NewPoint(0, 40)))
	assert.False(t, set.intersectsWith(h3Reverse)) // does not intersect with itself

	edge := NewOVGEdge(NewOVGNode(geo.NewPoint(6, 4)), NewOVGNode(geo.NewPoint(6, 10)))
	assert.True(t, set.intersectsWith(edge)) // intersects h1

	edge = NewOVGEdge(NewOVGNode(geo.NewPoint(5, 19)), NewOVGNode(geo.NewPoint(5, 50)))
	assert.True(t, set.intersectsWith(edge)) // intersects v2 & h3

	edge = NewOVGEdge(NewOVGNode(geo.NewPoint(1, 9)), NewOVGNode(geo.NewPoint(19, 9)))
	assert.True(t, set.intersectsWith(edge)) // intersects v2 & v3

	edge = NewOVGEdge(NewOVGNode(geo.NewPoint(6, 0)), NewOVGNode(geo.NewPoint(6, 4)))
	assert.False(t, set.intersectsWith(edge))

	edge = NewOVGEdge(NewOVGNode(geo.NewPoint(6, 39)), NewOVGNode(geo.NewPoint(30, 39)))
	assert.False(t, set.intersectsWith(edge))

	// no intersection for diagonals because they shouldn't be part of OVGs
	d1 := NewOVGEdge(NewOVGNode(geo.NewPoint(0, 0)), NewOVGNode(geo.NewPoint(50, 50)))
	assert.False(t, set.intersectsWith(d1))
}

func TestOverlappingEdges(t *testing.T) {
	set := newOvgEdgeSet()

	v1 := NewOVGEdge(NewOVGNode(geo.NewPoint(5, 5)), NewOVGNode(geo.NewPoint(5, 15)))
	set.add(v1)
	v2 := NewOVGEdge(NewOVGNode(geo.NewPoint(5, 25)), NewOVGNode(geo.NewPoint(5, 39)))
	set.add(v2)
	v3 := NewOVGEdge(NewOVGNode(geo.NewPoint(7, 5)), NewOVGNode(geo.NewPoint(7, 15)))
	set.add(v3)

	h1 := NewOVGEdge(NewOVGNode(geo.NewPoint(5, 5)), NewOVGNode(geo.NewPoint(7, 5)))
	set.add(h1)
	h2 := NewOVGEdge(NewOVGNode(geo.NewPoint(20, 25)), NewOVGNode(geo.NewPoint(30, 25)))
	set.add(h2)
	h3 := NewOVGEdge(NewOVGNode(geo.NewPoint(0, 40)), NewOVGNode(geo.NewPoint(50, 40)))
	set.add(h3)

	overlapping := set.overlappingEdges(h1)
	assert.Equal(t, 1, len(overlapping))
	assert.True(t, h1.equals(&overlapping[0]))

	overlapping = set.overlappingEdges(v1)
	assert.Equal(t, 1, len(overlapping))
	assert.True(t, v1.equals(&overlapping[0]))

	edge := NewOVGEdge(NewOVGNode(geo.NewPoint(5, 14)), NewOVGNode(geo.NewPoint(5, 25)))
	overlapping = set.overlappingEdges(edge)
	// ensure nodes are in sequence, so the test does not fail because map is indeterministic
	sort.Slice(overlapping, func(i, j int) bool {
		return overlapping[i].From.Y < overlapping[j].From.Y
	})
	assert.Equal(t, 2, len(overlapping))
	assert.True(t, v1.equals(&overlapping[0]))
	assert.True(t, v2.equals(&overlapping[1]))

	// intersections are not overlaps
	edge = NewOVGEdge(NewOVGNode(geo.NewPoint(6, 0)), NewOVGNode(geo.NewPoint(6, 80)))
	overlapping = set.overlappingEdges(edge)
	assert.Equal(t, 0, len(overlapping))

	// no overlapping for diagonals
	d1 := NewOVGEdge(NewOVGNode(geo.NewPoint(0, 0)), NewOVGNode(geo.NewPoint(50, 50)))
	assert.Equal(t, 0, len(set.overlappingEdges(d1)))
}
