package placement

import (
	"fmt"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
)

// optimizerCandidateMovement reuses the candidate search's existing rollback
// snapshot as its stable movement set. Scoring only changes geometry, so every
// trial visits the same descendants in the same order. Its caller must restore
// positions if the search fails or panics, including an error midway through a
// move; it must not retain this movement set across topology changes.
type optimizerCandidateMovement struct {
	positions      []nodePositionSnapshot
	descendantWork uint64
}

func captureOptimizerCandidateMovement(node *layoutgraph.Node, guard *limits.OptimizationWorkGuard) (optimizerCandidateMovement, error) {
	before := guard.Used()
	positions, err := captureOptimizerNodePositions([]*layoutgraph.Node{node}, guard)
	if err != nil {
		return optimizerCandidateMovement{}, err
	}
	// Single-root capture charges one Step for each retained position in addition
	// to the descendant walk. Retain the latter's charge so every trial preserves
	// the original work boundary and cancellation polling sequence.
	return optimizerCandidateMovement{
		positions:      positions,
		descendantWork: guard.Used() - before - uint64(len(positions)),
	}, nil
}

func (movement optimizerCandidateMovement) moveAbs(x, y float64, guard *limits.OptimizationWorkGuard) error {
	node := movement.positions[0].node
	if node.TopLeft == nil {
		return fmt.Errorf("TALA %s cannot move an unpositioned node", guard.Location())
	}
	if node.TopLeft.X == x && node.TopLeft.Y == y {
		return guard.Step()
	}
	// Replay the original walk's Step calls instead of batching them: custom
	// contexts and resource failures keep the same observation boundaries.
	for range movement.descendantWork {
		if err := guard.Step(); err != nil {
			return err
		}
	}
	descendants := movement.positions[1:]
	for _, position := range descendants {
		if err := guard.Step(); err != nil {
			return err
		}
		if position.node.TopLeft == nil {
			return fmt.Errorf("TALA %s cannot move an unpositioned descendant", guard.Location())
		}
	}
	dx, dy := x-node.TopLeft.X, y-node.TopLeft.Y
	node.Translate(dx, dy)
	for _, position := range descendants {
		if err := guard.Step(); err != nil {
			return err
		}
		position.node.Translate(dx, dy)
	}
	return nil
}
