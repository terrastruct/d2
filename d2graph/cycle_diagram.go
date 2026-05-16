package d2graph

import "oss.terrastruct.com/d2/d2target"

func (obj *Object) IsCycleDiagram() bool {
	return obj != nil && obj.Shape.Value == d2target.ShapeCycleDiagram
}

func (obj *Object) OuterCycleDiagram() *Object {
	for obj != nil {
		obj = obj.Parent
		if obj.IsCycleDiagram() {
			return obj
		}
	}
	return nil
}
