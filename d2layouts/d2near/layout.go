// d2near applies near keywords when they're constants
// Intended to be run as the last stage of layout after the diagram has already undergone layout
package d2near

import (
	"context"
	"math"
	"strings"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/label"
)

const pad = 20

// Layout finds the shapes which are assigned constant near keywords and places them.
func Layout(ctx context.Context, g *d2graph.Graph, constantNears []*d2graph.Object, descendantObjectMap map[*d2graph.Object][]*d2graph.Object, descendantEdgeMap map[*d2graph.Object][]*d2graph.Edge) error {
	if len(constantNears) == 0 {
		return nil
	}

	// Imagine the graph has two long texts, one at top center and one at top left.
	// Top left should go left enough to not collide with center.
	// So place the center ones first, then the later ones will consider them for bounding box
	for _, processCenters := range []bool{true, false} {
		for _, obj := range constantNears {
			if processCenters == strings.Contains(d2graph.Key(obj.Attributes.NearKey)[0], "-center") {
				preX, preY := obj.TopLeft.X, obj.TopLeft.Y
				obj.TopLeft = geo.NewPoint(place(obj))
				dx, dy := obj.TopLeft.X-preX, obj.TopLeft.Y-preY

				subObjects, subEdges := descendantObjectMap[obj], descendantEdgeMap[obj]
				for _, subObject := range subObjects {
					// `obj` already been replaced above by `place(obj)`
					if subObject == obj {
						continue
					}
					subObject.TopLeft.X += dx
					subObject.TopLeft.Y += dy
				}
				for _, subEdge := range subEdges {
					for _, point := range subEdge.Route {
						point.X += dx
						point.Y += dy
					}
				}

				g.Edges = append(g.Edges, subEdges...)
			}
		}
		for _, obj := range constantNears {
			if processCenters == strings.Contains(d2graph.Key(obj.Attributes.NearKey)[0], "-center") {
				// The z-index for constant nears does not matter, as it will not collide
				g.Objects = append(g.Objects, obj)
				obj.Parent.Children[obj.ID] = obj
				obj.Parent.ChildrenArray = append(obj.Parent.ChildrenArray, obj)
				attachChildren(g, obj)
			}
		}
	}

	// These shapes skipped core layout, which means they also skipped label placements
	// for _, obj := range constantNears {
	// 	if obj.HasOutsideBottomLabel() {
	// 		obj.LabelPosition = go2.Pointer(string(label.OutsideBottomCenter))
	// 	} else if obj.Attributes.Icon != nil {
	// 		obj.LabelPosition = go2.Pointer(string(label.InsideTopCenter))
	// 	} else {
	// 		obj.LabelPosition = go2.Pointer(string(label.InsideMiddleCenter))
	// 	}
	// }

	return nil
}

func attachChildren(g *d2graph.Graph, obj *d2graph.Object) {
	if obj.ChildrenArray != nil && len(obj.ChildrenArray) != 0 {
		for _, child := range obj.ChildrenArray {
			g.Objects = append(g.Objects, child)
			attachChildren(g, child)
		}
	}
}

// place returns the position of obj, taking into consideration its near value and the diagram
func place(obj *d2graph.Object) (float64, float64) {
	tl, br := boundingBox(obj.Graph)
	w := br.X - tl.X
	h := br.Y - tl.Y
	switch d2graph.Key(obj.Attributes.NearKey)[0] {
	case "top-left":
		return tl.X - obj.Width - pad, tl.Y - obj.Height - pad
	case "top-center":
		return tl.X + w/2 - obj.Width/2, tl.Y - obj.Height - pad
	case "top-right":
		return br.X + pad, tl.Y - obj.Height - pad
	case "center-left":
		return tl.X - obj.Width - pad, tl.Y + h/2 - obj.Height/2
	case "center-right":
		return br.X + pad, tl.Y + h/2 - obj.Height/2
	case "bottom-left":
		return tl.X - obj.Width - pad, br.Y + pad
	case "bottom-center":
		return br.X - w/2 - obj.Width/2, br.Y + pad
	case "bottom-right":
		return br.X + pad, br.Y + pad
	}
	return 0, 0
}

// WithoutConstantNears plucks out the graph objects which have "near" set to a constant value
// This is to be called before layout engines so they don't take part in regular positioning
func WithoutConstantNears(ctx context.Context, g *d2graph.Graph) (nears []*d2graph.Object, descendantObjectMap map[*d2graph.Object][]*d2graph.Object, descendantEdgeMap map[*d2graph.Object][]*d2graph.Edge) {
	descendantObjectMap = make(map[*d2graph.Object][]*d2graph.Object)
	descendantEdgeMap = make(map[*d2graph.Object][]*d2graph.Edge)

	for i := 0; i < len(g.Objects); i++ {
		obj := g.Objects[i]
		if obj.Attributes.NearKey == nil {
			continue
		}
		_, isKey := g.Root.HasChild(d2graph.Key(obj.Attributes.NearKey))
		if isKey {
			continue
		}
		_, isConst := d2graph.NearConstants[d2graph.Key(obj.Attributes.NearKey)[0]]
		if isConst {
			descendantObjects, edges := pluckOutNearObjectAndEdges(g, obj)
			descendantObjectMap[obj] = descendantObjects
			descendantEdgeMap[obj] = edges

			nears = append(nears, obj)
			i--
			delete(obj.Parent.Children, strings.ToLower(obj.ID))
			for i := 0; i < len(obj.Parent.ChildrenArray); i++ {
				if obj.Parent.ChildrenArray[i] == obj {
					obj.Parent.ChildrenArray = append(obj.Parent.ChildrenArray[:i], obj.Parent.ChildrenArray[i+1:]...)
					break
				}
			}
		}
	}
	return nears, descendantObjectMap, descendantEdgeMap
}

func pluckOutNearObjectAndEdges(g *d2graph.Graph, obj *d2graph.Object) (descendantsObjects []*d2graph.Object, edges []*d2graph.Edge) {
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
				subObjects, subEdges := pluckOutNearObjectAndEdges(g, child)
				descendantsObjects = append(descendantsObjects, subObjects...)
				edges = append(edges, subEdges...)
			}
			break
		}
	}

	return descendantsObjects, edges
}

// boundingBox gets the center of the graph as defined by shapes
// The bounds taking into consideration only shapes gives more of a feeling of true center
// It differs from d2target.BoundingBox which needs to include every visible thing
func boundingBox(g *d2graph.Graph) (tl, br *geo.Point) {
	if len(g.Objects) == 0 {
		return geo.NewPoint(0, 0), geo.NewPoint(0, 0)
	}
	x1 := math.Inf(1)
	y1 := math.Inf(1)
	x2 := math.Inf(-1)
	y2 := math.Inf(-1)

	for _, obj := range g.Objects {
		if obj.Attributes.NearKey != nil {
			// Top left should not be MORE top than top-center
			// But it should go more left if top-center label extends beyond bounds of diagram
			switch d2graph.Key(obj.Attributes.NearKey)[0] {
			case "top-center", "bottom-center":
				x1 = math.Min(x1, obj.TopLeft.X)
				x2 = math.Max(x2, obj.TopLeft.X+obj.Width)
			case "center-left", "center-right":
				y1 = math.Min(y1, obj.TopLeft.Y)
				y2 = math.Max(y2, obj.TopLeft.Y+obj.Height)
			}
		} else {
			x1 = math.Min(x1, obj.TopLeft.X)
			y1 = math.Min(y1, obj.TopLeft.Y)
			x2 = math.Max(x2, obj.TopLeft.X+obj.Width)
			y2 = math.Max(y2, obj.TopLeft.Y+obj.Height)
			if obj.Attributes.Label.Value != "" && obj.LabelPosition != nil {
				labelPosition := label.Position(*obj.LabelPosition)
				if labelPosition.IsOutside() {
					labelTL := labelPosition.GetPointOnBox(obj.Box, label.PADDING, float64(*obj.LabelWidth), float64(*obj.LabelHeight))
					x1 = math.Min(x1, labelTL.X)
					y1 = math.Min(y1, labelTL.Y)
					x2 = math.Max(x2, labelTL.X+float64(*obj.LabelWidth))
					y2 = math.Max(y2, labelTL.Y+float64(*obj.LabelHeight))
				}
			}
		}
	}

	if math.IsInf(x1, 1) && math.IsInf(x2, -1) {
		x1 = 0
		x2 = 0
	}
	if math.IsInf(y1, 1) && math.IsInf(y2, -1) {
		y1 = 0
		y2 = 0
	}

	return geo.NewPoint(x1, y1), geo.NewPoint(x2, y2)
}
