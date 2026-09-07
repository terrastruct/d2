package layoutgraph

import (
	"context"
	"errors"
	"testing"
)

var errReachabilityTestWorkLimit = errors.New("reachability test work limit")

type reachabilityTestWork struct {
	ctx    context.Context
	used   uint64
	limit  uint64
	cancel context.CancelFunc
}

func (work *reachabilityTestWork) Step() error {
	work.used++
	if work.used > work.limit {
		if work.cancel != nil {
			work.cancel()
			return work.ctx.Err()
		}
		return errReachabilityTestWorkLimit
	}
	return work.ctx.Err()
}

func (work *reachabilityTestWork) Finish() error { return work.ctx.Err() }

func newReachabilityTestWork(ctx context.Context, limit uint64) workStepper {
	return &reachabilityTestWork{ctx: ctx, limit: limit}
}

func TestReachabilityAncestryMatchesLegacyPrecedence(t *testing.T) {
	container := NewNode(1, 10, 10)
	other := NewNode(2, 10, 10)
	clusterVessel := NewNode(3, 10, 10)
	sequenceVessel := NewNode(4, 10, 10)
	containerChild := NewNode(5, 10, 10)
	containerChild.Container = container
	clusterChild := NewNode(6, 10, 10)
	clusterChild.Cluster = &Cluster{Vessel: clusterVessel}
	sequenceChild := NewNode(7, 10, 10)
	sequenceChild.Sequence = &Sequence{Vessel: sequenceVessel}
	containerPrecedence := NewNode(8, 10, 10)
	containerPrecedence.Container = container
	containerPrecedence.Cluster = &Cluster{Vessel: clusterVessel}

	tests := []struct {
		descendant *Node
		ancestor   *Node
	}{
		{descendant: nil, ancestor: nil},
		{descendant: nil, ancestor: container},
		{descendant: container, ancestor: container},
		{descendant: container, ancestor: nil},
		{descendant: container, ancestor: other},
		{descendant: containerChild, ancestor: container},
		{descendant: clusterChild, ancestor: clusterVessel},
		{descendant: sequenceChild, ancestor: sequenceVessel},
		{descendant: containerPrecedence, ancestor: container},
		{descendant: containerPrecedence, ancestor: clusterVessel},
	}
	for index, test := range tests {
		got, err := reachabilityIsDescendantOf(
			test.descendant,
			test.ancestor,
			newReachabilityTestWork(context.Background(), 10_000),
		)
		if err != nil {
			t.Fatalf("case %d: %v", index, err)
		}
		want := test.descendant.isDescendantOf(test.ancestor)
		if got != want {
			t.Fatalf("case %d: guarded ancestry = %v, want legacy %v", index, got, want)
		}
	}
}

func deepReachabilityChain(count int) (*Graph, *Node, *Node) {
	g := NewGraph()
	var root, previous *Node
	for index := 0; index < count; index++ {
		node := NewNode(EntityID(index+1), 10, 10)
		node.Container = previous
		g.AddNodeUnchecked(node)
		if root == nil {
			root = node
		}
		previous = node
	}
	return g, root, previous
}

func TestReachabilityAncestryConsumesAggregatePerParentHop(t *testing.T) {
	_, root, deepest := deepReachabilityChain(200)
	_, err := reachabilityIsDescendantOf(deepest, root, newReachabilityTestWork(context.Background(), 32))
	if !errors.Is(err, errReachabilityTestWorkLimit) {
		t.Fatalf("guarded ancestry error = %v, want shared route-stage work limit", err)
	}
}

func TestReachabilityAncestryCancellationIsObservedInsideParentWalk(t *testing.T) {
	g, root, deepest := deepReachabilityChain(3)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	work := &reachabilityTestWork{
		ctx:    ctx,
		limit:  1, // Visit the descendant, then cancel on its first parent.
		cancel: cancel,
	}
	_, err := reachabilityIsDescendantOf(deepest, root, work)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("reachability error = %v, want cancellation inside ancestry walk", err)
	}
	var parent *Node
	for _, node := range g.Nodes {
		if node.Graph != g {
			t.Fatalf("canceled reachability changed node %d owner", node.ID)
		}
		if node.Container != parent {
			t.Fatalf("canceled reachability changed node %d parent", node.ID)
		}
		parent = node
	}
}
