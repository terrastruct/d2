package d2isometricimg

import (
	"fmt"
	"math"
	"math/bits"
	"sort"
)

type svgVisibleFragment struct {
	points []svgPoint
	box    svgBox
}

// The visible pieces of a broad board can number in the thousands. Index
// those pieces as they change so a small node or ink ribbon only visits the
// local pieces it can cut, rather than scanning every existing fragment.
type svgVisibleFragmentSet struct {
	grid                                      svgVisibilityGrid
	cells                                     []map[int]struct{}
	large                                     map[int]struct{}
	fragments                                 map[int]svgVisibleFragment
	next, vertices, maxFragments, maxVertices int
	budget                                    *svgVisibilityBudget
	references                                int
}

func svgNewVisibleFragmentSet(points []svgPoint, budget *svgVisibilityBudget, maxFragments, maxVertices int) (*svgVisibleFragmentSet, error) {
	box := svgPolygonBox(points)
	w, h := box.maxX-box.minX, box.maxY-box.minY
	nx := max(1, min(32, int(math.Ceil(math.Sqrt(256*w/h)))))
	ny := max(1, min(32, int(math.Ceil(256/float64(nx)))))
	set := &svgVisibleFragmentSet{
		grid:  svgVisibilityGrid{bounds: box, nx: nx, ny: ny},
		cells: make([]map[int]struct{}, nx*ny), large: make(map[int]struct{}), fragments: make(map[int]svgVisibleFragment),
		maxFragments: maxFragments, maxVertices: maxVertices, budget: budget,
	}
	return set, set.add(points)
}

func (s *svgVisibleFragmentSet) changeCells(id int, box svgBox, add bool) error {
	x0, y0, x1, y1, ok := s.grid.cellsFor(box)
	if !ok {
		return nil
	}
	count := (x1 - x0 + 1) * (y1 - y0 + 1)
	large := count > 64
	if large {
		count = 1
	}
	if err := s.budget.spend(count); err != nil {
		return err
	}
	if add {
		if count > s.budget.limits.gridReferences-s.references {
			return fmt.Errorf("isometric SVG visibility exceeds fragment spatial index limit")
		}
		s.references += count
	} else {
		s.references -= count
	}
	if large {
		if add {
			s.large[id] = struct{}{}
		} else {
			delete(s.large, id)
		}
		return nil
	}
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			index := y*s.grid.nx + x
			if add {
				if s.cells[index] == nil {
					s.cells[index] = make(map[int]struct{})
				}
				s.cells[index][id] = struct{}{}
			} else {
				delete(s.cells[index], id)
			}
		}
	}
	return nil
}

func (s *svgVisibleFragmentSet) add(points []svgPoint) error {
	if err := s.budget.spend(len(points) + 1); err != nil {
		return err
	}
	if len(s.fragments) >= s.maxFragments || len(points) > s.maxVertices-s.vertices {
		return fmt.Errorf("isometric SVG visibility exceeds fragment limit")
	}
	fragment := svgVisibleFragment{points: points, box: svgPolygonBox(points)}
	if err := s.changeCells(s.next+1, fragment.box, true); err != nil {
		return err
	}
	s.next++
	s.fragments[s.next] = fragment
	s.vertices += len(points)
	return nil
}

func (s *svgVisibleFragmentSet) query(box svgBox) ([]int, error) {
	seen := make(map[int]bool)
	var result []int
	visit := func(bucket map[int]struct{}) error {
		if err := s.budget.spend(len(bucket)); err != nil {
			return err
		}
		for id := range bucket {
			if seen[id] {
				continue
			}
			seen[id] = true
			if s.fragments[id].box.overlaps(box) {
				result = append(result, id)
			}
		}
		return nil
	}
	if x0, y0, x1, y1, ok := s.grid.cellsFor(box); ok {
		for y := y0; y <= y1; y++ {
			for x := x0; x <= x1; x++ {
				if err := visit(s.cells[y*s.grid.nx+x]); err != nil {
					return nil, err
				}
			}
		}
		if err := visit(s.large); err != nil {
			return nil, err
		}
	}
	if err := s.budget.spend(len(result) * bits.Len(uint(len(result)))); err != nil {
		return nil, err
	}
	sort.Ints(result)
	return result, nil
}

func (s *svgVisibleFragmentSet) subtract(cut []svgPoint) error {
	ids, err := s.query(svgPolygonBox(cut))
	if err != nil {
		return err
	}
	for _, id := range ids {
		fragment := s.fragments[id]
		pieces, err := svgSubtractPolygon(fragment.points, cut, s.budget)
		if err != nil {
			return err
		}
		if len(pieces) == 1 && len(pieces[0]) > 0 && &pieces[0][0] == &fragment.points[0] {
			continue
		}
		if err := s.changeCells(id, fragment.box, false); err != nil {
			return err
		}
		delete(s.fragments, id)
		s.vertices -= len(fragment.points)
		for _, piece := range pieces {
			if err := s.add(piece); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *svgVisibleFragmentSet) polygons() ([][]svgPoint, error) {
	if err := s.budget.spend(len(s.fragments) * bits.Len(uint(len(s.fragments)))); err != nil {
		return nil, err
	}
	ids := make([]int, 0, len(s.fragments))
	for id := range s.fragments {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	polygons := make([][]svgPoint, 0, len(ids))
	for _, id := range ids {
		polygons = append(polygons, s.fragments[id].points)
	}
	return polygons, nil
}
