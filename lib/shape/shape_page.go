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

const PAGE_WIDTH = 66.
const PAGE_HEIGHT = 79.

func pageOuterPath(box *geo.Box) *svg.SvgPathContext {
	pc := svg.NewSVGPathContext(box.TopLeft, 1., 1.)
	baseX := box.Width - PAGE_WIDTH
	baseY := box.Height - PAGE_HEIGHT
	pc.StartAt(pc.Absolute(0.5, 0))
	pc.H(false, baseX+45.1836) // = width-(66+45.1836)
	pc.C(false, baseX+46.3544, 0.0, baseX+47.479, 0.456297, baseX+48.3189, 1.27202)
	pc.L(false, baseX+64.6353, 17.12)
	pc.C(false, baseX+65.5077, 17.9674, baseX+66., 19.1318, baseX+66., 20.348)
	// baseY is not needed above because the coordinates start at 0
	pc.V(false, baseY+78.5)
	pc.C(false, baseX+66.0, baseY+78.7761, baseX+65.7761, baseY+79.0, baseX+65.5, baseY+79.0)

	pc.H(false, .499999)
	pc.C(false, 0.223857, baseY+79.0, 0.0, baseY+78.7761, 0.0, baseY+78.5)
	pc.V(false, 0.499999)
	pc.C(false, 0.0, 0.223857, 0.223857, 0.0, 0.5, 0.0)
	pc.Z()
	return pc
}

func pageInnerPath(box *geo.Box) *svg.SvgPathContext {
	baseX := box.Width - PAGE_WIDTH
	baseY := box.Height - PAGE_HEIGHT

	pc := svg.NewSVGPathContext(box.TopLeft, 1., 1.)
	pc.StartAt(pc.Absolute(baseX+64.91803, baseY+79.))
	pc.H(false, 1.08196)
	pc.C(true, -0.64918, 0, -1.08196, -0.43287, -1.08196, -1.08219)
	pc.V(false, 1.08219)
	pc.C(true, 0, -0.64931, 0.43278, -1.08219, 1.08196, -1.08219)

	pc.H(true, baseX+43.27868)
	pc.C(true, 0.64918, 0, 1.08196, 0.43287, 1.08196, 1.08219)
	pc.V(true, 17.09863)
	pc.C(true, 0, 1.29863, 0.86557, 2.38082, 2.38032, 2.38082)
	pc.H(false, baseX+64.91803)
	pc.C(true, .64918, 0, 1.08196, 0.43287, 1.08196, 1.08196)
	pc.V(false, baseY+77.91780)
	pc.C(false, baseX+64.99999, baseY+78.56712, baseX+65.56721, baseY+79, baseX+64.91803, baseY+79)
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
