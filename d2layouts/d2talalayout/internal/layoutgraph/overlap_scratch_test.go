package layoutgraph

import (
	"context"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/lib/geo"
)

func TestOverlapExceptionScratchDoesNotLeakAcrossCalls(t *testing.T) {
	graph := NewGraph()
	moving := NewNode(1, 10, 10)
	moving.TopLeft = &geo.Point{}
	graph.AddNodeUnchecked(moving)
	exceptions := make([]*Node, 32)
	for i := range exceptions {
		node := NewNode(EntityID(i+2), 10, 10)
		node.TopLeft = &geo.Point{}
		graph.AddNodeUnchecked(node)
		exceptions[i] = node
	}
	check := func(exceptions []*Node, limit int64) (bool, error) {
		guard, err := limits.NewWorkGuard(context.Background(), "overlap test", limit)
		if err != nil {
			t.Fatal(err)
		}
		return graph.doesOverlapWithDimensionsContext(moving, moving.TopLeft, moving.Width, moving.Height, exceptions, nil, guard)
	}
	for range 20 {
		if overlap, err := check(exceptions, limits.MaxEngineWorkUnits); err != nil || overlap {
			t.Fatalf("excepted nodes: overlap=%v, error=%v", overlap, err)
		}
		// Failure midway through populating the scratch map must also release it.
		if _, err := check(exceptions, 12); err == nil {
			t.Fatal("limited check succeeded")
		}
		// Use another pooled map, excepting only nodes absent from the graph. No
		// exception from an earlier successful or rejected call may survive.
		unrelated := make([]*Node, 9)
		for i := range unrelated {
			unrelated[i] = NewNode(EntityID(i+100), 10, 10)
		}
		if overlap, err := check(unrelated, limits.MaxEngineWorkUnits); err != nil || !overlap {
			t.Fatalf("unexcepted nodes: overlap=%v, error=%v", overlap, err)
		}
	}
}

func TestOverlapExceptionScratchReleasesGraphReferences(t *testing.T) {
	for _, count := range []int{10, maxPooledOverlapExceptions + 1} {
		scratch := &overlapExceptionScratch{nodes: make(map[*Node]struct{})}
		for i := 0; i < count; i++ {
			scratch.nodes[NewNode(EntityID(i), 10, 10)] = struct{}{}
		}
		releaseOverlapExceptions(scratch)
		if len(scratch.nodes) != 0 {
			t.Fatal("scratch retained graph references")
		}
		if count > maxPooledOverlapExceptions && scratch.nodes != nil {
			t.Fatal("oversized scratch map was retained")
		}
	}
}
