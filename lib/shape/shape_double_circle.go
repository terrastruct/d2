package shape

import (
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/svg"
)

type shapeDoubleCircle struct {
	*baseShape
}

func NewDoubleCircle(box *geo.Box) Shape {
	return shapeDoubleCircle{
		baseShape: &baseShape{
			Type: DOUBLE_CIRCLE_TYPE,
			Box:  box,
		},
	}
}

func doubleCirclePath(box *geo.Box) *svg.SvgPathContext {
	// halfYFactor := 43.6 / 87.3
	pc := svg.NewSVGPathContext(box.TopLeft, box.Width, box.Height)
	pc.StartAt(pc.Absolute(0.25, 0))
	// pc
	return pc
}

func (s shapeDoubleCircle) Perimeter() []geo.Intersectable {
	return doubleCirclePath(s.Box).Path
}

func (s shapeDoubleCircle) GetSVGPathData() []string {
	return []string{
		doubleCirclePath(s.Box).PathData(),
	}
}
