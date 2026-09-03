// Package d2scene defines the renderer-neutral scene representation used by
// D2's raster export pipeline.
//
// A Document is owned by its builder and is treated as immutable once it is
// handed to a renderer. Renderers may therefore read the same document
// concurrently at different animation times.
package d2scene

import (
	"errors"
	"math"
)

// Point is a point or vector in logical D2 coordinates.
type Point struct {
	X float64
	Y float64
}

// Box is an axis-aligned rectangle expressed as an origin and size.
// Width and Height may be zero, but must not be negative in a valid scene.
type Box struct {
	X      float64
	Y      float64
	Width  float64
	Height float64
}

// Bounds is an axis-aligned bounding box. Valid distinguishes an empty set
// from a zero-area bound such as a point or a horizontal line.
type Bounds struct {
	Min   Point
	Max   Point
	Valid bool
}

// NewBounds constructs valid bounds and normalizes the two corners.
func NewBounds(x0, y0, x1, y1 float64) Bounds {
	return Bounds{
		Min:   Point{X: math.Min(x0, x1), Y: math.Min(y0, y1)},
		Max:   Point{X: math.Max(x0, x1), Y: math.Max(y0, y1)},
		Valid: true,
	}
}

// BoundsFromPoints returns the smallest bounds containing points.
func BoundsFromPoints(points ...Point) Bounds {
	var b Bounds
	for _, p := range points {
		b = b.Include(p)
	}
	return b
}

// Bounds converts b to min/max form. Negative sizes are normalized here so
// geometry helpers remain total; render preflight rejects them in scenes.
func (b Box) Bounds() Bounds {
	return NewBounds(b.X, b.Y, b.X+b.Width, b.Y+b.Height)
}

func (b Bounds) Width() float64 {
	if !b.Valid {
		return 0
	}
	return b.Max.X - b.Min.X
}

func (b Bounds) Height() float64 {
	if !b.Valid {
		return 0
	}
	return b.Max.Y - b.Min.Y
}

func (b Bounds) Box() Box {
	if !b.Valid {
		return Box{}
	}
	return Box{X: b.Min.X, Y: b.Min.Y, Width: b.Width(), Height: b.Height()}
}

// Include returns bounds containing b and p.
func (b Bounds) Include(p Point) Bounds {
	if !b.Valid {
		return Bounds{Min: p, Max: p, Valid: true}
	}
	b.Min.X = math.Min(b.Min.X, p.X)
	b.Min.Y = math.Min(b.Min.Y, p.Y)
	b.Max.X = math.Max(b.Max.X, p.X)
	b.Max.Y = math.Max(b.Max.Y, p.Y)
	return b
}

// Union returns the smallest bounds containing both operands.
func (b Bounds) Union(other Bounds) Bounds {
	if !b.Valid {
		return other
	}
	if !other.Valid {
		return b
	}
	return NewBounds(
		math.Min(b.Min.X, other.Min.X),
		math.Min(b.Min.Y, other.Min.Y),
		math.Max(b.Max.X, other.Max.X),
		math.Max(b.Max.Y, other.Max.Y),
	)
}

// Intersect returns the common area of two bounds, or invalid bounds when
// they do not overlap.
func (b Bounds) Intersect(other Bounds) Bounds {
	if !b.Valid || !other.Valid {
		return Bounds{}
	}
	minX := math.Max(b.Min.X, other.Min.X)
	minY := math.Max(b.Min.Y, other.Min.Y)
	maxX := math.Min(b.Max.X, other.Max.X)
	maxY := math.Min(b.Max.Y, other.Max.Y)
	if minX > maxX || minY > maxY {
		return Bounds{}
	}
	return NewBounds(minX, minY, maxX, maxY)
}

// Expand grows bounds in both directions. A negative amount shrinks it and
// may produce invalid bounds.
func (b Bounds) Expand(x, y float64) Bounds {
	if !b.Valid {
		return b
	}
	minX, minY := b.Min.X-x, b.Min.Y-y
	maxX, maxY := b.Max.X+x, b.Max.Y+y
	if minX > maxX || minY > maxY {
		return Bounds{}
	}
	return NewBounds(minX, minY, maxX, maxY)
}

// Translate returns bounds shifted by v.
func (b Bounds) Translate(v Point) Bounds {
	if !b.Valid {
		return b
	}
	return Bounds{
		Min:   Point{X: b.Min.X + v.X, Y: b.Min.Y + v.Y},
		Max:   Point{X: b.Max.X + v.X, Y: b.Max.Y + v.Y},
		Valid: true,
	}
}

func (b Bounds) IsFinite() bool {
	return !b.Valid || (finitePoint(b.Min) && finitePoint(b.Max))
}

func finite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

func finitePoint(p Point) bool {
	return finite(p.X) && finite(p.Y)
}

// Matrix is an SVG-style affine transform:
//
//	x' = A*x + C*y + E
//	y' = B*x + D*y + F
//
// The zero value is the zero transform, not identity. Use Identity explicitly.
type Matrix struct {
	A float64
	B float64
	C float64
	D float64
	E float64
	F float64
}

func Identity() Matrix {
	return Matrix{A: 1, D: 1}
}

func Translate(x, y float64) Matrix {
	return Matrix{A: 1, D: 1, E: x, F: y}
}

func Scale(x, y float64) Matrix {
	return Matrix{A: x, D: y}
}

// Rotate returns a counter-clockwise rotation in radians in Cartesian
// coordinates. In D2's screen coordinate system (positive Y down), it appears
// clockwise, matching SVG transform behavior.
func Rotate(radians float64) Matrix {
	s, c := math.Sincos(radians)
	return Matrix{A: c, B: s, C: -s, D: c}
}

func RotateAround(radians, x, y float64) Matrix {
	return Translate(x, y).Mul(Rotate(radians)).Mul(Translate(-x, -y))
}

func SkewX(radians float64) Matrix {
	return Matrix{A: 1, C: math.Tan(radians), D: 1}
}

func SkewY(radians float64) Matrix {
	return Matrix{A: 1, B: math.Tan(radians), D: 1}
}

// Mul composes m and right. The right transform is applied to a point first.
func (m Matrix) Mul(right Matrix) Matrix {
	return Matrix{
		A: m.A*right.A + m.C*right.B,
		B: m.B*right.A + m.D*right.B,
		C: m.A*right.C + m.C*right.D,
		D: m.B*right.C + m.D*right.D,
		E: m.A*right.E + m.C*right.F + m.E,
		F: m.B*right.E + m.D*right.F + m.F,
	}
}

func (m Matrix) Point(p Point) Point {
	return Point{
		X: m.A*p.X + m.C*p.Y + m.E,
		Y: m.B*p.X + m.D*p.Y + m.F,
	}
}

// Vector transforms a vector, excluding translation.
func (m Matrix) Vector(v Point) Point {
	return Point{
		X: m.A*v.X + m.C*v.Y,
		Y: m.B*v.X + m.D*v.Y,
	}
}

func (m Matrix) Determinant() float64 {
	return m.A*m.D - m.B*m.C
}

func (m Matrix) Inverse() (Matrix, error) {
	det := m.Determinant()
	if det == 0 || !finite(det) {
		return Matrix{}, errors.New("d2scene: singular transform")
	}
	return Matrix{
		A: m.D / det,
		B: -m.B / det,
		C: -m.C / det,
		D: m.A / det,
		E: (m.C*m.F - m.D*m.E) / det,
		F: (m.B*m.E - m.A*m.F) / det,
	}, nil
}

func (m Matrix) IsFinite() bool {
	return finite(m.A) && finite(m.B) && finite(m.C) && finite(m.D) && finite(m.E) && finite(m.F)
}

// MaxScale is the largest singular value of the linear portion of m. It is
// used to conservatively transform stroke and filter radii.
func (m Matrix) MaxScale() float64 {
	maximum := math.Max(math.Max(math.Abs(m.A), math.Abs(m.B)), math.Max(math.Abs(m.C), math.Abs(m.D)))
	if maximum == 0 || math.IsInf(maximum, 0) || math.IsNaN(maximum) {
		return maximum
	}
	a, b, c, d := m.A/maximum, m.B/maximum, m.C/maximum, m.D/maximum
	// Eigenvalues of M^T M are (trace +/- sqrt(trace^2-4*det^2))/2.
	trace := a*a + b*b + c*c + d*d
	det := a*d - b*c
	disc := math.Max(0, trace*trace-4*det*det)
	return maximum * math.Sqrt(math.Max(0, (trace+math.Sqrt(disc))/2))
}

// Transform returns the axis-aligned bounds of the transformed corners.
func (b Bounds) Transform(m Matrix) Bounds {
	if !b.Valid {
		return b
	}
	return BoundsFromPoints(
		m.Point(b.Min),
		m.Point(Point{X: b.Max.X, Y: b.Min.Y}),
		m.Point(b.Max),
		m.Point(Point{X: b.Min.X, Y: b.Max.Y}),
	)
}
