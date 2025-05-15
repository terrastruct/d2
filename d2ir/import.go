package d2ir

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"oss.terrastruct.com/d2/d2ast"
	"oss.terrastruct.com/d2/d2parser"
)

func (c *compiler) pushImportStack(imp *d2ast.Import) (string, bool) {
	impPath := imp.PathWithPre()
	if impPath == "" && imp.Range != (d2ast.Range{}) {
		c.errorf(imp, "imports must specify a path to import")
		return "", false
	}
	if len(c.importStack) > 0 {
		if path.Ext(impPath) != ".d2" {
			impPath += ".d2"
		}

		if !filepath.IsAbs(impPath) {
			impPath = path.Join(path.Dir(c.importStack[len(c.importStack)-1]), impPath)
		}
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

	// Only get immediate imports.
	if len(c.importStack) == 2 {
		if _, ok := c.seenImports[impPath]; !ok {
			c.imports = append(c.imports, imp.PathWithPre())
		}
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

	ast, err := d2parser.Parse(impPath, f, &d2parser.ParseOptions{
		UTF16Pos:   c.utf16Pos,
		ParseError: c.err,
	})
	if err != nil {
		return nil, false
	}

	ir := &Map{}
	ir.initRoot()
	ir.parent.(*Field).References[0].Context_.Scope = ast

	c.compileMap(ir, ast, ast)

	// We attempt to resolve variables in the imported file scope first
	// But ignore errors, in case the variable is meant to be resolved at the
	// importer
	savedErrors := make([]d2ast.Error, len(c.err.Errors))
	copy(savedErrors, c.err.Errors)
	c.compileSubstitutions(ir, nil)
	c.err.Errors = savedErrors

	c.seenImports[impPath] = struct{}{}

	return ir, true
}

func (c *compiler) peekImport(imp *d2ast.Import) (*Map, bool) {
	impPath := imp.PathWithPre()
	if impPath == "" && imp.Range != (d2ast.Range{}) {
		return nil, false
	}

	if len(c.importStack) > 0 {
		if path.Ext(impPath) != ".d2" {
			impPath += ".d2"
		}

		if !filepath.IsAbs(impPath) {
			impPath = path.Join(path.Dir(c.importStack[len(c.importStack)-1]), impPath)
		}
	}

	var f fs.File
	var err error
	if c.fs == nil {
		f, err = os.Open(impPath)
	} else {
		f, err = c.fs.Open(impPath)
	}
	if err != nil {
		return nil, false
	}
	defer f.Close()

	// Use a separate parse error to avoid polluting the main one
	localErr := &d2parser.ParseError{}
	ast, err := d2parser.Parse(impPath, f, &d2parser.ParseOptions{
		UTF16Pos:   c.utf16Pos,
		ParseError: localErr,
	})
	if err != nil {
		return nil, false
	}

	ir := &Map{}
	ir.initRoot()
	ir.parent.(*Field).References[0].Context_.Scope = ast

	c.compileMap(ir, ast, ast)

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
			r.Context_.ScopeMap = nil
		}
		if n.Map() != nil {
			nilScopeMap(n.Map())
		}
	case *Field:
		for _, r := range n.References {
			r.Context_.ScopeMap = nil
		}
		if n.Map() != nil {
			nilScopeMap(n.Map())
		}
	}
}
