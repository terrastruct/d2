package d2graph

func (obj *Object) IsGrid() bool {
	return obj != nil && obj.Attributes != nil &&
		(obj.Attributes.Rows != nil || obj.Attributes.Columns != nil)
}

func (obj *Object) ClosestGrid() *Object {
	if obj.Parent == nil {
		return nil
	}
	if obj.Parent.IsGrid() {
		return obj.Parent
	}
	return obj.Parent.ClosestGrid()
}
