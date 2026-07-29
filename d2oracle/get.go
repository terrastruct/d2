package d2oracle

import (
	"fmt"

	"oss.terrastruct.com/d2/d2ast"
	"oss.terrastruct.com/d2/d2format"
	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2parser"
)

func GetBoardGraph(g *d2graph.Graph, boardPath []string) *d2graph.Graph {
	if len(boardPath) == 0 {
		return g
	}
	for i, b := range g.Layers {
		if b.Name == boardPath[0] {
			g2 := GetBoardGraph(g.Layers[i], boardPath[1:])
			if g2 != nil {
				g2.FS = g.FS
			}
			return g2
		}
	}
	for i, b := range g.Scenarios {
		if b.Name == boardPath[0] {
			g2 := GetBoardGraph(g.Scenarios[i], boardPath[1:])
			if g2 != nil {
				g2.FS = g.FS
			}
			return g2
		}
	}
	for i, b := range g.Steps {
		if b.Name == boardPath[0] {
			g2 := GetBoardGraph(g.Steps[i], boardPath[1:])
			if g2 != nil {
				g2.FS = g.FS
			}
			return g2
		}
	}
	return nil
}

func ReplaceBoardNode(ast, ast2 *d2ast.Map, boardPath []string) bool {
	if len(boardPath) == 0 {
		return false
	}

	return replaceBoardNodeInMap(ast, ast2, boardPath, "layers") ||
		replaceBoardNodeInMap(ast, ast2, boardPath, "scenarios") ||
		replaceBoardNodeInMap(ast, ast2, boardPath, "steps")
}

func replaceBoardNodeInMap(ast, ast2 *d2ast.Map, boardPath []string, boardType string) bool {
	var matches []*d2ast.Map

	for _, n := range ast.Nodes {
		if n.MapKey != nil && n.MapKey.Key != nil &&
			n.MapKey.Key.Path[0].Unbox().ScalarString() == boardType &&
			n.MapKey.Value.Map != nil {
			matches = append(matches, n.MapKey.Value.Map)
		}
	}

	for _, boardMap := range matches {
		for _, n := range boardMap.Nodes {
			if n.MapKey != nil && n.MapKey.Key != nil &&
				n.MapKey.Key.Path[0].Unbox().ScalarString() == boardPath[0] &&
				n.MapKey.Value.Map != nil {
				if len(boardPath) > 1 {
					if ReplaceBoardNode(n.MapKey.Value.Map, ast2, boardPath[1:]) {
						return true
					}
				} else {
					n.MapKey.Value.Map.Nodes = ast2.Nodes
					return true
				}
			}
		}
	}

	return false
}

func GetChildrenIDs(g *d2graph.Graph, boardPath []string, absID string) (ids []string, _ error) {
	g = GetBoardGraph(g, boardPath)
	if g == nil {
		return nil, fmt.Errorf("board at path %v not found", boardPath)
	}

	mk, err := d2parser.ParseMapKey(absID)
	if err != nil {
		return nil, err
	}
	obj, ok := g.Root.HasChild(d2graph.Key(mk.Key))
	if !ok {
		return nil, fmt.Errorf("%v not found", absID)
	}

	for _, ch := range obj.ChildrenArray {
		ids = append(ids, ch.AbsID())
	}

	return ids, nil
}

func GetParentID(g *d2graph.Graph, boardPath []string, absID string) (string, error) {
	g = GetBoardGraph(g, boardPath)
	if g == nil {
		return "", fmt.Errorf("board at path %v not found", boardPath)
	}

	mk, err := d2parser.ParseMapKey(absID)
	if err != nil {
		return "", err
	}
	obj, ok := g.Root.HasChild(d2graph.Key(mk.Key))
	if !ok {
		return "", fmt.Errorf("%v not found", absID)
	}

	return obj.Parent.AbsID(), nil
}

func IsImportedObj(ast *d2ast.Map, obj *d2graph.Object) bool {
	for _, ref := range obj.References {
		if ref.Key.HasGlob() {
			return true
		}
		if ref.Key.Range.Path != ast.Range.Path {
			return true
		}
	}

	return false
}

// Glob creations count as imported for now
// TODO Probably rename later
func IsImportedEdge(ast *d2ast.Map, edge *d2graph.Edge) bool {
	for _, ref := range edge.References {
		// If edge index, the glob is just setting something, not responsible for creating the edge
		if (ref.Edge.Src.HasGlob() || ref.Edge.Dst.HasGlob()) && ref.MapKey.EdgeIndex == nil {
			return true
		}
		if ref.Edge.Range.Path != ast.Range.Path {
			return true
		}
	}

	return false
}

func GetObj(g *d2graph.Graph, boardPath []string, absID string) *d2graph.Object {
	g = GetBoardGraph(g, boardPath)
	if g == nil {
		return nil
	}

	mk, _ := d2parser.ParseMapKey(absID)
	obj, _ := g.Root.HasChild(d2graph.Key(mk.Key))
	return obj
}

func GetEdge(g *d2graph.Graph, boardPath []string, absID string) *d2graph.Edge {
	g = GetBoardGraph(g, boardPath)
	if g == nil {
		return nil
	}

	for _, e := range g.Edges {
		if e.AbsID() == absID {
			return e
		}
	}
	return nil
}

func GetObjOrder(g *d2graph.Graph, boardPath []string) ([]string, error) {
	g = GetBoardGraph(g, boardPath)
	if g == nil {
		return nil, fmt.Errorf("board at path %v not found", boardPath)
	}

	var order []string

	queue := []*d2graph.Object{g.Root}
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		if curr != g.Root {
			order = append(order, curr.AbsID())
		}
		for _, ch := range curr.ChildrenArray {
			queue = append(queue, ch)
		}
	}

	return order, nil
}

func IsLabelKeyID(key, label string) bool {
	mk, err := d2parser.ParseMapKey(key)
	if err != nil {
		return false
	}
	if len(mk.Edges) > 0 {
		return false
	}
	if mk.Key == nil {
		return false
	}

	return mk.Key.Path[len(mk.Key.Path)-1].Unbox().ScalarString() == label
}

func GetID(key string) string {
	mk, err := d2parser.ParseMapKey(key)
	if err != nil {
		return ""
	}
	if len(mk.Edges) > 0 {
		return ""
	}
	if mk.Key == nil {
		return ""
	}

	return d2format.Format(d2ast.RawString(mk.Key.Path[len(mk.Key.Path)-1].Unbox().ScalarString(), true))
}

func GetWriteableRefs(obj *d2graph.Object, writeableAST *d2ast.Map) (out []d2graph.Reference) {
	for i, ref := range obj.References {
		if ref.ScopeAST == writeableAST && ref.Key.Range.Path == writeableAST.Range.Path && len(ref.MapKey.Edges) == 0 {
			out = append(out, obj.References[i])
		}
	}
	return
}

func GetWriteableEdgeRefs(edge *d2graph.Edge, writeableAST *d2ast.Map) (out []d2graph.EdgeReference) {
	for i, ref := range edge.References {
		if ref.ScopeAST == writeableAST {
			out = append(out, edge.References[i])
		}
	}
	return
}
