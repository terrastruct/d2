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
	"oss.terrastruct.com/d2/d2target"
)

// Most errors returned by a node should be created with d2parser.Errorf
// to indicate the offending AST node.
type Node interface {
	node()
	Copy(newParent Node) Node
	Parent() Node
	Primary() *Scalar
	Map() *Map
	Equal(n2 Node) bool

	AST() d2ast.Node
	fmt.Stringer

	LastRef() Reference
	LastPrimaryRef() Reference
	LastPrimaryKey() *d2ast.Key
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

func (n *Scalar) Primary() *Scalar { return n }
func (n *Field) Primary() *Scalar  { return n.Primary_ }
func (n *Edge) Primary() *Scalar   { return n.Primary_ }
func (n *Array) Primary() *Scalar  { return nil }
func (n *Map) Primary() *Scalar    { return nil }

func (n *Scalar) Map() *Map { return nil }
func (n *Field) Map() *Map {
	if n == nil {
		return nil
	}
	if n.Composite == nil {
		return nil
	}
	return n.Composite.Map()
}
func (n *Edge) Map() *Map {
	if n == nil {
		return nil
	}
	return n.Map_
}
func (n *Array) Map() *Map { return nil }
func (n *Map) Map() *Map   { return n }

func (n *Scalar) value() {}
func (n *Array) value()  {}
func (n *Map) value()    {}

func (n *Array) composite() {}
func (n *Map) composite()   {}

func (n *Scalar) String() string { return d2format.Format(n.AST()) }
func (n *Field) String() string  { return d2format.Format(n.AST()) }
func (n *Edge) String() string   { return d2format.Format(n.AST()) }
func (n *Array) String() string  { return d2format.Format(n.AST()) }
func (n *Map) String() string    { return d2format.Format(n.AST()) }

func (n *Scalar) LastRef() Reference { return parentRef(n) }
func (n *Map) LastRef() Reference    { return parentRef(n) }
func (n *Array) LastRef() Reference  { return parentRef(n) }

func (n *Scalar) LastPrimaryRef() Reference { return parentPrimaryRef(n) }
func (n *Map) LastPrimaryRef() Reference    { return parentPrimaryRef(n) }
func (n *Array) LastPrimaryRef() Reference  { return parentPrimaryRef(n) }

func (n *Scalar) LastPrimaryKey() *d2ast.Key { return parentPrimaryKey(n) }
func (n *Map) LastPrimaryKey() *d2ast.Key    { return parentPrimaryKey(n) }
func (n *Array) LastPrimaryKey() *d2ast.Key  { return parentPrimaryKey(n) }

type Reference interface {
	reference()
	// Most specific AST node for the reference.
	AST() d2ast.Node
	Primary() bool
	Context() *RefContext
	// Result of a glob in Context or from above.
	DueToGlob() bool
	DueToLazyGlob() bool
}

var _ Reference = &FieldReference{}
var _ Reference = &EdgeReference{}

func (r *FieldReference) reference()           {}
func (r *EdgeReference) reference()            {}
func (r *FieldReference) Context() *RefContext { return r.Context_ }
func (r *EdgeReference) Context() *RefContext  { return r.Context_ }
func (r *FieldReference) DueToGlob() bool      { return r.DueToGlob_ }
func (r *EdgeReference) DueToGlob() bool       { return r.DueToGlob_ }
func (r *FieldReference) DueToLazyGlob() bool  { return r.DueToLazyGlob_ }
func (r *EdgeReference) DueToLazyGlob() bool   { return r.DueToLazyGlob_ }

type Scalar struct {
	parent Node
	Value  d2ast.Scalar `json:"value"`
}

func (s *Scalar) Copy(newParent Node) Node {
	tmp := *s
	s = &tmp

	s.parent = newParent
	return s
}

func (s *Scalar) Equal(n2 Node) bool {
	s2 := n2.(*Scalar)
	if _, ok := s.Value.(d2ast.String); ok {
		if _, ok = s2.Value.(d2ast.String); ok {
			return s.Value.ScalarString() == s2.Value.ScalarString()
		}
	}
	return s.Value.Type() == s2.Value.Type() && s.Value.ScalarString() == s2.Value.ScalarString()
}

type Map struct {
	parent    Node
	importAST d2ast.Node
	Fields    []*Field `json:"fields"`
	Edges     []*Edge  `json:"edges"`

	globs []*globContext
}

func (m *Map) initRoot() {
	m.parent = &Field{
		Name: d2ast.FlatUnquotedString("root"),
		References: []*FieldReference{{
			Context_: &RefContext{
				ScopeMap: m,
			},
		}},
	}
}

func (m *Map) ImportAST() d2ast.Node {
	return m.importAST
}

func (m *Map) SetImportAST(node d2ast.Node) {
	m.importAST = node
	for _, f := range m.Fields {
		f.SetImportAST(node)
	}
	for _, e := range m.Edges {
		e.SetImportAST(node)
	}
}

func (m *Map) Copy(newParent Node) Node {
	tmp := *m
	m = &tmp

	m.parent = newParent
	pfields := m.Fields
	m.Fields = make([]*Field, 0, len(pfields))
	for _, f := range pfields {
		m.Fields = append(m.Fields, f.Copy(m).(*Field))
	}
	m.Edges = append([]*Edge(nil), m.Edges...)
	for i := range m.Edges {
		m.Edges[i] = m.Edges[i].Copy(m).(*Edge)
	}
	if m.parent == nil {
		m.initRoot()
	}
	return m
}

// CopyBase copies the map m without layers/scenarios/steps.
func (m *Map) CopyBase(newParent Node) *Map {
	if m == nil {
		return (&Map{}).Copy(newParent).(*Map)
	}

	layers := m.DeleteField("layers")
	scenarios := m.DeleteField("scenarios")
	steps := m.DeleteField("steps")

	m2 := m.Copy(newParent).(*Map)
	if layers != nil {
		m.Fields = append(m.Fields, layers)
	}
	if scenarios != nil {
		m.Fields = append(m.Fields, scenarios)
	}
	if steps != nil {
		m.Fields = append(m.Fields, steps)
	}
	return m2
}

// Root reports whether the Map is the root of the D2 tree.
func (m *Map) Root() bool {
	// m.parent exists even on the root map as we store the root AST in
	// m.parent.References[0].Context.Map for reporting error messages about the whole IR.
	// Or if otherwise needed.
	f, ok := m.parent.(*Field)
	if !ok {
		return false
	}
	return f.Root()
}

func (f *Field) Root() bool {
	return f.parent == nil
}

type BoardKind string

const (
	BoardLayer    BoardKind = "layer"
	BoardScenario BoardKind = "scenario"
	BoardStep     BoardKind = "step"
)

// NodeBoardKind reports whether n represents the root of a board.
// n should be *Field or *Map
func NodeBoardKind(n Node) BoardKind {
	var f *Field
	switch n := n.(type) {
	case *Field:
		if n.parent == nil {
			return BoardLayer
		}
		f = ParentField(n)
	case *Map:
		if n == nil {
			return ""
		}
		var ok bool
		f, ok = n.parent.(*Field)
		if !ok {
			return ""
		}
		if f.Root() {
			return BoardLayer
		}
		f = ParentField(f)
	}
	if f == nil {
		return ""
	}
	switch f.Name.ScalarString() {
	case "layers":
		return BoardLayer
	case "scenarios":
		return BoardScenario
	case "steps":
		return BoardStep
	default:
		return ""
	}
}

type Importable interface {
	ImportAST() d2ast.Node
	SetImportAST(d2ast.Node)
}

var _ Importable = &Edge{}
var _ Importable = &Field{}
var _ Importable = &Map{}

type Field struct {
	// *Map.
	parent    Node
	importAST d2ast.Node
	suspended bool

	Name d2ast.String `json:"name"`

	// Primary_ to avoid clashing with Primary(). We need to keep it exported for
	// encoding/json to marshal it so cannot prefix _ instead.
	Primary_  *Scalar   `json:"primary,omitempty"`
	Composite Composite `json:"composite,omitempty"`

	References []*FieldReference `json:"references,omitempty"`
}

func (f *Field) ImportAST() d2ast.Node {
	return f.importAST
}

func (f *Field) SetImportAST(node d2ast.Node) {
	f.importAST = node
	if f.Map() != nil {
		f.Map().SetImportAST(node)
	}
}

func (f *Field) Copy(newParent Node) Node {
	tmp := *f
	f = &tmp

	f.parent = newParent
	f.References = append([]*FieldReference(nil), f.References...)
	if f.Primary_ != nil {
		f.Primary_ = f.Primary_.Copy(f).(*Scalar)
	}
	if f.Composite != nil {
		f.Composite = f.Composite.Copy(f).(Composite)
	}
	return f
}

func (f *Field) LastPrimaryRef() Reference {
	for i := len(f.References) - 1; i >= 0; i-- {
		if f.References[i].Primary() {
			return f.References[i]
		}
	}
	return nil
}

func (f *Field) LastPrimaryKey() *d2ast.Key {
	fr := f.LastPrimaryRef()
	if fr == nil {
		return nil
	}
	return fr.(*FieldReference).Context_.Key
}

func (f *Field) LastRef() Reference {
	return f.References[len(f.References)-1]
}

type EdgeID struct {
	SrcPath  []d2ast.String `json:"src_path"`
	SrcArrow bool           `json:"src_arrow"`

	DstPath  []d2ast.String `json:"dst_path"`
	DstArrow bool           `json:"dst_arrow"`

	// If nil, then any EdgeID with equal src/dst/arrows matches.
	Index *int `json:"index"`
	Glob  bool `json:"glob"`
}

func NewEdgeIDs(k *d2ast.Key) (eida []*EdgeID) {
	for _, ke := range k.Edges {
		eid := &EdgeID{
			SrcPath:  ke.Src.IDA(),
			SrcArrow: ke.SrcArrow == "<",
			DstPath:  ke.Dst.IDA(),
			DstArrow: ke.DstArrow == ">",
		}
		if k.EdgeIndex != nil {
			eid.Index = k.EdgeIndex.Int
			eid.Glob = k.EdgeIndex.Glob
		}
		eida = append(eida, eid)
	}
	return eida
}

func (eid *EdgeID) Copy() *EdgeID {
	tmp := *eid
	eid = &tmp

	eid.SrcPath = append([]d2ast.String(nil), eid.SrcPath...)
	eid.DstPath = append([]d2ast.String(nil), eid.DstPath...)
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
		if !strings.EqualFold(s.ScalarString(), eid2.SrcPath[i].ScalarString()) {
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
		if !strings.EqualFold(s.ScalarString(), eid2.DstPath[i].ScalarString()) {
			return false
		}
	}

	return true
}

// resolve resolves both underscores and commons in eid.
// It returns the new eid, containing map adjusted for underscores and common ida.
func (eid *EdgeID) resolve(m *Map) (_ *EdgeID, _ *Map, common []d2ast.String, _ error) {
	eid = eid.Copy()
	maxUnderscores := go2.Max(countUnderscores(eid.SrcPath), countUnderscores(eid.DstPath))
	for i := 0; i < maxUnderscores; i++ {
		if eid.SrcPath[0].ScalarString() == "_" && eid.SrcPath[0].IsUnquoted() {
			eid.SrcPath = eid.SrcPath[1:]
		} else {
			mf := ParentField(m)
			eid.SrcPath = append([]d2ast.String{mf.Name}, eid.SrcPath...)
		}
		if eid.DstPath[0].ScalarString() == "_" && eid.DstPath[0].IsUnquoted() {
			eid.DstPath = eid.DstPath[1:]
		} else {
			mf := ParentField(m)
			eid.DstPath = append([]d2ast.String{mf.Name}, eid.DstPath...)
		}
		m = ParentMap(m)
		if m == nil {
			return nil, nil, nil, errors.New("invalid underscore")
		}
	}

	for len(eid.SrcPath) > 1 && len(eid.DstPath) > 1 {
		if !strings.EqualFold(eid.SrcPath[0].ScalarString(), eid.DstPath[0].ScalarString()) || strings.Contains(eid.SrcPath[0].ScalarString(), "*") {
			return eid, m, common, nil
		}
		common = append(common, eid.SrcPath[0])
		eid.SrcPath = eid.SrcPath[1:]
		eid.DstPath = eid.DstPath[1:]
	}

	return eid, m, common, nil
}

type Edge struct {
	// *Map
	parent    Node
	importAST d2ast.Node
	suspended bool

	ID *EdgeID `json:"edge_id"`

	Primary_ *Scalar `json:"primary,omitempty"`
	Map_     *Map    `json:"map,omitempty"`

	References []*EdgeReference `json:"references,omitempty"`
}

func (e *Edge) ImportAST() d2ast.Node {
	return e.importAST
}

func (e *Edge) SetImportAST(node d2ast.Node) {
	e.importAST = node
	if e.Map() != nil {
		e.Map().SetImportAST(node)
	}
}

func (e *Edge) Copy(newParent Node) Node {
	tmp := *e
	e = &tmp

	e.parent = newParent
	e.References = append([]*EdgeReference(nil), e.References...)
	if e.Primary_ != nil {
		e.Primary_ = e.Primary_.Copy(e).(*Scalar)
	}
	if e.Map_ != nil {
		e.Map_ = e.Map_.Copy(e).(*Map)
	}
	return e
}

func (e *Edge) LastPrimaryRef() Reference {
	for i := len(e.References) - 1; i >= 0; i-- {
		fr := e.References[i]
		if fr.Context_.Key.EdgeKey == nil && !fr.DueToLazyGlob() {
			return fr
		}
	}
	return nil
}

func (e *Edge) LastPrimaryKey() *d2ast.Key {
	er := e.LastPrimaryRef()
	if er == nil {
		return nil
	}
	return er.(*EdgeReference).Context_.Key
}

func (e *Edge) LastRef() Reference {
	return e.References[len(e.References)-1]
}

type Array struct {
	parent Node
	Values []Value `json:"values"`
}

func (a *Array) Copy(newParent Node) Node {
	tmp := *a
	a = &tmp

	a.parent = newParent
	a.Values = append([]Value(nil), a.Values...)
	for i := range a.Values {
		a.Values[i] = a.Values[i].Copy(a).(Value)
	}
	return a
}

type FieldReference struct {
	String  d2ast.String   `json:"string"`
	KeyPath *d2ast.KeyPath `json:"key_path"`

	Context_       *RefContext `json:"context"`
	DueToGlob_     bool        `json:"due_to_glob"`
	DueToLazyGlob_ bool        `json:"due_to_lazy_glob"`
}

// Primary returns true if the Value in Context.Key.Value corresponds to the Field
// represented by String.
func (fr *FieldReference) Primary() bool {
	if fr.KeyPath == fr.Context_.Key.Key {
		return len(fr.Context_.Key.Edges) == 0 && fr.KeyPathIndex() == len(fr.KeyPath.Path)-1
	} else if fr.KeyPath == fr.Context_.Key.EdgeKey {
		return len(fr.Context_.Key.Edges) == 1 && fr.KeyPathIndex() == len(fr.KeyPath.Path)-1
	}
	return false
}

func (fr *FieldReference) KeyPathIndex() int {
	for i, sb := range fr.KeyPath.Path {
		if sb.Unbox() == fr.String {
			return i
		}
	}
	panic("d2ir.KeyReference.KeyPathIndex: String not in KeyPath?")
}

func (fr *FieldReference) EdgeDest() bool {
	return fr.KeyPath == fr.Context_.Edge.Dst
}

func (fr *FieldReference) InEdge() bool {
	return fr.Context_.Edge != nil
}

func (fr *FieldReference) AST() d2ast.Node {
	if fr.String == nil {
		// Root map.
		return fr.Context_.Scope
	}
	return fr.String
}

type EdgeReference struct {
	Context_       *RefContext `json:"context"`
	DueToGlob_     bool        `json:"due_to_glob"`
	DueToLazyGlob_ bool        `json:"due_to_lazy_glob"`
}

func (er *EdgeReference) AST() d2ast.Node {
	return er.Context_.Edge
}

// Primary returns true if the Value in Context.Key.Value corresponds to the *Edge
// represented by Context.Edge
func (er *EdgeReference) Primary() bool {
	return len(er.Context_.Key.Edges) == 1 && er.Context_.Key.EdgeKey == nil
}

type RefContext struct {
	Edge     *d2ast.Edge `json:"edge"`
	Key      *d2ast.Key  `json:"key"`
	Scope    *d2ast.Map  `json:"-"`
	ScopeMap *Map        `json:"-"`
	ScopeAST *d2ast.Map  `json:"-"`
}

func (rc *RefContext) Copy() *RefContext {
	tmp := *rc
	return &tmp
}

func (rc *RefContext) EdgeIndex() int {
	for i, e := range rc.Key.Edges {
		if e == rc.Edge {
			return i
		}
	}
	return -1
}

func (rc *RefContext) Equal(rc2 *RefContext) bool {
	// We intentionally ignore edges here because the same glob can produce multiple RefContexts that should be treated  the same with only the edge as the difference.
	// Same with ScopeMap.
	if !(rc.Key.Equals(rc2.Key) && rc.Scope == rc2.Scope && rc.ScopeAST == rc2.ScopeAST) {
		return false
	}

	// Check if suspension values match for suspension operations
	// We don't want these two to equal
	// 1. *: suspend
	// 2. *: unsuspend
	hasSuspension1 := (rc.Key.Primary.Suspension != nil || rc.Key.Value.Suspension != nil)
	hasSuspension2 := (rc2.Key.Primary.Suspension != nil || rc2.Key.Value.Suspension != nil)

	if hasSuspension1 || hasSuspension2 {
		var val1, val2 bool
		if rc.Key.Primary.Suspension != nil {
			val1 = rc.Key.Primary.Suspension.Value
		} else if rc.Key.Value.Suspension != nil {
			val1 = rc.Key.Value.Suspension.Value
		}

		if rc2.Key.Primary.Suspension != nil {
			val2 = rc2.Key.Primary.Suspension.Value
		} else if rc2.Key.Value.Suspension != nil {
			val2 = rc2.Key.Value.Suspension.Value
		}

		if hasSuspension1 && hasSuspension2 && val1 != val2 {
			return false
		}

		if hasSuspension1 != hasSuspension2 {
			return false
		}
	}

	return true
}

func (m *Map) FieldCountRecursive() int {
	if m == nil {
		return 0
	}
	acc := len(m.Fields)
	for _, f := range m.Fields {
		if f.Map() != nil {
			acc += f.Map().FieldCountRecursive()
		}
	}
	for _, e := range m.Edges {
		if e.Map_ != nil {
			acc += e.Map_.FieldCountRecursive()
		}
	}
	return acc
}

func (m *Map) IsContainer() bool {
	if m == nil {
		return false
	}
	// Check references as the fields and edges may not be compiled yet
	f := m.Parent().(*Field)
	for _, ref := range f.References {
		if ref.Primary() && ref.Context_.Key != nil && ref.Context_.Key.Value.Map != nil {
			for _, n := range ref.Context_.Key.Value.Map.Nodes {
				if len(n.MapKey.Edges) > 0 {
					return true
				}
				if n.MapKey.Key != nil {
					_, isReserved := d2ast.ReservedKeywords[n.MapKey.Key.Path[0].Unbox().ScalarString()]
					if !(isReserved && f.Name.IsUnquoted()) {
						return true
					}
				}
			}
		}
	}
	for _, f := range m.Fields {
		_, isReserved := d2ast.ReservedKeywords[f.Name.ScalarString()]
		if !(isReserved && f.Name.IsUnquoted()) {
			return true
		}
	}
	return false
}

func (m *Map) EdgeCountRecursive() int {
	if m == nil {
		return 0
	}
	acc := len(m.Edges)
	for _, f := range m.Fields {
		if f.Map() != nil {
			acc += f.Map().EdgeCountRecursive()
		}
	}
	for _, e := range m.Edges {
		if e.Map_ != nil {
			acc += e.Map_.EdgeCountRecursive()
		}
	}
	return acc
}

func (m *Map) GetClassMap(name string) *Map {
	root := RootMap(m)
	classes := root.Map().GetField(d2ast.FlatUnquotedString("classes"))
	if classes != nil && classes.Map() != nil {
		class := classes.Map().GetField(d2ast.FlatUnquotedString(name))
		if class != nil && class.Map() != nil {
			return class.Map()
		}
	}
	return nil
}

func (m *Map) GetField(ida ...d2ast.String) *Field {
	for len(ida) > 0 && ida[0].ScalarString() == "_" && ida[0].IsUnquoted() {
		m = ParentMap(m)
		if m == nil {
			return nil
		}
	}
	return m.getField(ida)
}

func (m *Map) getField(ida []d2ast.String) *Field {
	if len(ida) == 0 {
		return nil
	}

	s := ida[0]
	rest := ida[1:]

	if s.ScalarString() == "_" && s.IsUnquoted() {
		return nil
	}

	for _, f := range m.Fields {
		if f.Name == nil {
			continue
		}
		if !strings.EqualFold(f.Name.ScalarString(), s.ScalarString()) {
			continue
		}
		if _, isReserved := d2ast.ReservedKeywords[strings.ToLower(s.ScalarString())]; isReserved {
			if f.Name.IsUnquoted() != s.IsUnquoted() {
				continue
			}
		}
		if len(rest) == 0 {
			return f
		}
		if f.Map() != nil {
			return f.Map().getField(rest)
		}
	}
	return nil
}

// EnsureField is a bit of a misnomer. It's more of a Query/Ensure combination function at this point.
func (m *Map) EnsureField(kp *d2ast.KeyPath, refctx *RefContext, create bool, c *compiler) ([]*Field, error) {
	i := 0
	for kp.Path[i].Unbox().ScalarString() == "_" && kp.Path[i].Unbox().IsUnquoted() {
		m = ParentMap(m)
		if m == nil {
			return nil, d2parser.Errorf(kp.Path[i].Unbox(), "invalid underscore: no parent")
		}
		if i+1 == len(kp.Path) {
			return nil, d2parser.Errorf(kp.Path[i].Unbox(), "field key must contain more than underscores")
		}
		i++
	}

	var gctx *globContext
	if refctx != nil && refctx.Key.HasGlob() && c != nil {
		gctx = c.getGlobContext(refctx)
	}

	var fa []*Field
	err := m.ensureField(i, kp, refctx, create, gctx, c, &fa)
	if len(fa) > 0 && c != nil && len(c.globRefContextStack) == 0 {
		for _, gctx2 := range c.globContexts() {
			old := c.lazyGlobBeingApplied
			c.lazyGlobBeingApplied = true
			c.compileKey(gctx2.refctx)
			c.lazyGlobBeingApplied = old
		}
	}
	return fa, err
}

func (m *Map) ensureField(i int, kp *d2ast.KeyPath, refctx *RefContext, create bool, gctx *globContext, c *compiler, fa *[]*Field) error {
	filter := func(f *Field, passthrough bool) bool {
		if gctx != nil {
			var ks string
			if refctx.Key.HasMultiGlob() {
				ks = d2format.Format(d2ast.MakeKeyPathString(IDA(f)))
			} else {
				ks = d2format.Format(d2ast.MakeKeyPathString(BoardIDA(f)))
			}
			if !kp.HasGlob() {
				if !passthrough {
					gctx.appliedFields[ks] = struct{}{}
				}
				return true
			}
			// For globs with edges, we only ignore duplicate fields if the glob is not at the terminal of the keypath, the glob is on the common key or the glob is on the edge key. And only for globs with edge indexes.
			lastEl := kp.Path[len(kp.Path)-1]
			if len(refctx.Key.Edges) == 0 || lastEl.UnquotedString == nil || len(lastEl.UnquotedString.Pattern) == 0 || kp == refctx.Key.Key || kp == refctx.Key.EdgeKey {
				if _, ok := gctx.appliedFields[ks]; ok {
					return false
				}
			}
			if !passthrough {
				gctx.appliedFields[ks] = struct{}{}
			}
		}
		return true
	}
	faAppend := func(fa2 ...*Field) {
		for _, f := range fa2 {
			if filter(f, false) {
				*fa = append(*fa, f)
			}
		}
	}

	us, ok := kp.Path[i].Unbox().(*d2ast.UnquotedString)
	if ok && us.Pattern != nil {
		fa2, ok := m.multiGlob(us.Pattern)
		if ok {
			if i == len(kp.Path)-1 {
				faAppend(fa2...)
			} else {
				for _, f := range fa2 {
					if !filter(f, true) {
						continue
					}
					if f.Map() == nil {
						f.Composite = &Map{
							parent: f,
						}
					}
					err := f.Map().ensureField(i+1, kp, refctx, create, gctx, c, fa)
					if err != nil {
						return err
					}
				}
			}
			return nil
		}
		for _, f := range m.Fields {
			if f.Name == nil {
				continue
			}
			if matchPattern(f.Name.ScalarString(), us.Pattern) {
				if i == len(kp.Path)-1 {
					faAppend(f)
				} else {
					if !filter(f, true) {
						continue
					}
					if f.Map() == nil {
						f.Composite = &Map{
							parent: f,
						}
					}
					err := f.Map().ensureField(i+1, kp, refctx, create, gctx, c, fa)
					if err != nil {
						return err
					}
				}
			}
		}
		return nil
	}

	head := kp.Path[i].Unbox()
	headString := head.ScalarString()

	if _, ok := d2ast.ReservedKeywords[strings.ToLower(head.ScalarString())]; ok && head.IsUnquoted() {
		headString = strings.ToLower(head.ScalarString())
		if _, ok := d2ast.CompositeReservedKeywords[headString]; !ok && i < len(kp.Path)-1 {
			return d2parser.Errorf(kp.Path[i].Unbox(), fmt.Sprintf(`"%s" must be the last part of the key`, headString))
		}
	}

	if headString == "_" && head.IsUnquoted() {
		return d2parser.Errorf(kp.Path[i].Unbox(), `parent "_" can only be used in the beginning of paths, e.g. "_.x"`)
	}

	if headString == "classes" && head.IsUnquoted() && NodeBoardKind(m) == "" {
		return d2parser.Errorf(kp.Path[i].Unbox(), "%s is only allowed at a board root", headString)
	}

	if findBoardKeyword(head) != -1 && head.IsUnquoted() && NodeBoardKind(m) == "" {
		return d2parser.Errorf(kp.Path[i].Unbox(), "%s is only allowed at a board root", headString)
	}

	for _, f := range m.Fields {
		if !(f.Name != nil && strings.EqualFold(f.Name.ScalarString(), head.ScalarString())) {
			continue
		}
		if _, isReserved := d2ast.ReservedKeywords[strings.ToLower(f.Name.ScalarString())]; isReserved {
			if f.Name.IsUnquoted() != head.IsUnquoted() {
				continue
			}
		}

		// Don't add references for fake common KeyPath from trimCommon in CreateEdge.
		if refctx != nil {
			f.References = append(f.References, &FieldReference{
				String:         kp.Path[i].Unbox(),
				KeyPath:        kp,
				Context_:       refctx,
				DueToGlob_:     len(c.globRefContextStack) > 0,
				DueToLazyGlob_: c.lazyGlobBeingApplied,
			})
		}

		if i+1 == len(kp.Path) {
			faAppend(f)
			return nil
		}
		if !filter(f, true) {
			return nil
		}
		if _, ok := f.Composite.(*Array); ok {
			return d2parser.Errorf(kp.Path[i].Unbox(), "cannot index into array")
		}
		if f.Map() == nil {
			f.Composite = &Map{
				parent: f,
			}
		}
		return f.Map().ensureField(i+1, kp, refctx, create, gctx, c, fa)
	}

	if !create {
		return nil
	}
	shape := ParentShape(m)
	if _, ok := d2ast.ReservedKeywords[strings.ToLower(head.ScalarString())]; !(ok && head.IsUnquoted()) && len(c.globRefContextStack) > 0 {
		if shape == d2target.ShapeClass || shape == d2target.ShapeSQLTable {
			return nil
		}
	}
	f := &Field{
		parent: m,
		Name:   kp.Path[i].Unbox(),
	}
	defer func() {
		if i < kp.FirstGlob() {
			return
		}
		for _, grefctx := range c.globRefContextStack {
			var ks string
			if grefctx.Key.HasMultiGlob() {
				ks = d2format.Format(d2ast.MakeKeyPathString(IDA(f)))
			} else {
				ks = d2format.Format(d2ast.MakeKeyPathString(BoardIDA(f)))
			}
			gctx2 := c.getGlobContext(grefctx)
			gctx2.appliedFields[ks] = struct{}{}
		}
	}()
	// Don't add references for fake common KeyPath from trimCommon in CreateEdge.
	if refctx != nil {
		f.References = append(f.References, &FieldReference{
			String:         kp.Path[i].Unbox(),
			KeyPath:        kp,
			Context_:       refctx,
			DueToGlob_:     len(c.globRefContextStack) > 0,
			DueToLazyGlob_: c.lazyGlobBeingApplied,
		})
	}
	if !filter(f, true) {
		return nil
	}
	m.Fields = append(m.Fields, f)
	if i+1 == len(kp.Path) {
		faAppend(f)
		return nil
	}
	if f.Composite == nil {
		f.Composite = &Map{
			parent: f,
		}
	}
	return f.Map().ensureField(i+1, kp, refctx, create, gctx, c, fa)
}

func (m *Map) DeleteEdge(eid *EdgeID) *Edge {
	if eid == nil {
		return nil
	}

	resolvedEID, resolvedM, common, err := eid.resolve(m)
	if err != nil {
		return nil
	}

	if len(common) > 0 {
		f := resolvedM.GetField(common...)
		if f == nil {
			return nil
		}
		if f.Map() == nil {
			return nil
		}
		return f.Map().DeleteEdge(resolvedEID)
	}

	for i, e := range resolvedM.Edges {
		if e.ID.Match(resolvedEID) {
			resolvedM.Edges = append(resolvedM.Edges[:i], resolvedM.Edges[i+1:]...)
			return e
		}
	}
	return nil
}

func (m *Map) DeleteField(ida ...string) *Field {
	if len(ida) == 0 {
		return nil
	}

	s := ida[0]
	rest := ida[1:]

	for i, f := range m.Fields {
		if !strings.EqualFold(f.Name.ScalarString(), s) {
			continue
		}
		if len(rest) == 0 {
			for _, fr := range f.References {
				currM := m
				for currM != nil {
					for _, e := range currM.Edges {
						for _, er := range e.References {
							if er.Context_ == fr.Context_ {
								currM.DeleteEdge(e.ID)
								break
							}
						}
					}
					if NodeBoardKind(currM) != "" {
						break
					}
					currM = ParentMap(currM)
				}
			}
			m.Fields = append(m.Fields[:i], m.Fields[i+1:]...)

			// If a field was deleted from a keyword-holder keyword and that holder is empty,
			// then that holder becomes meaningless and should be deleted too
			parent := ParentField(f)
			for keywordHolder := range d2ast.ReservedKeywordHolders {
				if parent != nil && parent.Name.ScalarString() == keywordHolder && parent.Name.IsUnquoted() && len(parent.Map().Fields) == 0 {
					keywordHolderParentMap := ParentMap(parent)
					for i, f := range keywordHolderParentMap.Fields {
						if f.Name.ScalarString() == keywordHolder && f.Name.IsUnquoted() {
							keywordHolderParentMap.Fields = append(keywordHolderParentMap.Fields[:i], keywordHolderParentMap.Fields[i+1:]...)
							break
						}
					}
				}
			}
			return f
		}
		if f.Map() != nil {
			return f.Map().DeleteField(rest...)
		}
	}
	return nil
}

func (m *Map) GetEdges(eid *EdgeID, refctx *RefContext, c *compiler) []*Edge {
	if refctx != nil {
		var gctx *globContext
		if refctx.Key.HasGlob() && c != nil {
			gctx = c.ensureGlobContext(refctx)
		}
		var ea []*Edge
		m.getEdges(eid, refctx, gctx, &ea)
		return ea
	}

	eid, m, common, err := eid.resolve(m)
	if err != nil {
		return nil
	}
	if len(common) > 0 {
		f := m.GetField(common...)
		if f == nil {
			return nil
		}
		if f.Map() != nil {
			return f.Map().GetEdges(eid, nil, nil)
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

func (m *Map) getEdges(eid *EdgeID, refctx *RefContext, gctx *globContext, ea *[]*Edge) error {
	eid, m, common, err := eid.resolve(m)
	if err != nil {
		return err
	}

	if len(common) > 0 {
		commonKP := d2ast.MakeKeyPathString(common)
		lastMatch := 0
		for i, el := range commonKP.Path {
			for j := lastMatch; j < len(refctx.Edge.Src.Path); j++ {
				realEl := refctx.Edge.Src.Path[j]
				if el.ScalarString() == realEl.ScalarString() {
					commonKP.Path[i] = realEl
					lastMatch += j + 1
				}
			}
		}
		fa, err := m.EnsureField(commonKP, nil, false, nil)
		if err != nil {
			return nil
		}
		for _, f := range fa {
			if _, ok := f.Composite.(*Array); ok {
				return d2parser.Errorf(refctx.Edge.Src, "cannot index into array")
			}
			if f.Map() == nil {
				f.Composite = &Map{
					parent: f,
				}
			}
			err = f.Map().getEdges(eid, refctx, gctx, ea)
			if err != nil {
				return err
			}
		}
		return nil
	}

	srcFA, err := refctx.ScopeMap.EnsureField(refctx.Edge.Src, nil, false, nil)
	if err != nil {
		return err
	}
	dstFA, err := refctx.ScopeMap.EnsureField(refctx.Edge.Dst, nil, false, nil)
	if err != nil {
		return err
	}

	for _, src := range srcFA {
		for _, dst := range dstFA {
			eid2 := eid.Copy()
			eid2.SrcPath = RelIDA(m, src)
			eid2.DstPath = RelIDA(m, dst)

			ea2 := m.GetEdges(eid2, nil, nil)
			for _, e := range ea2 {
				if gctx != nil {
					var ks string
					if refctx.Key.HasMultiGlob() {
						ks = d2format.Format(d2ast.MakeKeyPathString(IDA(e)))
					} else {
						ks = d2format.Format(d2ast.MakeKeyPathString(BoardIDA(e)))
					}
					if _, ok := gctx.appliedEdges[ks]; ok {
						continue
					}
					gctx.appliedEdges[ks] = struct{}{}
				}
				*ea = append(*ea, e)
			}
		}
	}
	return nil
}

func (m *Map) CreateEdge(eid *EdgeID, refctx *RefContext, c *compiler) ([]*Edge, error) {
	var ea []*Edge
	var gctx *globContext
	if refctx != nil && refctx.Key.HasGlob() && c != nil {
		gctx = c.ensureGlobContext(refctx)
	}
	err := m.createEdge(eid, refctx, gctx, c, &ea)
	if len(ea) > 0 && c != nil && len(c.globRefContextStack) == 0 {
		for _, gctx2 := range c.globContexts() {
			old := c.lazyGlobBeingApplied
			c.lazyGlobBeingApplied = true
			c.compileKey(gctx2.refctx)
			c.lazyGlobBeingApplied = old
		}
	}
	return ea, err
}

func (m *Map) createEdge(eid *EdgeID, refctx *RefContext, gctx *globContext, c *compiler, ea *[]*Edge) error {
	if ParentEdge(m) != nil {
		return d2parser.Errorf(refctx.Edge, "cannot create edge inside edge")
	}

	eid, m, common, err := eid.resolve(m)
	if err != nil {
		return d2parser.Errorf(refctx.Edge, err.Error())
	}
	if len(common) > 0 {
		commonKP := d2ast.MakeKeyPathString(common)
		lastMatch := 0
		for i, el := range commonKP.Path {
			for j := lastMatch; j < len(refctx.Edge.Src.Path); j++ {
				realEl := refctx.Edge.Src.Path[j]
				if el.ScalarString() == realEl.ScalarString() {
					commonKP.Path[i] = realEl
					lastMatch += j + 1
				}
			}
		}
		fa, err := m.EnsureField(commonKP, nil, true, c)
		if err != nil {
			return err
		}
		for _, f := range fa {
			if _, ok := f.Composite.(*Array); ok {
				return d2parser.Errorf(refctx.Edge.Src, "cannot index into array")
			}
			if f.Map() == nil {
				f.Composite = &Map{
					parent: f,
				}
			}
			err = f.Map().createEdge(eid, refctx, gctx, c, ea)
			if err != nil {
				return err
			}
		}
		return nil
	}

	ij := findProhibitedEdgeKeyword(eid.SrcPath...)
	if ij != -1 {
		return d2parser.Errorf(refctx.Edge.Src.Path[ij].Unbox(), "reserved keywords are prohibited in edges")
	}
	ij = findBoardKeyword(eid.SrcPath...)
	if ij == len(eid.SrcPath)-1 {
		return d2parser.Errorf(refctx.Edge.Src.Path[ij].Unbox(), "edge with board keyword alone doesn't make sense")
	}

	ij = findProhibitedEdgeKeyword(eid.DstPath...)
	if ij != -1 {
		return d2parser.Errorf(refctx.Edge.Dst.Path[ij].Unbox(), "reserved keywords are prohibited in edges")
	}
	ij = findBoardKeyword(eid.DstPath...)
	if ij == len(eid.DstPath)-1 {
		return d2parser.Errorf(refctx.Edge.Dst.Path[ij].Unbox(), "edge with board keyword alone doesn't make sense")
	}

	srcFA, err := refctx.ScopeMap.EnsureField(refctx.Edge.Src, refctx, true, c)
	if err != nil {
		return err
	}
	dstFA, err := refctx.ScopeMap.EnsureField(refctx.Edge.Dst, refctx, true, c)
	if err != nil {
		return err
	}

	for _, src := range srcFA {
		for _, dst := range dstFA {
			if src == dst && (refctx.Edge.Src.HasGlob() || refctx.Edge.Dst.HasGlob()) {
				// Globs do not make self edges.
				continue
			}

			if refctx.Edge.Src.HasMultiGlob() {
				// If src has a double glob we only select leafs, those without children.
				if src.Map().IsContainer() {
					continue
				}
				if NodeBoardKind(src) != "" || ParentBoard(src) != ParentBoard(dst) {
					continue
				}
			}
			if refctx.Edge.Dst.HasMultiGlob() {
				// If dst has a double glob we only select leafs, those without children.
				if dst.Map().IsContainer() {
					continue
				}
				if NodeBoardKind(dst) != "" || ParentBoard(src) != ParentBoard(dst) {
					continue
				}
			}

			eid2 := eid.Copy()
			eid2.SrcPath = RelIDA(m, src)
			eid2.DstPath = RelIDA(m, dst)

			es, err := m.createEdge2(eid2, refctx, gctx, c, src, dst)
			if err != nil {
				return err
			}
			for _, e := range es {
				*ea = append(*ea, e)
			}
		}
	}
	return nil
}

func (m *Map) createEdge2(eid *EdgeID, refctx *RefContext, gctx *globContext, c *compiler, src, dst *Field) ([]*Edge, error) {
	if NodeBoardKind(src) != "" {
		return nil, d2parser.Errorf(refctx.Edge.Src, "cannot create edges between boards")
	}
	if NodeBoardKind(dst) != "" {
		return nil, d2parser.Errorf(refctx.Edge.Dst, "cannot create edges between boards")
	}
	if ParentBoard(src) != ParentBoard(dst) {
		return nil, d2parser.Errorf(refctx.Edge, "cannot create edges between boards")
	}

	eid, m, common, err := eid.resolve(m)
	if err != nil {
		return nil, d2parser.Errorf(refctx.Edge, err.Error())
	}
	if len(common) > 0 {
		commonKP := d2ast.MakeKeyPathString(common)
		lastMatch := 0
		for i, el := range commonKP.Path {
			for j := lastMatch; j < len(refctx.Edge.Src.Path); j++ {
				realEl := refctx.Edge.Src.Path[j]
				if el.ScalarString() == realEl.ScalarString() {
					commonKP.Path[i] = realEl
					lastMatch += j + 1
				}
			}
		}
		fa, err := m.EnsureField(commonKP, nil, true, c)
		if err != nil {
			return nil, err
		}
		var edges []*Edge
		for _, f := range fa {
			if _, ok := f.Composite.(*Array); ok {
				return nil, d2parser.Errorf(refctx.Edge.Src, "cannot index into array")
			}
			if f.Map() == nil {
				f.Composite = &Map{
					parent: f,
				}
			}
			edges2, err := f.Map().createEdge2(eid, refctx, gctx, c, src, dst)
			if err != nil {
				return nil, err
			}
			edges = append(edges, edges2...)
		}
		return edges, nil
	}

	eid.Index = nil
	eid.Glob = true
	ea := m.GetEdges(eid, nil, nil)
	index := len(ea)
	eid.Index = &index
	eid.Glob = false
	e := &Edge{
		parent: m,
		ID:     eid,
		References: []*EdgeReference{{
			Context_:       refctx,
			DueToGlob_:     len(c.globRefContextStack) > 0,
			DueToLazyGlob_: c.lazyGlobBeingApplied,
		}},
	}

	if gctx != nil {
		var ks string
		// We only ever want to create one of the edge per glob so we filter without the edge index.
		e2 := e.Copy(e.Parent()).(*Edge)
		e2.ID = e2.ID.Copy()
		e2.ID.Index = nil
		if refctx.Key.HasMultiGlob() {
			ks = d2format.Format(d2ast.MakeKeyPathString(IDA(e2)))
		} else {
			ks = d2format.Format(d2ast.MakeKeyPathString(BoardIDA(e2)))
		}
		if _, ok := gctx.appliedEdges[ks]; ok {
			return nil, nil
		}
		gctx.appliedEdges[ks] = struct{}{}
	}

	m.Edges = append(m.Edges, e)

	return []*Edge{e}, nil
}

func (s *Scalar) AST() d2ast.Node {
	return s.Value
}

func (f *Field) AST() d2ast.Node {
	k := &d2ast.Key{
		Key: &d2ast.KeyPath{
			Path: []*d2ast.StringBox{
				d2ast.MakeValueBox(f.Name).StringBox(),
			},
		},
	}

	if f.Primary_ != nil {
		k.Primary = d2ast.MakeValueBox(f.Primary_.AST().(d2ast.Value)).ScalarBox()
	}
	if f.Composite != nil {
		value := f.Composite.AST().(d2ast.Value)
		if m, ok := value.(*d2ast.Map); ok {
			path := m.Range.Path
			// Treat it as multi-line, but not file-map (line 0)
			m.Range = d2ast.MakeRange(",1:0:0-2:0:0")
			m.Range.Path = path
		}
		k.Value = d2ast.MakeValueBox(value)
	}

	return k
}

func (e *Edge) AST() d2ast.Node {
	astEdge := &d2ast.Edge{}

	astEdge.Src = d2ast.MakeKeyPathString(e.ID.SrcPath)
	if e.ID.SrcArrow {
		astEdge.SrcArrow = "<"
	}
	astEdge.Dst = d2ast.MakeKeyPathString(e.ID.DstPath)
	if e.ID.DstArrow {
		astEdge.DstArrow = ">"
	}

	k := &d2ast.Key{
		Edges: []*d2ast.Edge{astEdge},
	}

	if e.Primary_ != nil {
		k.Primary = d2ast.MakeValueBox(e.Primary_.AST().(d2ast.Value)).ScalarBox()
	}
	if e.Map_ != nil {
		k.Value = d2ast.MakeValueBox(e.Map_.AST().(*d2ast.Map))
	}

	return k
}

func (e *Edge) IDString() d2ast.String {
	ast := e.AST().(*d2ast.Key)
	if e.ID.Index != nil {
		ast.EdgeIndex = &d2ast.EdgeIndex{
			Int: e.ID.Index,
		}
	}
	ast.Primary = d2ast.ScalarBox{}
	ast.Value = d2ast.ValueBox{}
	formatted := d2format.Format(ast)
	return d2ast.FlatUnquotedString(formatted)
}

func (a *Array) AST() d2ast.Node {
	if a == nil {
		return nil
	}
	astArray := &d2ast.Array{}
	for _, av := range a.Values {
		astArray.Nodes = append(astArray.Nodes, d2ast.MakeArrayNodeBox(av.AST().(d2ast.ArrayNode)))
	}
	return astArray
}

func (m *Map) AST() d2ast.Node {
	if m == nil {
		return nil
	}
	astMap := &d2ast.Map{
		Range: d2ast.MakeRange(",0:0:0-1:0:0"),
	}
	if m.parent != nil && NodeBoardKind(m) != "" {
		f, ok := m.parent.(*Field)
		if ok {
			astMap.Range.Path = f.Name.GetRange().Path
		}
	}
	for _, f := range m.Fields {
		astMap.Nodes = append(astMap.Nodes, d2ast.MakeMapNodeBox(f.AST().(d2ast.MapNode)))
	}
	for _, e := range m.Edges {
		astMap.Nodes = append(astMap.Nodes, d2ast.MakeMapNodeBox(e.AST().(d2ast.MapNode)))
	}
	return astMap
}

func (m *Map) appendFieldReferences(i int, kp *d2ast.KeyPath, refctx *RefContext, c *compiler) {
	sb := kp.Path[i]
	f := m.GetField(sb.Unbox())
	if f == nil {
		return
	}

	f.References = append(f.References, &FieldReference{
		String:         sb.Unbox(),
		KeyPath:        kp,
		Context_:       refctx,
		DueToGlob_:     len(c.globRefContextStack) > 0,
		DueToLazyGlob_: c.lazyGlobBeingApplied,
	})
	if i+1 == len(kp.Path) {
		return
	}
	if f.Map() != nil {
		f.Map().appendFieldReferences(i+1, kp, refctx, c)
	}
}

func RootMap(m *Map) *Map {
	if m.Root() {
		return m
	}
	return RootMap(ParentMap(m))
}

func ParentMap(n Node) *Map {
	for {
		n = n.Parent()
		if n == nil {
			return nil
		}
		if m, ok := n.(*Map); ok {
			return m
		}
	}
}

func ParentField(n Node) *Field {
	for {
		n = n.Parent()
		if n == nil {
			return nil
		}
		if f, ok := n.(*Field); ok {
			return f
		}
	}
}

func IsVar(n Node) bool {
	for {
		if n == nil {
			return false
		}
		if NodeBoardKind(n) != "" {
			return false
		}
		if f, ok := n.(*Field); ok && f.Name.ScalarString() == "vars" && f.Name.IsUnquoted() {
			return true
		}
		if n == (*Map)(nil) {
			return false
		}
		n = n.Parent()
	}
}

func ParentBoard(n Node) Node {
	for {
		n = n.Parent()
		if n == nil {
			return nil
		}
		if NodeBoardKind(n) != "" {
			return n
		}
	}
}

func ParentEdge(n Node) *Edge {
	for {
		n = n.Parent()
		if n == nil {
			return nil
		}
		if e, ok := n.(*Edge); ok {
			return e
		}
	}
}

func ParentShape(n Node) string {
	for {
		f, ok := n.(*Field)
		if ok {
			if f.Map() != nil {
				shapef := f.Map().GetField(d2ast.FlatUnquotedString("shape"))
				if shapef != nil && shapef.Primary() != nil {
					return shapef.Primary().Value.ScalarString()
				}
			}
		}
		n = n.Parent()
		if n == nil {
			return ""
		}
	}
}

func countUnderscores(p []d2ast.String) int {
	for i, el := range p {
		if el.ScalarString() != "_" || !el.IsUnquoted() {
			return i
		}
	}
	return 0
}

func findBoardKeyword(ida ...d2ast.String) int {
	for i := range ida {
		if _, ok := d2ast.BoardKeywords[strings.ToLower(ida[i].ScalarString())]; ok && ida[i].IsUnquoted() {
			return i
		}
	}
	return -1
}

func findProhibitedEdgeKeyword(ida ...d2ast.String) int {
	for i := range ida {
		if _, ok := d2ast.SimpleReservedKeywords[ida[i].ScalarString()]; ok && ida[i].IsUnquoted() {
			return i
		}
		if _, ok := d2ast.ReservedKeywordHolders[ida[i].ScalarString()]; ok && ida[i].IsUnquoted() {
			return i
		}
	}
	return -1
}

func parentRef(n Node) Reference {
	f := ParentField(n)
	if f != nil {
		return f.LastRef()
	}
	e := ParentEdge(n)
	if e != nil {
		return e.LastRef()
	}
	return nil
}

func parentPrimaryRef(n Node) Reference {
	f := ParentField(n)
	if f != nil {
		return f.LastPrimaryRef()
	}
	e := ParentEdge(n)
	if e != nil {
		return e.LastPrimaryRef()
	}
	return nil
}

func parentPrimaryKey(n Node) *d2ast.Key {
	f := ParentField(n)
	if f != nil {
		return f.LastPrimaryKey()
	}
	e := ParentEdge(n)
	if e != nil {
		return e.LastPrimaryKey()
	}
	return nil
}

// BoardIDA returns the absolute path to n from the nearest board root.
func BoardIDA(n Node) (ida []d2ast.String) {
	for {
		switch n := n.(type) {
		case *Field:
			if n.Root() || NodeBoardKind(n) != "" {
				reverseIDA(ida)
				return ida
			}
			ida = append(ida, n.Name)
		case *Edge:
			ida = append(ida, n.IDString())
		}
		n = n.Parent()
		if n == nil {
			reverseIDA(ida)
			return ida
		}
	}
}

// IDA returns the absolute path to n.
func IDA(n Node) (ida []d2ast.String) {
	for {
		switch n := n.(type) {
		case *Field:
			ida = append(ida, n.Name)
			if n.Root() {
				reverseIDA(ida)
				return ida
			}
		case *Edge:
			ida = append(ida, n.IDString())
		}
		n = n.Parent()
		if n == nil {
			reverseIDA(ida)
			return ida
		}
	}
}

// RelIDA returns the path to n relative to p.
func RelIDA(p, n Node) (ida []d2ast.String) {
	for {
		switch n := n.(type) {
		case *Field:
			ida = append(ida, n.Name)
			if n.Root() {
				reverseIDA(ida)
				return ida
			}
		case *Edge:
			ida = append(ida, d2ast.FlatUnquotedString(n.String()))
		}
		n = n.Parent()
		f, fok := n.(*Field)
		e, eok := n.(*Edge)
		if n == nil || (fok && (f.Root() || f == p || f.Composite == p)) || (eok && (e == p || e.Map_ == p)) {
			reverseIDA(ida)
			return ida
		}
	}
}

func reverseIDA[T any](slice []T) {
	for i := 0; i < len(slice)/2; i++ {
		tmp := slice[i]
		slice[i] = slice[len(slice)-i-1]
		slice[len(slice)-i-1] = tmp
	}
}

func (f *Field) Equal(n2 Node) bool {
	f2 := n2.(*Field)

	if f.Name != f2.Name {
		return false
	}
	if !f.Primary_.Equal(f2.Primary_) {
		return false
	}
	if !f.Composite.Equal(f2.Composite) {
		return false
	}
	return true
}

func (e *Edge) Equal(n2 Node) bool {
	e2 := n2.(*Edge)

	if !e.ID.Match(e2.ID) {
		return false
	}
	if !e.Primary_.Equal(e2.Primary_) {
		return false
	}
	if !e.Map_.Equal(e2.Map_) {
		return false
	}
	return true
}

func (a *Array) Equal(n2 Node) bool {
	a2 := n2.(*Array)

	if len(a.Values) != len(a2.Values) {
		return false
	}

	for i := range a.Values {
		if !a.Values[i].Equal(a2.Values[i]) {
			return false
		}
	}

	return true
}

func (m *Map) Equal(n2 Node) bool {
	m2 := n2.(*Map)

	if len(m.Fields) != len(m2.Fields) {
		return false
	}
	if len(m.Edges) != len(m2.Edges) {
		return false
	}

	for i := range m.Fields {
		if !m.Fields[i].Equal(m2.Fields[i]) {
			return false
		}
	}
	for i := range m.Edges {
		if !m.Edges[i].Equal(m2.Edges[i]) {
			return false
		}
	}

	return true
}

func (m *Map) InClass(key *d2ast.Key) bool {
	classes := m.Map().GetField(d2ast.FlatUnquotedString("classes"))
	if classes == nil || classes.Map() == nil {
		return false
	}

	for _, class := range classes.Map().Fields {
		if class.Map() == nil {
			continue
		}
		classF := class.Map().GetField(key.Key.IDA()...)
		if classF == nil {
			continue
		}

		for _, ref := range classF.References {
			if ref.Context_.Key == key {
				return true
			}
		}
	}
	return false
}

func (m *Map) IsClass() bool {
	parentBoard := ParentBoard(m)
	if parentBoard.Map() == nil {
		return false
	}
	classes := parentBoard.Map().GetField(d2ast.FlatUnquotedString("classes"))
	if classes == nil || classes.Map() == nil {
		return false
	}

	for _, class := range classes.Map().Fields {
		if class.Map() == m {
			return true
		}
	}
	return false
}

func (m *Map) FindBoardRoot(path []string) *Map {
	if m == nil {
		return nil
	}
	if len(path) == 0 {
		return m
	}

	layersf := m.GetField(d2ast.FlatUnquotedString("layers"))
	scenariosf := m.GetField(d2ast.FlatUnquotedString("scenarios"))
	stepsf := m.GetField(d2ast.FlatUnquotedString("steps"))

	if layersf != nil && layersf.Map() != nil {
		for _, f := range layersf.Map().Fields {
			if f.Name.ScalarString() == path[0] {
				if len(path) == 1 {
					return f.Map()
				}
				return f.Map().FindBoardRoot(path[1:])
			}
		}
	}

	if scenariosf != nil && scenariosf.Map() != nil {
		for _, f := range scenariosf.Map().Fields {
			if f.Name.ScalarString() == path[0] {
				if len(path) == 1 {
					return f.Map()
				}
				return f.Map().FindBoardRoot(path[1:])
			}
		}
	}

	if stepsf != nil && stepsf.Map() != nil {
		for _, f := range stepsf.Map().Fields {
			if f.Name.ScalarString() == path[0] {
				if len(path) == 1 {
					return f.Map()
				}
				return f.Map().FindBoardRoot(path[1:])
			}
		}
	}

	return nil
}
