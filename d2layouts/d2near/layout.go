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

type set map[string]struct{}

var HorizontalCenterNears = set{
	"center-left":  {},
	"center-right": {},
}
var VerticalCenterNears = set{
	"top-center":    {},
	"bottom-center": {},
}
var NonCenterNears = set{
	"top-left":     {},
	"top-right":    {},
	"bottom-left":  {},
	"bottom-right": {},
}

// Layout finds the shapes which are assigned constant near keywords and places them.
func Layout(ctx context.Context, g *d2graph.Graph, constantNearGraphs []*d2graph.Graph) error {
	if len(constantNearGraphs) == 0 {
		return nil
	}

	for _, tempGraph := range constantNearGraphs {
		tempGraph.Root.ChildrenArray[0].Parent = g.Root
		for _, obj := range tempGraph.Objects {
			obj.Graph = g
		}
	}

	// Imagine the graph has two long texts, one at top center and one at top left.
	// Top left should go left enough to not collide with center.
	// So place the center ones first, then the later ones will consider them for bounding box
	for _, currentSet := range []set{VerticalCenterNears, HorizontalCenterNears, NonCenterNears} {
		for _, tempGraph := range constantNearGraphs {
			obj := tempGraph.Root.ChildrenArray[0]
			_, in := currentSet[d2graph.Key(obj.NearKey)[0]]
			if in {
				prevX, prevY := obj.TopLeft.X, obj.TopLeft.Y
				obj.TopLeft = geo.NewPoint(place(obj))
				dx, dy := obj.TopLeft.X-prevX, obj.TopLeft.Y-prevY

				for _, subObject := range tempGraph.Objects {
					// `obj` already been replaced above by `place(obj)`
					if subObject == obj {
						continue
					}
					subObject.TopLeft.X += dx
					subObject.TopLeft.Y += dy
				}
				for _, subEdge := range tempGraph.Edges {
					for _, point := range subEdge.Route {
						point.X += dx
						point.Y += dy
					}
				}
			}
		}
		for _, tempGraph := range constantNearGraphs {
			obj := tempGraph.Root.ChildrenArray[0]
			_, in := currentSet[d2graph.Key(obj.NearKey)[0]]
			if in {
				// The z-index for constant nears does not matter, as it will not collide
				g.Objects = append(g.Objects, tempGraph.Objects...)
				if obj.Parent.Children == nil {
					obj.Parent.Children = make(map[string]*d2graph.Object)
				}
				obj.Parent.Children[strings.ToLower(obj.ID)] = obj
				obj.Parent.ChildrenArray = append(obj.Parent.ChildrenArray, obj)
				g.Edges = append(g.Edges, tempGraph.Edges...)
			}
		}
	}

	return nil
}

// place returns the position of obj, taking into consideration its near value and the diagram
func place(obj *d2graph.Object) (float64, float64) {
	tl, br := boundingBox(obj.Graph)
	w := br.X - tl.X
	h := br.Y - tl.Y

	nearKeyStr := d2graph.Key(obj.NearKey)[0]
	var x, y float64
	switch nearKeyStr {
	case "top-left":
		x, y = tl.X-obj.Width-pad, tl.Y-obj.Height-pad
	case "top-center":
		x, y = tl.X+w/2-obj.Width/2, tl.Y-obj.Height-pad
	case "top-right":
		x, y = br.X+pad, tl.Y-obj.Height-pad
	case "center-left":
		x, y = tl.X-obj.Width-pad, tl.Y+h/2-obj.Height/2
	case "center-right":
		x, y = br.X+pad, tl.Y+h/2-obj.Height/2
	case "bottom-left":
		x, y = tl.X-obj.Width-pad, br.Y+pad
	case "bottom-center":
		x, y = br.X-w/2-obj.Width/2, br.Y+pad
	case "bottom-right":
		x, y = br.X+pad, br.Y+pad
	}

	if obj.LabelPosition != nil && !strings.Contains(*obj.LabelPosition, "INSIDE") {
		if strings.Contains(*obj.LabelPosition, "_TOP_") {
			// label is on the top, and container is placed on the bottom
			if strings.Contains(nearKeyStr, "bottom") {
				y += float64(obj.LabelDimensions.Height)
			}
		} else if strings.Contains(*obj.LabelPosition, "_LEFT_") {
			// label is on the left, and container is placed on the right
			if strings.Contains(nearKeyStr, "right") {
				x += float64(obj.LabelDimensions.Width)
			}
		} else if strings.Contains(*obj.LabelPosition, "_RIGHT_") {
			// label is on the right, and container is placed on the left
			if strings.Contains(nearKeyStr, "left") {
				x -= float64(obj.LabelDimensions.Width)
			}
		} else if strings.Contains(*obj.LabelPosition, "_BOTTOM_") {
			// label is on the bottom, and container is placed on the top
			if strings.Contains(nearKeyStr, "top") {
				y -= float64(obj.LabelDimensions.Height)
			}
		}
	}

	return x, y
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
		if obj.NearKey != nil {
			// Top left should not be MORE top than top-center
			// But it should go more left if top-center label extends beyond bounds of diagram
			switch d2graph.Key(obj.NearKey)[0] {
			case "top-center", "bottom-center":
				x1 = math.Min(x1, obj.TopLeft.X)
				x2 = math.Max(x2, obj.TopLeft.X+obj.Width)
			case "center-left", "center-right":
				y1 = math.Min(y1, obj.TopLeft.Y)
				y2 = math.Max(y2, obj.TopLeft.Y+obj.Height)
			}
		} else {
			if obj.OuterNearContainer() != nil {
				continue
			}
			x1 = math.Min(x1, obj.TopLeft.X)
			y1 = math.Min(y1, obj.TopLeft.Y)
			x2 = math.Max(x2, obj.TopLeft.X+obj.Width)
			y2 = math.Max(y2, obj.TopLeft.Y+obj.Height)
			if obj.Label.Value != "" && obj.LabelPosition != nil {
				labelPosition := label.FromString(*obj.LabelPosition)
				if labelPosition.IsOutside() {
					labelTL := labelPosition.GetPointOnBox(obj.Box, label.PADDING, float64(obj.LabelDimensions.Width), float64(obj.LabelDimensions.Height))
					x1 = math.Min(x1, labelTL.X)
					y1 = math.Min(y1, labelTL.Y)
					x2 = math.Max(x2, labelTL.X+float64(obj.LabelDimensions.Width))
					y2 = math.Max(y2, labelTL.Y+float64(obj.LabelDimensions.Height))
				}
			}
		}
	}

	for _, edge := range g.Edges {
		if edge.Src.OuterNearContainer() != nil || edge.Dst.OuterNearContainer() != nil {
			continue
		}
		if edge.Route != nil {
			for _, point := range edge.Route {
				x1 = math.Min(x1, point.X)
				y1 = math.Min(y1, point.Y)
				x2 = math.Max(x2, point.X)
				y2 = math.Max(y2, point.Y)
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
