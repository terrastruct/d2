package d2elklayout

import (
	"testing"

	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
	"github.com/d2lang/util-go/go2"
)

func TestDeconflictSelfLoopLabels(t *testing.T) {
	obj := &d2graph.Object{}
	edges := []*d2graph.Edge{
		newLabeledSelfLoop(obj, 48, 37, [][2]float64{
			{151, 352}, {151, 402}, {52, 402}, {52, 22}, {192.66666666666669, 22}, {192.66666666666669, 72},
		}),
		newLabeledSelfLoop(obj, 54, 85, [][2]float64{
			{176, 352}, {176, 412}, {39, 412}, {39, 12}, {259.33333333333337, 12}, {259.33333333333337, 72},
		}),
	}
	g := &d2graph.Graph{Edges: edges}

	beforeA := edgeLabelBox(edges[0], label.InsideMiddleCenter, 0)
	beforeB := edgeLabelBox(edges[1], label.InsideMiddleCenter, 0)
	if !materiallyOverlapsAny(beforeB, []*geo.Box{beforeA}, float64(label.PADDING)) {
		t.Fatal("candidate labels should reproduce the material overlap")
	}

	deconflictSelfLoopLabels(g)

	if got := *edges[0].LabelPosition; got != label.InsideMiddleCenter.String() {
		t.Fatalf("first label position = %q, want centered", got)
	}
	if edges[0].LabelPercentage != nil {
		t.Fatalf("first label percentage = %v, want nil", *edges[0].LabelPercentage)
	}
	if got := *edges[1].LabelPosition; got != label.UnlockedMiddle.String() {
		t.Fatalf("second label position = %q, want unlocked middle", got)
	}
	if edges[1].LabelPercentage == nil || *edges[1].LabelPercentage != 0.4 {
		t.Fatalf("second label percentage = %v, want 0.4", edges[1].LabelPercentage)
	}
	afterB := edgeLabelBox(edges[1], label.UnlockedMiddle, *edges[1].LabelPercentage)
	if beforeA.Overlaps(*afterB) {
		t.Fatalf("labels still overlap: first=%v second=%v", beforeA, afterB)
	}
}

func TestDeconflictSelfLoopLabelsPreservesFrozenNearTouch(t *testing.T) {
	obj := &d2graph.Object{}
	edges := []*d2graph.Edge{
		newLabeledSelfLoop(obj, 48, 37, [][2]float64{
			{97, 478}, {97, 528}, {22, 528}, {22, 148}, {138.66666666666669, 148}, {138.66666666666669, 198},
		}),
		newLabeledSelfLoop(obj, 54, 85, [][2]float64{
			{122, 478}, {122, 538}, {12, 538}, {12, 99}, {205.33333333333334, 99}, {205.33333333333334, 198},
		}),
	}

	deconflictSelfLoopLabels(&d2graph.Graph{Edges: edges})

	for i, edge := range edges {
		if got := *edge.LabelPosition; got != label.InsideMiddleCenter.String() {
			t.Fatalf("edge %d label position = %q, want centered", i, got)
		}
		if edge.LabelPercentage != nil {
			t.Fatalf("edge %d label percentage = %v, want nil", i, *edge.LabelPercentage)
		}
	}
}

func newLabeledSelfLoop(obj *d2graph.Object, width, height int, coordinates [][2]float64) *d2graph.Edge {
	route := make([]*geo.Point, 0, len(coordinates))
	for _, coordinate := range coordinates {
		route = append(route, geo.NewPoint(coordinate[0], coordinate[1]))
	}
	return &d2graph.Edge{
		Src:           obj,
		Dst:           obj,
		Route:         route,
		LabelPosition: go2.Pointer(label.InsideMiddleCenter.String()),
		Attributes: d2graph.Attributes{
			Label:           d2graph.Scalar{Value: "label"},
			LabelDimensions: d2target.TextDimensions{Width: width, Height: height},
		},
	}
}
