package packing

import (
	"slices"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

type exactTestSlice[T comparable] struct {
	header  []T
	backing []T
}

func captureExactTestSlice[T comparable](values []T) exactTestSlice[T] {
	return exactTestSlice[T]{header: values, backing: slices.Clone(values[:cap(values)])}
}

func (snapshot exactTestSlice[T]) assertRestored(t *testing.T, got []T, name string) {
	t.Helper()
	if len(got) != len(snapshot.header) || cap(got) != cap(snapshot.header) {
		t.Fatalf("%s header = len %d cap %d; want len %d cap %d", name, len(got), cap(got), len(snapshot.header), cap(snapshot.header))
	}
	if cap(got) > 0 && &got[:cap(got)][0] != &snapshot.header[:cap(snapshot.header)][0] {
		t.Fatalf("%s backing array identity changed", name)
	}
	if !slices.Equal(got[:cap(got)], snapshot.backing) {
		t.Fatalf("%s backing array contents changed", name)
	}
}

type exactRouteTestSnapshot struct {
	edge   *layoutgraph.Edge
	route  exactTestSlice[*geo.Point]
	values map[*geo.Point]geo.Point
}

func captureExactRouteTest(edge *layoutgraph.Edge) exactRouteTestSnapshot {
	values := make(map[*geo.Point]geo.Point)
	for _, point := range edge.Points[:cap(edge.Points)] {
		if point != nil {
			values[point] = *point
		}
	}
	return exactRouteTestSnapshot{
		edge:   edge,
		route:  captureExactTestSlice(edge.Points),
		values: values,
	}
}

func (snapshot exactRouteTestSnapshot) changed() bool {
	points := snapshot.edge.Points
	if len(points) != len(snapshot.route.header) || cap(points) != cap(snapshot.route.header) {
		return true
	}
	if cap(points) > 0 && &points[:cap(points)][0] != &snapshot.route.header[:cap(snapshot.route.header)][0] {
		return true
	}
	for index, point := range points[:cap(points)] {
		if point != snapshot.route.backing[index] {
			return true
		}
	}
	for point, value := range snapshot.values {
		if *point != value {
			return true
		}
	}
	return false
}

func (snapshot exactRouteTestSnapshot) assertRestored(t *testing.T) {
	t.Helper()
	snapshot.route.assertRestored(t, snapshot.edge.Points, "edge route")
	for point, value := range snapshot.values {
		if *point != value {
			t.Fatalf("point %p = %+v; want %+v", point, *point, value)
		}
	}
}

func routeWithSpareCapacity(points ...*geo.Point) []*geo.Point {
	backing := make([]*geo.Point, len(points)+3)
	copy(backing, points)
	for index := len(points); index < len(backing); index++ {
		backing[index] = geo.NewPoint(float64(10_000+index), float64(20_000+index))
	}
	return backing[:len(points)]
}
