package geo

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

//nolint:forbidigo

type RelativePoint struct {
	XPercentage float64
	YPercentage float64
}

func NewRelativePoint(xPercentage, yPercentage float64) *RelativePoint {
	// 3 decimal points of precision is enough. Floating points on Bezier curves can reach the level of precision where different machines round differently.
	return &RelativePoint{
		XPercentage: TruncateDecimals(xPercentage),
		YPercentage: TruncateDecimals(yPercentage),
	}
}

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

func NewPoint(x, y float64) *Point {
	return &Point{X: x, Y: y}
}

func (p1 *Point) Equals(p2 *Point) bool {
	if p1 == nil {
		return p2 == nil
	} else if p2 == nil {
		return false
	}
	return (p1.X == p2.X) && (p1.Y == p2.Y)
}

func (p1 *Point) Compare(p2 *Point) int {
	xCompare := Sign(p1.X - p2.X)
	if xCompare == 0 {
		return Sign(p1.Y - p2.Y)
	}
	return xCompare
}

func (p *Point) Copy() *Point {
	return &Point{X: p.X, Y: p.Y}
}

type Points []*Point

func (ps Points) Equals(other Points) bool {
	if ps == nil {
		return other == nil
	} else if other == nil {
		return false
	}
	pointsSet := make(map[Point]struct{})
	for _, p := range ps {
		pointsSet[*p] = struct{}{}
	}

	for _, otherPoint := range other {
		if _, exists := pointsSet[*otherPoint]; !exists {
			return false
		}
	}
	return true
}

func (ps Points) GetMedian() *Point {
	xs := make([]float64, 0)
	ys := make([]float64, 0)

	for _, p := range ps {
		xs = append(xs, p.X)
		ys = append(ys, p.Y)
	}

	sort.Float64s(xs)
	sort.Float64s(ys)

	middleIndex := len(xs) / 2

	medianX := xs[middleIndex]
	medianY := ys[middleIndex]

	if len(xs)%2 == 0 {
		medianX += xs[middleIndex-1]
		medianX /= 2
		medianY += ys[middleIndex-1]
		medianY /= 2
	}

	return &Point{X: medianX, Y: medianY}
}

// GetOrientation gets orientation of pFrom to pTo
// E.g. pFrom ---> pTo, here, pFrom is to the left of pTo, so Left would be returned
func (pFrom *Point) GetOrientation(pTo *Point) Orientation {
	if pFrom.Y < pTo.Y {
		if pFrom.X < pTo.X {
			return TopLeft
		}
		if pFrom.X > pTo.X {
			return TopRight
		}
		return Top
	}

	if pFrom.Y > pTo.Y {
		if pFrom.X < pTo.X {
			return BottomLeft
		}
		if pFrom.X > pTo.X {
			return BottomRight
		}
		return Bottom
	}

	if pFrom.X < pTo.X {
		return Left
	}

	if pFrom.X > pTo.X {
		return Right
	}

	return NONE
}

func (p *Point) ToString() string {
	if p == nil {
		return ""
	}
	return fmt.Sprintf("(%v, %v)", p.X, p.Y)
}

func (points Points) ToString() string {
	strs := make([]string, 0, len(points))
	for _, p := range points {
		strs = append(strs, p.ToString())
	}
	return strings.Join(strs, ", ")
}

// https://stackoverflow.com/questions/849211/shortest-distance-between-a-point-and-a-line-segment
func (p *Point) DistanceToLine(p1, p2 *Point) float64 {
	a := p.X - p1.X
	b := p.Y - p1.Y
	c := p2.X - p1.X
	d := p2.Y - p1.Y

	dot := (a * c) + (b * d)
	len_sq := (c * c) + (d * d)

	param := -1.0

	if len_sq != 0 {
		param = dot / len_sq
	}

	var xx float64
	var yy float64

	if param < 0.0 {
		xx = p1.X
		yy = p1.Y
	} else if param > 1.0 {
		xx = p2.X
		yy = p2.Y
	} else {
		xx = p1.X + (param * c)
		yy = p1.Y + (param * d)
	}

	dx := p.X - xx
	dy := p.Y - yy

	return math.Sqrt((dx * dx) + (dy * dy))
}

// Moves the given point by Vector
func (start *Point) AddVector(v Vector) *Point {
	return start.ToVector().Add(v).ToPoint()
}

// Creates a Vector of the size between start and endpoint, pointing to endpoint
func (start *Point) VectorTo(endpoint *Point) Vector {
	return endpoint.ToVector().Minus(start.ToVector())
}

func (p *Point) FormattedCoordinates() string {
	return fmt.Sprintf("%d,%d", int(p.X), int(p.Y))
}

// returns true if point p is on orthogonal segment between points a and b
func (p *Point) OnOrthogonalSegment(a, b *Point) bool {
	if a.X < b.X {
		if p.X < a.X || b.X < p.X {
			return false
		}
	} else if p.X < b.X || a.X < p.X {
		return false
	}
	if a.Y < b.Y {
		if p.Y < a.Y || b.Y < p.Y {
			return false
		}
	} else if p.Y < b.Y || a.Y < p.Y {
		return false
	}
	return true
}

// Creates a Vector pointing to point
func (endpoint *Point) ToVector() Vector {
	return []float64{endpoint.X, endpoint.Y}
}

// get the point of intersection between line segments u and v (or nil if they do not intersect)
func IntersectionPoint(u0, u1, v0, v1 *Point) *Point {
	// https://en.wikipedia.org/wiki/Intersection_(Euclidean_geometry)
	//
	// Example ('-' = 1, '|' = 1):
	//    v0
	//    |
	//u0 -+--- u1
	//    |
	//    |
	//    v1
	//
	// s = 0.2 (1/5 along u)
	// t = 0.25 (1/4 along v)
	// we compute s and t and if they are both in range [0,1], then
	// they intersect and we compute the point of intersection to return

	// when s = 0, x = u.Start.X; when s = 1, x = u.End.X
	// x = s * u1.X + (1 - s) * u0.X
	//   = u0.X + s * (u1.X - u0.X)

	// x = u0.X + s * (u1.X - u0.X)
	//   = v0.X + t * (v1.X - v0.X)
	// y = u0.Y + s * (u1.Y - u0.Y)
	//   = v0.Y + t * (v1.Y - v0.Y)

	// s * (u1.X - u0.X) - t * (v1.X - v0.X) = v0.X - u0.X
	// s*udx - t*vdx = uvdx
	// s*udy - t*vdy = uvdy
	udx := u1.X - u0.X
	vdx := v1.X - v0.X
	uvdx := v0.X - u0.X
	udy := u1.Y - u0.Y
	vdy := v1.Y - v0.Y
	uvdy := v0.Y - u0.Y

	denom := (udy*vdx - udx*vdy)
	if denom == 0 {
		// lines are parallel
		return nil
	}
	// Cramer's rule
	s := (vdx*uvdy - vdy*uvdx) / denom
	t := (udx*uvdy - udy*uvdx) / denom

	// lines don't intersect within segments
	if s < 0 || s > 1 || t < 0 || t > 1 {
		// if s or t is outside [0, 1], the intersection of the lines are not on the segments
		return nil
	}

	// use s parameter to get point along u
	intersection := new(Point)
	intersection.X = u0.X + math.Round(s*udx)
	intersection.Y = u0.Y + math.Round(s*udy)
	return intersection
}

func (p *Point) Transpose() {
	if p == nil {
		return
	}
	p.X, p.Y = p.Y, p.X
}

// point t% of the way between a and b
func (a *Point) Interpolate(b *Point, t float64) *Point {
	return NewPoint(
		a.X*(1.0-t)+b.X*t,
		a.Y*(1.0-t)+b.Y*t,
	)
}
