package d2ir

import (
	"bufio"
	"os"
	"path"
	"strings"

	"oss.terrastruct.com/d2/d2ast"
	"oss.terrastruct.com/d2/d2parser"
)

func (c *compiler) pushImportStack(imp *d2ast.Import) bool {
	if len(imp.Path) == 0 {
		c.errorf(imp, "imports must specify a path to import")
		return false
	}

	newPath := imp.Path[0].Unbox().ScalarString()
	for i, p := range c.importStack {
		if newPath == p {
			c.errorf(imp, "detected cyclic import chain: %s", formatCyclicChain(c.importStack[i:]))
			return false
		}
	}

	c.importStack = append(c.importStack, newPath)
	return true
}

func (c *compiler) popImportStack() {
	c.importStack = c.importStack[:len(c.importStack)-1]
}

func formatCyclicChain(cyclicChain []string) string {
	var b strings.Builder
	for _, p := range cyclicChain {
		b.WriteString(p)
		b.WriteString(" -> ")
	}
	b.WriteString(cyclicChain[0])
	return b.String()
}

// Returns either *Map or *Field.
func (c *compiler) _import(imp *d2ast.Import) (Node, bool) {
	ir, ok := c.__import(imp)
	if !ok {
		return nil, false
	}
	nilScopeMap(ir)
	if len(imp.IDA()) > 0 {
		f := ir.GetField(imp.IDA()...)
		if f == nil {
			c.errorf(imp, "import key %q doesn't exist inside import", imp.IDA())
			return nil, false
		}
		return f, true
	}
	return ir, true
}

func (c *compiler) __import(imp *d2ast.Import) (*Map, bool) {
	impPath := imp.Path[0].Unbox().ScalarString()
	if path.IsAbs(impPath) {
		c.errorf(imp, "import paths must be relative")
		return nil, false
	}

	if path.Ext(impPath) != ".d2" {
		impPath += ".d2"
	}

	// Imports are always relative to the importing file.
	impPath = path.Join(path.Dir(c.importStack[len(c.importStack)-1]), impPath)

	if !c.pushImportStack(imp) {
		return nil, false
	}
	defer c.popImportStack()

	ir, ok := c.importCache[impPath]
	if ok {
		return ir, true
	}

	p := path.Clean(impPath)
	if path.IsAbs(p) {
		// Path cannot be absolute. DirFS does not accept absolute paths. We strip off the leading
		// slash to make it relative to the root.
		p = p[1:]
	} else if c.fs == os.DirFS("/") {
		wd, err := os.Getwd()
		if err != nil {
			c.errorf(imp, "failed to import %q: %v", impPath, err)
			return nil, false
		}
		p = path.Join(wd, p)
		// See above explanation.
		if path.IsAbs(p) {
			p = p[1:]
		}
	}

	f, err := c.fs.Open(p)
	if err != nil {
		c.errorf(imp, "failed to import %q: %v", impPath, err)
		return nil, false
	}
	defer f.Close()

	ast, err := d2parser.Parse(impPath, bufio.NewReader(f), &d2parser.ParseOptions{
		UTF16:      c.utf16,
		ParseError: c.err,
	})
	if err != nil {
		return nil, false
	}

	ir = &Map{}
	ir.initRoot()
	ir.parent.(*Field).References[0].Context.Scope = ast

	c.compileMap(ir, ast)

	c.importCache[impPath] = ir

	return ir, true
}

func nilScopeMap(n Node) {
	switch n := n.(type) {
	case *Map:
		for _, f := range n.Fields {
			nilScopeMap(f)
		}
		for _, e := range n.Edges {
			nilScopeMap(e)
		}
	case *Edge:
		for _, r := range n.References {
			r.Context.ScopeMap = nil
		}
		if n.Map() != nil {
			nilScopeMap(n.Map())
		}
	case *Field:
		for _, r := range n.References {
			r.Context.ScopeMap = nil
		}
		if n.Map() != nil {
			nilScopeMap(n.Map())
		}
	}
}
