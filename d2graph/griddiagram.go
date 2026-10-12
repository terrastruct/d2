package d2graph

import "fmt"

const (
	// MaxGridDimension bounds each axis of an explicitly dimensioned grid. In
	// addition to layout work, this limits the coordinate span contributed by
	// empty cells before output-specific resource limits run.
	MaxGridDimension = 10_000
	// MaxGridCells bounds the capacity of an explicitly dimensioned grid. Grid
	// layout retains empty cells so that diagrams can add objects in later boards,
	// which makes rows*columns the relevant amount of user-controlled work even
	// when the current board contains few objects.
	MaxGridCells = 1_000_000
)

// GridCapacity returns rows*columns when both axes and their product are
// bounded. The division check keeps the multiplication from overflowing an int.
func GridCapacity(rows, columns int) (int, error) {
	if rows <= 0 || columns <= 0 {
		return 0, fmt.Errorf("grid dimensions must be positive, got %d by %d", rows, columns)
	}
	if rows > MaxGridDimension || columns > MaxGridDimension {
		return 0, fmt.Errorf("grid dimensions %d by %d exceed the maximum of %d rows or columns", rows, columns, MaxGridDimension)
	}
	if rows > MaxGridCells/columns {
		return 0, fmt.Errorf("grid dimensions %d by %d exceed the limit of %d cells", rows, columns, MaxGridCells)
	}
	return rows * columns, nil
}

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
