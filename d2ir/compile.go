package d2ir

import (
	"fmt"

	"oss.terrastruct.com/d2/d2ast"
	"oss.terrastruct.com/d2/d2format"
	"oss.terrastruct.com/d2/d2parser"
)

type compiler struct {
	err d2parser.ParseError
}

func (c *compiler) errorf(n d2ast.Node, f string, v ...interface{}) {
	f = "%v: " + f
	v = append([]interface{}{n.GetRange()}, v...)
	c.err.Errors = append(c.err.Errors, d2ast.Error{
		Range:   n.GetRange(),
		Message: fmt.Sprintf(f, v...),
	})
}

func Compile(ast *d2ast.Map) (*IR, error) {
	ir := &IR{
		AST: ast,
		Map: &Map{},
	}

	c := &compiler{}
	c.compile(ir)
	if !c.err.Empty() {
		return nil, c.err
	}
	return ir, nil
}

func (c *compiler) compile(ir *IR) {
	c.compileMap(ir.Map, ir.AST)
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
		c.compileField(dst, refctx.Key)
		dst.appendFieldReferences(0, refctx.Key.Key, refctx)
	} else {
		c.compileEdges(dst, refctx)
	}
}

func (c *compiler) compileField(dst *Map, k *d2ast.Key) {
	f, err := dst.EnsureField(d2format.KeyPath(k.Key))
	if err != nil {
		c.errorf(k, err.Error())
		return
	}

	if k.Primary.Unbox() != nil {
		f.Primary = &Scalar{
			parent: f,
			Value:  k.Primary.Unbox(),
		}
	}
	if k.Value.Array != nil {
		a := &Array{
			parent: f,
		}
		c.compileArray(a, k.Value.Array)
		f.Composite = a
	} else if k.Value.Map != nil {
		f_m := ToMap(f)
		if f_m == nil {
			f_m = &Map{
				parent: f,
			}
			f.Composite = f_m
		}
		c.compileMap(f_m, k.Value.Map)
	} else if k.Value.ScalarBox().Unbox() != nil {
		f.Primary = &Scalar{
			parent: f,
			Value:  k.Value.ScalarBox().Unbox(),
		}
	}
}

func (c *compiler) compileEdges(dst *Map, refctx *RefContext) {
	if refctx.Key.Key != nil && len(refctx.Key.Key.Path) > 0 {
		f, err := dst.EnsureField(d2format.KeyPath(refctx.Key.Key))
		if err != nil {
			c.errorf(refctx.Key.Key, err.Error())
			return
		}
		dst.appendFieldReferences(0, refctx.Key.Key, refctx)
		if _, ok := f.Composite.(*Array); ok {
			c.errorf(refctx.Key.Key, "cannot index into array")
			return
		}
		f_m := ToMap(f)
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
		var e *Edge
		if eid.Index != nil {
			ea := dst.GetEdges(eid)
			if len(ea) == 0 {
				c.errorf(refctx.Key.Edges[i], "indexed edge does not exist")
				continue
			}
			e = ea[0]
		} else {
			_, err := dst.EnsureField(eid.SrcPath)
			if err != nil {
				c.errorf(refctx.Key.Edges[i].Src, err.Error())
				continue
			}
			_, err = dst.EnsureField(eid.DstPath)
			if err != nil {
				c.errorf(refctx.Key.Edges[i].Dst, err.Error())
				continue
			}

			e, err = dst.EnsureEdge(eid)
			if err != nil {
				c.errorf(refctx.Key.Edges[i], err.Error())
				continue
			}
		}

		refctx = refctx.Copy()
		refctx.Edge = refctx.Key.Edges[i]
		dst.appendEdgeReferences(e, refctx)

		if refctx.Key.EdgeKey != nil {
			if e.Map == nil {
				e.Map = &Map{
					parent: e,
				}
			}
			tmpk := &d2ast.Key{
				Range: refctx.Key.EdgeKey.Range,
				Key:   refctx.Key.EdgeKey,
			}
			c.compileField(e.Map, tmpk)
			e.Map.appendFieldReferences(0, refctx.Key.EdgeKey, refctx)
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
