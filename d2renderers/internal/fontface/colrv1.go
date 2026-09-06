package fontface

import (
	"fmt"
	"image/color"
	"math"

	"github.com/go-text/typesetting/font/opentype/tables"
)

const (
	maxTrustedCOLRv1PaintNodes    = 8_192
	maxTrustedCOLRv1Depth         = 32
	maxTrustedCOLRv1Layers        = 256
	maxTrustedCOLRv1GradientStops = 32
)

// trustedCOLRv1Limits are fixed safety ceilings for bundled compilation. Tests
// can lower a ceiling to prove each accounting path rejects before exceeding it.
type trustedCOLRv1Limits struct {
	MaxPaintNodes    int
	MaxDepth         int
	MaxLayers        int
	MaxGradientStops int
}

// bundledNotoColorEmojiCOLRv1Limits leave headroom above the pinned font's
// known maximum graph:
// 6,665 paint nodes, depth 10, and 255 layers in one group.
func bundledNotoColorEmojiCOLRv1Limits() trustedCOLRv1Limits {
	return trustedCOLRv1Limits{
		MaxPaintNodes:    maxTrustedCOLRv1PaintNodes,
		MaxDepth:         maxTrustedCOLRv1Depth,
		MaxLayers:        maxTrustedCOLRv1Layers,
		MaxGradientStops: maxTrustedCOLRv1GradientStops,
	}
}

func (l trustedCOLRv1Limits) validate() error {
	if l.MaxPaintNodes <= 0 || l.MaxDepth <= 0 || l.MaxLayers <= 0 || l.MaxGradientStops <= 0 {
		return fmt.Errorf("d2fonts: every COLRv1 limit must be positive")
	}
	return nil
}

// COLRv1Plan is a renderer-neutral, fully palette-resolved paint graph for one
// glyph. Coordinates remain in the font's design units. Renderers choose their
// own outline rasterizer, transform convention, and temporary-layer strategy.
type COLRv1Plan struct {
	GlyphID uint32
	Root    COLRv1Paint
	Clip    *COLRv1ClipBox
	Usage   COLRv1PlanUsage
}

type COLRv1PlanUsage struct {
	PaintNodes       int
	MaxDepth         int
	MaxLayers        int
	MaxGradientStops int
}

// COLRv1Paint is the closed set of paint operations emitted by the trusted
// compiler.
type COLRv1Paint interface{ isCOLRv1Paint() }

type COLRv1Layers struct{ Paints []COLRv1Paint }
type COLRv1Solid struct{ Color color.NRGBA }
type COLRv1LinearGradient struct {
	ColorLine COLRv1ColorLine
	X0, Y0    float64
	X1, Y1    float64
	X2, Y2    float64
}
type COLRv1RadialGradient struct {
	ColorLine       COLRv1ColorLine
	X0, Y0, Radius0 float64
	X1, Y1, Radius1 float64
}
type COLRv1Glyph struct {
	GlyphID uint32
	Paint   COLRv1Paint
}
type COLRv1Transform struct {
	Matrix COLRv1Affine
	Paint  COLRv1Paint
}
type COLRv1Composite struct {
	Source, Backdrop COLRv1Paint
	Mode             COLRv1CompositeMode
}

func (COLRv1Layers) isCOLRv1Paint()         {}
func (COLRv1Solid) isCOLRv1Paint()          {}
func (COLRv1LinearGradient) isCOLRv1Paint() {}
func (COLRv1RadialGradient) isCOLRv1Paint() {}
func (COLRv1Glyph) isCOLRv1Paint()          {}
func (COLRv1Transform) isCOLRv1Paint()      {}
func (COLRv1Composite) isCOLRv1Paint()      {}

type COLRv1Affine struct{ Xx, Yx, Xy, Yy, Dx, Dy float64 }

type COLRv1ClipBox struct{ XMin, YMin, XMax, YMax float64 }

type COLRv1ColorStop struct {
	Offset float64
	Color  color.NRGBA
}

type COLRv1ColorLine struct {
	Stops []COLRv1ColorStop
}

type COLRv1CompositeMode uint8

const (
	COLRv1CompositeSrcIn COLRv1CompositeMode = iota
	COLRv1CompositeSoftLight
)

// COLRv1UntrustedFontError says that a caller attempted to use the compiler
// for arbitrary external font data.
type COLRv1UntrustedFontError struct{}

func (COLRv1UntrustedFontError) Error() string {
	return "d2fonts: COLRv1 font is not the parser-authenticated bundled resource"
}

type COLRv1LimitError struct {
	Limit          string
	Value, Maximum int
}

func (e *COLRv1LimitError) Error() string {
	return fmt.Sprintf("d2fonts: COLRv1 %s %d exceeds limit %d", e.Limit, e.Value, e.Maximum)
}

type COLRv1UnsupportedPaintError struct{ Type string }

func (e *COLRv1UnsupportedPaintError) Error() string {
	return fmt.Sprintf("d2fonts: unsupported COLRv1 paint %s", e.Type)
}

type COLRv1UnsupportedCompositeError struct{ Mode uint8 }

func (e *COLRv1UnsupportedCompositeError) Error() string {
	return fmt.Sprintf("d2fonts: unsupported COLRv1 composite mode %d", e.Mode)
}

type COLRv1UnsupportedExtendError struct{ Extend uint8 }

func (e *COLRv1UnsupportedExtendError) Error() string {
	return fmt.Sprintf("d2fonts: unsupported COLRv1 gradient extend %d", e.Extend)
}

// CompileBundledNotoColorEmojiCOLRv1Plan compiles the static COLRv1 subset used
// by D2's bundled Noto Color Emoji asset. A false result means the glyph has no
// COLR paint. Parser-issued provenance is checked before inspecting the color
// table; arbitrary external COLRv1 fonts remain unsupported and callers cannot
// replace the safety ceilings.
func (f *ParsedFace) CompileBundledNotoColorEmojiCOLRv1Plan(glyphID uint32) (*COLRv1Plan, bool, error) {
	return f.compileBundledNotoColorEmojiCOLRv1Plan(glyphID, bundledNotoColorEmojiCOLRv1Limits())
}

func (f *ParsedFace) compileBundledNotoColorEmojiCOLRv1Plan(glyphID uint32, limits trustedCOLRv1Limits) (plan *COLRv1Plan, found bool, err error) {
	if err := limits.validate(); err != nil {
		return nil, false, err
	}
	if f == nil || f.Outline == nil || f.Shaping == nil {
		return nil, false, fmt.Errorf("d2fonts: nil parsed font face")
	}
	if !f.hasParsedSource(bundledNotoColorEmojiCOLRv1SHA256) {
		return nil, false, COLRv1UntrustedFontError{}
	}
	return compilePrivateBundledNotoColorEmojiCOLRv1Plan(glyphID, limits)
}

// compileCOLRv1Plan compiles only from the package-private bundled-font parse.
// The caller's ParsedFace authenticates how it obtained the face, never as the
// source of COLR or CPAL tables.
func compileCOLRv1Plan(f *ParsedFace, glyphID uint32, limits trustedCOLRv1Limits) (plan *COLRv1Plan, found bool, err error) {
	if f == nil || f.Outline == nil || f.Shaping == nil {
		return nil, false, fmt.Errorf("d2fonts: nil private bundled font face")
	}
	// The parser dependency is not part of D2's trusted API surface. Contain any
	// unexpected table-resolution panic so a dependency regression fails the
	// export instead of terminating the process.
	defer func() {
		if recover() != nil {
			plan = nil
			found = false
			err = fmt.Errorf("d2fonts: COLRv1 glyph %d table resolution panicked", glyphID)
		}
	}()
	if glyphID == 0 || glyphID > math.MaxUint16 || int(glyphID) >= f.Outline.NumGlyphs() {
		return nil, false, fmt.Errorf("d2fonts: COLRv1 glyph ID %d is out of range", glyphID)
	}
	paint, found, err := searchCOLRPaint(f.Shaping.COLR, tables.GlyphID(glyphID))
	if err != nil {
		return nil, false, fmt.Errorf("d2fonts: COLRv1 glyph %d lookup: %w", glyphID, err)
	}
	if !found {
		return nil, false, nil
	}
	b := colrv1Builder{face: f, limits: limits}
	root, err := b.paint(paint, 1)
	if err != nil {
		return nil, false, fmt.Errorf("d2fonts: COLRv1 glyph %d: %w", glyphID, err)
	}
	clip, err := b.clip(tables.GlyphID(glyphID))
	if err != nil {
		return nil, false, fmt.Errorf("d2fonts: COLRv1 glyph %d: %w", glyphID, err)
	}
	return &COLRv1Plan{GlyphID: glyphID, Root: root, Clip: clip, Usage: b.usage}, true, nil
}

type colrv1Builder struct {
	face   *ParsedFace
	limits trustedCOLRv1Limits
	usage  COLRv1PlanUsage
}

func (b *colrv1Builder) paint(paint tables.PaintTable, depth int) (COLRv1Paint, error) {
	if depth > b.limits.MaxDepth {
		return nil, &COLRv1LimitError{"paint depth", depth, b.limits.MaxDepth}
	}
	b.usage.PaintNodes++
	if b.usage.PaintNodes > b.limits.MaxPaintNodes {
		return nil, &COLRv1LimitError{"paint node count", b.usage.PaintNodes, b.limits.MaxPaintNodes}
	}
	b.usage.MaxDepth = max(b.usage.MaxDepth, depth)
	if paint == nil {
		return nil, fmt.Errorf("d2fonts: COLRv1 nil paint")
	}
	switch p := paint.(type) {
	case tables.PaintColrLayers:
		if int(p.NumLayers) > b.limits.MaxLayers {
			return nil, &COLRv1LimitError{"layer count", int(p.NumLayers), b.limits.MaxLayers}
		}
		layers, err := b.face.Shaping.COLR.LayerList.Resolve(p)
		if err != nil {
			return nil, fmt.Errorf("d2fonts: COLRv1 layer range: %w", err)
		}
		b.usage.MaxLayers = max(b.usage.MaxLayers, len(layers))
		out := COLRv1Layers{Paints: make([]COLRv1Paint, 0, len(layers))}
		for _, layer := range layers {
			child, err := b.paint(layer, depth+1)
			if err != nil {
				return nil, err
			}
			out.Paints = append(out.Paints, child)
		}
		return out, nil
	case tables.PaintSolid:
		c, err := b.color(p.PaletteIndex, p.Alpha)
		if err != nil {
			return nil, err
		}
		return COLRv1Solid{Color: c}, nil
	case tables.PaintLinearGradient:
		line, err := b.colorLine(p.ColorLine)
		if err != nil {
			return nil, err
		}
		return COLRv1LinearGradient{line, float64(p.X0), float64(p.Y0), float64(p.X1), float64(p.Y1), float64(p.X2), float64(p.Y2)}, nil
	case tables.PaintRadialGradient:
		line, err := b.colorLine(p.ColorLine)
		if err != nil {
			return nil, err
		}
		return COLRv1RadialGradient{line, float64(p.X0), float64(p.Y0), float64(p.Radius0), float64(p.X1), float64(p.Y1), float64(p.Radius1)}, nil
	case tables.PaintGlyph:
		if int(p.GlyphID) >= b.face.Outline.NumGlyphs() {
			return nil, fmt.Errorf("d2fonts: COLRv1 paint glyph ID %d is out of range", p.GlyphID)
		}
		child, err := b.paint(p.Paint, depth+1)
		if err != nil {
			return nil, err
		}
		return COLRv1Glyph{GlyphID: uint32(p.GlyphID), Paint: child}, nil
	case tables.PaintTransform:
		child, err := b.paint(p.Paint, depth+1)
		if err != nil {
			return nil, err
		}
		m := p.Transform
		return COLRv1Transform{Matrix: COLRv1Affine{float64(m.Xx), float64(m.Yx), float64(m.Xy), float64(m.Yy), float64(m.Dx), float64(m.Dy)}, Paint: child}, nil
	case tables.PaintTranslate:
		child, err := b.paint(p.Paint, depth+1)
		if err != nil {
			return nil, err
		}
		return COLRv1Transform{Matrix: COLRv1Affine{Xx: 1, Yy: 1, Dx: float64(p.Dx), Dy: float64(p.Dy)}, Paint: child}, nil
	case tables.PaintScale:
		child, err := b.paint(p.Paint, depth+1)
		if err != nil {
			return nil, err
		}
		return COLRv1Transform{Matrix: COLRv1Affine{Xx: fixed214(p.ScaleX), Yy: fixed214(p.ScaleY)}, Paint: child}, nil
	case tables.PaintScaleAroundCenter:
		child, err := b.paint(p.Paint, depth+1)
		if err != nil {
			return nil, err
		}
		sx, sy := fixed214(p.ScaleX), fixed214(p.ScaleY)
		return COLRv1Transform{Matrix: COLRv1Affine{Xx: sx, Yy: sy, Dx: float64(p.CenterX) * (1 - sx), Dy: float64(p.CenterY) * (1 - sy)}, Paint: child}, nil
	case tables.PaintComposite:
		mode, err := colrv1CompositeMode(p.CompositeMode)
		if err != nil {
			return nil, err
		}
		backdrop, err := b.paint(p.BackdropPaint, depth+1)
		if err != nil {
			return nil, err
		}
		source, err := b.paint(p.SourcePaint, depth+1)
		if err != nil {
			return nil, err
		}
		return COLRv1Composite{Source: source, Backdrop: backdrop, Mode: mode}, nil
	default:
		return nil, &COLRv1UnsupportedPaintError{Type: fmt.Sprintf("%T", paint)}
	}
}

func (b *colrv1Builder) colorLine(line tables.ColorLine) (COLRv1ColorLine, error) {
	if len(line.ColorStops) == 0 {
		return COLRv1ColorLine{}, fmt.Errorf("d2fonts: COLRv1 gradient has no stops")
	}
	// The pinned font currently uses only ExtendPad. Repeat and Reflect are
	// defined over the interval from the first to the last color stop, which is
	// not interchangeable with SVG's fixed [0,1] spread interval.
	if line.Extend != tables.ExtendPad {
		return COLRv1ColorLine{}, &COLRv1UnsupportedExtendError{Extend: uint8(line.Extend)}
	}
	if len(line.ColorStops) > b.limits.MaxGradientStops {
		return COLRv1ColorLine{}, &COLRv1LimitError{"gradient stop count", len(line.ColorStops), b.limits.MaxGradientStops}
	}
	b.usage.MaxGradientStops = max(b.usage.MaxGradientStops, len(line.ColorStops))
	out := COLRv1ColorLine{Stops: make([]COLRv1ColorStop, 0, len(line.ColorStops))}
	for _, stop := range line.ColorStops {
		c, err := b.color(stop.PaletteIndex, stop.Alpha)
		if err != nil {
			return COLRv1ColorLine{}, err
		}
		out.Stops = append(out.Stops, COLRv1ColorStop{Offset: fixed214(stop.StopOffset), Color: c})
	}
	return out, nil
}

func (b *colrv1Builder) color(index uint16, alpha tables.Fixed214) (color.NRGBA, error) {
	if len(b.face.Shaping.CPAL) == 0 {
		return color.NRGBA{}, fmt.Errorf("d2fonts: COLRv1 has no CPAL palette")
	}
	palette := b.face.Shaping.CPAL[0]
	if int(index) >= len(palette) {
		return color.NRGBA{}, fmt.Errorf("d2fonts: COLRv1 palette index %d is out of range", index)
	}
	a := fixed214(alpha)
	if a < 0 || a > 1 {
		return color.NRGBA{}, fmt.Errorf("d2fonts: COLRv1 alpha %g is outside [0,1]", a)
	}
	entry := palette[index]
	return color.NRGBA{R: entry.Red, G: entry.Green, B: entry.Blue, A: uint8(math.Round(float64(entry.Alpha) * a))}, nil
}

func (b *colrv1Builder) clip(glyph tables.GlyphID) (*COLRv1ClipBox, error) {
	clip, found := b.face.Shaping.COLR.ClipList.Search(glyph)
	if !found {
		return nil, nil
	}
	box, ok := clip.(tables.ClipBoxFormat1)
	if !ok {
		return nil, &COLRv1UnsupportedPaintError{Type: fmt.Sprintf("%T clip box", clip)}
	}
	if box.XMin > box.XMax || box.YMin > box.YMax {
		return nil, fmt.Errorf("d2fonts: COLRv1 invalid clip box")
	}
	return &COLRv1ClipBox{float64(box.XMin), float64(box.YMin), float64(box.XMax), float64(box.YMax)}, nil
}

func fixed214(value tables.Fixed214) float64 { return float64(value) / (1 << 14) }

func colrv1CompositeMode(mode tables.CompositeMode) (COLRv1CompositeMode, error) {
	switch mode {
	case tables.CompositeSrcIn:
		return COLRv1CompositeSrcIn, nil
	case tables.CompositeSoftLight:
		return COLRv1CompositeSoftLight, nil
	default:
		return 0, &COLRv1UnsupportedCompositeError{Mode: uint8(mode)}
	}
}
