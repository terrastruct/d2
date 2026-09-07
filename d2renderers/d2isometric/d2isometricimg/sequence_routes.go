package d2isometricimg

import (
	"context"
	"fmt"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
	"github.com/d2lang/d2/lib/svg"
)

const (
	nativeSequenceLifelineY = .10
	nativeSequenceMessageY  = .30
)

func nativeSequenceEdge(edge d2isometric.Edge) bool {
	return edge.SequenceRole == "lifeline" || edge.SequenceRole == "message"
}

func nativeSequenceEdgeY(edge d2isometric.Edge) float64 {
	if edge.SequenceRole == "lifeline" {
		return nativeSequenceLifelineY
	}
	return nativeSequenceMessageY
}

// Sequence edges never enter dependency routing. Partitioning before those
// passes also prevents messages from adding bridges or lanes to ordinary
// connections in a mixed scene. All input slices remain source-owned.
func nativeEdgeRoutes(ctx context.Context, edges []d2isometric.Edge, nodes []d2isometric.Node, boards []d2isometric.Board, support ...map[string]float64) (lanes, paths [][]Vec, err error) {
	if ctx == nil {
		return nil, nil, fmt.Errorf("isometric routes require a context")
	}
	lanes, paths = make([][]Vec, len(edges)), make([][]Vec, len(edges))
	var ordinary []d2isometric.Edge
	var indices []int
	total := 0
	for i, edge := range edges {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		if len(edge.Points) > 10000 || total > nativeBridgeMaxPoints-len(edge.Points) {
			return nil, nil, fmt.Errorf("isometric routes exceed point budget")
		}
		total += len(edge.Points)
		if !nativeSequenceEdge(edge) {
			ordinary, indices = append(ordinary, edge), append(indices, i)
			continue
		}
		points := append([]Vec(nil), edge.Points...)
		for j := range points {
			if !captionFinite(points[j].X, points[j].Y, points[j].Z) {
				return nil, nil, fmt.Errorf("isometric sequence route %q has invalid coordinates", edge.ID)
			}
			points[j].Y = nativeSequenceEdgeY(edge)
		}
		lanes[i], paths[i] = points, points
	}
	ordinaryLanes, err := nativeLaneRoutes(ctx, ordinary, nodes, boards)
	if err != nil {
		return nil, nil, err
	}
	resolved := append([]d2isometric.Edge(nil), ordinary...)
	for i := range resolved {
		resolved[i].Points = ordinaryLanes[i]
	}
	ordinaryPaths, err := nativeBridgeRoutes(ctx, resolved)
	if err != nil {
		return nil, nil, err
	}
	ordinaryPaths, err = nativeSolidContactRoutes(ctx, ordinary, nodes, ordinaryPaths, support...)
	if err != nil {
		return nil, nil, err
	}
	for i, index := range indices {
		lanes[index], paths[index] = ordinaryLanes[i], ordinaryPaths[i]
	}
	return lanes, paths, nil
}

func nativeSequenceTint(edge d2isometric.Edge) string {
	if edge.StrokeExplicit {
		return edge.Stroke
	}
	return "#263c4e"
}

// Use the native 2D caption API, including its self-loop and endpoint rules.
// Compiled text remains aligned with the source X axis on the flat surface;
// route direction does not rotate or reposition that text.
type nativeSequenceCaption struct {
	style   d2target.Text
	surface labelSurface
	main    bool
}

func nativeSequenceCaptionData(edge d2isometric.Edge, scale float64) ([]nativeSequenceCaption, labelSurface) {
	if scale <= 0 || !captionFinite(scale) || len(edge.Points) < 2 {
		return nil, labelSurface{}
	}
	original := edge.Metadata.Original
	offsetX, offsetZ := 0., 0.
	if len(original.Route) >= 2 && original.Route[0] != nil {
		offsetX, offsetZ = edge.Points[0].X-original.Route[0].X*scale, edge.Points[0].Z-original.Route[0].Y*scale
	} else {
		original.Route = make([]*geo.Point, len(edge.Points))
		for i, point := range edge.Points {
			original.Route[i] = geo.NewPoint(point.X/scale, point.Z/scale)
		}
	}
	for _, p := range original.Route {
		if p == nil || !captionFinite(p.X, p.Y) {
			return nil, labelSurface{}
		}
	}
	if label.FromString(original.LabelPosition) == label.Unset {
		original.LabelPosition = label.InsideMiddleCenter.String()
	}
	if label.FromString(original.IconPosition) == label.Unset {
		original.IconPosition = label.InsideMiddleCenter.String()
	}
	surface := func(topLeft *geo.Point, width, height int) labelSurface {
		if topLeft == nil || !captionFinite(topLeft.X, topLeft.Y) {
			return labelSurface{}
		}
		return labelSurface{center: nv(offsetX+(topLeft.X+float64(width)/2)*scale, nativeSequenceEdgeY(edge)+.006, offsetZ+(topLeft.Y+float64(height)/2)*scale), width: float64(max(1, width)) * scale, depth: float64(max(1, height)) * scale}
	}
	var captions []nativeSequenceCaption
	if edge.Label != "" {
		style := original.Text
		style.Label = edge.Label
		captions = append(captions, nativeSequenceCaption{style, surface(original.GetLabelTopLeft(), style.LabelWidth, style.LabelHeight), true})
	}
	for _, endpoint := range []struct {
		text *d2target.Text
		dst  bool
	}{{edge.SourceLabel, false}, {edge.TargetLabel, true}} {
		if endpoint.text == nil || endpoint.text.Label == "" {
			continue
		}
		if endpoint.dst {
			original.DstLabel = endpoint.text
		} else {
			original.SrcLabel = endpoint.text
		}
		style := *endpoint.text
		// Native D2 endpoint captions use the main connection's font size
		// and the primary italic face, independently of a rich main label.
		style.FontSize, style.FontFamily, style.Bold, style.Italic, style.Underline = original.FontSize, "default", false, true, false
		captions = append(captions, nativeSequenceCaption{style, surface(original.GetArrowheadLabelPosition(endpoint.dst), style.LabelWidth, style.LabelHeight), false})
	}
	var icon labelSurface
	if original.Icon != nil {
		icon = surface(original.GetIconPosition(), d2target.DEFAULT_ICON_SIZE, d2target.DEFAULT_ICON_SIZE)
	}
	return captions, icon
}

func (b *meshBuilder) reserveSequenceEdge(edge d2isometric.Edge, points []Vec, placer *routeCaptionPlacer) {
	if edge.StrokeWidth > 0 {
		placer.AvoidRoute(points, float64(edge.StrokeWidth)*b.scale/2)
	}
	captions, icon := nativeSequenceCaptionData(edge, b.scale)
	for _, caption := range captions {
		placer.reserve(captionRect(caption.surface, true))
	}
	if icon.width > 0 && icon.depth > 0 {
		placer.reserve(captionRect(icon, true))
	}
}

func (b *meshBuilder) sequenceEdge(edge d2isometric.Edge, points []Vec, scene *d2isometric.Scene) []packetRoute {
	if len(points) < 2 || edge.Opacity <= 0 {
		return nil
	}
	tint := nativeSequenceTint(edge)
	mat := nativeMaterial(tint, 1, 0, edge.Opacity)
	mat.Unlit = true
	radius := float64(edge.StrokeWidth) * b.scale / 2
	lengths := routeLengths(points)
	total := lengths[len(lengths)-1]
	first := len(b.triangles)
	if radius > 0 && total > 0 {
		wire := points
		start, end := nativeArrowClearance(edge.SourceArrow, edge.StrokeWidth)/total, 1-nativeArrowClearance(edge.TargetArrow, edge.StrokeWidth)/total
		if start > 0 || end < 1 {
			wire = nativeRouteSection(points, lengths, start, end)
		}
		dashStyle := edge.StrokeDash
		if dashStyle <= 0 {
			b.routeInk(wire, radius, mat)
		} else {
			wireLengths := routeLengths(wire)
			if len(wireLengths) > 1 {
				length := wireLengths[len(wireLengths)-1]
				dash, gap := svg.GetStrokeDashAttributes(float64(edge.StrokeWidth), dashStyle)
				dash, gap = dash*b.scale, gap*b.scale
				if !captionFinite(dash, gap) || dash <= 0 || gap <= 0 {
					dash, gap = float64(edge.StrokeWidth)*dashStyle*b.scale, float64(edge.StrokeWidth)*dashStyle*b.scale
				}
				if !captionFinite(dash, gap) || dash <= 0 || gap <= 0 || length/(dash+gap) > 50000 {
					b.err = fmt.Errorf("isometric sequence dashes exceed geometry budget")
					return nil
				}
				for at := 0.; at < length; at += dash + gap {
					b.routeInk(nativeRouteSection(wire, wireLengths, at/length, min(1, (at+dash)/length)), radius, mat)
					if b.err != nil {
						return nil
					}
				}
			}
		}
	}
	captions, icon := nativeSequenceCaptionData(edge, b.scale)
	if nativeCaptionOnEdge(edge.Metadata.Original) {
		for _, caption := range captions {
			if caption.main {
				b.routeCaptionKnockout(first, caption.surface)
				break
			}
		}
	}
	markerStart := len(b.triangles)
	b.arrowWithOpacity(string(edge.SourceArrow), points[0], nsub(points[0], points[1]), mat, edge.StrokeWidth, edge.Opacity)
	b.arrowWithOpacity(string(edge.TargetArrow), points[len(points)-1], nsub(points[len(points)-1], points[len(points)-2]), mat, edge.StrokeWidth, edge.Opacity)
	for i := markerStart; i < len(b.triangles); i++ {
		for j := range b.triangles[i].V {
			b.triangles[i].V[j].Position.Y = nativeSequenceEdgeY(edge)
			b.triangles[i].V[j].Normal = nv(0, 1, 0)
		}
	}
	b.addMeshLink(edge.Link, edge.Tooltip, b.triangles[first:])
	paint := nativeCaptionPaint(scene)
	for _, caption := range captions {
		ink := paint(caption.style.Color)
		if nativeToken(caption.style.Color) {
			// Match the classic wire/outline ink. The default source caption
			// gray loses contrast on physical container plates.
			ink = "#263c4e"
		}
		if caption.main && edge.FontExplicit {
			ink = edge.FontColor
		}
		if caption.main && edge.Metadata.Original.Fill != "" {
			caption.style.LabelFill = edge.Metadata.Original.Fill
		}
		caption.style.LabelFill = paint(caption.style.LabelFill)
		if caption.main {
			b.addSurfaceLink(edge.Link, edge.Tooltip, caption.surface)
		}
		b.label(caption.style.Label, caption.surface, caption.style, ink, edge.Opacity, "edge")
	}
	if icon.width > 0 && icon.depth > 0 {
		b.shapeIcon(d2target.Shape{ID: edge.ID, Icon: edge.Metadata.Original.Icon}, icon, edge.Opacity, "edge", edge.Metadata.Original.IconBorderRadius)
		b.addSurfaceLink(edge.Link, edge.Tooltip, icon)
	}
	if edge.Metadata.Original.Animated && edge.StrokeWidth > 0 && mat.Color.A > 0 {
		packet := nativeMaterial("#e9f8ff", .2, .15, float64(mat.Color.A)/255)
		packet.Emissive = nativePaint(tint, "#263c4e")
		forward, reverse := edge.TargetArrow != "" && edge.TargetArrow != "none", edge.SourceArrow != "" && edge.SourceArrow != "none"
		if !forward && !reverse {
			forward = true
		}
		return []packetRoute{{points: points, lengths: lengths, material: packet, forward: forward, reverse: reverse}}
	}
	return nil
}
