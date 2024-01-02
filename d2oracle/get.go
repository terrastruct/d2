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

	findMap := func(root *d2ast.Map, name string) *d2ast.Map {
		for _, n := range root.Nodes {
			if n.MapKey != nil && n.MapKey.Key != nil && n.MapKey.Key.Path[0].Unbox().ScalarString() == name {
				return n.MapKey.Value.Map
			}
		}
		return nil
	}

	layersMap := findMap(ast, "layers")
	scenariosMap := findMap(ast, "scenarios")
	stepsMap := findMap(ast, "steps")

	if layersMap != nil {
		m := findMap(layersMap, boardPath[0])
		if m != nil {
			if len(boardPath) > 1 {
				return ReplaceBoardNode(m, ast2, boardPath[1:])
			} else {
				m.Nodes = ast2.Nodes
				return true
			}
		}
	}

	if scenariosMap != nil {
		m := findMap(scenariosMap, boardPath[0])
		if m != nil {
			if len(boardPath) > 1 {
				return ReplaceBoardNode(m, ast2, boardPath[1:])
			} else {
				m.Nodes = ast2.Nodes
				return true
			}
		}
	}

	if stepsMap != nil {
		m := findMap(stepsMap, boardPath[0])
		if m != nil {
			if len(boardPath) > 1 {
				return ReplaceBoardNode(m, ast2, boardPath[1:])
			} else {
				m.Nodes = ast2.Nodes
				return true
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

func IsImported(g *d2graph.Graph, obj *d2graph.Object) bool {
	for _, ref := range obj.References {
		if ref.MapKey.Range.Path == g.AST.Range.Path {
			return false
		}
	}

	return true
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
