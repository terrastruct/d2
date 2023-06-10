package d2ir

import (
	"bufio"
	"io/fs"
	"os"
	"path"
	"strings"

	"oss.terrastruct.com/d2/d2ast"
	"oss.terrastruct.com/d2/d2parser"
)

func (c *compiler) pushImportStack(imp *d2ast.Import) (string, bool) {
	impPath := imp.PathWithPre()
	if impPath == "" && imp.Range.Path != "" {
		c.errorf(imp, "imports must specify a path to import")
		return "", false
	}
	if len(c.importStack) > 0 {
		if path.IsAbs(impPath) {
			c.errorf(imp, "import paths must be relative")
			return "", false
		}

		if path.Ext(impPath) != ".d2" {
			impPath += ".d2"
		}

		// Imports are always relative to the importing file.
		impPath = path.Join(path.Dir(c.importStack[len(c.importStack)-1]), impPath)
	}

	for i, p := range c.importStack {
		if impPath == p {
			c.errorf(imp, "detected cyclic import chain: %s", formatCyclicChain(c.importStack[i:]))
			return "", false
		}
	}

	c.importStack = append(c.importStack, impPath)
	return impPath, true
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
	impPath, ok := c.pushImportStack(imp)
	if !ok {
		return nil, false
	}
	defer c.popImportStack()

	ir, ok := c.importCache[impPath]
	if ok {
		return ir, true
	}

	var f fs.File
	var err error
	if c.fs == nil {
		f, err = os.Open(impPath)
	} else {
		f, err = c.fs.Open(impPath)
	}
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
