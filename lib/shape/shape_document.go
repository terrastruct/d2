package shape

import (
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/svg"
)

type shapeDocument struct {
	*baseShape
}

func NewDocument(box *geo.Box) Shape {
	return shapeDocument{
		baseShape: &baseShape{
			Type: DOCUMENT_TYPE,
			Box:  box,
		},
	}
}

func documentPath(box *geo.Box) *svg.SvgPathContext {
	pathHeight := 18.925
	pc := svg.NewSVGPathContext(box.TopLeft, box.Width, box.Height)
	pc.StartAt(pc.Absolute(0, 16.3/pathHeight))
	pc.L(false, 0, 0)
	pc.L(false, 1, 0)
	pc.L(false, 1, 16.3/pathHeight)
	pc.C(false, 5/6.0, 12.8/pathHeight, 2/3.0, 12.8/pathHeight, 1/2.0, 16.3/pathHeight)
	pc.C(false, 1/3.0, 19.8/pathHeight, 1/6.0, 19.8/pathHeight, 0, 16.3/pathHeight)
	pc.Z()
	return pc
}

func (s shapeDocument) Perimeter() []geo.Intersectable {
	return documentPath(s.Box).Path
}

func (s shapeDocument) GetSVGPathData() []string {
	return []string{
		documentPath(s.Box).PathData(),
	}
}
