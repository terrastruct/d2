package d2cycle

import (
	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/lib/geo"
)

type cycleDiagram struct {
	objects []*d2graph.Object
	edges   []*d2graph.Edge

	width  float64
	height float64
}

func newCycleDiagram(root *d2graph.Object) *cycleDiagram {
	cd := &cycleDiagram{
		objects: root.ChildrenArray,
	}

	for _, o := range cd.objects {
		o.TopLeft = geo.NewPoint(0, 0)
	}

	return cd
}

func (cd *cycleDiagram) shift(dx, dy float64) {
	for _, obj := range cd.objects {
		obj.MoveWithDescendants(dx, dy)
	}
	for _, e := range cd.edges {
		e.Move(dx, dy)
	}
}
