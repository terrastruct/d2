package shape

import (
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/svg"
)

type shapeCylinder struct {
	*baseShape
}

const (
	defaultArcDepth = 24.
)

func NewCylinder(box *geo.Box) Shape {
	return shapeCylinder{
		baseShape: &baseShape{
			Type: CYLINDER_TYPE,
			Box:  box,
		},
	}
}

func getArcHeight(box *geo.Box) float64 {
	arcHeight := defaultArcDepth
	// Note: box height should always be larger than 3*default
	// this just handles after collapsing into an oval
	if box.Height < arcHeight*2 {
		arcHeight = box.Height / 2.0
	}
	return arcHeight
}

func (s shapeCylinder) GetInnerBox() *geo.Box {
	height := s.Box.Height
	tl := s.Box.TopLeft.Copy()
	arc := getArcHeight(s.Box)
	height -= 3 * arc
	tl.Y += 2 * arc
	return geo.NewBox(tl, s.Box.Width, height)
}

func cylinderOuterPath(box *geo.Box) *svg.SvgPathContext {
	arcHeight := getArcHeight(box)
	multiplier := 0.45
	pc := svg.NewSVGPathContext(box.TopLeft, 1, 1)
	pc.StartAt(pc.Absolute(0, arcHeight))
	pc.C(false, 0, 0, box.Width*multiplier, 0, box.Width/2, 0)
	pc.C(false, box.Width-box.Width*multiplier, 0, box.Width, 0, box.Width, arcHeight)
	pc.V(true, box.Height-arcHeight*2)
	pc.C(false, box.Width, box.Height, box.Width-box.Width*multiplier, box.Height, box.Width/2, box.Height)
	pc.C(false, box.Width*multiplier, box.Height, 0, box.Height, 0, box.Height-arcHeight)
	pc.V(true, -(box.Height - arcHeight*2))
	pc.Z()
	return pc
}

func cylinderInnerPath(box *geo.Box) *svg.SvgPathContext {
	arcHeight := getArcHeight(box)
	multiplier := 0.45
	pc := svg.NewSVGPathContext(box.TopLeft, 1, 1)
	pc.StartAt(pc.Absolute(0, arcHeight))
	pc.C(false, 0, arcHeight*2, box.Width*multiplier, arcHeight*2, box.Width/2, arcHeight*2)
	pc.C(false, box.Width-box.Width*multiplier, arcHeight*2, box.Width, arcHeight*2, box.Width, arcHeight)
	return pc
}

func (s shapeCylinder) Perimeter() []geo.Intersectable {
	return cylinderOuterPath(s.Box).Path
}

func (s shapeCylinder) GetSVGPathData() []string {
	return []string{
		cylinderOuterPath(s.Box).PathData(),
		cylinderInnerPath(s.Box).PathData(),
	}
}

func (s shapeCylinder) GetDimensionsToFit(width, height, paddingX, paddingY float64) (float64, float64) {
	// 2 arcs top, height + padding, 1 arc bottom
	totalHeight := height + paddingY + 3*defaultArcDepth
	return width + paddingX, totalHeight
}

func (s shapeCylinder) GetDefaultPadding() (paddingX, paddingY float64) {
	return defaultPadding, defaultPadding / 2
}
