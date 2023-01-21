package shape

import (
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/svg"
)

type shapeQueue struct {
	*baseShape
}

func NewQueue(box *geo.Box) Shape {
	return shapeQueue{
		baseShape: &baseShape{
			Type: QUEUE_TYPE,
			Box:  box,
		},
	}
}

func getArcWidth(box *geo.Box) float64 {
	arcWidth := defaultArcDepth
	// Note: box width should always be larger than 3*default
	// this just handles after collaping into an oval
	if box.Width < arcWidth*2 {
		arcWidth = box.Width / 2.0
	}
	return arcWidth
}

func (s shapeQueue) GetInnerBox() *geo.Box {
	width := s.Box.Width
	tl := s.Box.TopLeft.Copy()
	arcWidth := getArcWidth(s.Box)
	width -= 3 * arcWidth
	tl.X += arcWidth
	return geo.NewBox(tl, width, s.Box.Height)
}

func queueOuterPath(box *geo.Box) *svg.SvgPathContext {
	arcWidth := getArcWidth(box)
	multiplier := 0.45
	pc := svg.NewSVGPathContext(box.TopLeft, 1, 1)
	pc.StartAt(pc.Absolute(arcWidth, 0))
	pc.H(true, box.Width-2*arcWidth)
	pc.C(false, box.Width, 0, box.Width, box.Height*multiplier, box.Width, box.Height/2.0)
	pc.C(false, box.Width, box.Height-box.Height*multiplier, box.Width, box.Height, box.Width-arcWidth, box.Height)
	pc.H(true, -1*(box.Width-2*arcWidth))
	pc.C(false, 0, box.Height, 0, box.Height-box.Height*multiplier, 0, box.Height/2.0)
	pc.C(false, 0, box.Height*multiplier, 0, 0, arcWidth, 0)
	pc.Z()
	return pc
}

func queueInnerPath(box *geo.Box) *svg.SvgPathContext {
	arcWidth := getArcWidth(box)
	multiplier := 0.45
	pc := svg.NewSVGPathContext(box.TopLeft, 1, 1)
	pc.StartAt(pc.Absolute(box.Width-arcWidth, 0))
	pc.C(false, box.Width-2*arcWidth, 0, box.Width-2*arcWidth, box.Height*multiplier, box.Width-2*arcWidth, box.Height/2.0)
	pc.C(false, box.Width-2*arcWidth, box.Height-box.Height*multiplier, box.Width-2*arcWidth, box.Height, box.Width-arcWidth, box.Height)
	return pc
}

func (s shapeQueue) Perimeter() []geo.Intersectable {
	return queueOuterPath(s.Box).Path
}

func (s shapeQueue) GetSVGPathData() []string {
	return []string{
		queueOuterPath(s.Box).PathData(),
		queueInnerPath(s.Box).PathData(),
	}
}

func (s shapeQueue) GetDimensionsToFit(width, height, padding float64) (float64, float64) {
	// 1 arc left, width+ padding, 2 arcs right
	totalWidth := 3*defaultArcDepth + width + padding*2
	return totalWidth, height + padding*2
}
