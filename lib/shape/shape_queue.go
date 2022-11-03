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

func (s shapeQueue) GetInnerBox() *geo.Box {
	width := s.Box.Width
	tl := s.Box.TopLeft.Copy()
	arcDepth := 24.0
	if width < arcDepth*2 {
		arcDepth = width / 2.0
	}
	width -= 3 * arcDepth
	tl.X += arcDepth
	return geo.NewBox(tl, width, s.Box.Height)
}

func queueOuterPath(box *geo.Box) *svg.SvgPathContext {
	arcDepth := 24.0
	multiplier := 0.45
	if box.Width < arcDepth*2 {
		arcDepth = box.Width / 2.0
	}
	pc := svg.NewSVGPathContext(box.TopLeft, 1, 1)
	pc.StartAt(pc.Absolute(arcDepth, 0))
	pc.H(true, box.Width-2*arcDepth)
	pc.C(false, box.Width, 0, box.Width, box.Height*multiplier, box.Width, box.Height/2.0)
	pc.C(false, box.Width, box.Height-box.Height*multiplier, box.Width, box.Height, box.Width-arcDepth, box.Height)
	pc.H(true, -1*(box.Width-2*arcDepth))
	pc.C(false, 0, box.Height, 0, box.Height-box.Height*multiplier, 0, box.Height/2.0)
	pc.C(false, 0, box.Height*multiplier, 0, 0, arcDepth, 0)
	pc.Z()
	return pc
}

func queueInnerPath(box *geo.Box) *svg.SvgPathContext {
	arcDepth := 24.0
	multiplier := 0.45
	if box.Width < arcDepth*2 {
		arcDepth = box.Width / 2.0
	}
	pc := svg.NewSVGPathContext(box.TopLeft, 1, 1)
	pc.StartAt(pc.Absolute(box.Width-arcDepth, 0))
	pc.C(false, box.Width-2*arcDepth, 0, box.Width-2*arcDepth, box.Height*multiplier, box.Width-2*arcDepth, box.Height/2.0)
	pc.C(false, box.Width-2*arcDepth, box.Height-box.Height*multiplier, box.Width-2*arcDepth, box.Height, box.Width-arcDepth, box.Height)
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
