package d2graph

import "oss.terrastruct.com/d2/d2target"

func (obj *Object) IsCycleDiagram() bool {
	return obj != nil && obj.Shape.Value == d2target.ShapeCycleDiagram
}