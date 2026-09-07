package d2isometricimg

import (
	"fmt"
	"math"
	"sort"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2target"
)

type packetRoute struct {
	points           []Vec
	lengths          []float64
	material         *Material
	forward, reverse bool
}

func routeLengths(points []Vec) []float64 {
	out := make([]float64, len(points))
	for i := 1; i < len(points); i++ {
		out[i] = out[i-1] + nlen(nsub(points[i], points[i-1]))
	}
	return out
}
func pathPoint(points []Vec, lengths []float64, t float64) Vec {
	if len(points) == 0 {
		return Vec{}
	}
	distance := max(0, min(1, t)) * lengths[len(lengths)-1]
	for i := 1; i < len(points); i++ {
		if lengths[i] >= distance {
			if delta := lengths[i] - lengths[i-1]; delta > 1e-12 {
				return nlerp(points[i-1], points[i], (distance-lengths[i-1])/delta)
			}
		}
	}
	return points[len(points)-1]
}

// Keep every original bend/bridge vertex inside a dash. Uniform resampling can
// flatten a small bridge while traffic still follows its full curve.
func nativeRouteSection(points []Vec, lengths []float64, start, end float64) []Vec {
	if len(points) < 2 || len(lengths) != len(points) || end <= start {
		return nil
	}
	start, end = max(0, min(1, start)), max(0, min(1, end))
	total := lengths[len(lengths)-1]
	out := []Vec{pathPoint(points, lengths, start)}
	first := sort.Search(len(lengths), func(i int) bool { return lengths[i] > start*total })
	for i := first; i < len(points) && lengths[i] < end*total; i++ {
		out = append(out, points[i])
	}
	return append(out, pathPoint(points, lengths, end))
}

// Clip original dash intervals to a visible portion of the same path. Marker
// placement and stem length must never retime the rest of a dashed connection.
func nativeRouteDashes(points []Vec, lengths []float64, start, end, strokeDash float64, budget int) [][]Vec {
	if len(points) < 2 || len(lengths) != len(points) || end <= start {
		return nil
	}
	dash := max(.18, strokeDash*.07)
	steps := max(1, min(max(1, budget), int(math.Ceil(lengths[len(lengths)-1]/(dash*1.65)))))
	var sections [][]Vec
	for i := 0; i < steps; i++ {
		from, to := max(start, float64(i)/float64(steps)), min(end, min(1, (float64(i)+.62)/float64(steps)))
		if to > from {
			sections = append(sections, nativeRouteSection(points, lengths, from, to))
		}
	}
	return sections
}

// Corners are quadratic curves restricted to the original routed corridor.
// The finite subdivision count bounds both geometry and traffic interpolation.
func nativeRoundedRoute(points []Vec) []Vec {
	clean := make([]Vec, 0, len(points))
	for _, p := range points {
		if len(clean) == 0 || nlen(nsub(p, clean[len(clean)-1])) > 1e-6 {
			clean = append(clean, p)
		}
	}
	points = clean
	if len(points) < 2 {
		return append([]Vec(nil), points...)
	}
	out := []Vec{points[0]}
	for i := 1; i < len(points)-1; i++ {
		p, a, c := points[i], points[i-1], points[i+1]
		r := min(.18, min(nlen(nsub(p, a))/4, nlen(nsub(c, p))/4))
		entry := nadd(p, nmul(nunit(nsub(a, p)), r))
		exit := nadd(p, nmul(nunit(nsub(c, p)), r))
		out = append(out, entry)
		for j := 1; j <= 12; j++ {
			t := float64(j) / 12
			q := nadd(nadd(nmul(entry, (1-t)*(1-t)), nmul(p, 2*(1-t)*t)), nmul(exit, t*t))
			if entry.Y == p.Y && p.Y == exit.Y {
				q.Y = p.Y
			}
			out = append(out, q)
		}
	}
	return append(out, points[len(points)-1])
}

func (b *meshBuilder) tube(points []Vec, radius float64, mat *Material) {
	if len(points) < 2 || radius <= 0 {
		return
	}
	// Shared perpendicular rings keep the tube watertight through flat corners
	// and the small slopes at crossing bridges.
	rings := make([][6]Vertex, len(points))
	for i, p := range points {
		a, c := points[max(0, i-1)], points[min(len(points)-1, i+1)]
		tangent := nunit(nsub(c, a))
		side := nunit(ncross(tangent, nv(0, 1, 0)))
		up := nunit(ncross(side, tangent))
		for j := 0; j < 6; j++ {
			angle := 2 * math.Pi * float64(j) / 6
			n := nadd(nmul(up, math.Cos(angle)), nmul(side, math.Sin(angle)))
			rings[i][j] = Vertex{Position: nadd(p, nmul(n, radius)), Normal: n}
		}
	}
	for i := 1; i < len(rings); i++ {
		for j := 0; j < 6; j++ {
			next := (j + 1) % 6
			a, c, d, e := rings[i-1][j], rings[i-1][next], rings[i][next], rings[i][j]
			b.triangle(a, c, d, mat, true)
			b.triangle(a, d, e, mat, true)
		}
	}
}

// nativeArrowClearance excludes this connection's own core/casing from hollow
// arrow interiors or pointed tapers. Packet paths retain the original port.
func nativeArrowClearance(kind d2target.Arrowhead, strokeWidth int) float64 {
	switch kind {
	case d2target.ArrowArrowhead:
		w, _ := kind.Dimensions(float64(strokeWidth))
		return w * .0075 // Concave rear-center vertex is one quarter of the width.
	case d2target.TriangleArrowhead:
		w, _ := kind.Dimensions(float64(strokeWidth))
		return w * .01 // The broad rear base receives the wire.
	case d2target.FilledDiamondArrowhead, d2target.FilledCircleArrowhead, d2target.FilledBoxArrowhead:
		w, _ := kind.Dimensions(float64(strokeWidth))
		return w * .005 // Join at the broad center, before the taper or curved rim.
	case d2target.UnfilledTriangleArrowhead, d2target.DiamondArrowhead, d2target.CircleArrowhead, d2target.BoxArrowhead, d2target.CrossArrowhead:
		w, _ := kind.Dimensions(float64(strokeWidth))
		return w * .01
	case d2target.CfOne, d2target.CfMany, d2target.CfOneRequired, d2target.CfManyRequired:
		w, _ := kind.Dimensions(float64(strokeWidth))
		return (w + 3 + float64(strokeWidth)*1.8) * .01
	default:
		return 0
	}
}

func (b *meshBuilder) arrow(kind string, at, direction Vec, mat *Material, widths ...int) {
	width := 2
	if len(widths) > 0 {
		width = widths[0]
	}
	opacity := 1.
	if mat != nil {
		opacity = float64(mat.Color.A) / 255
	}
	b.arrowWithOpacity(kind, at, direction, mat, width, opacity)
}

func (b *meshBuilder) arrowWithOpacity(kind string, at, direction Vec, mat *Material, width int, opacity float64) {
	if kind == "" || kind == "none" || mat == nil {
		return
	}
	stroke := float64(max(0, width))
	scale := b.scale
	if scale <= 0 {
		scale = .01
	}
	w, h := d2target.Arrowhead(kind).Dimensions(stroke)
	if w <= 0 || h <= 0 {
		return
	}
	along := nunit(nv(direction.X, 0, direction.Z))
	side := nv(-along.Z, 0, along.X)
	// D2 marker coordinates point along +X. Its tip becomes the exact port.
	point := func(x, y float64) Vec { return nadd(at, nadd(nmul(along, (x-w)*scale), nmul(side, (y-h/2)*scale))) }
	line := func(coords [][2]float64) {
		if stroke <= 0 {
			return
		}
		pts := make([]Vec, len(coords))
		for i, p := range coords {
			pts[i] = point(p[0], p[1])
		}
		b.tube(pts, stroke*scale/2, mat)
	}
	polygon := func(coords [][2]float64, paint *Material) {
		p := make([]Vec, len(coords))
		for i, v := range coords {
			p[i] = point(v[0], v[1])
		}
		b.extrudedProfile(p, at.Y, paint, nil)
	}
	fill := func(coords [][2]float64) { polygon(coords, mat) }
	background := nativeMaterial(b.arrowBackground, .8, 0, opacity)
	background.Color = nativePaint(b.arrowBackground, "#ffffff")
	background.Color.A = uint8(math.Round(float64(background.Color.A) * math.Max(0, math.Min(1, opacity))))
	background.Unlit = true
	knockout := func(coords [][2]float64) { polygon(coords, background) }
	first := len(b.triangles)
	defer func() {
		// The marker is a printed knockout layer, not a raised physical
		// plate. Depth bias excludes crossing wire ink while leaving all
		// source coordinates and endpoints on their original plane.
		bias := math.Max(.012, math.Min(.075, stroke*.012)) * 1.1
		for i := first; i < len(b.triangles); i++ {
			b.triangles[i].DepthBias = bias
			b.triangles[i].CastShadow = false
			if b.triangles[i].Material != background {
				b.triangles[i].DepthBias += .002
			}
		}
	}()
	circle := func(cx, cy, r float64, solid bool) {
		p := make([][2]float64, 49)
		for i := range p {
			a := 2 * math.Pi * float64(i) / 48
			p[i] = [2]float64{cx + r*math.Cos(a), cy + r*math.Sin(a)}
		}
		if solid {
			fill(p[:48])
		} else {
			knockout(p[:48])
			line(p)
		}
	}
	inset := stroke / 2
	switch d2target.Arrowhead(kind) {
	case d2target.ArrowArrowhead:
		fill([][2]float64{{0, 0}, {w, h / 2}, {0, h}, {w / 4, h / 2}})
	case d2target.TriangleArrowhead:
		fill([][2]float64{{0, 0}, {w, h / 2}, {0, h}})
	case d2target.LineArrowhead:
		line([][2]float64{{inset, inset}, {w - inset, h / 2}, {inset, h - inset}})
	case d2target.UnfilledTriangleArrowhead:
		knockout([][2]float64{{inset, inset}, {w - inset, h / 2}, {inset, h - inset}})
		line([][2]float64{{inset, inset}, {w - inset, h / 2}, {inset, h - inset}, {inset, inset}})
	case d2target.DiamondArrowhead:
		knockout([][2]float64{{0, h / 2}, {w / 2, h / 8}, {w, h / 2}, {w / 2, h * .9}})
		line([][2]float64{{0, h / 2}, {w / 2, h / 8}, {w, h / 2}, {w / 2, h * .9}, {0, h / 2}})
	case d2target.FilledDiamondArrowhead:
		fill([][2]float64{{0, h / 2}, {w / 2, 0}, {w, h / 2}, {w / 2, h}})
	case d2target.CircleArrowhead:
		circle(w/2+inset, h/2, max(0, w/2-stroke), false)
	case d2target.FilledCircleArrowhead:
		circle(w/2+inset, h/2, max(0, w/2-inset), true)
	case d2target.BoxArrowhead:
		knockout([][2]float64{{inset, inset}, {w - inset, inset}, {w - inset, h - inset}, {inset, h - inset}})
		line([][2]float64{{inset, inset}, {w - inset, inset}, {w - inset, h - inset}, {inset, h - inset}, {inset, inset}})
	case d2target.FilledBoxArrowhead:
		fill([][2]float64{{0, 0}, {w, 0}, {w, h}, {0, h}})
	case d2target.CrossArrowhead:
		q := stroke / 8
		p := [][2]float64{{0, h/2 + q}, {w/2 - q, h/2 + q}, {w/2 - q, h}, {w/2 + q, h}, {w/2 + q, h/2 + q}, {w, h/2 + q}, {w, h/2 - q}, {w/2 + q, h/2 - q}, {w/2 + q, 0}, {w/2 - q, 0}, {w/2 - q, h/2 - q}, {0, h/2 - q}}
		for i, v := range p {
			x, y := v[0]-w/2, v[1]-h/2
			p[i] = [2]float64{w/2 + (x-y)/math.Sqrt2, h/2 + (x+y)/math.Sqrt2}
		}
		p = append(p, p[0])
		knockout(p[:len(p)-1])
		line(p)
		line([][2]float64{{w / 2, h / 2}, {w, h / 2}})
	case d2target.CfOne, d2target.CfMany, d2target.CfOneRequired, d2target.CfManyRequired:
		offset := 3 + stroke*1.8
		// Crow's-foot's actual marker tip includes its offset beyond nominal width.
		at = nsub(at, nmul(along, offset*scale))
		if kind == string(d2target.CfOneRequired) || kind == string(d2target.CfManyRequired) {
			line([][2]float64{{offset, 0}, {offset, h}})
		} else {
			circle(offset/2+2, h/2, offset/2, false)
		}
		line([][2]float64{{w - 3, h / 2}, {w + offset, h / 2}})
		if kind == string(d2target.CfMany) || kind == string(d2target.CfManyRequired) {
			line([][2]float64{{w + offset, 0}, {offset + 3, h / 2}, {w + offset, h}})
		} else {
			line([][2]float64{{offset * 2, 0}, {offset * 2, h}})
		}
	}
}

func captionPosition(points []Vec, fraction float64) (center Vec, angle, length float64) {
	lengths := routeLengths(points)
	if len(points) < 2 {
		return Vec{}, 0, 1
	}
	total := lengths[len(lengths)-1]
	target := total * fraction
	score := -math.MaxFloat64
	chosen := 1
	for i := 1; i < len(points); i++ {
		l := lengths[i] - lengths[i-1]
		if l < 1e-6 {
			continue
		}
		weight := 3.
		if fraction == .5 {
			weight = .35
		}
		s := l - math.Abs(lengths[i-1]+l/2-target)*weight
		if s > score {
			score, chosen = s, i
		}
	}
	a, c := points[chosen-1], points[chosen]
	length = max(1e-6, lengths[chosen]-lengths[chosen-1])
	direction := nunit(nsub(c, a))
	if direction.X < -1e-6 || math.Abs(direction.X) < 1e-6 && direction.Z < 0 {
		direction = nmul(direction, -1)
	}
	along := .5
	if fraction != .5 {
		along = max(.15, min(.85, (target-lengths[chosen-1])/length))
	}
	center = nlerp(a, c, along)
	center.Y += .006
	center.X += direction.Z * .25
	center.Z -= direction.X * .25
	return center, math.Atan2(direction.Z, direction.X), length
}

func (b *meshBuilder) edges(edges []d2isometric.Edge, placer *routeCaptionPlacer, scenes ...*d2isometric.Scene) []packetRoute {
	packets := []packetRoute{}
	var nodes []d2isometric.Node
	var boards []d2isometric.Board
	var scene *d2isometric.Scene
	if len(scenes) > 0 && scenes[0] != nil {
		scene = scenes[0]
		nodes, boards = scenes[0].Nodes, scenes[0].Boards
	}
	captionPaint := nativeCaptionPaint(scene)
	lanePaths, paths, err := nativeEdgeRoutes(b.ctx, edges, nodes, boards, b.hierarchySupports)
	if err != nil {
		b.err = err
		return packets
	}
	// Reserve fixed source captions and all visible wires before placing any
	// automatic caption. Edge order must not let an earlier automatic label
	// occupy a later authored anchor, or cover a route drawn later in the pass.
	for i, edge := range edges {
		if edge.Opacity <= 0 || len(lanePaths[i]) < 2 {
			continue
		}
		if nativeSequenceEdge(edge) {
			b.reserveSequenceEdge(edge, paths[i], placer)
			continue
		}
		if edge.StrokeWidth > 0 {
			placer.AvoidRoute(paths[i], nativeRouteRadius(edge)*1.6)
		}
		hasIcon := edge.Icon != "" && edge.Metadata.Original.Icon != nil
		if edge.Label == "" && !hasIcon {
			continue
		}
		pw, pd := nativeRouteCaptionSize(edge, .5, edge.Metadata.Original.Text, hasIcon, b.scale)
		if surface, positioned := nativeConnectionCaptionSurface(lanePaths[i], edge.Metadata.Original, pw, pd, b.scale); positioned {
			if nativeCaptionOnEdge(edge.Metadata.Original) {
				surface.center.Y += nativeRouteRadius(edge)
			}
			placer.reserve(captionRect(surface, true))
		}
	}
	for edgeIndex, edge := range edges {
		if b.err != nil {
			return packets
		}
		if len(edge.Points) < 2 || edge.Opacity <= 0 {
			continue
		}
		// BuildScene already bounds routes; additionally cap native tessellation.
		if len(edge.Points) > 10000 {
			b.err = fmt.Errorf("isometric route exceeds 10000 control points")
			return packets
		}
		if nativeSequenceEdge(edge) {
			packets = append(packets, b.sequenceEdge(edge, paths[edgeIndex], scene)...)
			continue
		}
		tint := nativeRouteTint(edge)
		mat := nativeMaterial(tint, .6, .05, edge.Opacity)
		mat.Unlit = true
		wireOpacity := float64(mat.Color.A) / 255
		points := paths[edgeIndex]
		if len(points) < 2 {
			continue
		}
		lengths := routeLengths(points)
		length := lengths[len(lengths)-1]
		start, end := b.visibleArrowRange(edge, points)
		displayPoints := points
		if start > 0 || end < 1 {
			displayPoints = nativeRouteSection(points, lengths, start, end)
		}
		wireStart, wireEnd := start, end
		if length > 1e-9 {
			wireStart += nativeArrowClearance(edge.SourceArrow, edge.StrokeWidth) / length
			wireEnd -= nativeArrowClearance(edge.TargetArrow, edge.StrokeWidth) / length
		}
		wirePoints := displayPoints
		if wireStart > 0 || wireEnd < 1 {
			wirePoints = nativeRouteSection(points, lengths, wireStart, wireEnd)
		}
		wireFirst := len(b.triangles)
		if edge.StrokeWidth > 0 && len(wirePoints) > 1 {
			radius := nativeRouteRadius(edge)
			if edge.StrokeDash <= 0 {
				b.routeCasing(wirePoints, radius, wireOpacity)
				b.routeInk(wirePoints, radius, mat)
			} else {
				// Dash positions belong to the complete routed connection.
				// Endpoint visibility clips them without shifting their phase.
				for _, seg := range nativeRouteDashes(points, lengths, wireStart, wireEnd, edge.StrokeDash, max(4, 8000/max(1, len(edges)))) {
					b.routeCasing(seg, radius, wireOpacity)
					b.routeInk(seg, radius, mat)
				}
			}
		}
		hasIcon := edge.Icon != "" && edge.Metadata.Original.Icon != nil
		if (edge.Label != "" || hasIcon) && nativeCaptionOnEdge(edge.Metadata.Original) {
			pw, pd := nativeRouteCaptionSize(edge, .5, edge.Metadata.Original.Text, hasIcon, b.scale)
			if surface, ok := nativeConnectionCaptionSurface(lanePaths[edgeIndex], edge.Metadata.Original, pw, pd, b.scale); ok {
				surface.center.Y += nativeRouteRadius(edge)
				// The print area can be wider than its text. Leave a close gap
				// around the compiled label dimensions, preserving the markers.
				if !hasIcon && edge.Metadata.Original.LabelWidth > 0 {
					surface.width = min(surface.width, float64(edge.Metadata.Original.LabelWidth)*b.scale+.1)
				}
				b.routeCaptionKnockout(wireFirst, surface)
			}
		}
		sourceDir := nsub(displayPoints[0], displayPoints[1])
		targetDir := nsub(displayPoints[len(displayPoints)-1], displayPoints[len(displayPoints)-2])
		b.arrowWithOpacity(string(edge.SourceArrow), displayPoints[0], sourceDir, mat, edge.StrokeWidth, edge.Opacity)
		b.arrowWithOpacity(string(edge.TargetArrow), displayPoints[len(displayPoints)-1], targetDir, mat, edge.StrokeWidth, edge.Opacity)
		if edge.Metadata.Original.Animated && edge.StrokeWidth > 0 && wireOpacity > 0 {
			packet := nativeMaterial("#e9f8ff", .2, .15, wireOpacity)
			packet.Emissive = nativePaint(tint, "#657e9e")
			forward, reverse := edge.TargetArrow != "" && edge.TargetArrow != "none", edge.SourceArrow != "" && edge.SourceArrow != "none"
			if !forward && !reverse {
				forward = true
			}
			packets = append(packets, packetRoute{points: points, lengths: lengths, material: packet, forward: forward, reverse: reverse})
		}
		captions := []struct {
			text     string
			fraction float64
			style    d2target.Text
			ink      string
		}{{edge.Label, .5, edge.Metadata.Original.Text, tint}}
		// Connection.Fill is the compiled caption backing, separate from the
		// wire stroke and the Text's optional label-fill value.
		if edge.Metadata.Original.Fill != "" {
			captions[0].style.LabelFill = captionPaint(edge.Metadata.Original.Fill)
		}
		if edge.FontExplicit {
			captions[0].ink = edge.FontColor
		}
		if edge.SourceLabel != nil {
			captions = append(captions, struct {
				text     string
				fraction float64
				style    d2target.Text
				ink      string
			}{edge.SourceLabel.Label, .08, *edge.SourceLabel, edge.SourceLabel.Color})
		}
		if edge.TargetLabel != nil {
			captions = append(captions, struct {
				text     string
				fraction float64
				style    d2target.Text
				ink      string
			}{edge.TargetLabel.Label, .92, *edge.TargetLabel, edge.TargetLabel.Color})
		}
		for captionIndex, caption := range captions {
			caption.style.LabelFill = captionPaint(caption.style.LabelFill)
			caption.ink = captionPaint(caption.ink)
			hasIcon := captionIndex == 0 && edge.Icon != "" && edge.Metadata.Original.Icon != nil
			if caption.text == "" && !hasIcon {
				continue
			}
			pw, pd := nativeRouteCaptionSize(edge, caption.fraction, caption.style, hasIcon, b.scale)
			var surface labelSurface
			positioned := false
			if captionIndex == 0 {
				surface, positioned = nativeConnectionCaptionSurface(lanePaths[edgeIndex], edge.Metadata.Original, pw, pd, b.scale)
			}
			if positioned {
				if nativeCaptionOnEdge(edge.Metadata.Original) {
					// Keep the printed text and any authored fill above
					// the cable crown.
					surface.center.Y += nativeRouteRadius(edge)
				}
			} else {
				captionPoints := lanePaths[edgeIndex]
				if captionIndex > 0 {
					// Endpoint captions follow the visible solid contact. Main
					// captions keep their original authored route anchors.
					captionPoints = displayPoints
				}
				surface = placer.Place(captionPoints, caption.fraction, pw, pd)
			}
			if captionIndex == 0 {
				b.addSurfaceLink(edge.Link, edge.Tooltip, surface)
			}
			if hasIcon {
				shape := d2target.Shape{ID: edge.ID, Text: edge.Metadata.Original.Text, Icon: edge.Metadata.Original.Icon, IconPosition: edge.Metadata.Original.IconPosition}
				surface = b.shapeIcon(shape, surface, edge.Opacity, "edge", edge.Metadata.Original.IconBorderRadius)
			}
			b.label(caption.text, surface, caption.style, caption.ink, edge.Opacity, "edge")
		}
	}
	return packets
}
