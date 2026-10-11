package d2isometricimg

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2target"
)

func arrowTestRoute(b *meshBuilder, edge d2isometric.Edge, points []Vec) []Vec {
	start, end := b.visibleArrowRange(edge, points)
	if start == 0 && end == 1 {
		return points
	}
	return nativeRouteSection(points, routeLengths(points), start, end)
}

func arrowTestBuilder(t *testing.T, nodes []d2isometric.Node, vector bool) *meshBuilder {
	t.Helper()
	ctx := context.Background()
	text, err := newTextPainter(ctx, len(nodes)*2+1)
	if err != nil {
		t.Fatal(err)
	}
	rich, err := newRichLabelPainter(ctx, len(nodes))
	if err != nil {
		t.Fatal(err)
	}
	b := &meshBuilder{ctx: ctx, scale: .01, text: text, rich: rich, options: nativeSceneOptions{vector: vector}}
	for _, n := range nodes {
		// Test the physical shape without independently allocated icon assets.
		n.Icon = ""
		n.Metadata.Original.Icon = nil
		first := len(b.triangles)
		b.hierarchyNode(n, "#bfd6e7")
		b.rememberArrowOwner(n, nil, first)
	}
	if b.err != nil {
		t.Fatal(b.err)
	}
	return b
}

func arrowTestOverlap(b *meshBuilder, id string, m nativeArrowMarker, p, away Vec) bool {
	footprint := m.footprint(p, away)
	for _, obstacle := range b.arrowObstacles(id, p.Y+m.minY) {
		if _, hit := nativeArrowOverlap(footprint, obstacle.points, routeCaptionPoint{}, 0); hit {
			return true
		}
	}
	return false
}

func TestArrowVisibilityRealFixtureAndBackendParity(t *testing.T) {
	d := sourcePanelFixture(t, "regression/dagre_child_id_id/elk/board.exp.json")
	s, err := d2isometric.BuildScene(d, &d2isometric.RenderOpts{})
	if err != nil {
		t.Fatal(err)
	}
	var nodes []d2isometric.Node
	for _, n := range s.Nodes {
		if !n.Container {
			nodes = append(nodes, n)
		}
	}
	raster, vector := arrowTestBuilder(t, nodes, false), arrowTestBuilder(t, nodes, true)
	_, paths, err := nativeEdgeRoutes(context.Background(), s.Edges, s.Nodes, s.Boards)
	if err != nil {
		t.Fatal(err)
	}
	changes := 0
	for i, e := range s.Edges {
		p := paths[i]
		before := append([]Vec(nil), p...)
		a, c := arrowTestRoute(raster, e, p), arrowTestRoute(vector, e, p)
		if !reflect.DeepEqual(a, c) {
			t.Fatalf("%s vector/raster marker geometry differs", e.ID)
		}
		if !reflect.DeepEqual(p, before) {
			t.Fatal("source path was mutated")
		}
		if !reflect.DeepEqual(a, p) {
			changes++
		}
		for _, end := range []struct {
			id       string
			kind     d2target.Arrowhead
			at, away Vec
		}{
			{e.Source, e.SourceArrow, a[0], nunit(nsub(a[1], a[0]))},
			{e.Target, e.TargetArrow, a[len(a)-1], nunit(nsub(a[len(a)-2], a[len(a)-1]))},
		} {
			m := raster.arrowMarker(end.kind, e.StrokeWidth)
			if len(m.vertices) > 0 && arrowTestOverlap(raster, end.id, m, end.at, end.away) {
				t.Fatalf("%s marker still covered by endpoint %s at %+v", e.ID, end.id, end.at)
			}
		}
	}
	if changes < 2 {
		t.Fatalf("expected fixture to exercise hidden arrows, got %d adjusted edges", changes)
	}
}

func TestArrowVisibilityPreservesFullMarkerAndSymmetricEndpoints(t *testing.T) {
	n := fidelityNode(d2target.ShapeRectangle)
	n.Position = nv(0, .42, 0)
	b := arrowTestBuilder(t, []d2isometric.Node{n}, false)
	for _, kind := range []d2target.Arrowhead{d2target.ArrowArrowhead, d2target.TriangleArrowhead, d2target.UnfilledTriangleArrowhead, d2target.DiamondArrowhead, d2target.FilledDiamondArrowhead, d2target.CircleArrowhead, d2target.BoxArrowhead, d2target.CrossArrowhead, d2target.CfMany, d2target.CfManyRequired, d2target.CfOneRequired} {
		t.Run(string(kind), func(t *testing.T) {
			p := []Vec{nv(-1, .08, 0), nv(-3, .08, 0)}
			m := b.arrowMarker(kind, 5)
			if !arrowTestOverlap(b, n.ID, m, p[0], nv(-1, 0, 0)) {
				t.Fatal("control marker does not reproduce cap occlusion")
			}
			e := d2isometric.Edge{Source: n.ID, SourceArrow: kind, StrokeWidth: 5}
			a := arrowTestRoute(b, e, p)
			if a[0].Y != p[0].Y || a[0].X >= p[0].X {
				t.Fatal("marker not retreated along flat path")
			}
			if arrowTestOverlap(b, n.ID, m, a[0], nv(-1, 0, 0)) {
				t.Fatal("complete marker still intersects its cap")
			}
			e.Source, e.Target = "", n.ID
			e.SourceArrow, e.TargetArrow = d2target.NoArrowhead, kind
			c := arrowTestRoute(b, e, []Vec{p[1], p[0]})
			if nlen(nsub(a[0], c[len(c)-1])) > 1e-12 {
				t.Fatal("source/target retreat is asymmetric")
			}
		})
	}
}

func TestArrowVisibilityWalksShortBendAndKeepsTraffic(t *testing.T) {
	n := fidelityNode(d2target.ShapeRectangle)
	n.Position = nv(0, .42, 0)
	b := arrowTestBuilder(t, []d2isometric.Node{n}, false)
	p := []Vec{nv(-1, .08, 0), nv(-1.005, .08, 0), nv(-1.005, .08, -2), nv(-3, .08, -2)}
	e := d2isometric.Edge{ID: "bent", Source: n.ID, SourceArrow: d2target.CfManyRequired, StrokeWidth: 3, Opacity: 1, Points: p}
	m := b.arrowMarker(e.SourceArrow, e.StrokeWidth)
	a := arrowTestRoute(b, e, p)
	if a[0].X != p[1].X || a[0].Z >= p[1].Z || a[0].Z <= p[2].Z {
		t.Fatal("short terminal leg was extrapolated or bend skipped", a)
	}
	if arrowTestOverlap(b, n.ID, m, a[0], nunit(nsub(a[1], a[0]))) {
		t.Fatal("marker on second segment is still hidden")
	}
	e.Metadata.Original.Animated = true
	packets := b.edges([]d2isometric.Edge{e}, newRouteCaptionPlacer(), &d2isometric.Scene{Nodes: []d2isometric.Node{n}})
	if b.err != nil {
		t.Fatal(b.err)
	}
	_, original, err := nativeEdgeRoutes(context.Background(), []d2isometric.Edge{e}, []d2isometric.Node{n}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(packets) != 1 || !reflect.DeepEqual(packets[0].points, original[0]) {
		t.Fatal("presentation marker retreat moved animated contact path")
	}
}

func TestArrowVisibilityIgnoresLowerAndTransparentFill(t *testing.T) {
	for _, shape := range []string{d2target.ShapeRectangle, d2target.ShapeCylinder, d2target.ShapeQueue, d2target.ShapeCircle, d2target.ShapeOval} {
		for _, transparent := range []bool{false, true} {
			node := fidelityNode(shape)
			node.Position = nv(0, .42, 0)
			if transparent {
				node.Fill = "transparent"
				node.Stroke = "transparent"
				node.StrokeWidth = 0
				node.Metadata.Original.Fill = "transparent"
				node.Metadata.Original.Stroke = "transparent"
				node.Metadata.Original.StrokeWidth = 0
			} else {
				node.Position.Y = -1
			}
			b := arrowTestBuilder(t, []d2isometric.Node{node}, false)
			// Starting inside the projected cap makes an incorrect opaque texture
			// rectangle/fan unambiguously move this already-visible marker.
			p := []Vec{nv(-.8, .08, 0), nv(-3, .08, 0)}
			e := d2isometric.Edge{Source: node.ID, SourceArrow: d2target.TriangleArrowhead, StrokeWidth: 2}
			if a := arrowTestRoute(b, e, p); !reflect.DeepEqual(a, p) {
				t.Fatalf("%s already-visible marker moved for lower/transparent endpoint (%v): %+v", shape, transparent, a)
			}
		}
	}
}

func TestArrowVisibilityExhaustedRoutesAndNoHeads(t *testing.T) {
	n := fidelityNode(d2target.ShapeRectangle)
	n.Position = nv(0, .42, 0)
	b := arrowTestBuilder(t, []d2isometric.Node{n}, false)
	for _, p := range [][]Vec{
		{nv(-1, .08, 0), nv(-1.005, .08, 0)},
		{nv(-1, .08, 0), nv(-1.005, .08, 0), nv(-1.01, .1, 0)},
		{nv(-1, .08, 0), nv(-1, .08, 0)},
	} {
		e := d2isometric.Edge{Source: n.ID, Target: n.ID, SourceArrow: d2target.CfMany, TargetArrow: d2target.CfManyRequired, StrokeWidth: 5}
		a := arrowTestRoute(b, e, p)
		if len(a) < 2 {
			t.Fatal("short route disappeared")
		}
		for _, v := range a {
			if math.IsNaN(v.X) || math.IsNaN(v.Y) || math.IsNaN(v.Z) {
				t.Fatal("invalid short-route result")
			}
		}
		e.SourceArrow, e.TargetArrow = d2target.NoArrowhead, d2target.NoArrowhead
		if !reflect.DeepEqual(arrowTestRoute(b, e, p), p) {
			t.Fatal("headless route changed")
		}
	}
}

func TestArrowVisibilityKeepsConcaveOpening(t *testing.T) {
	b := &meshBuilder{ctx: context.Background(), scale: .01}
	profile := []Vec{nv(0, .6, 0), nv(2, .6, 0), nv(2, .6, .5), nv(.5, .6, .5), nv(.5, .6, 1.5), nv(2, .6, 1.5), nv(2, .6, 2), nv(0, .6, 2)}
	mat := nativeMaterial("#bfd6e7", 0, 0, 1)
	b.extrudedProfile(profile, .07, mat, mat)
	n := fidelityNode(d2target.ShapeRectangle)
	b.rememberArrowOwner(n, nil, 0)
	b.arrowOwners[n.ID].prepared = true // The exact supplied C-shaped cap is complete.
	p := []Vec{nv(1.2, .08, .6), nv(3, .08, .6)}
	e := d2isometric.Edge{Source: n.ID, SourceArrow: d2target.TriangleArrowhead, StrokeWidth: 2}
	if a := arrowTestRoute(b, e, p); !reflect.DeepEqual(a, p) {
		t.Fatal("concave opening was filled by a bounding silhouette", a)
	}
	// A filled rectangle at the same outer footprint would cover this marker.
	full := &meshBuilder{ctx: context.Background(), scale: .01}
	full.extrudedProfile([]Vec{nv(0, .6, 0), nv(2, .6, 0), nv(2, .6, 2), nv(0, .6, 2)}, .07, mat, mat)
	full.rememberArrowOwner(n, nil, 0)
	full.arrowOwners[n.ID].prepared = true
	if reflect.DeepEqual(arrowTestRoute(full, e, p), p) {
		t.Fatal("control rectangle does not distinguish the concave opening")
	}
}

func TestArrowVisibilityUsesExistingDepthBias(t *testing.T) {
	b := &meshBuilder{ctx: context.Background(), scale: .01}
	marker := b.arrowMarker(d2target.TriangleArrowhead, 2)
	if marker.minY <= 0 {
		t.Fatal("positive marker depth bias disappeared")
	}
	triangle := Triangle{V: [3]Vertex{{Position: nv(-1, .09, -1)}, {Position: nv(1, .09, -1)}, {Position: nv(0, .09, 1)}}}
	if p := nativeArrowOccluder(triangle, .08+marker.minY); len(p) != 0 {
		t.Fatal("cap behind visible biased marker still forces retreat")
	}
	triangle.DepthBias = .1
	if p := nativeArrowOccluder(triangle, .08+marker.minY); len(p) < 3 {
		t.Fatal("receiver depth bias was omitted")
	}
}

func TestArrowVisibilityBoundsCachesWorkAndCancellation(t *testing.T) {
	n := fidelityNode(d2target.ShapeRectangle)
	n.Position = nv(0, .42, 0)
	b := arrowTestBuilder(t, []d2isometric.Node{n}, false)
	p := []Vec{nv(-1, .08, 0), nv(-3, .08, 0)}
	e := d2isometric.Edge{Source: n.ID, SourceArrow: d2target.TriangleArrowhead, StrokeWidth: 2}
	arrowTestRoute(b, e, p)
	if b.err != nil || b.arrowCachePoints == 0 || len(b.arrowMarkers) == 0 {
		t.Fatal("query cache wasn't populated", b.err)
	}
	count := b.arrowCachePoints
	arrowTestRoute(b, e, p)
	if b.arrowCachePoints != count {
		t.Fatal("repeated ports duplicate cached receiver polygons")
	}
	// Cancellation and the global limit must be checked even on cache hits.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	b.ctx = ctx
	arrowTestRoute(b, e, p)
	if !errors.Is(b.err, context.Canceled) {
		t.Fatal("cancelled cached query did not stop", b.err)
	}
	b.ctx = context.Background()
	b.err = nil
	b.arrowWork = maxNativeArrowWork
	arrowTestRoute(b, e, p)
	if b.err == nil {
		t.Fatal("exhausted cumulative work budget was ignored")
	}
	b.err = nil
	b.arrowWork = 0
	b.arrowCachePoints = maxNativeArrowCachePoints
	before := len(b.arrowOwners[n.ID].obstacles)
	b.arrowObstacles(n.ID, .12345)
	if len(b.arrowOwners[n.ID].obstacles) != before {
		t.Fatal("query cache exceeded its point budget")
	}
}

func TestFilledArrowStemsDoNotBluntTheTip(t *testing.T) {
	for _, kind := range []d2target.Arrowhead{d2target.ArrowArrowhead, d2target.TriangleArrowhead, d2target.FilledDiamondArrowhead, d2target.FilledCircleArrowhead, d2target.FilledBoxArrowhead} {
		t.Run(string(kind), func(t *testing.T) {
			b := &meshBuilder{ctx: context.Background(), scale: .01}
			e := d2isometric.Edge{ID: "point", Source: "source", Target: "target", Points: []Vec{nv(-2, .08, 0), nv(0, .08, 0)}, TargetArrow: kind, StrokeWidth: 2, Opacity: 1}
			e.Metadata.Original.Animated = true
			packets := b.edges([]d2isometric.Edge{e}, newRouteCaptionPlacer())
			if b.err != nil {
				t.Fatal(b.err)
			}
			if len(packets) != 1 || !reflect.DeepEqual(packets[0].points, e.Points) {
				t.Fatal("stem clipping changed animation path")
			}
			clearance := nativeArrowClearance(kind, e.StrokeWidth)
			if clearance <= 0 {
				t.Fatal("filled head has no wire clearance")
			}
			maxWireX := math.Inf(-1)
			for _, triangle := range b.triangles {
				if triangle.DepthBias > .002 {
					continue
				} // Existing marker bias, never changed by trim.
				for _, v := range triangle.V {
					maxWireX = max(maxWireX, v.Position.X)
				}
			}
			if maxWireX > -.4*clearance {
				t.Fatalf("wire/casing still reaches pointed taper: %g (clearance %g)", maxWireX, clearance)
			}
		})
	}
}
