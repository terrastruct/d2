package layoutgraph

// HubOrder returns hub sentinels in stable ID order.
func (g *Graph) HubOrder() []*Node {
	order := make([]*Node, 0, len(g.Hubs))
	for node := range g.Hubs {
		order = append(order, node)
	}
	sortNodesByID(order)
	return order
}
