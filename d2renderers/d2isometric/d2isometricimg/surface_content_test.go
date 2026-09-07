package d2isometricimg

import (
	"context"
	"math"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2target"
)

func TestMarkdownSurfaceUsesRaisedPaperWithinSourceAllocation(t *testing.T) {
	ctx := context.Background()
	rich, err := newRichLabelPainter(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	b := &meshBuilder{ctx: ctx, rich: rich, scale: .01}
	n := d2isometric.Node{ID: "note", Type: "text", Position: nv(0, .4, 0), Size: nv(3, .8, 2), Fill: "transparent", FillExplicit: true, Opacity: 1,
		Metadata: d2isometric.NodeMetadata{Original: d2target.Shape{Type: "text", Width: 300, Height: 200, Fill: "transparent", Text: d2target.Text{Label: "# Readable\n\n- First item\n- Second item", Language: "markdown", LabelWidth: 280, LabelHeight: 170, FontSize: 18}}}}
	b.node(n, "#849ebc")
	if b.err != nil {
		t.Fatal(b.err)
	}
	textures := 0
	for _, tri := range b.triangles {
		if tri.Material.Texture == nil || !tri.NoDepthWrite {
			continue
		}
		textures++
		for _, vertex := range tri.V {
			if math.Abs(vertex.Position.Y-(.115+.0015)) > 1e-9 {
				t.Fatalf("Markdown is not printed on the raised paper: %+v", vertex.Position)
			}
			if math.Abs(vertex.Position.X) > n.Size.X/2 || math.Abs(vertex.Position.Z) > n.Size.Z/2 {
				t.Fatal("surface content extends beyond the original layout footprint")
			}
		}
	}
	if textures != 2 {
		t.Fatalf("expected one two-triangle surface, got %d triangles", textures)
	}
}

func TestLaneSubdivisionPreservesCaptionPrintSize(t *testing.T) {
	neighbor := bridgeTestEdge("a", nv(0, .08, 0), nv(1.6, .08, 0))
	captioned := bridgeTestEdge("b", nv(0, .08, 0), nv(1.6, .08, 0))
	captioned.Label = "Store Task"
	captioned.Metadata.Original.Text = d2target.Text{Label: captioned.Label, LabelWidth: 130, LabelHeight: 30, FontSize: 24}
	width := func(edges []d2isometric.Edge) float64 {
		painter, err := newTextPainter(context.Background(), 1)
		if err != nil {
			t.Fatal(err)
		}
		b := &meshBuilder{ctx: context.Background(), text: painter, scale: .01}
		b.edges(edges, newRouteCaptionPlacer())
		if b.err != nil {
			t.Fatal(b.err)
		}
		for _, tri := range b.triangles {
			if tri.Material.Texture != nil && tri.V[1].U == 0 && tri.V[2].U == 1 {
				return nlen(nsub(tri.V[2].Position, tri.V[1].Position))
			}
		}
		t.Fatal("caption was not painted")
		return 0
	}
	before := width([]d2isometric.Edge{captioned})
	after := width([]d2isometric.Edge{neighbor, captioned})
	if math.Abs(before-after) > 1e-9 {
		t.Fatalf("adding a shared route shrank the caption from %f to %f", before, after)
	}
}
