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

type Position string

const (
	OutsideTopLeft   Position = "OUTSIDE_TOP_LEFT"
	OutsideTopCenter Position = "OUTSIDE_TOP_CENTER"
	OutsideTopRight  Position = "OUTSIDE_TOP_RIGHT"

	OutsideLeftTop    Position = "OUTSIDE_LEFT_TOP"
	OutsideLeftMiddle Position = "OUTSIDE_LEFT_MIDDLE"
	OutsideLeftBottom Position = "OUTSIDE_LEFT_BOTTOM"

	OutsideRightTop    Position = "OUTSIDE_RIGHT_TOP"
	OutsideRightMiddle Position = "OUTSIDE_RIGHT_MIDDLE"
	OutsideRightBottom Position = "OUTSIDE_RIGHT_BOTTOM"

	OutsideBottomLeft   Position = "OUTSIDE_BOTTOM_LEFT"
	OutsideBottomCenter Position = "OUTSIDE_BOTTOM_CENTER"
	OutsideBottomRight  Position = "OUTSIDE_BOTTOM_RIGHT"

	InsideTopLeft   Position = "INSIDE_TOP_LEFT"
	InsideTopCenter Position = "INSIDE_TOP_CENTER"
	InsideTopRight  Position = "INSIDE_TOP_RIGHT"

	InsideMiddleLeft   Position = "INSIDE_MIDDLE_LEFT"
	InsideMiddleCenter Position = "INSIDE_MIDDLE_CENTER"
	InsideMiddleRight  Position = "INSIDE_MIDDLE_RIGHT"

	InsideBottomLeft   Position = "INSIDE_BOTTOM_LEFT"
	InsideBottomCenter Position = "INSIDE_BOTTOM_CENTER"
	InsideBottomRight  Position = "INSIDE_BOTTOM_RIGHT"

	UnlockedTop    Position = "UNLOCKED_TOP"
	UnlockedMiddle Position = "UNLOCKED_MIDDLE"
	UnlockedBottom Position = "UNLOCKED_BOTTOM"
)

func (position Position) IsShapePosition() bool {
	switch position {
	case OutsideTopLeft, OutsideTopCenter, OutsideTopRight,
		OutsideBottomLeft, OutsideBottomCenter, OutsideBottomRight,
		OutsideLeftTop, OutsideLeftMiddle, OutsideLeftBottom,
		OutsideRightTop, OutsideRightMiddle, OutsideRightBottom,

		InsideTopLeft, InsideTopCenter, InsideTopRight,
		InsideMiddleLeft, InsideMiddleCenter, InsideMiddleRight,
		InsideBottomLeft, InsideBottomCenter, InsideBottomRight:
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

	case UnlockedTop:
		return UnlockedBottom
	case UnlockedBottom:
		return UnlockedTop
	case UnlockedMiddle:
		return UnlockedMiddle

	default:
		return ""
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
