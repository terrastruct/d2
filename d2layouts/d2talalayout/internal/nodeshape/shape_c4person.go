package nodeshape

import (
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
	"github.com/d2lang/d2/lib/shape"
)

type shapeC4Person struct {
	shape.Shape
}

func (s shapeC4Person) LabelPositionPreferences(preferenceSet LabelTier) map[label.Position]struct{} {
	switch preferenceSet {
	case Good:
		return map[label.Position]struct{}{
			label.InsideMiddleCenter: {},
		}
	case OK:
		return map[label.Position]struct{}{
			label.InsideTopCenter:    {},
			label.InsideBottomCenter: {},
		}
	case Unideal:
		return map[label.Position]struct{}{
			label.InsideBottomLeft:  {},
			label.InsideBottomRight: {},
			label.InsideTopLeft:     {},
			label.InsideTopRight:    {},
			label.InsideMiddleLeft:  {},
			label.InsideMiddleRight: {},
		}
	case Bad:
		return map[label.Position]struct{}{
			label.OutsideTopCenter:    {},
			label.OutsideBottomCenter: {},
			label.OutsideBottomLeft:   {},
			label.OutsideBottomRight:  {},
			label.OutsideLeftMiddle:   {},
			label.OutsideLeftBottom:   {},
			label.OutsideRightMiddle:  {},
			label.OutsideRightBottom:  {},
			label.OutsideRightTop:     {},
			label.OutsideLeftTop:      {},
			label.OutsideTopLeft:      {},
			label.OutsideTopRight:     {},
		}
	}
	return map[label.Position]struct{}{}
}

func (s shapeC4Person) SnapPointPercentages() [][]*geo.RelativePoint {
	return [][]*geo.RelativePoint{
		// Top (1 port at center of head)
		{
			geo.NewRelativePoint(0.5, 0),
		},
		// Left (3 ports on body only)
		{
			geo.NewRelativePoint(0, 0.45), // Just below where body starts
			geo.NewRelativePoint(0, 0.65), // Middle of body
			geo.NewRelativePoint(0, 0.85), // Lower part of body
		},
		// Bottom (3 ports)
		{
			geo.NewRelativePoint(0.25, 1),
			geo.NewRelativePoint(0.5, 1),
			geo.NewRelativePoint(0.75, 1),
		},
		// Right (3 ports on body only)
		{
			geo.NewRelativePoint(1, 0.45), // Just below where body starts
			geo.NewRelativePoint(1, 0.65), // Middle of body
			geo.NewRelativePoint(1, 0.85), // Lower part of body
		},
	}
}

func (s shapeC4Person) PortIndices(orientation geo.Orientation) []int {
	switch orientation {
	case geo.Top:
		return []int{0}
	case geo.Left:
		return []int{1, 2, 3}
	case geo.Bottom:
		return []int{4, 5, 6}
	case geo.Right:
		return []int{7, 8, 9}
	case geo.TopLeft:
		return []int{0, 1, 2, 3}
	case geo.TopRight:
		return []int{0, 7, 8, 9}
	case geo.BottomLeft:
		return []int{4, 5, 6, 1, 2, 3}
	case geo.BottomRight:
		return []int{4, 5, 6, 7, 8, 9}
	default:
		return []int{}
	}
}

func (s shapeC4Person) MirroredPortIndices() map[int]int {
	return map[int]int{
		// Top and bottom
		1: 6,
		6: 1,
		// Left and right
		3: 8,
		8: 3,
	}
}

func (s shapeC4Person) CenterPortIndices() []int {
	return []int{0, 2, 5, 7}
}

func (s shapeC4Person) CenterPortIndex(o geo.Orientation) int {
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
