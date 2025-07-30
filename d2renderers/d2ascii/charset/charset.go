package charset

// Character constants for comparisons (Unicode values)
const (
	UnicodeHorizontal     = "─"
	UnicodeVertical       = "│"
	UnicodeTopLeftCorner  = "┌"
	UnicodeTopRightCorner = "┐"
	UnicodeBottomLeftCorner = "└"
	UnicodeBottomRightCorner = "┘"
	UnicodeTopLeftArc     = "╭"
	UnicodeTopRightArc    = "╮"
	UnicodeBottomLeftArc  = "╰"
	UnicodeBottomRightArc = "╯"
	UnicodeUnderscore     = "_"
	UnicodeOverline       = "‾"
)

// Set defines the interface for character sets used in ASCII rendering
type Set interface {
	// Corners
	TopLeftArc() string
	TopRightArc() string
	BottomLeftArc() string
	BottomRightArc() string
	TopLeftCorner() string
	TopRightCorner() string
	BottomLeftCorner() string
	BottomRightCorner() string

	// Lines
	Horizontal() string
	Vertical() string
	LeftVertical() string
	RightVertical() string
	Backslash() string
	ForwardSlash() string
	Cross() string

	// Junctions
	TDown() string
	TLeft() string
	TRight() string
	TUp() string

	// Other
	Underscore() string
	Overline() string
	Dot() string
	Hyphen() string
	Tilde() string

	// Symbols
	Cloud() string
	Circle() string
	Oval() string
	Star() string

	// Arrows
	ArrowUp() string
	ArrowRight() string
	ArrowDown() string
	ArrowLeft() string
}

// Type represents the type of character set
type Type int

const (
	Unicode Type = iota
	ASCII
)

// New creates a new character set based on the specified type
func New(t Type) Set {
	switch t {
	case ASCII:
		return NewASCII()
	default:
		return NewUnicode()
	}
}