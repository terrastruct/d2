package trees

import (
	"context"
	"errors"
	"testing"

	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/invariant"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
)

func treeInvariantGuard(t *testing.T) *limits.WorkGuard {
	t.Helper()
	guard, err := newWorkGuard(context.Background(), "TreeInvariantTest")
	if err != nil {
		t.Fatal(err)
	}
	return guard
}

func TestValidateBottomOrientation(t *testing.T) {
	parent := &layoutgraph.Tree{Node: layoutgraph.NewNode(1, 10, 10)}
	parent.Node.TopLeft = geo.NewPoint(0, 0)
	child := &layoutgraph.Tree{Node: layoutgraph.NewNode(2, 10, 10), Parent: parent}
	child.Node.TopLeft = geo.NewPoint(0, 20)
	parent.Children = []*layoutgraph.Tree{child}

	if err := validateBottomOrientation(parent, treeInvariantGuard(t)); err != nil {
		t.Fatalf("bottom-oriented tree rejected: %v", err)
	}
	child.Node.TopLeft.Y = 5
	err := validateBottomOrientation(parent, treeInvariantGuard(t))
	if !errors.Is(err, invariant.ErrViolation) {
		t.Fatalf("orientation error = %v, want invariant.ErrViolation", err)
	}
}

func TestPositionTreeEdgeLabelsValidatesBeforeMutation(t *testing.T) {
	placementTree := &layoutgraph.Tree{Node: layoutgraph.NewNode(1, 10, 10), Orientation: geo.Bottom}
	placementTree.Node.TopLeft = geo.NewPoint(0, 0)
	valid := &layoutgraph.Tree{Node: layoutgraph.NewNode(2, 10, 10), Parent: placementTree}
	valid.Node.TopLeft = geo.NewPoint(0, 20)
	valid.SentinelEdge = layoutgraph.NewEdge(valid.Node, placementTree.Node)
	valid.SentinelEdge.Label = &layoutgraph.Label{}
	valid.SentinelEdge.MinHeight = 10
	invalid := &layoutgraph.Tree{Node: layoutgraph.NewNode(3, 10, 10), Parent: placementTree}
	invalid.Node.TopLeft = geo.NewPoint(20, 5)
	placementTree.Children = []*layoutgraph.Tree{valid, invalid}

	err := positionTreeEdgeLabels(placementTree, true, treeInvariantGuard(t))
	if !errors.Is(err, invariant.ErrViolation) {
		t.Fatalf("position labels error = %v, want invariant.ErrViolation", err)
	}
	if valid.SentinelEdge.Label.Position != label.Unset || valid.SentinelEdge.LabelPercentage != 0 {
		t.Fatal("a label was mutated before all tree orientations were validated")
	}
}

func TestPositionTreeEdgeLabelsRestoresOrientationOnError(t *testing.T) {
	g := layoutgraph.NewGraph()
	placementTree := &layoutgraph.Tree{Node: layoutgraph.NewNode(1, 10, 20), Orientation: geo.Right}
	placementTree.Node.TopLeft = geo.NewPoint(10, 20)
	invalid := &layoutgraph.Tree{Node: layoutgraph.NewNode(2, 6, 8), Parent: placementTree}
	invalid.Node.TopLeft = geo.NewPoint(10, 20)
	placementTree.Children = []*layoutgraph.Tree{invalid}
	g.AddNodeUnchecked(placementTree.Node)
	g.AddNodeUnchecked(invalid.Node)
	parentTopLeft := placementTree.Node.TopLeft.Copy()
	childTopLeft := invalid.Node.TopLeft.Copy()
	parentWidth, parentHeight := placementTree.Node.Width, placementTree.Node.Height
	childWidth, childHeight := invalid.Node.Width, invalid.Node.Height

	if err := positionTreeEdgeLabels(placementTree, true, treeInvariantGuard(t)); err == nil {
		t.Fatal("invalid orientation was not detected")
	}
	if !placementTree.Node.TopLeft.Equals(parentTopLeft) || placementTree.Node.Width != parentWidth || placementTree.Node.Height != parentHeight {
		t.Fatal("placement tree was not restored after validation error")
	}
	if !invalid.Node.TopLeft.Equals(childTopLeft) || invalid.Node.Width != childWidth || invalid.Node.Height != childHeight {
		t.Fatal("child tree was not restored after validation error")
	}
}
