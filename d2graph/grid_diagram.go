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

func (obj *Object) ClosestGridCell() *Object {
	if obj == nil {
		return nil
	}
	// grid cells can be a nested grid diagram
	if obj.Parent.IsGridDiagram() {
		return obj
	}
	return obj.Parent.ClosestGridCell()
}

// TopGridDiagram returns the least nested (outermost) grid diagram
func (obj *Object) TopGridDiagram() *Object {
	if obj == nil {
		return nil
	}
	var gd *Object
	if obj.IsGridDiagram() {
		gd = obj
	}
	curr := obj.Parent
	for curr != nil {
		if curr.IsGridDiagram() {
			gd = curr
		}
		curr = curr.Parent
	}
	return gd
}
