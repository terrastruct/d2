package d2isometricimg

import (
	"math"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/d2themes"
	"github.com/d2lang/d2/d2themes/d2themescatalog"
	"github.com/d2lang/d2/lib/label"
)

func nativeCaptionPaint(scene *d2isometric.Scene) func(string) string {
	theme := d2themescatalog.Find(0)
	if scene != nil {
		theme = d2themescatalog.Find(scene.ThemeID)
		theme.ApplyOverrides(scene.ThemeOverrides)
	}
	return func(value string) string { return d2themes.ResolveThemeColor(theme, value) }
}

func nativeCaptionOnEdge(original d2target.Connection) bool {
	return label.FromString(original.LabelPosition).IsOnEdge()
}

// Lane fans subdivide a source leg into shorter pieces. Caption reservations
// and painting use the same original print dimensions, regardless of lanes.
func nativeRouteCaptionSize(edge d2isometric.Edge, fraction float64, style d2target.Text, hasIcon bool, scale float64) (width, depth float64) {
	_, _, span := captionPosition(edge.Points, fraction)
	share := .86
	if fraction == .5 && (edge.SourceLabel != nil || edge.TargetLabel != nil) {
		share = .46
	} else if fraction != .5 && (edge.Label != "" || edge.Icon != "" || fraction < .5 && edge.TargetLabel != nil && edge.TargetLabel.Label != "" || fraction > .5 && edge.SourceLabel != nil && edge.SourceLabel.Label != "") {
		share = .23
	}
	width = min(span*share, max(1, float64(style.LabelWidth)*scale+.1))
	lh := style.LabelHeight
	if lh <= 0 {
		lh = 25
	}
	depth = max(.25, min(.7, float64(lh)*scale+.06))
	if hasIcon {
		if edge.Label == "" {
			width, depth = min(span*share, .38), .38
		} else {
			width = min(span*share, width+.38)
			depth = max(depth, .38)
		}
	}
	return width, depth
}

// The compiled label position and percentage are semantic placement inputs.
// Apply them along the displayed planar route, retaining a flat, leg-aligned
// print surface. Unspecified positions still use the collision-aware placer.
func nativeConnectionCaptionSurface(points []Vec, original d2target.Connection, width, depth, scale float64) (labelSurface, bool) {
	position := label.FromString(original.LabelPosition)
	fraction, side := .5, 0.
	switch position {
	case label.InsideMiddleLeft, label.OutsideTopLeft, label.OutsideBottomLeft:
		fraction = .25
	case label.InsideMiddleCenter, label.OutsideTopCenter, label.OutsideBottomCenter:
	case label.InsideMiddleRight, label.OutsideTopRight, label.OutsideBottomRight:
		fraction = .75
	case label.UnlockedTop, label.UnlockedMiddle, label.UnlockedBottom:
		fraction = max(0, min(1, original.LabelPercentage))
	default:
		return labelSurface{}, false
	}
	switch position {
	case label.OutsideTopLeft, label.OutsideTopCenter, label.OutsideTopRight, label.UnlockedTop:
		side = -1
	case label.OutsideBottomLeft, label.OutsideBottomCenter, label.OutsideBottomRight, label.UnlockedBottom:
		side = 1
	}
	if !captionFinite(fraction, width, depth, scale) || width <= 0 || depth <= 0 || scale <= 0 || len(points) < 2 || len(points) > 10000 {
		return labelSurface{}, false
	}
	lengths := routeLengths(points)
	total := lengths[len(lengths)-1]
	if total <= 1e-9 || !captionFinite(total) {
		return labelSurface{}, false
	}
	distance := fraction * total
	for i := 1; i < len(points); i++ {
		length := lengths[i] - lengths[i-1]
		if length <= 1e-9 || lengths[i] < distance {
			continue
		}
		direction := nunit(nsub(points[i], points[i-1]))
		center := nlerp(points[i-1], points[i], (distance-lengths[i-1])/length)
		// Preserve above/below relative to the source route direction. Unlike
		// upright SVG text, rotated text has depth as its normal extent.
		offset := side * (depth/2 + (float64(original.StrokeWidth)/2+label.PADDING)*scale)
		center.X -= direction.Z * offset
		center.Z += direction.X * offset
		center.Y += .006
		if direction.X < -1e-6 || math.Abs(direction.X) < 1e-6 && direction.Z < 0 {
			direction = nmul(direction, -1)
		}
		return labelSurface{center: center, width: width, depth: depth, angle: math.Atan2(direction.Z, direction.X)}, true
	}
	return labelSurface{}, false
}
