package d2scene

import (
	"fmt"
	"math"
)

type FillRule uint8

const (
	NonZero FillRule = iota
	EvenOdd
)

type PathCommandKind uint8

const (
	MoveCommand PathCommandKind = iota
	LineCommand
	QuadraticCommand
	CubicCommand
	ArcCommand
	CloseCommand
)

// PathCommand stores absolute, typed path geometry. P1 is the endpoint for a
// move or line; P1/P2 are control/end for a quadratic; P1/P2/P3 are the two
// controls/end for a cubic; and P1 is the endpoint for an arc. Arc rotation is
// in radians.
type PathCommand struct {
	Kind PathCommandKind
	P1   Point
	P2   Point
	P3   Point

	RadiusX  float64
	RadiusY  float64
	Rotation float64
	LargeArc bool
	Sweep    bool
}

func MoveTo(x, y float64) PathCommand {
	return PathCommand{Kind: MoveCommand, P1: Point{X: x, Y: y}}
}

func LineTo(x, y float64) PathCommand {
	return PathCommand{Kind: LineCommand, P1: Point{X: x, Y: y}}
}

func QuadraticTo(cx, cy, x, y float64) PathCommand {
	return PathCommand{
		Kind: QuadraticCommand,
		P1:   Point{X: cx, Y: cy},
		P2:   Point{X: x, Y: y},
	}
}

func CubicTo(c1x, c1y, c2x, c2y, x, y float64) PathCommand {
	return PathCommand{
		Kind: CubicCommand,
		P1:   Point{X: c1x, Y: c1y},
		P2:   Point{X: c2x, Y: c2y},
		P3:   Point{X: x, Y: y},
	}
}

func ArcTo(rx, ry, rotation float64, largeArc, sweep bool, x, y float64) PathCommand {
	return PathCommand{
		Kind:     ArcCommand,
		P1:       Point{X: x, Y: y},
		RadiusX:  rx,
		RadiusY:  ry,
		Rotation: rotation,
		LargeArc: largeArc,
		Sweep:    sweep,
	}
}

func ClosePath() PathCommand {
	return PathCommand{Kind: CloseCommand}
}

// Path is both typed path geometry and a paintable scene primitive.
type Path struct {
	Commands []PathCommand
	FillRule FillRule
	Fill     Paint
	Stroke   *Stroke
}

func (Path) isPrimitive() {}

// GeometryBounds computes exact analytic bounds of the centerline geometry.
func (p Path) GeometryBounds() (Bounds, error) {
	return p.transformedGeometryBounds(Identity())
}

func (p Path) transformedGeometryBounds(m Matrix) (Bounds, error) {
	if !m.IsFinite() {
		return Bounds{}, fmt.Errorf("d2scene: non-finite path transform")
	}

	var bounds Bounds
	var current Point
	var subpathStart Point
	haveCurrent := false
	for i, command := range p.Commands {
		if err := command.validate(); err != nil {
			return Bounds{}, fmt.Errorf("d2scene: path command %d: %w", i, err)
		}
		switch command.Kind {
		case MoveCommand:
			current = command.P1
			subpathStart = current
			haveCurrent = true
		case LineCommand:
			if !haveCurrent {
				return Bounds{}, fmt.Errorf("d2scene: path command %d: line before move", i)
			}
			bounds = bounds.Union(BoundsFromPoints(m.Point(current), m.Point(command.P1)))
			current = command.P1
		case QuadraticCommand:
			if !haveCurrent {
				return Bounds{}, fmt.Errorf("d2scene: path command %d: quadratic before move", i)
			}
			bounds = bounds.Union(quadraticBounds(m.Point(current), m.Point(command.P1), m.Point(command.P2)))
			current = command.P2
		case CubicCommand:
			if !haveCurrent {
				return Bounds{}, fmt.Errorf("d2scene: path command %d: cubic before move", i)
			}
			bounds = bounds.Union(cubicBounds(m.Point(current), m.Point(command.P1), m.Point(command.P2), m.Point(command.P3)))
			current = command.P3
		case ArcCommand:
			if !haveCurrent {
				return Bounds{}, fmt.Errorf("d2scene: path command %d: arc before move", i)
			}
			arcCommandBounds, err := arcBounds(current, command, m)
			if err != nil {
				return Bounds{}, fmt.Errorf("d2scene: path command %d: %w", i, err)
			}
			bounds = bounds.Union(arcCommandBounds)
			current = command.P1
		case CloseCommand:
			if !haveCurrent {
				return Bounds{}, fmt.Errorf("d2scene: path command %d: close before move", i)
			}
			bounds = bounds.Union(BoundsFromPoints(m.Point(current), m.Point(subpathStart)))
			current = subpathStart
		default:
			return Bounds{}, fmt.Errorf("d2scene: path command %d: unknown kind %d", i, command.Kind)
		}
	}
	if !bounds.IsFinite() {
		return Bounds{}, fmt.Errorf("d2scene: non-finite path bounds")
	}
	return bounds, nil
}

func (c PathCommand) validate() error {
	checkPoint := func(p Point) error {
		if !finitePoint(p) {
			return fmt.Errorf("non-finite point")
		}
		return nil
	}
	switch c.Kind {
	case MoveCommand, LineCommand:
		return checkPoint(c.P1)
	case QuadraticCommand:
		if err := checkPoint(c.P1); err != nil {
			return err
		}
		return checkPoint(c.P2)
	case CubicCommand:
		if err := checkPoint(c.P1); err != nil {
			return err
		}
		if err := checkPoint(c.P2); err != nil {
			return err
		}
		return checkPoint(c.P3)
	case ArcCommand:
		if err := checkPoint(c.P1); err != nil {
			return err
		}
		if !finite(c.RadiusX) || !finite(c.RadiusY) || !finite(c.Rotation) {
			return fmt.Errorf("non-finite arc")
		}
		return nil
	case CloseCommand:
		return nil
	default:
		return fmt.Errorf("unknown kind %d", c.Kind)
	}
}

func quadraticBounds(p0, p1, p2 Point) Bounds {
	b := BoundsFromPoints(p0, p2)
	if t, ok := quadraticExtremum(p0.X, p1.X, p2.X); ok {
		b = b.Include(quadraticPoint(p0, p1, p2, t))
	}
	if t, ok := quadraticExtremum(p0.Y, p1.Y, p2.Y); ok {
		b = b.Include(quadraticPoint(p0, p1, p2, t))
	}
	return b
}

func quadraticExtremum(p0, p1, p2 float64) (float64, bool) {
	denominator := p0 - 2*p1 + p2
	if denominator == 0 {
		return 0, false
	}
	t := (p0 - p1) / denominator
	return t, t > 0 && t < 1
}

func quadraticPoint(p0, p1, p2 Point, t float64) Point {
	u := 1 - t
	return Point{
		X: u*u*p0.X + 2*u*t*p1.X + t*t*p2.X,
		Y: u*u*p0.Y + 2*u*t*p1.Y + t*t*p2.Y,
	}
}

func cubicBounds(p0, p1, p2, p3 Point) Bounds {
	b := BoundsFromPoints(p0, p3)
	for _, t := range cubicExtrema(p0.X, p1.X, p2.X, p3.X) {
		b = b.Include(cubicPoint(p0, p1, p2, p3, t))
	}
	for _, t := range cubicExtrema(p0.Y, p1.Y, p2.Y, p3.Y) {
		b = b.Include(cubicPoint(p0, p1, p2, p3, t))
	}
	return b
}

func cubicExtrema(p0, p1, p2, p3 float64) []float64 {
	// Derivative divided by three: a*t^2 + b*t + c.
	a := -p0 + 3*p1 - 3*p2 + p3
	b := 2 * (p0 - 2*p1 + p2)
	c := p1 - p0
	return rootsInUnitInterval(a, b, c)
}

func rootsInUnitInterval(a, b, c float64) []float64 {
	if a == 0 {
		if b == 0 {
			return nil
		}
		t := -c / b
		if t > 0 && t < 1 {
			return []float64{t}
		}
		return nil
	}
	discriminant := b*b - 4*a*c
	if discriminant < 0 {
		return nil
	}
	sqrtDiscriminant := math.Sqrt(discriminant)
	t0 := (-b - sqrtDiscriminant) / (2 * a)
	t1 := (-b + sqrtDiscriminant) / (2 * a)
	roots := make([]float64, 0, 2)
	if t0 > 0 && t0 < 1 {
		roots = append(roots, t0)
	}
	if t1 > 0 && t1 < 1 && t1 != t0 {
		roots = append(roots, t1)
	}
	return roots
}

func cubicPoint(p0, p1, p2, p3 Point, t float64) Point {
	u := 1 - t
	uu, tt := u*u, t*t
	return Point{
		X: uu*u*p0.X + 3*uu*t*p1.X + 3*u*tt*p2.X + tt*t*p3.X,
		Y: uu*u*p0.Y + 3*uu*t*p1.Y + 3*u*tt*p2.Y + tt*t*p3.Y,
	}
}

// CenterArc is SVG endpoint arc geometry converted to a center
// parameterization. Its fields remain private so callers cannot construct an
// inconsistent arc; renderers use the accessors below.
type CenterArc struct {
	center Point
	u      Point
	v      Point
	start  float64
	delta  float64
}

func (a CenterArc) StartAngle() float64 { return a.start }
func (a CenterArc) DeltaAngle() float64 { return a.delta }

func (a CenterArc) PointAt(theta float64) Point {
	s, c := math.Sincos(theta)
	return Point{X: a.center.X + a.u.X*c + a.v.X*s, Y: a.center.Y + a.u.Y*c + a.v.Y*s}
}

// TransformedRadiusBound returns a conservative second-derivative bound after
// applying the linear part of m. It is used to prove a device-space chord
// error during adaptive flattening.
func (a CenterArc) TransformedRadiusBound(m Matrix) float64 {
	u := m.Vector(a.u)
	v := m.Vector(a.v)
	return math.Hypot(math.Hypot(u.X, u.Y), math.Hypot(v.X, v.Y))
}

func arcBounds(start Point, command PathCommand, m Matrix) (Bounds, error) {
	end := command.P1
	transformedStart := m.Point(start)
	transformedEnd := m.Point(end)
	if start == end {
		return BoundsFromPoints(transformedStart), nil
	}
	arc, ok, err := EndpointArcToCenter(start, command)
	if err != nil {
		return Bounds{}, err
	}
	if !ok {
		return BoundsFromPoints(transformedStart, transformedEnd), nil
	}

	center := m.Point(arc.center)
	u := m.Vector(arc.u)
	v := m.Vector(arc.v)
	pointAt := func(theta float64) Point {
		s, c := math.Sincos(theta)
		return Point{X: center.X + u.X*c + v.X*s, Y: center.Y + u.Y*c + v.Y*s}
	}
	b := BoundsFromPoints(transformedStart, transformedEnd, pointAt(arc.start), pointAt(arc.start+arc.delta))
	candidates := [4]float64{
		math.Atan2(v.X, u.X),
		math.Atan2(v.X, u.X) + math.Pi,
		math.Atan2(v.Y, u.Y),
		math.Atan2(v.Y, u.Y) + math.Pi,
	}
	for _, theta := range candidates {
		if angleOnArc(theta, arc.start, arc.delta) {
			b = b.Include(pointAt(theta))
		}
	}
	if !b.IsFinite() {
		return Bounds{}, fmt.Errorf("arc bounds are outside the numeric domain")
	}
	return b, nil
}

// EndpointArcToCenter implements SVG 2's endpoint-to-center conversion. The
// boolean is false for the two specified degeneracies: identical endpoints
// (omit the arc) and a zero radius (draw a line). Ratio exponents are
// normalized before division, so finite tiny radii can be corrected without
// overflowing to infinity first.
func EndpointArcToCenter(start Point, command PathCommand) (CenterArc, bool, error) {
	if command.Kind != ArcCommand {
		return CenterArc{}, false, fmt.Errorf("endpoint arc conversion requires an arc command")
	}
	if !finitePoint(start) {
		return CenterArc{}, false, fmt.Errorf("non-finite arc start")
	}
	if err := command.validate(); err != nil {
		return CenterArc{}, false, err
	}
	end := command.P1
	if start == end {
		return CenterArc{}, false, nil
	}
	rx := math.Abs(command.RadiusX)
	ry := math.Abs(command.RadiusY)
	if rx == 0 || ry == 0 {
		return CenterArc{}, false, nil
	}
	sinPhi, cosPhi := math.Sincos(command.Rotation)
	// Halve before subtracting so opposite large finite endpoints do not
	// overflow merely while forming their midpoint-relative vector.
	dx := start.X/2 - end.X/2
	dy := start.Y/2 - end.Y/2
	xPrime := cosPhi*dx + sinPhi*dy
	yPrime := -sinPhi*dx + cosPhi*dy
	if !finite(xPrime) || !finite(yPrime) {
		return CenterArc{}, false, fmt.Errorf("arc is outside the numeric domain")
	}

	normalizedX, normalizedY, ratioMantissa, ratioExponent, err := normalizedRadiusRatios(xPrime, yPrime, rx, ry)
	if err != nil {
		return CenterArc{}, false, err
	}
	ratioGreaterOrEqualOne := ratioExponent > 0 || ratioExponent == 0 && ratioMantissa >= 1
	ratioLength := 1.0
	if ratioGreaterOrEqualOne {
		rx = multiplyBinaryScale(rx, ratioMantissa, ratioExponent)
		ry = multiplyBinaryScale(ry, ratioMantissa, ratioExponent)
		if !finite(rx) || !finite(ry) {
			return CenterArc{}, false, fmt.Errorf("arc corrected radii are outside the numeric domain")
		}
	} else {
		ratioLength = math.Ldexp(ratioMantissa, ratioExponent)
	}
	root := math.Sqrt(math.Max(0, 1-ratioLength*ratioLength))
	if command.LargeArc == command.Sweep {
		root = -root
	}
	cxPrime := (root * rx) * normalizedY
	cyPrime := (-root * ry) * normalizedX

	center := Point{
		X: cosPhi*cxPrime - sinPhi*cyPrime + start.X/2 + end.X/2,
		Y: sinPhi*cxPrime + cosPhi*cyPrime + start.Y/2 + end.Y/2,
	}
	u := Point{X: rx * cosPhi, Y: rx * sinPhi}
	v := Point{X: -ry * sinPhi, Y: ry * cosPhi}
	if !finitePoint(center) || !finitePoint(u) || !finitePoint(v) {
		return CenterArc{}, false, fmt.Errorf("arc is outside the numeric domain")
	}

	endpointX := normalizedX
	endpointY := normalizedY
	if !ratioGreaterOrEqualOne {
		endpointX *= ratioLength
		endpointY *= ratioLength
	}
	cxRatio := cxPrime / rx
	cyRatio := cyPrime / ry
	ux := endpointX - cxRatio
	uy := endpointY - cyRatio
	vx := -endpointX - cxRatio
	vy := -endpointY - cyRatio
	startAngle := math.Atan2(uy, ux)
	delta := math.Atan2(ux*vy-uy*vx, ux*vx+uy*vy)
	if command.Sweep && delta < 0 {
		delta += 2 * math.Pi
	} else if !command.Sweep && delta > 0 {
		delta -= 2 * math.Pi
	}
	if !finite(startAngle) || !finite(delta) {
		return CenterArc{}, false, fmt.Errorf("arc angles are outside the numeric domain")
	}
	return CenterArc{
		center: center,
		u:      Point{X: rx * cosPhi, Y: rx * sinPhi},
		v:      Point{X: -ry * sinPhi, Y: ry * cosPhi},
		start:  startAngle,
		delta:  delta,
	}, true, nil
}

// normalizedRadiusRatios returns the unit vector of
// (xPrime/rx,yPrime/ry), plus a binary representation of its length as
// mantissa*2^exponent. No component ratio is formed directly.
func normalizedRadiusRatios(xPrime, yPrime, rx, ry float64) (unitX, unitY, mantissa float64, exponent int, err error) {
	xMantissa, xExponent, hasX := ratioMantissaExponent(xPrime, rx)
	yMantissa, yExponent, hasY := ratioMantissaExponent(yPrime, ry)
	if !hasX && !hasY {
		return 0, 0, 0, 0, fmt.Errorf("arc endpoint ratio is zero")
	}
	maxExponent := xExponent
	if !hasX || hasY && yExponent > maxExponent {
		maxExponent = yExponent
	}
	xScaled, yScaled := 0.0, 0.0
	if hasX {
		xScaled = math.Ldexp(xMantissa, xExponent-maxExponent)
	}
	if hasY {
		yScaled = math.Ldexp(yMantissa, yExponent-maxExponent)
	}
	norm := math.Hypot(xScaled, yScaled)
	if norm == 0 || !finite(norm) {
		return 0, 0, 0, 0, fmt.Errorf("arc endpoint ratio is outside the numeric domain")
	}
	lengthMantissa, lengthExponent := math.Frexp(norm)
	return xScaled / norm, yScaled / norm, lengthMantissa, maxExponent + lengthExponent, nil
}

func ratioMantissaExponent(numerator, denominator float64) (float64, int, bool) {
	if numerator == 0 {
		return 0, 0, false
	}
	numeratorMantissa, numeratorExponent := math.Frexp(numerator)
	denominatorMantissa, denominatorExponent := math.Frexp(denominator)
	ratioMantissa, ratioExponent := math.Frexp(numeratorMantissa / denominatorMantissa)
	return ratioMantissa, numeratorExponent - denominatorExponent + ratioExponent, true
}

func multiplyBinaryScale(value, mantissa float64, exponent int) float64 {
	valueMantissa, valueExponent := math.Frexp(value)
	return math.Ldexp(valueMantissa*mantissa, valueExponent+exponent)
}

func angleOnArc(theta, start, delta float64) bool {
	const tolerance = 1e-12
	if delta >= 0 {
		return positiveAngle(theta-start) <= delta+tolerance
	}
	return positiveAngle(start-theta) <= -delta+tolerance
}

func positiveAngle(theta float64) float64 {
	theta = math.Mod(theta, 2*math.Pi)
	if theta < 0 {
		theta += 2 * math.Pi
	}
	return theta
}
