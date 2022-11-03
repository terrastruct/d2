package geo

type Orientation int

const (
	TopLeft Orientation = iota
	TopRight
	BottomLeft
	BottomRight

	Top
	Right
	Bottom
	Left

	NONE
)

func (o Orientation) ToString() string {
	switch o {
	case TopLeft:
		return "TopLeft"
	case TopRight:
		return "TopRight"
	case BottomLeft:
		return "BottomLeft"
	case BottomRight:
		return "BottomRight"

	case Top:
		return "Top"
	case Right:
		return "Right"
	case Bottom:
		return "Bottom"
	case Left:
		return "Left"
	default:
		return ""
	}
}

func (o1 Orientation) SameSide(o2 Orientation) bool {
	sides := [][]Orientation{
		{TopLeft, Top, TopRight},
		{BottomLeft, Bottom, BottomRight},
		{Left, TopLeft, BottomLeft},
		{Right, TopRight, BottomRight},
	}
	for _, sameSides := range sides {
		isO1 := false
		for _, side := range sameSides {
			if side == o1 {
				isO1 = true
				break
			}
		}
		if isO1 {
			for _, side := range sameSides {
				if side == o2 {
					return true
				}
			}
		}
	}
	return false
}

func (o Orientation) IsDiagonal() bool {
	return o == TopLeft || o == TopRight || o == BottomLeft || o == BottomRight
}

func (o Orientation) IsHorizontal() bool {
	return o == Left || o == Right
}

func (o Orientation) IsVertical() bool {
	return o == Top || o == Bottom
}

func (o Orientation) GetOpposite() Orientation {
	switch o {
	case TopLeft:
		return BottomRight
	case TopRight:
		return BottomLeft
	case BottomLeft:
		return TopRight
	case BottomRight:
		return TopLeft

	case Top:
		return Bottom
	case Bottom:
		return Top
	case Right:
		return Left
	case Left:
		return Right

	default:
		return o
	}
}
