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

// Compiled text metrics describe the print area independently of the route.
// Curves and lane fans subdivide legs into arbitrarily short pieces; neither
// those pieces nor neighboring captions may shrink authored text. Placement
// handles conflicts while retaining these dimensions, including multiline text.
func nativeRouteCaptionSize(style d2target.Text, hasIcon bool, scale float64) (width, depth float64) {
	// Raw API callers may omit metrics. Keep a bounded fallback instead of
	// estimating from untrusted text length; the painter still enforces its
	// source, glyph and pixel budgets. Compiled metrics take precedence.
	width = max(1, float64(float64(style.LabelWidth)*scale)+.1)
	lh := style.LabelHeight
	if lh <= 0 {
		lh = 25
	}
	depth = max(.25, float64(float64(lh)*scale)+.06)
	if hasIcon {
		if style.Label == "" {
			return .38, .38
		}
		// surfaceIconLayout reserves a small glyph-overhang margin and gap.
		// Include them before adding the icon lane, so even long labels keep
		// their complete print area rather than pushing the icon above them.
		width = max(width, float64(float64(style.LabelWidth)*scale*1.06)+.04) + .38 + .08
		depth = max(depth, .38)
	}
	return width, depth
}

// The compiled label position and percentage are semantic placement inputs.
// Apply them along the displayed planar route, retaining a flat print surface
// that follows its leg when it fits. Unspecified positions still use the
// collision-aware placer.
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
		angle := math.Atan2(direction.Z, direction.X)
		start, end := nativeCaptionRouteRun(points, lengths, i)
		// D2 reserves horizontal label dimensions. A short leg cannot hold
		// that entire width after rotating it toward the endpoints. Retain
		// the source X orientation when it would overrun the usable run,
		// including clearance for the raised endpoint faces. This changes
		// only print direction and restores the compiled source label anchor;
		// font size and the physical plane stay unchanged.
		if width+2*depth > 2*min(distance-start, end-distance)+1e-9 {
			angle = 0
			if source, ok := nativeCaptionSourceCenter(points, original, scale); ok {
				center.X, center.Z = source.X, source.Z
			}
		}
		return labelSurface{center: center, width: width, depth: depth, angle: angle}, true
	}
	return labelSurface{}, false
}

// Aggregate a direction-consistent run around the caption anchor. Curved routes
// contain many tiny tessellation segments, while elbows delimit distinct legs.
// Their individual segment lengths must never determine text orientation.
func nativeCaptionRouteRun(points []Vec, lengths []float64, index int) (start, end float64) {
	direction := nunit(nsub(points[index], points[index-1]))
	aligned := func(i int) bool {
		if lengths[i]-lengths[i-1] <= 1e-9 {
			return true
		}
		return ndot(direction, nunit(nsub(points[i], points[i-1]))) >= math.Sqrt(3)/2
	}
	start, end = lengths[index-1], lengths[index]
	for i := index - 1; i >= 1 && aligned(i); i-- {
		start = lengths[i-1]
	}
	for i := index + 1; i < len(points) && aligned(i); i++ {
		end = lengths[i]
	}
	return start, end
}

// Curved D2 routes retain control points for their compiled label placement.
// Using the tessellated curve's midpoint instead moves labels inward and can
// overlap two labels whose source allocations were disjoint. Translate the
// compiled label rectangle with the same source-to-scene offset as the route.
func nativeCaptionSourceCenter(points []Vec, original d2target.Connection, scale float64) (Vec, bool) {
	if len(original.Route) < 2 || len(original.Route) > 10000 {
		return Vec{}, false
	}
	for _, p := range original.Route {
		if p == nil || !captionFinite(p.X, p.Y) || math.Abs(p.X) > rasterCoordinateLimit || math.Abs(p.Y) > rasterCoordinateLimit {
			return Vec{}, false
		}
	}
	p := original.GetLabelTopLeft()
	if p == nil || !captionFinite(p.X, p.Y) {
		return Vec{}, false
	}
	return nv(points[0].X+(p.X+float64(original.LabelWidth)/2-original.Route[0].X)*scale, 0,
		points[0].Z+(p.Y+float64(original.LabelHeight)/2-original.Route[0].Y)*scale), true
}
