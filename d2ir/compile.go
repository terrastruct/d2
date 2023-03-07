package d2ir

import (
	"strings"

	"oss.terrastruct.com/d2/d2ast"
	"oss.terrastruct.com/d2/d2format"
	"oss.terrastruct.com/d2/d2parser"
)

type compiler struct {
	err d2parser.ParseError
}

func (c *compiler) errorf(n d2ast.Node, f string, v ...interface{}) {
	c.err.Errors = append(c.err.Errors, d2parser.Errorf(n, f, v...).(d2ast.Error))
}

func Compile(ast *d2ast.Map) (*Map, error) {
	c := &compiler{}
	m := &Map{}
	m.initRoot()
	m.parent.(*Field).References[0].Context.Scope = ast
	c.compileMap(m, ast)
	c.compileScenarios(m)
	c.compileSteps(m)
	if !c.err.Empty() {
		return nil, c.err
	}
	return m, nil
}

func (c *compiler) compileScenarios(m *Map) {
	scenariosf := m.GetField("scenarios")
	if scenariosf == nil {
		return
	}
	scenarios := scenariosf.Map()
	if scenarios == nil {
		return
	}

	for _, sf := range scenarios.Fields {
		if sf.Map() == nil || sf.Primary() != nil {
			c.errorf(sf.References[0].Context.Key, "invalid scenario")
			continue
		}
		base := m.CopyBase(sf)
		OverlayMap(base, sf.Map())
		sf.Composite = base
		c.compileScenarios(sf.Map())
		c.compileSteps(sf.Map())
	}
}

func (c *compiler) compileSteps(m *Map) {
	stepsf := m.GetField("steps")
	if stepsf == nil {
		return
	}
	steps := stepsf.Map()
	if steps == nil {
		return
	}
	for i, sf := range steps.Fields {
		if sf.Map() == nil || sf.Primary() != nil {
			c.errorf(sf.References[0].Context.Key, "invalid step")
			break
		}
		var base *Map
		if i == 0 {
			base = m.CopyBase(sf)
		} else {
			base = steps.Fields[i-1].Map().CopyBase(sf)
		}
		OverlayMap(base, sf.Map())
		sf.Composite = base
		c.compileScenarios(sf.Map())
		c.compileSteps(sf.Map())
	}
}

func (c *compiler) compileMap(dst *Map, ast *d2ast.Map) {
	for _, n := range ast.Nodes {
		switch {
		case n.MapKey != nil:
			c.compileKey(&RefContext{
				Key:      n.MapKey,
				Scope:    ast,
				ScopeMap: dst,
			})
		case n.Substitution != nil:
			panic("TODO")
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
		c.compileArray(a, refctx.Key.Value.Array)
		f.Composite = a
	} else if refctx.Key.Value.Map != nil {
		if f.Map() == nil {
			f.Composite = &Map{
				parent: f,
			}
		}
		c.compileMap(f.Map(), refctx.Key.Value.Map)
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
	refctx.Key.Value = d2ast.MakeValueBox(d2ast.RawString(d2format.Format(kp), true))
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
				c.compileMap(e.Map_, refctx.Key.Value.Map)
			} else if refctx.Key.Value.ScalarBox().Unbox() != nil {
				e.Primary_ = &Scalar{
					parent: e,
					Value:  refctx.Key.Value.ScalarBox().Unbox(),
				}
			}
		}
	}
}

func (c *compiler) compileArray(dst *Array, a *d2ast.Array) {
	for _, an := range a.Nodes {
		var irv Value
		switch v := an.Unbox().(type) {
		case *d2ast.Array:
			ira := &Array{
				parent: dst,
			}
			c.compileArray(ira, v)
			irv = ira
		case *d2ast.Map:
			irm := &Map{
				parent: dst,
			}
			c.compileMap(irm, v)
			irv = irm
		case d2ast.Scalar:
			irv = &Scalar{
				parent: dst,
				Value:  v,
			}
		case *d2ast.Substitution:
			// panic("TODO")
		}

		dst.Values = append(dst.Values, irv)
	}
}
