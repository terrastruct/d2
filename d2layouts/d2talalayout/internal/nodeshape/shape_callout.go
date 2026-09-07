package nodeshape

import (
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
	"github.com/d2lang/d2/lib/shape"
)

type shapeCallout struct {
	shape.Shape
}

func (s shapeCallout) LabelPositionPreferences(preferenceSet LabelTier) map[label.Position]struct{} {
	switch preferenceSet {
	case Good:
		return map[label.Position]struct{}{
			label.InsideTopLeft:      {},
			label.InsideTopCenter:    {},
			label.InsideTopRight:     {},
			label.InsideMiddleLeft:   {},
			label.InsideMiddleCenter: {},
			label.InsideMiddleRight:  {},
			label.InsideBottomLeft:   {},
			label.InsideBottomCenter: {},
			label.InsideBottomRight:  {},
		}
	case OK:
		return map[label.Position]struct{}{
			label.OutsideTopLeft:      {},
			label.OutsideTopCenter:    {},
			label.OutsideTopRight:     {},
			label.OutsideBottomCenter: {},
		}
	case Unideal:
		return map[label.Position]struct{}{
			label.OutsideLeftTop:     {},
			label.OutsideLeftMiddle:  {},
			label.OutsideRightTop:    {},
			label.OutsideRightMiddle: {},
		}
	case Bad:
		return map[label.Position]struct{}{
			label.OutsideLeftBottom:  {},
			label.OutsideRightBottom: {},
			label.OutsideBottomLeft:  {},
			label.OutsideBottomRight: {},
		}
	}
	return map[label.Position]struct{}{}
}

func (s shapeCallout) SnapPointPercentages() [][]*geo.RelativePoint {
	box := s.GetBox()
	width := box.Width
	height := box.Height
	tipWidth := 30.0
	tipHeight := 45.0

	if width < tipWidth*2 {
		tipWidth = width / 2.0
	}

	if height < tipHeight*2 {
		tipHeight = height / 2.0
	}

	return [][]*geo.RelativePoint{
		// Top
		{
			geo.NewRelativePoint(0.25, 0),
			geo.NewRelativePoint(0.5, 0),
			geo.NewRelativePoint(0.75, 0),
		},
		// Left
		{
			geo.NewRelativePoint(0, ((height-tipHeight)/height)*0.25),
			geo.NewRelativePoint(0, ((height-tipHeight)/height)*0.5),
			geo.NewRelativePoint(0, ((height-tipHeight)/height)*0.75),
		},
		// Bottom
		{
			geo.NewRelativePoint(0.5*0.33, (height-tipHeight)/height),
			geo.NewRelativePoint(0.5*0.66, (height-tipHeight)/height),
			geo.NewRelativePoint(0.5, 1),
			geo.NewRelativePoint(
				1-((width/2.0-tipWidth)/width)*0.5,
				(height-tipHeight)/height,
			),
		},
		// Right
		{
			geo.NewRelativePoint(1, ((height-tipHeight)/height)*0.25),
			geo.NewRelativePoint(1, ((height-tipHeight)/height)*0.5),
			geo.NewRelativePoint(1, ((height-tipHeight)/height)*0.75),
		},
	}
}

func (s shapeCallout) PortIndices(orientation geo.Orientation) []int {
	switch orientation {
	case geo.Top:
		return []int{0, 1, 2}
	case geo.Left:
		return []int{3, 4, 5}
	case geo.Bottom:
		return []int{6, 7, 8, 9}
	case geo.Right:
		return []int{10, 11, 12}
	case geo.TopLeft:
		return []int{0, 1, 2, 3, 4, 5}
	case geo.TopRight:
		return []int{0, 1, 2, 10, 11, 12}
	case geo.BottomLeft:
		return []int{6, 7, 8, 9, 3, 4, 5}
	case geo.BottomRight:
		return []int{6, 7, 8, 9, 10, 11, 12}
	default:
		return []int{}
	}
}
func (s shapeCallout) CenterPortIndices() []int {
	return []int{1, 4, 8, 11}
}

func (s shapeCallout) MirroredPortIndices() map[int]int {
	// Left and right
	return map[int]int{4: 11, 11: 4}
}

func (s shapeCallout) CenterPortIndex(o geo.Orientation) int {
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
