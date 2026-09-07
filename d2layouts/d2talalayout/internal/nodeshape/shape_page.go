package nodeshape

import (
	"github.com/d2lang/d2/lib/label"
)

type shapePage struct {
	shapeSquare
}

func (s shapePage) LabelPositionPreferences(preferenceSet LabelTier) map[label.Position]struct{} {
	switch preferenceSet {
	case Good:
		return map[label.Position]struct{}{
			label.InsideMiddleCenter:  {},
			label.InsideBottomCenter:  {},
			label.OutsideBottomCenter: {},
		}
	case OK:
		return map[label.Position]struct{}{
			label.InsideTopLeft:      {},
			label.InsideMiddleLeft:   {},
			label.InsideMiddleRight:  {},
			label.InsideBottomLeft:   {},
			label.InsideBottomRight:  {},
			label.OutsideTopLeft:     {},
			label.OutsideTopCenter:   {},
			label.OutsideBottomLeft:  {},
			label.OutsideBottomRight: {},
		}
	case Unideal:
		return map[label.Position]struct{}{
			label.OutsideTopRight:    {},
			label.InsideTopCenter:    {},
			label.OutsideRightBottom: {},
			label.OutsideRightMiddle: {},
			label.OutsideLeftTop:     {},
			label.OutsideLeftMiddle:  {},
			label.OutsideLeftBottom:  {},
		}
	case Bad:
		return map[label.Position]struct{}{
			label.InsideTopRight:  {},
			label.OutsideRightTop: {},
		}
	}
	return map[label.Position]struct{}{}
}
