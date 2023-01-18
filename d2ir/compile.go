package d2ir

import (
	"oss.terrastruct.com/d2/d2ast"
	"oss.terrastruct.com/d2/d2parser"
)

type compiler struct {
	err d2parser.ParseError
}

func (c *compiler) errorf(n d2ast.Node, f string, v ...interface{}) {
	c.err.Errors = append(c.err.Errors, d2parser.Errorf(n, f, v...).(d2ast.Error))
}

func Compile(ast *d2ast.Map) (*Map, error) {
	m := &Map{}
	c := &compiler{}
	c.compile(m, ast)
	if !c.err.Empty() {
		return nil, c.err
	}
	return m, nil
}

func (c *compiler) compile(dst *Map, ast *d2ast.Map) {
	c.compileMap(dst, ast)
}

func (c *compiler) compileMap(dst *Map, ast *d2ast.Map) {
	for _, n := range ast.Nodes {
		switch {
		case n.MapKey != nil:
			c.compileKey(dst, &RefContext{
				Key:   n.MapKey,
				Scope: ast,
			})
		case n.Substitution != nil:
			panic("TODO")
		}
	}
}

func (c *compiler) compileKey(dst *Map, refctx *RefContext) {
	if len(refctx.Key.Edges) == 0 {
		c.compileField(dst, refctx.Key.Key, refctx)
	} else {
		c.compileEdges(dst, refctx)
	}
}

func (c *compiler) compileField(dst *Map, kp *d2ast.KeyPath, refctx *RefContext) {
	f, err := dst.EnsureField(kp, refctx)
	if err != nil {
		c.err.Errors = append(c.err.Errors, err.(d2ast.Error))
		return
	}

	if refctx.Key.Primary.Unbox() != nil {
		f.Primary = &Scalar{
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
		f_m := ChildMap(f)
		if f_m == nil {
			f_m = &Map{
				parent: f,
			}
			f.Composite = f_m
		}
		c.compileMap(f_m, refctx.Key.Value.Map)
	} else if refctx.Key.Value.ScalarBox().Unbox() != nil {
		f.Primary = &Scalar{
			parent: f,
			Value:  refctx.Key.Value.ScalarBox().Unbox(),
		}
	}
}

func (c *compiler) compileEdges(dst *Map, refctx *RefContext) {
	if refctx.Key.Key != nil {
		f, err := dst.EnsureField(refctx.Key.Key, refctx)
		if err != nil {
			c.err.Errors = append(c.err.Errors, err.(d2ast.Error))
			return
		}
		if _, ok := f.Composite.(*Array); ok {
			c.errorf(refctx.Key.Key, "cannot index into array")
			return
		}
		f_m := ChildMap(f)
		if f_m == nil {
			f_m = &Map{
				parent: f,
			}
			f.Composite = f_m
		}
		dst = f_m
	}

	eida := NewEdgeIDs(refctx.Key)
	for i, eid := range eida {
		refctx = refctx.Copy()
		refctx.Edge = refctx.Key.Edges[i]

		var e *Edge
		if eid.Index != nil {
			ea := dst.GetEdges(eid)
			if len(ea) == 0 {
				c.errorf(refctx.Edge, "indexed edge does not exist")
				continue
			}
			e = ea[0]
			e.References = append(e.References, EdgeReference{
				Context: refctx,
			})
			dst.appendFieldReferences(0, refctx.Edge.Src, refctx)
			dst.appendFieldReferences(0, refctx.Edge.Dst, refctx)
		} else {
			_, err := dst.EnsureField(refctx.Edge.Src, refctx)
			if err != nil {
				c.err.Errors = append(c.err.Errors, err.(d2ast.Error))
				continue
			}
			_, err = dst.EnsureField(refctx.Edge.Dst, refctx)
			if err != nil {
				c.err.Errors = append(c.err.Errors, err.(d2ast.Error))
				continue
			}

			e, err = dst.CreateEdge(eid, refctx)
			if err != nil {
				c.err.Errors = append(c.err.Errors, err.(d2ast.Error))
				continue
			}
		}

		if refctx.Key.EdgeKey != nil {
			if e.Map == nil {
				e.Map = &Map{
					parent: e,
				}
			}
			c.compileField(e.Map, refctx.Key.EdgeKey, refctx)
		} else {
			if refctx.Key.Primary.Unbox() != nil {
				e.Primary = &Scalar{
					parent: e,
					Value:  refctx.Key.Primary.Unbox(),
				}
			} else if refctx.Key.Value.Map != nil {
				if e.Map == nil {
					e.Map = &Map{
						parent: e,
					}
				}
				c.compileMap(e.Map, refctx.Key.Value.Map)
			} else if refctx.Key.Value.Unbox() != nil {
				c.errorf(refctx.Key.Value.Unbox(), "edges cannot be assigned arrays")
				continue
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
			panic("TODO")
		}

		dst.Values = append(dst.Values, irv)
	}
}
