// d2lsp contains functions useful for IDE clients
package d2lsp

import (
	"fmt"
	"path/filepath"
	"strings"

	"oss.terrastruct.com/d2/d2ast"
	"oss.terrastruct.com/d2/d2ir"
	"oss.terrastruct.com/d2/d2parser"
	"oss.terrastruct.com/d2/lib/memfs"
)

func GetRefs(path string, fs map[string]string, boardPath []string, key string) (refs []d2ir.Reference, _ error) {
	m, err := getBoardMap(path, fs, boardPath)
	if err != nil {
		return nil, err
	}

	mk, err := d2parser.ParseMapKey(key)
	if err != nil {
		return nil, err
	}
	if mk.Key == nil && len(mk.Edges) == 0 {
		return nil, fmt.Errorf(`"%s" is invalid`, key)
	}

	var f *d2ir.Field
	if mk.Key != nil {
		for _, p := range mk.Key.Path {
			f = m.GetField(p.Unbox().ScalarString())
			if f == nil {
				return nil, nil
			}
			m = f.Map()
		}
	}

	if len(mk.Edges) > 0 {
		eids := d2ir.NewEdgeIDs(mk)
		var edges []*d2ir.Edge
		for _, eid := range eids {
			edges = append(edges, m.GetEdges(eid, nil, nil)...)
		}
		if len(edges) == 0 {
			return nil, nil
		}
		for _, edge := range edges {
			for _, ref := range edge.References {
				refs = append(refs, ref)
			}
		}
		return refs, nil
	} else {
		for _, ref := range f.References {
			refs = append(refs, ref)
		}
	}
	return refs, nil
}

func GetImportRanges(path, file string, importPath string) (ranges []d2ast.Range, _ error) {
	r := strings.NewReader(file)
	ast, err := d2parser.Parse(path, r, nil)
	if err != nil {
		return nil, err
	}

	d2ast.Walk(ast, func(n d2ast.Node) bool {
		switch t := n.(type) {
		case *d2ast.Import:
			if (filepath.Join(filepath.Dir(path), t.PathWithPre()) + ".d2") == importPath {
				ranges = append(ranges, t.Range)
			}
		}
		return true
	})

	return ranges, nil
}

func getBoardMap(path string, fs map[string]string, boardPath []string) (*d2ir.Map, error) {
	if _, ok := fs[path]; !ok {
		return nil, fmt.Errorf(`"%s" not found`, path)
	}
	r := strings.NewReader(fs[path])
	ast, err := d2parser.Parse(path, r, nil)
	if err != nil {
		return nil, err
	}

	mfs, err := memfs.New(fs)
	if err != nil {
		return nil, err
	}

	m, _, err := d2ir.Compile(ast, &d2ir.CompileOptions{
		FS: mfs,
	})
	if err != nil {
		return nil, err
	}

	m = m.FindBoardRoot(boardPath)
	if m == nil {
		return nil, fmt.Errorf(`board "%v" not found`, boardPath)
	}
	return m, nil
}
