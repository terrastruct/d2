package d2isometricimg

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2target"
)

func TestHierarchyStrongTerracesAreLocalToTheSourceTree(t *testing.T) {
	d := sourcePanelFixture(t, "regression/dagre_child_id_id/elk/board.exp.json")
	scene, err := d2isometric.BuildScene(d, &d2isometric.RenderOpts{})
	if err != nil {
		t.Fatal(err)
	}
	nodes := make(map[string]*d2isometric.Node)
	for i := range scene.Nodes {
		nodes[scene.Nodes[i].ID] = &scene.Nodes[i]
	}
	boards := hierarchyPresentationBoards(scene.Boards, nodes)
	bySource := make(map[string]d2isometric.Board)
	for _, board := range boards {
		bySource[board.SourceID] = board
	}
	if bySource["x"].Position.Y != 0 || bySource["y"].Position.Y > -.199 || bySource["y.z"].Position.Y != 0 || bySource["y.z"].Kind != "terrace" {
		t.Fatalf("strong levels changed an unrelated root or lost the nested wall: %+v", bySource)
	}
	if delta := hierarchySurfaceY(bySource["y.z"]) - hierarchySurfaceY(bySource["y"]); delta < .199 || delta > .201 {
		t.Fatalf("first source tier has no prominent wall: %g", delta)
	}
	for i := 0; i <= 130; i++ {
		if h := hierarchyTierHeight(i); h < 0 || h > hierarchyMaxDescent || i > 0 && h < hierarchyTierHeight(i-1) {
			t.Fatalf("unbounded/nonmonotonic tier %d: %g", i, h)
		}
	}
	owner, parent, child := nodes["y"], bySource["y"], bySource["y.z"]
	surface := labelSurface{center: nv(0, hierarchySurfaceY(parent)+.00006, 0), width: float64(owner.Metadata.Original.LabelWidth) * .01, depth: float64(owner.Metadata.Original.LabelHeight) * .01}
	headers := hierarchyRenderNodes(scene.Nodes, hierarchySupportOffsets(boards))
	placed, fits := hierarchyBoardHeaderPlacement(surface, parent, *owner, headers, .01, boards)
	if !fits || placed.width != surface.width || placed.depth != surface.depth || placed.center.Y != surface.center.Y {
		t.Fatalf("raised child lost the parent's allocated title: %+v", placed)
	}
	b := &meshBuilder{ctx: context.Background(), scale: .01}
	b.hierarchyBoard(child, nodes[child.SourceID], nodes[child.SourceID].Fill, 1)
	if b.err != nil {
		t.Fatal(b.err)
	}
	// Intersect the actual projected wall triangles, without the larger gap
	// reserved for automatic route captions. Headers use their source margin.
	var caption []svgPoint
	for _, offset := range []Vec{nv(-placed.width/2, 0, -placed.depth/2), nv(placed.width/2, 0, -placed.depth/2), nv(placed.width/2, 0, placed.depth/2), nv(-placed.width/2, 0, placed.depth/2)} {
		p := captionProjection(nadd(placed.center, offset))
		caption = append(caption, svgPoint{x: p.x, y: p.z})
	}
	for _, triangle := range b.triangles {
		if triangle.Material.Texture != nil {
			continue
		}
		var wall []svgPoint
		for _, vertex := range triangle.V {
			p := captionProjection(vertex.Position)
			wall = append(wall, svgPoint{x: p.x, y: p.z})
		}
		if svgPolygonArea(wall) < 0 {
			wall[0], wall[2] = wall[2], wall[0]
		}
		remaining := caption
		for i, a := range wall {
			remaining, _ = svgSplitPolygon(remaining, svgEdgeDistance(a, wall[(i+1)%len(wall)]))
		}
		if math.Abs(svgPolygonArea(remaining)) > 1e-10 {
			t.Fatal("raised nested wall still obscures the parent title")
		}
	}
}

func TestHierarchyHeaderSupportDoesNotInventExtraUpperObstacles(t *testing.T) {
	board := d2isometric.Board{ID: "parent", SourceID: "parent", Kind: "platform", Position: nv(0, 0, 0), Size: nv(8, .14, 6)}
	owner := d2isometric.Node{ID: "parent", Metadata: d2isometric.NodeMetadata{Original: d2target.Shape{LabelPosition: "INSIDE_TOP_CENTER"}}}
	child := d2isometric.Node{ID: "child", BoardID: "support", Type: d2target.ShapeDiamond, Position: nv(0, .42, -1.65), Size: nv(4, .7, 1.7), Opacity: 1, Fill: "#eeeeee"}
	surface := labelSurface{center: nv(0, .032, 0), width: 2, depth: .35}
	original := hierarchyRenderNodes([]d2isometric.Node{child})
	lowered := hierarchyRenderNodes([]d2isometric.Node{child}, map[string]float64{"support": -.6})
	want := hierarchyBoardHeaderSurface(surface, board, owner, original, .01)
	got := hierarchyBoardHeaderSurface(surface, board, owner, lowered, .01, []d2isometric.Board{{ID: "support", Kind: "ungrouped", Position: nv(0, -.6, 0)}})
	if got != want {
		t.Fatalf("extending a diamond below the title plane invented obstruction above it: %+v / %+v", got, want)
	}
	// A lower sequence ancestor must not be inflated above the next source
	// header plane. This reproduces the former artificial +.0005 cap bias.
	ancestor := d2isometric.Node{ID: "ancestor", Container: true, Opacity: 1, Fill: "white"}
	base := []d2isometric.Board{{ID: "ancestor", SourceID: "ancestor", Kind: "platform", Size: nv(10, .13, 8)}}
	got = hierarchyBoardHeaderSurface(surface, board, owner, append(original, ancestor), .01, base)
	if got != want {
		t.Fatal("a lower ancestor disabled the existing source header avoidance")
	}
}

func TestHierarchySupportKeepsEveryCapAndPrintSurface(t *testing.T) {
	for _, kind := range []string{d2target.ShapeRectangle, d2target.ShapePage, d2target.ShapeCloud, d2target.ShapePerson, d2target.ShapeCylinder, d2target.ShapeQueue} {
		t.Run(kind, func(t *testing.T) {
			s := d2target.BaseShape()
			s.ID, s.Type, s.Width, s.Height = "shape", kind, 200, 140
			s.Fill, s.Stroke, s.Multiple, s.ThreeDee = "#cdd7eb", "#213b57", true, true
			s.Label, s.LabelWidth, s.LabelHeight, s.FontSize, s.LabelPosition = "caption", 55, 20, 16, "INSIDE_MIDDLE_CENTER"
			n := d2isometric.Node{ID: s.ID, BoardID: "lower", Type: kind, Label: s.Label, Opacity: 1, Fill: s.Fill, Stroke: s.Stroke, FontColor: "#213b57", FillExplicit: true, StrokeExplicit: true, StrokeWidth: s.StrokeWidth,
				Position: nv(2, .42, 3), Size: nv(2, .7, 1.4), Metadata: d2isometric.NodeMetadata{Original: *s}}
			build := func(drop float64) *meshBuilder {
				p, err := newTextPainter(context.Background(), 1)
				if err != nil {
					t.Fatal(err)
				}
				b := &meshBuilder{ctx: context.Background(), scale: .01, text: p, hierarchySupports: map[string]float64{"lower": drop}}
				b.hierarchyNode(n, "")
				if b.err != nil {
					t.Fatal(b.err)
				}
				if b.nodeSupportDrop != 0 {
					t.Fatal("one shape's support offset leaked into the next shape")
				}
				return b
			}
			before, lowered := build(0), build(-.4)
			body := func(b *meshBuilder) []Triangle {
				var out []Triangle
				for _, tr := range b.triangles {
					if !tr.Material.Unlit {
						out = append(out, tr)
					}
				}
				return out
			}
			a, b := body(before), body(lowered)
			if len(a) != len(b) || len(a) == 0 {
				t.Fatal("support extension changed physical tessellation")
			}
			loA, loB, caps := math.Inf(1), math.Inf(1), 0
			for i, triangle := range a {
				flat := true
				for _, v := range triangle.V {
					flat = flat && math.Abs(v.Position.Y-triangle.V[0].Position.Y) < 1e-10
				}
				for j, vertex := range triangle.V {
					p, q := vertex.Position, b[i].V[j].Position
					if p.X != q.X || p.Z != q.Z {
						t.Fatal("support extension changed the source footprint")
					}
					loA, loB = min(loA, p.Y), min(loB, q.Y)
					if flat && p.Y > .1 {
						caps++
						if math.Abs(p.Y-q.Y) > 1e-10 {
							t.Fatal("support extension moved a main or multiple cap")
						}
					}
				}
			}
			if math.Abs(loA-loB-.4) > 1e-10 || caps == 0 {
				t.Fatalf("body did not extend to the lower support: %g/%g, caps=%d", loA, loB, caps)
			}
			print := func(b *meshBuilder) [][3]Vertex {
				var out [][3]Vertex
				for _, tr := range b.triangles {
					if tr.Material.Texture != nil && tr.Material.Unlit && tr.V[0].Normal.Y > .99 {
						out = append(out, tr.V)
					}
				}
				return out
			}
			if p, q := print(before), print(lowered); len(p) == 0 || !reflect.DeepEqual(p, q) {
				t.Fatal("source caption or flat ink allocation moved")
			}
		})
	}
}

func TestHierarchySupportExtendsOnlyStructuredBacking(t *testing.T) {
	for _, fixture := range []string{"stable/class/dagre/board.exp.json", "stable/sql_table_row_connections/elk/board.exp.json"} {
		scene, nodes := structuredFixtureNodes(t, fixture)
		for _, n := range nodes {
			before, lower := structuredTestBuilder(t, scene), structuredTestBuilder(t, scene)
			lower.hierarchySupports = map[string]float64{n.BoardID: -.4}
			before.hierarchyNode(n, "")
			lower.hierarchyNode(n, "")
			if before.err != nil || lower.err != nil {
				t.Fatalf("structured extension failed: %v/%v", before.err, lower.err)
			}
			floor := n.Position.Y - n.Size.Y/2
			rows := func(b *meshBuilder) [][3]Vertex {
				var out [][3]Vertex
				for _, triangle := range b.triangles {
					above := true
					for _, v := range triangle.V {
						above = above && v.Position.Y > floor+.02
					}
					if above {
						out = append(out, triangle.V)
					}
				}
				return out
			}
			if a, b := rows(before), rows(lower); len(a) == 0 || !reflect.DeepEqual(a, b) {
				t.Fatal("lowering the backing altered row caps, walls, cells or row ports")
			}
		}
	}
}
