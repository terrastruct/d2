package shape

import (
	"math"

	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/svg"
	"oss.terrastruct.com/util-go/go2"
)

type shapeStoredData struct {
	*baseShape
}

const storedDataWedgeWidth = 15.

func NewStoredData(box *geo.Box) Shape {
	shape := shapeStoredData{
		baseShape: &baseShape{
			Type: STORED_DATA_TYPE,
			Box:  box,
		},
	}
	shape.FullShape = go2.Pointer(Shape(shape))
	return shape
}

func (s shapeStoredData) GetInnerBox() *geo.Box {
	width := s.Box.Width
	tl := s.Box.TopLeft.Copy()
	width -= 2 * storedDataWedgeWidth
	tl.X += storedDataWedgeWidth
	return geo.NewBox(tl, width, s.Box.Height)
}

func storedDataPath(box *geo.Box) *svg.SvgPathContext {
	wedgeWidth := storedDataWedgeWidth
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

func (s shapeStoredData) GetDimensionsToFit(width, height, paddingX, paddingY float64) (float64, float64) {
	totalWidth := width + paddingX + 2*storedDataWedgeWidth
	return math.Ceil(totalWidth), math.Ceil(height + paddingY)
}

func (s shapeStoredData) GetDefaultPadding() (paddingX, paddingY float64) {
	return defaultPadding - 10, defaultPadding
}
