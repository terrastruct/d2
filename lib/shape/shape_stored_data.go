package shape

import (
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/svg"
)

type shapeStoredData struct {
	*baseShape
}

func NewStoredData(box *geo.Box) Shape {
	return shapeStoredData{
		baseShape: &baseShape{
			Type: STORED_DATA_TYPE,
			Box:  box,
		},
	}
}

func storedDataPath(box *geo.Box) *svg.SvgPathContext {
	wedgeWidth := 15.0
	multiplier := 0.27
	if box.Width < wedgeWidth*2 {
		wedgeWidth = box.Width / 2.0
	}
	pc := svg.NewSVGPathContext(box.TopLeft, 1, 1)
	pc.StartAt(pc.Absolute(wedgeWidth, 0))
	pc.H(true, box.Width-wedgeWidth)
	pc.C(false, box.Width-wedgeWidth*multiplier, 0, box.Width-wedgeWidth, box.Height*multiplier, box.Width-wedgeWidth, box.Height/2.0)
	pc.C(false, box.Width-wedgeWidth, box.Height-box.Height*multiplier, box.Width-wedgeWidth*multiplier, box.Height, box.Width, box.Height)
	pc.H(true, -(box.Width - wedgeWidth))
	pc.C(false, wedgeWidth-wedgeWidth*multiplier, box.Height, 0, box.Height-box.Height*multiplier, 0, box.Height/2.0)
	pc.C(false, 0, box.Height*multiplier, wedgeWidth-wedgeWidth*multiplier, 0, wedgeWidth, 0)
	pc.Z()
	return pc
}

func (s shapeStoredData) Perimeter() []geo.Intersectable {
	return storedDataPath(s.Box).Path
}

func (s shapeStoredData) GetSVGPathData() []string {
	return []string{
		storedDataPath(s.Box).PathData(),
	}
}
