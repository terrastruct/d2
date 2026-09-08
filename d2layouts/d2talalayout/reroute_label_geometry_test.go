package d2talalayout

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
)

// These fixtures explicitly use a three-pixel stroke, matching the geometry
// consumed by TALA's edge-label placement.
func rerouteLabelBox(edge *d2graph.Edge) *geo.Box {
	percentage := 0.0
	if edge.LabelPercentage != nil {
		percentage = *edge.LabelPercentage
	}
	topLeft, _ := label.FromString(*edge.LabelPosition).GetPointOnRoute(
		edge.Route, 3, percentage,
		float64(edge.LabelDimensions.Width), float64(edge.LabelDimensions.Height),
	)
	return geo.NewBox(topLeft, float64(edge.LabelDimensions.Width), float64(edge.LabelDimensions.Height))
}

func TestTranslateGraphPreservesExistingEdgeLabelGeometry(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		for _, position := range []label.Position{
			label.UnlockedTop, label.UnlockedMiddle, label.UnlockedBottom,
			label.OutsideTopLeft, label.OutsideTopCenter, label.OutsideTopRight,
			label.InsideMiddleLeft, label.InsideMiddleCenter, label.InsideMiddleRight,
			label.OutsideBottomLeft, label.OutsideBottomCenter, label.OutsideBottomRight,
		} {
			for _, percentage := range []struct {
				name  string
				value *float64
			}{
				{name: "nil"},
				{name: "start", value: new(0.0)},
				{name: "interior", value: new(0.35)},
				{name: "end", value: new(1.0)},
			} {
				t.Run(fmt.Sprintf("reverse=%t/%s/%s", reverse, position, percentage.name), func(t *testing.T) {
					graph, edge := newD2TransactionGraph(false)
					edge.Dst.TopLeft.Y = 90
					edge.Route = []*geo.Point{
						geo.NewPoint(110, 50), geo.NewPoint(180, 50),
						geo.NewPoint(180, 120), geo.NewPoint(250, 120),
					}
					edge.SrcArrow, edge.DstArrow = reverse, !reverse
					edge.Label.Value = "existing label"
					edge.LabelDimensions = d2target.TextDimensions{Width: 80, Height: 20}
					edge.Style.StrokeWidth = &d2graph.Scalar{Value: "3"}
					edge.LabelPosition = new(position.String())
					edge.LabelPercentage = percentage.value
					want := rerouteLabelBox(edge).TopLeft
					before := snapshotD2Graph(t, graph)

					translated := layoutgraph.NewGraph()
					if _, err := translateGraph(t.Context(), graph, translated, true); err != nil {
						t.Fatal(err)
					}
					actual := translated.Edges[0]
					if percentage.value == nil && !position.IsUnlocked() && actual.LabelPercentage != 0 {
						t.Fatalf("saved fixed position without a percentage acquired search percentage %g", actual.LabelPercentage)
					}
					got := actual.LabelTopLeft(actual.Label.Position, actual.Label.Width, actual.Label.Height)
					if !got.Equals(want) {
						t.Fatalf("translated label origin = %v, want existing origin %v", got, want)
					}
					before.assertUnchanged(t, graph)
				})
			}
		}
	}
}

func TestTranslateGraphKeepsFreshEdgeLabelPlacementUnset(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprintf("reverse=%t", reverse), func(t *testing.T) {
			graph, edge := newD2TransactionGraph(false)
			edge.SrcArrow, edge.DstArrow = reverse, !reverse
			edge.Route = nil
			edge.Label.Value = "new label"
			edge.LabelDimensions = d2target.TextDimensions{Width: 80, Height: 20}
			translated := layoutgraph.NewGraph()
			if _, err := translateGraph(t.Context(), graph, translated, true); err != nil {
				t.Fatal(err)
			}
			actual := translated.Edges[0]
			if actual.Label.Position != label.Unset || actual.LabelPercentage != 0 {
				t.Fatalf("fresh label acquired placement %s at %g, which bypasses the unlocked-position search", actual.Label.Position, actual.LabelPercentage)
			}
		})
	}
}

func TestLayoutIgnoresValidExistingEdgeLabelGeometry(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprintf("reverse=%t", reverse), func(t *testing.T) {
			layout := func(percentage *float64) []byte {
				t.Helper()
				graph, edge := newD2TransactionGraph(false)
				edge.SrcArrow, edge.DstArrow = reverse, !reverse
				edge.Label.Value = "label"
				edge.LabelDimensions = d2target.TextDimensions{Width: 80, Height: 20}
				edge.Style.StrokeWidth = &d2graph.Scalar{Value: "3"}
				if percentage != nil {
					edge.LabelPosition = new(label.UnlockedTop.String())
					edge.LabelPercentage = percentage
				}
				if err := Layout(t.Context(), graph, &Options{Seeds: []int64{1}, MaxConcurrency: 1}); err != nil {
					t.Fatal(err)
				}
				serialized, err := d2graph.SerializeGraph(graph)
				if err != nil {
					t.Fatal(err)
				}
				return serialized
			}
			want := layout(nil)
			for _, percentage := range []float64{0, 0.35, 1} {
				if got := layout(new(percentage)); !bytes.Equal(got, want) {
					t.Fatalf("full Layout result depends on existing label percentage %g", percentage)
				}
			}
		})
	}
}

func TestRouteEdgesAvoidsExistingLabelGeometry(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		for _, percentage := range []float64{0.35, 0.5, 0.65} {
			t.Run(fmt.Sprintf("reverse=%t/percentage=%g", reverse, percentage), func(t *testing.T) {
				graph, existing := newD2TransactionGraph(false)
				existing.Src.TopLeft = geo.NewPoint(0, 0)
				existing.Src.Width, existing.Src.Height = 40, 40
				existing.Dst.TopLeft = geo.NewPoint(600, 0)
				existing.Dst.Width, existing.Dst.Height = 40, 40
				existing.Route = []*geo.Point{geo.NewPoint(40, 20), geo.NewPoint(600, 20)}
				existing.SrcArrow, existing.DstArrow = reverse, !reverse
				existing.Label.Value = "existing label"
				existing.LabelDimensions = d2target.TextDimensions{Width: 100, Height: 20}
				existing.Style.StrokeWidth = &d2graph.Scalar{Value: "3"}
				existing.LabelPosition = new(label.UnlockedTop.String())
				existing.LabelPercentage = new(percentage)
				labelPosition, labelPercentage := existing.LabelPosition, existing.LabelPercentage
				originalBox := rerouteLabelBox(existing)
				originalRoute := slices.Clone(existing.Route)
				originalPoints := []geo.Point{*existing.Route[0], *existing.Route[1]}

				added := &d2graph.Edge{Index: 1, Src: existing.Src, Dst: existing.Dst, DstArrow: true}
				added.Label.Value = "new label"
				added.LabelDimensions = d2target.TextDimensions{Width: 100, Height: 20}
				added.Style.StrokeWidth = &d2graph.Scalar{Value: "3"}
				graph.Edges = append(graph.Edges, added)
				if err := RouteEdges(t.Context(), graph, []*d2graph.Edge{added}); err != nil {
					t.Fatal(err)
				}
				if existing.LabelPosition != labelPosition || existing.LabelPercentage != labelPercentage ||
					*existing.LabelPosition != label.UnlockedTop.String() || *existing.LabelPercentage != percentage {
					t.Fatal("rerouting changed the unselected label's placement")
				}
				if !slices.Equal(existing.Route, originalRoute) || *existing.Route[0] != originalPoints[0] || *existing.Route[1] != originalPoints[1] {
					t.Fatal("rerouting changed the unselected edge's route")
				}
				if added.LabelPosition == nil || len(added.Route) < 2 {
					t.Fatal("new edge has no complete route and label placement")
				}
				newBox := rerouteLabelBox(added)
				if originalBox.TopLeft.X < newBox.TopLeft.X+newBox.Width &&
					newBox.TopLeft.X < originalBox.TopLeft.X+originalBox.Width &&
					originalBox.TopLeft.Y < newBox.TopLeft.Y+newBox.Height &&
					newBox.TopLeft.Y < originalBox.TopLeft.Y+originalBox.Height {
					t.Fatalf("new label at %v overlaps existing label at %v", newBox.TopLeft, originalBox.TopLeft)
				}
			})
		}
	}
}

func TestInvalidEdgeLabelPercentageIsRejectedAtomically(t *testing.T) {
	for _, operation := range []struct {
		name string
		run  func(context.Context, *d2graph.Graph, *d2graph.Edge) error
	}{
		{name: "Layout", run: func(ctx context.Context, graph *d2graph.Graph, _ *d2graph.Edge) error {
			return Layout(ctx, graph, &Options{Seeds: []int64{1}, MaxConcurrency: 1})
		}},
		{name: "RouteEdges", run: func(ctx context.Context, graph *d2graph.Graph, selected *d2graph.Edge) error {
			return RouteEdges(ctx, graph, []*d2graph.Edge{selected})
		}},
	} {
		for _, invalid := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), -0.1, 1.1} {
			t.Run(fmt.Sprintf("%s/%g", operation.name, invalid), func(t *testing.T) {
				graph, existing := newD2TransactionGraph(false)
				existing.Label.Value = "existing label"
				existing.LabelPosition = new(label.UnlockedTop.String())
				existing.LabelPercentage = new(0.5)
				selected := &d2graph.Edge{Index: 1, Src: existing.Src, Dst: existing.Dst, DstArrow: true}
				graph.Edges = append(graph.Edges, selected)
				before := snapshotD2Graph(t, graph)
				percentage := existing.LabelPercentage
				*percentage = invalid
				if err := operation.run(t.Context(), graph, selected); err == nil || !strings.Contains(err.Error(), "invalid label percentage") {
					t.Fatalf("invalid label percentage error = %v", err)
				}
				if existing.LabelPercentage != percentage || math.Float64bits(*percentage) != math.Float64bits(invalid) {
					t.Fatal("failed operation changed the invalid percentage")
				}
				// The snapshot uses JSON, which cannot represent NaN/infinities.
				// Restore only the verified-unchanged test value, then compare
				// every other field and pointer with the original snapshot.
				*percentage = 0.5
				before.assertUnchanged(t, graph)
			})
		}
	}
}
