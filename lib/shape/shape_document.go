package shape

import (
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/svg"
)

type shapeDocument struct {
	*baseShape
}

const (
	// the shape is taller than where the bottom of the path ends
	docPathHeight      = 18.925
	docPathInnerBottom = 14
	docPathBottom      = 16.3
)

func NewDocument(box *geo.Box) Shape {
	return shapeDocument{
		baseShape: &baseShape{
			Type: DOCUMENT_TYPE,
			Box:  box,
		},
	}
}

func (s shapeDocument) GetInnerBox() *geo.Box {
	height := s.Box.Height * docPathInnerBottom / docPathHeight
	return geo.NewBox(s.Box.TopLeft.Copy(), s.Box.Width, height)
}

func documentPath(box *geo.Box) *svg.SvgPathContext {
	pc := svg.NewSVGPathContext(box.TopLeft, box.Width, box.Height)
	pc.StartAt(pc.Absolute(0, docPathBottom/docPathHeight))
	pc.L(false, 0, 0)
	pc.L(false, 1, 0)
	pc.L(false, 1, docPathBottom/docPathHeight)
	pc.C(false, 5/6.0, 12.8/docPathHeight, 2/3.0, 12.8/docPathHeight, 1/2.0, docPathBottom/docPathHeight)
	pc.C(false, 1/3.0, 19.8/docPathHeight, 1/6.0, 19.8/docPathHeight, 0, docPathBottom/docPathHeight)
	pc.Z()
	return pc
}

func (s shapeDocument) Perimeter() []geo.Intersectable {
	return documentPath(s.Box).Path
}

func (s shapeDocument) GetSVGPathData() []string {
	return []string{
		documentPath(s.Box).PathData(),
		// debugging
		boxPath(s.GetInnerBox()).PathData(),
	}
}

func (s shapeDocument) GetDimensionsToFit(width, height, padding float64) (float64, float64) {
	baseHeight := (height + padding*2) * docPathHeight / docPathInnerBottom
	return width + padding*2, baseHeight
}
