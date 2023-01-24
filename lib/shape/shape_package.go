package shape

import (
	"math"

	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/svg"
)

type shapePackage struct {
	*baseShape
}

const (
	packageTopMinHeight     = 34.
	packageTopMaxHeight     = 55.
	packageTopMinWidth      = 50.
	packageTopMaxWidth      = 150.
	packageHorizontalScalar = 0.5
	packageVerticalScalar   = 0.2
)

func NewPackage(box *geo.Box) Shape {
	return shapePackage{
		baseShape: &baseShape{
			Type: PACKAGE_TYPE,
			Box:  box,
		},
	}
}

func (s shapePackage) GetInnerBox() *geo.Box {
	tl := s.Box.TopLeft.Copy()
	height := s.Box.Height

	_, topHeight := getTopDimensions(s.Box)
	tl.Y += topHeight
	height -= topHeight
	return geo.NewBox(tl, s.Box.Width, height)
}

func getTopDimensions(box *geo.Box) (width, height float64) {
	width = box.Width * packageHorizontalScalar
	if box.Width >= 2*packageTopMinWidth {
		width = math.Min(packageTopMaxWidth, math.Max(packageTopMinWidth, width))
	}
	height = math.Min(packageTopMaxHeight, box.Height*packageVerticalScalar)
	return width, height
}

func packagePath(box *geo.Box) *svg.SvgPathContext {
	topWidth, topHeight := getTopDimensions(box)

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

func (s shapePackage) GetDimensionsToFit(width, height, paddingX, paddingY float64) (float64, float64) {
	innerHeight := height + paddingY
	// We want to compute what the topHeight will be to add to inner height;
	// topHeight=(verticalScalar * totalHeight) and totalHeight=(topHeight + innerHeight)
	// so solving for topHeight we get: topHeight=innerHeight * (verticalScalar/(1-verticalScalar))
	topHeight := innerHeight * packageVerticalScalar / (1. - packageVerticalScalar)
	totalHeight := innerHeight + math.Min(topHeight, packageTopMaxHeight)

	return width + paddingX, totalHeight
}

func (s shapePackage) GetDefaultPadding() (paddingX, paddingY float64) {
	return defaultPadding, .8 * defaultPadding
}
