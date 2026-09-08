package hierarchy

import (
	"cmp"
	"errors"
	"fmt"
	"slices"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
)

func newRankProblem(g *layoutgraph.Graph, guard *limits.OptimizationWorkGuard) (*rankProblem, error) {
	if g == nil {
		return nil, errors.New("graph is required")
	}
	if len(g.Nodes) > limits.MaxEngineNodes {
		return nil, fmt.Errorf("node count exceeds limit %d", limits.MaxEngineNodes)
	}
	if len(g.Edges) > limits.MaxEngineEdges {
		return nil, fmt.Errorf("edge count exceeds limit %d", limits.MaxEngineEdges)
	}

	nodes := slices.Clone(g.Nodes)
	if err := guard.AddSort(len(nodes)); err != nil {
		return nil, err
	}
	slices.SortFunc(nodes, func(a, b *layoutgraph.Node) int {
		if a == nil {
			if b == nil {
				return 0
			}
			return -1
		}
		if b == nil {
			return 1
		}
		return cmp.Compare(a.ID, b.ID)
	})

	index := make(map[*layoutgraph.Node]int, len(nodes))
	for i, node := range nodes {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		if node == nil {
			return nil, errors.New("nil node")
		}
		if i > 0 && nodes[i-1].ID == node.ID {
			return nil, fmt.Errorf("duplicate node ID %d", node.ID)
		}
		if _, exists := index[node]; exists {
			return nil, fmt.Errorf("duplicate node %s", node.DebugID())
		}
		index[node] = i
	}

	type indexedEdge struct {
		edge     *layoutgraph.Edge
		from, to int
	}
	edges := make([]indexedEdge, 0, len(g.Edges))
	seenEdges := make(map[*layoutgraph.Edge]struct{}, len(g.Edges))
	seenEdgeIDs := make(map[layoutgraph.EntityID]struct{}, len(g.Edges))
	type endpoints struct{ from, to int }
	seenEndpoints := make(map[endpoints]struct{}, len(g.Edges))
	for _, edge := range g.Edges {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		if edge == nil || edge.From == nil || edge.To == nil {
			return nil, errors.New("edge has a nil endpoint")
		}
		from, fromExists := index[edge.From]
		to, toExists := index[edge.To]
		if !fromExists || !toExists {
			return nil, fmt.Errorf("edge %d references a node outside the graph", edge.ID)
		}
		if from == to {
			return nil, fmt.Errorf("self edge on %s", edge.From.DebugID())
		}
		if edge.HasSourceArrow() || !edge.HasTargetArrow() {
			return nil, errors.New("canonical source-to-target directed edges required")
		}
		weight := edge.HierarchyRankWeight()
		if weight <= 0 || weight > limits.MaxEngineEdges {
			return nil, fmt.Errorf("edge %d has invalid rank weight %d", edge.ID, weight)
		}
		if _, exists := seenEdges[edge]; exists {
			return nil, fmt.Errorf("duplicate edge reference %d", edge.ID)
		}
		if _, exists := seenEdgeIDs[edge.ID]; exists {
			return nil, fmt.Errorf("duplicate edge ID %d", edge.ID)
		}
		pair := endpoints{from: from, to: to}
		if _, exists := seenEndpoints[pair]; exists {
			return nil, fmt.Errorf("duplicate directed edge %s -> %s", edge.From.DebugID(), edge.To.DebugID())
		}
		seenEdges[edge] = struct{}{}
		seenEdgeIDs[edge.ID] = struct{}{}
		seenEndpoints[pair] = struct{}{}
		edges = append(edges, indexedEdge{edge: edge, from: from, to: to})
	}

	if err := guard.AddSort(len(edges)); err != nil {
		return nil, err
	}
	slices.SortFunc(edges, func(a, b indexedEdge) int {
		if order := cmp.Compare(a.from, b.from); order != 0 {
			return order
		}
		if order := cmp.Compare(a.to, b.to); order != 0 {
			return order
		}
		return cmp.Compare(a.edge.ID, b.edge.ID)
	})

	type endpointCounts struct{ from, to int }
	incidentCounts := make(map[*layoutgraph.Edge]endpointCounts, len(edges))
	incidentReferences := 0
	for _, node := range nodes {
		for _, edge := range node.Edges {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			incidentReferences++
			if incidentReferences > limits.MaxEngineEdges*2 {
				return nil, fmt.Errorf("incident edge references exceed limit %d", limits.MaxEngineEdges*2)
			}
			if edge == nil || (edge.From != node && edge.To != node) {
				return nil, fmt.Errorf("malformed incident edge on %s", node.DebugID())
			}
			if _, exists := seenEdges[edge]; !exists {
				return nil, fmt.Errorf("node %s references an edge outside the graph", node.DebugID())
			}
			counts := incidentCounts[edge]
			if edge.From == node {
				counts.from++
			} else {
				counts.to++
			}
			incidentCounts[edge] = counts
		}
	}
	for _, edge := range edges {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		counts := incidentCounts[edge.edge]
		if counts != (endpointCounts{from: 1, to: 1}) {
			return nil, fmt.Errorf("edge %d must appear once on each endpoint; got source=%d target=%d", edge.edge.ID, counts.from, counts.to)
		}
	}

	if len(nodes) > 1 {
		seen := make([]bool, len(nodes))
		seen[0] = true
		queue := []int{0}
		for head := 0; head < len(queue); head++ {
			u := queue[head]
			for _, edge := range nodes[u].Edges {
				if err := guard.Step(); err != nil {
					return nil, err
				}
				v := index[nodes[u].Adjacent(edge)]
				if !seen[v] {
					seen[v] = true
					queue = append(queue, v)
				}
			}
		}
		if len(queue) != len(nodes) {
			return nil, errors.New("input graph is disconnected")
		}
	}

	problemEdges := make([]rankEdge, len(edges))
	outgoingStart := make([]int, len(nodes)+1)
	for i, edge := range edges {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		problemEdges[i] = rankEdge{from: edge.from, to: edge.to, weight: int64(edge.edge.HierarchyRankWeight()), id: edge.edge.ID}
		outgoingStart[edge.from+1]++
	}
	for node := range nodes {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		outgoingStart[node+1] += outgoingStart[node]
	}
	pivotOrder := make([]int, len(problemEdges))
	for edgeIndex := range pivotOrder {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		pivotOrder[edgeIndex] = edgeIndex
	}
	if err := guard.AddSort(len(pivotOrder)); err != nil {
		return nil, err
	}
	slices.SortFunc(pivotOrder, func(a, b int) int {
		return cmp.Compare(problemEdges[a].id, problemEdges[b].id)
	})
	return &rankProblem{nodes: nodes, edges: problemEdges, outgoingStart: outgoingStart, pivotOrder: pivotOrder}, nil
}

func (problem *rankProblem) longestPathLevels(guard *limits.OptimizationWorkGuard) ([]int64, error) {
	indegree := make([]int, len(problem.nodes))
	for _, edge := range problem.edges {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		indegree[edge.to]++
	}

	ready := make(nodeMinHeap, 0, len(problem.nodes))
	for node, degree := range indegree {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		if degree == 0 {
			ready.push(node)
			if err := guard.Step(); err != nil {
				return nil, err
			}
		}
	}

	levels := make([]int64, len(problem.nodes))
	visited := 0
	for len(ready) > 0 {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		u := ready.pop()
		visited++
		for edgeIndex := problem.outgoingStart[u]; edgeIndex < problem.outgoingStart[u+1]; edgeIndex++ {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			edge := problem.edges[edgeIndex]
			candidate, ok := checkedAddInt64(levels[u], rankMinSpan)
			if !ok {
				return nil, errors.New("initial level overflow")
			}
			levels[edge.to] = max(levels[edge.to], candidate)
			indegree[edge.to]--
			if indegree[edge.to] == 0 {
				ready.push(edge.to)
				if err := guard.Step(); err != nil {
					return nil, err
				}
			}
		}
	}
	if visited != len(problem.nodes) {
		return nil, errors.New("input graph contains a directed cycle")
	}
	return levels, nil
}
