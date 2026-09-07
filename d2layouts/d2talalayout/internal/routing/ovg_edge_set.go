package routing

import "sort"

// Keeps OVG edges as one set for vertical edges and other set of horizontal ones.
// Can quickly check for intersections/overlaps
type ovgEdgeSet struct {
	horizontalEdges map[float64][]OVGEdge
	verticalEdges   map[float64][]OVGEdge
	horizontals     []float64
	verticals       []float64
	edges           map[OVGEdge]struct{}
}

func newOvgEdgeSet() *ovgEdgeSet {
	return &ovgEdgeSet{
		horizontalEdges: make(map[float64][]OVGEdge, 16),
		verticalEdges:   make(map[float64][]OVGEdge, 16),
		horizontals:     make([]float64, 0, 16),
		verticals:       make([]float64, 0, 16),
		edges:           make(map[OVGEdge]struct{}, 16),
	}
}

// insertSortedFloat64 inserts a value into a sorted slice while maintaining order
// Uses binary search to find insertion point - O(log n) instead of O(n log n)
func (set *ovgEdgeSet) insertSortedFloat64(slice *[]float64, val float64) {
	i := sort.SearchFloat64s(*slice, val)
	if i < len(*slice) && (*slice)[i] == val {
		return // Already exists
	}
	*slice = append(*slice, 0)
	copy((*slice)[i+1:], (*slice)[i:])
	(*slice)[i] = val
}

// insertSortedEdge inserts an edge into a sorted edge slice while maintaining order
// Edges are sorted by their minimum coordinate (Y for vertical, X for horizontal)
func (set *ovgEdgeSet) insertSortedEdge(edges *[]OVGEdge, edge OVGEdge, isVertical bool) {
	getMinCoord := func(e OVGEdge) float64 {
		if isVertical {
			if e.From.Y < e.To.Y {
				return e.From.Y
			}
			return e.To.Y
		} else {
			if e.From.X < e.To.X {
				return e.From.X
			}
			return e.To.X
		}
	}

	edgeMinCoord := getMinCoord(edge)

	// Binary search for insertion point
	i := sort.Search(len(*edges), func(j int) bool {
		return getMinCoord((*edges)[j]) >= edgeMinCoord
	})

	*edges = append(*edges, OVGEdge{})
	copy((*edges)[i+1:], (*edges)[i:])
	(*edges)[i] = edge
}

func (set *ovgEdgeSet) add(edge *OVGEdge) {
	if _, exists := set.edges[*edge]; exists {
		return
	}
	set.edges[*edge] = struct{}{}

	if edge.isVertical() {
		if _, exists := set.verticalEdges[edge.From.X]; !exists {
			set.verticalEdges[edge.From.X] = make([]OVGEdge, 0, 16)
			set.insertSortedFloat64(&set.verticals, edge.From.X)
		}
		edges := set.verticalEdges[edge.From.X]
		set.insertSortedEdge(&edges, *edge, true)
		set.verticalEdges[edge.From.X] = edges
	} else if edge.isHorizontal() {
		if _, exists := set.horizontalEdges[edge.From.Y]; !exists {
			set.horizontalEdges[edge.From.Y] = make([]OVGEdge, 0, 16)
			set.insertSortedFloat64(&set.horizontals, edge.From.Y)
		}
		edges := set.horizontalEdges[edge.From.Y]
		set.insertSortedEdge(&edges, *edge, false)
		set.horizontalEdges[edge.From.Y] = edges
	}
}

// addGuarded reserves every sorted-slice shift before mutating the set. The
// duplicate lookup itself is also accounted, and a guard failure leaves the set
// unchanged.
func (set *ovgEdgeSet) addGuarded(edge *OVGEdge, guard workBudget) error {
	if err := guard.step(); err != nil {
		return err
	}
	if _, exists := set.edges[*edge]; exists {
		return nil
	}
	if edge.isVertical() {
		if _, exists := set.verticalEdges[edge.From.X]; !exists {
			if err := guard.add(uint64(len(set.verticals)) + 1); err != nil {
				return err
			}
		}
		if err := guard.add(uint64(len(set.verticalEdges[edge.From.X])) + 1); err != nil {
			return err
		}
	} else if edge.isHorizontal() {
		if _, exists := set.horizontalEdges[edge.From.Y]; !exists {
			if err := guard.add(uint64(len(set.horizontals)) + 1); err != nil {
				return err
			}
		}
		if err := guard.add(uint64(len(set.horizontalEdges[edge.From.Y])) + 1); err != nil {
			return err
		}
	}
	set.add(edge)
	return guard.check()
}

func (set *ovgEdgeSet) intersectsWithGuarded(edge *OVGEdge, guard workBudget) (bool, error) {
	return set.intersectsWithChecked(edge, guard)
}

func (set *ovgEdgeSet) intersectsWithChecked(edge *OVGEdge, guard workBudget) (bool, error) {
	step := func() error {
		if guard == nil {
			return nil
		}
		return guard.step()
	}
	if edge.isHorizontal() {
		minX, maxX := edge.From.X, edge.To.X
		if maxX < minX {
			minX, maxX = maxX, minX
		}

		// check for overlaps
		for _, e := range set.horizontalEdges[edge.From.Y] {
			if err := step(); err != nil {
				return false, err
			}
			if e.From.X < e.To.X {
				if maxX <= e.From.X {
					// horizontalEdges is ordered by minX, so no following edges have a minX less than this one
					break
				}
				if e.To.X <= minX {
					continue
				}
			} else {
				if maxX <= e.To.X {
					break
				}
				if e.From.X <= minX {
					continue
				}
			}
			if !edge.sharePoints(e) {
				return true, nil
			}
		}

		y := edge.From.Y
		// The axes are sorted. Charge the skipped prefix as before, but begin
		// geometry checks at the first line that can cross this segment.
		start := sort.Search(len(set.verticals), func(i int) bool { return !(set.verticals[i] < minX) })
		if err := chargeSkippedRouteWork(guard, start); err != nil {
			return false, err
		}
		// check for crossings
		for _, lineCoordinate := range set.verticals[start:] {
			if err := step(); err != nil {
				return false, err
			}
			// when edge is horizontal, lineCoordinate is the X of vertical lines
			// so we filter out the vertical lines outside the span of edge as they won't intersect
			// similarly, it happens when edge is vertical and we filter the horizontal ones
			if lineCoordinate < minX {
				continue
			}
			if maxX < lineCoordinate {
				break
			}
			for _, e := range set.verticalEdges[lineCoordinate] {
				if err := step(); err != nil {
					return false, err
				}
				if e.From.Y < e.To.Y {
					if y < e.From.Y {
						break
					}
					if e.To.Y < y {
						continue
					}
				} else {
					if y < e.To.Y {
						break
					}
					if e.From.Y < y {
						continue
					}
				}
				if !edge.sharePoints(e) {
					return true, nil
				}
			}
		}
	} else if edge.isVertical() {
		minY, maxY := edge.From.Y, edge.To.Y
		if maxY < minY {
			minY, maxY = maxY, minY
		}
		// check for overlaps
		for _, e := range set.verticalEdges[edge.From.X] {
			if err := step(); err != nil {
				return false, err
			}
			if e.From.Y < e.To.Y {
				if maxY <= e.From.Y {
					// verticalEdges is ordered by minY, so no following edges have a minY less than this one
					break
				}
				if e.To.Y <= minY {
					continue
				}
			} else {
				if maxY <= e.To.Y {
					break
				}
				if e.From.Y <= minY {
					continue
				}
			}
			if !edge.sharePoints(e) {
				return true, nil
			}
		}

		x := edge.From.X
		start := sort.Search(len(set.horizontals), func(i int) bool { return !(set.horizontals[i] < minY) })
		if err := chargeSkippedRouteWork(guard, start); err != nil {
			return false, err
		}
		for _, lineCoordinate := range set.horizontals[start:] {
			if err := step(); err != nil {
				return false, err
			}
			if lineCoordinate < minY {
				continue
			}
			if maxY < lineCoordinate {
				break
			}
			for _, e := range set.horizontalEdges[lineCoordinate] {
				if err := step(); err != nil {
					return false, err
				}
				if e.From.X < e.To.X {
					if x < e.From.X {
						break
					}
					if e.To.X < x {
						continue
					}
				} else {
					if x < e.To.X {
						break
					}
					if e.From.X < x {
						continue
					}
				}
				if !edge.sharePoints(e) {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

func (set *ovgEdgeSet) overlappingEdgesGuarded(edge *OVGEdge, guard workBudget) ([]OVGEdge, error) {
	return set.overlappingEdgesChecked(edge, guard)
}

func (set *ovgEdgeSet) overlappingEdgesChecked(edge *OVGEdge, guard workBudget) ([]OVGEdge, error) {
	var overlapping []OVGEdge
	var maybeOverlap []OVGEdge
	if edge.isVertical() {
		maybeOverlap = set.verticalEdges[edge.From.X]
	} else if edge.isHorizontal() {
		maybeOverlap = set.horizontalEdges[edge.From.Y]
	}

	for _, e := range maybeOverlap {
		if guard != nil {
			if err := guard.step(); err != nil {
				return nil, err
			}
		}
		if intersects(edge.From.Point, edge.To.Point, e.From.Point, e.To.Point) {
			overlapping = append(overlapping, e)
		}
	}

	return overlapping, nil
}
