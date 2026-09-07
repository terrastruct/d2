package d2isometricimg

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2target"
)

func TestNativeBorderLabelKeepsSourceStrokeAperture(t *testing.T) {
	for _, tc := range []struct {
		position               string
		x, y, intactX, intactY float64
	}{
		{"BORDER_TOP_CENTER", 100, 0, 20, 0},
		{"BORDER_BOTTOM_CENTER", 100, 120, 20, 120},
		{"BORDER_LEFT_MIDDLE", 0, 60, 0, 10},
		{"BORDER_RIGHT_MIDDLE", 200, 60, 200, 10},
	} {
		t.Run(tc.position, func(t *testing.T) {
			n := fidelityNode("rectangle")
			n.Stroke, n.StrokeWidth = "#ff0000", 4
			n.Metadata.Original.Stroke, n.Metadata.Original.StrokeWidth = n.Stroke, n.StrokeWidth
			n.Metadata.Original.Label, n.Metadata.Original.LabelPosition = "Border title", tc.position
			n.Metadata.Original.LabelWidth, n.Metadata.Original.LabelHeight = 70, 24
			paint := func(node d2isometric.Node) (*image.RGBA, labelSurface) {
				b := &meshBuilder{ctx: context.Background(), scale: .01}
				tex, area := b.nativeFace(nativeFaceSource(node, node.Fill))
				if b.err != nil {
					t.Fatal(b.err)
				}
				return tex, area
			}
			masked, area := paint(n)
			n.Metadata.Original.Label = ""
			plain, plainArea := paint(n)
			redAt := func(tex *image.RGBA, a labelSurface, x, y float64) bool {
				px := int((x - a.center.X + a.width/2) / a.width * float64(tex.Bounds().Dx()))
				py := int((y - a.center.Z + a.depth/2) / a.depth * float64(tex.Bounds().Dy()))
				c := tex.RGBAAt(px, py)
				return c.R > 200 && c.G < 70 && c.B < 70 && c.A > 200
			}
			if !redAt(plain, plainArea, tc.x, tc.y) {
				t.Fatal("fixture does not contain the original border")
			}
			if redAt(masked, area, tc.x, tc.y) {
				t.Fatal("source border crosses its border-position label")
			}
			if !redAt(masked, area, tc.intactX, tc.intactY) {
				t.Fatal("border aperture erased unrelated stroke")
			}
		})
	}
}

func fidelityNode(kind string) d2isometric.Node {
	s := *d2target.BaseShape()
	s.ID = "component"
	s.Type = kind
	s.Width = 200
	s.Height = 120
	s.Fill = "#bfd6e7"
	s.Stroke = "#20354a"
	return d2isometric.Node{ID: s.ID, Type: kind, Size: nv(2, .7, 1.2), Position: nv(3, .42, 4), Fill: s.Fill, Stroke: s.Stroke, StrokeWidth: s.StrokeWidth, Opacity: 1, FillExplicit: true, Metadata: d2isometric.NodeMetadata{Original: s}}
}

func TestNativeCanonicalMissingSilhouettes(t *testing.T) {
	for _, kind := range []string{"page", "parallelogram", "document", "package", "step", "callout", "stored_data", "hexagon", "cloud", "person", "c4-person"} {
		t.Run(kind, func(t *testing.T) {
			n := fidelityNode(kind)
			p, err := nativeShapeProfile(n.Metadata.Original)
			if err != nil {
				t.Fatal(err)
			}
			if len(p) < 4 || len(p) > 2048 || math.Abs(nativePolygonArea(p)) < 1 {
				t.Fatalf("missing source silhouette: %s (%d)", kind, len(p))
			}
			b := &meshBuilder{ctx: context.Background(), scale: .01}
			before, _ := json.Marshal(n)
			b.node(n, "#849ebc")
			after, _ := json.Marshal(n)
			if b.err != nil || len(b.triangles) < 4 {
				t.Fatalf("shape geometry absent: %v", b.err)
			}
			if string(before) != string(after) {
				t.Fatal("source shape mutated")
			}
			for _, tri := range b.triangles {
				for _, v := range tri.V {
					if !captionFinite(v.Position.X, v.Position.Y, v.Position.Z) || math.Abs(nlen(v.Normal)-1) > 1e-9 {
						t.Fatal("invalid native contour geometry")
					}
				}
			}
		})
	}
}

func TestNativeSourceFaceDecorationsAreDistinct(t *testing.T) {
	base := fidelityNode("rectangle")
	cases := []struct {
		name   string
		change func(*d2isometric.Node)
	}{
		{"plain", func(*d2isometric.Node) {}},
		{"border-color", func(n *d2isometric.Node) { n.Stroke = "#ec2050" }},
		{"border-width", func(n *d2isometric.Node) { n.StrokeWidth = 9 }},
		{"border-dash", func(n *d2isometric.Node) { n.StrokeDash = 5 }},
		{"border-radius", func(n *d2isometric.Node) { n.Metadata.Original.BorderRadius = 25 }},
		{"double-border", func(n *d2isometric.Node) { n.Metadata.Original.DoubleBorder = true }},
		{"dots", func(n *d2isometric.Node) { n.Metadata.Original.FillPattern = "dots" }},
		{"lines", func(n *d2isometric.Node) { n.Metadata.Original.FillPattern = "lines" }},
		{"grain", func(n *d2isometric.Node) { n.Metadata.Original.FillPattern = "grain" }},
		{"paper", func(n *d2isometric.Node) { n.Metadata.Original.FillPattern = "paper" }},
	}
	seen := map[[32]byte]string{}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			n := base
			c.change(&n)
			s := nativeFaceSource(n, n.Fill)
			b := &meshBuilder{ctx: context.Background()}
			tex, _ := b.nativeFace(s)
			if b.err != nil || tex == nil {
				t.Fatalf("decoration unavailable: %v", b.err)
			}
			sum := sha256.Sum256(tex.Pix)
			if previous, ok := seen[sum]; ok {
				t.Fatalf("%s paint identical to %s", c.name, previous)
			}
			seen[sum] = c.name
		})
	}
}

func TestNativeMultipleShadowReliefAndAnimation(t *testing.T) {
	n := fidelityNode("document")
	build := func(n d2isometric.Node) *meshBuilder {
		b := &meshBuilder{ctx: context.Background(), scale: .01}
		b.node(n, "#849ebc")
		if b.err != nil {
			t.Fatal(b.err)
		}
		return b
	}
	plain := build(n)
	contactCasters := 0
	for _, tri := range plain.triangles {
		if tri.CastShadow {
			contactCasters++
			if tri.ShadowOpacity == nil || *tri.ShadowOpacity <= 0 || *tri.ShadowOpacity >= 1 {
				t.Fatal("ordinary solid lost its restrained physical shadow")
			}
		}
	}
	if contactCasters == 0 {
		t.Fatal("solid shape has no physical shadow")
	}
	n.Metadata.Original.Shadow = true
	n.Metadata.Original.Animated = true
	shadow := build(n)
	casters := 0
	for _, tri := range shadow.triangles {
		if tri.CastShadow {
			casters++
			if tri.ShadowOpacity == nil || *tri.ShadowOpacity != 1 {
				t.Fatal("authored shadow did not strengthen the physical shadow")
			}
		}
	}
	if casters == 0 || len(shadow.animatedNodes) != 1 || shadow.animatedNodes[0].first != 0 || shadow.animatedNodes[0].last != len(shadow.triangles) {
		t.Fatal("authored effects not registered")
	}
	n.Metadata.Original.Multiple = true
	multiple := build(n)
	if len(multiple.triangles) != 2*len(plain.triangles) {
		t.Fatal("multiple geometry copy missing")
	}
	bounds := func(b *meshBuilder) (Vec, Vec) {
		lo, hi := nv(math.Inf(1), math.Inf(1), math.Inf(1)), nv(math.Inf(-1), math.Inf(-1), math.Inf(-1))
		for _, tri := range b.triangles {
			for _, v := range tri.V {
				p := v.Position
				lo.X, lo.Y, lo.Z = min(lo.X, p.X), min(lo.Y, p.Y), min(lo.Z, p.Z)
				hi.X, hi.Y, hi.Z = max(hi.X, p.X), max(hi.Y, p.Y), max(hi.Z, p.Z)
			}
		}
		return lo, hi
	}
	pl, ph := bounds(plain)
	ml, mh := bounds(multiple)
	if math.Abs(mh.X-ph.X-d2target.MULTIPLE_OFFSET*.01) > 1e-8 || math.Abs(pl.Z-ml.Z-d2target.MULTIPLE_OFFSET*.01) > 1e-8 {
		t.Fatal("multiple source offset changed")
	}
	n.Metadata.Original.Multiple = false
	n.Metadata.Original.ThreeDee = true
	_, deep := bounds(build(n))
	if math.Abs(deep.Y-ph.Y-d2target.THREE_DEE_OFFSET*.01) > 1e-8 {
		t.Fatal("authored 3D depth ignored")
	}
}

func TestHierarchyCanonicalBodyRetainsDepthWithinSourceFootprint(t *testing.T) {
	n := fidelityNode("rectangle")
	b := &meshBuilder{ctx: context.Background(), scale: .01}
	b.hierarchyNode(n, "#849ebc")
	if b.err != nil {
		t.Fatal(b.err)
	}
	lo, hi := math.Inf(1), math.Inf(-1)
	sideColors := map[color.NRGBA]bool{}
	for _, tri := range b.triangles {
		if tri.Material.Texture != nil || tri.Material.Unlit {
			continue // Centered source stroke is paint, not physical footprint.
		}
		sideColors[tri.Material.Color] = true
		for _, v := range tri.V {
			lo, hi = min(lo, v.Position.Y), max(hi, v.Position.Y)
			if math.Abs(v.Position.X-n.Position.X) > n.Size.X/2+1e-9 || math.Abs(v.Position.Z-n.Position.Z) > n.Size.Z/2+1e-9 {
				t.Fatal("depth treatment expanded the source footprint")
			}
		}
	}
	if hi-lo < .15 || len(sideColors) != 1 {
		t.Fatal("solid contour lost its relief or continuous sidewall")
	}
	faceColor := nativeMaterial(n.Fill, .68, 0, n.Opacity).Color
	if !sideColors[faceColor] {
		t.Fatal("classic body must share the source material across its faces; light supplies contrast")
	}
}

func TestNativeFaceTextureBudgetAndCancellation(t *testing.T) {
	n := fidelityNode("rectangle")
	b := &meshBuilder{ctx: context.Background(), faceMaxPixels: 4096}
	tex, _ := b.nativeFace(nativeFaceSource(n, n.Fill))
	if b.err != nil {
		t.Fatal(b.err)
	}
	if tex.Bounds().Dx()*tex.Bounds().Dy() > 4300 {
		t.Fatal("per-face allocation not bounded")
	}
	b.facePixels = 24 * 1024 * 1024
	b.nativeFace(nativeFaceSource(n, n.Fill))
	if b.err == nil {
		t.Fatal("aggregate texture admission ignored")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	b = &meshBuilder{ctx: ctx}
	b.nativeFace(nativeFaceSource(n, n.Fill))
	if b.err == nil {
		t.Fatal("canceled shape build succeeded")
	}
}

func TestNativeFaceClearsNonpaintInteractionMetadata(t *testing.T) {
	n := fidelityNode("rectangle")
	n.Metadata.Original.Link = "https://example.com/source"
	n.Metadata.Original.PrettyLink = "source"
	n.Metadata.Original.Tooltip = "Source information"
	n.Metadata.Original.TooltipPosition = "OUTSIDE_TOP_RIGHT"
	n.Metadata.Original.Classes = []string{"source-class"}
	s := nativeFaceSource(n, n.Fill)
	if s.Link != "" || s.PrettyLink != "" || s.Tooltip != "" || s.TooltipPosition != "" || len(s.Classes) != 0 {
		t.Fatal("face retained nonpaint metadata")
	}
	b := &meshBuilder{ctx: context.Background()}
	tex, _ := b.nativeFace(s)
	if b.err != nil || tex == nil {
		t.Fatalf("linked source face failed: %v", b.err)
	}
	if n.Metadata.Original.Link == "" || n.Metadata.Original.Tooltip == "" {
		t.Fatal("original interaction metadata mutated")
	}
}

func TestNativeArrowSourceMetricsAndHollowClearance(t *testing.T) {
	for _, kind := range []d2target.Arrowhead{d2target.ArrowArrowhead, d2target.TriangleArrowhead, d2target.UnfilledTriangleArrowhead, d2target.LineArrowhead, d2target.DiamondArrowhead, d2target.FilledDiamondArrowhead, d2target.CircleArrowhead, d2target.FilledCircleArrowhead, d2target.BoxArrowhead, d2target.FilledBoxArrowhead, d2target.CrossArrowhead, d2target.CfOne, d2target.CfMany, d2target.CfOneRequired, d2target.CfManyRequired} {
		for _, width := range []int{1, 2, 9} {
			b := &meshBuilder{ctx: context.Background(), scale: .01}
			b.arrow(string(kind), nv(0, .08, 0), nv(1, 0, 0), nativeMaterial("#244765", .3, .1, 1), width)
			if b.err != nil || len(b.triangles) == 0 {
				t.Fatalf("missing %s at width%d: %v", kind, width, b.err)
			}
			w, h := kind.Dimensions(float64(width))
			for _, tri := range b.triangles {
				for _, v := range tri.V {
					if !captionFinite(v.Position.X, v.Position.Y, v.Position.Z) || math.Abs(v.Position.Z) > (h+float64(width))*.01 || math.Abs(v.Position.X) > (w+2*float64(width)+8)*.01 {
						t.Fatalf("%s dimensions not stroke-derived: %+v", kind, v.Position)
					}
				}
			}
		}
	}
	for _, kind := range []d2target.Arrowhead{d2target.UnfilledTriangleArrowhead, d2target.CircleArrowhead, d2target.DiamondArrowhead, d2target.BoxArrowhead} {
		if nativeArrowClearance(kind, 7) <= nativeArrowClearance(kind, 2) {
			t.Fatal("hollow wire exclusion did not follow stroke metrics")
		}
	}
	b := &meshBuilder{ctx: context.Background(), scale: .01}
	b.arrow("arrow", nv(0, .08, 0), nv(1, 0, 0), nativeMaterial("#123456", .3, .1, 1), 2)
	if len(b.triangles) != 2 {
		t.Fatalf("arrow is not a filled four-point chevron: %d", len(b.triangles))
	}
}

func TestNativeContainerUsesSourceContourAndStyle(t *testing.T) {
	n := fidelityNode("hexagon")
	n.Container = true
	n.Metadata.Original.StrokeDash = 5
	n.StrokeDash = 5
	board := d2isometric.Board{Kind: "group", Position: nv(3, 0, 4), Size: nv(2, .14, 1.2)}
	b := &meshBuilder{ctx: context.Background(), scale: .01}
	b.hierarchyBoard(board, &n, n.Fill, .5)
	if b.err != nil || len(b.triangles) != 2 {
		t.Fatalf("container source face missing: %v", b.err)
	}
	tex := b.triangles[0].Material.Texture
	if tex == nil {
		t.Fatal("styled container has no exact face paint")
	}
	r, _, _, a := tex.At(tex.Bounds().Min.X, tex.Bounds().Min.Y).RGBA()
	if r != 0 || a != 0 {
		t.Fatal("hexagonal container replaced with rectangular wash")
	}
	for _, tri := range b.triangles {
		if tri.Material.Color.A != 128 {
			t.Fatal("container opacity applied more than once")
		}
	}
}

func TestNativeShapeLabelUsesSourceInnerBox(t *testing.T) {
	n := fidelityNode("callout")
	n.Metadata.Original.Label = "Note"
	n.Metadata.Original.LabelWidth = 60
	n.Metadata.Original.LabelHeight = 20
	s := nativeFaceSource(n, n.Fill)
	got := nativeNodeLabelSurface(n, s, .2)
	if got.center.Z >= n.Position.Z || math.Abs(got.width-.6) > 1e-10 || math.Abs(got.depth-.2) > 1e-10 {
		t.Fatalf("callout label covers its tail: %+v", got)
	}
	n.Metadata.Original.LabelPosition = "OUTSIDE_TOP_CENTER"
	outside := nativeNodeLabelSurface(n, s, .2)
	if outside.center.Z+outside.depth/2 >= n.Position.Z-n.Size.Z/2 {
		t.Fatal("authored outside label moved inside")
	}
	n = fidelityNode("diamond")
	if !nativeCanonicalNode(n) {
		t.Fatal("diamond has no actual label face")
	}
}

func TestNativeDisjointContoursKeepHeadAndSkipFoldWalls(t *testing.T) {
	for _, c := range []struct {
		kind  string
		count int
	}{{"c4-person", 2}, {"page", 1}, {"rectangle", 1}} {
		n := fidelityNode(c.kind)
		profiles, err := nativeShapeProfiles(n.Metadata.Original)
		if err != nil || len(profiles) != c.count {
			t.Fatalf("%s contour count=%d want%d: %v", c.kind, len(profiles), c.count, err)
		}
	}
}

func TestNativeHollowArrowKnocksOutCrossingWire(t *testing.T) {
	b := &meshBuilder{ctx: context.Background(), scale: .01, arrowBackground: "#ffffff"}
	center := nv(-.08, .08, 0)
	// Connections are printed on the shared route plane. A raised tube can
	// legitimately occlude the flat marker from a different camera angle.
	wire := nativeMaterial("#ff0000", .4, 0, 1)
	wire.Unlit = true
	b.routeInk([]Vec{nadd(center, nv(0, 0, -.25)), nadd(center, nv(0, 0, .25))}, .024, wire)
	wireCount := len(b.triangles)
	b.arrowWithOpacity("circle", nv(0, .08, 0), nv(1, 0, 0), nativeMaterial("#123456", .3, 0, .5), 2, 1)
	r, err := NewRaster(context.Background(), 320, 200, b.triangles, color.NRGBA{180, 200, 220, 255})
	if err != nil {
		t.Fatal(err)
	}
	frame, err := r.Frame(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	p := r.camera.project(center)
	before, err := newRaster(context.Background(), 320, 200, b.triangles[:wireCount], color.NRGBA{180, 200, 220, 255}, &r.camera, nil)
	if err != nil {
		t.Fatal(err)
	}
	if c := before.output.RGBAAt(int(p.x)/r.aa, int(p.y)/r.aa); c != (color.RGBA{R: 255, A: 255}) {
		t.Fatalf("fixture has no crossing wire at marker center: %+v", c)
	}
	c := frame.RGBAAt(int(p.x)/r.aa, int(p.y)/r.aa)
	if c.R < 245 || c.G < 245 || c.B < 245 {
		t.Fatalf("crossing wire shows through hollow marker: %+v", c)
	}
	backdrop := false
	for _, tri := range b.triangles {
		if tri.Material.Unlit && tri.Material.Color.R == 255 {
			backdrop = true
			if tri.Material.Color.A != 255 {
				t.Fatal("stroke alpha incorrectly reduced background knockout opacity")
			}
		}
	}
	if !backdrop {
		t.Fatal("hollow marker has no background knockout")
	}
}
