package routing

import "github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"

// makeLoopRoute makes a `Route` based on the edge points
func makeLoopRoute(e *layoutgraph.Edge, ovg *OVG) *Route {
	var nodes []*OVGNode
	nodes = append(nodes, ovg.Centers[e.From])
	for _, p := range e.Points {
		nodes = append(nodes, NewOVGNode(p))
	}
	nodes = append(nodes, ovg.Centers[e.To])
	return &Route{
		GEdge:    e,
		FromPort: *e.Points[0],
		ToPort:   *e.Points[len(e.Points)-1],
		OVGNodes: nodes,
	}
}
