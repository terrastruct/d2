package d2scene

import (
	"fmt"
	"math"
)

// Primitive is the closed set of paintable scene leaves. Groups are Nodes
// with a nil Primitive and one or more Children.
type Primitive interface {
	isPrimitive()
}

type Rect struct {
	Box     Box
	RadiusX float64
	RadiusY float64
	Fill    Paint
	Stroke  *Stroke
}

func (Rect) isPrimitive() {}

type Ellipse struct {
	Center  Point
	RadiusX float64
	RadiusY float64
	Fill    Paint
	Stroke  *Stroke
}

func (Ellipse) isPrimitive() {}

type Font struct {
	Family string
	Style  string
	Weight int
	Size   float64
	Asset  AssetID
}

type TextAnchor uint8

const (
	AnchorStart TextAnchor = iota
	AnchorMiddle
	AnchorEnd
)

// Glyph optionally carries a shaper's exact glyph placement. Ink is relative
// to Origin+Position. A renderer may shape Text itself when Glyphs is empty.
type Glyph struct {
	ID uint32
	// Empty retains an invisible shaper output (for example a default-
	// ignorable control) so its placement and advance survive in the scene.
	// Empty glyphs use ID zero and are never sent to the outline rasterizer.
	Empty bool
	// Asset overrides TextRun.Font.Asset for this glyph. An empty value keeps
	// the primary asset. This makes explicit shaping capable of retaining
	// mixed-font fallback decisions in the scene itself.
	Asset    AssetID
	Position Point
	Advance  float64
	Ink      Bounds
}

// TextRun is one consistently styled baseline run. Ink is the exact measured
// node-local ink bounds when available. Underline and Strike are explicit so
// link and label decoration does not require renderer-specific child nodes.
type TextRun struct {
	Text   string
	Origin Point
	Anchor TextAnchor
	Font   Font
	// Fallbacks is an ordered list of already-resolved font assets. Renderers
	// may use them for missing glyphs but must never perform font discovery or
	// filesystem I/O while painting a document. It lives on TextRun rather than
	// Font so Font remains comparable and usable as a map key.
	Fallbacks []AssetID
	Fill      Paint
	Stroke    *Stroke
	Underline bool
	Strike    bool
	Glyphs    []Glyph
	Ink       Bounds
}

func (TextRun) isPrimitive() {}

type AspectAlign uint8

const (
	AlignNone AspectAlign = iota
	AlignXMinYMin
	AlignXMidYMin
	AlignXMaxYMin
	AlignXMinYMid
	AlignXMidYMid
	AlignXMaxYMid
	AlignXMinYMax
	AlignXMidYMax
	AlignXMaxYMax
)

type AspectFit uint8

const (
	AspectMeet AspectFit = iota
	AspectSlice
)

type AspectRatio struct {
	Align AspectAlign
	Fit   AspectFit
}

type Image struct {
	Asset  AssetID
	Box    Box
	Aspect AspectRatio
}

func (Image) isPrimitive() {}

// PrimitiveBounds returns conservative painted bounds after applying m. Path
// and ellipse geometry extrema are analytic; stroke expansion is deliberately
// conservative until the stroker computes its exact outline.
func PrimitiveBounds(primitive Primitive, m Matrix) (Bounds, error) {
	if primitive == nil {
		return Bounds{}, nil
	}
	if !m.IsFinite() {
		return Bounds{}, fmt.Errorf("d2scene: non-finite primitive transform")
	}
	switch primitive := primitive.(type) {
	case Rect:
		return rectPrimitiveBounds(primitive, m)
	case *Rect:
		if primitive == nil {
			return Bounds{}, nil
		}
		return rectPrimitiveBounds(*primitive, m)
	case Ellipse:
		return ellipsePrimitiveBounds(primitive, m)
	case *Ellipse:
		if primitive == nil {
			return Bounds{}, nil
		}
		return ellipsePrimitiveBounds(*primitive, m)
	case Path:
		return pathPrimitiveBounds(primitive, m)
	case *Path:
		if primitive == nil {
			return Bounds{}, nil
		}
		return pathPrimitiveBounds(*primitive, m)
	case TextRun:
		return textPrimitiveBounds(primitive, m)
	case *TextRun:
		if primitive == nil {
			return Bounds{}, nil
		}
		return textPrimitiveBounds(*primitive, m)
	case Image:
		return imagePrimitiveBounds(primitive, m)
	case *Image:
		if primitive == nil {
			return Bounds{}, nil
		}
		return imagePrimitiveBounds(*primitive, m)
	default:
		return Bounds{}, fmt.Errorf("d2scene: unsupported primitive %T", primitive)
	}
}

func rectPrimitiveBounds(rect Rect, m Matrix) (Bounds, error) {
	if err := validateBox(rect.Box); err != nil {
		return Bounds{}, fmt.Errorf("d2scene: rectangle: %w", err)
	}
	if rect.RadiusX < 0 || rect.RadiusY < 0 || !finite(rect.RadiusX) || !finite(rect.RadiusY) {
		return Bounds{}, fmt.Errorf("d2scene: rectangle: invalid corner radius")
	}
	if rect.Fill == nil && !rect.Stroke.visible() {
		return Bounds{}, nil
	}
	return expandStroke(rect.Box.Bounds().Transform(m), rect.Stroke, m), nil
}

func ellipsePrimitiveBounds(ellipse Ellipse, m Matrix) (Bounds, error) {
	if !finitePoint(ellipse.Center) || !finite(ellipse.RadiusX) || !finite(ellipse.RadiusY) || ellipse.RadiusX < 0 || ellipse.RadiusY < 0 {
		return Bounds{}, fmt.Errorf("d2scene: ellipse: invalid geometry")
	}
	if ellipse.Fill == nil && !ellipse.Stroke.visible() {
		return Bounds{}, nil
	}
	center := m.Point(ellipse.Center)
	xRadius := hypot(m.A*ellipse.RadiusX, m.C*ellipse.RadiusY)
	yRadius := hypot(m.B*ellipse.RadiusX, m.D*ellipse.RadiusY)
	bounds := NewBounds(center.X-xRadius, center.Y-yRadius, center.X+xRadius, center.Y+yRadius)
	return expandStroke(bounds, ellipse.Stroke, m), nil
}

func pathPrimitiveBounds(path Path, m Matrix) (Bounds, error) {
	if path.Fill == nil && !path.Stroke.visible() {
		return Bounds{}, nil
	}
	bounds, err := path.transformedGeometryBounds(m)
	if err != nil {
		return Bounds{}, err
	}
	return expandStroke(bounds, path.Stroke, m), nil
}

func textPrimitiveBounds(text TextRun, m Matrix) (Bounds, error) {
	if text.Fill == nil && !text.Stroke.visible() {
		return Bounds{}, nil
	}
	if text.Font.Size <= 0 || !finite(text.Font.Size) || !finitePoint(text.Origin) {
		return Bounds{}, fmt.Errorf("d2scene: text: invalid font size or origin")
	}
	bounds := text.Ink
	if !bounds.Valid {
		for _, glyph := range text.Glyphs {
			if !finitePoint(glyph.Position) || !finite(glyph.Advance) || !glyph.Ink.IsFinite() {
				return Bounds{}, fmt.Errorf("d2scene: text: invalid glyph geometry")
			}
			bounds = bounds.Union(glyph.Ink.Translate(Point{
				X: text.Origin.X + glyph.Position.X,
				Y: text.Origin.Y + glyph.Position.Y,
			}))
		}
	}
	if !bounds.IsFinite() {
		return Bounds{}, fmt.Errorf("d2scene: text: invalid ink bounds")
	}
	if !bounds.Valid {
		return Bounds{}, fmt.Errorf("d2scene: text: missing ink or glyph bounds")
	}
	return expandStroke(bounds.Transform(m), text.Stroke, m), nil
}

func imagePrimitiveBounds(image Image, m Matrix) (Bounds, error) {
	if image.Asset == "" {
		return Bounds{}, fmt.Errorf("d2scene: image: empty asset ID")
	}
	if err := validateBox(image.Box); err != nil {
		return Bounds{}, fmt.Errorf("d2scene: image: %w", err)
	}
	return image.Box.Bounds().Transform(m), nil
}

func expandStroke(bounds Bounds, stroke *Stroke, m Matrix) Bounds {
	if !bounds.Valid || !stroke.visible() {
		return bounds
	}
	expansion := stroke.expansion() * m.MaxScale()
	return bounds.Expand(expansion, expansion)
}

func validateBox(box Box) error {
	if !finite(box.X) || !finite(box.Y) || !finite(box.Width) || !finite(box.Height) {
		return fmt.Errorf("non-finite box")
	}
	if box.Width < 0 || box.Height < 0 {
		return fmt.Errorf("negative box size")
	}
	return nil
}

func hypot(x, y float64) float64 {
	// Kept behind one helper so every analytic bounds path uses the same
	// deterministic standard-library operation.
	return math.Hypot(x, y)
}
