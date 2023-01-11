package d2ir

import (
	"encoding/json"
	"fmt"
	"strings"

	"oss.terrastruct.com/d2/d2ast"
)

type Node interface {
	node()
	Copy(newp Parent) Node
}

var _ Node = &Scalar{}
var _ Node = &Field{}
var _ Node = &Edge{}
var _ Node = &Array{}
var _ Node = &Map{}

type Parent interface {
	Node
	Parent() Parent
}

var _ Parent = &Field{}
var _ Parent = &Edge{}
var _ Parent = &Array{}
var _ Parent = &Map{}

type Value interface {
	Node
	value()
}

var _ Value = &Scalar{}
var _ Value = &Array{}
var _ Value = &Map{}

type Composite interface {
	Node
	Value
	composite()
}

var _ Composite = &Array{}
var _ Composite = &Map{}

func (n *Scalar) node() {}
func (n *Field) node()  {}
func (n *Edge) node()   {}
func (n *Array) node()  {}
func (n *Map) node()    {}

func (n *Scalar) Parent() Parent { return n.parent }
func (n *Field) Parent() Parent { return n.parent }
func (n *Edge) Parent() Parent  { return n.parent }
func (n *Array) Parent() Parent { return n.parent }
func (n *Map) Parent() Parent   { return n.parent }

func (n *Scalar) value() {}
func (n *Array) value()  {}
func (n *Map) value()    {}

func (n *Array) composite() {}
func (n *Map) composite()   {}

type Scalar struct {
	parent Parent
	Value  d2ast.Scalar `json:"value"`
}

func (s *Scalar) Copy(newp Parent) Node {
	tmp := *s
	s = &tmp

	s.parent = newp
	return s
}

func (s *Scalar) Equal(s2 *Scalar) bool {
	if s == nil {
		if s2 == nil {
			return true
		}
		_, ok := s2.Value.(*d2ast.Null)
		return ok
	}
	if s2 == nil {
		_, ok := s.Value.(*d2ast.Null)
		return ok
	}
	return s.Value.ScalarString() == s2.Value.ScalarString() && s.Value.Type() == s2.Value.Type()
}

type Map struct {
	parent Parent
	Fields []*Field `json:"fields"`
	Edges  []*Edge  `json:"edges"`
}

func (m *Map) Copy(newp Parent) Node {
	tmp := *m
	m = &tmp

	m.parent = newp
	m.Fields = append([]*Field(nil), m.Fields...)
	for i := range m.Fields {
		m.Fields[i] = m.Fields[i].Copy(m).(*Field)
	}
	m.Edges = append([]*Edge(nil), m.Edges...)
	for i := range m.Edges {
		m.Edges[i] = m.Edges[i].Copy(m).(*Edge)
	}
	return m
}

// Root reports whether the Map is the root of the D2 tree.
// The root map has no parent.
func (m *Map) Root() bool {
	return m.parent == nil
}

type Field struct {
	parent *Map

	Name string `json:"name"`

	Primary   *Scalar   `json:"primary,omitempty"`
	Composite Composite `json:"composite,omitempty"`

	Refs []KeyReference `json:"refs,omitempty"`
}

func (f *Field) Copy(newp Parent) Node {
	tmp := *f
	f = &tmp

	f.parent = newp.(*Map)
	f.Refs = append([]KeyReference(nil), f.Refs...)
	if f.Primary != nil {
		f.Primary = f.Primary.Copy(f).(*Scalar)
	}
	if f.Composite != nil {
		f.Composite = f.Composite.Copy(f).(Composite)
	}
	return f
}

type EdgeID struct {
	SrcPath  []string `json:"src_path"`
	SrcArrow bool     `json:"src_arrow"`

	DstPath  []string `json:"dst_path"`
	DstArrow bool     `json:"dst_arrow"`

	Index int `json:"index"`
}

func (eid *EdgeID) Copy() *EdgeID {
	tmp := *eid
	eid = &tmp

	eid.SrcPath = append([]string(nil), eid.SrcPath...)
	eid.DstPath = append([]string(nil), eid.DstPath...)
	return eid
}

func (eid *EdgeID) Equal(eid2 *EdgeID) bool {
	if eid.Index != eid2.Index {
		return false
	}

	if len(eid.SrcPath) != len(eid2.SrcPath) {
		return false
	}
	if eid.SrcArrow != eid2.SrcArrow {
		return false
	}
	for i, s := range eid.SrcPath {
		if !strings.EqualFold(s, eid2.SrcPath[i]) {
			return false
		}
	}

	if len(eid.DstPath) != len(eid2.DstPath) {
		return false
	}
	if eid.DstArrow != eid2.DstArrow {
		return false
	}
	for i, s := range eid.DstPath {
		if !strings.EqualFold(s, eid2.DstPath[i]) {
			return false
		}
	}

	return true
}

func (eid *EdgeID) trimCommon() (common []string, _ *EdgeID) {
	eid = eid.Copy()
	for len(eid.SrcPath) > 1 && len(eid.DstPath) > 1 {
		if !strings.EqualFold(eid.SrcPath[0], eid.DstPath[0]) {
			return common, eid
		}
		common = append(common, eid.SrcPath[0])
		eid.SrcPath = eid.SrcPath[1:]
		eid.DstPath = eid.DstPath[1:]
	}
	return common, eid
}

type Edge struct {
	parent *Map

	ID *EdgeID `json:"edge_id"`

	Primary *Scalar `json:"primary,omitempty"`
	Map     *Map    `json:"map,omitempty"`

	Refs []EdgeReference `json:"refs,omitempty"`
}

func (e *Edge) Copy(newp Parent) Node {
	tmp := *e
	e = &tmp

	e.parent = newp.(*Map)
	e.Refs = append([]EdgeReference(nil), e.Refs...)
	if e.Primary != nil {
		e.Primary = e.Primary.Copy(e).(*Scalar)
	}
	if e.Map != nil {
		e.Map = e.Map.Copy(e).(*Map)
	}
	return e
}

type Array struct {
	parent Parent
	Values []Value `json:"values"`
}

func (a *Array) Copy(newp Parent) Node {
	tmp := *a
	a = &tmp

	a.parent = newp
	a.Values = append([]Value(nil), a.Values...)
	for i := range a.Values {
		a.Values[i] = a.Values[i].Copy(a).(Value)
	}
	return a
}

type KeyReference struct {
	String  *d2ast.StringBox `json:"string"`
	KeyPath *d2ast.KeyPath   `json:"key_path"`

	RefCtx *RefContext `json:"ref_ctx"`
}

type EdgeReference struct {
	RefCtx *RefContext `json:"ref_ctx"`
}

type RefContext struct {
	Key   *d2ast.Key  `json:"-"`
	Edge  *d2ast.Edge `json:"-"`
	Scope *d2ast.Map  `json:"-"`
}

func (m *Map) FieldCount() int {
	acc := len(m.Fields)
	for _, f := range m.Fields {
		if f_m, ok := f.Composite.(*Map); ok {
			acc += f_m.FieldCount()
		}
	}
	return acc
}

func (m *Map) EdgeCount() int {
	acc := len(m.Edges)
	for _, e := range m.Edges {
		if e.Map != nil {
			acc += e.Map.EdgeCount()
		}
	}
	return acc
}

func (m *Map) Get(ida []string) (*Field, bool) {
	if len(ida) == 0 {
		return nil, false
	}

	s := ida[0]
	rest := ida[1:]

	for _, f := range m.Fields {
		if !strings.EqualFold(f.Name, s) {
			continue
		}
		if len(rest) == 0 {
			return f, true
		}
		if f_m, ok := f.Composite.(*Map); ok {
			return f_m.Get(rest)
		}
	}
	return nil, false
}

func (m *Map) Ensure(ida []string) (*Field, bool) {
	if len(ida) == 0 {
		return nil, false
	}

	s := ida[0]
	rest := ida[1:]

	for _, f := range m.Fields {
		if !strings.EqualFold(f.Name, s) {
			continue
		}
		if len(rest) == 0 {
			return f, true
		}
		switch fc := f.Composite.(type) {
		case *Map:
			return fc.Ensure(rest)
		case *Array:
			return nil, false
		}
		f.Composite = &Map{
			parent: f,
		}
		return f.Composite.(*Map).Ensure(rest)
	}

	f := &Field{
		parent: m,
		Name:   s,
	}
	m.Fields = append(m.Fields, f)
	if len(rest) == 0 {
		return f, true
	}
	f.Composite = &Map{
		parent: f,
	}
	return f.Composite.(*Map).Ensure(rest)
}

func (m *Map) Delete(ida []string) bool {
	if len(ida) == 0 {
		return false
	}

	s := ida[0]
	rest := ida[1:]

	for i, f := range m.Fields {
		if !strings.EqualFold(f.Name, s) {
			continue
		}
		if len(rest) == 0 {
			copy(m.Fields[i:], m.Fields[i+1:])
			return true
		}
		if f_m, ok := f.Composite.(*Map); ok {
			return f_m.Delete(rest)
		}
	}
	return false
}

func (m *Map) GetEdge(eid *EdgeID) (*Edge, bool) {
	common, eid := eid.trimCommon()
	if len(common) > 0 {
		f, ok := m.Get(common)
		if !ok {
			return nil, false
		}
		if f_m, ok := f.Composite.(*Map); ok {
			return f_m.GetEdge(eid)
		}
		return nil, false
	}

	for _, e := range m.Edges {
		if e.ID.Equal(eid) {
			return e, true
		}
	}
	return nil, false
}

func (m *Map) String() string {
	b, err := json.Marshal(m)
	if err != nil {
		panic(fmt.Sprintf("d2ir: failed to marshal d2ir.Map: %v", err))
	}
	return string(b)
}
