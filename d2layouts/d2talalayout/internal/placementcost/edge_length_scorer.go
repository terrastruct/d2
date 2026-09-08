package placementcost

import (
	"context"
	"fmt"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

type edgeLengthStatic struct {
	direction       geo.Orientation
	directionFactor float64
}

// NodeEdgeLengthScorer reuses incident-edge topology during one candidate sweep.
// Geometry is read from live nodes on every Score. The caller must keep edges,
// abductions, container/cluster membership, labels and directions unchanged until
// Close. In particular, a scorer must not survive a topology-changing swap.
// It is local mutable scratch and must not be shared between goroutines.
type NodeEdgeLengthScorer struct {
	node       *layoutgraph.Node
	options    EdgeLengthOptions
	scratch    *edgeScratch
	setup      edgeLengthStatic
	prepChecks int
}

// NewNodeEdgeLengthScorer starts an unprepared sweep. Preparation is lazy so
// cancellation and the caller's work charge occur at the original score point.
// The returned value must not be copied after its first Score; call Close when
// the sweep ends to release references retained by its pooled scratch.
func NewNodeEdgeLengthScorer(node *layoutgraph.Node, options EdgeLengthOptions) NodeEdgeLengthScorer {
	return NodeEdgeLengthScorer{node: node, options: options}
}

// Score has the same value and cancellation checkpoints as NodeEdgeLength while
// the scorer's topology remains fixed. The caller still charges the original
// scoring work budget for each candidate; reusing setup does not bypass limits.
func (scorer *NodeEdgeLengthScorer) Score(ctx context.Context) (float64, error) {
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("EdgeLength: %w", err)
	}
	if scorer.node == nil {
		return 0, fmt.Errorf("EdgeLength: scorer is closed")
	}
	if scorer.scratch == nil {
		s := scratchPool.Get()
		// Publish ownership before preparation so a caller's deferred Close also
		// releases scratch if a malformed graph or context panics during setup.
		scorer.scratch = s
		checks := edgeLengthPreparationContext{Context: ctx}
		setup, err := prepareNodeEdgeLength(&checks, scorer.node, scorer.options, s)
		if err != nil {
			putEdgeScratch(s)
			scorer.scratch = nil
			return 0, err
		}
		scorer.scratch, scorer.setup, scorer.prepChecks = s, setup, checks.count
	} else {
		// Preparation only inspects topology. Replay its exact polls without
		// repeating endpoint matching and label-count construction. This keeps
		// cancellation observability equal even for a context counting Err calls.
		for range scorer.prepChecks {
			if err := checkScoringCancellation(ctx); err != nil {
				return 0, err
			}
		}
	}
	return evaluateNodeEdgeLength(ctx, scorer.node, scorer.options, scorer.scratch, scorer.setup)
}

// Close releases scratch and makes the scorer unusable. It is safe to call more
// than once, including after an unsuccessful Score.
func (scorer *NodeEdgeLengthScorer) Close() {
	if scorer.scratch != nil {
		putEdgeScratch(scorer.scratch)
	}
	*scorer = NodeEdgeLengthScorer{}
}

type edgeLengthPreparationContext struct {
	context.Context
	count int
}

func (ctx *edgeLengthPreparationContext) Err() error {
	ctx.count++
	return ctx.Context.Err()
}
