package d2isometricimg

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image/color"
	"math"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2isometric"
)

func TestHierarchyTerracesKeepCompiledLayoutAndExposeNestedWall(t *testing.T) {
	diagram := sourcePanelFixture(t, "regression/dagre_child_id_id/elk/board.exp.json")
	scene, err := d2isometric.BuildScene(diagram, &d2isometric.RenderOpts{})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(scene)
	nodes := map[string]*d2isometric.Node{}
	for i := range scene.Nodes {
		nodes[scene.Nodes[i].ID] = &scene.Nodes[i]
	}
	boards := hierarchyPresentationBoards(scene.Boards, nodes)
	var parent, child d2isometric.Board
	for i, board := range boards {
		original := scene.Boards[i]
		if board.Position.X != original.Position.X || board.Position.Z != original.Position.Z || board.Size.X != original.Size.X || board.Size.Z != original.Size.Z || board.SourceID != original.SourceID || board.ParentID != original.ParentID {
			t.Fatal("terrace changed source footprint or containment")
		}
		if board.SourceID == "y" {
			parent = board
		}
		if board.SourceID == "y.z" {
			child = board
		}
	}
	bottom, top := hierarchySurfaceY(parent), hierarchySurfaceY(child)
	if top-bottom < .199 || top-bottom > .201 || top >= .0608 || math.Abs(top-child.Size.Y-bottom) > 1e-12 {
		t.Fatalf("first terrace does not expose a wall below the flat route casing: bottom=%g top=%g thickness=%g", bottom, top, child.Size.Y)
	}
	b := &meshBuilder{ctx: nativeVectorContext(context.Background()), scale: .01}
	owner := nodes[child.SourceID]
	b.hierarchyBoard(child, owner, owner.Fill, owner.Opacity)
	if b.err != nil {
		t.Fatal(b.err)
	}
	walls, caps := 0, 0
	for _, triangle := range b.triangles {
		m := triangle.Material
		if m.Texture != nil {
			caps++
			bounds := m.Texture.Bounds()
			paint := color.NRGBAModel.Convert(m.Texture.At(bounds.Min.X+bounds.Dx()/2, bounds.Min.Y+bounds.Dy()/2)).(color.NRGBA)
			if paint != nativePaint(owner.Fill, "transparent") {
				t.Fatalf("physical nested cap lost source fill: %v", paint)
			}
			for _, v := range triangle.V {
				if math.Abs(v.Position.Y-top) > 1e-12 {
					t.Fatal("nested cap and header plane disagree")
				}
			}
			continue
		}
		walls++
		for _, v := range triangle.V {
			if math.Abs(v.Position.Y-bottom) > 1e-12 && math.Abs(v.Position.Y-top) > 1e-12 {
				t.Fatal("nested sidewall is buried inside its parent")
			}
		}
	}
	if walls == 0 || caps == 0 {
		t.Fatal("nested container has no physical wall and cap")
	}
	translucent := *owner
	translucent.Fill, translucent.Opacity = "rgba(32,64,96,0.5)", .4
	alpha := &meshBuilder{ctx: context.Background(), scale: .01}
	alpha.hierarchyBoard(child, &translucent, translucent.Fill, translucent.Opacity)
	if alpha.err != nil {
		t.Fatal(alpha.err)
	}
	for _, triangle := range alpha.triangles {
		m := triangle.Material
		if m.Texture != nil {
			bounds := m.Texture.Bounds()
			paint := color.NRGBAModel.Convert(m.Texture.At(bounds.Min.X+bounds.Dx()/2, bounds.Min.Y+bounds.Dy()/2)).(color.NRGBA)
			if paint.A != 128 || m.Color.A != 102 {
				t.Fatalf("physical nested cap gained an artificial wash: source alpha=%d object alpha=%d", paint.A, m.Color.A)
			}
		} else if m.Color.A != 51 {
			t.Fatalf("nested wall lost authored fill/object opacity: %d", m.Color.A)
		}
	}
	for _, edge := range scene.Edges {
		for _, point := range edge.Points {
			if point.Y != .08 {
				t.Fatal("terrace changed the flat route plane")
			}
		}
	}
	after, _ := json.Marshal(scene)
	if !bytes.Equal(before, after) {
		t.Fatal("presentation relief mutated the compiled scene")
	}
}

func TestHierarchyTerracesBoundDeepNestingAndPreserveRegions(t *testing.T) {
	boards := make([]d2isometric.Board, 130)
	nodes := make(map[string]*d2isometric.Node, len(boards))
	for i := range boards {
		id := fmt.Sprint(i)
		boards[i] = d2isometric.Board{ID: id, SourceID: id, Kind: "group", Level: i, Size: nv(3, .14, 2)}
		if i == 0 {
			boards[i].Kind = "platform"
		} else {
			boards[i].ParentID = fmt.Sprint(i - 1)
		}
		nodes[id] = &d2isometric.Node{Label: id, Fill: "#c9d3ef", Opacity: 1, StrokeWidth: 2, Stroke: "#123456"}
	}
	deep := hierarchyPresentationBoards(boards, nodes)
	for i, board := range deep {
		top := hierarchySurfaceY(board)
		if top > .041 || top >= .07 || top < -.6 {
			t.Fatalf("deep container intersects the common component plane: level=%d top=%g", i, top)
		}
		if i > 0 && (top < hierarchySurfaceY(deep[i-1]) || !hierarchyPhysicalPlate(board, nodes[board.SourceID], 255)) {
			t.Fatal("deep source plate lost monotonic depth or source fill")
		}
	}
	for _, kind := range []string{"wrapper", "dashed", "transparent"} {
		t.Run(kind, func(t *testing.T) {
			owner := *nodes["1"]
			switch kind {
			case "wrapper":
				owner.Label, owner.Fill, owner.Stroke = "", "transparent", "transparent"
			case "dashed":
				owner.StrokeDash = 5
			case "transparent":
				owner.Fill = "transparent"
			}
			local := map[string]*d2isometric.Node{"0": nodes["0"], "1": &owner, "2": nodes["2"]}
			result := hierarchyPresentationBoards(boards[:3], local)
			if hierarchyPhysicalPlate(result[1], &owner, nativePaint(owner.Fill, "transparent").A) {
				t.Fatal("layout or semantic region became a solid plate")
			}
			if rise := hierarchySurfaceY(result[2]) - hierarchySurfaceY(result[0]); rise < .199 || rise > .201 {
				t.Fatalf("nonphysical wrapper consumed a terrace step: %g", rise)
			}
		})
	}
}

func TestHierarchyTerracesKeepSequenceBackgroundPlanes(t *testing.T) {
	nodes := map[string]*d2isometric.Node{
		"root":        {Label: "root", Fill: "#eef0fc", Opacity: 1},
		"ordinary":    {Label: "ordinary", Fill: "#eef0fc", Opacity: 1},
		"sequence":    {Label: "sequence", Fill: "#ffffff", Opacity: 1, SequenceRole: "container"},
		"actor-child": {Label: "actor-child", Fill: "#eef0fc", Opacity: 1},
		"sibling":     {Label: "sibling", Fill: "#eef0fc", Opacity: 1},
	}
	boards := []d2isometric.Board{
		{ID: "root", SourceID: "root", Kind: "platform"},
		{ID: "ordinary", SourceID: "ordinary", ParentID: "root", Kind: "group", Level: 1},
		{ID: "sequence", SourceID: "sequence", ParentID: "ordinary", Kind: "group", Level: 2},
		{ID: "actor-child", SourceID: "actor-child", ParentID: "sequence", Kind: "group", Level: 3},
		{ID: "sibling", SourceID: "sibling", ParentID: "root", Kind: "group", Level: 1},
	}
	for i := range boards {
		boards[i].Size = nv(3, .14, 2)
	}
	result := hierarchyPresentationBoards(boards, nodes)
	for _, board := range result[:4] {
		if hierarchySurfaceY(board) != hierarchyBaseSurfaceY(board) {
			t.Fatal("new container relief buried a sequence background")
		}
	}
	if result[4].Position.Y <= 0 {
		t.Fatal("a sequence disabled relief on an independent ordinary branch")
	}
	semantic := d2isometric.Board{Kind: "sequence-group", Level: 4, Position: nv(0, .6, 0)}
	if hierarchySurfaceY(semantic) != hierarchyBaseSurfaceY(semantic) {
		t.Fatal("sequence annotation body elevation became its background plane")
	}
}
