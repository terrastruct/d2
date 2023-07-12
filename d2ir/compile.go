package d2ir

import (
	"io/fs"
	"strings"

	"oss.terrastruct.com/d2/d2ast"
	"oss.terrastruct.com/d2/d2format"
	"oss.terrastruct.com/d2/d2parser"
	"oss.terrastruct.com/util-go/go2"
)

type compiler struct {
	err *d2parser.ParseError

	fs fs.FS
	// importStack is used to detect cyclic imports.
	importStack []string
	// importCache enables reuse of files imported multiple times.
	importCache map[string]*Map
	utf16       bool
}

type CompileOptions struct {
	UTF16 bool
	// Pass nil to disable imports.
	FS fs.FS
}

func (c *compiler) errorf(n d2ast.Node, f string, v ...interface{}) {
	c.err.Errors = append(c.err.Errors, d2parser.Errorf(n, f, v...).(d2ast.Error))
}

func Compile(ast *d2ast.Map, opts *CompileOptions) (*Map, error) {
	if opts == nil {
		opts = &CompileOptions{}
	}
	c := &compiler{
		err: &d2parser.ParseError{},
		fs:  opts.FS,

		importCache: make(map[string]*Map),
		utf16:       opts.UTF16,
	}
	m := &Map{}
	m.initRoot()
	m.parent.(*Field).References[0].Context.Scope = ast
	m.parent.(*Field).References[0].Context.ScopeAST = ast

	c.pushImportStack(&d2ast.Import{
		Path: []*d2ast.StringBox{d2ast.RawStringBox(ast.GetRange().Path, true)},
	})
	defer c.popImportStack()

	c.compileMap(m, ast, ast)
	c.compileSubstitutions(m, nil)
	c.overlayClasses(m)
	if !c.err.Empty() {
		return nil, c.err
	}
	return m, nil
}

func (c *compiler) overlayClasses(m *Map) {
	classes := m.GetField("classes")
	if classes == nil || classes.Map() == nil {
		return
	}

	layersField := m.GetField("layers")
	if layersField == nil {
		return
	}
	layers := layersField.Map()
	if layers == nil {
		return
	}

	for _, lf := range layers.Fields {
		if lf.Map() == nil || lf.Primary() != nil {
			c.errorf(lf.References[0].Context.Key, "invalid layer")
			continue
		}
		l := lf.Map()
		lClasses := l.GetField("classes")

		if lClasses == nil {
			lClasses = classes.Copy(l).(*Field)
			l.Fields = append(l.Fields, lClasses)
		} else {
			base := classes.Copy(l).(*Field)
			OverlayMap(base.Map(), lClasses.Map())
			l.DeleteField("classes")
			l.Fields = append(l.Fields, base)
		}

		c.overlayClasses(l)
	}
}

func (c *compiler) compileSubstitutions(m *Map, varsStack []*Map) {
	for _, f := range m.Fields {
		if f.Name == "vars" && f.Map() != nil {
			varsStack = append([]*Map{f.Map()}, varsStack...)
		}
		if f.Primary() != nil {
			c.resolveSubstitutions(varsStack, f.LastRef().AST(), f.Primary())
		}
		if f.Map() != nil {
			c.compileSubstitutions(f.Map(), varsStack)
		}
	}
	for _, e := range m.Edges {
		if e.Primary() != nil {
			c.resolveSubstitutions(varsStack, e.LastRef().AST(), e.Primary())
		}
		if e.Map() != nil {
			c.compileSubstitutions(e.Map(), varsStack)
		}
	}
}

func (c *compiler) resolveSubstitutions(varsStack []*Map, node d2ast.Node, scalar *Scalar) {
	var subbed bool
	var resolvedField *Field

	switch s := scalar.Value.(type) {
	case *d2ast.UnquotedString:
		for i, box := range s.Value {
			if box.Substitution != nil {
				for _, vars := range varsStack {
					resolvedField = c.resolveSubstitution(vars, box.Substitution)
					if resolvedField != nil {
						break
					}
				}
				if resolvedField != nil {
					if resolvedField.Composite != nil {
						c.errorf(node, `cannot reference map variable "%s"`, strings.Join(box.Substitution.IDA(), "."))
						return
					}
					// If lone and unquoted, replace with value of sub
					if len(s.Value) == 1 {
						scalar.Value = resolvedField.Primary().Value
					} else {
						s.Value[i].String = go2.Pointer(resolvedField.Primary().String())
						subbed = true
					}
				} else {
					c.errorf(node, `could not resolve variable "%s"`, strings.Join(box.Substitution.IDA(), "."))
					return
				}
			}
		}
		if subbed {
			s.Coalesce()
			scalar.Value = d2ast.RawString(s.ScalarString(), false)
		}
	case *d2ast.DoubleQuotedString:
		for i, box := range s.Value {
			if box.Substitution != nil {
				for _, vars := range varsStack {
					resolvedField = c.resolveSubstitution(vars, box.Substitution)
					if resolvedField != nil {
						break
					}
				}
				if resolvedField != nil {
					if resolvedField.Composite != nil {
						c.errorf(node, `cannot reference map variable "%s"`, strings.Join(box.Substitution.IDA(), "."))
						return
					}
					s.Value[i].String = go2.Pointer(resolvedField.Primary().String())
					subbed = true
				} else {
					c.errorf(node, `could not resolve variable "%s"`, strings.Join(box.Substitution.IDA(), "."))
					return
				}
			}
		}
		if subbed {
			s.Coalesce()
		}
	}
}

func (c *compiler) resolveSubstitution(vars *Map, substitution *d2ast.Substitution) *Field {
	var resolved *Field
	for _, p := range substitution.Path {
		if vars == nil {
			resolved = nil
			break
		}
		r := vars.GetField(p.Unbox().ScalarString())
		if r == nil {
			resolved = nil
			break
		}
		vars = r.Map()
		resolved = r
	}
	return resolved
}

func (c *compiler) overlayVars(base, overlay *Map) {
	vars := overlay.GetField("vars")
	if vars == nil {
		return
	}

	lVars := base.GetField("vars")

	if lVars == nil {
		lVars = vars.Copy(base).(*Field)
		base.Fields = append(base.Fields, lVars)
	} else {
		overlayed := vars.Copy(base).(*Field)
		OverlayMap(overlayed.Map(), lVars.Map())
		base.DeleteField("vars")
		base.Fields = append(base.Fields, overlayed)
	}
}

func (c *compiler) overlay(base *Map, f *Field) {
	if f.Map() == nil || f.Primary() != nil {
		c.errorf(f.References[0].Context.Key, "invalid %s", NodeBoardKind(f))
		return
	}
	base = base.CopyBase(f)
	OverlayMap(base, f.Map())
	f.Composite = base
}

func (c *compiler) compileMap(dst *Map, ast, scopeAST *d2ast.Map) {
	for _, n := range ast.Nodes {
		switch {
		case n.MapKey != nil:
			c.compileKey(&RefContext{
				Key:      n.MapKey,
				Scope:    ast,
				ScopeMap: dst,
				ScopeAST: scopeAST,
			})
		case n.Import != nil:
			impn, ok := c._import(n.Import)
			if !ok {
				continue
			}
			if impn.Map() == nil {
				c.errorf(n.Import, "cannot spread import non map into map")
				continue
			}
			OverlayMap(dst, impn.Map())

			if impnf, ok := impn.(*Field); ok {
				if impnf.Primary_ != nil {
					dstf := ParentField(dst)
					if dstf != nil {
						dstf.Primary_ = impnf.Primary_
					}
				}
			}
		}
	}
}

func (c *compiler) compileKey(refctx *RefContext) {
	if len(refctx.Key.Edges) == 0 {
		c.compileField(refctx.ScopeMap, refctx.Key.Key, refctx)
	} else {
		c.compileEdges(refctx)
	}
}

func (c *compiler) compileField(dst *Map, kp *d2ast.KeyPath, refctx *RefContext) {
	if refctx.Key != nil && len(refctx.Key.Edges) == 0 && refctx.Key.Value.Null != nil {
		dst.DeleteField(kp.IDA()...)
		return
	}
	f, err := dst.EnsureField(kp, refctx)
	if err != nil {
		c.err.Errors = append(c.err.Errors, err.(d2ast.Error))
		return
	}

	if refctx.Key.Primary.Unbox() != nil {
		f.Primary_ = &Scalar{
			parent: f,
			Value:  refctx.Key.Primary.Unbox(),
		}
	}
	if refctx.Key.Value.Array != nil {
		a := &Array{
			parent: f,
		}
		c.compileArray(a, refctx.Key.Value.Array, refctx.ScopeAST)
		f.Composite = a
	} else if refctx.Key.Value.Map != nil {
		if f.Map() == nil {
			f.Composite = &Map{
				parent: f,
			}
		}
		scopeAST := refctx.Key.Value.Map
		switch NodeBoardKind(f) {
		case BoardScenario:
			c.overlay(ParentBoard(f).Map(), f)
		case BoardStep:
			stepsMap := ParentMap(f)
			for i := range stepsMap.Fields {
				if stepsMap.Fields[i] == f {
					if i == 0 {
						c.overlay(ParentBoard(f).Map(), f)
					} else {
						c.overlay(stepsMap.Fields[i-1].Map(), f)
					}
					break
				}
			}
		case BoardLayer:
			c.overlayVars(f.Map(), ParentBoard(f).Map())
		default:
			// If new board type, use that as the new scope AST, otherwise, carry on
			scopeAST = refctx.ScopeAST
		}
		c.compileMap(f.Map(), refctx.Key.Value.Map, scopeAST)
		switch NodeBoardKind(f) {
		case BoardScenario, BoardStep:
			c.overlayClasses(f.Map())
		}
	} else if refctx.Key.Value.Import != nil {
		n, ok := c._import(refctx.Key.Value.Import)
		if !ok {
			return
		}
		switch n := n.(type) {
		case *Field:
			if n.Primary_ != nil {
				f.Primary_ = n.Primary_.Copy(f).(*Scalar)
			}
			if n.Composite != nil {
				f.Composite = n.Composite.Copy(f).(Composite)
			}
		case *Map:
			f.Composite = &Map{
				parent: f,
			}
			switch NodeBoardKind(f) {
			case BoardScenario:
				c.overlay(ParentBoard(f).Map(), f)
			case BoardStep:
				stepsMap := ParentMap(f)
				for i := range stepsMap.Fields {
					if stepsMap.Fields[i] == f {
						if i == 0 {
							c.overlay(ParentBoard(f).Map(), f)
						} else {
							c.overlay(stepsMap.Fields[i-1].Map(), f)
						}
						break
					}
				}
			}
			OverlayMap(f.Map(), n)
			c.updateLinks(f.Map())
			switch NodeBoardKind(f) {
			case BoardScenario, BoardStep:
				c.overlayClasses(f.Map())
			}
		}
	} else if refctx.Key.Value.ScalarBox().Unbox() != nil {
		// If the link is a board, we need to transform it into an absolute path.
		if f.Name == "link" {
			c.compileLink(refctx)
		}
		f.Primary_ = &Scalar{
			parent: f,
			Value:  refctx.Key.Value.ScalarBox().Unbox(),
		}
	}
}

func (c *compiler) updateLinks(m *Map) {
	for _, f := range m.Fields {
		if f.Name == "link" {
			bida := BoardIDA(f)
			aida := IDA(f)
			if len(bida) != len(aida) {
				prependIDA := aida[:len(aida)-len(bida)]
				kp := d2ast.MakeKeyPath(prependIDA)
				s := d2format.Format(kp) + strings.TrimPrefix(f.Primary_.Value.ScalarString(), "root")
				f.Primary_.Value = d2ast.MakeValueBox(d2ast.FlatUnquotedString(s)).ScalarBox().Unbox()
			}
		}
		if f.Map() != nil {
			c.updateLinks(f.Map())
		}
	}
}

func (c *compiler) compileLink(refctx *RefContext) {
	val := refctx.Key.Value.ScalarBox().Unbox().ScalarString()
	link, err := d2parser.ParseKey(val)
	if err != nil {
		return
	}

	scopeIDA := IDA(refctx.ScopeMap)

	if len(scopeIDA) == 0 {
		return
	}

	linkIDA := link.IDA()
	if len(linkIDA) == 0 {
		return
	}

	if linkIDA[0] == "root" {
		c.errorf(refctx.Key.Key, "cannot refer to root in link")
		return
	}

	// If it doesn't start with one of these reserved words, the link is definitely not a board link.
	if !strings.EqualFold(linkIDA[0], "layers") && !strings.EqualFold(linkIDA[0], "scenarios") && !strings.EqualFold(linkIDA[0], "steps") && linkIDA[0] != "_" {
		return
	}

	// Chop off the non-board portion of the scope, like if this is being defined on a nested object (e.g. `x.y.z`)
	for i := len(scopeIDA) - 1; i > 0; i-- {
		if strings.EqualFold(scopeIDA[i-1], "layers") || strings.EqualFold(scopeIDA[i-1], "scenarios") || strings.EqualFold(scopeIDA[i-1], "steps") {
			scopeIDA = scopeIDA[:i+1]
			break
		}
		if scopeIDA[i-1] == "root" {
			scopeIDA = scopeIDA[:i]
			break
		}
	}

	// Resolve underscores
	for len(linkIDA) > 0 && linkIDA[0] == "_" {
		if len(scopeIDA) < 2 {
			// IR compiler only validates bad underscore usage
			// The compiler will validate if the target board actually exists
			c.errorf(refctx.Key.Key, "invalid underscore usage")
			return
		}
		// pop 2 off path per one underscore
		scopeIDA = scopeIDA[:len(scopeIDA)-2]
		linkIDA = linkIDA[1:]
	}
	if len(scopeIDA) == 0 {
		scopeIDA = []string{"root"}
	}

	// Create the absolute path by appending scope path with value specified
	scopeIDA = append(scopeIDA, linkIDA...)
	kp := d2ast.MakeKeyPath(scopeIDA)
	refctx.Key.Value = d2ast.MakeValueBox(d2ast.FlatUnquotedString(d2format.Format(kp)))
}

func (c *compiler) compileEdges(refctx *RefContext) {
	if refctx.Key.Key != nil {
		f, err := refctx.ScopeMap.EnsureField(refctx.Key.Key, refctx)
		if err != nil {
			c.err.Errors = append(c.err.Errors, err.(d2ast.Error))
			return
		}
		if _, ok := f.Composite.(*Array); ok {
			c.errorf(refctx.Key.Key, "cannot index into array")
			return
		}
		if f.Map() == nil {
			f.Composite = &Map{
				parent: f,
			}
		}
		refctx.ScopeMap = f.Map()
	}

	eida := NewEdgeIDs(refctx.Key)
	for i, eid := range eida {
		if refctx.Key != nil && refctx.Key.Value.Null != nil {
			refctx.ScopeMap.DeleteEdge(eid)
			continue
		}

		refctx = refctx.Copy()
		refctx.Edge = refctx.Key.Edges[i]

		var e *Edge
		if eid.Index != nil {
			ea := refctx.ScopeMap.GetEdges(eid)
			if len(ea) == 0 {
				c.errorf(refctx.Edge, "indexed edge does not exist")
				continue
			}
			e = ea[0]
			e.References = append(e.References, &EdgeReference{
				Context: refctx,
			})
			refctx.ScopeMap.appendFieldReferences(0, refctx.Edge.Src, refctx)
			refctx.ScopeMap.appendFieldReferences(0, refctx.Edge.Dst, refctx)
		} else {
			_, err := refctx.ScopeMap.EnsureField(refctx.Edge.Src, refctx)
			if err != nil {
				c.err.Errors = append(c.err.Errors, err.(d2ast.Error))
				continue
			}
			_, err = refctx.ScopeMap.EnsureField(refctx.Edge.Dst, refctx)
			if err != nil {
				c.err.Errors = append(c.err.Errors, err.(d2ast.Error))
				continue
			}

			e, err = refctx.ScopeMap.CreateEdge(eid, refctx)
			if err != nil {
				c.err.Errors = append(c.err.Errors, err.(d2ast.Error))
				continue
			}
		}

		if refctx.Key.EdgeKey != nil {
			if e.Map_ == nil {
				e.Map_ = &Map{
					parent: e,
				}
			}
			c.compileField(e.Map_, refctx.Key.EdgeKey, refctx)
		} else {
			if refctx.Key.Primary.Unbox() != nil {
				e.Primary_ = &Scalar{
					parent: e,
					Value:  refctx.Key.Primary.Unbox(),
				}
			}
			if refctx.Key.Value.Array != nil {
				c.errorf(refctx.Key.Value.Unbox(), "edges cannot be assigned arrays")
				continue
			} else if refctx.Key.Value.Map != nil {
				if e.Map_ == nil {
					e.Map_ = &Map{
						parent: e,
					}
				}
				c.compileMap(e.Map_, refctx.Key.Value.Map, refctx.ScopeAST)
			} else if refctx.Key.Value.ScalarBox().Unbox() != nil {
				e.Primary_ = &Scalar{
					parent: e,
					Value:  refctx.Key.Value.ScalarBox().Unbox(),
				}
			}
		}
	}
}

func (c *compiler) compileArray(dst *Array, a *d2ast.Array, scopeAST *d2ast.Map) {
	for _, an := range a.Nodes {
		var irv Value
		switch v := an.Unbox().(type) {
		case *d2ast.Array:
			ira := &Array{
				parent: dst,
			}
			c.compileArray(ira, v, scopeAST)
			irv = ira
		case *d2ast.Map:
			irm := &Map{
				parent: dst,
			}
			c.compileMap(irm, v, scopeAST)
			irv = irm
		case d2ast.Scalar:
			irv = &Scalar{
				parent: dst,
				Value:  v,
			}
		case *d2ast.Import:
			n, ok := c._import(v)
			if !ok {
				continue
			}
			switch n := n.(type) {
			case *Field:
				if v.Spread {
					a, ok := n.Composite.(*Array)
					if !ok {
						c.errorf(v, "can only spread import array into array")
						continue
					}
					dst.Values = append(dst.Values, a.Values...)
					continue
				}
				if n.Composite != nil {
					irv = n.Composite
				} else {
					irv = n.Primary_
				}
			case *Map:
				if v.Spread {
					c.errorf(v, "can only spread import array into array")
					continue
				}
				irv = n
			}
		case *d2ast.Substitution:
			// panic("TODO")
		}

		dst.Values = append(dst.Values, irv)
	}
}
