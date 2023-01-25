package shape

import (
	"math"

	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/svg"
)

type shapePage struct {
	*baseShape
}

const (
	// TODO: cleanup
	pageCornerWidth  = 20.8164
	pageCornerHeight = 20.348
)

func NewPage(box *geo.Box) Shape {
	return shapePage{
		baseShape: &baseShape{
			Type: PAGE_TYPE,
			Box:  box,
		},
	}
}

func (s shapePage) GetInnerBox() *geo.Box {
	// Note: for simplicity this assumes shape padding is greater than pageCornerSize
	width := s.Box.Width
	// consider right hand side occupied by corner for short pages
	if s.Box.Height < 3*pageCornerHeight {
		width -= pageCornerWidth
	}
	return geo.NewBox(s.Box.TopLeft.Copy(), width, s.Box.Height)
}

func pageOuterPath(box *geo.Box) *svg.SvgPathContext {
	// TODO: cleanup
	pc := svg.NewSVGPathContext(box.TopLeft, 1., 1.)
	pc.StartAt(pc.Absolute(0.5, 0))
	pc.H(false, box.Width-20.8164)
	pc.C(false, box.Width-19.6456, 0.0, box.Width-18.521, 0.456297, box.Width-17.6811, 1.27202)
	pc.L(false, box.Width-1.3647, 17.12)
	pc.C(false, box.Width-0.4923, 17.9674, box.Width, 19.1318, box.Width, 20.348)
	pc.V(false, box.Height-0.5)
	pc.C(false, box.Width, box.Height-0.2239, box.Width-0.2239, box.Height, box.Width-0.5, box.Height)

	pc.H(false, 0.499999)
	pc.C(false, 0.223857, box.Height, 0, box.Height-0.2239, 0, box.Height-0.5)
	pc.V(false, 0.499999)
	pc.C(false, 0, 0.223857, 0.223857, 0, 0.5, 0)
	pc.Z()
	return pc
}

func pageInnerPath(box *geo.Box) *svg.SvgPathContext {
	pc := svg.NewSVGPathContext(box.TopLeft, 1., 1.)
	pc.StartAt(pc.Absolute(box.Width-1.08197, box.Height))
	pc.H(false, 1.08196)
	pc.C(true, -0.64918, 0, -1.08196, -0.43287, -1.08196, -1.08219)
	pc.V(false, 1.08219)
	pc.C(true, 0, -0.64931, 0.43278, -1.08219, 1.08196, -1.08219)

	pc.H(true, box.Width-22.72132)
	pc.C(true, 0.64918, 0, 1.08196, 0.43287, 1.08196, 1.08219)
	pc.V(true, 17.09863)
	pc.C(true, 0, 1.29863, 0.86557, 2.38082, 2.38032, 2.38082)
	pc.H(false, box.Width-1.08197)
	pc.C(true, .64918, 0, 1.08196, 0.43287, 1.08196, 1.08196)
	pc.V(false, box.Height-1.0822)
	pc.C(false, box.Width-1.0, box.Height-0.43288, box.Width-0.43279, box.Height, box.Width-1.08197, box.Height)
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

func (s shapePage) GetDimensionsToFit(width, height, paddingX, paddingY float64) (float64, float64) {
	totalWidth := width + paddingX
	totalHeight := height + paddingY
	// add space for corner with short pages
	if totalHeight < 3*pageCornerHeight {
		totalWidth += pageCornerWidth
	}
	totalWidth = math.Max(totalWidth, 2*pageCornerWidth)
	totalHeight = math.Max(totalHeight, pageCornerHeight)
	return math.Ceil(totalWidth), math.Ceil(totalHeight)
}

func (s shapePage) GetDefaultPadding() (paddingX, paddingY float64) {
	return defaultPadding, pageCornerHeight + defaultPadding
}
