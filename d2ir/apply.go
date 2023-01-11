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

func Apply(dst *Map, ast *d2ast.Map) error {
	var c compiler
	c.compileMap(dst, ast)
	if !c.err.Empty() {
		return c.err
	}
	return nil
}

func (c *compiler) compileMap(dst *Map, ast *d2ast.Map) {
	for _, n := range ast.Nodes {
		switch {
		case n.MapKey != nil:
			c.compileField(dst, n.MapKey)
		case n.Substitution != nil:
			panic("TODO")
		}
	}
}

func (c *compiler) compileField(dst *Map, k *d2ast.Key) {
	if k.Key != nil && len(k.Key.Path) > 0 {
		f, ok := dst.Ensure(d2format.KeyPath(k.Key))
		if !ok {
			c.errorf(k, "cannot index into array")
			return
		}

		if len(k.Edges) == 0 {
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
				m := &Map{
					parent: f,
				}
				c.compileMap(m, k.Value.Map)
				f.Composite = m
			} else if k.Value.ScalarBox().Unbox() != nil {
				f.Primary = &Scalar{
					parent: f,
					Value:  k.Value.ScalarBox().Unbox(),
				}
			}
		}
	}
}

func (c *compiler) compileArray(dst *Array, a *d2ast.Array) {
	panic(fmt.Sprintf("TODO"))
}
