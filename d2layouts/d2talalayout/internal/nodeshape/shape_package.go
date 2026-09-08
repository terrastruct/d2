package nodeshape

import (
	"math"

	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
	"github.com/d2lang/d2/lib/shape"
)

type shapePackage struct {
	shape.Shape
}

func (s shapePackage) LabelPositionPreferences(preferenceSet LabelTier) map[label.Position]struct{} {
	switch preferenceSet {
	case Good:
		return map[label.Position]struct{}{
			label.InsideMiddleCenter:  {},
			label.InsideMiddleLeft:    {},
			label.InsideBottomLeft:    {},
			label.InsideBottomCenter:  {},
			label.OutsideBottomLeft:   {},
			label.OutsideBottomCenter: {},
			label.InsideTopLeft:       {},
			label.InsideTopCenter:     {},
			label.InsideTopRight:      {},
		}
	case OK:
		return map[label.Position]struct{}{
			label.OutsideTopLeft:     {},
			label.InsideMiddleRight:  {},
			label.InsideBottomRight:  {},
			label.OutsideBottomRight: {},
		}
	case Unideal:
		return map[label.Position]struct{}{
			label.OutsideTopCenter:   {},
			label.OutsideLeftMiddle:  {},
			label.OutsideRightMiddle: {},
			label.OutsideLeftTop:     {},
			label.OutsideLeftBottom:  {},
			label.OutsideRightBottom: {},
		}
	case Bad:
		return map[label.Position]struct{}{
			label.OutsideTopRight: {},
			label.OutsideRightTop: {},
		}
	}
	return map[label.Position]struct{}{}
}

func (s shapePackage) SnapPointPercentages() [][]*geo.RelativePoint {
	box := s.GetBox()
	width := box.Width
	height := box.Height
	verticalScalar := 0.2
	horizontalScalar := 0.5
	minTopHeight := 34.0
	maxTopHeight := 55.0

	minTopWidth := 50.0
	maxTopWidth := 150.0

	topWidth := math.Min(maxTopWidth, math.Max(minTopWidth, width*horizontalScalar))

	if width < 2*minTopWidth {
		topWidth = width * horizontalScalar
	}

	topHeight := math.Min(maxTopHeight, math.Max(minTopHeight, height*verticalScalar))

	if height < 2*minTopHeight {
		topHeight = height * verticalScalar
	}

	topWidthRatio := topWidth / width
	topHeightRatio := topHeight / height

	// Space occupied by main shape not overlapping
	negativeTopWidthRatio := 1 - topWidthRatio
	negativeTopHeightRatio := 1 - topHeightRatio

	return [][]*geo.RelativePoint{
		// Top
		{
			geo.NewRelativePoint(topWidthRatio*0.33, 0),
			geo.NewRelativePoint(topWidthRatio*0.66, 0),
			geo.NewRelativePoint(topWidthRatio+negativeTopWidthRatio*0.5, topHeightRatio),
		},
		// Left
		{
			geo.NewRelativePoint(0, topHeightRatio+negativeTopHeightRatio*0.25),
			geo.NewRelativePoint(0, topHeightRatio+negativeTopHeightRatio*0.5),
			geo.NewRelativePoint(0, topHeightRatio+negativeTopHeightRatio*0.75),
		},
		// Bottom
		{
			geo.NewRelativePoint(0.25, 1),
			geo.NewRelativePoint(0.5, 1),
			geo.NewRelativePoint(0.75, 1),
		},
		// Right
		{
			geo.NewRelativePoint(1, topHeightRatio+negativeTopHeightRatio*0.25),
			geo.NewRelativePoint(1, topHeightRatio+negativeTopHeightRatio*0.5),
			geo.NewRelativePoint(1, topHeightRatio+negativeTopHeightRatio*0.75),
		},
	}
}

func (s shapePackage) PortIndices(orientation geo.Orientation) []int {
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

func (s shapePackage) MirroredPortIndices() map[int]int {
	return map[int]int{
		// Left and right
		4:  10,
		10: 4,
	}
}

func (s shapePackage) CenterPortIndices() []int {
	return []int{0, 1, 4, 7, 10}
}

func (s shapePackage) CenterPortIndex(o geo.Orientation) int {
	// Package type has two top center ports: 0 is also a center port, but 1 is closer to the center
	centerPorts := s.CenterPortIndices()
	var centerPortsIndex int
	switch o {
	case geo.Top:
		centerPortsIndex = 1
	case geo.Left:
		centerPortsIndex = 2
	case geo.Bottom:
		centerPortsIndex = 3
	case geo.Right:
		centerPortsIndex = 4
	default:
		return -1
	}

	return centerPorts[centerPortsIndex]
}
