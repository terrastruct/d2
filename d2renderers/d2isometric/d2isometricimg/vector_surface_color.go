package d2isometricimg

import (
	"fmt"
	"strings"

	"golang.org/x/image/math/fixed"

	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/internal/fontface"
)

func (w *nativeSurfaceSVGWriter) colorGlyph(face *fontface.ParsedFace, plan *fontface.COLRv1Plan, origin d2scene.Point, size float64, depth int) string {
	if plan == nil || plan.Root == nil || plan.Clip == nil {
		w.err = fmt.Errorf("native SVG color glyph has no bounded paint plan")
		return ""
	}
	scale := size / float64(face.Outline.UnitsPerEm())
	transform := d2scene.Translate(origin.X, origin.Y).Mul(d2scene.Scale(scale, -scale))
	box := d2scene.Box{X: plan.Clip.XMin, Y: plan.Clip.YMin, Width: plan.Clip.XMax - plan.Clip.XMin, Height: plan.Clip.YMax - plan.Clip.YMin}
	id := w.id("color-clip")
	w.def("<clipPath id=\"" + id + "\"><rect" + nativeSVGFloat("x", box.X) + nativeSVGFloat("y", box.Y) + nativeSVGFloat("width", box.Width) + nativeSVGFloat("height", box.Height) + "/></clipPath>")
	return "<g transform=\"" + nativeSVGMatrix(transform) + "\"><g clip-path=\"url(#" + id + ")\">" + w.colorPaint(face, plan.Root, box, d2scene.Identity(), depth+1) + "</g></g>"
}

func (w *nativeSurfaceSVGWriter) colorPaint(face *fontface.ParsedFace, paint fontface.COLRv1Paint, box d2scene.Box, current d2scene.Matrix, depth int) string {
	if !w.admit(depth) {
		return ""
	}
	stops := func(line fontface.COLRv1ColorLine) []d2scene.GradientStop {
		result := make([]d2scene.GradientStop, len(line.Stops))
		for i, s := range line.Stops {
			result[i] = d2scene.GradientStop{Offset: s.Offset, Color: s.Color}
		}
		return result
	}
	var fill d2scene.Paint
	switch p := paint.(type) {
	case fontface.COLRv1Layers:
		var s strings.Builder
		for _, child := range p.Paints {
			w.append(&s, w.colorPaint(face, child, box, current, depth+1))
		}
		return s.String()
	case fontface.COLRv1Transform:
		m := d2scene.Matrix{A: p.Matrix.Xx, B: p.Matrix.Yx, C: p.Matrix.Xy, D: p.Matrix.Yy, E: p.Matrix.Dx, F: p.Matrix.Dy}
		return "<g transform=\"" + nativeSVGMatrix(m) + "\">" + w.colorPaint(face, p.Paint, box, current.Mul(m), depth+1) + "</g>"
	case fontface.COLRv1Glyph:
		name := w.glyphDefinition(face, p.GlyphID, fixed.I(int(face.Outline.UnitsPerEm())))
		if w.err != nil {
			return ""
		}
		id := w.id("glyph-clip")
		w.def("<clipPath id=\"" + id + "\"><use transform=\"scale(1 -1)\" href=\"#" + name + "\"/></clipPath>")
		return "<g clip-path=\"url(#" + id + ")\">" + w.colorPaint(face, p.Paint, box, current, depth+1) + "</g>"
	case fontface.COLRv1Composite:
		backdrop := w.colorPaint(face, p.Backdrop, box, current, depth+1)
		source := w.colorPaint(face, p.Source, box, current, depth+1)
		switch p.Mode {
		case fontface.COLRv1CompositeSrcIn:
			id := w.id("color-mask")
			inverse, err := current.Inverse()
			if err != nil {
				w.err = err
				return ""
			}
			bounds := box.Bounds().Transform(inverse)
			maskBox := d2scene.Box{X: bounds.Min.X, Y: bounds.Min.Y, Width: bounds.Max.X - bounds.Min.X, Height: bounds.Max.Y - bounds.Min.Y}
			w.def("<mask id=\"" + id + "\" maskUnits=\"userSpaceOnUse\"" + nativeSVGBoxAttributes(maskBox) + " mask-type=\"alpha\">" + backdrop + "</mask>")
			return "<g mask=\"url(#" + id + ")\">" + source + "</g>"
		case fontface.COLRv1CompositeSoftLight:
			return "<g style=\"isolation:isolate\">" + backdrop + "<g style=\"mix-blend-mode:soft-light\">" + source + "</g></g>"
		default:
			w.err = fmt.Errorf("native SVG unsupported color composite %d", p.Mode)
			return ""
		}
	case fontface.COLRv1Solid:
		fill = d2scene.SolidPaint{Color: p.Color}
	case fontface.COLRv1LinearGradient:
		vx, vy := p.X1-p.X0, p.Y1-p.Y0
		rx, ry := p.X2-p.X0, p.Y2-p.Y0
		den := rx*rx + ry*ry
		if den <= 0 {
			w.err = fmt.Errorf("native SVG invalid color gradient")
			return ""
		}
		k := (vx*rx + vy*ry) / den
		fill = d2scene.LinearGradient{Start: d2scene.Point{X: p.X0, Y: p.Y0}, End: d2scene.Point{X: p.X0 + vx - k*rx, Y: p.Y0 + vy - k*ry}, Stops: stops(p.ColorLine), Units: d2scene.UserSpaceOnUse, Transform: d2scene.Identity()}
	case fontface.COLRv1RadialGradient:
		fill = d2scene.RadialGradient{Center: d2scene.Point{X: p.X1, Y: p.Y1}, Radius: p.Radius1, Focal: d2scene.Point{X: p.X0, Y: p.Y0}, FocalRadius: p.Radius0, Stops: stops(p.ColorLine), Units: d2scene.UserSpaceOnUse, Transform: d2scene.Identity()}
	default:
		w.err = fmt.Errorf("native SVG unsupported color paint %T", paint)
		return ""
	}
	inverse, err := current.Inverse()
	if err != nil {
		w.err = err
		return ""
	}
	bounds := box.Bounds().Transform(inverse)
	previous := w.linearGradients
	w.linearGradients = true // OpenType COLR interpolates color in linear light.
	defer func() { w.linearGradients = previous }()
	return w.primitive(d2scene.Rect{Box: d2scene.Box{X: bounds.Min.X, Y: bounds.Min.Y, Width: bounds.Max.X - bounds.Min.X, Height: bounds.Max.Y - bounds.Min.Y}, Fill: fill}, depth+1)
}
