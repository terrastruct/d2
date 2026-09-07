package placementcost

import (
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

func newScorerFixture(edgeCount int, abducted bool) (*layoutgraph.Node, *layoutgraph.Node, EdgeLengthOptions) {
	g, node, options := newParallelLabeledEdgeLengthGraph(edgeCount)
	adjacent := node.Adjacent(node.Edges[0])
	delete(g.Directions, nil) // Exercise inferred direction from parallel labels.
	obstacle := layoutgraph.NewNode(3, 15, 20)
	obstacle.TopLeft = geo.NewPoint(15, 50)
	g.AddNode(obstacle)
	g.AddNodeToContainer(nil, obstacle)
	node.Nears[obstacle] = struct{}{}
	if abducted {
		for range edgeCount {
			options.EdgeAbductions = append(options.EdgeAbductions, &layoutgraph.EdgeAbduction{CurrentFrom: node, CurrentTo: adjacent})
		}
	}
	return node, adjacent, options
}

func TestNodeEdgeLengthScorerReadsLiveGeometry(t *testing.T) {
	for _, includeSizes := range []bool{false, true} {
		for _, direction := range []bool{false, true} {
			for _, abducted := range []bool{false, true} {
				t.Run(fmt.Sprintf("sizes=%t/direction=%t/abducted=%t", includeSizes, direction, abducted), func(t *testing.T) {
					node, adjacent, options := newScorerFixture(7, abducted)
					options.IncludeNodeSizes, options.PenalizeDirection = includeSizes, direction
					options.EnforceMinimumGap = true
					scorer := NewNodeEdgeLengthScorer(node, options)
					defer scorer.Close()
					for i, point := range []geo.Point{{X: 0, Y: 0}, {X: -90, Y: 100}, {X: 200, Y: 300}, {X: 20, Y: 30}, {X: 0, Y: 0}} {
						node.TopLeft.X, node.TopLeft.Y = point.X, point.Y
						node.Width, node.Height = 10+float64(i), 15+float64(i)
						adjacent.TopLeft.X, adjacent.TopLeft.Y = 10*float64(i), 90-10*float64(i)
						want, err := NodeEdgeLength(t.Context(), node, options)
						if err != nil {
							t.Fatal(err)
						}
						got, err := scorer.Score(t.Context())
						if err != nil {
							t.Fatal(err)
						}
						if math.Float64bits(got) != math.Float64bits(want) {
							t.Fatalf("candidate %d: prepared=%v original=%v", i, got, want)
						}
					}
				})
			}
		}
	}
}

type scorerCountingContext struct {
	context.Context
	checks, cancelAt int
}

func (ctx *scorerCountingContext) Err() error {
	ctx.checks++
	if ctx.cancelAt > 0 && ctx.checks >= ctx.cancelAt {
		return context.Canceled
	}
	return nil
}

func TestNodeEdgeLengthScorerPreservesEveryCancellationCheckpoint(t *testing.T) {
	node, _, options := newScorerFixture(129, true)
	ctx := &scorerCountingContext{Context: context.Background()}
	if _, err := NodeEdgeLength(ctx, node, options); err != nil {
		t.Fatal(err)
	}
	checks := ctx.checks
	scorer := NewNodeEdgeLengthScorer(node, options)
	defer scorer.Close()
	first := &scorerCountingContext{Context: context.Background()}
	if _, err := scorer.Score(first); err != nil || first.checks != checks {
		t.Fatalf("first evaluation: err=%v checks=%d want=%d", err, first.checks, checks)
	}
	for cancelAt := 1; cancelAt <= checks+1; cancelAt++ {
		originalCtx := &scorerCountingContext{Context: context.Background(), cancelAt: cancelAt}
		preparedCtx := &scorerCountingContext{Context: context.Background(), cancelAt: cancelAt}
		want, wantErr := NodeEdgeLength(originalCtx, node, options)
		got, gotErr := scorer.Score(preparedCtx)
		if math.Float64bits(got) != math.Float64bits(want) || fmt.Sprint(gotErr) != fmt.Sprint(wantErr) || originalCtx.checks != preparedCtx.checks {
			t.Fatalf("cancelAt=%d: prepared=(%v, %v, %d checks) original=(%v, %v, %d checks)", cancelAt, got, gotErr, preparedCtx.checks, want, wantErr, originalCtx.checks)
		}
	}
}

func TestNodeEdgeLengthScorerReleasesReferences(t *testing.T) {
	node, _, options := newScorerFixture(7, true)
	scorer := NewNodeEdgeLengthScorer(node, options)
	if _, err := scorer.Score(t.Context()); err != nil {
		t.Fatal(err)
	}
	scratch := scorer.scratch
	scorer.Close()
	scorer.Close()
	if scorer.node != nil || scorer.scratch != nil || scorer.options.EdgeAbductions != nil {
		t.Fatal("closed scorer retained graph references")
	}
	for _, n := range append(scratch.nRepl[:cap(scratch.nRepl)], scratch.aRepl[:cap(scratch.aRepl)]...) {
		if n != nil {
			t.Fatal("returned scratch retained endpoint references")
		}
	}
	if _, err := scorer.Score(t.Context()); err == nil {
		t.Fatal("closed scorer accepted evaluation")
	}
	// A fresh sweep must see a label/topology edit rather than inherit counts
	// retained by the previous sweep's scratch.
	for _, edge := range node.Edges {
		edge.Label = nil
	}
	scorer = NewNodeEdgeLengthScorer(node, options)
	defer scorer.Close()
	want, err := NodeEdgeLength(t.Context(), node, options)
	if err != nil {
		t.Fatal(err)
	}
	got, err := scorer.Score(t.Context())
	if err != nil || math.Float64bits(got) != math.Float64bits(want) {
		t.Fatalf("fresh sweep=(%v, %v), want %v", got, err, want)
	}
}

func TestNodeEdgeLengthScorerCanceledPreparationCanRetry(t *testing.T) {
	node, _, options := newScorerFixture(7, true)
	scorer := NewNodeEdgeLengthScorer(node, options)
	defer scorer.Close()
	ctx := &scorerCountingContext{Context: context.Background(), cancelAt: 3}
	if _, err := scorer.Score(ctx); err == nil || scorer.scratch != nil {
		t.Fatalf("canceled preparation err=%v scratch=%v", err, scorer.scratch)
	}
	want, err := NodeEdgeLength(t.Context(), node, options)
	if err != nil {
		t.Fatal(err)
	}
	got, err := scorer.Score(t.Context())
	if err != nil || math.Float64bits(got) != math.Float64bits(want) {
		t.Fatalf("retry=(%v, %v), want %v", got, err, want)
	}
}

func BenchmarkNodeEdgeLengthScorer(b *testing.B) {
	for _, abducted := range []bool{false, true} {
		b.Run(fmt.Sprintf("abducted=%t", abducted), func(b *testing.B) {
			for _, prepared := range []bool{false, true} {
				b.Run(fmt.Sprintf("prepared=%t", prepared), func(b *testing.B) {
					node, _, options := newScorerFixture(64, abducted)
					scorer := NewNodeEdgeLengthScorer(node, options)
					defer scorer.Close()
					if _, err := scorer.Score(b.Context()); err != nil {
						b.Fatal(err)
					}
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						node.TopLeft.X = float64(i % 10)
						var err error
						if prepared {
							_, err = scorer.Score(b.Context())
						} else {
							_, err = NodeEdgeLength(b.Context(), node, options)
						}
						if err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}
