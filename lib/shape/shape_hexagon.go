package shape

import (
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/svg"
)

type shapeHexagon struct {
	*baseShape
}

func NewHexagon(box *geo.Box) Shape {
	return shapeHexagon{
		baseShape: &baseShape{
			Type: HEXAGON_TYPE,
			Box:  box,
		},
	}
}

func hexagonPath(box *geo.Box) *svg.SvgPathContext {
	halfYFactor := 43.6 / 87.3
	pc := svg.NewSVGPathContext(box.TopLeft, box.Width, box.Height)
	pc.StartAt(pc.Absolute(0.25, 0))
	pc.L(false, 0, halfYFactor)
	pc.L(false, 0.25, 1)
	pc.L(false, 0.75, 1)
	pc.L(false, 1, halfYFactor)
	pc.L(false, 0.75, 0)
	pc.Z()
	return pc
}

func (s shapeHexagon) Perimeter() []geo.Intersectable {
	return hexagonPath(s.Box).Path
}

func (s shapeHexagon) GetSVGPathData() []string {
	return []string{
		hexagonPath(s.Box).PathData(),
	}
}
