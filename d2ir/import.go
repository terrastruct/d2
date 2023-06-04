package d2ir

import (
	"bufio"
	"path"

	"oss.terrastruct.com/d2/d2ast"
	"oss.terrastruct.com/d2/d2parser"
)

func (c *compiler) pushImportStack(imp *d2ast.Import) bool {
	if len(imp.Path) == 0 {
		c.errorf(imp, "imports must specify a path to import")
		return false
	}

	newPath := imp.Path[0].Unbox().ScalarString()
	for _, p := range c.importStack {
		if newPath == p {
			c.errorf(imp, "detected cyclic import of %q", newPath)
			return false
		}
	}

	c.importStack = append(c.importStack, newPath)
	return true
}

func (c *compiler) popImportStack() {
	c.importStack = c.importStack[:len(c.importStack)-1]
}

// Returns either *Map or *Field.
func (c *compiler) _import(imp *d2ast.Import) (Node, bool) {
	ir, ok := c.__import(imp)
	if !ok {
		return nil, false
	}
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

	f, err := c.fs.Open(impPath)
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
