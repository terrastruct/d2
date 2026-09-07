package d2isometricimg

import (
	"context"
	"fmt"
	"math"
	"math/bits"
	"sort"
)

// SVG visibility is solved in the final orthographic camera. Depth is affine
// over a projected face, so even intersecting faces can be cut exactly without
// a global BSP or a centroid painter approximation. Larger z is nearer. The
// caller includes Triangle.DepthBias in z; it never changes projected x/y.
type svgPoint struct{ x, y, z float64 }

type svgVisibilityFace struct {
	points  []svgPoint // Convex, planar projected polygon, either winding.
	opaque  bool       // A fully opaque, depth-writing surface, not a decal.
	group   *nativeOpacityGroup
	owner   *nativePaintOwner
	contour bool
	order   int // Later source paint wins an exactly coplanar tie.
}

type svgVisibilityLimits struct {
	faces, vertices, fragments, work, gridReferences int
}

var svgDefaultVisibilityLimits = svgVisibilityLimits{
	faces: rasterMaxTriangles, vertices: 8_000_000, fragments: 2_000_000,
	work: 100_000_000, gridReferences: 8_000_000,
}

type svgVisibilityBudget struct {
	ctx                                context.Context
	limits                             svgVisibilityLimits
	work                               int
	face, faces, candidates, fragments int
}

func (b *svgVisibilityBudget) spend(n int) error {
	if n < 0 || n > b.limits.work-b.work {
		return fmt.Errorf("isometric SVG visibility exceeds work limit (%d faces, face %d, %d candidates, %d fragments)", b.faces, b.face, b.candidates, b.fragments)
	}
	b.work += n
	return b.ctx.Err()
}

type svgBox struct{ minX, minY, maxX, maxY float64 }

func svgPolygonBox(points []svgPoint) svgBox {
	b := svgBox{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
	for _, p := range points {
		b.minX, b.minY = math.Min(b.minX, p.x), math.Min(b.minY, p.y)
		b.maxX, b.maxY = math.Max(b.maxX, p.x), math.Max(b.maxY, p.y)
	}
	return b
}

func (b svgBox) overlaps(c svgBox) bool {
	return b.minX < c.maxX && c.minX < b.maxX && b.minY < c.maxY && c.minY < b.maxY
}

// Depth planes use a nearby origin to avoid loss of precision after a large
// screen translation. The plane and every cut remain in the subject's depth.
type svgDepthPlane struct {
	origin svgPoint
	dx, dy float64
}

func (p svgDepthPlane) at(v svgPoint) float64 {
	return p.origin.z + p.dx*(v.x-p.origin.x) + p.dy*(v.y-p.origin.y)
}

func svgPolygonArea(points []svgPoint) float64 {
	if len(points) < 3 {
		return 0
	}
	var area float64
	o := points[0]
	for i := 1; i+1 < len(points); i++ {
		a, b := points[i], points[i+1]
		area += (a.x-o.x)*(b.y-o.y) - (a.y-o.y)*(b.x-o.x)
	}
	return area / 2
}

func svgPolygonPlane(points []svgPoint) (svgDepthPlane, bool) {
	if len(points) < 3 {
		return svgDepthPlane{}, false
	}
	o := points[0]
	best, index := 0.0, 0
	for i := 1; i+1 < len(points); i++ {
		a, b := points[i], points[i+1]
		d := (a.x-o.x)*(b.y-o.y) - (a.y-o.y)*(b.x-o.x)
		if math.Abs(d) > math.Abs(best) {
			best, index = d, i
		}
	}
	if math.Abs(best) <= 1e-14 {
		return svgDepthPlane{}, false
	}
	a, b := points[index], points[index+1]
	return svgDepthPlane{o,
		((a.z-o.z)*(b.y-o.y) - (b.z-o.z)*(a.y-o.y)) / best,
		((a.x-o.x)*(b.z-o.z) - (b.x-o.x)*(a.z-o.z)) / best,
	}, true
}

type svgVisibilityPolygon struct {
	points []svgPoint
	plane  svgDepthPlane
	box    svgBox
	area   float64
}

// A uniform grid keeps ordinary diagram meshes local. Very large polygons use
// one overflow bucket instead of occupying thousands of cells. All candidate
// visits, including duplicates and overflow visits, consume the work budget.
type svgVisibilityGrid struct {
	bounds svgBox
	nx, ny int
	cells  [][]int
	large  []int
}

func (g *svgVisibilityGrid) cellsFor(b svgBox) (x0, y0, x1, y1 int, ok bool) {
	if !g.bounds.overlaps(b) {
		return 0, 0, 0, 0, false
	}
	x := func(v float64) int {
		return max(0, min(g.nx-1, int(math.Floor((v-g.bounds.minX)/(g.bounds.maxX-g.bounds.minX)*float64(g.nx)))))
	}
	y := func(v float64) int {
		return max(0, min(g.ny-1, int(math.Floor((v-g.bounds.minY)/(g.bounds.maxY-g.bounds.minY)*float64(g.ny)))))
	}
	return x(b.minX), y(b.minY), x(b.maxX), y(b.maxY), true
}

func svgNewVisibilityGrid(polygons []svgVisibilityPolygon, faces []svgVisibilityFace, budget *svgVisibilityBudget) (*svgVisibilityGrid, error) {
	bounds := svgBox{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
	count := 0
	for i, p := range polygons {
		if err := budget.spend(1); err != nil {
			return nil, err
		}
		if len(p.points) == 0 || !faces[i].opaque {
			continue
		}
		bounds.minX, bounds.minY = math.Min(bounds.minX, p.box.minX), math.Min(bounds.minY, p.box.minY)
		bounds.maxX, bounds.maxY = math.Max(bounds.maxX, p.box.maxX), math.Max(bounds.maxY, p.box.maxY)
		count++
	}
	if count == 0 {
		return nil, nil
	}
	w, h := bounds.maxX-bounds.minX, bounds.maxY-bounds.minY
	cells := math.Max(1, math.Min(65536, float64(count)/4))
	nx := max(1, min(256, int(math.Ceil(math.Sqrt(cells*w/h)))))
	ny := max(1, min(256, int(math.Ceil(cells/float64(nx)))))
	g := &svgVisibilityGrid{bounds: bounds, nx: nx, ny: ny, cells: make([][]int, nx*ny)}
	references := 0
	for i, p := range polygons {
		if err := budget.spend(1); err != nil {
			return nil, err
		}
		if len(p.points) == 0 || !faces[i].opaque {
			continue
		}
		x0, y0, x1, y1, _ := g.cellsFor(p.box)
		n := (x1 - x0 + 1) * (y1 - y0 + 1)
		if n > 64 {
			g.large = append(g.large, i)
			n = 1
		} else {
			for y := y0; y <= y1; y++ {
				for x := x0; x <= x1; x++ {
					g.cells[y*nx+x] = append(g.cells[y*nx+x], i)
				}
			}
		}
		references += n
		if references > budget.limits.gridReferences {
			return nil, fmt.Errorf("isometric SVG visibility exceeds spatial index limit")
		}
	}
	return g, nil
}

// svgSplitPolygon cuts a convex polygon into two convex pieces. New vertices
// interpolate all three coordinates, so the output remains on its own face.
func svgSplitPolygon(points []svgPoint, distance func(svgPoint) float64) (inside, outside []svgPoint) {
	if len(points) == 0 {
		return nil, nil
	}
	distances := make([]float64, len(points))
	positive, negative := false, false
	for i, point := range points {
		d := distance(point)
		distances[i] = d
		positive, negative = positive || d > 0, negative || d < 0
	}
	// An exactly coincident or zero-length clipping edge contains the whole
	// face. It must not also emit the whole face as an outside fragment: that
	// would resurrect previously hidden geometry and multiply later cuts.
	if !negative {
		return points, nil
	}
	if !positive {
		return nil, points
	}
	previous := points[len(points)-1]
	dp := distances[len(points)-1]
	for i, current := range points {
		dc := distances[i]
		if (dp < 0 && dc > 0) || (dp > 0 && dc < 0) {
			t := dp / (dp - dc)
			p := svgPoint{previous.x + t*(current.x-previous.x), previous.y + t*(current.y-previous.y), previous.z + t*(current.z-previous.z)}
			inside, outside = append(inside, p), append(outside, p)
		}
		if dc >= 0 {
			inside = append(inside, current)
		}
		if dc <= 0 {
			outside = append(outside, current)
		}
		previous, dp = current, dc
	}
	return svgValidFragment(inside), svgValidFragment(outside)
}

func svgValidFragment(points []svgPoint) []svgPoint {
	if len(points) < 3 {
		return nil
	}
	// A depth cut through a shared vertex can create two identical projected
	// points with a last-bit difference in z. Remove those zero-length edges
	// before they become half-plane constraints in a subsequent subtraction.
	same := func(a, b svgPoint) bool {
		epsilon := math.Max(1e-9, math.Max(math.Max(math.Abs(a.x), math.Abs(a.y)), math.Max(math.Abs(b.x), math.Abs(b.y)))*1e-15)
		return math.Abs(a.x-b.x) <= epsilon && math.Abs(a.y-b.y) <= epsilon
	}
	for i, p := range points {
		if !same(p, points[(i+1)%len(points)]) {
			continue
		}
		clean := make([]svgPoint, 0, len(points))
		for _, point := range points {
			if len(clean) == 0 || !same(point, clean[len(clean)-1]) {
				clean = append(clean, point)
			}
		}
		if len(clean) > 1 && same(clean[0], clean[len(clean)-1]) {
			clean = clean[:len(clean)-1]
		}
		points = clean
		break
	}
	if len(points) < 3 {
		return nil
	}
	box := svgPolygonBox(points)
	span := math.Max(box.maxX-box.minX, box.maxY-box.minY)
	if math.Abs(svgPolygonArea(points)) <= math.Max(1e-12, span*span*1e-13) {
		return nil
	}
	return points
}

func svgEdgeDistance(a, b svgPoint) func(svgPoint) float64 {
	return func(p svgPoint) float64 { return (b.x-a.x)*(p.y-a.y) - (b.y-a.y)*(p.x-a.x) }
}

// A convex subtraction yields disjoint convex fragments. Checking intersection
// first is essential: subtracting a remote polygon must not fragment a face
// along infinitely extended edges that never actually cover any of it.
func svgSubtractPolygon(subject, cut []svgPoint, budget *svgVisibilityBudget) ([][]svgPoint, error) {
	intersection := subject
	for i, a := range cut {
		if err := budget.spend(len(intersection)); err != nil {
			return nil, err
		}
		intersection, _ = svgSplitPolygon(intersection, svgEdgeDistance(a, cut[(i+1)%len(cut)]))
		if len(intersection) == 0 {
			return [][]svgPoint{subject}, nil
		}
	}
	remaining := subject
	var visible [][]svgPoint
	for i, a := range cut {
		if err := budget.spend(len(remaining)); err != nil {
			return nil, err
		}
		var outside []svgPoint
		remaining, outside = svgSplitPolygon(remaining, svgEdgeDistance(a, cut[(i+1)%len(cut)]))
		if len(outside) != 0 {
			visible = append(visible, outside)
		}
		if len(remaining) == 0 {
			break
		}
	}
	return visible, nil
}

func svgVisibleFaces(ctx context.Context, faces []svgVisibilityFace) ([][][]svgPoint, error) {
	limits := svgDefaultVisibilityLimits
	limits.work = svgVisibilityWorkLimit(len(faces))
	return svgVisibleFacesWithLimits(ctx, faces, limits)
}

// Preserve the ordinary diagram budget while giving large meshes a linear
// amount of work per input face for indexing, local depth tests and clipping.
// Fragment indexing controls combinatorial growth independently. The native
// raster work ceiling remains an absolute bound for every visibility pass.
func svgVisibilityWorkLimit(faces int) int {
	if faces > rasterMaxWork/2048 {
		return rasterMaxWork
	}
	return max(svgDefaultVisibilityLimits.work, faces*2048)
}

func svgVisibleFacesWithLimits(ctx context.Context, faces []svgVisibilityFace, limits svgVisibilityLimits) ([][][]svgPoint, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(faces) > limits.faces {
		return nil, fmt.Errorf("isometric SVG visibility exceeds face limit")
	}
	budget := &svgVisibilityBudget{ctx: ctx, limits: limits, faces: len(faces)}
	polygons := make([]svgVisibilityPolygon, len(faces))
	inputVertices := 0
	for i, face := range faces {
		if err := budget.spend(len(face.points) + 1); err != nil {
			return nil, err
		}
		inputVertices += len(face.points)
		if inputVertices > limits.vertices {
			return nil, fmt.Errorf("isometric SVG visibility exceeds vertex limit")
		}
		for _, p := range face.points {
			if math.IsNaN(p.x) || math.IsNaN(p.y) || math.IsNaN(p.z) || math.Abs(p.x) > rasterCoordinateLimit || math.Abs(p.y) > rasterCoordinateLimit || math.Abs(p.z) > rasterCoordinateLimit {
				return nil, fmt.Errorf("isometric SVG visibility face %d has invalid coordinates", i)
			}
		}
		valid := svgValidFragment(face.points)
		plane, ok := svgPolygonPlane(valid)
		if !ok {
			continue
		}
		points := append([]svgPoint(nil), valid...)
		if svgPolygonArea(points) < 0 {
			for a, b := 0, len(points)-1; a < b; a, b = a+1, b-1 {
				points[a], points[b] = points[b], points[a]
			}
		}
		polygons[i] = svgVisibilityPolygon{points, plane, svgPolygonBox(points), svgPolygonArea(points)}
	}
	var grid *svgVisibilityGrid
	var tree *svgVisibilityTree
	var err error
	if len(faces) > 8192 {
		tree, err = svgNewVisibilityTree(polygons, faces, budget)
	} else {
		grid, err = svgNewVisibilityGrid(polygons, faces, budget)
	}
	if err != nil {
		return nil, err
	}
	output := make([][][]svgPoint, len(faces))
	seen := make([]int, len(faces))
	outputVertices, outputFragments := 0, 0
	for index, subject := range polygons {
		budget.face, budget.candidates, budget.fragments = index, 0, 1
		if err := budget.spend(1); err != nil {
			return nil, err
		}
		if len(subject.points) == 0 {
			continue
		}
		fragments := [][]svgPoint{subject.points}
		var candidates []int
		visit := func(indices []int) error {
			if err := budget.spend(len(indices)); err != nil {
				return err
			}
			for _, other := range indices {
				if other == index || seen[other] == index+1 {
					continue
				}
				seen[other] = index + 1
				if faces[other].group != nil && faces[other].group != faces[index].group {
					continue
				}
				if faces[other].contour && !faces[index].contour && ((faces[index].owner != nil && faces[index].owner == faces[other].owner) || (faces[index].group != nil && faces[index].group == faces[other].group)) {
					continue
				}
				if subject.box.overlaps(polygons[other].box) {
					candidates = append(candidates, other)
				}
			}
			return nil
		}
		if tree != nil {
			indices, err := tree.query(subject.box, budget)
			if err != nil {
				return nil, err
			}
			if err := visit(indices); err != nil {
				return nil, err
			}
		} else if grid != nil {
			if x0, y0, x1, y1, ok := grid.cellsFor(subject.box); ok {
				for y := y0; y <= y1; y++ {
					for x := x0; x <= x1; x++ {
						if err := visit(grid.cells[y*grid.nx+x]); err != nil {
							return nil, err
						}
					}
				}
				if err := visit(grid.large); err != nil {
					return nil, err
				}
			}
		}
		// Remove broad solid faces before the narrow ink ribbons around them.
		// Cutting thousands of ribbons first would unnecessarily partition a
		// large board into slivers that its few broad caps subsequently hide.
		budget.candidates = len(candidates)
		if err := budget.spend(len(candidates) * bits.Len(uint(len(candidates)))); err != nil {
			return nil, err
		}
		sort.Slice(candidates, func(a, b int) bool {
			i, j := candidates[a], candidates[b]
			if polygons[i].area != polygons[j].area {
				return polygons[i].area > polygons[j].area
			}
			if faces[i].order == faces[j].order {
				return i > j
			}
			return faces[i].order > faces[j].order
		})
		var indexedFragments *svgVisibleFragmentSet
		if len(candidates) > 64 {
			indexedFragments, err = svgNewVisibleFragmentSet(subject.points, budget, limits.fragments-outputFragments, limits.vertices-outputVertices)
			if err != nil {
				return nil, err
			}
		}
		for _, other := range candidates {
			occluder := polygons[other]
			if err := budget.spend(len(occluder.points) + 4); err != nil {
				return nil, err
			}
			cut := occluder.points
			difference := func(p svgPoint) float64 { return occluder.plane.at(p) - subject.plane.at(p) }
			lo, hi, depthMagnitude := math.Inf(1), math.Inf(-1), 1.0
			// The overlap bbox bounds both affine depth extrema without requiring
			// another polygon allocation merely to classify a coplanar pair.
			box := svgBox{math.Max(subject.box.minX, occluder.box.minX), math.Max(subject.box.minY, occluder.box.minY), math.Min(subject.box.maxX, occluder.box.maxX), math.Min(subject.box.maxY, occluder.box.maxY)}
			for _, p := range []svgPoint{{x: box.minX, y: box.minY}, {x: box.maxX, y: box.minY}, {x: box.maxX, y: box.maxY}, {x: box.minX, y: box.maxY}} {
				d := difference(p)
				lo, hi = math.Min(lo, d), math.Max(hi, d)
				depthMagnitude = math.Max(depthMagnitude, math.Max(math.Abs(subject.plane.at(p)), math.Abs(occluder.plane.at(p))))
			}
			epsilon := math.Max(1e-9, depthMagnitude*2e-14)
			if lo >= -epsilon && hi <= epsilon {
				if faces[other].order < faces[index].order || (faces[other].order == faces[index].order && other < index) {
					continue
				}
			} else if hi <= 0 {
				continue
			} else if lo < 0 {
				cut, _ = svgSplitPolygon(cut, difference)
				if len(cut) == 0 {
					continue
				}
			}
			if indexedFragments != nil {
				if err := indexedFragments.subtract(cut); err != nil {
					return nil, err
				}
				budget.fragments = len(indexedFragments.fragments)
				if budget.fragments == 0 {
					break
				}
				continue
			}
			cutBox := svgPolygonBox(cut)
			var next [][]svgPoint
			vertices := 0
			for _, fragment := range fragments {
				if err := budget.spend(len(fragment) + 1); err != nil {
					return nil, err
				}
				pieces := [][]svgPoint{fragment}
				if svgPolygonBox(fragment).overlaps(cutBox) {
					pieces, err = svgSubtractPolygon(fragment, cut, budget)
					if err != nil {
						return nil, err
					}
				}
				for _, piece := range pieces {
					vertices += len(piece)
				}
				next = append(next, pieces...)
				if len(next) > limits.fragments-outputFragments || vertices > limits.vertices-outputVertices {
					return nil, fmt.Errorf("isometric SVG visibility exceeds fragment limit")
				}
			}
			fragments = next
			budget.fragments = len(fragments)
			if len(fragments) == 0 {
				break
			}
		}
		if indexedFragments != nil {
			fragments, err = indexedFragments.polygons()
			if err != nil {
				return nil, err
			}
		}
		outputFragments += len(fragments)
		for _, fragment := range fragments {
			outputVertices += len(fragment)
		}
		if outputFragments > limits.fragments || outputVertices > limits.vertices {
			return nil, fmt.Errorf("isometric SVG visibility exceeds fragment limit")
		}
		output[index] = fragments
	}
	return output, nil
}
