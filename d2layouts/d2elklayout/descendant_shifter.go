package d2elklayout

import "github.com/d2lang/d2/d2graph"

// descendantShifter applies the same geometry updates as
// d2graph.Object.ShiftDescendants while avoiding its repeated whole-graph edge
// scans. ELK performs several sequential margin corrections per object, so the
// shifter deliberately keeps each correction separate: ShiftStart/ShiftEnd may
// simplify a route after each individual horizontal or vertical move.
type descendantShifter struct {
	preorder   map[*d2graph.Object]int
	subtreeEnd map[*d2graph.Object]int
	incident   map[*d2graph.Object][]*d2graph.Edge
	seen       map[*d2graph.Edge]uint64
	candidates []*d2graph.Edge
	generation uint64
}

func newDescendantShifter(g *d2graph.Graph) *descendantShifter {
	s := &descendantShifter{
		preorder:   make(map[*d2graph.Object]int, len(g.Objects)+1),
		subtreeEnd: make(map[*d2graph.Object]int, len(g.Objects)+1),
		incident:   make(map[*d2graph.Object][]*d2graph.Edge, len(g.Objects)),
		seen:       make(map[*d2graph.Edge]uint64, len(g.Edges)),
	}
	next := 0
	var indexSubtree func(*d2graph.Object)
	indexSubtree = func(obj *d2graph.Object) {
		s.preorder[obj] = next
		next++
		for _, child := range obj.ChildrenArray {
			indexSubtree(child)
		}
		s.subtreeEnd[obj] = next
	}
	indexSubtree(g.Root)

	for _, edge := range g.Edges {
		s.incident[edge.Src] = append(s.incident[edge.Src], edge)
		if edge.Dst != edge.Src {
			s.incident[edge.Dst] = append(s.incident[edge.Dst], edge)
		}
	}
	return s
}

func (s *descendantShifter) inSubtree(root, obj *d2graph.Object) bool {
	rootStart, rootOK := s.preorder[root]
	objStart, objOK := s.preorder[obj]
	return rootOK && objOK && rootStart <= objStart && objStart < s.subtreeEnd[root]
}

func (s *descendantShifter) shift(root *d2graph.Object, dx, dy float64) {
	if dx == 0 && dy == 0 {
		return
	}

	s.generation++
	if s.generation == 0 {
		clear(s.seen)
		s.generation = 1
	}
	generation := s.generation
	candidates := s.candidates[:0]
	addIncident := func(obj *d2graph.Object) {
		for _, edge := range s.incident[obj] {
			if s.seen[edge] == generation {
				continue
			}
			s.seen[edge] = generation
			candidates = append(candidates, edge)
		}
	}

	// IsDescendantOf, used by the original implementation's first pass,
	// includes the object itself. Include root's incident edges so root self
	// loops and root-to-descendant edges preserve that behavior.
	addIncident(root)
	root.IterDescendants(func(_ *d2graph.Object, descendant *d2graph.Object) {
		descendant.TopLeft.X += dx
		descendant.TopLeft.Y += dy
		addIncident(descendant)
	})
	s.candidates = candidates

	for _, edge := range candidates {
		srcInSubtree := s.inSubtree(root, edge.Src)
		dstInSubtree := s.inSubtree(root, edge.Dst)
		if srcInSubtree && dstInSubtree {
			for _, point := range edge.Route {
				point.X += dx
				point.Y += dy
			}
			continue
		}

		// The original second pass visits strict descendants only. An edge
		// from root to an outside object therefore remains unchanged.
		if srcInSubtree && edge.Src != root {
			shiftEdgeStart(edge, dx, dy)
		} else if dstInSubtree && edge.Dst != root {
			shiftEdgeEnd(edge, dx, dy)
		}
	}
}

func shiftEdgeStart(edge *d2graph.Edge, dx, dy float64) {
	if dx == 0 {
		edge.ShiftStart(dy, false)
	} else if dy == 0 {
		edge.ShiftStart(dx, true)
	} else {
		edge.Route[0].X += dx
		edge.Route[0].Y += dy
	}
}

func shiftEdgeEnd(edge *d2graph.Edge, dx, dy float64) {
	if dx == 0 {
		edge.ShiftEnd(dy, false)
	} else if dy == 0 {
		edge.ShiftEnd(dx, true)
	} else {
		edge.Route[len(edge.Route)-1].X += dx
		edge.Route[len(edge.Route)-1].Y += dy
	}
}
