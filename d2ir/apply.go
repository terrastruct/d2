package d2ir

import (
	"fmt"

	"oss.terrastruct.com/d2/d2ast"
	"oss.terrastruct.com/d2/d2graph"
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
	c.apply(dst, ast)
	if !c.err.Empty() {
		return c.err
	}
	return nil
}

func (c *compiler) apply(dst *Map, ast *d2ast.Map) {
	for _, n := range ast.Nodes {
		if n.MapKey == nil {
			continue
		}

		c.applyKey(dst, n.MapKey)
	}
}

func (c *compiler) applyKey(dst *Map, k *d2ast.Key) {
	if k.Key != nil && len(k.Key.Path) > 0 {
		f, ok := dst.Ensure(d2graph.Key(k.Key))
		if !ok {
			c.errorf(k.Key, "cannot index into array")
			return
		}

		if len(k.Edges) == 0 {
			if k.Primary.Unbox() != nil {
				f.Primary = &Scalar{
					parent: f,
					Value:  k.Primary.Unbox(),
				}
			}
		}
	}
}
