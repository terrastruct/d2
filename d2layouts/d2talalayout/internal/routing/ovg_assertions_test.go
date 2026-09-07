package routing

import "fmt"

func (g *OVG) Equals(other *OVG) bool {
	if len(g.Nodes) != len(other.Nodes) || len(g.Edges) != len(other.Edges) {
		return false
	}

	points := make(map[string]struct{})
	for _, node := range g.Nodes {
		points[node.FormattedCoordinates()] = struct{}{}
	}
	for _, node := range other.Nodes {
		delete(points, node.FormattedCoordinates())
	}
	if len(points) > 0 {
		return false
	}

	edges := make(map[string]struct{})
	for _, edge := range g.Edges {
		edges[fmt.Sprintf("%s:%s", edge.From.FormattedCoordinates(), edge.To.FormattedCoordinates())] = struct{}{}
	}
	for _, edge := range other.Edges {
		// Edge direction is irrelevant to OVG equality.
		delete(edges, fmt.Sprintf("%s:%s", edge.From.FormattedCoordinates(), edge.To.FormattedCoordinates()))
		delete(edges, fmt.Sprintf("%s:%s", edge.To.FormattedCoordinates(), edge.From.FormattedCoordinates()))
	}
	return len(edges) == 0
}

func (e *OVGEdge) equals(other *OVGEdge) bool {
	return (e.From.Equals(other.From.Point) && e.To.Equals(other.To.Point)) ||
		(e.From.Equals(other.To.Point) && e.To.Equals(other.From.Point))
}
