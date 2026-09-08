package nodeshape

import (
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
	"github.com/d2lang/d2/lib/shape"
)

type shapeDiamond struct {
	shape.Shape
}

func (s shapeDiamond) LabelPositionPreferences(preferenceSet LabelTier) map[label.Position]struct{} {
	switch preferenceSet {
	case Good:
		return map[label.Position]struct{}{
			label.InsideMiddleCenter: {},
		}
	case OK:
		return map[label.Position]struct{}{
			label.OutsideTopCenter:    {},
			label.OutsideBottomCenter: {},
			label.InsideMiddleLeft:    {},
			label.InsideMiddleRight:   {},
			label.OutsideLeftMiddle:   {},
			label.OutsideRightMiddle:  {},
			label.InsideBottomCenter:  {},
			label.InsideTopCenter:     {},
		}
	case Unideal:
		return map[label.Position]struct{}{
			label.InsideTopLeft:     {},
			label.InsideTopRight:    {},
			label.InsideBottomLeft:  {},
			label.InsideBottomRight: {},
		}
	case Bad:
		return map[label.Position]struct{}{
			label.OutsideTopLeft:     {},
			label.OutsideTopRight:    {},
			label.OutsideLeftTop:     {},
			label.OutsideRightTop:    {},
			label.OutsideBottomLeft:  {},
			label.OutsideBottomRight: {},
			label.OutsideLeftBottom:  {},
			label.OutsideRightBottom: {},
		}
	}
	return map[label.Position]struct{}{}
}

func (s shapeDiamond) SnapPointPercentages() [][]*geo.RelativePoint {
	return [][]*geo.RelativePoint{
		// Top
		{
			geo.NewRelativePoint(0.5, 0),
		},
		// Left
		{
			geo.NewRelativePoint(0, 0.5),
		},
		// Bottom
		{
			geo.NewRelativePoint(0.5, 1),
		},
		// Right
		{
			geo.NewRelativePoint(1, 0.5),
		},
	}
}

func (s shapeDiamond) PortIndices(orientation geo.Orientation) []int {
	switch orientation {
	case geo.Top:
		return []int{0}
	case geo.Left:
		return []int{1}
	case geo.Bottom:
		return []int{2}
	case geo.Right:
		return []int{3}
	case geo.TopLeft:
		return []int{0, 1}
	case geo.TopRight:
		return []int{0, 3}
	case geo.BottomLeft:
		return []int{2, 1}
	case geo.BottomRight:
		return []int{2, 3}
	default:
		return []int{}
	}
}

func (s shapeDiamond) MirroredPortIndices() map[int]int {
	return map[int]int{
		// Top and bottom
		0: 2,
		2: 0,
		// Left and right
		1: 3,
		3: 1,
	}
}

func (s shapeDiamond) CenterPortIndices() []int {
	return []int{0, 1, 2, 3}
}

func (s shapeDiamond) CenterPortIndex(o geo.Orientation) int {
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
