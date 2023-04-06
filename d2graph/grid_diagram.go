package d2graph

func (obj *Object) IsGridDiagram() bool {
	return obj != nil && obj.Attributes != nil &&
		(obj.Attributes.GridRows != nil || obj.Attributes.GridColumns != nil)
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
