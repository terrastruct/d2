package charset

// UnicodeSet implements the Set interface using Unicode box-drawing characters
type UnicodeSet struct{}

// NewUnicode creates a new Unicode character set
func NewUnicode() Set {
	return &UnicodeSet{}
}

// Corners
func (u *UnicodeSet) TopLeftArc() string     { return "╭" }
func (u *UnicodeSet) TopRightArc() string    { return "╮" }
func (u *UnicodeSet) BottomLeftArc() string  { return "╰" }
func (u *UnicodeSet) BottomRightArc() string { return "╯" }
func (u *UnicodeSet) TopLeftCorner() string  { return "┌" }
func (u *UnicodeSet) TopRightCorner() string { return "┐" }
func (u *UnicodeSet) BottomLeftCorner() string { return "└" }
func (u *UnicodeSet) BottomRightCorner() string { return "┘" }

// Lines
func (u *UnicodeSet) Horizontal() string     { return "─" }
func (u *UnicodeSet) Vertical() string       { return "│" }
func (u *UnicodeSet) LeftVertical() string   { return "▏" }
func (u *UnicodeSet) RightVertical() string  { return "▕" }
func (u *UnicodeSet) Backslash() string      { return "╲" }
func (u *UnicodeSet) ForwardSlash() string   { return "╱" }
func (u *UnicodeSet) Cross() string          { return "╳" }

// Junctions
func (u *UnicodeSet) TDown() string  { return "┬" }
func (u *UnicodeSet) TLeft() string  { return "┤" }
func (u *UnicodeSet) TRight() string { return "├" }
func (u *UnicodeSet) TUp() string    { return "┴" }

// Other
func (u *UnicodeSet) Underscore() string { return "_" }
func (u *UnicodeSet) Overline() string   { return "‾" }
func (u *UnicodeSet) Dot() string        { return "." }
func (u *UnicodeSet) Hyphen() string     { return "-" }
func (u *UnicodeSet) Tilde() string      { return "`" }

// Symbols
func (u *UnicodeSet) Cloud() string  { return "☁" }
func (u *UnicodeSet) Circle() string { return "●" }
func (u *UnicodeSet) Oval() string   { return "⬭" }
func (u *UnicodeSet) Star() string   { return "*" }

// Arrows
func (u *UnicodeSet) ArrowUp() string    { return "▲" }
func (u *UnicodeSet) ArrowRight() string { return "▶" }
func (u *UnicodeSet) ArrowDown() string  { return "▼" }
func (u *UnicodeSet) ArrowLeft() string  { return "◀" }