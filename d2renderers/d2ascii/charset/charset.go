package charset

const (
	UnicodeHorizontal        = "─"
	UnicodeVertical          = "│"
	UnicodeTopLeftCorner     = "┌"
	UnicodeTopRightCorner    = "┐"
	UnicodeBottomLeftCorner  = "└"
	UnicodeBottomRightCorner = "┘"
	UnicodeTopLeftArc        = "╭"
	UnicodeTopRightArc       = "╮"
	UnicodeBottomLeftArc     = "╰"
	UnicodeBottomRightArc    = "╯"
	UnicodeUnderscore        = "_"
	UnicodeOverline          = "‾"
)

type Set interface {
	TopLeftArc() string
	TopRightArc() string
	BottomLeftArc() string
	BottomRightArc() string
	TopLeftCorner() string
	TopRightCorner() string
	BottomLeftCorner() string
	BottomRightCorner() string

	Horizontal() string
	Vertical() string
	LeftVertical() string
	RightVertical() string
	Backslash() string
	ForwardSlash() string
	Cross() string

	TDown() string
	TLeft() string
	TRight() string
	TUp() string

	Underscore() string
	Overline() string
	Dot() string
	Hyphen() string
	Tilde() string

	Cloud() string
	Circle() string
	Oval() string
	Star() string

	ArrowUp() string
	ArrowRight() string
	ArrowDown() string
	ArrowLeft() string
}

type Type int

const (
	Unicode Type = iota
	ASCII
)

func New(t Type) Set {
	switch t {
	case ASCII:
		return NewASCII()
	default:
		return NewUnicode()
	}
}
