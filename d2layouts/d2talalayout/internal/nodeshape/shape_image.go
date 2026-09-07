package nodeshape

import (
	"github.com/d2lang/d2/lib/label"
)

type shapeImage struct {
	shapeSquare
}

func (s shapeImage) LabelPositionPreferences(preferenceSet LabelTier) map[label.Position]struct{} {
	switch preferenceSet {
	case Good:
		return map[label.Position]struct{}{
			label.OutsideBottomCenter: {},
			label.OutsideTopCenter:    {},
		}
	case OK:
		return map[label.Position]struct{}{
			label.OutsideLeftMiddle:  {},
			label.OutsideRightMiddle: {},
		}
	case Unideal:
		return map[label.Position]struct{}{
			label.OutsideTopLeft:     {},
			label.OutsideTopRight:    {},
			label.OutsideBottomLeft:  {},
			label.OutsideBottomRight: {},
			label.OutsideLeftTop:     {},
			label.OutsideRightTop:    {},
			label.OutsideLeftBottom:  {},
			label.OutsideRightBottom: {},
		}
	case Bad:
		return map[label.Position]struct{}{}
	}
	return map[label.Position]struct{}{}
}
