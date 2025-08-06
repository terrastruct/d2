// bezier.go is a manual translation of the code from (including some amendments in the comments section)
// https://www.particleincell.com/2013/cubic-line-intersection/

package geo

import (
	"math"
)

// How precise should comparisons be, avoid being too precise due to floating point issues
const PRECISION = 0.0001

// Local types to replace gonum dependencies
type bezierPoint struct {
	X, Y float64
}

type bezierControlPoint struct {
	Point, Control bezierPoint
}

type bezierCurveImpl []bezierControlPoint

// newBezierCurveImpl creates a new bezier curve from control points
// Implementation based on Robert D. Miller's algorithm from Graphics Gems 5
func newBezierCurveImpl(cp ...bezierPoint) bezierCurveImpl {
	if len(cp) == 0 {
		return nil
	}
	c := make(bezierCurveImpl, len(cp))
	for i, p := range cp {
		c[i].Point = p
	}

	var w float64
	for i, p := range c {
		switch i {
		case 0:
			w = 1
		case 1:
			w = float64(len(c)) - 1
		default:
			w *= float64(len(c)-i) / float64(i)
		}
		c[i].Control.X = p.Point.X * w
		c[i].Control.Y = p.Point.Y * w
	}

	return c
}

// pointAt returns the point at t along the curve, where 0 ≤ t ≤ 1
func (c bezierCurveImpl) pointAt(t float64) bezierPoint {
	c[0].Point = c[0].Control
	u := t
	for i, p := range c[1:] {
		c[i+1].Point = bezierPoint{
			X: p.Control.X * u,
			Y: p.Control.Y * u,
		}
		u *= t
	}

	var (
		t1 = 1 - t
		tt = t1
	)
	p := c[len(c)-1].Point
	for i := len(c) - 2; i >= 0; i-- {
		p.X += c[i].Point.X * tt
		p.Y += c[i].Point.Y * tt
		tt *= t1
	}

	return p
}

type BezierCurve struct {
	curve  bezierCurveImpl
	points []*Point
}

func NewBezierCurve(points []*Point) *BezierCurve {
	localPoints := make([]bezierPoint, len(points))
	for i := 0; i < len(points); i++ {
		localPoints[i] = bezierPoint{
			X: points[i].X,
			Y: points[i].Y,
		}
	}
	localCurve := newBezierCurveImpl(localPoints...)
	
	curve := &BezierCurve{
		curve:  localCurve,
		points: points,
	}
	return curve
}

func (bc BezierCurve) Intersections(segment Segment) []*Point {
	return ComputeIntersections(
		[]float64{
			bc.points[0].X,
			bc.points[1].X,
			bc.points[2].X,
			bc.points[3].X,
		},
		[]float64{
			bc.points[0].Y,
			bc.points[1].Y,
			bc.points[2].Y,
			bc.points[3].Y,
		},
		[]float64{
			segment.Start.X,
			segment.End.X,
		},
		[]float64{
			segment.Start.Y,
			segment.End.Y,
		},
	)
}

func (bc BezierCurve) At(point float64) *Point {
	curvePoint := bc.curve.pointAt(point)
	return NewPoint(curvePoint.X, curvePoint.Y)
}

// nolint
func ComputeIntersections(px, py, lx, ly []float64) []*Point {
	out := make([]*Point, 0)

	var A = ly[1] - ly[0]
	var B = lx[0] - lx[1]
	var C = lx[0]*(ly[0]-ly[1]) + ly[0]*(lx[1]-lx[0])

	var bx = bezierCoeffs(px[0], px[1], px[2], px[3])
	var by = bezierCoeffs(py[0], py[1], py[2], py[3])

	P := make([]float64, 4)
	P[0] = A*bx[0] + B*by[0]
	P[1] = A*bx[1] + B*by[1]
	P[2] = A*bx[2] + B*by[2]
	P[3] = A*bx[3] + B*by[3] + C

	var r = cubicRoots(P)

	for i := 0; i < 3; i++ {
		t := r[i]

		point := &Point{
			X: bx[0]*t*t*t + bx[1]*t*t + bx[2]*t + bx[3],
			Y: by[0]*t*t*t + by[1]*t*t + by[2]*t + by[3],
		}

		var s float64
		if (lx[1] - lx[0]) != 0 {
			s = (point.X - lx[0]) / (lx[1] - lx[0])
		} else {
			s = (point.Y - ly[0]) / (ly[1] - ly[0])
		}

		tLT0 := PrecisionCompare(t, 0, PRECISION) < 0
		tGT1 := PrecisionCompare(t, 1, PRECISION) > 0
		sLT0 := PrecisionCompare(s, 0, PRECISION) < 0
		sGT1 := PrecisionCompare(s, 1, PRECISION) > 0
		if !(tLT0 || tGT1 || sLT0 || sGT1) {
			out = append(out, point)
		}
	}

	return out
}

// nolint
func cubicRoots(P []float64) []float64 {
	if PrecisionCompare(P[0], 0, PRECISION) == 0 {
		if PrecisionCompare(P[1], 0, PRECISION) == 0 {
			t := make([]float64, 3)
			t[0] = -1 * (P[3] / P[2])
			t[1] = -1
			t[2] = -1

			for i := 0; i < 1; i++ {
				tiLT0 := PrecisionCompare(t[i], 0, PRECISION) < 0
				tiGT1 := PrecisionCompare(t[i], 1, PRECISION) > 0
				if tiLT0 || tiGT1 {
					t[i] = -1
				}
			}

			t = sortSpecial(t)
			return t
		}

		var DQ = math.Pow(P[2], 2) - 4*P[1]*P[3]
		if PrecisionCompare(DQ, 0, PRECISION) >= 0 {
			DQ = math.Sqrt(DQ)
			t := make([]float64, 3)
			t[0] = -1 * ((DQ + P[2]) / (2 * P[1]))
			t[1] = ((DQ - P[2]) / (2 * P[1]))
			t[2] = -1

			//lint:ignore SA4008 TODO this returns before looping?
			for i := 0; i < 2; i++ {
				tiLT0 := PrecisionCompare(t[i], 0, PRECISION) < 0
				tiGT1 := PrecisionCompare(t[i], 1, PRECISION) > 0
				if tiLT0 || tiGT1 {
					t[i] = -1
				}

				t = sortSpecial(t)

				//lint:ignore SA4004 TODO this always returns on the first iteration
				return t
			}
		}
	}

	var a = P[0]
	var b = P[1]
	var c = P[2]
	var d = P[3]

	var A = b / a
	var B = c / a
	var C = d / a

	var Q, R, D, Im float64

	Q = (3*B - math.Pow(A, 2)) / 9
	R = (9*A*B - 27*C - 2*math.Pow(A, 3)) / 54
	D = math.Pow(Q, 3) + math.Pow(R, 2)

	t := make([]float64, 3)

	if PrecisionCompare(D, 0, PRECISION) >= 0 {
		var S = sgn(R+math.Sqrt(D)) * math.Pow(math.Abs(R+math.Sqrt(D)), (1/3.0))
		var T = sgn(R-math.Sqrt(D)) * math.Pow(math.Abs(R-math.Sqrt(D)), (1/3.0))

		t[0] = -A/3 + (S + T)
		t[1] = -A/3 - (S+T)/2
		t[2] = -A/3 - (S+T)/2
		Im = math.Abs(math.Sqrt(3) * (S - T) / 2)

		if PrecisionCompare(Im, 0, PRECISION) != 0 {
			t[1] = -1
			t[2] = -1
		}

	} else {
		var th = math.Acos(R / math.Sqrt(-math.Pow(Q, 3)))

		t[0] = 2*math.Sqrt(-Q)*math.Cos(th/3) - A/3
		t[1] = 2*math.Sqrt(-Q)*math.Cos((th+2*math.Pi)/3) - A/3
		t[2] = 2*math.Sqrt(-Q)*math.Cos((th+4*math.Pi)/3) - A/3
		Im = 0.0
	}

	for i := 0; i < 3; i++ {
		tiLT0 := PrecisionCompare(t[i], 0, PRECISION) < 0
		tiGT1 := PrecisionCompare(t[i], 1, PRECISION) > 0
		if tiLT0 || tiGT1 {
			t[i] = -1
		}
	}

	t = sortSpecial(t)

	return t
}

// nolint
func sortSpecial(a []float64) []float64 {
	var flip bool
	var temp float64

	for {
		flip = false
		for i := 0; i < len(a)-1; i++ {
			ai1GTEQ0 := PrecisionCompare(a[i+1], 0, PRECISION) >= 0
			aiGTai1 := PrecisionCompare(a[i], a[i+1], PRECISION) > 0
			aiLT0 := PrecisionCompare(a[i], 0, PRECISION) < 0
			if (ai1GTEQ0 && aiGTai1) || (aiLT0 && ai1GTEQ0) {
				flip = true
				temp = a[i]
				a[i] = a[i+1]
				a[i+1] = temp

			}
		}
		if !flip {
			break
		}
	}
	return a
}

// nolint
func sgn(x float64) float64 {
	if x < 0.0 {
		return -1
	}
	return 1
}

// nolint
func bezierCoeffs(P0, P1, P2, P3 float64) []float64 {
	Z := make([]float64, 4)
	Z[0] = -P0 + 3*P1 + -3*P2 + P3
	Z[1] = 3*P0 - 6*P1 + 3*P2
	Z[2] = -3*P0 + 3*P1
	Z[3] = P0
	return Z
}
