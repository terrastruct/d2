package shape

import (
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/svg"
)

type shapeParallelogram struct {
	*baseShape
}

func NewParallelogram(box *geo.Box) Shape {
	return shapeParallelogram{
		baseShape: &baseShape{
			Type: PARALLELOGRAM_TYPE,
			Box:  box,
		},
	}
}

func parallelogramPath(box *geo.Box) *svg.SvgPathContext {
	wedgeWidth := 26.0
	if box.Width <= wedgeWidth {
		wedgeWidth = box.Width / 2.0
	}
	pc := svg.NewSVGPathContext(box.TopLeft, 1, 1)
	pc.StartAt(pc.Absolute(wedgeWidth, 0))
	pc.L(false, box.Width, 0)
	pc.L(false, box.Width-wedgeWidth, box.Height)
	pc.L(false, 0, box.Height)
	pc.L(false, 0, box.Height)
	pc.Z()
	return pc
}

func (s shapeParallelogram) Perimeter() []geo.Intersectable {
	return parallelogramPath(s.Box).Path
}

func (s shapeParallelogram) GetSVGPathData() []string {
	return []string{
		parallelogramPath(s.Box).PathData(),
	}
}
