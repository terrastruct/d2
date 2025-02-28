// TODO: Remove boxes and cleanup like d2ir
//
// d2ast implements the d2 language's abstract syntax tree.
//
// Special characters to think about in parser:
// #
// """
// ;
// []
// {}
// |
// $
// '
// "
// \
// :
// .
// --
// <>
// *
// &
// ()
package d2ast

import (
	"bytes"
	"encoding"
	"errors"
	"fmt"
	"math/big"
	"path"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"oss.terrastruct.com/util-go/xdefer"
)

// Node is the base interface implemented by all d2 AST nodes.
// TODO: add error node for autofmt of incomplete AST
type Node interface {
	node()

	// Type returns the user friendly name of the node.
	Type() string

	// GetRange returns the range a node occupies in its file.
	GetRange() Range

	Children() []Node
}

var _ Node = &Comment{}
var _ Node = &BlockComment{}

var _ Node = &Null{}
var _ Node = &Suspension{}
var _ Node = &Boolean{}
var _ Node = &Number{}
var _ Node = &UnquotedString{}
var _ Node = &DoubleQuotedString{}
var _ Node = &SingleQuotedString{}
var _ Node = &BlockString{}
var _ Node = &Substitution{}
var _ Node = &Import{}

var _ Node = &Array{}
var _ Node = &Map{}
var _ Node = &Key{}
var _ Node = &KeyPath{}
var _ Node = &Edge{}
var _ Node = &EdgeIndex{}

// Range represents a range between Start and End in Path.
// It's also used in the d2parser package to represent the range of an error.
//
// note: See docs on Position.
//
// It has a custom JSON string encoding with encoding.TextMarshaler and
// encoding.TextUnmarshaler to keep it compact as the JSON struct encoding is too verbose,
// especially with json.MarshalIndent.
//
// It looks like path,start-end
type Range struct {
	Path  string
	Start Position
	End   Position
}

var _ fmt.Stringer = Range{}
var _ encoding.TextMarshaler = Range{}
var _ encoding.TextUnmarshaler = &Range{}

func MakeRange(s string) Range {
	var r Range
	_ = r.UnmarshalText([]byte(s))
	return r
}

// String returns a string representation of the range including only the path and start.
//
// If path is empty, it will be omitted.
//
// The format is path:start
func (r Range) String() string {
	var s strings.Builder
	if r.Path != "" {
		s.WriteString(r.Path)
		s.WriteByte(':')
	}

	s.WriteString(r.Start.String())
	return s.String()
}

// OneLine returns true if the Range starts and ends on the same line.
func (r Range) OneLine() bool {
	return r.Start.Line == r.End.Line
}

// See docs on Range.
func (r Range) MarshalText() ([]byte, error) {
	start, _ := r.Start.MarshalText()
	end, _ := r.End.MarshalText()
	return []byte(fmt.Sprintf("%s,%s-%s", r.Path, start, end)), nil
}

// See docs on Range.
func (r *Range) UnmarshalText(b []byte) (err error) {
	defer xdefer.Errorf(&err, "failed to unmarshal Range from %q", b)

	i := bytes.LastIndexByte(b, '-')
	if i == -1 {
		return errors.New("missing End field")
	}
	end := b[i+1:]
	b = b[:i]

	i = bytes.LastIndexByte(b, ',')
	if i == -1 {
		return errors.New("missing Start field")
	}
	start := b[i+1:]
	b = b[:i]

	r.Path = string(b)
	err = r.Start.UnmarshalText(start)
	if err != nil {
		return err
	}
	return r.End.UnmarshalText(end)
}

func (r Range) Before(r2 Range) bool {
	return r.Start.Before(r2.Start)
}

// Position represents a line:column and byte position in a file.
//
// note: Line and Column are zero indexed.
// note: Column and Byte are UTF-8 byte indexes unless byUTF16 was passed to Position.Advance in
// .     which they are UTF-16 code unit indexes.
// .     If intended for Javascript consumption like in the browser or via LSP, byUTF16 is
// .     set to true.
type Position struct {
	Line   int
	Column int
	// -1 is used as sentinel that a constructed position is missing byte offset (for LSP usage)
	Byte int
}

var _ fmt.Stringer = Position{}
var _ encoding.TextMarshaler = Position{}
var _ encoding.TextUnmarshaler = &Position{}

// String returns a line:column representation of the position suitable for error messages.
//
// note: Should not normally be used directly, see Range.String()
func (p Position) String() string {
	return fmt.Sprintf("%d:%d", p.Line+1, p.Column+1)
}

func (p Position) Debug() string {
	return fmt.Sprintf("%d:%d:%d", p.Line, p.Column, p.Byte)
}

// See docs on Range.
func (p Position) MarshalText() ([]byte, error) {
	return []byte(fmt.Sprintf("%d:%d:%d", p.Line, p.Column, p.Byte)), nil
}

// See docs on Range.
func (p *Position) UnmarshalText(b []byte) (err error) {
	defer xdefer.Errorf(&err, "failed to unmarshal Position from %q", b)

	fields := bytes.Split(b, []byte{':'})
	if len(fields) != 3 {
		return errors.New("expected three fields")
	}

	p.Line, err = strconv.Atoi(string(fields[0]))
	if err != nil {
		return err
	}
	p.Column, err = strconv.Atoi(string(fields[1]))
	if err != nil {
		return err
	}
	p.Byte, err = strconv.Atoi(string(fields[2]))
	return err
}

// From copies src into p. It's used in the d2parser package to set a node's Range.End to
// the parser's current pos on all return paths with defer.
func (p *Position) From(src *Position) {
	*p = *src
}

// Advance advances p's Line, Column and Byte by r and returns
// the new Position.
// Set byUTF16 to advance the position as though r represents
// a UTF-16 codepoint.
func (p Position) Advance(r rune, byUTF16 bool) Position {
	size := utf8.RuneLen(r)
	if byUTF16 {
		size = 1
		r1, r2 := utf16.EncodeRune(r)
		if r1 != '\uFFFD' && r2 != '\uFFFD' {
			size = 2
		}
	}

	if r == '\n' {
		p.Line++
		p.Column = 0
	} else {
		p.Column += size
	}
	p.Byte += size

	return p
}

func (p Position) Subtract(r rune, byUTF16 bool) Position {
	size := utf8.RuneLen(r)
	if byUTF16 {
		size = 1
		r1, r2 := utf16.EncodeRune(r)
		if r1 != '\uFFFD' && r2 != '\uFFFD' {
			size = 2
		}
	}

	if r == '\n' {
		panic("d2ast: cannot subtract newline from Position")
	} else {
		p.Column -= size
	}
	p.Byte -= size

	return p
}

func (p Position) AdvanceString(s string, byUTF16 bool) Position {
	for _, r := range s {
		p = p.Advance(r, byUTF16)
	}
	return p
}

func (p Position) SubtractString(s string, byUTF16 bool) Position {
	for _, r := range s {
		p = p.Subtract(r, byUTF16)
	}
	return p
}

func (p Position) Before(p2 Position) bool {
	if p.Byte != p2.Byte && p.Byte != -1 && p2.Byte != -1 {
		return p.Byte < p2.Byte
	}
	if p.Line != p2.Line {
		return p.Line < p2.Line
	}
	return p.Column < p2.Column
}

// MapNode is implemented by nodes that may be children of Maps.
type MapNode interface {
	Node
	mapNode()
}

var _ MapNode = &Comment{}
var _ MapNode = &BlockComment{}
var _ MapNode = &Key{}
var _ MapNode = &Substitution{}
var _ MapNode = &Import{}

// ArrayNode is implemented by nodes that may be children of Arrays.
type ArrayNode interface {
	Node
	arrayNode()
}

// See Value for the rest.
var _ ArrayNode = &Comment{}
var _ ArrayNode = &BlockComment{}
var _ ArrayNode = &Substitution{}
var _ ArrayNode = &Import{}

// Value is implemented by nodes that may be values of a key.
type Value interface {
	ArrayNode
	value()
}

// See Scalar for rest.
var _ Value = &Array{}
var _ Value = &Map{}

// Scalar is implemented by nodes that represent scalar values.
type Scalar interface {
	Value
	scalar()
	ScalarString() string
}

// See String for rest.
var _ Scalar = &Null{}
var _ Scalar = &Suspension{}
var _ Scalar = &Boolean{}
var _ Scalar = &Number{}

// String is implemented by nodes that represent strings.
type String interface {
	Scalar
	SetString(string)
	Copy() String
	_string()
	IsUnquoted() bool
}

var _ String = &UnquotedString{}
var _ String = &SingleQuotedString{}
var _ String = &DoubleQuotedString{}
var _ String = &BlockString{}

func (c *Comment) node()            {}
func (c *BlockComment) node()       {}
func (n *Null) node()               {}
func (n *Suspension) node()         {}
func (b *Boolean) node()            {}
func (n *Number) node()             {}
func (s *UnquotedString) node()     {}
func (s *DoubleQuotedString) node() {}
func (s *SingleQuotedString) node() {}
func (s *BlockString) node()        {}
func (s *Substitution) node()       {}
func (i *Import) node()             {}
func (a *Array) node()              {}
func (m *Map) node()                {}
func (k *Key) node()                {}
func (k *KeyPath) node()            {}
func (e *Edge) node()               {}
func (i *EdgeIndex) node()          {}

func (c *Comment) Type() string            { return "comment" }
func (c *BlockComment) Type() string       { return "block comment" }
func (n *Null) Type() string               { return "null" }
func (n *Suspension) Type() string         { return "suspension" }
func (b *Boolean) Type() string            { return "boolean" }
func (n *Number) Type() string             { return "number" }
func (s *UnquotedString) Type() string     { return "unquoted string" }
func (s *DoubleQuotedString) Type() string { return "double quoted string" }
func (s *SingleQuotedString) Type() string { return "single quoted string" }
func (s *BlockString) Type() string        { return s.Tag + " block string" }
func (s *Substitution) Type() string       { return "substitution" }
func (i *Import) Type() string             { return "import" }
func (a *Array) Type() string              { return "array" }
func (m *Map) Type() string                { return "map" }
func (k *Key) Type() string                { return "map key" }
func (k *KeyPath) Type() string            { return "key path" }
func (e *Edge) Type() string               { return "edge" }
func (i *EdgeIndex) Type() string          { return "edge index" }

func (c *Comment) GetRange() Range            { return c.Range }
func (c *BlockComment) GetRange() Range       { return c.Range }
func (n *Null) GetRange() Range               { return n.Range }
func (n *Suspension) GetRange() Range         { return n.Range }
func (b *Boolean) GetRange() Range            { return b.Range }
func (n *Number) GetRange() Range             { return n.Range }
func (s *UnquotedString) GetRange() Range     { return s.Range }
func (s *DoubleQuotedString) GetRange() Range { return s.Range }
func (s *SingleQuotedString) GetRange() Range { return s.Range }
func (s *BlockString) GetRange() Range        { return s.Range }
func (s *Substitution) GetRange() Range       { return s.Range }
func (i *Import) GetRange() Range             { return i.Range }
func (a *Array) GetRange() Range              { return a.Range }
func (m *Map) GetRange() Range                { return m.Range }
func (k *Key) GetRange() Range                { return k.Range }
func (k *KeyPath) GetRange() Range            { return k.Range }
func (e *Edge) GetRange() Range               { return e.Range }
func (i *EdgeIndex) GetRange() Range          { return i.Range }

func (c *Comment) mapNode()      {}
func (c *BlockComment) mapNode() {}
func (k *Key) mapNode()          {}
func (s *Substitution) mapNode() {}
func (i *Import) mapNode()       {}

func (c *Comment) arrayNode()            {}
func (c *BlockComment) arrayNode()       {}
func (n *Null) arrayNode()               {}
func (n *Suspension) arrayNode()         {}
func (b *Boolean) arrayNode()            {}
func (n *Number) arrayNode()             {}
func (s *UnquotedString) arrayNode()     {}
func (s *DoubleQuotedString) arrayNode() {}
func (s *SingleQuotedString) arrayNode() {}
func (s *BlockString) arrayNode()        {}
func (s *Substitution) arrayNode()       {}
func (i *Import) arrayNode()             {}
func (a *Array) arrayNode()              {}
func (m *Map) arrayNode()                {}

func (n *Null) value()               {}
func (n *Suspension) value()         {}
func (b *Boolean) value()            {}
func (n *Number) value()             {}
func (s *UnquotedString) value()     {}
func (s *DoubleQuotedString) value() {}
func (s *SingleQuotedString) value() {}
func (s *BlockString) value()        {}
func (a *Array) value()              {}
func (m *Map) value()                {}
func (i *Import) value()             {}

func (n *Null) scalar()               {}
func (n *Suspension) scalar()         {}
func (b *Boolean) scalar()            {}
func (n *Number) scalar()             {}
func (s *UnquotedString) scalar()     {}
func (s *DoubleQuotedString) scalar() {}
func (s *SingleQuotedString) scalar() {}
func (s *BlockString) scalar()        {}

func (c *Comment) Children() []Node            { return nil }
func (c *BlockComment) Children() []Node       { return nil }
func (n *Null) Children() []Node               { return nil }
func (n *Suspension) Children() []Node         { return nil }
func (b *Boolean) Children() []Node            { return nil }
func (n *Number) Children() []Node             { return nil }
func (s *SingleQuotedString) Children() []Node { return nil }
func (s *BlockString) Children() []Node        { return nil }
func (ei *EdgeIndex) Children() []Node         { return nil }

func (s *UnquotedString) Children() []Node {
	var children []Node
	for _, box := range s.Value {
		if box.Substitution != nil {
			children = append(children, box.Substitution)
		}
	}
	return children
}

func (s *DoubleQuotedString) Children() []Node {
	var children []Node
	for _, box := range s.Value {
		if box.Substitution != nil {
			children = append(children, box.Substitution)
		}
	}
	return children
}

func (s *Substitution) Children() []Node {
	var children []Node
	for _, sb := range s.Path {
		if sb != nil {
			if child := sb.Unbox(); child != nil {
				children = append(children, child)
			}
		}
	}
	return children
}

func (i *Import) Children() []Node {
	var children []Node
	for _, sb := range i.Path {
		if sb != nil {
			if child := sb.Unbox(); child != nil {
				children = append(children, child)
			}
		}
	}
	return children
}

func (a *Array) Children() []Node {
	var children []Node
	for _, box := range a.Nodes {
		if child := box.Unbox(); child != nil {
			children = append(children, child)
		}
	}
	return children
}

func (m *Map) Children() []Node {
	var children []Node
	for _, box := range m.Nodes {
		if child := box.Unbox(); child != nil {
			children = append(children, child)
		}
	}
	return children
}

func (k *Key) Children() []Node {
	var children []Node
	if k.Key != nil {
		children = append(children, k.Key)
	}
	for _, edge := range k.Edges {
		if edge != nil {
			children = append(children, edge)
		}
	}
	if k.EdgeIndex != nil {
		children = append(children, k.EdgeIndex)
	}
	if k.EdgeKey != nil {
		children = append(children, k.EdgeKey)
	}
	if scalar := k.Primary.Unbox(); scalar != nil {
		children = append(children, scalar)
	}
	if value := k.Value.Unbox(); value != nil {
		children = append(children, value)
	}
	return children
}

func (kp *KeyPath) Children() []Node {
	var children []Node
	for _, sb := range kp.Path {
		if sb != nil {
			if child := sb.Unbox(); child != nil {
				children = append(children, child)
			}
		}
	}
	return children
}

func (e *Edge) Children() []Node {
	var children []Node
	if e.Src != nil {
		children = append(children, e.Src)
	}
	if e.Dst != nil {
		children = append(children, e.Dst)
	}
	return children
}

func Walk(node Node, fn func(Node) bool) {
	if node == nil {
		return
	}
	if !fn(node) {
		return
	}
	for _, child := range node.Children() {
		Walk(child, fn)
	}
}

// TODO: mistake, move into parse.go
func (n *Null) ScalarString() string       { return "" }
func (n *Suspension) ScalarString() string { return "" }
func (b *Boolean) ScalarString() string    { return strconv.FormatBool(b.Value) }
func (n *Number) ScalarString() string     { return n.Raw }
func (s *UnquotedString) ScalarString() string {
	if len(s.Value) == 0 {
		return ""
	}
	if s.Value[0].String == nil {
		return ""
	}
	return *s.Value[0].String
}
func (s *DoubleQuotedString) ScalarString() string {
	if len(s.Value) == 0 {
		return ""
	}
	if s.Value[0].String == nil {
		return ""
	}
	return *s.Value[0].String
}
func (s *SingleQuotedString) ScalarString() string { return s.Value }
func (s *BlockString) ScalarString() string        { return s.Value }

func (s *UnquotedString) SetString(s2 string)     { s.Value = []InterpolationBox{{String: &s2}} }
func (s *DoubleQuotedString) SetString(s2 string) { s.Value = []InterpolationBox{{String: &s2}} }
func (s *SingleQuotedString) SetString(s2 string) { s.Raw = ""; s.Value = s2 }
func (s *BlockString) SetString(s2 string)        { s.Value = s2 }

func (s *UnquotedString) Copy() String     { tmp := *s; return &tmp }
func (s *DoubleQuotedString) Copy() String { tmp := *s; return &tmp }
func (s *SingleQuotedString) Copy() String { tmp := *s; return &tmp }
func (s *BlockString) Copy() String        { tmp := *s; return &tmp }

func (s *UnquotedString) _string()     {}
func (s *DoubleQuotedString) _string() {}
func (s *SingleQuotedString) _string() {}
func (s *BlockString) _string()        {}

func (s *UnquotedString) IsUnquoted() bool     { return true }
func (s *DoubleQuotedString) IsUnquoted() bool { return false }
func (s *SingleQuotedString) IsUnquoted() bool { return false }
func (s *BlockString) IsUnquoted() bool        { return false }

type Comment struct {
	Range Range  `json:"range"`
	Value string `json:"value"`
}

type BlockComment struct {
	Range Range  `json:"range"`
	Value string `json:"value"`
}

type Null struct {
	Range Range `json:"range"`
}

type Suspension struct {
	Range Range `json:"range"`
	Value bool  `json:"value"`
}

type Boolean struct {
	Range Range `json:"range"`
	Value bool  `json:"value"`
}

type Number struct {
	Range Range    `json:"range"`
	Raw   string   `json:"raw"`
	Value *big.Rat `json:"value"`
}

type UnquotedString struct {
	Range Range              `json:"range"`
	Value []InterpolationBox `json:"value"`
	// Pattern holds the parsed glob pattern if in a key and the unquoted string represents a valid pattern.
	Pattern []string `json:"pattern,omitempty"`
}

func (s *UnquotedString) Coalesce() {
	var b strings.Builder
	for _, box := range s.Value {
		if box.String == nil {
			break
		}
		b.WriteString(*box.String)
	}
	s.SetString(b.String())
}

func FlatUnquotedString(s string) *UnquotedString {
	return &UnquotedString{
		Value: []InterpolationBox{{String: &s}},
	}
}

type DoubleQuotedString struct {
	Range Range              `json:"range"`
	Value []InterpolationBox `json:"value"`
}

func (s *DoubleQuotedString) Coalesce() {
	var b strings.Builder
	for _, box := range s.Value {
		if box.String == nil {
			break
		}
		b.WriteString(*box.String)
	}
	s.SetString(b.String())
}

func FlatDoubleQuotedString(s string) *DoubleQuotedString {
	return &DoubleQuotedString{
		Value: []InterpolationBox{{String: &s}},
	}
}

type SingleQuotedString struct {
	Range Range  `json:"range"`
	Raw   string `json:"raw"`
	Value string `json:"value"`
}

type BlockString struct {
	Range Range `json:"range"`

	// Quote contains the pipe delimiter for the block string.
	// e.g. if 5 pipes were used to begin a block string, then Quote == "||||".
	// The tag is not included.
	Quote string `json:"quote"`
	Tag   string `json:"tag"`
	Value string `json:"value"`
}

type Array struct {
	Range Range          `json:"range"`
	Nodes []ArrayNodeBox `json:"nodes"`
}

type Map struct {
	Range Range        `json:"range"`
	Nodes []MapNodeBox `json:"nodes"`
}

func (m *Map) InsertAfter(cursor, n MapNode) {
	afterIndex := len(m.Nodes) - 1

	for i, n := range m.Nodes {
		if n.Unbox() == cursor {
			afterIndex = i
		}
	}

	a := make([]MapNodeBox, 0, len(m.Nodes))
	a = append(a, m.Nodes[:afterIndex+1]...)
	a = append(a, MakeMapNodeBox(n))
	a = append(a, m.Nodes[afterIndex+1:]...)
	m.Nodes = a
}

func (m *Map) InsertBefore(cursor, n MapNode) {
	beforeIndex := len(m.Nodes)

	for i, n := range m.Nodes {
		if n.Unbox() == cursor {
			beforeIndex = i
		}
	}

	a := make([]MapNodeBox, 0, len(m.Nodes))
	a = append(a, m.Nodes[:beforeIndex]...)
	a = append(a, MakeMapNodeBox(n))
	a = append(a, m.Nodes[beforeIndex:]...)
	m.Nodes = a
}

func (m *Map) IsFileMap() bool {
	return m.Range.Start.Line == 0 && m.Range.Start.Column == 0
}

func (m *Map) HasFilter() bool {
	for _, n := range m.Nodes {
		if n.MapKey != nil && (n.MapKey.Ampersand || n.MapKey.NotAmpersand) {
			return true
		}
	}
	return false
}

// TODO: require @ on import values for readability
type Key struct {
	Range Range `json:"range"`

	// Indicates this MapKey is a filter selector.
	Ampersand bool `json:"ampersand,omitempty"`

	// Indicates this MapKey is a not filter selector.
	NotAmpersand bool `json:"not_ampersand,omitempty"`

	// At least one of Key and Edges will be set but all four can also be set.
	// The following are all valid MapKeys:
	// Key:
	//   x
	// Edges:
	//   x -> y
	// Edges and EdgeIndex:
	//   (x -> y)[*]
	// Edges and EdgeKey:
	//   (x -> y).label
	// Key and Edges:
	//   container.(x -> y)
	// Key, Edges and EdgeKey:
	//   container.(x -> y -> z).label
	// Key, Edges, EdgeIndex EdgeKey:
	//   container.(x -> y -> z)[4].label
	Key       *KeyPath   `json:"key,omitempty"`
	Edges     []*Edge    `json:"edges,omitempty"`
	EdgeIndex *EdgeIndex `json:"edge_index,omitempty"`
	EdgeKey   *KeyPath   `json:"edge_key,omitempty"`

	Primary ScalarBox `json:"primary,omitempty"`
	Value   ValueBox  `json:"value"`
}

func (mk1 *Key) D2OracleEquals(mk2 *Key) bool {
	if mk1 == nil && mk2 == nil {
		return true
	}
	if (mk1 == nil) || (mk2 == nil) {
		return false
	}
	if mk1.Ampersand != mk2.Ampersand {
		return false
	}
	if mk1.NotAmpersand != mk2.NotAmpersand {
		return false
	}
	if (mk1.Key == nil) != (mk2.Key == nil) {
		return false
	}
	if (mk1.EdgeIndex == nil) != (mk2.EdgeIndex == nil) {
		return false
	}
	if (mk1.EdgeKey == nil) != (mk2.EdgeKey == nil) {
		return false
	}
	if len(mk1.Edges) != len(mk2.Edges) {
		return false
	}
	if (mk1.Value.Map == nil) != (mk2.Value.Map == nil) {
		if mk1.Value.Map != nil && len(mk1.Value.Map.Nodes) > 0 {
			return false
		}
		if mk2.Value.Map != nil && len(mk2.Value.Map.Nodes) > 0 {
			return false
		}
	} else if (mk1.Value.Unbox() == nil) != (mk2.Value.Unbox() == nil) {
		return false
	}

	if mk1.Key != nil {
		if len(mk1.Key.Path) != len(mk2.Key.Path) {
			return false
		}
		for i, id := range mk1.Key.Path {
			if id.Unbox().ScalarString() != mk2.Key.Path[i].Unbox().ScalarString() {
				return false
			}
		}
	}
	if mk1.EdgeKey != nil {
		if len(mk1.EdgeKey.Path) != len(mk2.EdgeKey.Path) {
			return false
		}
		for i, id := range mk1.EdgeKey.Path {
			if id.Unbox().ScalarString() != mk2.EdgeKey.Path[i].Unbox().ScalarString() {
				return false
			}
		}
	}

	if mk1.Value.Map != nil && len(mk1.Value.Map.Nodes) > 0 {
		if len(mk1.Value.Map.Nodes) != len(mk2.Value.Map.Nodes) {
			return false
		}
		for i := range mk1.Value.Map.Nodes {
			if !mk1.Value.Map.Nodes[i].MapKey.Equals(mk2.Value.Map.Nodes[i].MapKey) {
				return false
			}
		}
	}

	if mk1.Value.Unbox() != nil {
		if (mk1.Value.ScalarBox().Unbox() == nil) != (mk2.Value.ScalarBox().Unbox() == nil) {
			return false
		}
		if mk1.Value.ScalarBox().Unbox() != nil {
			if mk1.Value.ScalarBox().Unbox().ScalarString() != mk2.Value.ScalarBox().Unbox().ScalarString() {
				return false
			}
		}
	}

	return true
}

func (mk1 *Key) Equals(mk2 *Key) bool {
	if mk1 == nil && mk2 == nil {
		return true
	}
	if (mk1 == nil) || (mk2 == nil) {
		return false
	}
	if mk1.Ampersand != mk2.Ampersand {
		return false
	}
	if mk1.NotAmpersand != mk2.NotAmpersand {
		return false
	}
	if (mk1.Key == nil) != (mk2.Key == nil) {
		return false
	}
	if (mk1.EdgeIndex == nil) != (mk2.EdgeIndex == nil) {
		return false
	}
	if mk1.EdgeIndex != nil {
		if !mk1.EdgeIndex.Equals(mk2.EdgeIndex) {
			return false
		}
	}
	if (mk1.EdgeKey == nil) != (mk2.EdgeKey == nil) {
		return false
	}
	if len(mk1.Edges) != len(mk2.Edges) {
		return false
	}
	for i := range mk1.Edges {
		if !mk1.Edges[i].Equals(mk2.Edges[i]) {
			return false
		}
	}
	if (mk1.Value.Map == nil) != (mk2.Value.Map == nil) {
		if mk1.Value.Map != nil && len(mk1.Value.Map.Nodes) > 0 {
			return false
		}
		if mk2.Value.Map != nil && len(mk2.Value.Map.Nodes) > 0 {
			return false
		}
	} else if (mk1.Value.Unbox() == nil) != (mk2.Value.Unbox() == nil) {
		return false
	}

	if mk1.Key != nil {
		if len(mk1.Key.Path) != len(mk2.Key.Path) {
			return false
		}
		for i, id := range mk1.Key.Path {
			if id.Unbox().ScalarString() != mk2.Key.Path[i].Unbox().ScalarString() {
				return false
			}
		}
	}
	if mk1.EdgeKey != nil {
		if len(mk1.EdgeKey.Path) != len(mk2.EdgeKey.Path) {
			return false
		}
		for i, id := range mk1.EdgeKey.Path {
			if id.Unbox().ScalarString() != mk2.EdgeKey.Path[i].Unbox().ScalarString() {
				return false
			}
		}
	}

	if mk1.Value.Map != nil && len(mk1.Value.Map.Nodes) > 0 {
		if len(mk1.Value.Map.Nodes) != len(mk2.Value.Map.Nodes) {
			return false
		}
		for i := range mk1.Value.Map.Nodes {
			if !mk1.Value.Map.Nodes[i].MapKey.Equals(mk2.Value.Map.Nodes[i].MapKey) {
				return false
			}
		}
	}

	if mk1.Value.Unbox() != nil {
		if (mk1.Value.ScalarBox().Unbox() == nil) != (mk2.Value.ScalarBox().Unbox() == nil) {
			return false
		}
		if mk1.Value.ScalarBox().Unbox() != nil {
			if mk1.Value.ScalarBox().Unbox().ScalarString() != mk2.Value.ScalarBox().Unbox().ScalarString() {
				return false
			}
		}
	}

	if mk1.Primary.Unbox() != nil {
		if (mk1.Primary.Unbox() == nil) != (mk2.Primary.Unbox() == nil) {
			return false
		}
		if mk1.Primary.ScalarString() != mk2.Primary.ScalarString() {
			return false
		}
	}

	return true
}

func (mk *Key) SetScalar(scalar ScalarBox) {
	if mk.Value.Unbox() != nil && mk.Value.ScalarBox().Unbox() == nil {
		mk.Primary = scalar
	} else {
		mk.Value = MakeValueBox(scalar.Unbox())
	}
}

func (mk *Key) HasGlob() bool {
	if mk.Key.HasGlob() {
		return true
	}
	for _, e := range mk.Edges {
		if e.Src.HasGlob() || e.Dst.HasGlob() {
			return true
		}
	}
	if mk.EdgeIndex != nil && mk.EdgeIndex.Glob {
		return true
	}
	if mk.EdgeKey.HasGlob() {
		return true
	}
	return false
}

func (mk *Key) HasTripleGlob() bool {
	if mk.Key.HasTripleGlob() {
		return true
	}
	for _, e := range mk.Edges {
		if e.Src.HasTripleGlob() || e.Dst.HasTripleGlob() {
			return true
		}
	}
	if mk.EdgeKey.HasTripleGlob() {
		return true
	}
	return false
}

func (mk *Key) HasMultiGlob() bool {
	if mk.Key.HasMultiGlob() {
		return true
	}
	for _, e := range mk.Edges {
		if e.Src.HasMultiGlob() || e.Dst.HasMultiGlob() {
			return true
		}
	}
	if mk.EdgeKey.HasMultiGlob() {
		return true
	}
	return false
}

func (mk *Key) SupportsGlobFilters() bool {
	if mk.Key.HasGlob() && len(mk.Edges) == 0 {
		return true
	}
	if mk.EdgeIndex != nil && mk.EdgeIndex.Glob && mk.EdgeKey == nil {
		return true
	}
	if mk.EdgeKey.HasGlob() {
		return true
	}
	return false
}

func (mk *Key) Copy() *Key {
	mk2 := *mk
	return &mk2
}

type KeyPath struct {
	Range Range        `json:"range"`
	Path  []*StringBox `json:"path"`
}

func MakeKeyPath(a []string) *KeyPath {
	kp := &KeyPath{}
	for _, el := range a {
		kp.Path = append(kp.Path, MakeValueBox(RawString(el, true)).StringBox())
	}
	return kp
}

func MakeKeyPathString(a []String) *KeyPath {
	kp := &KeyPath{}
	for _, el := range a {
		kp.Path = append(kp.Path, MakeValueBox(RawString(el.ScalarString(), true)).StringBox())
	}
	return kp
}

func (kp *KeyPath) IDA() (ida []String) {
	for _, el := range kp.Path {
		ida = append(ida, el.Unbox())
	}
	return ida
}

func (kp *KeyPath) StringIDA() (ida []string) {
	for _, el := range kp.Path {
		ida = append(ida, el.Unbox().ScalarString())
	}
	return ida
}

func (kp *KeyPath) Copy() *KeyPath {
	kp2 := *kp
	kp2.Path = nil
	kp2.Path = append(kp2.Path, kp.Path...)
	return &kp2
}

func (kp *KeyPath) Last() *StringBox {
	return kp.Path[len(kp.Path)-1]
}

func IsDoubleGlob(pattern []string) bool {
	return len(pattern) == 3 && pattern[0] == "*" && pattern[1] == "" && pattern[2] == "*"
}

func IsTripleGlob(pattern []string) bool {
	return len(pattern) == 5 && pattern[0] == "*" && pattern[1] == "" && pattern[2] == "*" && pattern[3] == "" && pattern[4] == "*"
}

func (kp *KeyPath) HasGlob() bool {
	if kp == nil {
		return false
	}
	for _, el := range kp.Path {
		if el.UnquotedString != nil && len(el.UnquotedString.Pattern) > 0 {
			return true
		}
	}
	return false
}

func (kp *KeyPath) FirstGlob() int {
	if kp == nil {
		return -1
	}
	for i, el := range kp.Path {
		if el.UnquotedString != nil && len(el.UnquotedString.Pattern) > 0 {
			return i
		}
	}
	return -1
}

func (kp *KeyPath) HasTripleGlob() bool {
	if kp == nil {
		return false
	}
	for _, el := range kp.Path {
		if el.UnquotedString != nil && IsTripleGlob(el.UnquotedString.Pattern) {
			return true
		}
	}
	return false
}

func (kp *KeyPath) HasMultiGlob() bool {
	if kp == nil {
		return false
	}
	for _, el := range kp.Path {
		if el.UnquotedString != nil && (IsDoubleGlob(el.UnquotedString.Pattern) || IsTripleGlob(el.UnquotedString.Pattern)) {
			return true
		}
	}
	return false
}

func (kp1 *KeyPath) Equals(kp2 *KeyPath) bool {
	if len(kp1.Path) != len(kp2.Path) {
		return false
	}
	for i, id := range kp1.Path {
		if id.Unbox().ScalarString() != kp2.Path[i].Unbox().ScalarString() {
			return false
		}
	}
	return true
}

type Edge struct {
	Range Range `json:"range"`

	Src *KeyPath `json:"src"`
	// empty, < or *
	SrcArrow string `json:"src_arrow"`

	Dst *KeyPath `json:"dst"`
	// empty, > or *
	DstArrow string `json:"dst_arrow"`
}

func (e1 *Edge) Equals(e2 *Edge) bool {
	if !e1.Src.Equals(e2.Src) {
		return false
	}
	if e1.SrcArrow != e2.SrcArrow {
		return false
	}
	if !e1.Dst.Equals(e2.Dst) {
		return false
	}
	if e1.DstArrow != e2.DstArrow {
		return false
	}
	return true
}

type EdgeIndex struct {
	Range Range `json:"range"`

	// [n] or [*]
	Int  *int `json:"int"`
	Glob bool `json:"glob"`
}

func (ei1 *EdgeIndex) Equals(ei2 *EdgeIndex) bool {
	// TODO probably should be checking the values, but will wait until something breaks to change
	if ei1.Int != ei2.Int {
		return false
	}
	if ei1.Glob != ei2.Glob {
		return false
	}
	return true
}

type Substitution struct {
	Range Range `json:"range"`

	Spread bool         `json:"spread"`
	Path   []*StringBox `json:"path"`
}

type Import struct {
	Range Range `json:"range"`

	Spread bool         `json:"spread"`
	Pre    string       `json:"pre"`
	Path   []*StringBox `json:"path"`
}

// MapNodeBox is used to box MapNode for JSON persistence.
type MapNodeBox struct {
	Comment      *Comment      `json:"comment,omitempty"`
	BlockComment *BlockComment `json:"block_comment,omitempty"`
	Substitution *Substitution `json:"substitution,omitempty"`
	Import       *Import       `json:"import,omitempty"`
	MapKey       *Key          `json:"map_key,omitempty"`
}

func MakeMapNodeBox(n MapNode) MapNodeBox {
	var box MapNodeBox
	switch n := n.(type) {
	case *Comment:
		box.Comment = n
	case *BlockComment:
		box.BlockComment = n
	case *Substitution:
		box.Substitution = n
	case *Import:
		box.Import = n
	case *Key:
		box.MapKey = n
	}
	return box
}

func (mb MapNodeBox) Unbox() MapNode {
	switch {
	case mb.Comment != nil:
		return mb.Comment
	case mb.BlockComment != nil:
		return mb.BlockComment
	case mb.Substitution != nil:
		return mb.Substitution
	case mb.Import != nil:
		return mb.Import
	case mb.MapKey != nil:
		return mb.MapKey
	default:
		return nil
	}
}

func (mb MapNodeBox) IsBoardNode() bool {
	if mb.MapKey == nil || mb.MapKey.Key == nil || len(mb.MapKey.Key.Path) != 1 {
		return false
	}
	switch mb.MapKey.Key.Path[0].Unbox().ScalarString() {
	case "layers", "scenarios", "steps":
		return true
	default:
		return false
	}
}

// ArrayNodeBox is used to box ArrayNode for JSON persistence.
type ArrayNodeBox struct {
	Comment            *Comment            `json:"comment,omitempty"`
	BlockComment       *BlockComment       `json:"block_comment,omitempty"`
	Substitution       *Substitution       `json:"substitution,omitempty"`
	Import             *Import             `json:"import,omitempty"`
	Null               *Null               `json:"null,omitempty"`
	Boolean            *Boolean            `json:"boolean,omitempty"`
	Number             *Number             `json:"number,omitempty"`
	UnquotedString     *UnquotedString     `json:"unquoted_string,omitempty"`
	DoubleQuotedString *DoubleQuotedString `json:"double_quoted_string,omitempty"`
	SingleQuotedString *SingleQuotedString `json:"single_quoted_string,omitempty"`
	BlockString        *BlockString        `json:"block_string,omitempty"`
	Array              *Array              `json:"array,omitempty"`
	Map                *Map                `json:"map,omitempty"`
}

func MakeArrayNodeBox(an ArrayNode) ArrayNodeBox {
	var ab ArrayNodeBox
	switch an := an.(type) {
	case *Comment:
		ab.Comment = an
	case *BlockComment:
		ab.BlockComment = an
	case *Substitution:
		ab.Substitution = an
	case *Import:
		ab.Import = an
	case *Null:
		ab.Null = an
	case *Boolean:
		ab.Boolean = an
	case *Number:
		ab.Number = an
	case *UnquotedString:
		ab.UnquotedString = an
	case *DoubleQuotedString:
		ab.DoubleQuotedString = an
	case *SingleQuotedString:
		ab.SingleQuotedString = an
	case *BlockString:
		ab.BlockString = an
	case *Array:
		ab.Array = an
	case *Map:
		ab.Map = an
	}
	return ab
}

func (ab ArrayNodeBox) Unbox() ArrayNode {
	switch {
	case ab.Comment != nil:
		return ab.Comment
	case ab.BlockComment != nil:
		return ab.BlockComment
	case ab.Substitution != nil:
		return ab.Substitution
	case ab.Import != nil:
		return ab.Import
	case ab.Null != nil:
		return ab.Null
	case ab.Boolean != nil:
		return ab.Boolean
	case ab.Number != nil:
		return ab.Number
	case ab.UnquotedString != nil:
		return ab.UnquotedString
	case ab.DoubleQuotedString != nil:
		return ab.DoubleQuotedString
	case ab.SingleQuotedString != nil:
		return ab.SingleQuotedString
	case ab.BlockString != nil:
		return ab.BlockString
	case ab.Array != nil:
		return ab.Array
	case ab.Map != nil:
		return ab.Map
	default:
		return nil
	}
}

// ValueBox is used to box Value for JSON persistence.
type ValueBox struct {
	Null               *Null               `json:"null,omitempty"`
	Suspension         *Suspension         `json:"suspension,omitempty"`
	Boolean            *Boolean            `json:"boolean,omitempty"`
	Number             *Number             `json:"number,omitempty"`
	UnquotedString     *UnquotedString     `json:"unquoted_string,omitempty"`
	DoubleQuotedString *DoubleQuotedString `json:"double_quoted_string,omitempty"`
	SingleQuotedString *SingleQuotedString `json:"single_quoted_string,omitempty"`
	BlockString        *BlockString        `json:"block_string,omitempty"`
	Array              *Array              `json:"array,omitempty"`
	Map                *Map                `json:"map,omitempty"`
	Import             *Import             `json:"import,omitempty"`
}

func (vb ValueBox) Unbox() Value {
	switch {
	case vb.Null != nil:
		return vb.Null
	case vb.Suspension != nil:
		return vb.Suspension
	case vb.Boolean != nil:
		return vb.Boolean
	case vb.Number != nil:
		return vb.Number
	case vb.UnquotedString != nil:
		return vb.UnquotedString
	case vb.DoubleQuotedString != nil:
		return vb.DoubleQuotedString
	case vb.SingleQuotedString != nil:
		return vb.SingleQuotedString
	case vb.BlockString != nil:
		return vb.BlockString
	case vb.Array != nil:
		return vb.Array
	case vb.Map != nil:
		return vb.Map
	case vb.Import != nil:
		return vb.Import
	default:
		return nil
	}
}

func MakeValueBox(v Value) ValueBox {
	var vb ValueBox
	switch v := v.(type) {
	case *Null:
		vb.Null = v
	case *Suspension:
		vb.Suspension = v
	case *Boolean:
		vb.Boolean = v
	case *Number:
		vb.Number = v
	case *UnquotedString:
		vb.UnquotedString = v
	case *DoubleQuotedString:
		vb.DoubleQuotedString = v
	case *SingleQuotedString:
		vb.SingleQuotedString = v
	case *BlockString:
		vb.BlockString = v
	case *Array:
		vb.Array = v
	case *Map:
		vb.Map = v
	case *Import:
		vb.Import = v
	}
	return vb
}

func (vb ValueBox) ScalarBox() ScalarBox {
	var sb ScalarBox
	sb.Null = vb.Null
	sb.Suspension = vb.Suspension
	sb.Boolean = vb.Boolean
	sb.Number = vb.Number
	sb.UnquotedString = vb.UnquotedString
	sb.DoubleQuotedString = vb.DoubleQuotedString
	sb.SingleQuotedString = vb.SingleQuotedString
	sb.BlockString = vb.BlockString
	return sb
}

func (vb ValueBox) StringBox() *StringBox {
	var sb StringBox
	sb.UnquotedString = vb.UnquotedString
	sb.DoubleQuotedString = vb.DoubleQuotedString
	sb.SingleQuotedString = vb.SingleQuotedString
	sb.BlockString = vb.BlockString
	return &sb
}

// ScalarBox is used to box Scalar for JSON persistence.
// TODO: implement ScalarString()
type ScalarBox struct {
	Null               *Null               `json:"null,omitempty"`
	Suspension         *Suspension         `json:"suspension,omitempty"`
	Boolean            *Boolean            `json:"boolean,omitempty"`
	Number             *Number             `json:"number,omitempty"`
	UnquotedString     *UnquotedString     `json:"unquoted_string,omitempty"`
	DoubleQuotedString *DoubleQuotedString `json:"double_quoted_string,omitempty"`
	SingleQuotedString *SingleQuotedString `json:"single_quoted_string,omitempty"`
	BlockString        *BlockString        `json:"block_string,omitempty"`
}

func (sb ScalarBox) Unbox() Scalar {
	switch {
	case sb.Null != nil:
		return sb.Null
	case sb.Suspension != nil:
		return sb.Suspension
	case sb.Boolean != nil:
		return sb.Boolean
	case sb.Number != nil:
		return sb.Number
	case sb.UnquotedString != nil:
		return sb.UnquotedString
	case sb.DoubleQuotedString != nil:
		return sb.DoubleQuotedString
	case sb.SingleQuotedString != nil:
		return sb.SingleQuotedString
	case sb.BlockString != nil:
		return sb.BlockString
	default:
		return nil
	}
}

func (sb ScalarBox) ScalarString() string {
	return sb.Unbox().ScalarString()
}

// StringBox is used to box String for JSON persistence.
type StringBox struct {
	UnquotedString     *UnquotedString     `json:"unquoted_string,omitempty"`
	DoubleQuotedString *DoubleQuotedString `json:"double_quoted_string,omitempty"`
	SingleQuotedString *SingleQuotedString `json:"single_quoted_string,omitempty"`
	BlockString        *BlockString        `json:"block_string,omitempty"`
}

func (sb *StringBox) Unbox() String {
	switch {
	case sb.UnquotedString != nil:
		return sb.UnquotedString
	case sb.DoubleQuotedString != nil:
		return sb.DoubleQuotedString
	case sb.SingleQuotedString != nil:
		return sb.SingleQuotedString
	case sb.BlockString != nil:
		return sb.BlockString
	default:
		return nil
	}
}

func (sb *StringBox) ScalarString() string {
	return sb.Unbox().ScalarString()
}

// InterpolationBox is used to select between strings and substitutions in unquoted and
// double quoted strings. There is no corresponding interface to avoid unnecessary
// abstraction.
type InterpolationBox struct {
	String       *string       `json:"string,omitempty"`
	StringRaw    *string       `json:"raw_string,omitempty"`
	Substitution *Substitution `json:"substitution,omitempty"`
}

// & is only special if it begins a key.
// - is only special if followed by another - in a key.
// ' " and | are only special if they begin an unquoted key or value.
var UnquotedKeySpecials = string([]rune{'#', ';', '\n', '\\', '{', '}', '[', ']', '\'', '"', '|', ':', '.', '-', '<', '>', '*', '&', '(', ')', '@', '&'})
var UnquotedValueSpecials = string([]rune{'#', ';', '\n', '\\', '{', '}', '[', ']', '\'', '"', '|', '$', '@'})

// RawString returns s in a AST String node that can format s in the most aesthetically
// pleasing way.
func RawString(s string, inKey bool) String {
	if s == "" {
		return FlatDoubleQuotedString(s)
	}

	if inKey {
		for i, r := range s {
			switch r {
			case '-':
				if i+1 < len(s) && s[i+1] != '-' {
					continue
				}
			}
			if strings.ContainsRune(UnquotedKeySpecials, r) {
				if !strings.ContainsRune(s, '"') {
					return FlatDoubleQuotedString(s)
				}
				if strings.ContainsRune(s, '\n') {
					return FlatDoubleQuotedString(s)
				}
				return &SingleQuotedString{Value: s}
			}
		}
	} else if s == "null" || s == "suspend" || s == "unsuspend" || strings.ContainsAny(s, UnquotedValueSpecials) {
		if !strings.ContainsRune(s, '"') && !strings.ContainsRune(s, '$') {
			return FlatDoubleQuotedString(s)
		}
		if strings.ContainsRune(s, '\n') {
			return FlatDoubleQuotedString(s)
		}
		return &SingleQuotedString{Value: s}
	}

	if hasSurroundingWhitespace(s) {
		return FlatDoubleQuotedString(s)
	}

	return FlatUnquotedString(s)
}

func RawStringBox(s string, inKey bool) *StringBox {
	return MakeValueBox(RawString(s, inKey)).StringBox()
}

func hasSurroundingWhitespace(s string) bool {
	r, _ := utf8.DecodeRuneInString(s)
	r2, _ := utf8.DecodeLastRuneInString(s)
	return unicode.IsSpace(r) || unicode.IsSpace(r2)
}

func (s *Substitution) IDA() (ida []string) {
	for _, el := range s.Path {
		ida = append(ida, el.Unbox().ScalarString())
	}
	return ida
}

func (i *Import) IDA() (ida []String) {
	for _, el := range i.Path[1:] {
		ida = append(ida, el.Unbox())
	}
	return ida
}

func (i *Import) PathWithPre() string {
	if len(i.Path) == 0 {
		return ""
	}
	return path.Join(i.Pre, i.Path[0].Unbox().ScalarString())
}

func (i *Import) Dir() string {
	return path.Dir(i.PathWithPre())
}
