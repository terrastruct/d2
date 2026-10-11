package d2isometricimg

import (
	"context"
	"fmt"
	"math"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/label"
)

const (
	nativeBarrelSegments  = 72
	nativeUprightSegments = 72
)

// Flatten only the crown needed by the source's existing print allocation.
// This is part of the barrel, with one material and no attached label plate.
// The remaining profile is stretched vertically to retain the original full
// height; the footprint and every print rectangle stay unchanged.
func nativeQueueCrown(n d2isometric.Node) float64 {
	if n.Size.X <= 0 || n.Size.Z <= 0 {
		return 1
	}
	s := nativeFaceSource(n, n.Fill)
	position := label.FromString(n.Metadata.Original.LabelPosition)
	text := nativeNodeLabelSurface(n, s, 0)
	extent := 0.
	include := func(surface labelSurface) {
		if surface.width > 0 && surface.depth > 0 {
			extent = max(extent, math.Abs(surface.center.Z-n.Position.Z)+surface.depth/2)
		}
	}
	if n.Metadata.Original.Icon != nil {
		face := nativeNodeIconSurface(n, s, 0)
		scale := n.Size.Z / float64(s.Height)
		icon, remaining := surfaceIconLayout(face, n.Metadata.Original, scale, "node")
		include(icon)
		if !position.IsOutside() && !position.IsBorder() {
			text = remaining
			if n.Metadata.Original.LabelHeight > 0 {
				text.depth = min(text.depth, float64(n.Metadata.Original.LabelHeight)*scale*1.06)
			}
		}
	}
	if n.Metadata.Original.Label != "" && !position.IsOutside() && !position.IsBorder() {
		include(text)
	}
	if extent <= 0 {
		return 1
	}
	extent = min(1, (extent+min(.02, n.Size.Z*.02))/(n.Size.Z/2))
	// Snap outward to a tessellation vertex so the entire authored print area
	// is supported, including the sliver between ideal ellipse and polygon.
	angle := math.Ceil(math.Asin(extent)/(2*math.Pi/nativeBarrelSegments)) * (2 * math.Pi / nativeBarrelSegments)
	return max(0, math.Cos(angle))
}

func nativeBarrelPoint(i int, depth, height, crown float64) (y, z float64) {
	angle := float64(i) * 2 * math.Pi / nativeBarrelSegments
	return height * (1 + min(math.Cos(angle), crown)) / (1 + crown), math.Sin(angle) * depth / 2
}

func nativeUprightPoint(i int, hex bool) (float64, float64) {
	if hex {
		// D2's hexagon has slightly off-center side points.
		p := [6][2]float64{{1, 2*(43.6/87.3) - 1}, {.5, 1}, {-.5, 1}, {-1, 2*(43.6/87.3) - 1}, {-.5, -1}, {.5, -1}}[i%6]
		return p[0], p[1]
	}
	angle := float64(i) * 2 * math.Pi / nativeUprightSegments
	return math.Cos(angle), math.Sin(angle)
}

// Source ports lie on the D2 outline, which can extend beyond a solid's curved
// wall near the board plane. Keep the source port as a path
// vertex and add only the short inward segment needed to reach the real solid.
// All exterior bends, source metadata, and route elevations remain untouched.
func nativeSolidContactRoutes(ctx context.Context, edges []d2isometric.Edge, nodes []d2isometric.Node, paths [][]Vec, support ...map[string]float64) ([][]Vec, error) {
	if ctx == nil {
		return nil, fmt.Errorf("native solid contacts require a context")
	}
	if len(edges) != len(paths) {
		return nil, fmt.Errorf("native solid contacts require one route per edge")
	}
	solids := make(map[string]d2isometric.Node)
	for _, n := range nodes {
		if nativeSolidNode(n) && n.Opacity > 0 {
			solids[n.ID] = n
		}
	}
	contact := func(n d2isometric.Node, start, inward Vec) (Vec, bool) {
		y := start.Y
		if len(support) > 0 {
			// The cap stays at its existing print height while a lower
			// supporting plate extends the body downward. Intersect in the
			// original body's coordinates, then restore the route's plane.
			if drop := support[0][n.BoardID]; drop < 0 {
				start.Y = hierarchyNodeExtension(n, drop).inverseY(start.Y)
			}
		}
		at, moved := nativeSolidContact(n, start, inward)
		at.Y = y
		return at, moved
	}
	out := make([][]Vec, len(paths))
	for i, points := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		out[i] = points
		if len(points) < 2 || edges[i].Opacity <= 0 {
			continue
		}
		last := len(points) - 1
		source, target := points[0], points[last]
		var sourceMoved, targetMoved bool
		if n, ok := solids[edges[i].Source]; ok {
			source, sourceMoved = contact(n, points[0], nsub(points[0], points[1]))
		}
		if n, ok := solids[edges[i].Target]; ok {
			target, targetMoved = contact(n, points[last], nsub(points[last], points[last-1]))
		}
		if !sourceMoved && !targetMoved {
			continue
		}
		out[i] = make([]Vec, 0, len(points)+2)
		if sourceMoved {
			out[i] = append(out[i], source)
		}
		out[i] = append(out[i], points...)
		if targetMoved {
			out[i] = append(out[i], target)
		}
	}
	return out, nil
}

func nativeSolidContact(n d2isometric.Node, start, inward Vec) (Vec, bool) {
	if n.Type == d2target.ShapeQueue {
		return nativeQueueContact(n, start, inward)
	}
	return nativeUprightContact(n, start, inward)
}

// Intersect the actual upright mesh's horizontal cross-section, including its
// polygonal tessellation. D2 cylinder side ports need this because their
// source outline has straight side walls,
// while the 3D footprint is elliptical. The short connector remains inside the
// compiled footprint, and does not alter the source endpoint or its tangent.
func nativeUprightContact(n d2isometric.Node, start, inward Vec) (Vec, bool) {
	if !captionFinite(n.Size.X, n.Size.Y, n.Size.Z, n.Position.X, n.Position.Y, n.Position.Z, start.X, start.Y, start.Z, inward.X, inward.Z) || n.Size.X <= 0 || n.Size.Z <= 0 {
		return start, false
	}
	w, d, height := n.Size.X, n.Size.Z, nativeSolidHeight(n)
	relief := hierarchyNodeRelief(n)
	floor := n.Position.Y - n.Size.Y/2
	y := (start.Y - floor) / relief
	if y <= 0 || y >= height {
		return start, false
	}
	length := math.Hypot(inward.X, inward.Z)
	if length <= 1e-9 {
		return start, false
	}
	direction := nv(inward.X/length, 0, inward.Z/length)
	x, z := start.X-n.Position.X, start.Z-n.Position.Z
	scale := .01
	if n.Metadata.Original.Width > 0 {
		scale = w / float64(n.Metadata.Original.Width)
	}
	margin := min(.05, max(1e-7, float64(n.StrokeWidth)*scale/2))
	if math.Abs(x) > w/2+margin || math.Abs(z) > d/2+margin {
		return start, false
	}
	radius := 1.
	count, hex := nativeUprightSegments, n.Type == d2target.ShapeHexagon
	if hex {
		count = 6
	}
	point := func(i int) (float64, float64) {
		x, z := nativeUprightPoint(i, hex)
		return x * w / 2 * radius, z * d / 2 * radius
	}
	enter, leave := 0., math.Hypot(w, d)+2*margin
	for i := 0; i < count; i++ {
		ax, az := point(i)
		bx, bz := point(i + 1)
		nx, nz := bz-az, ax-bx
		room, rate := nx*(ax-x)+nz*(az-z), nx*direction.X+nz*direction.Z
		if math.Abs(rate) < 1e-12 {
			if room < -1e-10 {
				return start, false
			}
			continue
		}
		at := room / rate
		if rate > 0 {
			leave = min(leave, at)
		} else {
			enter = max(enter, at)
		}
		if enter > leave+1e-10 {
			return start, false
		}
	}
	if enter <= 1e-7 || !captionFinite(enter) {
		return start, false
	}
	contact := nadd(start, nmul(direction, enter))
	contact.Y = start.Y
	return contact, true
}

// Clip a horizontal ray against the actual convex polygonal barrel, including
// its flat print crown and full end caps. An ideal ellipse intersection can
// otherwise stop outside the tessellated wall.
func nativeQueueContact(n d2isometric.Node, start, inward Vec) (Vec, bool) {
	if !captionFinite(n.Size.X, n.Size.Y, n.Size.Z, n.Position.X, n.Position.Y, n.Position.Z, start.X, start.Y, start.Z, inward.X, inward.Z) || n.Size.X <= 0 || n.Size.Z <= 0 {
		return start, false
	}
	w, d, height := n.Size.X, n.Size.Z, nativeSolidHeight(n)
	relief := hierarchyNodeRelief(n)
	floor, h := n.Position.Y-n.Size.Y/2, height*relief
	if start.Y <= floor || start.Y >= floor+h {
		return start, false
	}
	length := math.Hypot(inward.X, inward.Z)
	if length <= 1e-9 {
		return start, false
	}
	direction := nv(inward.X/length, 0, inward.Z/length)
	x, z := start.X-n.Position.X, start.Z-n.Position.Z
	// An authored stroke may place its port just outside the fill footprint.
	// Reject unrelated external points instead of extending a route across
	// arbitrary free space to reach a distant component.
	scale := .01
	if n.Metadata.Original.Width > 0 {
		scale = w / float64(n.Metadata.Original.Width)
	}
	margin := min(.05, max(1e-7, float64(n.StrokeWidth)*scale/2))
	if math.Abs(x) > w/2+margin || math.Abs(z) > d/2+margin {
		return start, false
	}
	enter, leave := 0., math.Hypot(w, d)+2*margin
	clip := func(rate, room float64) bool {
		if math.Abs(rate) < 1e-12 {
			return room >= -1e-10
		}
		at := room / rate
		if rate > 0 {
			leave = min(leave, at)
		} else {
			enter = max(enter, at)
		}
		return enter <= leave+1e-10
	}
	if !clip(direction.X, w/2-x) || !clip(-direction.X, w/2+x) {
		return start, false
	}
	crown := nativeQueueCrown(n)
	for i := 0; i < nativeBarrelSegments; i++ {
		ay, az := nativeBarrelPoint(i, d, h, crown)
		by, bz := nativeBarrelPoint(i+1, d, h, crown)
		ny, nz := bz-az, ay-by
		room := ny*(ay-(start.Y-floor)) + nz*(az-z)
		if !clip(nz*direction.Z, room) {
			return start, false
		}
	}
	if enter <= 1e-7 || !captionFinite(enter) {
		return start, false
	}
	// The exact boundary is shared with the solid mesh. An epsilon would move
	// an arrow tip under the material and is unnecessary for these same planes.
	contact := nadd(start, nmul(direction, enter))
	contact.Y = start.Y
	return contact, true
}
