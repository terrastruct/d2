package nodeshape

import (
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
	"github.com/d2lang/d2/lib/shape"
)

type shapeStep struct {
	shape.Shape
}

func (s shapeStep) LabelPositionPreferences(preferenceSet LabelTier) map[label.Position]struct{} {
	switch preferenceSet {
	case Good:
		return map[label.Position]struct{}{
			label.InsideMiddleCenter: {},
		}
	case OK:
		return map[label.Position]struct{}{
			label.InsideTopCenter:     {},
			label.InsideTopRight:      {},
			label.InsideBottomRight:   {},
			label.InsideMiddleLeft:    {},
			label.InsideMiddleRight:   {},
			label.InsideBottomCenter:  {},
			label.OutsideTopLeft:      {},
			label.OutsideTopCenter:    {},
			label.OutsideBottomLeft:   {},
			label.OutsideBottomCenter: {},
		}
	case Unideal:
		return map[label.Position]struct{}{
			label.OutsideRightMiddle: {},
			label.OutsideLeftTop:     {},
			label.OutsideLeftBottom:  {},
			label.InsideTopLeft:      {},
			label.InsideBottomLeft:   {},
		}
	case Bad:
		return map[label.Position]struct{}{
			label.OutsideTopRight:    {},
			label.OutsideLeftMiddle:  {},
			label.OutsideRightTop:    {},
			label.OutsideRightBottom: {},
			label.OutsideBottomRight: {},
		}
	}
	return map[label.Position]struct{}{}
}

func (s shapeStep) SnapPointPercentages() [][]*geo.RelativePoint {
	width := s.GetBox().Width
	wedgeWidth := shape.STEP_WEDGE_WIDTH
	if width < wedgeWidth {
		wedgeWidth = width / 2.0
	}

	return [][]*geo.RelativePoint{
		// Top
		{
			geo.NewRelativePoint(((width-wedgeWidth)/width)*0.25, 0),
			geo.NewRelativePoint(((width-wedgeWidth)/width)*0.5, 0),
			geo.NewRelativePoint(((width-wedgeWidth)/width)*0.75, 0),
		},
		// Left
		{
			geo.NewRelativePoint(wedgeWidth/width/2, 0.25),
			geo.NewRelativePoint(wedgeWidth/width, 0.5),
			geo.NewRelativePoint(wedgeWidth/width/2, 0.75),
		},
		// Bottom
		{
			geo.NewRelativePoint(((width-wedgeWidth)/width)*0.25, 1),
			geo.NewRelativePoint(((width-wedgeWidth)/width)*0.5, 1),
			geo.NewRelativePoint(((width-wedgeWidth)/width)*0.75, 1),
		},
		// Right
		{
			geo.NewRelativePoint(1, 0.5),
		},
	}
}

func (s shapeStep) PortIndices(orientation geo.Orientation) []int {
	switch orientation {
	case geo.Top:
		return []int{0, 1, 2}
	case geo.Left:
		return []int{3, 4, 5}
	case geo.Bottom:
		return []int{6, 7, 8}
	case geo.Right:
		return []int{9}
	case geo.TopLeft:
		return []int{0, 1, 2, 3, 4, 5}
	case geo.TopRight:
		return []int{0, 1, 2, 9}
	case geo.BottomLeft:
		return []int{6, 7, 8, 3, 4, 5}
	case geo.BottomRight:
		return []int{6, 7, 8, 9}
	default:
		return []int{}
	}
}

func (s shapeStep) MirroredPortIndices() map[int]int {
	return map[int]int{
		// Top and bottom
		1: 7,
		7: 1,
		// Left and right
		4: 9,
		9: 4,
	}
}

func (s shapeStep) CenterPortIndices() []int {
	return []int{1, 4, 7, 9}
}

func (s shapeStep) CenterPortIndex(o geo.Orientation) int {
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
