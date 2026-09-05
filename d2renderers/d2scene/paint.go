package d2scene

import "image/color"

// Paint is a closed set of renderer-neutral D2 paints. A nil Paint means no
// paint. The closed interface makes unsupported paint kinds fail at compile
// time instead of disappearing in a renderer.
type Paint interface {
	isPaint()
}

type SolidPaint struct {
	Color color.NRGBA
}

func (SolidPaint) isPaint() {}

type GradientStop struct {
	Offset float64
	Color  color.NRGBA
}

type PaintUnits uint8

const (
	ObjectBoundingBox PaintUnits = iota
	UserSpaceOnUse
)

type SpreadMethod uint8

const (
	SpreadPad SpreadMethod = iota
	SpreadReflect
	SpreadRepeat
)

type LinearGradient struct {
	Start     Point
	End       Point
	Stops     []GradientStop
	Units     PaintUnits
	Transform Matrix
	Spread    SpreadMethod
}

func (LinearGradient) isPaint() {}

type RadialGradient struct {
	Center      Point
	Radius      float64
	Focal       Point
	FocalRadius float64
	Stops       []GradientStop
	Units       PaintUnits
	Transform   Matrix
	Spread      SpreadMethod
}

func (RadialGradient) isPaint() {}

// PatternPaint repeats Root over Tile. Root is in Tile's local coordinate
// system and is subject to Transform before the pattern is sampled.
type PatternPaint struct {
	Tile      Box
	Root      *Node
	Units     PaintUnits
	Transform Matrix
}

func (PatternPaint) isPaint() {}

type LineCap uint8

const (
	CapButt LineCap = iota
	CapRound
	CapSquare
)

type LineJoin uint8

const (
	JoinMiter LineJoin = iota
	JoinRound
	JoinBevel
)

// Stroke describes path stroking in logical coordinates. Dashes alternate on
// and off lengths and are interpreted before DashOffset, as in SVG.
type Stroke struct {
	Paint      Paint
	Width      float64
	Cap        LineCap
	Join       LineJoin
	MiterLimit float64
	Dashes     []float64
	DashOffset float64
}

func (s *Stroke) visible() bool {
	return s != nil && s.Paint != nil && s.Width > 0
}

// expansion is a conservative local-coordinate expansion around centerline
// geometry. Exact stroked bounds are the stroker's responsibility. Square caps
// can extend along and across a diagonal tangent; miters use the declared SVG
// miter limit.
func (s *Stroke) expansion() float64 {
	if !s.visible() {
		return 0
	}
	half := s.Width / 2
	factor := 1.0
	if s.Cap == CapSquare {
		factor = 1.4142135623730951 // sqrt(2)
	}
	if s.Join == JoinMiter {
		miterLimit := s.MiterLimit
		if miterLimit <= 0 {
			miterLimit = 4
		}
		if miterLimit > factor {
			factor = miterLimit
		}
	}
	return half * factor
}
