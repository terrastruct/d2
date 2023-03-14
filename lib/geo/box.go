package geo

import "fmt"

type Box struct {
	TopLeft *Point
	Width   float64
	Height  float64
}

func NewBox(tl *Point, width, height float64) *Box {
	return &Box{
		TopLeft: tl,
		Width:   width,
		Height:  height,
	}
}

func (b *Box) Copy() *Box {
	if b == nil {
		return nil
	}
	return NewBox(b.TopLeft.Copy(), b.Width, b.Height)
}

func (b *Box) Center() *Point {
	return NewPoint(b.TopLeft.X+b.Width/2, b.TopLeft.Y+b.Height/2)
}

// Intersects returns true if the segment comes within buffer of the box
func (b *Box) Intersects(s Segment, buffer float64) bool {
	tl := NewPoint(b.TopLeft.X-buffer, b.TopLeft.Y-buffer)
	tr := NewPoint(tl.X+b.Width+buffer*2, tl.Y)
	br := NewPoint(tr.X, tr.Y+b.Height+buffer*2)
	bl := NewPoint(tl.X, br.Y)

	if p := IntersectionPoint(s.Start, s.End, tl, tr); p != nil {
		return true
	}
	if p := IntersectionPoint(s.Start, s.End, tr, br); p != nil {
		return true
	}
	if p := IntersectionPoint(s.Start, s.End, br, bl); p != nil {
		return true
	}
	if p := IntersectionPoint(s.Start, s.End, bl, tl); p != nil {
		return true
	}
	return false
}

func (b *Box) Intersections(s Segment) []*Point {
	pts := []*Point{}

	tl := b.TopLeft
	tr := NewPoint(tl.X+b.Width, tl.Y)
	br := NewPoint(tr.X, tr.Y+b.Height)
	bl := NewPoint(tl.X, br.Y)

	if p := IntersectionPoint(s.Start, s.End, tl, tr); p != nil {
		pts = append(pts, p)
	}
	if p := IntersectionPoint(s.Start, s.End, tr, br); p != nil {
		pts = append(pts, p)
	}
	if p := IntersectionPoint(s.Start, s.End, br, bl); p != nil {
		pts = append(pts, p)
	}
	if p := IntersectionPoint(s.Start, s.End, bl, tl); p != nil {
		pts = append(pts, p)
	}
	return pts
}

func (b *Box) ToString() string {
	if b == nil {
		return ""
	}
	return fmt.Sprintf("{TopLeft: %s, Width: %.0f, Height: %.0f}", b.TopLeft.ToString(), b.Width, b.Height)
}
