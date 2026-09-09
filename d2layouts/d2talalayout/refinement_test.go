package d2talalayout

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

func TestOptionalSeedRefinementKeepsSelectedPlacementDespiteHigherScore(t *testing.T) {
	g := layoutgraph.NewGraph()
	from := g.AddNode(layoutgraph.NewNode(1, 40, 40))
	to := g.AddNode(layoutgraph.NewNode(2, 40, 40))
	from.TopLeft, to.TopLeft = geo.NewPoint(0, 0), geo.NewPoint(200, 0)
	edge := g.Connect(from, to)
	edge.Points = []*geo.Point{geo.NewPoint(40, 20), geo.NewPoint(200, 20)}
	incumbent := seedResult{graph: g, score: layoutScore{penalty: 10, area: 9600}}
	routed, err := layoutgraph.Clone(t.Context(), g)
	if err != nil {
		t.Fatal(err)
	}
	// Placement is already selected. A longer retained bus may score worse
	// while preserving the routing structure the refinement is meant to keep.
	routed.Edges[0].Points = []*geo.Point{geo.NewPoint(40, 20), geo.NewPoint(80, 20), geo.NewPoint(80, 80), geo.NewPoint(160, 80), geo.NewPoint(160, 20), geo.NewPoint(200, 20)}
	refinement := seedResult{graph: routed, score: layoutScore{penalty: 11, area: 19200}, sequenceEdges: map[layoutgraph.EntityID]struct{}{3: {}}}
	if refinement.score.compare(incumbent.score) <= 0 {
		t.Fatal("fixture must have a strictly worse generic score")
	}
	for index, node := range routed.Nodes {
		original := g.Nodes[index]
		if *node.TopLeft != *original.TopLeft || node.Width != original.Width || node.Height != original.Height {
			t.Fatal("fixture changed the selected node placement")
		}
	}
	selected, err := refineSeedResult(t.Context(), incumbent, func() (seedResult, error) { return refinement, nil })
	if err != nil {
		t.Fatal(err)
	}
	if selected.graph != refinement.graph || !reflect.DeepEqual(selected, refinement) {
		t.Fatal("successful routing refinement was replaced by generic placement scoring")
	}
	// The ordinary placement-candidate path still rejects the very same score.
	ordinary, err := considerSeedCandidate(t.Context(), incumbent, func() (seedResult, error) { return refinement, nil })
	if err != nil || !reflect.DeepEqual(ordinary, incumbent) {
		t.Fatalf("routing acceptance leaked into placement selection: selected=%p err=%v", ordinary.graph, err)
	}
}

func TestOptionalSeedRefinementFailurePreservesCompleteSelectedResult(t *testing.T) {
	incumbent := seedResult{graph: layoutgraph.NewGraph(), score: layoutScore{penalty: 10, area: 100}, sequenceEdges: map[layoutgraph.EntityID]struct{}{7: {}}}
	for _, test := range []struct {
		name  string
		err   error
		panic bool
		want  error
	}{
		{name: "ordinary error", err: errors.New("refined geometry failed validation")},
		{name: "panic", panic: true},
		{name: "canceled", err: context.Canceled, want: context.Canceled},
		{name: "wrapped cancellation", err: fmt.Errorf("routing: %w", context.Canceled), want: context.Canceled},
		{name: "deadline", err: context.DeadlineExceeded, want: context.DeadlineExceeded},
	} {
		t.Run(test.name, func(t *testing.T) {
			selected, err := refineSeedResult(t.Context(), incumbent, func() (seedResult, error) {
				if test.panic {
					panic("optional routing invariant")
				}
				return seedResult{graph: layoutgraph.NewGraph(), score: layoutScore{penalty: 0}}, test.err
			})
			if !errors.Is(err, test.want) {
				t.Fatalf("got error %v, want %v", err, test.want)
			}
			if !reflect.DeepEqual(selected, incumbent) {
				t.Fatal("failed refinement replaced part of the selected result")
			}
		})
	}
}

func TestOptionalSeedRefinementCallerCancellationAlwaysPropagates(t *testing.T) {
	for _, kind := range []string{"success", "error", "panic"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			incumbent := seedResult{graph: layoutgraph.NewGraph(), score: layoutScore{penalty: 10}}
			_, err := refineSeedResult(ctx, incumbent, func() (seedResult, error) {
				cancel()
				if kind == "panic" {
					panic("interrupted refinement")
				}
				if kind == "error" {
					return seedResult{}, errors.New("local failure during cancellation")
				}
				return seedResult{graph: layoutgraph.NewGraph(), score: layoutScore{penalty: 0}}, nil
			})
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("got %v, want caller cancellation", err)
			}
		})
	}
}
