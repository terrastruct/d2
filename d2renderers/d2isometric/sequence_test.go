package d2isometric

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2sequence"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/geo"
)

func sequenceTestLifeline(id, actor, local string) d2target.Connection {
	edge := testEdge(id, actor, d2sequence.LifelineEndID(local))
	edge.ZIndex = d2sequence.LIFELINE_Z_INDEX
	edge.SrcArrow, edge.DstArrow = d2target.NoArrowhead, d2target.NoArrowhead
	edge.Label, edge.StrokeDash = "", 6
	edge.Route = []*geo.Point{{X: 130, Y: 80}, {X: 130, Y: 600}}
	return edge
}

func TestSequenceRolesPreserveQuotedAuthoredTimeline(t *testing.T) {
	for _, local := range []string{`actor`, `"actor.with.dots"`, `"actor\"with quote"`} {
		t.Run(local, func(t *testing.T) {
			d := d2target.NewDiagram()
			container := testNode(`"outer.scope"`, -200, -100)
			container.Type, container.Width, container.Height = d2target.ShapeSequenceDiagram, 1000, 1200
			actor := testNode(container.ID+"."+local, 80, 10)
			span := testNode(actor.ID+".activation", 124, 180)
			span.ZIndex, span.Width, span.Height, span.Label = d2sequence.SPAN_Z_INDEX, 12, 330, ""
			nested := testNode(span.ID+".nested", 120, 250)
			nested.ZIndex, nested.Width, nested.Height, nested.Label = d2sequence.SPAN_Z_INDEX, 20, 110, ""
			note := testNode(span.ID+".note", 60, 420)
			note.ZIndex, note.Type = d2sequence.NOTE_Z_INDEX, d2target.ShapePage
			group := testNode(container.ID+".group", 40, 150)
			group.ZIndex, group.Blend = d2sequence.GROUP_Z_INDEX, true
			// These unrelated objects deliberately have matching paint indices.
			ordinary := testNode("ordinary", 1200, -300)
			ordinary.ZIndex = d2sequence.SPAN_Z_INDEX
			ordinaryGroup := testNode("ordinaryGroup", 1400, -300)
			ordinaryGroup.ZIndex, ordinaryGroup.Blend = d2sequence.GROUP_Z_INDEX, true
			d.Shapes = []d2target.Shape{nested, ordinary, note, group, span, actor, ordinaryGroup, container}
			message := testEdge("message", span.ID, nested.ID)
			message.ZIndex, message.Animated = d2sequence.MESSAGE_Z_INDEX, true
			message.Route = []*geo.Point{{X: 136, Y: 240}, {X: 310, Y: 240}, {X: 310, Y: 315}, {X: 140, Y: 315}}
			message.LabelPosition, message.LabelPercentage = "INSIDE_MIDDLE_CENTER", .42
			message.Link, message.Tooltip = "https://example.test/message", "source timing"
			ordinaryEdge := testEdge("ordinaryEdge", ordinary.ID, ordinaryGroup.ID)
			ordinaryEdge.ZIndex = d2sequence.MESSAGE_Z_INDEX
			cross := testEdge("cross", span.ID, ordinary.ID)
			cross.ZIndex = d2sequence.MESSAGE_Z_INDEX
			d.Connections = []d2target.Connection{message, sequenceTestLifeline("lifeline", actor.ID, local), ordinaryEdge, cross}
			before := jsonBytes(t, d)
			scene, err := BuildScene(d, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !scene.HasSequence {
				t.Fatal("sequence took the default presentation repacking path")
			}
			wantRoles := []string{"span", "", "note", "group", "span", "actor", "", "container"}
			for i, node := range scene.Nodes {
				if node.SequenceRole != wantRoles[i] || node.Container != (node.ID == container.ID) {
					t.Fatalf("incorrect role/container for %s: %q, %v", node.ID, node.SequenceRole, node.Container)
				}
				if node.SequenceRole != "" && node.BoardID != "@container:"+container.ID {
					t.Fatalf("%s did not skip its semantic parents when finding a board: %s", node.ID, node.BoardID)
				}
				original := d.Shapes[i]
				if node.Position.X != (float64(original.Pos.X)+float64(original.Width)/2)*SceneScale || node.Position.Z != (float64(original.Pos.Y)+float64(original.Height)/2)*SceneScale || node.Size.X != float64(original.Width)*SceneScale || node.Size.Z != float64(original.Height)*SceneScale {
					t.Fatal("role assignment changed a source footprint")
				}
				if !reflect.DeepEqual(node.Metadata.Original, original) {
					t.Fatal("role assignment altered original shape metadata")
				}
			}
			if scene.Nodes[0].ParentID != span.ID || scene.Nodes[2].ParentID != span.ID || scene.Nodes[4].ParentID != actor.ID {
				t.Fatal("semantic parent identity was discarded")
			}
			if len(scene.Boards) != 2 {
				t.Fatalf("activation or note created a fictitious physical container: %#v", scene.Boards)
			}
			for i, edge := range scene.Edges {
				if edge.SequenceRole != []string{"message", "lifeline", "", ""}[i] {
					t.Fatalf("edge %s escaped its confirmed sequence scope: %q", edge.ID, edge.SequenceRole)
				}
				original := d.Connections[i]
				if !reflect.DeepEqual(edge.Metadata.Original, original) || len(edge.Points) != len(original.Route) {
					t.Fatal("sequence roles altered source messages or routes")
				}
				for j, point := range edge.Points {
					if point != (Vec3{original.Route[j].X * SceneScale, surfaceHeight, original.Route[j].Y * SceneScale}) {
						t.Fatal("self/span loop bend or source message timing changed")
					}
				}
			}
			if !bytes.Equal(before, jsonBytes(t, d)) {
				t.Fatal("BuildScene changed the input diagram")
			}
		})
	}
}

func TestSequenceLifelinesRequireExactSyntheticIdentity(t *testing.T) {
	for _, mode := range []string{"valid", "wrong-hash", "real-target", "wrong-z", "arrow"} {
		t.Run(mode, func(t *testing.T) {
			d := d2target.NewDiagram()
			d.Shapes = []d2target.Shape{testNode(`"actor.with.dots"`, 0, 0)}
			edge := sequenceTestLifeline("lifeline", d.Shapes[0].ID, d.Shapes[0].ID)
			switch mode {
			case "wrong-hash":
				// The decoded ID is not the syntax ID used by the layout.
				edge.Dst = d2sequence.LifelineEndID("actor.with.dots")
			case "real-target":
				// Use a simple actor so the colliding destination is valid D2 syntax.
				d.Shapes[0].ID = "actor"
				edge = sequenceTestLifeline("lifeline", "actor", "actor")
				d.Shapes = append(d.Shapes, testNode(edge.Dst, 10, 100))
			case "wrong-z":
				edge.ZIndex = 0
			case "arrow":
				edge.DstArrow = d2target.TriangleArrowhead
			}
			d.Connections = []d2target.Connection{edge}
			scene, err := BuildScene(d, &RenderOpts{})
			if err != nil {
				t.Fatal(err)
			}
			if scene.HasSequence != (mode == "valid") || (scene.Edges[0].SequenceRole == "lifeline") != (mode == "valid") {
				t.Fatal("ordinary connection mistaken for a compiler lifeline")
			}
		})
	}
}

func TestSequenceRealMixedActorSubgraphsAndSemanticChildren(t *testing.T) {
	for _, fixture := range []string{"nested_diagram_types", "sequence_diagram_nested_span", "sequence_diagram_nested_groups", "sequence-inter-span-self"} {
		t.Run(fixture, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "..", "e2etests", "testdata", "stable", fixture, "dagre", "board.exp.json"))
			if err != nil {
				t.Fatal(err)
			}
			var diagram d2target.Diagram
			if err := json.Unmarshal(data, &diagram); err != nil {
				t.Fatal(err)
			}
			scene, err := BuildScene(&diagram, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !scene.HasSequence {
				t.Fatal("real compiled sequence was not recognized")
			}
			byID := map[string]Node{}
			for i, node := range scene.Nodes {
				byID[node.ID] = node
				if sequenceSemanticChild(node.SequenceRole) && node.Container {
					t.Fatal("semantic sequence child became a physical container")
				}
				if !reflect.DeepEqual(node.Metadata.Original, diagram.Shapes[i]) {
					t.Fatal("real compiled source metadata changed")
				}
			}
			for _, edge := range scene.Edges {
				if edge.Metadata.Original.ZIndex == d2sequence.LIFELINE_Z_INDEX && edge.SequenceRole != "lifeline" || edge.Metadata.Original.ZIndex == d2sequence.MESSAGE_Z_INDEX && edge.SequenceRole != "message" {
					t.Fatalf("real compiled message/lifeline not recognized: %+v", edge)
				}
			}
			if fixture != "nested_diagram_types" {
				if len(scene.Boards) != 1 || scene.Boards[0].Kind != "ungrouped" {
					t.Fatal("pure sequence acquired an invented actor/group baseboard")
				}
				return
			}
			if byID["b.1"].Container || byID["b.1"].SequenceRole != "actor" || !byID["b.2"].Container || byID["b.2"].SequenceRole != "actor" {
				t.Fatal("mixed sequence failed to distinguish activation ownership from an ordinary actor subgraph")
			}
			for _, id := range []string{"b.2.x", "b.2.y", "b.2.z"} {
				if byID[id].SequenceRole != "" || byID[id].BoardID != "@container:b.2" {
					t.Fatalf("ordinary actor content %s lost its physical container", id)
				}
			}
			if byID["b.1.x"].BoardID != "@container:b" || byID["b.1.x.u"].BoardID != "@container:b" {
				t.Fatal("nested activation/note retained an actor or activation baseboard")
			}
		})
	}
}
