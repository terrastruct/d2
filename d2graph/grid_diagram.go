package d2graph

func (obj *Object) IsGridDiagram() bool {
	return obj != nil && obj.Attributes != nil &&
		(obj.Attributes.Rows != nil || obj.Attributes.Columns != nil)
}

func (obj *Object) ClosestGridDiagram() *Object {
	if obj.Parent == nil {
		return nil
	}
	if obj.Parent.IsGridDiagram() {
		return obj.Parent
	}
	return obj.Parent.ClosestGridDiagram()
}