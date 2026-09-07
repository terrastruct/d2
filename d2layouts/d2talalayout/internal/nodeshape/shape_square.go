package nodeshape

import (
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
	"github.com/d2lang/d2/lib/shape"
)

type shapeSquare struct {
	shape.Shape
}

func (s shapeSquare) LabelPositionPreferences(preferenceSet LabelTier) map[label.Position]struct{} {
	switch preferenceSet {
	case Good:
		return map[label.Position]struct{}{
			label.OutsideTopCenter:    {},
			label.InsideTopCenter:     {},
			label.InsideMiddleCenter:  {},
			label.InsideBottomCenter:  {},
			label.OutsideBottomCenter: {},
			label.InsideTopLeft:       {},
			label.InsideTopRight:      {},
			label.InsideBottomLeft:    {},
			label.InsideBottomRight:   {},
		}
	case OK:
		return map[label.Position]struct{}{
			label.InsideMiddleLeft:   {},
			label.InsideMiddleRight:  {},
			label.OutsideTopLeft:     {},
			label.OutsideTopRight:    {},
			label.OutsideBottomLeft:  {},
			label.OutsideBottomRight: {},
		}
	case Unideal:
		return map[label.Position]struct{}{
			label.OutsideLeftTop:     {},
			label.OutsideRightTop:    {},
			label.OutsideLeftMiddle:  {},
			label.OutsideRightMiddle: {},
			label.OutsideLeftBottom:  {},
			label.OutsideRightBottom: {},
		}
	case Bad:
		return map[label.Position]struct{}{}
	}
	return map[label.Position]struct{}{}
}

func (s shapeSquare) SnapPointPercentages() [][]*geo.RelativePoint {
	return [][]*geo.RelativePoint{
		// Top
		{
			geo.NewRelativePoint(0.25, 0),
			geo.NewRelativePoint(0.5, 0),
			geo.NewRelativePoint(0.75, 0),
		},
		// Left
		{
			geo.NewRelativePoint(0, 0.25),
			geo.NewRelativePoint(0, 0.5),
			geo.NewRelativePoint(0, 0.75),
		},
		// Bottom
		{
			geo.NewRelativePoint(0.25, 1),
			geo.NewRelativePoint(0.5, 1),
			geo.NewRelativePoint(0.75, 1),
		},
		// Right
		{
			geo.NewRelativePoint(1, 0.25),
			geo.NewRelativePoint(1, 0.5),
			geo.NewRelativePoint(1, 0.75),
		},
	}
}

func (s shapeSquare) PortIndices(orientation geo.Orientation) []int {
	switch orientation {
	case geo.Top:
		return []int{0, 1, 2}
	case geo.Left:
		return []int{3, 4, 5}
	case geo.Bottom:
		return []int{6, 7, 8}
	case geo.Right:
		return []int{9, 10, 11}
	case geo.TopLeft:
		return []int{0, 1, 2, 3, 4, 5}
	case geo.TopRight:
		return []int{0, 1, 2, 9, 10, 11}
	case geo.BottomLeft:
		return []int{6, 7, 8, 3, 4, 5}
	case geo.BottomRight:
		return []int{6, 7, 8, 9, 10, 11}
	default:
		return []int{}
	}
}

func (s shapeSquare) MirroredPortIndices() map[int]int {
	return map[int]int{
		// Top and bottom
		1: 7,
		7: 1,
		// Left and right
		4:  10,
		10: 4,
	}
}

func (s shapeSquare) CenterPortIndices() []int {
	return []int{1, 4, 7, 10}
}

func (s shapeSquare) CenterPortIndex(o geo.Orientation) int {
	centerPorts := s.CenterPortIndices()
	var centerPortsIndex int
	switch o {
	case geo.Top:
		centerPortsIndex = 0
	case geo.Left:
		centerPortsIndex = 1
	case geo.Bottom:
		centerPortsIndex = 2
	case geo.Right:
		centerPortsIndex = 3
	default:
		return -1
	}

	return centerPorts[centerPortsIndex]
}
