package proximity

import (
	"context"
	"fmt"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/invariant"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
)

// AssignNears marks otherwise unconnected siblings that share an external
// neighbor so placement keeps them close together.
func AssignNears(ctx context.Context, graph *layoutgraph.Graph, root *layoutgraph.Node, abductions []*layoutgraph.EdgeAbduction) error {
	return assignNearsWithWorkLimit(ctx, graph, root, abductions, limits.MaxEngineWorkUnits)
}

func assignNearsWithWorkLimit(ctx context.Context, graph *layoutgraph.Graph, root *layoutgraph.Node, abductions []*layoutgraph.EdgeAbduction, workLimit int64) (err error) {
	guard, err := limits.NewWorkGuard(ctx, "AssignNears", workLimit)
	if err != nil {
		return err
	}
	uncles := make(map[*layoutgraph.Node]map[*layoutgraph.Node]struct{})
	for _, abduction := range abductions {
		if err := guard.Step(); err != nil {
			return err
		}
		if abduction == nil {
			return invariant.New("nil edge abduction while assigning nears")
		}
		originallyFrom := groupVessel(abduction.OriginallyFrom)
		originallyTo := groupVessel(abduction.OriginallyTo)
		var uncle, connected *layoutgraph.Node
		for _, node := range graph.Containers[root] {
			if err := guard.Step(); err != nil {
				return err
			}
			fromDescendant, err := isDescendantOf(originallyFrom, node, guard)
			if err != nil {
				return err
			}
			toDescendant, err := isDescendantOf(originallyTo, node, guard)
			if err != nil {
				return err
			}
			if fromDescendant {
				uncle, connected = abduction.CurrentTo, node
				break
			} else if toDescendant {
				uncle, connected = abduction.CurrentFrom, node
				break
			}
		}
		if uncle != nil && connected != nil {
			if uncles[uncle] == nil {
				uncles[uncle] = make(map[*layoutgraph.Node]struct{})
			}
			uncles[uncle][connected] = struct{}{}
		}
	}

	orderedUncles := make([]*layoutgraph.Node, 0, len(uncles))
	for uncle := range uncles {
		if err := guard.Step(); err != nil {
			return err
		}
		orderedUncles = append(orderedUncles, uncle)
	}
	layoutgraph.SortNodesByID(orderedUncles)

	originalNears := make(map[*layoutgraph.Node]map[*layoutgraph.Node]struct{})
	replacementNears := make(map[*layoutgraph.Node]map[*layoutgraph.Node]struct{})
	var nearReferences int64
	mutableNears := func(node *layoutgraph.Node) (map[*layoutgraph.Node]struct{}, error) {
		if replacement, ok := replacementNears[node]; ok {
			return replacement, nil
		}
		if int64(len(node.Nears)) > layoutgraph.MaxTopologyReferences-nearReferences {
			return nil, fmt.Errorf("TALA AssignNears topology references exceed limit %d", layoutgraph.MaxTopologyReferences)
		}
		nearReferences += int64(len(node.Nears))
		originalNears[node] = node.Nears
		replacement := make(map[*layoutgraph.Node]struct{}, len(node.Nears)+1)
		for near := range node.Nears {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			replacement[near] = struct{}{}
		}
		replacementNears[node] = replacement
		return replacement, nil
	}
	hasConnection := func(node, other *layoutgraph.Node) (bool, error) {
		for _, edge := range node.Edges {
			if err := guard.Step(); err != nil {
				return false, err
			}
			if edge != nil && node.Adjacent(edge) == other {
				return true, nil
			}
		}
		return false, nil
	}

	for _, uncle := range orderedUncles {
		nodes := make([]*layoutgraph.Node, 0, len(uncles[uncle]))
		for node := range uncles[uncle] {
			if err := guard.Step(); err != nil {
				return err
			}
			nodes = append(nodes, node)
		}
		layoutgraph.SortNodesByID(nodes)
		if len(nodes) == 1 {
			continue
		}
		for i, first := range nodes {
			for _, second := range nodes[i+1:] {
				if err := guard.Step(); err != nil {
					return err
				}
				if first.Hierarchy != nil || second.Hierarchy != nil {
					continue
				}
				connected, err := hasConnection(first, second)
				if err != nil {
					return err
				}
				if connected {
					continue
				}
				firstNears, err := mutableNears(first)
				if err != nil {
					return err
				}
				secondNears, err := mutableNears(second)
				if err != nil {
					return err
				}
				if _, exists := firstNears[second]; !exists {
					if nearReferences >= layoutgraph.MaxTopologyReferences {
						return fmt.Errorf("TALA AssignNears topology references exceed limit %d", layoutgraph.MaxTopologyReferences)
					}
					firstNears[second] = struct{}{}
					nearReferences++
				}
				if _, exists := secondNears[first]; !exists {
					if nearReferences >= layoutgraph.MaxTopologyReferences {
						return fmt.Errorf("TALA AssignNears topology references exceed limit %d", layoutgraph.MaxTopologyReferences)
					}
					secondNears[first] = struct{}{}
					nearReferences++
				}
			}
		}
	}
	if err := guard.Finish(); err != nil {
		return err
	}

	complete := false
	defer func() {
		if recovered := recover(); recovered != nil {
			for node, nears := range originalNears {
				node.Nears = nears
			}
			panic(recovered)
		}
		if !complete {
			for node, nears := range originalNears {
				node.Nears = nears
			}
		}
	}()
	commitOrder := make([]*layoutgraph.Node, 0, len(replacementNears))
	for node := range replacementNears {
		commitOrder = append(commitOrder, node)
	}
	layoutgraph.SortNodesByID(commitOrder)
	for _, node := range commitOrder {
		node.Nears = replacementNears[node]
		if err := guard.Finish(); err != nil {
			return err
		}
	}
	complete = true
	return nil
}

func groupVessel(node *layoutgraph.Node) *layoutgraph.Node {
	if node == nil {
		return nil
	}
	if node.Cluster != nil {
		return node.Cluster.Vessel
	}
	if node.Sequence != nil {
		return node.Sequence.Vessel
	}
	return node
}

func isDescendantOf(descendant, ancestor *layoutgraph.Node, guard *limits.WorkGuard) (bool, error) {
	for {
		if err := guard.Step(); err != nil {
			return false, err
		}
		if descendant == ancestor {
			return true, nil
		}
		if descendant == nil {
			return false, nil
		}
		switch {
		case descendant.Container != nil:
			descendant = descendant.Container
		case descendant.Cluster != nil:
			descendant = descendant.Cluster.Vessel
		case descendant.Sequence != nil:
			descendant = descendant.Sequence.Vessel
		default:
			descendant = nil
		}
	}
}
