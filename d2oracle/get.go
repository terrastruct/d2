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
	switch boardPath[0] {
	case "root":
		if g.Parent == nil {
			return GetBoardGraph(g, boardPath[1:])
		}
		return nil
	case "layers":
		if len(boardPath) < 2 {
			return nil
		}
		for i, b := range g.Layers {
			if b.Name == boardPath[1] {
				return GetBoardGraph(g.Layers[i], boardPath[2:])
			}
		}
	case "scenarios":
		if len(boardPath) < 2 {
			return nil
		}
		for i, b := range g.Scenarios {
			if b.Name == boardPath[1] {
				return GetBoardGraph(g.Scenarios[i], boardPath[2:])
			}
		}
	case "steps":
		if len(boardPath) < 2 {
			return nil
		}
		for i, b := range g.Steps {
			if b.Name == boardPath[1] {
				return GetBoardGraph(g.Steps[i], boardPath[2:])
			}
		}
	}

	return nil
}

func ReplaceBoardNode(ast, ast2 *d2ast.Map, boardPath []string) bool {
	if len(boardPath) == 0 {
		return false
	}
	switch boardPath[0] {
	case "root":
		return ReplaceBoardNode(ast, ast2, boardPath[1:])
	case "layers":
		if len(boardPath) < 2 {
			return false
		}
		for _, n := range ast.Nodes {
			if n.MapKey != nil && n.MapKey.Key != nil && n.MapKey.Key.Path[0].Unbox().ScalarString() == "layers" {
				return ReplaceBoardNode(n.MapKey.Value.Map, ast2, boardPath[1:])
			}
		}
	case "scenarios":
		if len(boardPath) < 2 {
			return false
		}
		for _, n := range ast.Nodes {
			if n.MapKey != nil && n.MapKey.Key != nil && n.MapKey.Key.Path[0].Unbox().ScalarString() == "scenarios" {
				return ReplaceBoardNode(n.MapKey.Value.Map, ast2, boardPath[1:])
			}
		}
	case "steps":
		if len(boardPath) < 2 {
			return false
		}
		for _, n := range ast.Nodes {
			if n.MapKey != nil && n.MapKey.Key != nil && n.MapKey.Key.Path[0].Unbox().ScalarString() == "steps" {
				return ReplaceBoardNode(n.MapKey.Value.Map, ast2, boardPath[1:])
			}
		}
	default:
		for _, n := range ast.Nodes {
			if n.MapKey != nil && n.MapKey.Key != nil && n.MapKey.Key.Path[0].Unbox().ScalarString() == boardPath[0] {
				if len(boardPath) == 1 {
					n.MapKey.Value.Map.Nodes = ast2.Nodes
					return true
				}
				return ReplaceBoardNode(n.MapKey.Value.Map, ast2, boardPath[1:])
			}
		}
	}

	return false
}

func GetChildrenIDs(g *d2graph.Graph, absID string) (ids []string, _ error) {
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

func GetParentID(g *d2graph.Graph, absID string) (string, error) {
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

func GetObj(g *d2graph.Graph, absID string) *d2graph.Object {
	mk, _ := d2parser.ParseMapKey(absID)
	obj, _ := g.Root.HasChild(d2graph.Key(mk.Key))
	return obj
}

func GetEdge(g *d2graph.Graph, absID string) *d2graph.Edge {
	for _, e := range g.Edges {
		if e.AbsID() == absID {
			return e
		}
	}
	return nil
}

func GetObjOrder(g *d2graph.Graph) []string {
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

	return order
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
