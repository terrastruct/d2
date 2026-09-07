package nodeshape

import (
	"github.com/d2lang/d2/lib/label"
)

type shapeCircle struct {
	shapeSquare
}

func (s shapeCircle) LabelPositionPreferences(preferenceSet LabelTier) map[label.Position]struct{} {
	switch preferenceSet {
	case Good:
		return map[label.Position]struct{}{
			label.OutsideTopCenter:    {},
			label.InsideMiddleCenter:  {},
			label.OutsideBottomCenter: {},
		}
	case OK:
		return map[label.Position]struct{}{
			label.InsideBottomCenter: {},
			label.InsideTopCenter:    {},
			label.InsideMiddleLeft:   {},
			label.InsideMiddleRight:  {},
		}
	case Unideal:
		return map[label.Position]struct{}{
			label.InsideTopLeft:      {},
			label.InsideTopRight:     {},
			label.InsideBottomLeft:   {},
			label.InsideBottomRight:  {},
			label.OutsideLeftMiddle:  {},
			label.OutsideRightMiddle: {},
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
