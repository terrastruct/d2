package d2scene

import (
	"image/color"
	"math"
	"math/rand"
	"testing"
)

func TestQuadraticGeometryBoundsUsesDerivativeExtrema(t *testing.T) {
	path := Path{Commands: []PathCommand{
		MoveTo(0, 0),
		QuadraticTo(10, 20, 20, 0),
	}}
	bounds, err := path.GeometryBounds()
	if err != nil {
		t.Fatal(err)
	}
	assertBounds(t, bounds, 0, 0, 20, 10)
}

func TestCubicGeometryBoundsUsesDerivativeExtrema(t *testing.T) {
	path := Path{Commands: []PathCommand{
		MoveTo(0, 0),
		CubicTo(0, 30, 30, 30, 30, 0),
	}}
	bounds, err := path.GeometryBounds()
	if err != nil {
		t.Fatal(err)
	}
	assertBounds(t, bounds, 0, 0, 30, 22.5)
}

func TestTransformedCubicBoundsFindsWorldSpaceExtrema(t *testing.T) {
	path := Path{Commands: []PathCommand{
		MoveTo(0, 0),
		CubicTo(0, 4, 4, 4, 4, 0),
	}}
	// x'=x+y. Transforming the local AABB would report max X=7. Analytic
	// world-space extrema correctly find 4*sqrt(2).
	transform := Matrix{A: 1, C: 1, D: 1}
	bounds, err := path.transformedGeometryBounds(transform)
	if err != nil {
		t.Fatal(err)
	}
	assertBounds(t, bounds, 0, 0, 4*math.Sqrt(2), 3)
}

func TestArcGeometryBoundsIncludesInteriorExtrema(t *testing.T) {
	path := Path{Commands: []PathCommand{
		MoveTo(1, 0),
		ArcTo(1, 1, 0, false, true, 0, 1),
	}}
	bounds, err := path.GeometryBounds()
	if err != nil {
		t.Fatal(err)
	}
	assertBounds(t, bounds, 0, 0, 1, 1)

	rotated, err := path.transformedGeometryBounds(Rotate(math.Pi / 4))
	if err != nil {
		t.Fatal(err)
	}
	assertBounds(t, rotated, -math.Sqrt(0.5), math.Sqrt(0.5), math.Sqrt(0.5), 1)
}

func TestArcRadiiCorrectionIsExponentScaled(t *testing.T) {
	t.Parallel()
	tiny := math.SmallestNonzeroFloat64
	path := Path{Commands: []PathCommand{
		MoveTo(-1, 0),
		ArcTo(tiny, tiny, 0, false, true, 1, 0),
	}}
	bounds, err := path.GeometryBounds()
	if err != nil {
		t.Fatal(err)
	}
	assertBounds(t, bounds, -1, -1, 1, 0)

	arc, ok, err := EndpointArcToCenter(Point{X: -1}, path.Commands[1])
	if err != nil || !ok {
		t.Fatalf("EndpointArcToCenter() = %#v, %t, %v", arc, ok, err)
	}
	if got := arc.PointAt(arc.StartAngle() + arc.DeltaAngle()/2); math.Abs(got.X) > 1e-12 || math.Abs(got.Y+1) > 1e-12 {
		t.Fatalf("corrected midpoint = %#v, want (0,-1)", got)
	}
}

func TestMaxScaleAvoidsIntermediateOverflowAndUnderflow(t *testing.T) {
	t.Parallel()
	for _, scale := range []float64{1e-200, 1e-80, 1e80, 1e200} {
		got := Scale(scale, scale).MaxScale()
		if !finite(got) || math.Abs(got/scale-1) > 1e-15 {
			t.Fatalf("Scale(%g).MaxScale() = %g", scale, got)
		}
	}
}

func TestExponentScaledArcMatchesReferenceAtOrdinaryMagnitudes(t *testing.T) {
	t.Parallel()
	random := rand.New(rand.NewSource(7))
	for iteration := 0; iteration < 2_000; iteration++ {
		start := Point{X: random.Float64()*100 - 50, Y: random.Float64()*100 - 50}
		end := Point{X: random.Float64()*100 - 50, Y: random.Float64()*100 - 50}
		command := ArcTo(
			random.Float64()*99+1,
			random.Float64()*99+1,
			random.Float64()*2*math.Pi-math.Pi,
			random.Intn(2) != 0,
			random.Intn(2) != 0,
			end.X, end.Y,
		)
		got, ok, err := EndpointArcToCenter(start, command)
		if err != nil || !ok {
			t.Fatalf("iteration %d conversion = %#v, %t, %v", iteration, got, ok, err)
		}
		want := referenceCenterArc(start, command)
		for _, fraction := range []float64{0, 0.125, 0.5, 0.875, 1} {
			gotPoint := got.PointAt(got.start + got.delta*fraction)
			wantPoint := want.PointAt(want.start + want.delta*fraction)
			scale := math.Max(1, math.Max(math.Hypot(gotPoint.X, gotPoint.Y), math.Hypot(wantPoint.X, wantPoint.Y)))
			if math.Hypot(gotPoint.X-wantPoint.X, gotPoint.Y-wantPoint.Y) > 2e-12*scale {
				t.Fatalf("iteration %d fraction %g point = %#v, want %#v; start=%#v command=%#v gotArc=%#v wantArc=%#v", iteration, fraction, gotPoint, wantPoint, start, command, got, want)
			}
		}
	}
}

func referenceCenterArc(start Point, command PathCommand) CenterArc {
	end := command.P1
	rx, ry := math.Abs(command.RadiusX), math.Abs(command.RadiusY)
	sinPhi, cosPhi := math.Sincos(command.Rotation)
	dx, dy := (start.X-end.X)/2, (start.Y-end.Y)/2
	xPrime := cosPhi*dx + sinPhi*dy
	yPrime := -sinPhi*dx + cosPhi*dy
	radiiScale := xPrime*xPrime/(rx*rx) + yPrime*yPrime/(ry*ry)
	corrected := radiiScale > 1
	if radiiScale > 1 {
		scale := math.Sqrt(radiiScale)
		rx, ry = rx*scale, ry*scale
	}
	numerator := rx*rx*ry*ry - rx*rx*yPrime*yPrime - ry*ry*xPrime*xPrime
	denominator := rx*rx*yPrime*yPrime + ry*ry*xPrime*xPrime
	coefficient := 0.0
	if !corrected && denominator != 0 {
		coefficient = math.Sqrt(math.Max(0, numerator/denominator))
	}
	if command.LargeArc == command.Sweep {
		coefficient = -coefficient
	}
	cxPrime := coefficient * rx * yPrime / ry
	cyPrime := -coefficient * ry * xPrime / rx
	center := Point{
		X: cosPhi*cxPrime - sinPhi*cyPrime + (start.X+end.X)/2,
		Y: sinPhi*cxPrime + cosPhi*cyPrime + (start.Y+end.Y)/2,
	}
	ux, uy := (xPrime-cxPrime)/rx, (yPrime-cyPrime)/ry
	vx, vy := (-xPrime-cxPrime)/rx, (-yPrime-cyPrime)/ry
	startAngle := math.Atan2(uy, ux)
	delta := math.Atan2(ux*vy-uy*vx, ux*vx+uy*vy)
	if command.Sweep && delta < 0 {
		delta += 2 * math.Pi
	} else if !command.Sweep && delta > 0 {
		delta -= 2 * math.Pi
	}
	return CenterArc{
		center: center,
		u:      Point{X: rx * cosPhi, Y: rx * sinPhi},
		v:      Point{X: -ry * sinPhi, Y: ry * cosPhi},
		start:  startAngle,
		delta:  delta,
	}
}

func TestEllipseBoundsUnderAffineTransform(t *testing.T) {
	paint := SolidPaint{Color: color.NRGBA{A: 255}}
	ellipse := Ellipse{
		Center:  Point{},
		RadiusX: 2,
		RadiusY: 1,
		Fill:    paint,
	}
	bounds, err := PrimitiveBounds(ellipse, Rotate(math.Pi/4))
	if err != nil {
		t.Fatal(err)
	}
	radius := math.Sqrt(2.5)
	assertBounds(t, bounds, -radius, -radius, radius, radius)
}

func TestStrokeBoundsAreConservativeAndTransformed(t *testing.T) {
	paint := SolidPaint{Color: color.NRGBA{A: 255}}
	path := Path{
		Commands: []PathCommand{MoveTo(0, 0), LineTo(10, 0)},
		Stroke: &Stroke{
			Paint: paint,
			Width: 2,
			Cap:   CapRound,
			Join:  JoinRound,
		},
	}
	bounds, err := PrimitiveBounds(path, Scale(2, 3))
	if err != nil {
		t.Fatal(err)
	}
	// MaxScale=3, so the 1-unit local half-width expands by 3 in world space.
	assertBounds(t, bounds, -3, -3, 23, 3)
}

func TestAnalyticBoundsAreBitwiseDeterministic(t *testing.T) {
	path := Path{Commands: []PathCommand{
		MoveTo(-3.25, 8.5),
		CubicTo(10.125, -12.75, 20.5, 31.25, 40.75, -2.5),
		ArcTo(11.5, 7.25, 0.31, true, false, -3.25, 8.5),
		ClosePath(),
	}}
	transform := Translate(17.5, -8.25).Mul(Rotate(0.37)).Mul(Scale(1.25, 0.8))
	want, err := path.transformedGeometryBounds(transform)
	if err != nil {
		t.Fatal(err)
	}
	bits := [4]uint64{
		math.Float64bits(want.Min.X), math.Float64bits(want.Min.Y),
		math.Float64bits(want.Max.X), math.Float64bits(want.Max.Y),
	}
	for i := 0; i < 1_000; i++ {
		got, err := path.transformedGeometryBounds(transform)
		if err != nil {
			t.Fatal(err)
		}
		gotBits := [4]uint64{
			math.Float64bits(got.Min.X), math.Float64bits(got.Min.Y),
			math.Float64bits(got.Max.X), math.Float64bits(got.Max.Y),
		}
		if gotBits != bits {
			t.Fatalf("iteration %d is non-deterministic: got %#v, want %#v", i, gotBits, bits)
		}
	}
}

func TestPathRejectsMalformedCommandOrder(t *testing.T) {
	_, err := (Path{Commands: []PathCommand{LineTo(1, 1)}}).GeometryBounds()
	if err == nil {
		t.Fatal("expected line-before-move error")
	}
}

func assertBounds(t *testing.T, got Bounds, minX, minY, maxX, maxY float64) {
	t.Helper()
	if !got.Valid {
		t.Fatal("bounds are invalid")
	}
	want := [4]float64{minX, minY, maxX, maxY}
	values := [4]float64{got.Min.X, got.Min.Y, got.Max.X, got.Max.Y}
	for i := range values {
		if diff := math.Abs(values[i] - want[i]); diff > 1e-9 {
			t.Fatalf("bounds mismatch: got [%g,%g]-[%g,%g], want [%g,%g]-[%g,%g] (component %d differs by %g)",
				got.Min.X, got.Min.Y, got.Max.X, got.Max.Y,
				minX, minY, maxX, maxY, i, diff)
		}
	}
}
