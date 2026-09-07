package d2talalayout

import (
	"context"
	"errors"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
)

func TestOptionalSeedCandidatePreservesValidIncumbent(t *testing.T) {
	incumbent := seedResult{graph: layoutgraph.NewGraph(), score: layoutScore{penalty: 10}}
	for _, kind := range []string{"error", "panic", "worse", "tie", "better"} {
		t.Run(kind, func(t *testing.T) {
			other := seedResult{graph: layoutgraph.NewGraph(), score: layoutScore{penalty: 11}}
			selected, err := considerSeedCandidate(t.Context(), incumbent, func() (seedResult, error) {
				switch kind {
				case "error":
					return seedResult{}, errors.New("optional search cannot route this geometry")
				case "panic":
					panic("optional invariant")
				case "tie":
					other.score = incumbent.score
				case "better":
					other.score.penalty = 9
				}
				return other, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			want := incumbent.graph
			if kind == "better" {
				want = other.graph
			}
			if selected.graph != want {
				t.Fatalf("%s changed candidate selection", kind)
			}
		})
	}
}

func TestOptionalSeedCandidateCancellationPropagates(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	_, err := considerSeedCandidate(ctx, seedResult{}, func() (seedResult, error) { cancel(); panic("interrupted proposal") })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want cancellation", err)
	}
}
