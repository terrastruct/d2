package d2graph

func (obj *Object) IsGridDiagram() bool {
	return obj != nil &&
		(obj.GridRows != nil || obj.GridColumns != nil)
}

func (obj *Object) ClosestGridDiagram() *Object {
	if obj == nil {
		return nil
	}
	if obj.IsGridDiagram() {
		return obj
	}
	return obj.Parent.ClosestGridDiagram()
}
