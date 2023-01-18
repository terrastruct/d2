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
			c.compileKey(dst, n.MapKey)
		case n.Substitution != nil:
			panic("TODO")
		}
	}
}

func (c *compiler) compileKey(dst *Map, k *d2ast.Key) {
	if len(k.Edges) == 0 {
		c.compileField(dst, k)
	} else {
		c.compileEdges(dst, k)
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
		if f_m, ok := f.Composite.(*Map); ok {
			c.compileMap(f_m, k.Value.Map)
		} else {
			m := &Map{
				parent: f,
			}
			c.compileMap(m, k.Value.Map)
			f.Composite = m
		}
	} else if k.Value.ScalarBox().Unbox() != nil {
		f.Primary = &Scalar{
			parent: f,
			Value:  k.Value.ScalarBox().Unbox(),
		}
	}
}

func (c *compiler) compileEdges(dst *Map, k *d2ast.Key) {
	if k.Key != nil && len(k.Key.Path) > 0 {
		f, err := dst.EnsureField(d2format.KeyPath(k.Key))
		if err != nil {
			c.errorf(k, err.Error())
			return
		}
		if f_m, ok := f.Composite.(*Map); ok {
			dst = f_m
		} else {
			m := &Map{
				parent: f,
			}
			f.Composite = m
			dst = m
		}
	}

	eida := NewEdgeIDs(k)
	for i, eid := range eida {
		var e *Edge
		if eid.Index != nil {
			ea := dst.GetEdges(eid)
			if len(ea) == 0 {
				c.errorf(k.Edges[i], "indexed edge does not exist")
				continue
			}
			e = ea[0]
		} else {
			_, err := dst.EnsureField(eid.SrcPath)
			if err != nil {
				c.errorf(k.Edges[i].Src, err.Error())
				continue
			}
			_, err = dst.EnsureField(eid.DstPath)
			if err != nil {
				c.errorf(k.Edges[i].Dst, err.Error())
				continue
			}

			e, err = dst.EnsureEdge(eid)
			if err != nil {
				c.errorf(k.Edges[i], err.Error())
				continue
			}
		}

		if k.EdgeKey != nil {
			if e.Map == nil {
				e.Map = &Map{
					parent: e,
				}
			}
			tmpk := &d2ast.Key{
				Range: k.EdgeKey.Range,
				Key:   k.EdgeKey,
			}
			c.compileField(e.Map, tmpk)
		} else {
			if k.Primary.Unbox() != nil {
				e.Primary = &Scalar{
					parent: e,
					Value:  k.Primary.Unbox(),
				}
			} else if k.Value.Map != nil {
				if e.Map == nil {
					e.Map = &Map{
						parent: e,
					}
				}
				c.compileMap(e.Map, k.Value.Map)
			} else if k.Value.Unbox() != nil {
				c.errorf(k.Value.Unbox(), "edges cannot be assigned arrays")
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
