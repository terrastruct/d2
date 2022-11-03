package shape

import (
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/svg"
)

type shapeCylinder struct {
	*baseShape
}

func NewCylinder(box *geo.Box) Shape {
	return shapeCylinder{
		baseShape: &baseShape{
			Type: CYLINDER_TYPE,
			Box:  box,
		},
	}
}

func (s shapeCylinder) GetInnerBox() *geo.Box {
	height := s.Box.Height
	tl := s.Box.TopLeft.Copy()
	arcDepth := 24.0
	if height < arcDepth*2 {
		arcDepth = height / 2.0
	}
	height -= 3 * arcDepth
	tl.Y += 2 * arcDepth
	return geo.NewBox(tl, s.Box.Width, height)
}

func cylinderOuterPath(box *geo.Box) *svg.SvgPathContext {
	arcDepth := 24.0
	if box.Height < arcDepth*2 {
		arcDepth = box.Height / 2
	}
	multiplier := 0.45
	pc := svg.NewSVGPathContext(box.TopLeft, 1, 1)
	pc.StartAt(pc.Absolute(0, arcDepth))
	pc.C(false, 0, 0, box.Width*multiplier, 0, box.Width/2, 0)
	pc.C(false, box.Width-box.Width*multiplier, 0, box.Width, 0, box.Width, arcDepth)
	pc.V(true, box.Height-arcDepth*2)
	pc.C(false, box.Width, box.Height, box.Width-box.Width*multiplier, box.Height, box.Width/2, box.Height)
	pc.C(false, box.Width*multiplier, box.Height, 0, box.Height, 0, box.Height-arcDepth)
	pc.V(true, -(box.Height - arcDepth*2))
	pc.Z()
	return pc
}

func cylinderInnerPath(box *geo.Box) *svg.SvgPathContext {
	arcDepth := 24.0
	if box.Height < arcDepth*2 {
		arcDepth = box.Height / 2
	}
	multiplier := 0.45
	pc := svg.NewSVGPathContext(box.TopLeft, 1, 1)
	pc.StartAt(pc.Absolute(0, arcDepth))
	pc.C(false, 0, arcDepth*2, box.Width*multiplier, arcDepth*2, box.Width/2, arcDepth*2)
	pc.C(false, box.Width-box.Width*multiplier, arcDepth*2, box.Width, arcDepth*2, box.Width, arcDepth)
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
