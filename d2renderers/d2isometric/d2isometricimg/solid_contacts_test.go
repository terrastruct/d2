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

func contactTestQueue() d2isometric.Node {
	s := d2target.BaseShape()
	s.ID, s.Type, s.Width, s.Height = "queue", d2target.ShapeQueue, 200, 120
	s.Fill, s.Stroke = "#ccd9e5", "#617588"
	return d2isometric.Node{ID: s.ID, Type: s.Type, Fill: s.Fill, Stroke: s.Stroke, Opacity: 1, StrokeWidth: 2, Size: nv(2, .85, 1.2), Position: nv(0, .07+.85/2, 0), Metadata: d2isometric.NodeMetadata{Original: *s}}
}

func contactTestOnTriangle(p Vec, triangle Triangle) bool {
	a, b, c := triangle.V[0].Position, triangle.V[1].Position, triangle.V[2].Position
	u, v, q := nsub(b, a), nsub(c, a), nsub(p, a)
	normal := ncross(u, v)
	if nlen(normal) < 1e-12 || math.Abs(ndot(q, nunit(normal))) > 1e-8 {
		return false
	}
	uu, uv, vv := ndot(u, u), ndot(u, v), ndot(v, v)
	qu, qv := ndot(q, u), ndot(q, v)
	denominator := uu*vv - uv*uv
	if math.Abs(denominator) < 1e-18 {
		return false
	}
	x, y := (qu*vv-qv*uv)/denominator, (qv*uu-qu*uv)/denominator
	return x >= -1e-7 && y >= -1e-7 && x+y <= 1+1e-7
}

func TestQueueContactsReachActualBarrelAndSharpCaps(t *testing.T) {
	n := contactTestQueue()
	b := &meshBuilder{ctx: context.Background(), scale: .01}
	b.solidNode(n)
	if b.err != nil {
		t.Fatal(b.err)
	}
	floor := n.Position.Y - n.Size.Y/2
	for i := range b.triangles {
		for j := range b.triangles[i].V {
			v := &b.triangles[i].V[j]
			v.Position.Y = floor + (v.Position.Y-floor)*hierarchyNodeRelief(n)
		}
	}
	for _, fixture := range []struct{ start, inward Vec }{
		{nv(0, .085, .6), nv(0, 0, -1)}, {nv(0, .085, -.6), nv(0, 0, 1)},
		{nv(1, .085, 0), nv(-1, 0, 0)}, {nv(-1, .085, 0), nv(1, 0, 0)},
		{nv(1, .085, .6), nv(-1, 0, -1)},
	} {
		contact, moved := nativeQueueContact(n, fixture.start, fixture.inward)
		if !moved && contact != fixture.start || contact.Y != fixture.start.Y || math.Abs(contact.X) > 1+1e-9 || math.Abs(contact.Z) > .6+1e-9 {
			t.Fatalf("invalid flat in-footprint contact: %+v -> %+v", fixture.start, contact)
		}
		found := false
		for _, triangle := range b.triangles {
			found = found || contactTestOnTriangle(contact, triangle)
		}
		if !found {
			t.Fatalf("contact misses actual tessellated barrel/cap: %+v", contact)
		}
	}
}

func TestQueueContactRoutesPreserveSourceVerticesAndBounds(t *testing.T) {
	source := contactTestQueue()
	target := source
	target.ID, target.Position.Z = "other-queue", 3
	points := []Vec{nv(0, .085, .6), nv(0, .085, 1.4), nv(.2, .085, 1.4), nv(.2, .085, 2.4)}
	e := d2isometric.Edge{ID: "contact", Source: source.ID, Target: target.ID, Opacity: 1, Points: points}
	before := append([]Vec(nil), points...)
	paths, err := nativeSolidContactRoutes(context.Background(), []d2isometric.Edge{e}, []d2isometric.Node{source, target}, [][]Vec{points})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths[0]) != len(points)+2 || !reflect.DeepEqual(paths[0][1:len(paths[0])-1], before) || !reflect.DeepEqual(points, before) || !reflect.DeepEqual(e.Points, before) {
		t.Fatal("contact extension changed an authored endpoint or exterior route bend")
	}
	for _, p := range paths[0] {
		if p.Y != .085 {
			t.Fatal("queue contact raised the planar route")
		}
	}
	for _, fixture := range []struct{ start, direction Vec }{
		{nv(0, .085, .6), nv(1, 0, 0)},  // tangent never reaches the narrowed barrel
		{nv(0, 0, .6), nv(0, 0, -1)},    // below the source floor
		{nv(3, .085, .6), nv(-1, 0, 0)}, // unrelated exterior point
	} {
		if p, moved := nativeQueueContact(source, fixture.start, fixture.direction); moved || p != fixture.start {
			t.Fatal("invalid contact invented an external route segment")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := nativeSolidContactRoutes(ctx, []d2isometric.Edge{e}, []d2isometric.Node{source}, [][]Vec{points}); !errors.Is(err, context.Canceled) {
		t.Fatal("queue contacts ignored cancellation")
	}
}

func TestQueueContactsMatchFlattenedPrintedCrown(t *testing.T) {
	n := contactTestQueue()
	n.Label, n.Metadata.Original.Label = "Work queue", "Work queue"
	n.Metadata.Original.FontSize, n.Metadata.Original.LabelWidth, n.Metadata.Original.LabelHeight = 16, 100, 28
	n.Metadata.Original.LabelPosition = "INSIDE_TOP_CENTER"
	physical := solidTestPhysical(solidTestBuild(t, n).triangles)
	if nativeQueueCrown(n) >= 1 {
		t.Fatal("fixture does not exercise the flattened crown")
	}
	mesh := append([]Triangle(nil), physical...)
	floor, relief := n.Position.Y-n.Size.Y/2, hierarchyNodeRelief(n)
	for i := range mesh {
		for j := range mesh[i].V {
			p := &mesh[i].V[j].Position
			p.Y = floor + (p.Y-floor)*relief
		}
	}
	for _, height := range []float64{.015, n.Size.Y * .4, n.Size.Y * .9} {
		for _, sign := range []float64{-1, 1} {
			start := nv(0, floor+height*relief, sign*n.Size.Z/2)
			contact, _ := nativeQueueContact(n, start, nv(0, 0, -sign))
			found := false
			for _, triangle := range mesh {
				found = found || contactTestOnTriangle(contact, triangle)
			}
			if !found || contact.Y != start.Y {
				t.Fatalf("printed crown and flat route disagree: %+v", contact)
			}
		}
	}
}

func TestQueueContactArrowsTrafficAndMainCaptionUseCorrectAnchors(t *testing.T) {
	testSolidContactArrowsTrafficAndMainCaption(t, contactTestQueue())
}

func TestUprightContactArrowsTrafficAndMainCaptionUseCorrectAnchors(t *testing.T) {
	for _, shape := range []string{d2target.ShapeCylinder, d2target.ShapeCircle, d2target.ShapeOval, d2target.ShapeHexagon} {
		t.Run(shape, func(t *testing.T) {
			n := contactTestQueue()
			n.Type, n.Metadata.Original.Type = shape, shape
			testSolidContactArrowsTrafficAndMainCaption(t, n)
		})
	}
}

func testSolidContactArrowsTrafficAndMainCaption(t *testing.T, queue d2isometric.Node) {
	t.Helper()
	ctx := context.Background()
	e := d2isometric.Edge{ID: "out", Source: queue.ID, Target: "package", Opacity: 1, StrokeWidth: 2, SourceArrow: d2target.TriangleArrowhead, TargetArrow: d2target.TriangleArrowhead,
		// An off-center port lies outside both elliptical and hexagonal walls;
		// the front midpoint already touches an unbeveled upright solid.
		Label: "main", Points: []Vec{nv(.75, .085, .6), nv(.75, .085, 2.4)}}
	e.Metadata.Original.Text = d2target.Text{Label: "main", LabelWidth: 60, LabelHeight: 20, FontSize: 16}
	e.Metadata.Original.LabelPosition, e.Metadata.Original.LabelPercentage, e.Metadata.Original.Animated = "UNLOCKED_MIDDLE", .5, true
	painter, err := newTextPainter(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	b := &meshBuilder{ctx: ctx, scale: .01, text: painter}
	packets := b.edges([]d2isometric.Edge{e}, newRouteCaptionPlacer(), &d2isometric.Scene{Nodes: []d2isometric.Node{queue}})
	if b.err != nil || len(packets) != 1 {
		t.Fatal("missing contact route", b.err)
	}
	path := packets[0].points
	if len(path) != 3 || path[1] != e.Points[0] || path[2] != e.Points[1] || path[0].Z >= e.Points[0].Z {
		t.Fatal("traffic does not terminate at the rendered queue contact")
	}
	expected := &meshBuilder{ctx: ctx, scale: .01}
	mat := nativeMaterial(nativeRouteTint(e), .6, .05, 1)
	expected.arrowWithOpacity(string(e.SourceArrow), path[0], nsub(path[0], path[1]), mat, e.StrokeWidth, 1)
	expected.arrowWithOpacity(string(e.TargetArrow), path[2], nsub(path[2], path[1]), mat, e.StrokeWidth, 1)
	var markers []Triangle
	for _, triangle := range b.triangles {
		if triangle.DepthBias > .001 && triangle.Material.Texture == nil {
			markers = append(markers, triangle)
		}
	}
	if len(markers) != len(expected.triangles) || len(markers) != 2 {
		t.Fatalf("queue contact changed arrow count: %d triangles, want two intact triangle heads", len(markers))
	}
	for i := range markers {
		for j := range markers[i].V {
			if markers[i].V[j].Position != expected.triangles[i].V[j].Position {
				t.Fatal("arrowhead remained at the old floating endpoint")
			}
		}
	}
	width, depth := nativeRouteCaptionSize(e, .5, e.Metadata.Original.Text, false, .01)
	anchor, _ := nativeConnectionCaptionSurface(e.Points, e.Metadata.Original, width, depth, .01)
	anchor.center.Y += nativeRouteRadius(e)
	for _, triangle := range b.triangles {
		if triangle.Material.Texture != nil {
			center := nmul(nadd(triangle.V[0].Position, triangle.V[2].Position), .5)
			if nlen(nsub(center, anchor.center)) > 1e-9 {
				t.Fatal("connector extension displaced the authored main caption")
			}
			return
		}
	}
	t.Fatal("main caption disappeared")
}

func TestUprightContactsReachActualSharpRimsAndWalls(t *testing.T) {
	for _, shape := range []string{d2target.ShapeCylinder, d2target.ShapeCircle, d2target.ShapeOval, d2target.ShapeHexagon} {
		t.Run(shape, func(t *testing.T) {
			n := contactTestQueue()
			n.Type, n.Metadata.Original.Type = shape, shape
			for _, threeDee := range []bool{false, true} {
				n.Metadata.Original.ThreeDee = threeDee
				b := &meshBuilder{ctx: context.Background(), scale: .01}
				b.solidNode(n)
				if b.err != nil {
					t.Fatal(b.err)
				}
				floor, relief := n.Position.Y-n.Size.Y/2, 1.
				relief = hierarchyNodeRelief(n)
				for i := range b.triangles {
					for j := range b.triangles[i].V {
						v := &b.triangles[i].V[j]
						v.Position.Y = floor + (v.Position.Y-floor)*relief
					}
				}
				for _, y := range []float64{.01, nativeSolidHeight(n) / 2, nativeSolidHeight(n) - .01} {
					for _, fixture := range []struct{ start, inward Vec }{
						{nv(1, 0, .24), nv(-1, 0, 0)}, {nv(-1, 0, -.24), nv(1, 0, 0)},
						{nv(.75, 0, .6), nv(0, 0, -1)}, {nv(-.75, 0, -.6), nv(0, 0, 1)},
						{nv(1, 0, .6), nv(-1, 0, -1)},
					} {
						fixture.start.Y = floor + y*relief
						contact, moved := nativeUprightContact(n, fixture.start, fixture.inward)
						if !moved || contact.Y != fixture.start.Y || math.Abs(contact.X) > 1+1e-9 || math.Abs(contact.Z) > .6+1e-9 {
							t.Fatalf("invalid flat contact: %+v -> %+v, 3d=%v", fixture.start, contact, threeDee)
						}
						found := false
						for _, triangle := range b.triangles {
							found = found || contactTestOnTriangle(contact, triangle)
						}
						if !found {
							t.Fatalf("contact misses actual upright wall: %+v, 3d=%v", contact, threeDee)
						}
					}
				}
			}
		})
	}
}

func TestCylinderContactClosesRealRingBufferGaps(t *testing.T) {
	// Compiled ELK geometry from the real-world golang-queue workers diagram:
	// both right ports lie on the source cylinder's straight outline, outside
	// the new solid's ellipse. Their former gap is several output pixels wide.
	n := contactTestQueue()
	n.ID, n.Type, n.Metadata.Original.Type = "database", d2target.ShapeCylinder, d2target.ShapeCylinder
	n.Size, n.Position = nv(1.58, 1.15, 1.7), nv(18.57, .645, 2.96)
	n.Metadata.Original.Width, n.Metadata.Original.Height = 158, 170
	for _, points := range [][]Vec{
		{nv(19.36, .08, 2.68), nv(20.45, .08, 2.678330078125), nv(20.45, .08, 1.68), nv(24.57, .08, 1.68)},
		{nv(19.36, .08, 3.25), nv(22.57, .08, 3.245), nv(22.57, .08, 5), nv(24.57, .08, 5)},
	} {
		edge := d2isometric.Edge{Source: n.ID, Target: "consumer", Opacity: 1, Points: points}
		before := append([]Vec(nil), points...)
		paths, err := nativeSolidContactRoutes(context.Background(), []d2isometric.Edge{edge}, []d2isometric.Node{n}, [][]Vec{points})
		if err != nil {
			t.Fatal(err)
		}
		if len(paths[0]) != len(points)+1 || !reflect.DeepEqual(paths[0][1:], before) || !reflect.DeepEqual(edge.Points, before) {
			t.Fatal("cylinder contact changed a source port, bend, or route metadata")
		}
		gap := nlen(nsub(paths[0][0], before[0]))
		if gap < .02 || gap > .12 || paths[0][0].Y != .08 {
			t.Fatalf("expected the observed planar cylinder gap to close, got %g: %+v", gap, paths[0][0])
		}
		b := &meshBuilder{ctx: context.Background(), scale: .01}
		b.solidNode(n)
		found := false
		floor := n.Position.Y - n.Size.Y/2
		for _, tri := range b.triangles {
			for i := range tri.V {
				tri.V[i].Position.Y = floor + (tri.V[i].Position.Y-floor)*hierarchyNodeRelief(n)
			}
			if contactTestOnTriangle(before[0], tri) {
				t.Fatal("regression fixture did not exhibit the original floating contact")
			}
			found = found || contactTestOnTriangle(paths[0][0], tri)
		}
		if !found {
			t.Fatal("corrected Ring Buffer route still misses the physical cylinder")
		}
	}
}

func TestUprightContactsDoNotInventExteriorRoutes(t *testing.T) {
	n := contactTestQueue()
	n.Type = d2target.ShapeCylinder
	for _, fixture := range []struct{ start, direction Vec }{
		{nv(1, .085, .24), nv(0, 0, 1)},  // tangent misses the curved side
		{nv(1, 0, .24), nv(-1, 0, 0)},    // below the floor
		{nv(3, .085, .24), nv(-1, 0, 0)}, // unrelated exterior endpoint
		{nv(0, .085, 0), nv(-1, 0, 0)},   // already inside the solid
		{nv(1, .085, .24), nv(0, 0, 0)},  // no usable endpoint tangent
		{nv(1, math.NaN(), .24), nv(-1, 0, 0)},
	} {
		if _, moved := nativeUprightContact(n, fixture.start, fixture.direction); moved {
			t.Fatal("invalid contact invented an exterior route segment")
		}
	}
}
