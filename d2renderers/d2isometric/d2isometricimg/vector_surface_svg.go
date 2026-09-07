package d2isometricimg

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"html"
	"image/color"
	"image/png"
	"math"
	"strconv"
	"strings"

	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/internal/fontface"
	"github.com/d2lang/d2/internal/rasterimage"
)

type nativeSurfaceSVGWriter struct {
	ctx                   context.Context
	doc                   *d2scene.Document
	prefix                string
	defs                  strings.Builder
	next, nodes, commands int
	err                   error
	fonts                 map[d2scene.AssetID]*fontface.ParsedFace
	rasterURLs            map[d2scene.AssetID]string
	transform             d2scene.Matrix
	linearGradients       bool
	glyphs                map[nativeSVGFontGlyph]string
}

// nativeSurfaceSVG emits a self-contained fragment in normalized UV space.
// All generated ink remains paths. Only authored raster-image assets use image.
func nativeSurfaceSVG(ctx context.Context, surface *nativeVectorSurface, prefix string) (string, error) {
	if ctx == nil || surface == nil || surface.document == nil || prefix == "" {
		return "", fmt.Errorf("native SVG requires a vector surface and prefix")
	}
	for _, c := range prefix {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return "", fmt.Errorf("native SVG has an invalid resource prefix")
		}
	}
	w := &nativeSurfaceSVGWriter{ctx: ctx, doc: surface.document, prefix: prefix, fonts: make(map[d2scene.AssetID]*fontface.ParsedFace), rasterURLs: make(map[d2scene.AssetID]string)}
	matrix, err := nativeVectorViewport(w.doc)
	if err != nil {
		return "", err
	}
	w.transform = d2scene.Scale(w.doc.LogicalWidth, w.doc.LogicalHeight).Mul(matrix)
	body := w.node(w.doc.Root, 0)
	if w.err != nil {
		return "", w.err
	}
	content := "<g transform=\"" + nativeSVGMatrix(matrix) + "\">" + body + "</g>"
	if surface.capBackground != nil {
		content = "<rect width=\"1\" height=\"1\" fill=\"" + nativeSVGColor(*surface.capBackground) + "\"/>" + content
	}
	if surface.inkCoverage != nil {
		ink, err := nativeSurfaceSVG(ctx, surface.inkCoverage, prefix+"-coverage")
		if err != nil {
			return "", err
		}
		filter, mask := w.id("coverage-filter"), w.id("coverage-mask")
		// The raster compensation uses 8-bit ink coverage. Preserve every one
		// of its 256 input values exactly, with linear interpolation between
		// them for vector antialiasing at any export resolution.
		values := make([]string, 256)
		for i := range values {
			a := float64(i) / 255
			values[i] = nativeSVGNumber((1 - a) / (1 - a*surface.coverageOpacity))
		}
		// SourceAlpha has black RGB. Make its compensated coverage white so
		// SVG 1.1 luminance-mask readers agree with SVG 2 alpha-mask readers.
		w.def("<filter id=\"" + filter + "\" filterUnits=\"userSpaceOnUse\" x=\"0\" y=\"0\" width=\"1\" height=\"1\" color-interpolation-filters=\"sRGB\"><feComponentTransfer in=\"SourceAlpha\"><feFuncR type=\"linear\" slope=\"0\" intercept=\"1\"/><feFuncG type=\"linear\" slope=\"0\" intercept=\"1\"/><feFuncB type=\"linear\" slope=\"0\" intercept=\"1\"/><feFuncA type=\"table\" tableValues=\"" + strings.Join(values, " ") + "\"/></feComponentTransfer></filter>")
		w.def("<mask id=\"" + mask + "\" maskUnits=\"userSpaceOnUse\" x=\"0\" y=\"0\" width=\"1\" height=\"1\" mask-type=\"alpha\"><g filter=\"url(#" + filter + ")\">" + ink + "</g></mask>")
		content = "<g mask=\"url(#" + mask + ")\">" + content + "</g>"
	}
	if w.err != nil {
		return "", w.err
	}
	if !w.withinBytes(len(content) + 13) {
		return "", w.err
	}
	return "<defs>" + w.defs.String() + "</defs>" + content, nil
}

func nativeSVGNumber(n float64) string {
	if n == 0 {
		return "0"
	}
	return strconv.FormatFloat(n, 'g', -1, 64)
}
func nativeSVGMatrix(m d2scene.Matrix) string {
	return "matrix(" + nativeSVGNumber(m.A) + " " + nativeSVGNumber(m.B) + " " + nativeSVGNumber(m.C) + " " + nativeSVGNumber(m.D) + " " + nativeSVGNumber(m.E) + " " + nativeSVGNumber(m.F) + ")"
}
func nativeSVGAttr(name, value string) string {
	return " " + name + "=\"" + html.EscapeString(value) + "\""
}
func nativeSVGFloat(name string, value float64) string {
	return nativeSVGAttr(name, nativeSVGNumber(value))
}
func nativeSVGColor(c color.NRGBA) string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }
func (w *nativeSurfaceSVGWriter) id(kind string) string {
	w.next++
	return fmt.Sprintf("%s-%s-%d", w.prefix, kind, w.next)
}
func (w *nativeSurfaceSVGWriter) admit(depth int) bool {
	if w.err != nil {
		return false
	}
	if err := w.ctx.Err(); err != nil {
		w.err = err
		return false
	}
	w.nodes++
	if depth > 256 || w.nodes > 200000 || w.commands > 2000000 {
		w.err = fmt.Errorf("native SVG surface exceeds node, depth, path or byte budget")
		return false
	}
	return true
}
func (w *nativeSurfaceSVGWriter) withinBytes(size int) bool {
	if w.err != nil {
		return false
	}
	if size > (32<<20)-w.defs.Len() {
		w.err = fmt.Errorf("native SVG surface exceeds 32 MiB byte budget")
		return false
	}
	return true
}
func (w *nativeSurfaceSVGWriter) keep(s string) string {
	if !w.withinBytes(len(s)) {
		return ""
	}
	return s
}
func (w *nativeSurfaceSVGWriter) append(out *strings.Builder, value string) {
	if w.withinBytes(out.Len() + len(value)) {
		out.WriteString(value)
	}
}

func (w *nativeSurfaceSVGWriter) def(s string) {
	if w.keep(s) != "" {
		w.defs.WriteString(s)
	}
}

func (w *nativeSurfaceSVGWriter) node(n *d2scene.Node, depth int) string {
	if n == nil || !w.admit(depth) {
		return ""
	}
	var err error
	n, err = nativeVectorInitialNode(n)
	if err != nil {
		w.err = err
		return ""
	}
	if n.Opacity <= 0 {
		return ""
	}
	if !n.Transform.IsFinite() || !captionFinite(n.Opacity) {
		w.err = fmt.Errorf("native SVG has invalid node transform or opacity")
		return ""
	}
	previousTransform := w.transform
	w.transform = previousTransform.Mul(n.Transform)
	defer func() { w.transform = previousTransform }()
	attrs := nativeSVGAttr("transform", nativeSVGMatrix(n.Transform))
	if n.Opacity < 1 {
		attrs += nativeSVGFloat("opacity", n.Opacity)
	}
	if n.Blend != d2scene.BlendNormal {
		modes := map[d2scene.BlendMode]string{d2scene.BlendMultiply: "multiply", d2scene.BlendDarken: "darken", d2scene.BlendColorBurn: "color-burn", d2scene.BlendOverlay: "overlay", d2scene.BlendLighten: "lighten"}
		mode, ok := modes[n.Blend]
		if !ok {
			w.err = fmt.Errorf("native SVG unsupported blend mode %d", n.Blend)
			return ""
		}
		attrs += nativeSVGAttr("style", "mix-blend-mode:"+mode+";isolation:isolate")
	}
	if n.Clip != nil {
		id := w.id("clip")
		w.def("<clipPath id=\"" + id + "\" clipPathUnits=\"userSpaceOnUse\"><path transform=\"" + nativeSVGMatrix(n.Clip.Transform) + "\" d=\"" + w.path(n.Clip.Path) + "\" clip-rule=\"" + nativeSVGFillRule(n.Clip.Path.FillRule) + "\"/></clipPath>")
		attrs += nativeSVGAttr("clip-path", "url(#"+id+")")
	}
	if n.Mask != nil {
		id := w.id("mask")
		kind := "alpha"
		if n.Mask.Type == d2scene.MaskLuminance {
			kind = "luminance"
		}
		body := w.node(n.Mask.Root, depth+1)
		w.def("<mask id=\"" + id + "\" maskUnits=\"userSpaceOnUse\"" + nativeSVGBoxAttributes(w.surfaceBounds()) + " mask-type=\"" + kind + "\"><g transform=\"" + nativeSVGMatrix(n.Mask.Transform) + "\">" + body + "</g></mask>")
		attrs += nativeSVGAttr("mask", "url(#"+id+")")
	}
	if len(n.Filters) > 0 {
		id := w.id("filter")
		var effects strings.Builder
		for _, filter := range n.Filters {
			switch f := filter.(type) {
			case *d2scene.GaussianBlur:
				if f != nil {
					filter = *f
				}
			case *d2scene.DropShadow:
				if f != nil {
					filter = *f
				}
			}
			switch f := filter.(type) {
			case d2scene.GaussianBlur:
				effects.WriteString("<feGaussianBlur stdDeviation=\"" + nativeSVGNumber(f.SigmaX) + " " + nativeSVGNumber(f.SigmaY) + "\"/>")
			case d2scene.DropShadow:
				effects.WriteString("<feDropShadow" + nativeSVGFloat("dx", f.OffsetX) + nativeSVGFloat("dy", f.OffsetY) + nativeSVGAttr("stdDeviation", nativeSVGNumber(f.SigmaX)+" "+nativeSVGNumber(f.SigmaY)) + nativeSVGAttr("flood-color", nativeSVGColor(f.Color)) + nativeSVGFloat("flood-opacity", float64(f.Color.A)/255) + "/>")
			default:
				w.err = fmt.Errorf("native SVG unsupported filter %T", filter)
			}
		}
		w.def("<filter id=\"" + id + "\" x=\"-100%\" y=\"-100%\" width=\"300%\" height=\"300%\" color-interpolation-filters=\"sRGB\">" + effects.String() + "</filter>")
		attrs += nativeSVGAttr("filter", "url(#"+id+")")
	}
	var body strings.Builder
	w.append(&body, w.primitive(n.Primitive, depth+1))
	for _, child := range n.Children {
		w.append(&body, w.node(child, depth+1))
	}
	return w.keep("<g" + attrs + ">" + body.String() + "</g>")
}

func nativeSVGFillRule(rule d2scene.FillRule) string {
	if rule == d2scene.EvenOdd {
		return "evenodd"
	}
	return "nonzero"
}
func (w *nativeSurfaceSVGWriter) path(path d2scene.Path) string {
	w.commands += len(path.Commands)
	if w.commands > 2000000 {
		w.err = fmt.Errorf("native SVG surface exceeds path command budget")
		return ""
	}
	var s strings.Builder
	p := func(p d2scene.Point) string { return nativeSVGNumber(p.X) + " " + nativeSVGNumber(p.Y) }
	for _, c := range path.Commands {
		switch c.Kind {
		case d2scene.MoveCommand:
			s.WriteString("M" + p(c.P1))
		case d2scene.LineCommand:
			s.WriteString("L" + p(c.P1))
		case d2scene.QuadraticCommand:
			s.WriteString("Q" + p(c.P1) + " " + p(c.P2))
		case d2scene.CubicCommand:
			s.WriteString("C" + p(c.P1) + " " + p(c.P2) + " " + p(c.P3))
		case d2scene.ArcCommand:
			large, sweep := "0", "0"
			if c.LargeArc {
				large = "1"
			}
			if c.Sweep {
				sweep = "1"
			}
			s.WriteString("A" + nativeSVGNumber(c.RadiusX) + " " + nativeSVGNumber(c.RadiusY) + " " + nativeSVGNumber(c.Rotation*180/math.Pi) + " " + large + " " + sweep + " " + p(c.P1))
		case d2scene.CloseCommand:
			s.WriteByte('Z')
		default:
			w.err = fmt.Errorf("native SVG unsupported path command %d", c.Kind)
		}
		if !w.withinBytes(s.Len()) {
			return ""
		}
	}
	return s.String()
}
func (w *nativeSurfaceSVGWriter) paint(paint d2scene.Paint, role string, depth int) string {
	if paint == nil {
		return nativeSVGAttr(role, "none")
	}
	switch p := paint.(type) {
	case *d2scene.SolidPaint:
		if p != nil {
			return w.paint(*p, role, depth)
		}
	case d2scene.SolidPaint:
		return nativeSVGAttr(role, nativeSVGColor(p.Color)) + nativeSVGFloat(role+"-opacity", float64(p.Color.A)/255)
	case *d2scene.LinearGradient:
		if p != nil {
			return w.paint(*p, role, depth)
		}
	case *d2scene.RadialGradient:
		if p != nil {
			return w.paint(*p, role, depth)
		}
	case *d2scene.PatternPaint:
		if p != nil {
			return w.paint(*p, role, depth)
		}
	}
	id := w.id("paint")
	units := func(u d2scene.PaintUnits) string {
		if u == d2scene.UserSpaceOnUse {
			return "userSpaceOnUse"
		}
		return "objectBoundingBox"
	}
	spread := func(s d2scene.SpreadMethod) string {
		if s == d2scene.SpreadReflect {
			return "reflect"
		}
		if s == d2scene.SpreadRepeat {
			return "repeat"
		}
		return "pad"
	}
	stops := func(stops []d2scene.GradientStop) string {
		var s strings.Builder
		for _, stop := range stops {
			s.WriteString("<stop" + nativeSVGFloat("offset", stop.Offset) + nativeSVGAttr("stop-color", nativeSVGColor(stop.Color)) + nativeSVGFloat("stop-opacity", float64(stop.Color.A)/255) + "/>")
		}
		return s.String()
	}
	interpolation := "sRGB"
	if w.linearGradients {
		interpolation = "linearRGB"
	}
	gradientStyle := nativeSVGAttr("color-interpolation", interpolation)
	switch p := paint.(type) {
	case d2scene.LinearGradient:
		w.def("<linearGradient id=\"" + id + "\"" + nativeSVGAttr("gradientUnits", units(p.Units)) + gradientStyle + nativeSVGAttr("gradientTransform", nativeSVGMatrix(p.Transform)) + nativeSVGAttr("spreadMethod", spread(p.Spread)) + nativeSVGFloat("x1", p.Start.X) + nativeSVGFloat("y1", p.Start.Y) + nativeSVGFloat("x2", p.End.X) + nativeSVGFloat("y2", p.End.Y) + ">" + stops(p.Stops) + "</linearGradient>")
	case d2scene.RadialGradient:
		w.def("<radialGradient id=\"" + id + "\"" + nativeSVGAttr("gradientUnits", units(p.Units)) + gradientStyle + nativeSVGAttr("gradientTransform", nativeSVGMatrix(p.Transform)) + nativeSVGAttr("spreadMethod", spread(p.Spread)) + nativeSVGFloat("cx", p.Center.X) + nativeSVGFloat("cy", p.Center.Y) + nativeSVGFloat("r", p.Radius) + nativeSVGFloat("fx", p.Focal.X) + nativeSVGFloat("fy", p.Focal.Y) + nativeSVGFloat("fr", p.FocalRadius) + ">" + stops(p.Stops) + "</radialGradient>")
	case d2scene.PatternPaint:
		body := w.node(p.Root, depth+1)
		w.def("<pattern id=\"" + id + "\"" + nativeSVGAttr("patternUnits", units(p.Units)) + nativeSVGAttr("patternTransform", nativeSVGMatrix(p.Transform)) + nativeSVGFloat("x", p.Tile.X) + nativeSVGFloat("y", p.Tile.Y) + nativeSVGFloat("width", p.Tile.Width) + nativeSVGFloat("height", p.Tile.Height) + nativeSVGAttr("viewBox", "0 0 "+nativeSVGNumber(p.Tile.Width)+" "+nativeSVGNumber(p.Tile.Height)) + " preserveAspectRatio=\"none\">" + body + "</pattern>")
	default:
		w.err = fmt.Errorf("native SVG unsupported paint %T", paint)
	}
	return nativeSVGAttr(role, "url(#"+id+")")
}
func (w *nativeSurfaceSVGWriter) style(fill d2scene.Paint, stroke *d2scene.Stroke, depth int) string {
	s := w.paint(fill, "fill", depth)
	if stroke == nil || stroke.Paint == nil || stroke.Width <= 0 {
		return s + " stroke=\"none\""
	}
	s += w.paint(stroke.Paint, "stroke", depth) + nativeSVGFloat("stroke-width", stroke.Width)
	caps := []string{"butt", "round", "square"}
	joins := []string{"miter", "round", "bevel"}
	if int(stroke.Cap) >= len(caps) || int(stroke.Join) >= len(joins) {
		w.err = fmt.Errorf("native SVG has invalid stroke joins")
		return ""
	}
	s += nativeSVGAttr("stroke-linecap", caps[stroke.Cap]) + nativeSVGAttr("stroke-linejoin", joins[stroke.Join])
	if stroke.MiterLimit > 0 {
		s += nativeSVGFloat("stroke-miterlimit", stroke.MiterLimit)
	}
	if len(stroke.Dashes) > 0 {
		parts := make([]string, len(stroke.Dashes))
		for i, v := range stroke.Dashes {
			parts[i] = nativeSVGNumber(v)
		}
		s += nativeSVGAttr("stroke-dasharray", strings.Join(parts, " ")) + nativeSVGFloat("stroke-dashoffset", stroke.DashOffset)
	}
	return s
}

func (w *nativeSurfaceSVGWriter) primitive(primitive d2scene.Primitive, depth int) string {
	switch p := primitive.(type) {
	case nil:
		return ""
	case *d2scene.Path:
		if p != nil {
			return w.primitive(*p, depth)
		}
	case *d2scene.Rect:
		if p != nil {
			return w.primitive(*p, depth)
		}
	case *d2scene.Ellipse:
		if p != nil {
			return w.primitive(*p, depth)
		}
	case *d2scene.TextRun:
		if p != nil {
			return w.primitive(*p, depth)
		}
	case *d2scene.Image:
		if p != nil {
			return w.primitive(*p, depth)
		}
	case d2scene.Path:
		return "<path" + nativeSVGAttr("d", w.path(p)) + nativeSVGAttr("fill-rule", nativeSVGFillRule(p.FillRule)) + w.style(p.Fill, p.Stroke, depth) + "/>"
	case d2scene.Rect:
		return "<rect" + nativeSVGFloat("x", p.Box.X) + nativeSVGFloat("y", p.Box.Y) + nativeSVGFloat("width", p.Box.Width) + nativeSVGFloat("height", p.Box.Height) + nativeSVGFloat("rx", p.RadiusX) + nativeSVGFloat("ry", p.RadiusY) + w.style(p.Fill, p.Stroke, depth) + "/>"
	case d2scene.Ellipse:
		return "<ellipse" + nativeSVGFloat("cx", p.Center.X) + nativeSVGFloat("cy", p.Center.Y) + nativeSVGFloat("rx", p.RadiusX) + nativeSVGFloat("ry", p.RadiusY) + w.style(p.Fill, p.Stroke, depth) + "/>"
	case d2scene.TextRun:
		return w.text(p, depth)
	case d2scene.Image:
		asset, ok := w.doc.Assets[p.Asset]
		if !ok {
			w.err = fmt.Errorf("native SVG missing image asset %q", p.Asset)
			return ""
		}
		switch a := asset.(type) {
		case d2scene.RasterAsset:
			if len(a.Data) > 32<<20 {
				w.err = fmt.Errorf("native SVG raster asset exceeds byte budget")
				return ""
			}
			source, err := w.rasterURL(p.Asset, a)
			if err != nil {
				w.err = err
				return ""
			}
			aspect := nativeSVGAspect(p.Aspect)
			return "<image" + nativeSVGFloat("x", p.Box.X) + nativeSVGFloat("y", p.Box.Y) + nativeSVGFloat("width", p.Box.Width) + nativeSVGFloat("height", p.Box.Height) + nativeSVGAttr("preserveAspectRatio", aspect) + nativeSVGAttr("href", source) + "/>"
		case d2scene.VectorAsset:
			matrix, err := d2scene.AspectRatioMatrix(a.ViewBox, p.Box, p.Aspect)
			if err != nil {
				w.err = err
				return ""
			}
			id := w.id("image-clip")
			w.def("<clipPath id=\"" + id + "\"><rect" + nativeSVGFloat("x", p.Box.X) + nativeSVGFloat("y", p.Box.Y) + nativeSVGFloat("width", p.Box.Width) + nativeSVGFloat("height", p.Box.Height) + "/></clipPath>")
			previous := w.transform
			w.transform = previous.Mul(matrix)
			body := w.node(a.Root, depth+1)
			w.transform = previous
			return "<g clip-path=\"url(#" + id + ")\"><g transform=\"" + nativeSVGMatrix(matrix) + "\">" + body + "</g></g>"
		default:
			w.err = fmt.Errorf("native SVG unsupported image asset %T", asset)
		}
	}
	if w.err == nil {
		w.err = fmt.Errorf("native SVG unsupported primitive %T", primitive)
	}
	return ""
}

func nativeSVGAspect(a d2scene.AspectRatio) string {
	align := []string{"none", "xMinYMin", "xMidYMin", "xMaxYMin", "xMinYMid", "xMidYMid", "xMaxYMid", "xMinYMax", "xMidYMax", "xMaxYMax"}
	if int(a.Align) >= len(align) {
		return "none"
	}
	if a.Align == d2scene.AlignNone {
		return "none"
	}
	fit := "meet"
	if a.Fit == d2scene.AspectSlice {
		fit = "slice"
	}
	return align[a.Align] + " " + fit
}

// Only authored raster assets are encoded as images. Normalize the same first
// frame and JPEG orientation as PNG output; browsers must not animate an
// embedded GIF or reinterpret the source image's dimensions.
func (w *nativeSurfaceSVGWriter) rasterURL(id d2scene.AssetID, asset d2scene.RasterAsset) (string, error) {
	if value := w.rasterURLs[id]; value != "" {
		return value, nil
	}
	format := strings.TrimPrefix(strings.ToLower(asset.MIMEType), "image/")
	if format == "jpg" {
		format = "jpeg"
	}
	config, _, err := rasterimage.Config(w.ctx, asset.Data, format)
	if err != nil {
		return "", err
	}
	if config.Width <= 0 || config.Height <= 0 || int64(config.Width) > 16000000/int64(config.Height) {
		return "", fmt.Errorf("native SVG raster asset exceeds 16 million pixels")
	}
	decoded, err := rasterimage.DecodeFirst(w.ctx, asset.Data, format)
	if err != nil {
		return "", err
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, decoded); err != nil {
		return "", err
	}
	if err := w.ctx.Err(); err != nil {
		return "", err
	}
	if encoded.Len() > 32<<20 {
		return "", fmt.Errorf("native SVG raster asset exceeds 32 MiB")
	}
	value := "data:image/png;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes())
	w.rasterURLs[id] = value
	return value, nil
}

func nativeSVGBoxAttributes(box d2scene.Box) string {
	return nativeSVGFloat("x", box.X) + nativeSVGFloat("y", box.Y) + nativeSVGFloat("width", box.Width) + nativeSVGFloat("height", box.Height)
}

// Resource allocations only need to cover the retained viewport. A mask with
// an effectively infinite user-space rectangle can ask SVG clients to allocate
// enormous offscreen surfaces even though the face itself is small.
func (w *nativeSurfaceSVGWriter) surfaceBounds() d2scene.Box {
	inverse, err := w.transform.Inverse()
	if err != nil {
		w.err = err
		return d2scene.Box{}
	}
	bounds := (d2scene.Box{Width: w.doc.LogicalWidth, Height: w.doc.LogicalHeight}).Bounds().Transform(inverse)
	return d2scene.Box{X: bounds.Min.X, Y: bounds.Min.Y, Width: bounds.Max.X - bounds.Min.X, Height: bounds.Max.Y - bounds.Min.Y}
}
