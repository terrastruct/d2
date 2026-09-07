package d2isometricimg

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"reflect"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2renderers/d2scenebuild"
	"github.com/d2lang/d2/d2target"
)

func markdownCardTestNode() d2isometric.Node {
	n := fidelityNode(d2target.ShapeText)
	n.ID, n.BoardID, n.Label = "notes", "parent", "**Read** the [guide](https://example.test/guide)"
	n.Fill, n.Stroke = "transparent", "#263c4e"
	n.Metadata.Original.ID, n.Metadata.Original.Fill, n.Metadata.Original.Stroke = n.ID, n.Fill, "N1"
	n.Metadata.Original.Text = d2target.Text{Label: n.Label, Language: "markdown", FontSize: 16, LabelWidth: 180, LabelHeight: 70, Color: "N1"}
	n.Metadata.Original.LabelPosition = "INSIDE_MIDDLE_CENTER"
	return n
}

func markdownCardTestBuilder(t *testing.T) *meshBuilder {
	t.Helper()
	ctx := context.Background()
	rich, err := newRichLabelPainter(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	icons, err := newSurfaceIconPainter(ctx, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	return &meshBuilder{ctx: ctx, scale: .01, rich: rich, icons: icons, faceMaxPixels: 128 << 10,
		options: nativeSceneOptions{links: &d2scenebuild.LinkBudget{MaxRegions: 20, MaxStringBytes: 4096}}}
}

func markdownCardTestDecals(b *meshBuilder) []Triangle {
	var out []Triangle
	for _, triangle := range b.triangles {
		if triangle.NoDepthWrite && triangle.Material != nil && triangle.Material.Texture != nil {
			out = append(out, triangle)
		}
	}
	return out
}

func TestMarkdownCardsKeepSourceFootprintsAndAttachToSupport(t *testing.T) {
	for _, tiny := range []bool{false, true} {
		for _, hierarchy := range []bool{false, true} {
			n := markdownCardTestNode()
			if tiny {
				n.Size.X, n.Size.Z = .01, .01
				n.Metadata.Original.Width, n.Metadata.Original.Height = 1, 1
				n.Label, n.Metadata.Original.Label = "", ""
				n.Metadata.Original.LabelWidth, n.Metadata.Original.LabelHeight = 1, 1
			}
			before, _ := json.Marshal(n)
			b := markdownCardTestBuilder(t)
			support := 0.
			if hierarchy {
				support = -.12
				b.hierarchySupports = map[string]float64{n.BoardID: support}
				b.hierarchyNode(n, "")
			} else {
				b.node(n, "")
			}
			if b.err != nil {
				t.Fatal(b.err)
			}
			physical := reliefTestWalls(b.triangles)
			lo, hi := solidTestBounds(physical)
			floor := n.Position.Y - n.Size.Y/2
			wantLo, wantHi := nv(n.Position.X-n.Size.X/2, floor+support, n.Position.Z-n.Size.Z/2), nv(n.Position.X+n.Size.X/2, floor+.115, n.Position.Z+n.Size.Z/2)
			if nlen(nsub(lo, wantLo)) > 1e-9 || nlen(nsub(hi, wantHi)) > 1e-9 {
				t.Fatalf("tiny=%v hierarchy=%v: physical allocation %v to %v, want %v to %v", tiny, hierarchy, lo, hi, wantLo, wantHi)
			}
			for _, triangle := range physical {
				for _, v := range triangle.V {
					if !captionFinite(v.Position.X, v.Position.Y, v.Position.Z) || math.Abs(nlen(v.Normal)-1) > 1e-9 {
						t.Fatal("tiny card produced invalid geometry")
					}
				}
			}
			after, _ := json.Marshal(n)
			if !bytes.Equal(before, after) {
				t.Fatal("presentation mutated the compiled node")
			}
		}
	}
}

func TestMarkdownCardsRaiseExistingDocumentIconsAndLinksTogether(t *testing.T) {
	for _, position := range []string{"INSIDE_MIDDLE_CENTER", "OUTSIDE_BOTTOM_CENTER"} {
		n := markdownCardTestNode()
		n.Metadata.Original.LabelPosition = position
		n.Metadata.Original.Icon = iconData(t, "image/svg+xml", []byte(surfaceTestSVG))
		n.Icon = n.Metadata.Original.Icon.String()
		before := markdownCardTestBuilder(t)
		floor := n.Position.Y - n.Size.Y/2
		before.canonicalNodeContent(n, nativeFaceSource(n, n.Fill), n.Fill, floor+.018)
		after := markdownCardTestBuilder(t)
		after.hierarchyNode(n, "")
		if before.err != nil || after.err != nil {
			t.Fatal(before.err, after.err)
		}
		original, raised := markdownCardTestDecals(before), markdownCardTestDecals(after)
		if len(original) != 4 || len(raised) != len(original) {
			t.Fatal("the complete icon and Markdown document were not retained")
		}
		for i, triangle := range original {
			for j, v := range triangle.V {
				want := v
				want.Position.Y += .115 - .018
				if nlen(nsub(raised[i].V[j].Position, want.Position)) > 1e-9 || raised[i].V[j].U != want.U || raised[i].V[j].V != want.V {
					t.Fatal("raising the paper moved or resized source print coordinates")
				}
			}
			if !reflect.DeepEqual(triangle.Material.Texture, raised[i].Material.Texture) {
				t.Fatal("the source Markdown or icon was reflowed or repainted")
			}
		}
		if len(before.links) != 1 || len(after.links) != 1 || after.links[0].region.URL != before.links[0].region.URL {
			t.Fatal("Markdown document link was lost")
		}
		for i, point := range before.links[0].points {
			point.Y += .115 - .018
			if nlen(nsub(after.links[0].points[i], point)) > 1e-9 {
				t.Fatal("link hit region does not follow the raised printed face")
			}
		}
	}
}

func TestMarkdownCardsKeepAuthoredPaintModifiersAndOpacity(t *testing.T) {
	for _, opacity := range []float64{1, .4, 0} {
		n := markdownCardTestNode()
		n.Fill, n.Stroke, n.StrokeWidth, n.StrokeDash = "#ceedee", "#af2670", 4, 3
		n.StrokeExplicit, n.Opacity = true, opacity
		n.Metadata.Original.Fill, n.Metadata.Original.Stroke = n.Fill, n.Stroke
		n.Metadata.Original.StrokeWidth, n.Metadata.Original.StrokeDash = n.StrokeWidth, n.StrokeDash
		n.Metadata.Original.BorderRadius, n.Metadata.Original.FillPattern = 18, "lines"
		n.Metadata.Original.Multiple, n.Metadata.Original.ThreeDee, n.Metadata.Original.DoubleBorder, n.Metadata.Original.Animated = true, true, true, true
		b := markdownCardTestBuilder(t)
		b.hierarchyNode(n, "")
		if b.err != nil {
			t.Fatal(b.err)
		}
		if opacity == 0 {
			if len(b.triangles) != 0 || b.rich.used != 0 {
				t.Fatal("invisible card allocated rendered content")
			}
			continue
		}
		if b.facePixels == 0 || b.rich.used != 1 {
			t.Fatal("authored face decorations or Markdown bypassed their native renderer")
		}
		if len(b.animatedNodes) != 1 || b.animatedNodes[0].first != 0 || b.animatedNodes[0].last != len(b.triangles) {
			t.Fatal("animation does not include the entire paper, ink and document")
		}
		physical := reliefTestWalls(b.triangles)
		lo, hi := solidTestBounds(physical)
		if math.Abs(hi.X-(n.Position.X+n.Size.X/2+d2target.MULTIPLE_OFFSET*.01)) > 1e-9 || math.Abs(lo.Z-(n.Position.Z-n.Size.Z/2-d2target.MULTIPLE_OFFSET*.01)) > 1e-9 {
			t.Fatal("multiple paper lost its source copy offset")
		}
		if math.Abs(hi.Y-(n.Position.Y-n.Size.Y/2+.115+d2target.THREE_DEE_OFFSET*.01)) > 1e-9 {
			t.Fatal("authored 3D depth was lost")
		}
		for _, triangle := range physical {
			if triangle.Material.Color != nativePaint(n.Fill, "") {
				t.Fatal("authored paper paint was replaced by the default palette")
			}
		}
		var group *nativeOpacityGroup
		for _, triangle := range b.triangles {
			if opacity < 1 {
				if triangle.OpacityGroup == nil || triangle.OpacityGroup.Opacity != opacity || group != nil && triangle.OpacityGroup != group {
					t.Fatal("paper, ink and document do not share one opacity composition")
				}
				group = triangle.OpacityGroup
			} else if triangle.OpacityGroup != nil {
				t.Fatal("opaque paper acquired an opacity group")
			}
		}
	}
}

func TestMarkdownCardsDoNotChangeOtherTextSurfaces(t *testing.T) {
	for _, kind := range []string{"container", "sequence", "plain", "latex", "code"} {
		n := markdownCardTestNode()
		switch kind {
		case "container":
			n.Container = true
		case "sequence":
			n.SequenceRole = "note"
		case "plain":
			n.Metadata.Original.Language = ""
		case "latex":
			n.Metadata.Original.Language = "latex"
		case "code":
			n.Type = d2target.ShapeCode
		}
		if nativeMarkdownCard(n) {
			t.Fatalf("%s acquired a Markdown backing", kind)
		}
	}
}

func TestMarkdownCardHeaderAvoidsRaisedPaper(t *testing.T) {
	board := d2isometric.Board{ID: "parent", Size: nv(4, .14, 4)}
	owner := d2isometric.Node{Metadata: d2isometric.NodeMetadata{Original: d2target.Shape{LabelPosition: "INSIDE_TOP_CENTER"}}}
	header := labelSurface{center: nv(0, .032, 0), width: 2, depth: .4, align: "left"}
	n := markdownCardTestNode()
	n.Position.X, n.Position.Z = 0, -1.28
	n.Size.X, n.Size.Z = 2.5, .3
	n.Metadata.Original.Width, n.Metadata.Original.Height = 250, 30
	n.Label, n.Metadata.Original.Label = "", ""
	n.Metadata.Original.LabelWidth, n.Metadata.Original.LabelHeight = 250, 30
	if !n.FillExplicit || nativePaint(n.Fill, "").A != 0 {
		t.Fatal("regression requires the compiler's explicitly transparent Markdown body")
	}
	b := markdownCardTestBuilder(t)
	b.hierarchyNode(n, "")
	if b.err != nil {
		t.Fatal(b.err)
	}
	lo, hi := routeCaptionPoint{math.Inf(1), math.Inf(1)}, routeCaptionPoint{math.Inf(-1), math.Inf(-1)}
	for _, triangle := range reliefTestWalls(b.triangles) {
		for _, v := range triangle.V {
			p := captionProjection(v.Position)
			lo.x, lo.z = min(lo.x, p.x), min(lo.z, p.z)
			hi.x, hi.z = max(hi.x, p.x), max(hi.z, p.z)
		}
	}
	overlaps := func(s labelSurface) bool {
		p := captionProjection(s.center)
		return p.x+s.width/2 > lo.x && p.x-s.width/2 < hi.x && p.z+s.depth/2 > lo.z && p.z-s.depth/2 < hi.z
	}
	initial := hierarchyBoardHeaderSurface(header, board, owner, nil, .01)
	if !overlaps(initial) {
		t.Fatal("fixture no longer exposes a header behind the paper")
	}
	placed, fits := hierarchyBoardHeaderPlacement(header, board, owner, hierarchyRenderNodes([]d2isometric.Node{n}), .01)
	if !fits || overlaps(placed) || placed.width != header.width || placed.depth != header.depth || placed.center.Y != header.center.Y || placed.angle != header.angle {
		t.Fatalf("full source header remains behind the raised paper: %+v, fits=%v", placed, fits)
	}
}

func TestMarkdownCardHeaderUsesClearSourceBottomMargin(t *testing.T) {
	// The Lion Reader Dagre cache_helpers container has 30px margins around
	// a 148px document and a 31px title. The recessed support adds 4.5px of
	// visible clearance to the lower margin. Preserve the whole title on the
	// parent plate without moving its document or crossing an ancestor rim.
	board := d2isometric.Board{ID: "parent", SourceID: "parent", Size: nv(3.47, .14, 2.08)}
	owner := d2isometric.Node{ID: "parent", Container: true, Metadata: d2isometric.NodeMetadata{Original: d2target.Shape{Width: 347, Height: 208, LabelPosition: "OUTSIDE_TOP_CENTER"}}}
	header := labelSurface{center: nv(0, .066, 0), width: 1.43, depth: .31, align: "left"}
	n := markdownCardTestNode()
	n.ParentID, n.Position.X, n.Position.Z = owner.ID, .15, 0
	n.Size.X, n.Size.Z = 2.57, 1.48
	n.Metadata.Original.Width, n.Metadata.Original.Height = 257, 148
	n.Metadata.Original.Pos = d2target.Point{X: 60, Y: 30}
	n.Label, n.Metadata.Original.Label = "", ""
	n.Metadata.Original.LabelWidth, n.Metadata.Original.LabelHeight = 257, 148
	before := n
	placed, fits := hierarchyBoardHeaderPlacement(header, board, owner, hierarchyRenderNodes([]d2isometric.Node{n}), .01)
	if !fits || placed.width != header.width || placed.depth != header.depth || placed.center.Y != header.center.Y || placed.angle != header.angle {
		t.Fatalf("source title was not retained at full print size: %+v, fits=%v", placed, fits)
	}
	if placed.center.Z-header.depth/2 <= 0 || placed.center.Z+header.depth/2 > board.Size.Z/2-.02+1e-9 {
		t.Fatalf("title escaped its source bottom margin or touched its border: %+v", placed)
	}
	b := markdownCardTestBuilder(t)
	b.hierarchyNode(n, "")
	if b.err != nil {
		t.Fatal(b.err)
	}
	front := math.Inf(-1)
	for _, triangle := range reliefTestWalls(b.triangles) {
		for _, v := range triangle.V {
			front = max(front, captionProjection(v.Position).z)
		}
	}
	if captionProjection(placed.center).z-placed.depth/2 <= front {
		t.Fatal("the complete title does not clear the actual paper and support")
	}
	if !reflect.DeepEqual(n, before) {
		t.Fatal("title placement changed source component geometry")
	}
	// An authored solid fill uses a full-width body. The same source margin
	// cannot clear that shape, so its existing outside anchor is the fallback.
	n.Fill, n.Metadata.Original.Fill = "#ceedee", "#ceedee"
	outside, fits := hierarchyBoardHeaderPlacement(header, board, owner, hierarchyRenderNodes([]d2isometric.Node{n}), .01)
	wantZ := -board.Size.Z/2 - .05 - header.depth/2
	if !fits || math.Abs(outside.center.X) > 1e-9 || math.Abs(outside.center.Z-wantZ) > 1e-9 || outside.width != header.width || outside.depth != header.depth {
		t.Fatalf("blocked solid card title did not retain its source outside anchor: %+v, fits=%v", outside, fits)
	}
	// A peer outside this container can occupy the source title's reserved
	// location. The fallback must query that space as well as the interior.
	peer := fidelityNode(d2target.ShapeRectangle)
	peer.BoardID, peer.Position.X, peer.Position.Z = "peer", 0, -1.25
	peer.Size.X, peer.Size.Z = 2, .4
	if _, fits := hierarchyBoardHeaderPlacement(header, board, owner, hierarchyRenderNodes([]d2isometric.Node{n, peer}), .01); fits {
		t.Fatal("outside title fallback ignored an occluding peer")
	}
}
