package label

import (
	"math"

	"oss.terrastruct.com/d2/lib/geo"
)

// These are % locations where labels will be placed along the connection
const LEFT_LABEL_POSITION = 1.0 / 4.0
const CENTER_LABEL_POSITION = 2.0 / 4.0
const RIGHT_LABEL_POSITION = 3.0 / 4.0

// This is the space between a node border and its outside label
const PADDING = 5

type Position int8

const (
	Unset Position = iota

	OutsideTopLeft
	OutsideTopCenter
	OutsideTopRight

	OutsideLeftTop
	OutsideLeftMiddle
	OutsideLeftBottom

	OutsideRightTop
	OutsideRightMiddle
	OutsideRightBottom

	OutsideBottomLeft
	OutsideBottomCenter
	OutsideBottomRight

	InsideTopLeft
	InsideTopCenter
	InsideTopRight

	InsideMiddleLeft
	InsideMiddleCenter
	InsideMiddleRight

	InsideBottomLeft
	InsideBottomCenter
	InsideBottomRight

	BorderTopLeft
	BorderTopCenter
	BorderTopRight

	BorderLeftTop
	BorderLeftMiddle
	BorderLeftBottom

	BorderRightTop
	BorderRightMiddle
	BorderRightBottom

	BorderBottomLeft
	BorderBottomCenter
	BorderBottomRight

	UnlockedTop
	UnlockedMiddle
	UnlockedBottom
)

func FromString(s string) Position {
	switch s {
	case "OUTSIDE_TOP_LEFT":
		return OutsideTopLeft
	case "OUTSIDE_TOP_CENTER":
		return OutsideTopCenter
	case "OUTSIDE_TOP_RIGHT":
		return OutsideTopRight

	case "OUTSIDE_LEFT_TOP":
		return OutsideLeftTop
	case "OUTSIDE_LEFT_MIDDLE":
		return OutsideLeftMiddle
	case "OUTSIDE_LEFT_BOTTOM":
		return OutsideLeftBottom

	case "OUTSIDE_RIGHT_TOP":
		return OutsideRightTop
	case "OUTSIDE_RIGHT_MIDDLE":
		return OutsideRightMiddle
	case "OUTSIDE_RIGHT_BOTTOM":
		return OutsideRightBottom

	case "OUTSIDE_BOTTOM_LEFT":
		return OutsideBottomLeft
	case "OUTSIDE_BOTTOM_CENTER":
		return OutsideBottomCenter
	case "OUTSIDE_BOTTOM_RIGHT":
		return OutsideBottomRight

	case "INSIDE_TOP_LEFT":
		return InsideTopLeft
	case "INSIDE_TOP_CENTER":
		return InsideTopCenter
	case "INSIDE_TOP_RIGHT":
		return InsideTopRight

	case "INSIDE_MIDDLE_LEFT":
		return InsideMiddleLeft
	case "INSIDE_MIDDLE_CENTER":
		return InsideMiddleCenter
	case "INSIDE_MIDDLE_RIGHT":
		return InsideMiddleRight

	case "INSIDE_BOTTOM_LEFT":
		return InsideBottomLeft
	case "INSIDE_BOTTOM_CENTER":
		return InsideBottomCenter
	case "INSIDE_BOTTOM_RIGHT":
		return InsideBottomRight

	case "BORDER_TOP_LEFT":
		return BorderTopLeft
	case "BORDER_TOP_CENTER":
		return BorderTopCenter
	case "BORDER_TOP_RIGHT":
		return BorderTopRight

	case "BORDER_LEFT_TOP":
		return BorderLeftTop
	case "BORDER_LEFT_MIDDLE":
		return BorderLeftMiddle
	case "BORDER_LEFT_BOTTOM":
		return BorderLeftBottom

	case "BORDER_RIGHT_TOP":
		return BorderRightTop
	case "BORDER_RIGHT_MIDDLE":
		return BorderRightMiddle
	case "BORDER_RIGHT_BOTTOM":
		return BorderRightBottom

	case "BORDER_BOTTOM_LEFT":
		return BorderBottomLeft
	case "BORDER_BOTTOM_CENTER":
		return BorderBottomCenter
	case "BORDER_BOTTOM_RIGHT":
		return BorderBottomRight

	case "UNLOCKED_TOP":
		return UnlockedTop
	case "UNLOCKED_MIDDLE":
		return UnlockedMiddle
	case "UNLOCKED_BOTTOM":
		return UnlockedBottom
	default:
		return Unset
	}
}

func (position Position) String() string {
	switch position {
	case OutsideTopLeft:
		return "OUTSIDE_TOP_LEFT"
	case OutsideTopCenter:
		return "OUTSIDE_TOP_CENTER"
	case OutsideTopRight:
		return "OUTSIDE_TOP_RIGHT"

	case OutsideLeftTop:
		return "OUTSIDE_LEFT_TOP"
	case OutsideLeftMiddle:
		return "OUTSIDE_LEFT_MIDDLE"
	case OutsideLeftBottom:
		return "OUTSIDE_LEFT_BOTTOM"

	case OutsideRightTop:
		return "OUTSIDE_RIGHT_TOP"
	case OutsideRightMiddle:
		return "OUTSIDE_RIGHT_MIDDLE"
	case OutsideRightBottom:
		return "OUTSIDE_RIGHT_BOTTOM"

	case OutsideBottomLeft:
		return "OUTSIDE_BOTTOM_LEFT"
	case OutsideBottomCenter:
		return "OUTSIDE_BOTTOM_CENTER"
	case OutsideBottomRight:
		return "OUTSIDE_BOTTOM_RIGHT"

	case InsideTopLeft:
		return "INSIDE_TOP_LEFT"
	case InsideTopCenter:
		return "INSIDE_TOP_CENTER"
	case InsideTopRight:
		return "INSIDE_TOP_RIGHT"

	case InsideMiddleLeft:
		return "INSIDE_MIDDLE_LEFT"
	case InsideMiddleCenter:
		return "INSIDE_MIDDLE_CENTER"
	case InsideMiddleRight:
		return "INSIDE_MIDDLE_RIGHT"

	case InsideBottomLeft:
		return "INSIDE_BOTTOM_LEFT"
	case InsideBottomCenter:
		return "INSIDE_BOTTOM_CENTER"
	case InsideBottomRight:
		return "INSIDE_BOTTOM_RIGHT"

	case BorderTopLeft:
		return "BORDER_TOP_LEFT"
	case BorderTopCenter:
		return "BORDER_TOP_CENTER"
	case BorderTopRight:
		return "BORDER_TOP_RIGHT"

	case BorderLeftTop:
		return "BORDER_LEFT_TOP"
	case BorderLeftMiddle:
		return "BORDER_LEFT_MIDDLE"
	case BorderLeftBottom:
		return "BORDER_LEFT_BOTTOM"

	case BorderRightTop:
		return "BORDER_RIGHT_TOP"
	case BorderRightMiddle:
		return "BORDER_RIGHT_MIDDLE"
	case BorderRightBottom:
		return "BORDER_RIGHT_BOTTOM"

	case BorderBottomLeft:
		return "BORDER_BOTTOM_LEFT"
	case BorderBottomCenter:
		return "BORDER_BOTTOM_CENTER"
	case BorderBottomRight:
		return "BORDER_BOTTOM_RIGHT"

	case UnlockedTop:
		return "UNLOCKED_TOP"
	case UnlockedMiddle:
		return "UNLOCKED_MIDDLE"
	case UnlockedBottom:
		return "UNLOCKED_BOTTOM"

	default:
		return ""
	}
}

func (position Position) IsShapePosition() bool {
	switch position {
	case OutsideTopLeft, OutsideTopCenter, OutsideTopRight,
		OutsideBottomLeft, OutsideBottomCenter, OutsideBottomRight,
		OutsideLeftTop, OutsideLeftMiddle, OutsideLeftBottom,
		OutsideRightTop, OutsideRightMiddle, OutsideRightBottom,

		InsideTopLeft, InsideTopCenter, InsideTopRight,
		InsideMiddleLeft, InsideMiddleCenter, InsideMiddleRight,
		InsideBottomLeft, InsideBottomCenter, InsideBottomRight,

		BorderTopLeft, BorderTopCenter, BorderTopRight,
		BorderLeftTop, BorderLeftMiddle, BorderLeftBottom,
		BorderRightTop, BorderRightMiddle, BorderRightBottom,
		BorderBottomLeft, BorderBottomCenter, BorderBottomRight:
		return true
	default:
		return false
	}
}

func (position Position) IsEdgePosition() bool {
	switch position {
	case OutsideTopLeft, OutsideTopCenter, OutsideTopRight,
		InsideMiddleLeft, InsideMiddleCenter, InsideMiddleRight,
		OutsideBottomLeft, OutsideBottomCenter, OutsideBottomRight,
		UnlockedTop, UnlockedMiddle, UnlockedBottom:
		return true
	default:
		return false
	}
}

func (position Position) IsOutside() bool {
	switch position {
	case OutsideTopLeft, OutsideTopCenter, OutsideTopRight,
		OutsideBottomLeft, OutsideBottomCenter, OutsideBottomRight,
		OutsideLeftTop, OutsideLeftMiddle, OutsideLeftBottom,
		OutsideRightTop, OutsideRightMiddle, OutsideRightBottom:
		return true
	default:
		return false
	}
}

func (position Position) IsUnlocked() bool {
	switch position {
	case UnlockedTop, UnlockedMiddle, UnlockedBottom:
		return true
	default:
		return false
	}
}

func (position Position) IsBorder() bool {
	switch position {
	case BorderTopLeft, BorderTopCenter, BorderTopRight,
		BorderLeftTop, BorderLeftMiddle, BorderLeftBottom,
		BorderRightTop, BorderRightMiddle, BorderRightBottom,
		BorderBottomLeft, BorderBottomCenter, BorderBottomRight:
		return true
	default:
		return false
	}
}

func (position Position) IsOnEdge() bool {
	switch position {
	case InsideMiddleLeft, InsideMiddleCenter, InsideMiddleRight, UnlockedMiddle:
		return true
	default:
		return false
	}
}

func (position Position) Mirrored() Position {
	switch position {
	case OutsideTopLeft:
		return OutsideBottomRight
	case OutsideTopCenter:
		return OutsideBottomCenter
	case OutsideTopRight:
		return OutsideBottomLeft

	case OutsideLeftTop:
		return OutsideRightBottom
	case OutsideLeftMiddle:
		return OutsideRightMiddle
	case OutsideLeftBottom:
		return OutsideRightTop

	case OutsideRightTop:
		return OutsideLeftBottom
	case OutsideRightMiddle:
		return OutsideLeftMiddle
	case OutsideRightBottom:
		return OutsideLeftTop

	case OutsideBottomLeft:
		return OutsideTopRight
	case OutsideBottomCenter:
		return OutsideTopCenter
	case OutsideBottomRight:
		return OutsideTopLeft

	case InsideTopLeft:
		return InsideBottomRight
	case InsideTopCenter:
		return InsideBottomCenter
	case InsideTopRight:
		return InsideBottomLeft

	case InsideMiddleLeft:
		return InsideMiddleRight
	case InsideMiddleCenter:
		return InsideMiddleCenter
	case InsideMiddleRight:
		return InsideMiddleLeft

	case InsideBottomLeft:
		return InsideTopRight
	case InsideBottomCenter:
		return InsideTopCenter
	case InsideBottomRight:
		return InsideTopLeft

	case BorderTopLeft:
		return BorderBottomRight
	case BorderTopCenter:
		return BorderBottomCenter
	case BorderTopRight:
		return BorderBottomLeft

	case BorderLeftTop:
		return BorderRightBottom
	case BorderLeftMiddle:
		return BorderRightMiddle
	case BorderLeftBottom:
		return BorderRightTop

	case BorderRightTop:
		return BorderLeftBottom
	case BorderRightMiddle:
		return BorderLeftMiddle
	case BorderRightBottom:
		return BorderLeftTop

	case BorderBottomLeft:
		return BorderTopRight
	case BorderBottomCenter:
		return BorderTopCenter
	case BorderBottomRight:
		return BorderTopLeft

	case UnlockedTop:
		return UnlockedBottom
	case UnlockedBottom:
		return UnlockedTop
	case UnlockedMiddle:
		return UnlockedMiddle

	default:
		return Unset
	}
}

func (labelPosition Position) GetPointOnBox(box *geo.Box, padding, width, height float64) *geo.Point {
	p := box.TopLeft.Copy()
	boxCenter := box.Center()

	switch labelPosition {
	case OutsideTopLeft:
		p.X -= padding
		p.Y -= padding + height
	case OutsideTopCenter:
		p.X = boxCenter.X - width/2
		p.Y -= padding + height
	case OutsideTopRight:
		p.X += box.Width - width - padding
		p.Y -= padding + height

	case OutsideLeftTop:
		p.X -= padding + width
		p.Y += padding
	case OutsideLeftMiddle:
		p.X -= padding + width
		p.Y = boxCenter.Y - height/2
	case OutsideLeftBottom:
		p.X -= padding + width
		p.Y += box.Height - height - padding

	case OutsideRightTop:
		p.X += box.Width + padding
		p.Y += padding
	case OutsideRightMiddle:
		p.X += box.Width + padding
		p.Y = boxCenter.Y - height/2
	case OutsideRightBottom:
		p.X += box.Width + padding
		p.Y += box.Height - height - padding

	case OutsideBottomLeft:
		p.X += padding
		p.Y += box.Height + padding
	case OutsideBottomCenter:
		p.X = boxCenter.X - width/2
		p.Y += box.Height + padding
	case OutsideBottomRight:
		p.X += box.Width - width - padding
		p.Y += box.Height + padding

	case InsideTopLeft:
		p.X += padding
		p.Y += padding
	case InsideTopCenter:
		p.X = boxCenter.X - width/2
		p.Y += padding
	case InsideTopRight:
		p.X += box.Width - width - padding
		p.Y += padding

	case InsideMiddleLeft:
		p.X += padding
		p.Y = boxCenter.Y - height/2
	case InsideMiddleCenter:
		p.X = boxCenter.X - width/2
		p.Y = boxCenter.Y - height/2
	case InsideMiddleRight:
		p.X += box.Width - width - padding
		p.Y = boxCenter.Y - height/2

	case InsideBottomLeft:
		p.X += padding
		p.Y += box.Height - height - padding
	case InsideBottomCenter:
		p.X = boxCenter.X - width/2
		p.Y += box.Height - height - padding
	case InsideBottomRight:
		p.X += box.Width - width - padding
		p.Y += box.Height - height - padding

	case BorderTopLeft:
		p.X += padding
		p.Y -= height / 2
	case BorderTopCenter:
		p.X = boxCenter.X - width/2
		p.Y -= height / 2
	case BorderTopRight:
		p.X += box.Width - width - padding
		p.Y -= height / 2

	case BorderLeftTop:
		p.X -= width / 2
		p.Y += padding
	case BorderLeftMiddle:
		p.X -= width / 2
		p.Y = boxCenter.Y - height/2
	case BorderLeftBottom:
		p.X -= width / 2
		p.Y += box.Height - height - padding

	case BorderRightTop:
		p.X += box.Width - width/2
		p.Y += padding
	case BorderRightMiddle:
		p.X += box.Width - width/2
		p.Y = boxCenter.Y - height/2
	case BorderRightBottom:
		p.X += box.Width - width/2
		p.Y += box.Height - height - padding

	case BorderBottomLeft:
		p.X += padding
		p.Y += box.Height - height/2
	case BorderBottomCenter:
		p.X = boxCenter.X - width/2
		p.Y += box.Height - height/2
	case BorderBottomRight:
		p.X += box.Width - width - padding
		p.Y += box.Height - height/2
	}

	return p
}

// return the top left point of a width x height label at the given label position on the route
// also return the index of the route segment that point is on
func (labelPosition Position) GetPointOnRoute(route geo.Route, strokeWidth, labelPercentage, width, height float64) (point *geo.Point, index int) {
	totalLength := route.Length()
	leftPosition := LEFT_LABEL_POSITION * totalLength
	centerPosition := CENTER_LABEL_POSITION * totalLength
	rightPosition := RIGHT_LABEL_POSITION * totalLength
	unlockedPosition := labelPercentage * totalLength

	// outside labels have to be offset in the direction of the edge's normal Vector
	// Note: we flip the normal for Top labels but keep it as is for Bottom labels since positive Y is below in SVG
	getOffsetLabelPosition := func(basePoint, normStart, normEnd *geo.Point, flip bool) *geo.Point {
		// get the normal as a unit Vector so we can multiply to project in its direction
		normalX, normalY := geo.GetUnitNormalVector(
			normStart.X,
			normStart.Y,
			normEnd.X,
			normEnd.Y,
		)
		if flip {
			normalX *= -1
			normalY *= -1
		}

		// Horizontal Edge with Outside Label          |      Vertical Edge with Outside Label
		//  ┌────────────────────┐    ┬                |       ┌─┬─┐
		//  │                    │    │                |       │ │ │    ┌───────────┬───────────┐
		//  │                    │    │                |       │ e │    │           │           │
		//  ├────label─center────┤  ┬ ┼label height    |       │ d │    │         label         │
		//  │                    │  │ │                |       │ g │    │         center        │
		//  │                    │  │ │                |       │ e │    │           │           │
		//  └────────────────────┘  │ ┴ ┬              |       │ │ │    └───────────┴───────────┘
		//                          │   │              |       └─┴─┘   offset
		//                    offset│   │label padding |         ├──────────────────┤
		//                          │   │              |
		// ┌──────────────────────┐ │ ┬ ┴              |                ├───────────┼───────────┤
		// │                      │ │ │                |           ├────┤      label width
		// ├─────edge─center──────┤ ┴ ┼stroke width    |        label padding
		// │                      │   │                |       ├─┼─┤
		// └──────────────────────┘   ┴                |    stroke width
		//
		// TODO: get actual edge stroke width on edge
		offsetX := strokeWidth/2 + float64(PADDING) + width/2
		offsetY := strokeWidth/2 + float64(PADDING) + height/2

		return geo.NewPoint(basePoint.X+normalX*offsetX, basePoint.Y+normalY*offsetY)
	}

	var labelCenter *geo.Point
	switch labelPosition {
	case InsideMiddleLeft:
		labelCenter, index = route.GetPointAtDistance(leftPosition)
	case InsideMiddleCenter:
		labelCenter, index = route.GetPointAtDistance(centerPosition)
	case InsideMiddleRight:
		labelCenter, index = route.GetPointAtDistance(rightPosition)

	case OutsideTopLeft:
		basePoint, index := route.GetPointAtDistance(leftPosition)
		labelCenter = getOffsetLabelPosition(basePoint, route[index], route[index+1], true)
	case OutsideTopCenter:
		basePoint, index := route.GetPointAtDistance(centerPosition)
		labelCenter = getOffsetLabelPosition(basePoint, route[index], route[index+1], true)
	case OutsideTopRight:
		basePoint, index := route.GetPointAtDistance(rightPosition)
		labelCenter = getOffsetLabelPosition(basePoint, route[index], route[index+1], true)

	case OutsideBottomLeft:
		basePoint, index := route.GetPointAtDistance(leftPosition)
		labelCenter = getOffsetLabelPosition(basePoint, route[index], route[index+1], false)
	case OutsideBottomCenter:
		basePoint, index := route.GetPointAtDistance(centerPosition)
		labelCenter = getOffsetLabelPosition(basePoint, route[index], route[index+1], false)
	case OutsideBottomRight:
		basePoint, index := route.GetPointAtDistance(rightPosition)
		labelCenter = getOffsetLabelPosition(basePoint, route[index], route[index+1], false)

	case UnlockedTop:
		basePoint, index := route.GetPointAtDistance(unlockedPosition)
		labelCenter = getOffsetLabelPosition(basePoint, route[index], route[index+1], true)
	case UnlockedMiddle:
		labelCenter, index = route.GetPointAtDistance(unlockedPosition)
	case UnlockedBottom:
		basePoint, index := route.GetPointAtDistance(unlockedPosition)
		labelCenter = getOffsetLabelPosition(basePoint, route[index], route[index+1], false)
	default:
		return nil, -1
	}
	// convert from center to top left
	labelCenter.X = chopPrecision(labelCenter.X - width/2)
	labelCenter.Y = chopPrecision(labelCenter.Y - height/2)
	return labelCenter, index
}

// TODO probably use math.Big
func chopPrecision(f float64) float64 {
	// bring down to float32 precision before rounding for consistency across architectures
	return math.Round(float64(float32(f*10000)) / 10000)
}
