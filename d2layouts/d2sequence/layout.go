package d2sequence

import (
	"context"
	"fmt"
	"math"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/go2"
	"oss.terrastruct.com/d2/lib/label"
)

func Layout(ctx context.Context, g *d2graph.Graph) (err error) {
	pad := 50. // 2 * 25
	edgeYStep := 100.
	objectXStep := 200.
	maxObjectHeight := 0.

	var objectsInOrder []*d2graph.Object
	seen := make(map[*d2graph.Object]struct{})
	for _, edge := range g.Edges {
		if _, exists := seen[edge.Src]; !exists {
			seen[edge.Src] = struct{}{}
			objectsInOrder = append(objectsInOrder, edge.Src)
		}
		if _, exists := seen[edge.Dst]; !exists {
			seen[edge.Dst] = struct{}{}
			objectsInOrder = append(objectsInOrder, edge.Dst)
		}

		edgeYStep = math.Max(edgeYStep, float64(edge.LabelDimensions.Height)+pad)
		objectXStep = math.Max(objectXStep, float64(edge.LabelDimensions.Width)+pad)
		maxObjectHeight = math.Max(maxObjectHeight, edge.Src.Height+pad)
		maxObjectHeight = math.Max(maxObjectHeight, edge.Dst.Height+pad)
	}

	placeObjects(objectsInOrder, maxObjectHeight, objectXStep)
	// edges are placed in the order users define them
	routeEdges(g.Edges, maxObjectHeight, edgeYStep)
	addLifelineEdges(g, objectsInOrder, edgeYStep)

	return nil
}

// placeObjects places objects side by side
func placeObjects(objectsInOrder []*d2graph.Object, maxHeight, xStep float64) {
	x := 0.
	for _, obj := range objectsInOrder {
		yDiff := maxHeight - obj.Height
		obj.TopLeft = geo.NewPoint(x, yDiff/2.)
		x += obj.Width + xStep
		obj.LabelPosition = go2.Pointer(string(label.InsideMiddleCenter))
	}
}

// routeEdges routes horizontal edges from Src to Dst
func routeEdges(edgesInOrder []*d2graph.Edge, startY, yStep float64) {
	edgeY := startY + yStep // in case the first edge has a tall label
	for _, edge := range edgesInOrder {
		start := edge.Src.Center()
		start.Y = edgeY
		end := edge.Dst.Center()
		end.Y = edgeY
		edge.Route = []*geo.Point{start, end}
		edgeY += yStep

		if edge.Attributes.Label.Value != "" {
			// TODO: consider label right-to-left
			edge.LabelPosition = go2.Pointer(string(label.OutsideTopCenter))
		}
	}
}

func addLifelineEdges(g *d2graph.Graph, objectsInOrder []*d2graph.Object, yStep float64) {
	endY := g.Edges[len(g.Edges)-1].Route[0].Y + yStep
	for _, obj := range objectsInOrder {
		objBottom := obj.Center()
		objBottom.Y = obj.TopLeft.Y + obj.Height
		objLifelineEnd := obj.Center()
		objLifelineEnd.Y = endY
		g.Edges = append(g.Edges, &d2graph.Edge{
			Attributes: d2graph.Attributes{
				Style: d2graph.Style{
					StrokeDash: &d2graph.Scalar{
						Value: "10",
					},
					Stroke:      obj.Attributes.Style.Stroke,
					StrokeWidth: obj.Attributes.Style.StrokeWidth,
				},
			},
			Src:      obj,
			SrcArrow: false,
			Dst: &d2graph.Object{
				ID: obj.ID + fmt.Sprintf("-lifeline-end-%d", go2.StringToIntHash(obj.ID+"-lifeline-end")),
			},
			DstArrow: false,
			Route:    []*geo.Point{objBottom, objLifelineEnd},
		})
	}
}
