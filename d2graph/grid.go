package d2graph

func (obj *Object) IsGrid() bool {
	return obj != nil && obj.Attributes != nil &&
		(obj.Attributes.Rows != nil || obj.Attributes.Columns != nil)
}
