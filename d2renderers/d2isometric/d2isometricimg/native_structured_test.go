package d2isometricimg

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"math"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2target"
)

func structuredFixtureNodes(t *testing.T, path string) (*d2isometric.Scene, []d2isometric.Node) {
	t.Helper()
	d := sourcePanelFixture(t, path)
	s, err := d2isometric.BuildScene(d, &d2isometric.RenderOpts{})
	if err != nil {
		t.Fatal(err)
	}
	var nodes []d2isometric.Node
	for _, n := range s.Nodes {
		if nativeStructuredNode(n) {
			nodes = append(nodes, n)
		}
	}
	if len(nodes) == 0 {
		t.Fatal("fixture contains no structured component")
	}
	return s, nodes
}

func structuredTestBuilder(t *testing.T, s *d2isometric.Scene) *meshBuilder {
	t.Helper()
	p, err := newRichLabelPainter(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	p.configureFontFamilies(s.FontFamily, s.MonoFontFamily)
	p.configureTheme(s.ThemeID, s.ThemeOverrides)
	return &meshBuilder{ctx: context.Background(), scale: s.PixelScale, rich: p, outputDensity: 120, faceMaxPixels: 1024 * 1024}
}

func TestStructuredRailsUseRealRowsAndCompiledFootprints(t *testing.T) {
	for _, fixture := range []string{"stable/class/dagre/board.exp.json", "stable/class_and_sqlTable_border_radius/dagre/board.exp.json", "txtar/multiline-class-headers/dagre/board.exp.json"} {
		scene, nodes := structuredFixtureNodes(t, fixture)
		for _, n := range nodes {
			t.Run(fixture+"/"+n.ID, func(t *testing.T) {
				before, _ := json.Marshal(n)
				b := structuredTestBuilder(t, scene)
				b.hierarchyNode(n, "")
				if b.err != nil {
					t.Fatal(b.err)
				}
				if len(b.panels) != 0 {
					t.Fatal("structured component became a flat panel")
				}
				lo, hi := nv(math.Inf(1), math.Inf(1), math.Inf(1)), nv(math.Inf(-1), math.Inf(-1), math.Inf(-1))
				caps, decals := map[int]bool{}, 0
				floor := n.Position.Y - n.Size.Y/2
				rows := nativeStructuredRows(n.Metadata.Original)
				for _, tri := range b.triangles {
					if tri.Material.Texture != nil {
						decals++
						// One native text cell per decal: no texture spans the
						// full table/class or includes its physical row paint.
						zlo, zhi := math.Inf(1), math.Inf(-1)
						for _, v := range tri.V {
							zlo, zhi = min(zlo, v.Position.Z), max(zhi, v.Position.Z)
						}
						if zhi-zlo > rows[0].depth*scene.PixelScale+1e-8 {
							t.Fatal("text decal spans more than its source row")
						}
						continue
					}
					if tri.Material.Unlit {
						continue
					}
					for _, v := range tri.V {
						p := v.Position
						lo = nv(min(lo.X, p.X), min(lo.Y, p.Y), min(lo.Z, p.Z))
						hi = nv(max(hi.X, p.X), max(hi.Y, p.Y), max(hi.Z, p.Z))
					}
					if tri.V[0].Normal.Y > .99 {
						caps[int(math.Round((tri.V[0].Position.Y-floor)*1000))] = true
					}
				}
				if math.Abs(hi.X-lo.X-n.Size.X) > 1e-8 || math.Abs(hi.Z-lo.Z-n.Size.Z) > 1e-8 || math.Abs((hi.X+lo.X)/2-n.Position.X) > 1e-8 || math.Abs((hi.Z+lo.Z)/2-n.Position.Z) > 1e-8 {
					t.Fatalf("source footprint moved: %v to %v", lo, hi)
				}
				if !caps[10] || len(rows) > 1 && !caps[320] || !caps[460] || decals < 2*len(rows) {
					t.Fatalf("physical plinth/rows/header or source text missing: caps=%v decals=%d", caps, decals)
				}
				for i, row := range rows[1:] {
					center := row.z + row.depth/2
					if center <= row.z+row.back || center >= row.z+row.depth-row.front {
						t.Fatalf("row %d no longer contains its compiled port/label center", i)
					}
				}
				after, _ := json.Marshal(n)
				if !bytes.Equal(before, after) {
					t.Fatal("renderer mutated compiled source")
				}
			})
		}
	}
}

func TestStructuredCellsRetainNativeFontsBaselinesAndSemantics(t *testing.T) {
	for _, fixture := range []string{"stable/class_underline/dagre/board.exp.json", "stable/sql_table_constraints_width/dagre/board.exp.json", "txtar/multiline-class-headers/dagre/board.exp.json"} {
		scene, nodes := structuredFixtureNodes(t, fixture)
		for _, n := range nodes {
			b := structuredTestBuilder(t, scene)
			doc, cells, err := b.structuredDocument(n)
			if err != nil {
				t.Fatal(err)
			}
			visibleRuns := 0
			for _, run := range richRuns(doc) {
				if run.Text != "" {
					visibleRuns++
				}
			}
			if len(cells) != visibleRuns {
				t.Fatal("native text cell was dropped")
			}
			rows := nativeStructuredRows(n.Metadata.Original)
			underlines := 0
			for _, cell := range cells {
				run := cell.node.Primitive.(d2scene.TextRun)
				if cell.row < 0 || cell.row >= len(rows) {
					t.Fatal("cell assigned to a nonexistent row")
				}
				r := rows[cell.row]
				if run.Origin.Y < r.z || run.Origin.Y > r.z+r.depth {
					t.Fatal("native baseline moved outside compiled row")
				}
				if n.Type == d2target.ShapeClass && run.Font.Family != "SourceCodePro" {
					t.Fatal("class member stopped using its native mono face")
				}
				if run.Underline {
					underlines++
				}
				if strings.HasSuffix(cell.node.ID, ":constraint") && run.Anchor != d2scene.AnchorEnd {
					t.Fatal("SQL constraint lost its right alignment")
				}
			}
			want := 0
			for _, f := range n.Metadata.Original.Fields {
				if f.Underline {
					want++
				}
			}
			for _, m := range n.Metadata.Original.Methods {
				if m.Underline {
					want++
				}
			}
			if underlines != want {
				t.Fatalf("static member underlines changed: %d != %d", underlines, want)
			}
		}
	}
}

func TestStructuredRailsKeepSourceEffectsAndBudgets(t *testing.T) {
	scene, nodes := structuredFixtureNodes(t, "stable/class_and_sqlTable_border_radius/dagre/board.exp.json")
	n := nodes[0]
	for _, opacity := range []float64{1, .45, 0} {
		b := structuredTestBuilder(t, scene)
		copyNode := n
		copyNode.Opacity = opacity
		copyNode.Metadata.Original.Multiple, copyNode.Metadata.Original.DoubleBorder, copyNode.Metadata.Original.Animated = true, true, true
		copyNode.StrokeDash = 3
		b.hierarchyNode(copyNode, "")
		if b.err != nil {
			t.Fatal(b.err)
		}
		if opacity == 0 {
			if len(b.triangles) != 0 || b.rich.used != 0 {
				t.Fatal("invisible structured shape allocated visible geometry or text")
			}
			continue
		}
		if len(b.animatedNodes) != 1 || b.animatedNodes[0].first != 0 || b.animatedNodes[0].last != len(b.triangles) {
			t.Fatal("animation does not own all rails, ink and cell decals")
		}
		if b.facePixels == 0 || b.rich.used != 1 || b.rich.pixels == 0 {
			t.Fatal("source border effects or text skipped their bounded native renderer")
		}
		copySeen := false
		for _, tri := range b.triangles {
			if opacity < 1 && (tri.OpacityGroup == nil || tri.OpacityGroup.Opacity != opacity) {
				t.Fatal("physical rails, cell decals and source ink do not share object opacity")
			}
			if opacity == 1 && tri.OpacityGroup != nil {
				t.Fatal("opaque structured shape acquired an offscreen compositing group")
			}
			for _, v := range tri.V {
				if v.Position.X > n.Position.X+n.Size.X/2+.05 && !tri.Material.Unlit {
					copySeen = true
				}
			}
		}
		if !copySeen {
			t.Fatal("multiple modifier lost its rear physical copy")
		}
	}
	b := structuredTestBuilder(t, scene)
	b.rich.rows = maxRichTotalRows
	b.structuredNode(n)
	if b.err == nil || !strings.Contains(b.err.Error(), "budget") {
		t.Fatal("per-cell rendering bypassed aggregate source row budget")
	}
}

func TestStructuredRowPortsMeetWallsWithoutLiftingRoutes(t *testing.T) {
	scene, nodes := structuredFixtureNodes(t, "stable/sql_tables/elk/board.exp.json")
	meshes := map[string][]Triangle{}
	for _, n := range nodes {
		b := structuredTestBuilder(t, scene)
		b.hierarchyNode(n, "")
		if b.err != nil {
			t.Fatal(b.err)
		}
		meshes[n.ID] = b.triangles
	}
	ports := 0
	for _, edge := range scene.Edges {
		for _, p := range edge.Points {
			if p.Y != .08 {
				t.Fatal("row attachment lifted the flat connection route")
			}
		}
		for _, endpoint := range []struct {
			id string
			p  Vec
		}{{edge.Source, edge.Points[0]}, {edge.Target, edge.Points[len(edge.Points)-1]}} {
			found := false
			for _, tri := range meshes[endpoint.id] {
				if tri.Material.Texture != nil || tri.Material.Unlit || math.Abs(tri.V[0].Normal.X) < .99 {
					continue
				}
				lo, hi := tri.V[0].Position, tri.V[0].Position
				for _, v := range tri.V[1:] {
					lo = nv(min(lo.X, v.Position.X), min(lo.Y, v.Position.Y), min(lo.Z, v.Position.Z))
					hi = nv(max(hi.X, v.Position.X), max(hi.Y, v.Position.Y), max(hi.Z, v.Position.Z))
				}
				p := endpoint.p
				// A compiled outline port may include half the source stroke.
				if math.Abs(p.X-lo.X) <= .011 && p.Y >= lo.Y-1e-8 && p.Y <= hi.Y+1e-8 && p.Z >= lo.Z-1e-8 && p.Z <= hi.Z+1e-8 && hi.Y-lo.Y > .2 {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("source port %s %v ends below its physical row wall", endpoint.id, endpoint.p)
			}
			ports++
		}
	}
	if ports != 6 {
		t.Fatalf("expected the fixture's six foreign-key row endpoints, got %d", ports)
	}
}

func TestStructuredOpacityFadesTheFinishedObjectOnce(t *testing.T) {
	scene, nodes := structuredFixtureNodes(t, "stable/class/dagre/board.exp.json")
	n := nodes[0]
	build := func(opacity float64) []Triangle {
		b := structuredTestBuilder(t, scene)
		n.Opacity = opacity
		b.hierarchyNode(n, "")
		if b.err != nil {
			t.Fatal(b.err)
		}
		for i := range b.triangles {
			b.triangles[i].CastShadow = false
		}
		return b.triangles
	}
	opaque, faded := build(1), build(.45)
	var extent projectedExtent
	extent.mesh(opaque)
	camera := extent.camera(280, 240, false)
	bg := color.NRGBA{R: 245, G: 247, B: 251, A: 255}
	render := func(mesh []Triangle) *image.RGBA {
		r, err := newRaster(context.Background(), 280, 240, mesh, bg, &camera, nil)
		if err != nil {
			t.Fatal(err)
		}
		frame, err := r.Frame(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		return frame
	}
	a, c := render(opaque), render(faded)
	changed := 0
	for y := 0; y < a.Rect.Dy(); y++ {
		for x := 0; x < a.Rect.Dx(); x++ {
			at := a.PixOffset(x, y)
			for channel, background := range []uint8{bg.R, bg.G, bg.B} {
				want := .45*float64(a.Pix[at+channel]) + .55*float64(background)
				if math.Abs(float64(c.Pix[at+channel])-want) > 2 {
					t.Fatalf("object opacity compounded at (%d,%d) channel%d: got%d want%.2f", x, y, channel, c.Pix[at+channel], want)
				}
				if a.Pix[at+channel] != background {
					changed++
				}
			}
		}
	}
	if changed < 5000 {
		t.Fatal("opacity comparison did not exercise the physical object and labels")
	}
}

func TestStructuredOutsideIconsKeepTheReservedSourceSpace(t *testing.T) {
	scene, nodes := structuredFixtureNodes(t, "txtar/sql-icon/elk/board.exp.json")
	checked := 0
	for _, n := range nodes {
		if n.Metadata.Original.Icon == nil {
			continue
		}
		s := n.Metadata.Original
		if s.IconPosition != "OUTSIDE_TOP_LEFT" {
			t.Fatal("fixture no longer exercises outside icon space")
		}
		face := nativeStructuredIconSurface(n, .53)
		left, top := n.Position.X-n.Size.X/2, n.Position.Z-n.Size.Z/2
		if math.Abs(face.center.X-face.width/2-(left-.05)) > 1e-8 || math.Abs(face.center.Z+face.depth/2-(top-.05)) > 1e-8 {
			t.Fatal("outside icon moved into the header or lost its native margin")
		}
		b := structuredTestBuilder(t, scene)
		_, withIcon, err := b.structuredDocument(n)
		if err != nil {
			t.Fatal(err)
		}
		n.Metadata.Original.Icon = nil
		b = structuredTestBuilder(t, scene)
		_, withoutIcon, err := b.structuredDocument(n)
		if err != nil {
			t.Fatal(err)
		}
		with, _ := json.Marshal(withIcon[0].node.Primitive)
		without, _ := json.Marshal(withoutIcon[0].node.Primitive)
		if !bytes.Equal(with, without) {
			t.Fatal("placing the outside icon changed the source header text")
		}
		checked++
	}
	if checked != 2 {
		t.Fatalf("expected one table and one class icon, got %d", checked)
	}
}
