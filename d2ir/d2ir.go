// Package d2ir implements a tree data structure to keep track of the resolved value of D2
// keys.
package d2ir

import (
	"errors"
	"fmt"
	"strings"

	"oss.terrastruct.com/util-go/go2"

	"oss.terrastruct.com/d2/d2ast"
	"oss.terrastruct.com/d2/d2format"
	"oss.terrastruct.com/d2/d2parser"
)

// Most errors returned by a node should be created with d2parser.Errorf
// to indicate the offending AST node.
type Node interface {
	node()
	Copy(parent Node) Node
	Parent() Node

	ast() d2ast.Node
	fmt.Stringer
}

var _ Node = &Scalar{}
var _ Node = &Field{}
var _ Node = &Edge{}
var _ Node = &Array{}
var _ Node = &Map{}

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

func (n *Scalar) Parent() Node { return n.parent }
func (n *Field) Parent() Node  { return n.parent }
func (n *Edge) Parent() Node   { return n.parent }
func (n *Array) Parent() Node  { return n.parent }
func (n *Map) Parent() Node    { return n.parent }

func (n *Scalar) value() {}
func (n *Array) value()  {}
func (n *Map) value()    {}

func (n *Array) composite() {}
func (n *Map) composite()   {}

func (n *Scalar) String() string { return d2format.Format(n.ast()) }
func (n *Field) String() string  { return d2format.Format(n.ast()) }
func (n *Edge) String() string   { return d2format.Format(n.ast()) }
func (n *Array) String() string  { return d2format.Format(n.ast()) }
func (n *Map) String() string    { return d2format.Format(n.ast()) }

type Scalar struct {
	parent Node
	Value  d2ast.Scalar `json:"value"`
}

func (s *Scalar) Copy(newp Node) Node {
	tmp := *s
	s = &tmp

	s.parent = newp
	return s
}

func (s *Scalar) Equal(s2 *Scalar) bool {
	if _, ok := s.Value.(d2ast.String); ok {
		if _, ok = s2.Value.(d2ast.String); ok {
			return s.Value.ScalarString() == s2.Value.ScalarString()
		}
	}
	return s.Value.Type() == s2.Value.Type() && s.Value.ScalarString() == s2.Value.ScalarString()

}

type Map struct {
	parent Node
	Fields []*Field `json:"fields"`
	Edges  []*Edge  `json:"edges"`
}

func (m *Map) Copy(newp Node) Node {
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
func (m *Map) Root() bool {
	return ParentMap(m) == nil
}

// Layer reports whether the Map represents the root of a layer.
func (m *Map) Layer() bool {
	f := ParentField(m)
	if f == nil {
		return true
	}
	f = ParentField(f)
	if f == nil {
		return false
	}
	switch f.Name {
	case "layers", "scenarios", "steps":
		return true
	default:
		return false
	}
}

type Field struct {
	parent *Map

	Name string `json:"name"`

	Primary   *Scalar   `json:"primary,omitempty"`
	Composite Composite `json:"composite,omitempty"`

	References []FieldReference `json:"references,omitempty"`
}

func (f *Field) Copy(newp Node) Node {
	tmp := *f
	f = &tmp

	f.parent = newp.(*Map)
	f.References = append([]FieldReference(nil), f.References...)
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

	// If nil, then any EdgeID with equal src/dst/arrows matches.
	Index *int `json:"index"`
}

func NewEdgeIDs(k *d2ast.Key) (eida []*EdgeID) {
	for _, ke := range k.Edges {
		eida = append(eida, &EdgeID{
			SrcPath:  d2format.KeyPath(ke.Src),
			SrcArrow: ke.SrcArrow == "<",
			DstPath:  d2format.KeyPath(ke.Dst),
			DstArrow: ke.DstArrow == ">",
		})
	}
	if k.EdgeIndex != nil && k.EdgeIndex.Int != nil {
		eida[0].Index = k.EdgeIndex.Int
	}
	return eida
}

func (eid *EdgeID) Copy() *EdgeID {
	tmp := *eid
	eid = &tmp

	eid.SrcPath = append([]string(nil), eid.SrcPath...)
	eid.DstPath = append([]string(nil), eid.DstPath...)
	return eid
}

func (eid *EdgeID) Match(eid2 *EdgeID) bool {
	if eid.Index != nil && eid2.Index != nil {
		if *eid.Index != *eid2.Index {
			return false
		}
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

func (eid *EdgeID) resolveUnderscores(m *Map) (*EdgeID, *Map, error) {
	eid = eid.Copy()
	maxUnderscores := go2.Max(countUnderscores(eid.SrcPath), countUnderscores(eid.DstPath))
	for i := 0; i < maxUnderscores; i++ {
		if eid.SrcPath[0] == "_" {
			eid.SrcPath = eid.SrcPath[1:]
		} else {
			mf := ParentField(m)
			eid.SrcPath = append([]string{mf.Name}, eid.SrcPath...)
		}
		if eid.DstPath[0] == "_" {
			eid.DstPath = eid.DstPath[1:]
		} else {
			mf := ParentField(m)
			eid.DstPath = append([]string{mf.Name}, eid.DstPath...)
		}
		m = ParentMap(m)
		if m == nil {
			return nil, nil, errors.New("invalid underscore")
		}
	}
	return eid, m, nil
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

	References []EdgeReference `json:"references,omitempty"`
}

func (e *Edge) Copy(newp Node) Node {
	tmp := *e
	e = &tmp

	e.parent = newp.(*Map)
	e.References = append([]EdgeReference(nil), e.References...)
	if e.Primary != nil {
		e.Primary = e.Primary.Copy(e).(*Scalar)
	}
	if e.Map != nil {
		e.Map = e.Map.Copy(e).(*Map)
	}
	return e
}

type Array struct {
	parent Node
	Values []Value `json:"values"`
}

func (a *Array) Copy(newp Node) Node {
	tmp := *a
	a = &tmp

	a.parent = newp
	a.Values = append([]Value(nil), a.Values...)
	for i := range a.Values {
		a.Values[i] = a.Values[i].Copy(a).(Value)
	}
	return a
}

type FieldReference struct {
	String  d2ast.String `json:"string"`
	KeyPath *d2ast.KeyPath   `json:"key_path"`

	Context *RefContext `json:"context"`
}

func (kr FieldReference) KeyPathIndex() int {
	for i, sb := range kr.KeyPath.Path {
		if sb.Unbox() == kr.String {
			return i
		}
	}
	panic("d2ir.KeyReference.KeyPathIndex: String not in KeyPath?")
}

func (kr FieldReference) EdgeDest() bool {
	return kr.KeyPath == kr.Context.Edge.Dst
}

func (kr FieldReference) InEdge() bool {
	return kr.Context.Edge != nil
}

type EdgeReference struct {
	Context *RefContext `json:"context"`
}

type RefContext struct {
	Key   *d2ast.Key  `json:"key"`
	Edge  *d2ast.Edge `json:"edge"`
	Scope *d2ast.Map  `json:"-"`
}

func (rc *RefContext) Copy() *RefContext {
	tmp := *rc
	return &tmp
}

// UnresolvedScopeMap is scope prior to interpreting _
// It does this by finding the referenced *Map of rc.Scope
func (rc *RefContext) UnresolvedScopeMap(m *Map) *Map {
	for {
		fm := ParentField(m)
		if fm == nil {
			return m
		}
		for _, ref := range fm.References {
			if ref.KeyPath != ref.Context.Key.Key {
				continue
			}
			if ref.Context.Key.Value.Unbox() == rc.Scope {
				return m
			}
		}
		m = ParentMap(m)
	}
}

func (rc *RefContext) EdgeIndex() int {
	for i, e := range rc.Key.Edges {
		if e == rc.Edge {
			return i
		}
	}
	panic("d2ir.RefContext.EdgeIndex: Edge not in Key.Edges?")
}

func (m *Map) FieldCountRecursive() int {
	if m == nil {
		return 0
	}
	acc := len(m.Fields)
	for _, f := range m.Fields {
		f_m := ChildMap(f)
		if f_m != nil {
			acc += f_m.FieldCountRecursive()
		}
	}
	for _, e := range m.Edges {
		if e.Map != nil {
			acc += e.Map.FieldCountRecursive()
		}
	}
	return acc
}

func (m *Map) EdgeCountRecursive() int {
	if m == nil {
		return 0
	}
	acc := len(m.Edges)
	for _, f := range m.Fields {
		f_m := ChildMap(f)
		if f_m != nil {
			acc += f_m.EdgeCountRecursive()
		}
	}
	for _, e := range m.Edges {
		if e.Map != nil {
			acc += e.Map.EdgeCountRecursive()
		}
	}
	return acc
}

func (m *Map) GetField(ida ...string) *Field {
	for len(ida) > 0 && ida[0] == "_" {
		m = ParentMap(m)
		if m == nil {
			return nil
		}
	}
	return m.getField(ida)
}

func (m *Map) getField(ida []string) *Field {
	if len(ida) == 0 {
		return nil
	}

	s := ida[0]
	rest := ida[1:]

	if s == "_" {
		return nil
	}

	for _, f := range m.Fields {
		if !strings.EqualFold(f.Name, s) {
			continue
		}
		if len(rest) == 0 {
			return f
		}
		f_m := ChildMap(f)
		if f_m != nil {
			return f_m.getField(rest)
		}
	}
	return nil
}

func (m *Map) EnsureField(kp *d2ast.KeyPath, refctx *RefContext) (*Field, error) {
	i := 0
	for kp.Path[i].Unbox().ScalarString() == "_" {
		m = ParentMap(m)
		if m == nil {
			return nil, d2parser.Errorf(kp.Path[i].Unbox(), "invalid underscore: no parent")
		}
		if i+1 == len(kp.Path) {
			return nil, d2parser.Errorf(kp.Path[i].Unbox(), "field key must contain more than underscores")
		}
		i++
	}
	return m.ensureField(i, kp, refctx)
}

func (m *Map) ensureField(i int, kp *d2ast.KeyPath, refctx *RefContext) (*Field, error) {
	head := kp.Path[i].Unbox().ScalarString()

	if head == "_" {
		return nil, d2parser.Errorf(kp.Path[i].Unbox(), `parent "_" can only be used in the beginning of paths, e.g. "_.x"`)
	}

	switch head {
	case "layers", "scenarios", "steps":
		if !m.Layer() {
			return nil, d2parser.Errorf(kp.Path[i].Unbox(), "%s is only allowed at a layer root", head)
		}
	}

	for _, f := range m.Fields {
		if !strings.EqualFold(f.Name, head) {
			continue
		}

		f.References = append(f.References, FieldReference{
			String:  kp.Path[i].Unbox(),
			KeyPath: kp,
			Context: refctx,
		})

		if i+1 == len(kp.Path) {
			return f, nil
		}
		if _, ok := f.Composite.(*Array); ok {
			return nil, d2parser.Errorf(kp.Path[i].Unbox(), "cannot index into array")
		}
		f_m := ChildMap(f)
		if f_m == nil {
			f_m = &Map{
				parent: f,
			}
			f.Composite = f_m
		}
		return f_m.ensureField(i+1, kp, refctx)
	}

	f := &Field{
		parent: m,
		Name:   head,
		References: []FieldReference{{
			String:  kp.Path[i].Unbox(),
			KeyPath: kp,
			Context: refctx,
		}},
	}
	m.Fields = append(m.Fields, f)
	if i+1 == len(kp.Path) {
		return f, nil
	}
	f_m := &Map{
		parent: f,
	}
	f.Composite = f_m
	return f_m.ensureField(i+1, kp, refctx)
}

func (m *Map) DeleteField(ida []string) bool {
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
		f_m := ChildMap(f)
		if f_m != nil {
			return f_m.DeleteField(rest)
		}
	}
	return false
}

func (m *Map) GetEdges(eid *EdgeID) []*Edge {
	eid, m, err := eid.resolveUnderscores(m)
	if err != nil {
		return nil
	}
	common, eid := eid.trimCommon()
	if len(common) > 0 {
		f := m.GetField(common...)
		if f == nil {
			return nil
		}
		f_m := ChildMap(f)
		if f_m != nil {
			return f_m.GetEdges(eid)
		}
		return nil
	}

	var ea []*Edge
	for _, e := range m.Edges {
		if e.ID.Match(eid) {
			ea = append(ea, e)
		}
	}
	return ea
}

func (m *Map) CreateEdge(eid *EdgeID, refctx *RefContext) (*Edge, error) {
	eid, m, err := eid.resolveUnderscores(m)
	if err != nil {
		return nil, d2parser.Errorf(refctx.Edge, err.Error())
	}
	common, eid := eid.trimCommon()
	if len(common) > 0 {
		tmp := *refctx.Edge.Src
		kp := &tmp
		kp.Path = kp.Path[:len(common)]
		f, err := m.EnsureField(kp, refctx)
		if err != nil {
			return nil, err
		}
		if _, ok := f.Composite.(*Array); ok {
			return nil, d2parser.Errorf(refctx.Edge.Src, "cannot index into array")
		}
		f_m := ChildMap(f)
		if f_m == nil {
			f_m = &Map{
				parent: f,
			}
			f.Composite = f_m
		}
		return f_m.CreateEdge(eid, refctx)
	}

	eid.Index = nil
	ea := m.GetEdges(eid)
	index := len(ea)
	eid.Index = &index
	e := &Edge{
		parent: m,
		ID:     eid,
		References: []EdgeReference{{
			Context: refctx,
		}},
	}
	m.Edges = append(m.Edges, e)

	return e, nil
}

func (s *Scalar) ast() d2ast.Node {
	return s.Value
}

func (f *Field) ast() d2ast.Node {
	k := &d2ast.Key{
		Key: &d2ast.KeyPath{
			Path: []*d2ast.StringBox{
				d2ast.MakeValueBox(d2ast.RawString(f.Name, true)).StringBox(),
			},
		},
	}

	if f.Primary != nil {
		k.Primary = d2ast.MakeValueBox(f.Primary.ast().(d2ast.Value)).ScalarBox()
	}
	if f.Composite != nil {
		k.Value = d2ast.MakeValueBox(f.Composite.ast().(d2ast.Value))
	}

	return k
}

func (e *Edge) ast() d2ast.Node {
	astEdge := &d2ast.Edge{}

	astEdge.Src = d2ast.MakeKeyPath(e.ID.SrcPath)
	if e.ID.SrcArrow {
		astEdge.SrcArrow = "<"
	}
	astEdge.Dst = d2ast.MakeKeyPath(e.ID.DstPath)
	if e.ID.DstArrow {
		astEdge.DstArrow = ">"
	}

	k := &d2ast.Key{
		Edges: []*d2ast.Edge{astEdge},
	}

	if e.Primary != nil {
		k.Primary = d2ast.MakeValueBox(e.Primary.ast().(d2ast.Value)).ScalarBox()
	}
	if e.Map != nil {
		k.Value = d2ast.MakeValueBox(e.Map.ast().(*d2ast.Map))
	}

	return k
}

func (a *Array) ast() d2ast.Node {
	if a == nil {
		return nil
	}
	astArray := &d2ast.Array{}
	for _, av := range a.Values {
		astArray.Nodes = append(astArray.Nodes, d2ast.MakeArrayNodeBox(av.ast().(d2ast.ArrayNode)))
	}
	return astArray
}

func (m *Map) ast() d2ast.Node {
	if m == nil {
		return nil
	}
	astMap := &d2ast.Map{}
	if m.Root() {
		astMap.Range = d2ast.MakeRange(",0:0:0-1:0:0")
	} else {
		astMap.Range = d2ast.MakeRange(",1:0:0-2:0:0")
	}
	for _, f := range m.Fields {
		astMap.Nodes = append(astMap.Nodes, d2ast.MakeMapNodeBox(f.ast().(d2ast.MapNode)))
	}
	for _, e := range m.Edges {
		astMap.Nodes = append(astMap.Nodes, d2ast.MakeMapNodeBox(e.ast().(d2ast.MapNode)))
	}
	return astMap
}

func (m *Map) appendFieldReferences(i int, kp *d2ast.KeyPath, refctx *RefContext) {
	sb := kp.Path[i]
	f := m.GetField(sb.Unbox().ScalarString())
	if f == nil {
		return
	}

	f.References = append(f.References, FieldReference{
		String:  sb.Unbox(),
		KeyPath: kp,
		Context: refctx,
	})
	if i+1 == len(kp.Path) {
		return
	}
	f_m := ChildMap(f)
	if f_m != nil {
		f_m.appendFieldReferences(i+1, kp, refctx)
	}
}

func ChildMap(n Node) *Map {
	switch n := n.(type) {
	case *Map:
		return n
	case *Field:
		return ChildMap(n.Composite)
	case *Edge:
		return n.Map
	default:
		return nil
	}
}

func ToScalar(n Node) *Scalar {
	switch n := n.(type) {
	case *Field:
		return n.Primary
	case *Edge:
		return n.Primary
	default:
		return nil
	}
}

func ParentMap(n Node) *Map {
	for n.Parent() != nil {
		n = n.Parent()
		if n_m, ok := n.(*Map); ok {
			return n_m
		}
	}
	return nil
}

func ParentField(n Node) *Field {
	for n.Parent() != nil {
		n = n.Parent()
		if n_f, ok := n.(*Field); ok {
			return n_f
		}
	}
	return nil
}

func ParentLayer(n Node) *Map {
	for {
		m := ParentMap(n)
		if m == nil {
			return nil
		}
		if m.Layer() {
			return m
		}
		n = m
	}
}

func countUnderscores(p []string) int {
	var count int
	for _, el := range p {
		if el != "_" {
			break
		}
		count++
	}
	return count
}
