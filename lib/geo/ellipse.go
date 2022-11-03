package geo

import (
	"math"
	"sort"
)

type Ellipse struct {
	Center *Point
	Rx     float64
	Ry     float64
}

func NewEllipse(center *Point, rx, ry float64) *Ellipse {
	return &Ellipse{
		Center: center,
		Rx:     rx,
		Ry:     ry,
	}
}

func (e Ellipse) Intersections(segment Segment) []*Point {
	// we check for intersections between the ellipse and line segment in the following manner:
	// 0. we compute ignoring the ellipse's position, as if it were centered at 0,0 for a simpler computation
	// 1. translate the line segment variables to match the ellipse's "translation" to 0,0
	// 2. get the (infinite) line equation for the given line segment
	// 3. compute the intersections between the line and ellipse
	// 4. filter out intersections that are on the line but not on the line segment
	// 5. apply the inverse translation to the intersection to get the result relative to where the ellipse and line actually are
	intersections := []*Point{}

	// ellipse equation: (x-cx)^2/rx^2 + (y-cy)^2/ry^2 = 1
	// in the form: x^2/a^2 + y^2/b^2 = 1
	a := e.Rx
	b := e.Ry
	a2 := a * a
	b2 := b * b
	if a <= 0 || b <= 0 {
		return nil
	}

	// line for a line segment between (x1,y1) and (x2, y2): (see https://en.wikipedia.org/wiki/Linear_equation#Two-point_form)
	//   y - y1 = ((y2 - y1)/(x2 - x1))(x - x1)
	x1 := segment.Start.X - e.Center.X
	y1 := segment.Start.Y - e.Center.Y
	x2 := segment.End.X - e.Center.X
	y2 := segment.End.Y - e.Center.Y

	// Handle the edge case of a vertical line (avoiding dividing by 0)
	if x1 == x2 {
		// Ellipse solutions for a given x (from wolfram "(x^2)/(a^2)+(y^2)/(b^2)=1"):
		// 1. y = +b*root(a^2 - x^2)/a
		intersection1 := NewPoint(x1, +b*math.Sqrt(a2-x1*x1)/a)
		// 2. y = -b*root(a^2 - x^2)/a
		intersection2 := intersection1.Copy()
		intersection2.Y *= -1

		isPointOnLine := func(p *Point) bool {
			ps := []float64{p.Y, y1, y2}
			sort.Slice(ps, func(i, j int) bool {
				return ps[i] < ps[j]
			})
			return ps[1] == p.Y
		}

		if isPointOnLine(intersection1) {
			intersections = append(intersections, intersection1)
		}
		// when y = 0, intersection2 will be a duplicate of intersection1
		if intersection2.Y != 0.0 && isPointOnLine(intersection2) {
			intersections = append(intersections, intersection2)
		}

		for _, p := range intersections {
			p.X += e.Center.X
			p.Y += e.Center.Y
		}
		return intersections
	}

	// converting line to form: y = mx + c
	// from: y - y1 = ((y2-y1)/(x2-x1))(x - x1)
	// m = (y2-y1)/(x2-x1), c = y1 - m*x1
	m := (y2 - y1) / (x2 - x1)
	c := y1 - m*x1
	isPointOnLine := func(p *Point) bool {
		return PrecisionCompare(p.DistanceToLine(NewPoint(x1, y1), NewPoint(x2, y2)), 0, PRECISION) == 0
	}

	// "(x^2)/(a^2)+(y^2)/(b^2) =1, y = mx + c" solutions from wolfram:
	// if (a^2)(m^2) + b^2 != 0, and ab != 0
	// 2 solutions 1 with +, 1 with -
	// x = ( -c m a^2 {+/-} root(a^2 b^2 (a^2 m^2 + b^2 - c^2)) ) / (a^2 m^2 + b^2)
	// y = ( c b^2 {+/-} m root(a^2 b^2 (a^2 m^2 + b^2 - c^2)) ) / (a^2 m^2 + b^2)
	denom := a2*m*m + b2
	// Note: we already checked a and b != 0 so denom == 0 is impossible (assuming no imaginary numbers)
	root := math.Sqrt(a2 * b2 * (denom - c*c))

	intersection1 := NewPoint((-m*c*a2+root)/denom, (c*b2+m*root)/denom)
	intersection2 := NewPoint((-m*c*a2-root)/denom, (c*b2-m*root)/denom)

	if isPointOnLine(intersection1) {
		intersections = append(intersections, intersection1)
	}
	if !intersection1.Equals(intersection2) && isPointOnLine(intersection2) {
		intersections = append(intersections, intersection2)
	}

	for _, p := range intersections {
		p.X += e.Center.X
		p.Y += e.Center.Y
	}
	return intersections
}
