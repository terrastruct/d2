package shape

import (
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/svg"
)

type shapePage struct {
	*baseShape
}

func NewPage(box *geo.Box) Shape {
	return shapePage{
		baseShape: &baseShape{
			Type: PAGE_TYPE,
			Box:  box,
		},
	}
}

func pageOuterPath(box *geo.Box) *svg.SvgPathContext {
	pc := svg.NewSVGPathContext(box.TopLeft, box.Width/66, box.Height/79.0)
	pc.StartAt(pc.Absolute(0.5, 0))
	pc.H(false, 45.1836)
	pc.C(false, 46.3544, 0.0, 47.479, 0.456297, 48.3189, 1.27202)
	pc.L(false, 64.6353, 17.12)
	pc.C(false, 65.5077, 17.9674, 66.0, 19.1318, 66.0, 20.348)
	pc.V(false, 78.5)
	pc.C(false, 66.0, 78.7761, 65.7761, 79.0, 65.5, 79.0)
	pc.H(false, 0.499999)
	pc.C(false, 0.223857, 79.0, 0.0, 78.7761, 0.0, 78.5)
	pc.V(false, 0.499999)
	pc.C(false, 0.0, 0.223857, 0.223857, 0.0, 0.5, 0.0)
	pc.Z()
	return pc
}

func pageInnerPath(box *geo.Box) *svg.SvgPathContext {
	pc := svg.NewSVGPathContext(box.TopLeft, box.Width/30.5, box.Height/36.5)
	pc.StartAt(pc.Absolute(30, 36.5))
	pc.H(false, 0.5)
	pc.C(true, -0.3, 0, -0.5, -0.2, -0.5, -0.5)
	pc.V(false, 0.5)
	pc.C(true, 0, -0.3, 0.2, -0.5, 0.5, -0.5)
	pc.H(true, 20.3)
	pc.C(true, 0.3, 0, 0.5, 0.2, 0.5, 0.5)
	pc.V(true, 7.9)
	pc.C(true, 0, 0.6, 0.4, 1.1, 1.1, 1.1)
	pc.H(false, 30)
	pc.C(true, 0.3, 0, 0.5, 0.2, 0.5, 0.5)
	pc.V(false, 36)
	pc.C(false, 30.5, 36.3, 30.3, 36.5, 30, 36.5)
	pc.Z()
	return pc
}

func (s shapePage) Perimeter() []geo.Intersectable {
	return pageOuterPath(s.Box).Path
}

func (s shapePage) GetSVGPathData() []string {
	return []string{
		pageOuterPath(s.Box).PathData(),
		pageInnerPath(s.Box).PathData(),
	}
}
