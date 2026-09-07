package d2isometricimg

import (
	"math"
	"reflect"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2target"
)

func TestHierarchyHeaderSourcePositionsAndOwnership(t *testing.T) {
	board := d2isometric.Board{ID: "parent", Position: nv(20, 0, 20), Size: nv(10, .14, 6)}
	surface := labelSurface{center: nv(0, .032, 0), width: 2, depth: .4, align: "left"}
	for _, tc := range []struct {
		position string
		x, z     float64
	}{
		{"INSIDE_TOP_LEFT", 16.05, 17.25},
		{"INSIDE_TOP_CENTER", 20, 17.25},
		{"INSIDE_BOTTOM_RIGHT", 23.95, 22.75},
		{"INSIDE_MIDDLE_CENTER", 20, 20},
		{"OUTSIDE_TOP_RIGHT", 23.95, 17.25},
		{"BORDER_BOTTOM_CENTER", 20, 22.75},
	} {
		t.Run(tc.position, func(t *testing.T) {
			owner := d2isometric.Node{ID: "parent", Metadata: d2isometric.NodeMetadata{Original: d2target.Shape{LabelPosition: tc.position}}}
			before, boardBefore := owner, board
			got := hierarchyBoardHeaderSurface(surface, board, owner, nil, .01)
			if math.Abs(got.center.X-tc.x) > 1e-9 || math.Abs(got.center.Z-tc.z) > 1e-9 || got.center.Y != surface.center.Y || got.width != surface.width || got.depth != surface.depth || got.align != surface.align {
				t.Fatalf("source anchor or print dimensions changed: %+v", got)
			}
			if !reflect.DeepEqual(owner, before) || !reflect.DeepEqual(board, boardBefore) {
				t.Fatal("header placement mutated source geometry")
			}
		})
	}
}

func TestHierarchyHeaderAvoidsNestedRaisedSilhouette(t *testing.T) {
	board := d2isometric.Board{ID: "parent", Size: nv(10, .14, 4)}
	owner := d2isometric.Node{Metadata: d2isometric.NodeMetadata{Original: d2target.Shape{LabelPosition: "INSIDE_TOP_LEFT"}}}
	surface := labelSurface{center: nv(0, .032, 0), width: 3, depth: .4, align: "left"}
	child := d2isometric.Node{ID: "nested.database", BoardID: "child-board", Opacity: 1, Position: nv(-3, 1, -.4), Size: nv(2, 2, 1.8)}
	got := hierarchyBoardHeaderSurface(surface, board, owner, []d2isometric.Node{child}, .01)
	wantZ := -2 + .05 + surface.depth/2
	if got.center.X-got.width/2 <= child.Position.X+child.Size.X/2+.005 || got.center.Z != wantZ || got.center.Y != surface.center.Y || got.width != surface.width || got.depth != surface.depth {
		t.Fatalf("nested component still obscures the source header strip: %+v", got)
	}
	if got.center.X+got.width/2 > 4.95 {
		t.Fatal("header escaped physical container")
	}
	// A clear centered title remains centered, rather than jumping to the
	// leftmost gap rather than retaining the authored anchor.
	owner.Metadata.Original.LabelPosition = "INSIDE_TOP_CENTER"
	child.Position.X = -4.8
	centered := hierarchyBoardHeaderSurface(surface, board, owner, []d2isometric.Node{child}, .01)
	if centered.center.X != 0 {
		t.Fatalf("unobscured source center moved: %+v", centered)
	}
}

func TestHierarchyHeaderVerticalStripDeterminismAndBlockedFallback(t *testing.T) {
	board := d2isometric.Board{ID: "parent", Size: nv(10, .14, 8)}
	owner := d2isometric.Node{Metadata: d2isometric.NodeMetadata{Original: d2target.Shape{LabelPosition: "INSIDE_MIDDLE_LEFT"}}}
	surface := labelSurface{center: nv(0, .032, 0), width: .6, depth: .8}
	child := d2isometric.Node{Opacity: 1, Position: nv(-3.9, .7, 0), Size: nv(1.6, 1.2, 1.5)}
	other := d2isometric.Node{Opacity: 1, Position: nv(4, .7, 3), Size: nv(1, 1.2, 1)}
	got := hierarchyBoardHeaderSurface(surface, board, owner, []d2isometric.Node{child, other}, .01)
	reordered := hierarchyBoardHeaderSurface(surface, board, owner, []d2isometric.Node{other, child}, .01)
	if got != reordered || math.Abs(got.center.X-(-4.65)) > 1e-9 || math.Abs(got.center.Z) < 1e-9 || got.center.Y != surface.center.Y || got.width != surface.width || got.depth != surface.depth {
		t.Fatalf("side header did not slide deterministically along its vertical strip: %+v / %+v", got, reordered)
	}
	child.Size = nv(100, 100, 100)
	blocked, fits := hierarchyBoardHeaderPlacement(surface, board, owner, []d2isometric.Node{child}, .01)
	if fits || math.Abs(blocked.center.Z) > 1e-9 || blocked.width != surface.width || blocked.depth != surface.depth {
		t.Fatalf("no-space fallback resized or displaced source label: %+v", blocked)
	}
}

func TestHierarchyHeaderFallsBackToClearBottomStrip(t *testing.T) {
	board := d2isometric.Board{ID: "parent", Size: nv(4, .14, 4)}
	owner := d2isometric.Node{Metadata: d2isometric.NodeMetadata{Original: d2target.Shape{LabelPosition: "INSIDE_TOP_CENTER"}}}
	surface := labelSurface{center: nv(0, .032, 0), width: 3.5, depth: .4, align: "left"}
	// No horizontal position on the source top strip can escape this raised
	// child. The source-sized bottom margin has room for the full title.
	child := d2isometric.Node{BoardID: "nested", Opacity: 1, Position: nv(0, .5, -1), Size: nv(3.8, .8, 1.2)}
	before := child
	got, fits := hierarchyBoardHeaderPlacement(surface, board, owner, []d2isometric.Node{child}, .01)
	if !fits || math.Abs(got.center.Z-1.75) > 1e-9 || got.center.X != 0 || got.center.Y != surface.center.Y || got.width != surface.width || got.depth != surface.depth || got.angle != surface.angle {
		t.Fatalf("full title was not relocated to the clear bottom strip: %+v, fit=%v", got, fits)
	}
	if !reflect.DeepEqual(child, before) {
		t.Fatal("fallback changed source component geometry")
	}
}

func TestHierarchyHeaderTinySurfaceAndIconPrintArea(t *testing.T) {
	owner := d2isometric.Node{Metadata: d2isometric.NodeMetadata{Original: d2target.Shape{LabelPosition: "INSIDE_TOP_RIGHT"}}}
	board := d2isometric.Board{Size: nv(8, .14, 3)}
	// This surface includes an icon allowance. The whole print area must be
	// anchored to the source right edge without losing its text allocation.
	surface := labelSurface{center: nv(0, .032, 0), width: 3.2, depth: .5}
	got := hierarchyBoardHeaderSurface(surface, board, owner, nil, .01)
	if got.width != surface.width || got.depth != surface.depth || math.Abs(got.center.X+got.width/2-3.95) > 1e-9 {
		t.Fatalf("combined icon and text area changed: %+v", got)
	}
	board.Size = nv(.01, .14, .02)
	got = hierarchyBoardHeaderSurface(surface, board, owner, nil, .01)
	if got.width <= 0 || got.depth <= 0 || got.width > board.Size.X || got.depth > board.Size.Z || math.Abs(got.width/got.depth-surface.width/surface.depth) > 1e-9 {
		t.Fatalf("tiny source container was enlarged or content stretched: %+v", got)
	}
	if math.Abs(got.center.X)+got.width/2 > board.Size.X/2+1e-12 || math.Abs(got.center.Z)+got.depth/2 > board.Size.Z/2+1e-12 {
		t.Fatal("tiny print surface escapes its original footprint")
	}
}

func TestHierarchyHeaderCompiledWorkspaceMargin(t *testing.T) {
	// Exact ELK dimensions from the unmodified Jupyter EKS fixture. Its two
	// lines occupy 206x41px; the source's 50px bottom margin has room for this
	// print area once the low base is projected at its actual height.
	board := d2isometric.Board{ID: "workspace", Position: nv(9.74, 0, 7.465), Size: nv(3.56, .14, 1.67)}
	owner := d2isometric.Node{Metadata: d2isometric.NodeMetadata{Original: d2target.Shape{LabelPosition: "INSIDE_TOP_CENTER", Text: d2target.Text{LabelWidth: 206, LabelHeight: 41}}}}
	surface := labelSurface{center: nv(0, .02846, 0), width: 2.06, depth: .41, align: "left"}
	nodes := []d2isometric.Node{
		{Opacity: 1, Position: nv(9.075, .14, 7.47), Size: nv(1.23, .14, .66)},
		{Opacity: 1, Position: nv(10.455, .14, 7.47), Size: nv(1.13, .14, .66)},
	}
	got, fits := hierarchyBoardHeaderPlacement(surface, board, owner, nodes, .01)
	if !fits || got.width != surface.width || got.depth != surface.depth || got.center.Y != surface.center.Y {
		t.Fatalf("source-sized two-line header did not fit: %+v, fit=%v", got, fits)
	}
	if got.center.Z < 8 || got.center.Z+got.depth/2 > 8.30 || got.center.Z-got.depth/2 < 7.83 {
		t.Fatalf("header is not in the clear source bottom margin: %+v", got)
	}
}
