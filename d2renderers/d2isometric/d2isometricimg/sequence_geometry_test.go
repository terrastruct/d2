package d2isometricimg

import (
	"context"
	"math"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2isometric"
)

func TestSequenceComponentsHaveSourceFootprintsAndPhysicalWalls(t *testing.T) {
	d := sourcePanelFixture(t, "stable/sequence_diagram_groups/dagre/board.exp.json")
	scene, err := d2isometric.BuildScene(d, nil)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for _, node := range scene.Nodes {
		if node.Container || node.SequenceRole == "" {
			continue
		}
		t.Run(node.ID, func(t *testing.T) {
			painter, err := newTextPainter(context.Background(), 1)
			if err != nil {
				t.Fatal(err)
			}
			b := &meshBuilder{ctx: context.Background(), scale: scene.PixelScale, text: painter}
			b.sequenceNode(node, "#7898ba")
			if b.err != nil {
				t.Fatal(b.err)
			}
			if len(b.panels) != 0 || len(b.triangles) == 0 {
				t.Fatal("sequence component is missing or became a diagram panel")
			}
			seen[node.SequenceRole]++
			lo, hi := nv(math.Inf(1), math.Inf(1), math.Inf(1)), nv(math.Inf(-1), math.Inf(-1), math.Inf(-1))
			for _, tri := range b.triangles {
				// Sidewalls are geometric, lit and untextured. Printed labels,
				// outlines and cap paint must not disguise a flat billboard.
				if tri.Material.Texture != nil || tri.Material.Unlit || math.Abs(tri.V[0].Normal.Y) > .5 {
					continue
				}
				for _, v := range tri.V {
					p := v.Position
					lo = nv(min(lo.X, p.X), min(lo.Y, p.Y), min(lo.Z, p.Z))
					hi = nv(max(hi.X, p.X), max(hi.Y, p.Y), max(hi.Z, p.Z))
				}
			}
			if node.SequenceRole == "group" {
				if !math.IsInf(lo.X, 1) {
					t.Fatal("message group became an opaque raised container")
				}
				return
			}
			if math.IsInf(lo.X, 1) || hi.Y-lo.Y < .15 {
				t.Fatal("actor, activation or note has no substantial sidewall")
			}
			if math.Abs(hi.X-lo.X-node.Size.X) > 1e-8 || math.Abs(hi.Z-lo.Z-node.Size.Z) > 1e-8 ||
				math.Abs((hi.X+lo.X)/2-node.Position.X) > 1e-8 || math.Abs((hi.Z+lo.Z)/2-node.Position.Z) > 1e-8 {
				t.Fatalf("source footprint moved: %+v to %+v / %+v", node, lo, hi)
			}
			if node.SequenceRole == "note" && lo.Y <= .10 || node.SequenceRole == "span" && (hi.Y < .28 || hi.Y >= .30) {
				t.Fatal("notes or activations no longer cover the lifeline")
			}
		})
	}
	for _, role := range []string{"actor", "span", "note", "group"} {
		if seen[role] == 0 {
			t.Fatalf("fixture did not exercise %s", role)
		}
	}
}

func TestSequenceActorOutsideLabelsKeepTheirReservedTimelineSpace(t *testing.T) {
	d := sourcePanelFixture(t, "stable/sequence_diagram_simple/dagre/board.exp.json")
	scene, err := d2isometric.BuildScene(d, nil)
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, node := range scene.Nodes {
		if node.Metadata.Original.LabelPosition != "OUTSIDE_BOTTOM_CENTER" {
			continue
		}
		painter, err := newTextPainter(context.Background(), 1)
		if err != nil {
			t.Fatal(err)
		}
		b := &meshBuilder{ctx: context.Background(), scale: scene.PixelScale, text: painter}
		b.sequenceNode(node, "#7898ba")
		if b.err != nil {
			t.Fatal(b.err)
		}
		var print []Triangle
		for _, tri := range b.triangles {
			if tri.NoDepthWrite && tri.Material.Texture != nil {
				print = append(print, tri)
			}
		}
		if len(print) < 2 {
			t.Fatal("actor caption missing")
		}
		actual := nativeMeshBounds(print[len(print)-2:])
		want := nativeNodeLabelSurface(node, nativeFaceSource(node, node.Fill), 0)
		if math.Abs(actual.X+actual.Width/2-want.center.X) > 1e-8 || math.Abs(actual.Y+actual.Height/2-want.center.Z) > 1e-8 ||
			math.Abs(actual.Width-want.width) > 1e-8 || math.Abs(actual.Height-want.depth) > 1e-8 {
			t.Fatalf("actor caption moved into its lifeline: %+v, want %+v", actual, want)
		}
		checked++
	}
	if checked != 2 {
		t.Fatal("fixture no longer exercises both outside actor captions")
	}
}

func TestDeepThreeDeeActivationsRemainBelowMessages(t *testing.T) {
	node := fidelityNode("rectangle")
	node.SequenceRole, node.Metadata.Original.Level, node.Metadata.Original.ThreeDee = "span", 128, true
	b := &meshBuilder{ctx: context.Background(), scale: .01}
	b.sequenceNode(node, "#7898ba")
	if b.err != nil {
		t.Fatal(b.err)
	}
	for _, tri := range b.triangles {
		for _, vertex := range tri.V {
			if vertex.Position.Y >= nativeSequenceMessageY {
				t.Fatal("nested activation would hide an interior message crossing")
			}
		}
	}
}
