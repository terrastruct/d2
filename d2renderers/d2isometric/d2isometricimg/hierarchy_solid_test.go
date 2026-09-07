package d2isometricimg

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2target"
)

func TestSolidHierarchyHeaderHeightMatchesBodyBeforeCompression(t *testing.T) {
	for _, kind := range []string{d2target.ShapeCylinder, d2target.ShapeQueue, d2target.ShapeCircle, d2target.ShapeOval, d2target.ShapeHexagon} {
		for _, height := range []float64{.01, .85, 1.15} {
			for _, threeD := range []bool{false, true} {
				s := d2target.BaseShape()
				s.Type, s.Width, s.Height, s.ThreeDee = kind, 100, 80, threeD
				// Styling must not turn the same volumetric source shape into
				// a shallow plaque, or change its header occlusion estimate.
				s.FillPattern, s.StrokeDash = "lines", 4
				n := d2isometric.Node{Type: kind, Size: nv(2, height, 1.6), Position: nv(4, .07+height/2, 5), Metadata: d2isometric.NodeMetadata{Original: *s}}
				original := n
				got := hierarchyRenderNodes([]d2isometric.Node{n})[0]
				wantHeight := max(.04, height)
				if threeD {
					wantHeight += d2target.THREE_DEE_OFFSET * .02
				}
				wantHeight *= .60
				if math.Abs(got.Size.Y-wantHeight) > 1e-10 || math.Abs(got.Position.Y-got.Size.Y/2-.07) > 1e-10 {
					t.Fatalf("%s body height/floor mismatch: %+v, want height %g", kind, got, wantHeight)
				}
				if got.Size.X != n.Size.X || got.Size.Z != n.Size.Z || got.Position.X != n.Position.X || got.Position.Z != n.Position.Z || !reflect.DeepEqual(original, n) {
					t.Fatal("header projection changed the source footprint or metadata")
				}
			}
		}
	}
	panel := d2isometric.Node{Type: d2target.ShapeRectangle, Size: nv(2, .7, 1.6)}
	if hierarchyNodeRelief(panel) != hierarchyRelief || hierarchyNodeRelief(panel) < .30 {
		t.Fatal("ordinary source panels lost their visible body relief")
	}
}

func TestHierarchyHeaderClearsTallerStyledSolid(t *testing.T) {
	board := d2isometric.Board{ID: "system", Size: nv(8, .14, 6)}
	owner := d2isometric.Node{Metadata: d2isometric.NodeMetadata{Original: d2target.Shape{LabelPosition: "INSIDE_TOP_CENTER"}}}
	surface := labelSurface{center: nv(0, .032, 0), width: 3, depth: .4}
	s := d2target.BaseShape()
	s.ID, s.Type, s.Width, s.Height = "database", d2target.ShapeCylinder, 400, 180
	s.Fill, s.Stroke, s.FillPattern = "#ccd9e5", "#617588", "lines"
	child := d2isometric.Node{ID: s.ID, Type: s.Type, Fill: s.Fill, Stroke: s.Stroke, FillExplicit: true, StrokeWidth: s.StrokeWidth, Opacity: 1,
		Position: nv(0, .07+1.15/2, -1.15), Size: nv(4, 1.15, 1.8), Metadata: d2isometric.NodeMetadata{Original: *s}}
	headers := hierarchyRenderNodes([]d2isometric.Node{child})
	got, fits := hierarchyBoardHeaderPlacement(surface, board, owner, headers, .01)
	if !fits || got.width != surface.width || got.depth != surface.depth || got.center.Y != surface.center.Y {
		t.Fatalf("source-sized header lost its placement: %+v, fit=%v", got, fits)
	}
	if math.Abs(got.center.Z-2.75) > 1e-9 {
		t.Fatalf("header remained in the taller cylinder's projected top margin: %+v", got)
	}
	// Compare against the completed solid mesh as a separate, final-projection
	// check. The header planner itself operates before node meshes exist.
	b := &meshBuilder{ctx: context.Background(), scale: .01}
	b.hierarchyNode(child, "#849ebc")
	if b.err != nil {
		t.Fatal(b.err)
	}
	obstacles := newRouteCaptionPlacer()
	obstacles.AvoidMesh(b.triangles)
	if score, complete := obstacles.score(captionRect(got, false)); !complete || score != 0 {
		t.Fatal("planned header is covered by finalized styled-solid geometry")
	}
}
