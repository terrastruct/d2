package shape

import (
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/svg"
)

type shapeCallout struct {
	*baseShape
}

func NewCallout(box *geo.Box) Shape {
	return shapeCallout{
		baseShape: &baseShape{
			Type: CALLOUT_TYPE,
			Box:  box,
		},
	}
}

func (s shapeCallout) GetInnerBox() *geo.Box {
	height := s.Box.Height
	tipHeight := 45.0
	if height < tipHeight*2 {
		tipHeight = height / 2.0
	}
	height -= tipHeight
	return geo.NewBox(s.Box.TopLeft.Copy(), s.Box.Width, height)
}

func calloutPath(box *geo.Box) *svg.SvgPathContext {
	tipWidth := 30.0
	if box.Width < tipWidth*2 {
		tipWidth = box.Width / 2.0
	}
	tipHeight := 45.0
	if box.Height < tipHeight*2 {
		tipHeight = box.Height / 2.0
	}
	pc := svg.NewSVGPathContext(box.TopLeft, 1, 1)
	pc.StartAt(pc.Absolute(0, 0))
	pc.V(true, box.Height-tipHeight)
	pc.H(true, box.Width/2.0)
	pc.V(true, tipHeight)
	pc.L(true, tipWidth, -tipHeight)
	pc.H(true, box.Width/2.0-tipWidth)
	pc.V(true, -(box.Height - tipHeight))
	pc.H(true, -box.Width)
	pc.Z()
	return pc
}

func (s shapeCallout) Perimeter() []geo.Intersectable {
	return calloutPath(s.Box).Path
}

func (s shapeCallout) GetSVGPathData() []string {
	return []string{
		calloutPath(s.Box).PathData(),
	}
}
