package shape

import (
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/svg"
)

type shapeStep struct {
	*baseShape
}

func NewStep(box *geo.Box) Shape {
	return shapeStep{
		baseShape: &baseShape{
			Type: STEP_TYPE,
			Box:  box,
		},
	}
}

const STEP_WEDGE_WIDTH = 35.0

func stepPath(box *geo.Box) *svg.SvgPathContext {
	wedgeWidth := STEP_WEDGE_WIDTH
	if box.Width <= wedgeWidth {
		wedgeWidth = box.Width / 2.0
	}
	pc := svg.NewSVGPathContext(box.TopLeft, 1, 1)
	pc.StartAt(pc.Absolute(0, 0))
	pc.L(false, box.Width-wedgeWidth, 0)
	pc.L(false, box.Width, box.Height/2)
	pc.L(false, box.Width-wedgeWidth, box.Height)
	pc.L(false, 0, box.Height)
	pc.L(false, wedgeWidth, box.Height/2)
	pc.Z()
	return pc
}

func (s shapeStep) Perimeter() []geo.Intersectable {
	return stepPath(s.Box).Path
}

func (s shapeStep) GetSVGPathData() []string {
	return []string{
		stepPath(s.Box).PathData(),
	}
}
