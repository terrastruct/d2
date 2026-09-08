package nodeshape

import (
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
	"github.com/d2lang/d2/lib/shape"
)

type shapePerson struct {
	shape.Shape
}

func (s shapePerson) LabelPositionPreferences(preferenceSet LabelTier) map[label.Position]struct{} {
	switch preferenceSet {
	case Good:
		return map[label.Position]struct{}{
			label.OutsideBottomCenter: {},
		}
	case OK:
		return map[label.Position]struct{}{
			label.OutsideTopCenter:   {},
			label.OutsideBottomLeft:  {},
			label.OutsideBottomRight: {},
		}
	case Unideal:
		return map[label.Position]struct{}{
			label.InsideTopCenter:    {},
			label.InsideMiddleCenter: {},
			label.InsideBottomCenter: {},
			label.InsideBottomLeft:   {},
			label.InsideBottomRight:  {},
			label.OutsideLeftMiddle:  {},
			label.OutsideLeftBottom:  {},
			label.OutsideRightMiddle: {},
			label.OutsideRightBottom: {},
		}
	case Bad:
		return map[label.Position]struct{}{
			label.InsideTopLeft:     {},
			label.InsideTopRight:    {},
			label.InsideMiddleLeft:  {},
			label.InsideMiddleRight: {},
			label.OutsideRightTop:   {},
			label.OutsideLeftTop:    {},
			label.OutsideTopLeft:    {},
			label.OutsideTopRight:   {},
		}
	}
	return map[label.Position]struct{}{}
}

func (s shapePerson) SnapPointPercentages() [][]*geo.RelativePoint {
	return [][]*geo.RelativePoint{
		// Top
		{
			geo.NewRelativePoint(0.21, 0.122),
			geo.NewRelativePoint(0.5, 0),
			geo.NewRelativePoint(0.79, 0.122),
		},
		// Left
		{
			geo.NewRelativePoint(0.135, 0.35),
			geo.NewRelativePoint(0.08, 0.75),
		},
		// Bottom
		{
			geo.NewRelativePoint(0.25, 0.985),
			geo.NewRelativePoint(0.5, 1),
			geo.NewRelativePoint(0.75, 0.985),
		},
		// Right
		{
			geo.NewRelativePoint(0.865, 0.35),
			geo.NewRelativePoint(0.92, 0.75),
		},
	}
}

func (s shapePerson) PortIndices(orientation geo.Orientation) []int {
	switch orientation {
	case geo.Top:
		return []int{0, 1, 2}
	case geo.Left:
		return []int{3, 4}
	case geo.Bottom:
		return []int{5, 6, 7}
	case geo.Right:
		return []int{8, 9}
	case geo.TopLeft:
		return []int{0, 1, 2, 3, 4}
	case geo.TopRight:
		return []int{0, 1, 2, 8, 9}
	case geo.BottomLeft:
		return []int{5, 6, 7, 3, 4}
	case geo.BottomRight:
		return []int{5, 6, 7, 8, 9}
	default:
		return []int{}
	}
}

func (s shapePerson) MirroredPortIndices() map[int]int {
	return map[int]int{
		// Top and bottom
		1: 6,
		6: 1,
		// Left and right
		3: 8,
		8: 3,
	}
}

func (s shapePerson) CenterPortIndices() []int {
	return []int{1, 3, 6, 8}
}

func (s shapePerson) CenterPortIndex(o geo.Orientation) int {
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
