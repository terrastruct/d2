package d2graph

func (obj *Object) IsGrid() bool {
	return obj != nil && obj.Attributes != nil && len(obj.ChildrenArray) != 0 &&
		(obj.Attributes.Rows != nil || obj.Attributes.Columns != nil)
}
