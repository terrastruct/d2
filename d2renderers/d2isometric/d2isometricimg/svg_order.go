package d2isometricimg

import (
	"container/heap"
	"context"
	"fmt"
	"math"
	"math/bits"
	"sort"
)

// A large face's centroid can be nearer than a label on its far end, even
// though that label is above the face everywhere they overlap. Order complete
// source paint groups using depth at their actual projected intersections.
// This preserves group opacity and the existing order of substrate/decal ink.
func svgOrderPaintUnits(ctx context.Context, units []*svgPaintUnit) error {
	_, err := svgOrderPaintUnitsWithLimits(ctx, units, svgDefaultVisibilityLimits)
	return err
}

type svgPaintOrderStats struct{ relations, cyclicGroups, conflictingPairs int }

type svgOrderPolygon struct {
	points []svgPoint
	plane  svgDepthPlane
	box    svgBox
}

func svgOrderPaintUnitsWithLimits(ctx context.Context, units []*svgPaintUnit, limits svgVisibilityLimits) (svgPaintOrderStats, error) {
	var stats svgPaintOrderStats
	budget := &svgVisibilityBudget{ctx: ctx, limits: limits}
	if len(units) > limits.faces {
		return stats, fmt.Errorf("isometric SVG paint order exceeds unit limit")
	}
	polygons := make([][]svgOrderPolygon, len(units))
	boxes := make([]svgVisibilityPolygon, len(units))
	faces := make([]svgVisibilityFace, len(units))
	vertices, fragments := 0, 0
	for i, u := range units {
		box := svgBox{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
		for _, batch := range u.batches {
			for _, points := range batch.polygons {
				if err := budget.spend(len(points) + 1); err != nil {
					return stats, err
				}
				plane, ok := svgPolygonPlane(points)
				if !ok {
					continue
				}
				vertices += len(points)
				fragments++
				if vertices > limits.vertices || fragments > limits.fragments {
					return stats, fmt.Errorf("isometric SVG paint order exceeds polygon limit")
				}
				bounds := svgPolygonBox(points)
				polygons[i] = append(polygons[i], svgOrderPolygon{points: points, plane: plane, box: bounds})
				box.minX, box.minY = math.Min(box.minX, bounds.minX), math.Min(box.minY, bounds.minY)
				box.maxX, box.maxY = math.Max(box.maxX, bounds.maxX), math.Max(box.maxY, bounds.maxY)
			}
		}
		if len(polygons[i]) > 0 {
			boxes[i] = svgVisibilityPolygon{box: box, points: polygons[i][0].points}
			faces[i].opaque = true // Index every paint unit, including alpha surfaces.
		}
	}
	trees := make([]*svgOrderTree, len(polygons))
	for i, p := range polygons {
		var err error
		trees[i], err = svgNewOrderTree(p, budget)
		if err != nil {
			return stats, err
		}
	}
	grid, err := svgNewVisibilityGrid(boxes, faces, budget)
	if err != nil {
		return stats, err
	}
	edges := make([][]int, len(units))
	seen := make([]int, len(units))
	for i, box := range boxes {
		if err := budget.spend(1); err != nil {
			return stats, err
		}
		if grid == nil || len(box.points) == 0 {
			continue
		}
		visit := func(candidates []int) error {
			if err := budget.spend(len(candidates)); err != nil {
				return err
			}
			for _, j := range candidates {
				if j <= i || seen[j] == i+1 {
					continue
				}
				seen[j] = i + 1
				if !box.box.overlaps(boxes[j].box) {
					continue
				}
				relation, err := svgOrderRelation(trees[i], trees[j], units[i].first, units[j].first, budget)
				if err != nil {
					return err
				}
				if relation == 3 {
					stats.conflictingPairs++
				}
				if relation&1 != 0 {
					edges[i] = append(edges[i], j)
					stats.relations++
				}
				if relation&2 != 0 {
					edges[j] = append(edges[j], i)
					stats.relations++
				}
				if stats.relations > min(limits.gridReferences, 1000000) {
					return fmt.Errorf("isometric SVG paint order exceeds relation limit")
				}
			}
			return nil
		}
		if x0, y0, x1, y1, ok := grid.cellsFor(box.box); ok {
			for y := y0; y <= y1; y++ {
				for x := x0; x <= x1; x++ {
					if err := visit(grid.cells[y*grid.nx+x]); err != nil {
						return stats, err
					}
				}
			}
			if err := visit(grid.large); err != nil {
				return stats, err
			}
		}
	}
	components, membership, err := svgOrderComponents(edges, budget)
	if err != nil {
		return stats, err
	}
	for _, component := range components {
		if len(component) > 1 {
			stats.cyclicGroups++
		}
		// A true intersection/cycle between indivisible source opacity groups has
		// no single correct painter order. Keep the native renderer's stable order
		// inside that component, without violating any external depth relation.
		sort.SliceStable(component, func(a, b int) bool { return svgOrderLess(units, component[a], component[b]) })
	}
	successors := make([][]int, len(components))
	indegree := make([]int, len(components))
	for from, list := range edges {
		for _, to := range list {
			a, b := membership[from], membership[to]
			if a == b {
				continue
			}
			successors[a] = append(successors[a], b)
			indegree[b]++
		}
	}
	ready := &svgOrderHeap{units: units, components: components}
	for i, n := range indegree {
		if n == 0 {
			heap.Push(ready, i)
		}
	}
	ordered := make([]*svgPaintUnit, 0, len(units))
	for ready.Len() > 0 {
		if err := budget.spend(1); err != nil {
			return stats, err
		}
		component := heap.Pop(ready).(int)
		for _, i := range components[component] {
			ordered = append(ordered, units[i])
		}
		for _, to := range successors[component] {
			indegree[to]--
			if indegree[to] == 0 {
				heap.Push(ready, to)
			}
		}
	}
	if len(ordered) != len(units) {
		return stats, fmt.Errorf("isometric SVG paint order has invalid component graph")
	}
	copy(units, ordered)
	return stats, nil
}

// Return 1 for a→b, 2 for b→a, or 3 for a genuine crossing. Shared edges have
// no overlap area and impose no paint order. DepthBias is already in points.z.
// Index the visible fragments once per source paint group. A large board may
// have thousands of small openings after clipping; comparing every fragment
// with every fragment of each overlapping object would be quadratic.
type svgOrderTree struct {
	polygons []svgOrderPolygon
	indices  []int
	nodes    []svgOrderTreeNode
}
type svgOrderTreeNode struct {
	box                     svgBox
	start, end, left, right int
}

func svgNewOrderTree(polygons []svgOrderPolygon, budget *svgVisibilityBudget) (*svgOrderTree, error) {
	tree := &svgOrderTree{polygons: polygons, indices: make([]int, len(polygons))}
	for i := range tree.indices {
		tree.indices[i] = i
	}
	var build func(int, int) (int, error)
	build = func(start, end int) (int, error) {
		if err := budget.spend(end - start + 1); err != nil {
			return 0, err
		}
		box := svgBox{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
		for _, i := range tree.indices[start:end] {
			b := polygons[i].box
			box.minX, box.minY = math.Min(box.minX, b.minX), math.Min(box.minY, b.minY)
			box.maxX, box.maxY = math.Max(box.maxX, b.maxX), math.Max(box.maxY, b.maxY)
		}
		index := len(tree.nodes)
		tree.nodes = append(tree.nodes, svgOrderTreeNode{box: box, start: start, end: end, left: -1, right: -1})
		if end-start <= 8 {
			return index, nil
		}
		if err := budget.spend((end - start) * bits.Len(uint(end-start))); err != nil {
			return 0, err
		}
		x := box.maxX-box.minX >= box.maxY-box.minY
		sort.Slice(tree.indices[start:end], func(i, j int) bool {
			ai, bi := tree.indices[start+i], tree.indices[start+j]
			a, b := polygons[ai].box, polygons[bi].box
			av, bv := a.minY+a.maxY, b.minY+b.maxY
			if x {
				av, bv = a.minX+a.maxX, b.minX+b.maxX
			}
			if av == bv {
				return ai < bi
			}
			return av < bv
		})
		middle := (start + end) / 2
		left, err := build(start, middle)
		if err != nil {
			return 0, err
		}
		right, err := build(middle, end)
		if err != nil {
			return 0, err
		}
		tree.nodes[index].left, tree.nodes[index].right = left, right
		return index, nil
	}
	if len(polygons) > 0 {
		if _, err := build(0, len(polygons)); err != nil {
			return nil, err
		}
	}
	return tree, nil
}
func (tree *svgOrderTree) visit(box svgBox, budget *svgVisibilityBudget, fn func(svgOrderPolygon) (bool, error)) (bool, error) {
	var walk func(int) (bool, error)
	walk = func(index int) (bool, error) {
		if err := budget.spend(1); err != nil {
			return false, err
		}
		node := tree.nodes[index]
		if !box.overlaps(node.box) {
			return false, nil
		}
		if node.left >= 0 {
			stop, err := walk(node.left)
			if stop || err != nil {
				return stop, err
			}
			return walk(node.right)
		}
		for _, i := range tree.indices[node.start:node.end] {
			if err := budget.spend(1); err != nil {
				return false, err
			}
			p := tree.polygons[i]
			if !box.overlaps(p.box) {
				continue
			}
			stop, err := fn(p)
			if stop || err != nil {
				return stop, err
			}
		}
		return false, nil
	}
	if len(tree.nodes) == 0 {
		return false, nil
	}
	return walk(0)
}
func svgOrderRelation(a, b *svgOrderTree, sourceA, sourceB int, budget *svgVisibilityBudget) (int, error) {
	swapped := len(a.polygons) > len(b.polygons)
	if swapped {
		a, b = b, a
		sourceA, sourceB = sourceB, sourceA
	}
	relation := 0
	for _, left := range a.polygons {
		stop, err := b.visit(left.box, budget, func(right svgOrderPolygon) (bool, error) {
			next, err := svgOrderPolygonRelation(left, right, sourceA, sourceB, budget)
			relation |= next
			return relation == 3, err
		})
		if err != nil {
			return 0, err
		}
		if stop {
			break
		}
	}
	if swapped {
		relation = (relation&1)<<1 | (relation&2)>>1
	}
	return relation, nil
}
func svgOrderPolygonRelation(left, right svgOrderPolygon, sourceA, sourceB int, budget *svgVisibilityBudget) (int, error) {
	overlap := left.points
	winding := 1.
	if svgPolygonArea(right.points) < 0 {
		winding = -1
	}
	for i, p := range right.points {
		if err := budget.spend(len(overlap) + 1); err != nil {
			return 0, err
		}
		q := right.points[(i+1)%len(right.points)]
		overlap, _ = svgSplitPolygon(overlap, func(v svgPoint) float64 { return winding * ((q.x-p.x)*(v.y-p.y) - (q.y-p.y)*(v.x-p.x)) })
		if len(overlap) == 0 {
			return 0, nil
		}
	}
	low, high, magnitude := math.Inf(1), math.Inf(-1), 1.
	for _, p := range overlap {
		az, bz := left.plane.at(p), right.plane.at(p)
		low, high = math.Min(low, bz-az), math.Max(high, bz-az)
		magnitude = math.Max(magnitude, math.Max(math.Abs(az), math.Abs(bz)))
	}
	epsilon := math.Max(1e-9, magnitude*2e-14)
	if low >= -epsilon && high <= epsilon {
		if sourceA <= sourceB {
			return 1, nil
		}
		return 2, nil
	}
	relation := 0
	if high > epsilon {
		relation |= 1
	}
	if low < -epsilon {
		relation |= 2
	}
	return relation, nil
}

func svgOrderLess(units []*svgPaintUnit, a, b int) bool {
	left, right := units[a], units[b]
	if left.opaque != right.opaque {
		return left.opaque
	}
	if !left.opaque && left.depth != right.depth {
		return left.depth < right.depth
	}
	if left.first != right.first {
		return left.first < right.first
	}
	return a < b
}

type svgOrderHeap struct {
	indices    []int
	units      []*svgPaintUnit
	components [][]int
}

func (h svgOrderHeap) Len() int { return len(h.indices) }
func (h svgOrderHeap) Less(i, j int) bool {
	return svgOrderLess(h.units, h.components[h.indices[i]][0], h.components[h.indices[j]][0])
}
func (h svgOrderHeap) Swap(i, j int) { h.indices[i], h.indices[j] = h.indices[j], h.indices[i] }
func (h *svgOrderHeap) Push(v any)   { h.indices = append(h.indices, v.(int)) }
func (h *svgOrderHeap) Pop() any {
	n := len(h.indices) - 1
	v := h.indices[n]
	h.indices = h.indices[:n]
	return v
}

// Iterative Kosaraju keeps even long source-order chains off the call stack.
func svgOrderComponents(edges [][]int, budget *svgVisibilityBudget) ([][]int, []int, error) {
	type cursor struct{ node, next int }
	reverse := make([][]int, len(edges))
	for from, list := range edges {
		for _, to := range list {
			reverse[to] = append(reverse[to], from)
		}
	}
	seen := make([]bool, len(edges))
	order := make([]int, 0, len(edges))
	for start := range edges {
		if seen[start] {
			continue
		}
		seen[start] = true
		stack := []cursor{{node: start}}
		for len(stack) > 0 {
			if err := budget.spend(1); err != nil {
				return nil, nil, err
			}
			top := &stack[len(stack)-1]
			if top.next == len(edges[top.node]) {
				order = append(order, top.node)
				stack = stack[:len(stack)-1]
				continue
			}
			next := edges[top.node][top.next]
			top.next++
			if !seen[next] {
				seen[next] = true
				stack = append(stack, cursor{node: next})
			}
		}
	}
	membership := make([]int, len(edges))
	for i := range membership {
		membership[i] = -1
	}
	var components [][]int
	for i := len(order) - 1; i >= 0; i-- {
		start := order[i]
		if membership[start] >= 0 {
			continue
		}
		index := len(components)
		membership[start] = index
		stack := []int{start}
		var component []int
		for len(stack) > 0 {
			if err := budget.spend(1); err != nil {
				return nil, nil, err
			}
			n := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			component = append(component, n)
			for _, next := range reverse[n] {
				if membership[next] < 0 {
					membership[next] = index
					stack = append(stack, next)
				}
			}
		}
		components = append(components, component)
	}
	return components, membership, nil
}
