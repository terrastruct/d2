package d2isometricimg

import (
	"context"
	"encoding/json"
	"errors"
	"image/color"
	"math"
	"reflect"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/svg"
)

func sequenceTestEdge(id, role string, points ...Vec) d2isometric.Edge {
	original := *d2target.BaseConnection()
	original.ID = id
	for _, p := range points {
		original.Route = append(original.Route, geo.NewPoint(p.X/.01, p.Z/.01))
	}
	return d2isometric.Edge{ID: id, SequenceRole: role, Points: points, StrokeWidth: 2, Opacity: 1, Metadata: d2isometric.EdgeMetadata{Original: original}}
}

func TestSequenceRoutesPreserveCompiledVerticesInMixedScenes(t *testing.T) {
	ordinary := []d2isometric.Edge{
		sequenceTestEdge("ordinary-a", "", nv(-2, .08, 0), nv(2, .08, 0)),
		sequenceTestEdge("ordinary-b", "", nv(0, .08, -2), nv(0, .08, 2)),
	}
	messages := []d2isometric.Edge{
		sequenceTestEdge("loop", "message", nv(0, .08, 0), nv(2, .08, 0), nv(2, .08, 1), nv(0, .08, 1)),
		sequenceTestEdge("same-path", "message", nv(-2, .08, 0), nv(2, .08, 0)),
		sequenceTestEdge("actor-lifeline", "lifeline", nv(0, .08, -2), nv(0, .08, 2)),
		sequenceTestEdge("actor-message", "message", nv(0, .085, .6), nv(0, .085, 1.4)),
	}
	actor := contactTestQueue()
	messages[3].Source = actor.ID
	contact, err := nativeSolidContactRoutes(context.Background(), messages[3:], []d2isometric.Node{actor}, [][]Vec{messages[3].Points})
	if err != nil || reflect.DeepEqual(contact[0], messages[3].Points) {
		t.Fatal("contact-bypass fixture does not require a solid contact extension", err)
	}
	all := append(append([]d2isometric.Edge(nil), ordinary...), messages...)
	before, _ := json.Marshal(all)
	_, want, err := nativeEdgeRoutes(context.Background(), ordinary, []d2isometric.Node{actor}, nil)
	if err != nil {
		t.Fatal(err)
	}
	lanes, got, err := nativeEdgeRoutes(context.Background(), all, []d2isometric.Node{actor}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := range ordinary {
		if !reflect.DeepEqual(want[i], got[i]) {
			t.Fatal("sequence paths changed ordinary dependency routing")
		}
	}
	for i, edge := range all[len(ordinary):] {
		path := got[len(ordinary)+i]
		if len(path) != len(edge.Points) || !reflect.DeepEqual(lanes[len(ordinary)+i], path) {
			t.Fatal("sequence route acquired rounding, lanes or contact vertices")
		}
		for j, p := range path {
			if p.X != edge.Points[j].X || p.Z != edge.Points[j].Z || p.Y != nativeSequenceEdgeY(edge) {
				t.Fatal("sequence route moved an authored point or added a crossing bridge")
			}
		}
	}
	after, _ := json.Marshal(all)
	if string(before) != string(after) {
		t.Fatal("sequence routing modified source metadata")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := nativeEdgeRoutes(ctx, all, nil, nil); !errors.Is(err, context.Canceled) {
		t.Fatal("sequence routing ignored cancellation")
	}
}

func TestSequenceCaptionsUseCompiledSelfLoopAndEndpointBoxes(t *testing.T) {
	e := sequenceTestEdge("loop", "message", nv(1, .08, 1), nv(3, .08, 1), nv(3, .08, 2), nv(1, .08, 2))
	e.Label = "First line\nSecond line"
	e.Metadata.Original.Text = d2target.Text{Label: e.Label, FontSize: 20, LabelWidth: 270, LabelHeight: 88, Bold: true}
	e.Metadata.Original.LabelPosition, e.Metadata.Original.LabelPercentage = "UNLOCKED_TOP", .6
	e.SourceLabel = &d2target.Text{Label: "source", LabelWidth: 70, LabelHeight: 24}
	e.TargetLabel = &d2target.Text{Label: "target", LabelWidth: 80, LabelHeight: 25}
	e.Metadata.Original.SrcLabel, e.Metadata.Original.DstLabel = e.SourceLabel, e.TargetLabel
	e.Metadata.Original.Icon = iconData(t, "image/svg+xml", []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20"><rect width="20" height="20" fill="#ff0080"/></svg>`))
	for i := range e.Points {
		e.Points[i].X += 5
		e.Points[i].Z -= 3
	}
	before, _ := json.Marshal(e)
	got, icon := nativeSequenceCaptionData(e, .01)
	if len(got) != 3 {
		t.Fatal("source captions were omitted")
	}
	want := []*geo.Point{e.Metadata.Original.GetLabelTopLeft(), e.Metadata.Original.GetArrowheadLabelPosition(false), e.Metadata.Original.GetArrowheadLabelPosition(true)}
	for i, caption := range got {
		w, h := float64(caption.style.LabelWidth)*.01, float64(caption.style.LabelHeight)*.01
		center := nv(5+want[i].X*.01+w/2, .306, -3+want[i].Y*.01+h/2)
		if nlen(nsub(caption.surface.center, center)) > 1e-10 || caption.surface.width != w || caption.surface.depth != h || caption.surface.angle != 0 {
			t.Fatalf("caption %d changed native source anchor, dimensions or orientation: %+v", i, caption.surface)
		}
	}
	iconTL := e.Metadata.Original.GetIconPosition()
	w := float64(d2target.DEFAULT_ICON_SIZE) * .01
	if nlen(nsub(icon.center, nv(5+iconTL.X*.01+w/2, .306, -3+iconTL.Y*.01+w/2))) > 1e-10 || icon.width != w || icon.depth != w {
		t.Fatal("sequence icon displaced or shrank the compiled caption")
	}
	after, _ := json.Marshal(e)
	if string(before) != string(after) {
		t.Fatal("source caption calculations modified input")
	}
}

func TestSequenceInkHasNativeDashSpacingFlatMarkersAndAuthoredAnimation(t *testing.T) {
	e := sequenceTestEdge("lifeline", "lifeline", nv(0, .08, 0), nv(0, .08, 4))
	e.StrokeDash = 6 // The sequence compiler supplies the default, not the renderer.
	b := &meshBuilder{ctx: context.Background(), scale: .01}
	packets := b.edges([]d2isometric.Edge{e}, newRouteCaptionPlacer())
	if b.err != nil || len(packets) != 0 || len(b.triangles) < 4 {
		t.Fatal("lifeline did not produce static dashed ink", b.err)
	}
	dash, _ := svg.GetStrokeDashAttributes(2, e.StrokeDash)
	lo, hi := solidTestBounds(b.triangles[:2])
	if math.Abs(hi.Z-lo.Z-dash*.01) > 1e-10 || math.Abs(hi.X-lo.X-.02) > 1e-10 {
		t.Fatal("lifeline dash length or source stroke width changed")
	}
	for _, tri := range b.triangles {
		if !tri.Material.Unlit || tri.CastShadow || tri.Material.Color != nativePaint("#263c4e", "") {
			t.Fatal("lifeline gained dependency colors, casing, or lighting")
		}
		for _, v := range tri.V {
			if v.Position.Y != .10 {
				t.Fatal("lifeline ink left its plane")
			}
		}
	}
	e.StrokeDash = 0
	solid := &meshBuilder{ctx: context.Background(), scale: .01}
	solid.edges([]d2isometric.Edge{e}, newRouteCaptionPlacer())
	lo, hi = solidTestBounds(solid.triangles)
	if solid.err != nil || len(solid.triangles) != 2 || math.Abs(hi.Z-lo.Z-4) > 1e-10 {
		t.Fatal("authored solid lifeline became dashed", solid.err)
	}
	e.SequenceRole, e.StrokeExplicit, e.Stroke = "message", true, "#b52d65"
	e.TargetArrow = d2target.LineArrowhead
	e.Metadata.Original.Animated = true
	b = &meshBuilder{ctx: context.Background(), scale: .01}
	packets = b.edges([]d2isometric.Edge{e}, newRouteCaptionPlacer())
	if b.err != nil || len(packets) != 1 || !packets[0].forward || packets[0].reverse {
		t.Fatal("authored message animation/arrow direction disappeared", b.err)
	}
	for _, tri := range b.triangles {
		if tri.Material.Color != nativePaint(e.Stroke, "") {
			t.Fatal("explicit message ink was replaced")
		}
		for _, v := range tri.V {
			if v.Position.Y != .30 {
				t.Fatal("message or arrow geometry left the activation plane")
			}
		}
	}
}

func TestSequenceOnEdgeCaptionMasksThickWire(t *testing.T) {
	for _, fill := range []string{"", "#ffffb3"} {
		t.Run("fill="+fill, func(t *testing.T) {
			e := sequenceTestEdge("masked-message", "message", nv(0, .08, 0), nv(3, .08, 0))
			e.Label, e.StrokeWidth, e.StrokeExplicit, e.Stroke = ".", 12, true, "#263c4e"
			e.Metadata.Original.Text = d2target.Text{Label: e.Label, LabelWidth: 140, LabelHeight: 40, FontSize: 16}
			e.Metadata.Original.LabelPosition = "INSIDE_MIDDLE_CENTER"
			e.Metadata.Original.Fill = fill
			painter, err := newTextPainter(context.Background(), 1)
			if err != nil {
				t.Fatal(err)
			}
			b := &meshBuilder{ctx: context.Background(), scale: .01, text: painter}
			group := nativeMaterial("#c4d4e3", 1, 0, 1)
			group.Unlit = true
			b.flat(nv(-1, .05, -1), nv(-1, .05, 1), nv(4, .05, 1), group, false)
			b.flat(nv(-1, .05, -1), nv(4, .05, 1), nv(4, .05, -1), group, false)
			region := append([]Triangle(nil), b.triangles...)
			scene := &d2isometric.Scene{Background: "#edf3f7"}
			b.edges([]d2isometric.Edge{e}, newRouteCaptionPlacer(), scene)
			if b.err != nil {
				t.Fatal(b.err)
			}
			background := nativePaint(scene.Background, "")
			covered, err := NewRaster(b.ctx, 600, 300, b.triangles, background)
			if err != nil {
				t.Fatal(err)
			}
			withoutLabel := e
			withoutLabel.Label = ""
			wire := &meshBuilder{ctx: b.ctx, scale: .01, triangles: region}
			wire.edges([]d2isometric.Edge{withoutLabel}, newRouteCaptionPlacer(), scene)
			uncovered, err := newRaster(b.ctx, 600, 300, wire.triangles, background, &covered.camera, nil)
			if err != nil {
				t.Fatal(err)
			}
			tl := e.Metadata.Original.GetLabelTopLeft()
			p := covered.camera.project(nv(tl.X*.01+.12, .306, 0))
			x, y := int(p.x)/covered.aa, int(p.y)/covered.aa
			want := group.Color
			if fill != "" {
				want = nativePaint(fill, "")
			}
			got := color.NRGBAModel.Convert(covered.output.At(x, y)).(color.NRGBA)
			if got != want {
				t.Fatalf("caption mask obscures the source group or exposes its wire: got %v, want %v", got, want)
			}
			if color.NRGBAModel.Convert(uncovered.output.At(x, y)).(color.NRGBA) == want {
				t.Fatal("wire-knockout probe did not intersect the authored thick wire")
			}
		})
	}
}

func TestSequenceRichCaptionIgnoresAutomaticClearance(t *testing.T) {
	e := sequenceTestEdge("rich-message", "message", nv(0, .08, 0), nv(2, .08, 0))
	e.Label = "**Read** the `request`"
	e.Metadata.Original.Text = d2target.Text{Label: e.Label, Language: "markdown", LabelWidth: 320, LabelHeight: 50, FontSize: 16}
	e.Metadata.Original.LabelPosition = "OUTSIDE_TOP_CENTER"
	rich, err := newRichLabelPainter(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	b := &meshBuilder{ctx: context.Background(), scale: .01, rich: rich}
	placer := newRouteCaptionPlacer()
	placer.Avoid(nv(0, .306, 0), 100, 100)
	b.edges([]d2isometric.Edge{e}, placer)
	if b.err != nil {
		t.Fatal(b.err)
	}
	var printed []Triangle
	for _, tri := range b.triangles {
		if tri.Material.Texture != nil {
			printed = append(printed, tri)
		}
	}
	if len(printed) != 2 {
		t.Fatal("complete rich caption surface disappeared")
	}
	lo, hi := solidTestBounds(printed)
	tl := e.Metadata.Original.GetLabelTopLeft()
	if math.Abs(lo.X-tl.X*.01) > 1e-10 || math.Abs(lo.Z-tl.Y*.01) > 1e-10 || math.Abs(hi.X-lo.X-3.2) > 1e-10 || math.Abs(hi.Z-lo.Z-.5) > 1e-10 || lo.Y != .306 || hi.Y != .306 {
		t.Fatal("automatic caption avoidance changed a compiled sequence label")
	}
}

func TestSequenceCaptionKnockoutPreservesDashPhaseAndPartialAlpha(t *testing.T) {
	e := sequenceTestEdge("diagonal-message", "message", nv(-2, .08, -1), nv(2, .08, 1))
	e.Label, e.StrokeDash, e.StrokeWidth, e.Opacity = ".", 6, 8, .45
	e.Metadata.Original.Text = d2target.Text{Label: e.Label, LabelWidth: 90, LabelHeight: 40, FontSize: 16}
	e.Metadata.Original.LabelPosition = "INSIDE_MIDDLE_CENTER"
	painter, err := newTextPainter(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	with := &meshBuilder{ctx: context.Background(), scale: .01, text: painter}
	with.edges([]d2isometric.Edge{e}, newRouteCaptionPlacer())
	without := &meshBuilder{ctx: with.ctx, scale: .01}
	plain := e
	plain.Label = ""
	without.edges([]d2isometric.Edge{plain}, newRouteCaptionPlacer())
	if with.err != nil || without.err != nil {
		t.Fatal(with.err, without.err)
	}
	background := color.NRGBA{230, 240, 250, 255}
	reference, err := NewRaster(with.ctx, 600, 350, without.triangles, background)
	if err != nil {
		t.Fatal(err)
	}
	masked, err := newRaster(with.ctx, 600, 350, with.triangles, background, &reference.camera, nil)
	if err != nil {
		t.Fatal(err)
	}
	caption, _ := nativeSequenceCaptionData(e, .01)
	s := caption[0].surface
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, x := range []float64{-s.width / 2, s.width / 2} {
		for _, z := range []float64{-s.depth / 2, s.depth / 2} {
			p := masked.camera.project(nadd(s.center, nv(x, 0, z)))
			px, py := p.x/float64(masked.aa), p.y/float64(masked.aa)
			minX, minY, maxX, maxY = min(minX, px-3), min(minY, py-3), max(maxX, px+3), max(maxY, py+3)
		}
	}
	changed := 0
	for y := 0; y < masked.height; y++ {
		for x := 0; x < masked.width; x++ {
			if masked.output.RGBAAt(x, y) == reference.output.RGBAAt(x, y) {
				continue
			}
			changed++
			if float64(x) < minX || float64(x) > maxX || float64(y) < minY || float64(y) > maxY {
				t.Fatalf("caption clipping changed dash phase or compounded alpha outside its source rectangle at %d,%d", x, y)
			}
		}
	}
	if changed == 0 {
		t.Fatal("caption mask did not intersect the diagonal dashed message")
	}
}
