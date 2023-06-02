package d2graph

import (
	"strings"

	"oss.terrastruct.com/d2/d2ast"
	"oss.terrastruct.com/d2/d2parser"
	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/label"
	"oss.terrastruct.com/d2/lib/shape"
)

func (obj *Object) MoveWithDescendants(dx, dy float64) {
	obj.TopLeft.X += dx
	obj.TopLeft.Y += dy
	for _, child := range obj.ChildrenArray {
		child.MoveWithDescendants(dx, dy)
	}
}

func (obj *Object) MoveWithDescendantsTo(x, y float64) {
	dx := x - obj.TopLeft.X
	dy := y - obj.TopLeft.Y
	obj.MoveWithDescendants(dx, dy)
}

func (parent *Object) removeChild(child *Object) {
	delete(parent.Children, strings.ToLower(child.ID))
	for i := 0; i < len(parent.ChildrenArray); i++ {
		if parent.ChildrenArray[i] == child {
			parent.ChildrenArray = append(parent.ChildrenArray[:i], parent.ChildrenArray[i+1:]...)
			break
		}
	}
}

// remove obj and all descendants from graph, as a new Graph
func (g *Graph) ExtractAsNestedGraph(obj *Object) *Graph {
	descendantObjects, edges := pluckObjAndEdges(g, obj)

	tempGraph := NewGraph()
	tempGraph.Root.ChildrenArray = []*Object{obj}
	tempGraph.Root.Children[strings.ToLower(obj.ID)] = obj

	for _, descendantObj := range descendantObjects {
		descendantObj.Graph = tempGraph
	}
	tempGraph.Objects = descendantObjects
	tempGraph.Edges = edges

	obj.Parent.removeChild(obj)
	obj.Parent = tempGraph.Root

	return tempGraph
}

func pluckObjAndEdges(g *Graph, obj *Object) (descendantsObjects []*Object, edges []*Edge) {
	for i := 0; i < len(g.Edges); i++ {
		edge := g.Edges[i]
		if edge.Src == obj || edge.Dst == obj {
			edges = append(edges, edge)
			g.Edges = append(g.Edges[:i], g.Edges[i+1:]...)
			i--
		}
	}

	for i := 0; i < len(g.Objects); i++ {
		temp := g.Objects[i]
		if temp.AbsID() == obj.AbsID() {
			descendantsObjects = append(descendantsObjects, obj)
			g.Objects = append(g.Objects[:i], g.Objects[i+1:]...)
			for _, child := range obj.ChildrenArray {
				subObjects, subEdges := pluckObjAndEdges(g, child)
				descendantsObjects = append(descendantsObjects, subObjects...)
				edges = append(edges, subEdges...)
			}
			break
		}
	}

	return descendantsObjects, edges
}

func (g *Graph) InjectNestedGraph(tempGraph *Graph, parent *Object) {
	obj := tempGraph.Root.ChildrenArray[0]
	obj.MoveWithDescendantsTo(0, 0)
	obj.Parent = parent
	for _, obj := range tempGraph.Objects {
		obj.Graph = g
	}
	g.Objects = append(g.Objects, tempGraph.Objects...)
	parent.Children[strings.ToLower(obj.ID)] = obj
	parent.ChildrenArray = append(parent.ChildrenArray, obj)
	g.Edges = append(g.Edges, tempGraph.Edges...)
}

// ShiftDescendants moves Object's descendants (not including itself)
// descendants' edges are also moved by the same dx and dy (the whole route is moved if both ends are a descendant)
func (obj *Object) ShiftDescendants(dx, dy float64) {
	// also need to shift edges of descendants that are shifted
	movedEdges := make(map[*Edge]struct{})
	for _, e := range obj.Graph.Edges {
		isSrcDesc := e.Src.IsDescendantOf(obj)
		isDstDesc := e.Dst.IsDescendantOf(obj)

		if isSrcDesc && isDstDesc {
			movedEdges[e] = struct{}{}
			for _, p := range e.Route {
				p.X += dx
				p.Y += dy
			}
		}
	}

	obj.IterDescendants(func(_, curr *Object) {
		curr.TopLeft.X += dx
		curr.TopLeft.Y += dy
		for _, e := range obj.Graph.Edges {
			if _, ok := movedEdges[e]; ok {
				continue
			}
			isSrc := e.Src == curr
			isDst := e.Dst == curr

			if isSrc && isDst {
				for _, p := range e.Route {
					p.X += dx
					p.Y += dy
				}
			} else if isSrc {
				e.Route[0].X += dx
				e.Route[0].Y += dy
			} else if isDst {
				e.Route[len(e.Route)-1].X += dx
				e.Route[len(e.Route)-1].Y += dy
			}

			if isSrc || isDst {
				movedEdges[e] = struct{}{}
			}
		}
	})
}

// GetModifierElementAdjustments returns width/height adjustments to account for shapes with 3d or multiple
func (obj *Object) GetModifierElementAdjustments() (dx, dy float64) {
	if obj.Is3D() {
		if obj.Shape.Value == d2target.ShapeHexagon {
			dy = d2target.THREE_DEE_OFFSET / 2
		} else {
			dy = d2target.THREE_DEE_OFFSET
		}
		dx = d2target.THREE_DEE_OFFSET
	} else if obj.IsMultiple() {
		dy = d2target.MULTIPLE_OFFSET
		dx = d2target.MULTIPLE_OFFSET
	}
	return dx, dy
}

func (obj *Object) ToShape() shape.Shape {
	tl := obj.TopLeft
	if tl == nil {
		tl = geo.NewPoint(0, 0)
	}
	dslShape := strings.ToLower(obj.Shape.Value)
	shapeType := d2target.DSL_SHAPE_TO_SHAPE_TYPE[dslShape]
	contentBox := geo.NewBox(tl, obj.Width, obj.Height)
	return shape.NewShape(shapeType, contentBox)
}

func (obj *Object) GetLabelTopLeft() *geo.Point {
	if obj.LabelPosition == nil {
		return nil
	}

	s := obj.ToShape()
	labelPosition := label.Position(*obj.LabelPosition)

	var box *geo.Box
	if labelPosition.IsOutside() {
		box = s.GetBox()
	} else {
		box = s.GetInnerBox()
	}

	labelTL := labelPosition.GetPointOnBox(box, label.PADDING,
		float64(obj.LabelDimensions.Width),
		float64(obj.LabelDimensions.Height),
	)
	return labelTL
}

type LayoutError struct {
	Errors []d2ast.Error `json:"errs"`
}

func (le LayoutError) Empty() bool {
	return len(le.Errors) == 0
}

func (le LayoutError) Error() string {
	var sb strings.Builder
	for i, err := range le.Errors {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(err.Error())
	}
	return sb.String()
}

func (g *Graph) errorf(n d2ast.Node, f string, v ...interface{}) {
	g.err.Errors = append(g.err.Errors, d2parser.Errorf(n, f, v...).(d2ast.Error))
}
