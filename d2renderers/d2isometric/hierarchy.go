package d2isometric

import (
	"fmt"
	"math"

	"github.com/d2lang/d2/d2target"
)

// composeHierarchy is a projection of the compiled layout, not a second layout.
// Parent links are resolved by BuildScene's D2 key parser, including quoted IDs.
// An invisible root board provides membership for leaves without a container.
func composeHierarchy(scene *Scene, indices map[string]int, parents []int) error {
	boardOf := make([]int, len(scene.Nodes))
	for i := range boardOf {
		boardOf[i] = -1
	}
	for i := range scene.Nodes {
		n := &scene.Nodes[i]
		s := n.Metadata.Original
		n.Size = moduleSize(s)
		n.Position = hierarchyCenter(s)
		n.Position.Y = .07 + n.Size.Y/2
		if !n.Container {
			continue
		}
		depth := 0
		for p := parents[i]; p >= 0; p = parents[p] {
			if scene.Nodes[p].Container {
				depth++
			}
		}
		kind := "platform"
		if depth > 0 {
			kind = "group"
		}
		b := Board{
			ID: "@container:" + n.ID, SourceID: n.ID, Label: n.Label,
			Kind: kind, Level: depth, Position: hierarchyCenter(s),
			Size: Vec3{n.Size.X, .14, n.Size.Z}, NodeIDs: []string{},
		}
		if b.Label != "" {
			_, height := boardLabelMetrics(b, scene.Nodes, indices)
			// Header metadata must never enlarge the source footprint or move a
			// child. The native renderer places text using source label anchors.
			b.HeaderDepth = math.Min(b.Size.Z, math.Max(.5, height+.3))
		}
		boardOf[i] = len(scene.Boards)
		scene.Boards = append(scene.Boards, b)
		n.Position, n.Size, n.BoardID = b.Position, b.Size, b.ID
	}
	root := -1
	for i := range scene.Nodes {
		n := &scene.Nodes[i]
		parent := parents[i]
		// Sequence semantic parents (actors, spans and groups) can own
		// annotations without enclosing them. Keep ParentID, but attach the
		// render membership to the nearest actual source container board.
		for parent >= 0 && boardOf[parent] < 0 {
			parent = parents[parent]
		}
		if parent >= 0 {
			b := &scene.Boards[boardOf[parent]]
			b.NodeIDs = append(b.NodeIDs, n.ID)
			if n.Container {
				scene.Boards[boardOf[i]].ParentID = b.ID
			} else {
				n.BoardID = b.ID
			}
		} else if !n.Container {
			if root < 0 {
				root = len(scene.Boards)
				scene.Boards = append(scene.Boards, Board{ID: "@ungrouped", Kind: "ungrouped", NodeIDs: []string{}})
			}
			b := &scene.Boards[root]
			b.NodeIDs = append(b.NodeIDs, n.ID)
			n.BoardID = b.ID
		}
	}
	if root >= 0 {
		// This bound is only membership metadata; no visible plate or invented
		// dependency caption is introduced around ungrouped source objects.
		b := &scene.Boards[root]
		left, top := math.Inf(1), math.Inf(1)
		right, bottom := math.Inf(-1), math.Inf(-1)
		for _, id := range b.NodeIDs {
			n := scene.Nodes[indices[id]]
			left = math.Min(left, n.Position.X-n.Size.X/2)
			right = math.Max(right, n.Position.X+n.Size.X/2)
			top = math.Min(top, n.Position.Z-n.Size.Z/2)
			bottom = math.Max(bottom, n.Position.Z+n.Size.Z/2)
		}
		b.Position = Vec3{(left + right) / 2, 0, (top + bottom) / 2}
		b.Size = Vec3{math.Max(SceneScale, right-left), .14, math.Max(SceneScale, bottom-top)}
	}
	points := 0
	fallback := false
	for i := range scene.Edges {
		e := &scene.Edges[i]
		original := e.Metadata.Original
		if len(original.Route) >= 2 {
			var err error
			e.Points, err = hierarchyRoute(original, &points)
			if err != nil {
				return fmt.Errorf("isometric: hierarchy route %q: %w", e.ID, err)
			}
		} else {
			// BuildScene has already rejected unknown endpoints without a route.
			// A caller can supply a typed target before routing; keep it usable
			// without claiming this simple fallback is a layout engine's route.
			a, b := scene.Nodes[indices[e.Source]], scene.Nodes[indices[e.Target]]
			e.Points = hierarchyFallback(a, b)
			points += len(e.Points)
			fallback = true
		}
		if points > maxEntries {
			return fmt.Errorf("isometric: hierarchy route points exceed %d", maxEntries)
		}
	}
	if fallback {
		scene.Warnings = append(scene.Warnings, "Connections without a usable compiled route use a simple perimeter fallback; obstacle avoidance requires a routed D2 layout.")
	}
	return nil
}

func hierarchyCenter(s d2target.Shape) Vec3 {
	// Convert before adding to avoid integer overflow on 32-bit targets. Zero
	// dimensions use the same one-source-pixel footprint as moduleSize.
	return Vec3{
		(float64(s.Pos.X) + float64(max(1, s.Width))/2) * SceneScale,
		0,
		(float64(s.Pos.Y) + float64(max(1, s.Height))/2) * SceneScale,
	}
}

// hierarchyRoute preserves polylines exactly. Cubic controls are metadata, not
// corners: flatten each cubic to a quarter-source-pixel geometric tolerance.
// The fixed recursion depth and shared output budget bound pathological curves.
func hierarchyRoute(c d2target.Connection, count *int) ([]Vec3, error) {
	if c.IsCurve && (len(c.Route)-1)%3 != 0 {
		return nil, fmt.Errorf("invalid cubic control point count %d", len(c.Route))
	}
	out := make([]Vec3, 0, len(c.Route))
	appendPoint := func(p Vec3) error {
		if *count >= maxEntries {
			return fmt.Errorf("flattened route points exceed %d", maxEntries)
		}
		*count++
		out = append(out, p)
		return nil
	}
	point := func(i int) Vec3 {
		return Vec3{c.Route[i].X * SceneScale, surfaceHeight, c.Route[i].Y * SceneScale}
	}
	if !c.IsCurve {
		for i := range c.Route {
			if err := appendPoint(point(i)); err != nil {
				return nil, err
			}
		}
		return out, nil
	}
	if err := appendPoint(point(0)); err != nil {
		return nil, err
	}
	var flatten func(Vec3, Vec3, Vec3, Vec3, int) error
	flatten = func(a, b, c, d Vec3, depth int) error {
		if hierarchyControlDistance(b, a, d) <= SceneScale*.25 && hierarchyControlDistance(c, a, d) <= SceneScale*.25 {
			return appendPoint(d)
		}
		if depth >= 24 {
			return fmt.Errorf("curve subdivision exceeds precision budget")
		}
		ab, bc, cd := hierarchyMidpoint(a, b), hierarchyMidpoint(b, c), hierarchyMidpoint(c, d)
		abc, bcd := hierarchyMidpoint(ab, bc), hierarchyMidpoint(bc, cd)
		mid := hierarchyMidpoint(abc, bcd)
		if err := flatten(a, ab, abc, mid, depth+1); err != nil {
			return err
		}
		return flatten(mid, bcd, cd, d, depth+1)
	}
	for i := 1; i < len(c.Route); i += 3 {
		if err := flatten(point(i-1), point(i), point(i+1), point(i+2), 0); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func hierarchyMidpoint(a, b Vec3) Vec3 {
	return Vec3{(a.X + b.X) / 2, surfaceHeight, (a.Z + b.Z) / 2}
}

func hierarchyControlDistance(p, a, b Vec3) float64 {
	dx, dz := b.X-a.X, b.Z-a.Z
	t := 0.0
	if n := dx*dx + dz*dz; n > 0 {
		t = math.Max(0, math.Min(1, ((p.X-a.X)*dx+(p.Z-a.Z)*dz)/n))
	}
	return math.Hypot(p.X-a.X-dx*t, p.Z-a.Z-dz*t)
}

func hierarchyFallback(a, b Node) []Vec3 {
	if a.ID == b.ID {
		x, z := a.Size.X/2, a.Size.Z/2
		p := a.Position
		return []Vec3{{p.X + x, surfaceHeight, p.Z}, {p.X + x + .35, surfaceHeight, p.Z}, {p.X + x + .35, surfaceHeight, p.Z + z + .35}, {p.X, surfaceHeight, p.Z + z + .35}, {p.X, surfaceHeight, p.Z + z}}
	}
	port := func(n Node, toward Vec3, fallback float64) Vec3 {
		dx, dz := toward.X-n.Position.X, toward.Z-n.Position.Z
		if dx == 0 && dz == 0 {
			dx = fallback
		}
		t := 1 / math.Max(math.Abs(dx)/(n.Size.X/2), math.Abs(dz)/(n.Size.Z/2))
		return Vec3{n.Position.X + dx*t, surfaceHeight, n.Position.Z + dz*t}
	}
	return []Vec3{port(a, b.Position, 1), port(b, a.Position, -1)}
}
