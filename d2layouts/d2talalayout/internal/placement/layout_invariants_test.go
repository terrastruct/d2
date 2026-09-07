package placement

import (
	"errors"
	"testing"

	"github.com/d2lang/d2/lib/geo"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/invariant"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
)

func requireBadState(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, invariant.ErrViolation) {
		t.Fatalf("error = %v, want invariant.ErrViolation", err)
	}
}

func TestValidatePlacedNodes(t *testing.T) {
	root := layoutgraph.NewNode(1, 100, 100)
	placed := layoutgraph.NewNode(2, 10, 10)
	placed.TopLeft = geo.NewPoint(0, 0)
	unplaced := layoutgraph.NewNode(3, 10, 10)

	if err := validatePlacedNodes(root, []*layoutgraph.Node{placed}); err != nil {
		t.Fatalf("placed node rejected: %v", err)
	}
	if err := validatePlacedNodes(root, []*layoutgraph.Node{placed, unplaced}); err == nil {
		t.Fatal("unplaced node was not detected")
	} else {
		requireBadState(t, err)
	}
}

func TestValidateGridAlignment(t *testing.T) {
	g := layoutgraph.NewGraph()
	g.CellSize = 10
	aligned := layoutgraph.NewNode(1, 10, 10)
	aligned.TopLeft = geo.NewPoint(20, 30)
	g.AddNode(aligned)

	if err := validateGridAlignment(g); err != nil {
		t.Fatalf("aligned graph rejected: %v", err)
	}
	aligned.TopLeft.X = 21
	if err := validateGridAlignment(g); err == nil {
		t.Fatal("misaligned node was not detected")
	} else {
		requireBadState(t, err)
	}
	aligned.TopLeft.X = 20.5
	if err := validateGridAlignment(g); err == nil {
		t.Fatal("fractional misalignment was not detected")
	} else {
		requireBadState(t, err)
	}
	aligned.FixedTopLeft = aligned.TopLeft.Copy()
	if err := validateGridAlignment(g); err != nil {
		t.Fatalf("fixed node must be exempt from grid alignment: %v", err)
	}

	g.CellSize = 0
	if err := validateGridAlignment(g); err == nil {
		t.Fatal("zero cell size was not detected")
	} else {
		requireBadState(t, err)
	}
	g.CellSize = 10.5
	if err := validateGridAlignment(g); err == nil {
		t.Fatal("fractional cell size was not detected")
	} else {
		requireBadState(t, err)
	}
}
