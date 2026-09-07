package routing

import (
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

type pointerSnapshot[T any] struct {
	pointer *T
	value   T
}

func snapshotPointer[T any](pointer *T) pointerSnapshot[T] {
	if pointer == nil {
		return pointerSnapshot[T]{}
	}
	return pointerSnapshot[T]{pointer: pointer, value: *pointer}
}

func (snapshot pointerSnapshot[T]) restore() *T {
	if snapshot.pointer == nil {
		return nil
	}
	*snapshot.pointer = snapshot.value
	return snapshot.pointer
}

type exactSliceSnapshot[S ~[]V, V any] struct {
	original S
	backing  []V
}

func (snapshot exactSliceSnapshot[S, V]) restore() S {
	if snapshot.original == nil {
		return nil
	}
	copy(snapshot.original[:cap(snapshot.original)], snapshot.backing)
	return snapshot.original
}

type edgeSnapshot struct {
	value layoutgraph.Edge

	d2ID                 pointerSnapshot[string]
	points               exactSliceSnapshot[[]*geo.Point, *geo.Point]
	pointValues          []pointerSnapshot[geo.Point]
	label                pointerSnapshot[layoutgraph.Label]
	sourceArrowheadLabel pointerSnapshot[layoutgraph.Label]
	targetArrowheadLabel pointerSnapshot[layoutgraph.Label]
	fromTableColumnIndex pointerSnapshot[int]
	toTableColumnIndex   pointerSnapshot[int]
}

func (snapshot edgeSnapshot) restore(edge *layoutgraph.Edge) {
	snapshot.d2ID.restore()
	snapshot.label.restore()
	snapshot.sourceArrowheadLabel.restore()
	snapshot.targetArrowheadLabel.restore()
	snapshot.fromTableColumnIndex.restore()
	snapshot.toTableColumnIndex.restore()
	for _, point := range snapshot.pointValues {
		point.restore()
	}
	originalPoints := snapshot.points.restore()

	*edge = snapshot.value
	edge.D2ID = snapshot.d2ID.pointer
	edge.Points = originalPoints
	edge.Label = snapshot.label.pointer
	edge.SourceArrowheadLabel = snapshot.sourceArrowheadLabel.pointer
	edge.TargetArrowheadLabel = snapshot.targetArrowheadLabel.pointer
	edge.FromTableColumnIndex = snapshot.fromTableColumnIndex.pointer
	edge.ToTableColumnIndex = snapshot.toTableColumnIndex.pointer
}
