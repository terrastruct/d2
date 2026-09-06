package d2elklayout

import (
	"math"

	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/lib/geo"
)

const (
	bendIndexCellSize = 128.0
	bendIndexMaxCells = 256
)

type spatialBounds struct {
	minX float64
	minY float64
	maxX float64
	maxY float64
}

func boundsForSegment(segment geo.Segment) spatialBounds {
	return spatialBounds{
		minX: math.Min(segment.Start.X, segment.End.X),
		minY: math.Min(segment.Start.Y, segment.End.Y),
		maxX: math.Max(segment.Start.X, segment.End.X),
		maxY: math.Max(segment.Start.Y, segment.End.Y),
	}
}

func (bounds spatialBounds) expanded(amount float64) spatialBounds {
	bounds.minX -= amount
	bounds.minY -= amount
	bounds.maxX += amount
	bounds.maxY += amount
	return bounds
}

func (bounds spatialBounds) overlaps(other spatialBounds) bool {
	return bounds.minX <= other.maxX && other.minX <= bounds.maxX &&
		bounds.minY <= other.maxY && other.minY <= bounds.maxY
}

type spatialCell struct {
	x int
	y int
}

type spatialEntry struct {
	bounds spatialBounds
	cells  []spatialCell
	large  bool
}

// spatialIndex is an exact-query broad phase: it may return false positives,
// but every item whose bounds overlap the query is returned. Very large or
// non-finite entries are kept in a small fallback set instead of filling an
// unbounded number of grid cells.
type spatialIndex[T comparable] struct {
	cellSize float64
	maxCells int
	cells    map[spatialCell]map[T]struct{}
	large    map[T]struct{}
	entries  map[T]spatialEntry
	seen     map[T]uint64
	queryID  uint64
}

func newSpatialIndex[T comparable](capacity int) *spatialIndex[T] {
	return &spatialIndex[T]{
		cellSize: bendIndexCellSize,
		maxCells: bendIndexMaxCells,
		cells:    make(map[spatialCell]map[T]struct{}),
		large:    make(map[T]struct{}),
		entries:  make(map[T]spatialEntry, capacity),
		seen:     make(map[T]uint64, capacity),
	}
}

func (index *spatialIndex[T]) add(item T, bounds spatialBounds) {
	index.remove(item)
	cells, ok := index.coveredCells(bounds)
	entry := spatialEntry{bounds: bounds, cells: cells, large: !ok}
	index.entries[item] = entry
	if !ok {
		index.large[item] = struct{}{}
		return
	}
	for _, cell := range cells {
		items := index.cells[cell]
		if items == nil {
			items = make(map[T]struct{})
			index.cells[cell] = items
		}
		items[item] = struct{}{}
	}
}

func (index *spatialIndex[T]) remove(item T) {
	entry, ok := index.entries[item]
	if !ok {
		return
	}
	delete(index.entries, item)
	delete(index.seen, item)
	if entry.large {
		delete(index.large, item)
		return
	}
	for _, cell := range entry.cells {
		items := index.cells[cell]
		delete(items, item)
		if len(items) == 0 {
			delete(index.cells, cell)
		}
	}
}

func (index *spatialIndex[T]) query(bounds spatialBounds, visit func(T)) {
	cells, ok := index.coveredCells(bounds)
	if !ok {
		for item, entry := range index.entries {
			if entry.bounds.overlaps(bounds) {
				visit(item)
			}
		}
		return
	}

	index.queryID++
	if index.queryID == 0 {
		clear(index.seen)
		index.queryID = 1
	}
	queryID := index.queryID
	for _, cell := range cells {
		for item := range index.cells[cell] {
			if index.seen[item] == queryID {
				continue
			}
			index.seen[item] = queryID
			if index.entries[item].bounds.overlaps(bounds) {
				visit(item)
			}
		}
	}
	for item := range index.large {
		if index.seen[item] == queryID {
			continue
		}
		if index.entries[item].bounds.overlaps(bounds) {
			visit(item)
		}
	}
}

func (index *spatialIndex[T]) coveredCells(bounds spatialBounds) ([]spatialCell, bool) {
	if !finiteBounds(bounds) {
		return nil, false
	}
	minX := math.Floor(bounds.minX / index.cellSize)
	maxX := math.Floor(bounds.maxX / index.cellSize)
	minY := math.Floor(bounds.minY / index.cellSize)
	maxY := math.Floor(bounds.maxY / index.cellSize)
	// Keep float-to-int conversion safe on every supported target, including
	// wasm32. Leave one unit of headroom so the inclusive loops below cannot
	// overflow when incrementing their final coordinate.
	const safeCellCoordinate = float64(1<<31 - 2)
	if math.Abs(minX) > safeCellCoordinate || math.Abs(maxX) > safeCellCoordinate ||
		math.Abs(minY) > safeCellCoordinate || math.Abs(maxY) > safeCellCoordinate {
		return nil, false
	}
	width := maxX - minX + 1
	height := maxY - minY + 1
	if width <= 0 || height <= 0 || width > float64(index.maxCells) ||
		height > float64(index.maxCells) || width*height > float64(index.maxCells) {
		return nil, false
	}
	cells := make([]spatialCell, 0, int(width*height))
	for y := int(minY); y <= int(maxY); y++ {
		for x := int(minX); x <= int(maxX); x++ {
			cells = append(cells, spatialCell{x: x, y: y})
		}
	}
	return cells, true
}

func finiteBounds(bounds spatialBounds) bool {
	return !math.IsNaN(bounds.minX) && !math.IsNaN(bounds.minY) &&
		!math.IsNaN(bounds.maxX) && !math.IsNaN(bounds.maxY) &&
		!math.IsInf(bounds.minX, 0) && !math.IsInf(bounds.minY, 0) &&
		!math.IsInf(bounds.maxX, 0) && !math.IsInf(bounds.maxY, 0)
}

type indexedEdgeSegment struct {
	edge    *d2graph.Edge
	segment geo.Segment
}

type bendCollisionIndex struct {
	objects      *spatialIndex[*d2graph.Object]
	segments     *spatialIndex[*indexedEdgeSegment]
	edgeSegments map[*d2graph.Edge][]*indexedEdgeSegment
}

func newBendCollisionIndex(g *d2graph.Graph) *bendCollisionIndex {
	index := &bendCollisionIndex{
		objects:      newSpatialIndex[*d2graph.Object](len(g.Objects)),
		segments:     newSpatialIndex[*indexedEdgeSegment](len(g.Edges) * 3),
		edgeSegments: make(map[*d2graph.Edge][]*indexedEdgeSegment, len(g.Edges)),
	}
	objectBuffer := float64(edge_node_spacing) - 1
	for _, obj := range g.Objects {
		bounds := spatialBounds{
			minX: obj.TopLeft.X,
			minY: obj.TopLeft.Y,
			maxX: obj.TopLeft.X + obj.Width,
			maxY: obj.TopLeft.Y + obj.Height,
		}.expanded(objectBuffer)
		index.objects.add(obj, bounds)
	}
	for _, edge := range g.Edges {
		index.updateEdge(edge)
	}
	return index
}

func (index *bendCollisionIndex) updateEdge(edge *d2graph.Edge) {
	previous := index.edgeSegments[edge]
	for _, segment := range previous {
		index.segments.remove(segment)
	}
	segmentCount := max(0, len(edge.Route)-1)
	segments := previous[:0]
	if cap(segments) < segmentCount {
		segments = make([]*indexedEdgeSegment, 0, segmentCount)
	}
	for i := 0; i+1 < len(edge.Route); i++ {
		var segment *indexedEdgeSegment
		if i < len(previous) {
			segment = previous[i]
		} else {
			segment = &indexedEdgeSegment{}
		}
		segment.edge = edge
		segment.segment = geo.Segment{Start: edge.Route[i], End: edge.Route[i+1]}
		segments = append(segments, segment)
		index.segments.add(segment, boundsForSegment(segment.segment))
	}
	for i := len(segments); i < len(previous); i++ {
		previous[i] = nil
	}
	index.edgeSegments[edge] = segments
}

func (index *bendCollisionIndex) countObjectIntersects(src, dst *d2graph.Object, segment geo.Segment) int {
	count := 0
	index.objects.query(boundsForSegment(segment), func(obj *d2graph.Object) {
		if obj == src || obj == dst {
			return
		}
		if obj.Intersects(segment, float64(edge_node_spacing)-1) {
			count++
		}
	})
	return count
}

func (index *bendCollisionIndex) countEdgeIntersects(sourceEdge *d2graph.Edge, segment geo.Segment) (int, int, int, int) {
	isHorizontal := math.Ceil(segment.Start.Y) == math.Ceil(segment.End.Y)
	crossingsCount := 0
	overlapsCount := 0
	closeOverlapsCount := 0
	touchingCount := 0
	queryBounds := boundsForSegment(segment).expanded(float64(edge_node_spacing) / 2)
	index.segments.query(queryBounds, func(other *indexedEdgeSegment) {
		if other.edge == sourceEdge {
			return
		}
		otherSegment := other.segment
		otherIsHorizontal := math.Ceil(otherSegment.Start.Y) == math.Ceil(otherSegment.End.Y)
		if isHorizontal == otherIsHorizontal {
			if segment.Overlaps(otherSegment, !isHorizontal, 0.) {
				if isHorizontal {
					if math.Abs(segment.Start.Y-otherSegment.Start.Y) < float64(edge_node_spacing)/2. {
						overlapsCount++
						if math.Abs(segment.Start.Y-otherSegment.Start.Y) < float64(edge_node_spacing)/4. {
							closeOverlapsCount++
							if math.Abs(segment.Start.Y-otherSegment.Start.Y) < 1. {
								touchingCount++
							}
						}
					}
				} else {
					if math.Abs(segment.Start.X-otherSegment.Start.X) < float64(edge_node_spacing)/2. {
						overlapsCount++
						if math.Abs(segment.Start.X-otherSegment.Start.X) < float64(edge_node_spacing)/4. {
							closeOverlapsCount++
							// Preserve the existing predicate, including its use of Y
							// for vertical touching segments.
							if math.Abs(segment.Start.Y-otherSegment.Start.Y) < 1. {
								touchingCount++
							}
						}
					}
				}
			}
		} else if segment.Intersects(otherSegment) {
			crossingsCount++
		}
	})
	return crossingsCount, overlapsCount, closeOverlapsCount, touchingCount
}
