package charset

// ASCIISet implements the Set interface using standard ASCII characters
type ASCIISet struct{}

func NewASCII() Set {
	return &ASCIISet{}
}

// Corners
func (a *ASCIISet) TopLeftArc() string        { return "." }
func (a *ASCIISet) TopRightArc() string       { return "." }
func (a *ASCIISet) BottomLeftArc() string     { return "'" }
func (a *ASCIISet) BottomRightArc() string    { return "'" }
func (a *ASCIISet) TopLeftCorner() string     { return "+" }
func (a *ASCIISet) TopRightCorner() string    { return "+" }
func (a *ASCIISet) BottomLeftCorner() string  { return "+" }
func (a *ASCIISet) BottomRightCorner() string { return "+" }

// Lines
func (a *ASCIISet) Horizontal() string    { return "-" }
func (a *ASCIISet) Vertical() string      { return "|" }
func (a *ASCIISet) LeftVertical() string  { return "|" }
func (a *ASCIISet) RightVertical() string { return "|" }
func (a *ASCIISet) Backslash() string     { return "\\" }
func (a *ASCIISet) ForwardSlash() string  { return "/" }
func (a *ASCIISet) Cross() string         { return "X" }

// Junctions
func (a *ASCIISet) TDown() string  { return "+" }
func (a *ASCIISet) TLeft() string  { return "+" }
func (a *ASCIISet) TRight() string { return "+" }
func (a *ASCIISet) TUp() string    { return "+" }

// Other
func (a *ASCIISet) Underscore() string { return "_" }
func (a *ASCIISet) Overline() string   { return "-" }
func (a *ASCIISet) Dot() string        { return "." }
func (a *ASCIISet) Hyphen() string     { return "-" }
func (a *ASCIISet) Tilde() string      { return "~" }

// Symbols
func (a *ASCIISet) Cloud() string  { return "@" }
func (a *ASCIISet) Circle() string { return "o" }
func (a *ASCIISet) Oval() string   { return "O" }
func (a *ASCIISet) Star() string   { return "*" }

// Arrows
func (a *ASCIISet) ArrowUp() string    { return "^" }
func (a *ASCIISet) ArrowRight() string { return ">" }
func (a *ASCIISet) ArrowDown() string  { return "v" }
func (a *ASCIISet) ArrowLeft() string  { return "<" }
