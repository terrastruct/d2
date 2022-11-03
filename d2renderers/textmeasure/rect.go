package textmeasure

import (
	"math"

	"oss.terrastruct.com/d2/lib/geo"
)

type rect struct {
	tl *geo.Point
	br *geo.Point
}

func newRect() *rect {
	return &rect{
		tl: geo.NewPoint(0, 0),
		br: geo.NewPoint(0, 0),
	}
}

func (r rect) w() float64 {
	return r.br.X - r.tl.X
}

func (r rect) h() float64 {
	return r.br.Y - r.tl.Y
}

// norm returns the Rect in normal form, such that Max is component-wise greater or equal than Min.
func (r rect) norm() *rect {
	return &rect{
		tl: geo.NewPoint(
			math.Min(r.tl.X, r.br.X),
			math.Min(r.tl.Y, r.br.Y),
		),
		br: geo.NewPoint(
			math.Max(r.tl.X, r.br.X),
			math.Max(r.tl.Y, r.br.Y),
		),
	}
}

func (r1 *rect) union(r2 *rect) *rect {
	r := newRect()
	r.tl.X = math.Min(r1.tl.X, r2.tl.X)
	r.tl.Y = math.Min(r1.tl.Y, r2.tl.Y)
	r.br.X = math.Max(r1.br.X, r2.br.X)
	r.br.Y = math.Max(r1.br.Y, r2.br.Y)

	return r
}
