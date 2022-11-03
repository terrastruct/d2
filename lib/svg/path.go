package svg

import (
	"fmt"
	"math"
	"strings"

	"oss.terrastruct.com/d2/lib/geo"
)

type SvgPathContext struct {
	Path     []geo.Intersectable
	Commands []string
	Start    *geo.Point
	Current  *geo.Point
	TopLeft  *geo.Point
	ScaleX   float64
	ScaleY   float64
}

// TODO probably use math.Big
func chopPrecision(f float64) float64 {
	return math.Round(f*10000) / 10000
}

func NewSVGPathContext(tl *geo.Point, sx, sy float64) *SvgPathContext {
	return &SvgPathContext{TopLeft: tl.Copy(), ScaleX: sx, ScaleY: sy}
}

func (c *SvgPathContext) Relative(base *geo.Point, dx, dy float64) *geo.Point {
	return geo.NewPoint(chopPrecision(base.X+c.ScaleX*dx), chopPrecision(base.Y+c.ScaleY*dy))
}
func (c *SvgPathContext) Absolute(x, y float64) *geo.Point {
	return c.Relative(c.TopLeft, x, y)
}

func (c *SvgPathContext) StartAt(p *geo.Point) {
	c.Start = p
	c.Commands = append(c.Commands, fmt.Sprintf("M %v %v", p.X, p.Y))
	c.Current = p.Copy()
}

func (c *SvgPathContext) Z() {
	c.Path = append(c.Path, &geo.Segment{Start: c.Current.Copy(), End: c.Start.Copy()})
	c.Commands = append(c.Commands, "Z")
	c.Current = c.Start.Copy()
}

func (c *SvgPathContext) L(isLowerCase bool, x, y float64) {
	var endPoint *geo.Point
	if isLowerCase {
		endPoint = c.Relative(c.Current, x, y)
	} else {
		endPoint = c.Absolute(x, y)
	}
	c.Path = append(c.Path, &geo.Segment{Start: c.Current.Copy(), End: endPoint})
	c.Commands = append(c.Commands, fmt.Sprintf("L %v %v", endPoint.X, endPoint.Y))
	c.Current = endPoint.Copy()
}

func (c *SvgPathContext) C(isLowerCase bool, x1, y1, x2, y2, x3, y3 float64) {
	p := func(x, y float64) *geo.Point {
		if isLowerCase {
			return c.Relative(c.Current, x, y)
		}
		return c.Absolute(x, y)
	}
	points := []*geo.Point{c.Current.Copy(), p(x1, y1), p(x2, y2), p(x3, y3)}
	c.Path = append(c.Path, geo.NewBezierCurve(points))
	c.Commands = append(c.Commands, fmt.Sprintf(
		"C %v %v %v %v %v %v",
		points[1].X, points[1].Y,
		points[2].X, points[2].Y,
		points[3].X, points[3].Y,
	))
	c.Current = points[3].Copy()
}

func (c *SvgPathContext) H(isLowerCase bool, x float64) {
	var endPoint *geo.Point
	if isLowerCase {
		endPoint = c.Relative(c.Current, x, 0)
	} else {
		endPoint = c.Absolute(x, 0)
		endPoint.Y = c.Current.Y
	}
	c.Path = append(c.Path, &geo.Segment{Start: c.Current.Copy(), End: endPoint.Copy()})
	c.Commands = append(c.Commands, fmt.Sprintf("H %v", endPoint.X))
	c.Current = endPoint.Copy()
}

func (c *SvgPathContext) V(isLowerCase bool, y float64) {
	var endPoint *geo.Point
	if isLowerCase {
		endPoint = c.Relative(c.Current, 0, y)
	} else {
		endPoint = c.Absolute(0, y)
		endPoint.X = c.Current.X
	}
	c.Path = append(c.Path, &geo.Segment{Start: c.Current.Copy(), End: endPoint})
	c.Commands = append(c.Commands, fmt.Sprintf("V %v", endPoint.Y))
	c.Current = endPoint.Copy()
}

func (c *SvgPathContext) PathData() string {
	return strings.Join(c.Commands, " ")
}
