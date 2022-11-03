package shape

import (
	"math"

	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/svg"
)

type shapePackage struct {
	*baseShape
}

func NewPackage(box *geo.Box) Shape {
	return shapePackage{
		baseShape: &baseShape{
			Type: PACKAGE_TYPE,
			Box:  box,
		},
	}
}

func packagePath(box *geo.Box) *svg.SvgPathContext {
	const MIN_TOP_HEIGHT = 34
	const MAX_TOP_HEIGHT = 55
	const MIN_TOP_WIDTH = 50
	const MAX_TOP_WIDTH = 150

	const horizontalScalar = 0.5
	topWidth := box.Width * horizontalScalar
	if box.Width >= 2*MIN_TOP_WIDTH {
		topWidth = math.Min(MAX_TOP_WIDTH, math.Max(MIN_TOP_WIDTH, topWidth))
	}
	const verticalScalar = 0.2
	topHeight := box.Height * verticalScalar
	if box.Height >= 2*MIN_TOP_HEIGHT {
		topHeight = math.Min(MAX_TOP_HEIGHT, math.Max(MIN_TOP_HEIGHT, topHeight))
	}

	pc := svg.NewSVGPathContext(box.TopLeft, 1, 1)
	pc.StartAt(pc.Absolute(0, 0))
	pc.L(false, topWidth, 0)
	pc.L(false, topWidth, topHeight)
	pc.L(false, box.Width, topHeight)
	pc.L(false, box.Width, box.Height)
	pc.L(false, 0, box.Height)
	pc.Z()
	return pc
}

func (s shapePackage) Perimeter() []geo.Intersectable {
	return packagePath(s.Box).Path
}

func (s shapePackage) GetSVGPathData() []string {
	return []string{
		packagePath(s.Box).PathData(),
	}
}
