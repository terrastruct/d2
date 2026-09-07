package d2isometricimg

import (
	"context"
	"image"
	"image/color"
	"math"
	"reflect"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2isometric"
)

func groundReceiverTestTriangle(receiver *float64) Triangle {
	strength, alpha := .37, uint8(153)
	return Triangle{
		V: [3]Vertex{
			{Position: nv(-.8, .6, -.3), Normal: nv(0, 1, 0), U: .1, V: .2},
			{Position: nv(.7, .8, -.2), Normal: nv(.1, 1, 0), U: .8, V: .1},
			{Position: nv(.2, 1, .9), Normal: nv(0, 1, .1), U: .6, V: .9},
		},
		Material:        &Material{Color: color.NRGBA{R: 89, G: 137, B: 181, A: 179}, Roughness: .68, Texture: image.NewNRGBA(image.Rect(0, 0, 2, 2))},
		CastShadow:      true,
		ShadowGround:    receiver,
		ShadowOpacity:   &strength,
		ShadowFillAlpha: &alpha,
		DepthBias:       .003,
		NoDepthWrite:    true,
		OpacityGroup:    &nativeOpacityGroup{Opacity: .6},
		PaintOwner:      &nativePaintOwner{Opaque: false},
	}
}

func groundReceiverTestShadow(p Vec, ground float64) Vec {
	light := rasterShadowDirection()
	q := nsub(p, nmul(light, (p.Y-ground-rasterShadowNormalOffset)/light.Y))
	q.Y = ground
	return q
}

func groundReceiverTestSameProjection(t *testing.T, camera rasterCamera, a, b Vec) {
	t.Helper()
	p, q := camera.project(a), camera.project(b)
	if math.Abs(p.x-q.x) > 1e-9 || math.Abs(p.y-q.y) > 1e-9 {
		t.Fatalf("ground normalization moved visible projection: (%g,%g) -> (%g,%g)", p.x, p.y, q.x, q.y)
	}
}

func TestGroundReceiversPreserveMeshAndShadowProjection(t *testing.T) {
	first, second, ground := .08, -.31, -.56
	ts := []Triangle{groundReceiverTestTriangle(&first), groundReceiverTestTriangle(&second), groundReceiverTestTriangle(nil), groundReceiverTestTriangle(&ground)}
	before := append([]Triangle(nil), ts...)
	view := nativeViewDirection()
	camera := rasterFit(ts, view, 640, 480, 1.08)
	got := rasterGroundTriangles(ts, ground, view)
	if len(got) != len(ts) || &got[0] == &ts[0] {
		t.Fatal("receiver translation did not preserve triangle count or copied mutable source geometry")
	}
	for i, triangle := range got {
		source := before[i]
		if triangle.Material != source.Material || triangle.OpacityGroup != source.OpacityGroup || triangle.PaintOwner != source.PaintOwner || triangle.ShadowOpacity != source.ShadowOpacity || triangle.ShadowFillAlpha != source.ShadowFillAlpha || triangle.CastShadow != source.CastShadow || triangle.NoDepthWrite != source.NoDepthWrite || triangle.DepthBias != source.DepthBias {
			t.Fatal("receiver translation changed material, source opacity, paint ownership, or depth behavior")
		}
		receiver := ground
		if source.ShadowGround != nil {
			receiver = *source.ShadowGround
		}
		for j, vertex := range triangle.V {
			original := source.V[j]
			if vertex.U != original.U || vertex.V != original.V || vertex.Normal != original.Normal {
				t.Fatal("receiver translation changed source texture coordinates or lighting normals")
			}
			groundReceiverTestSameProjection(t, camera, original.Position, vertex.Position)
			if math.Abs((vertex.Position.Y-ground)-(original.Position.Y-receiver)) > 1e-12 {
				t.Fatal("receiver translation changed caster height above its own receiver")
			}
			groundReceiverTestSameProjection(t, camera,
				groundReceiverTestShadow(original.Position, receiver),
				groundReceiverTestShadow(vertex.Position, ground))
		}
	}
	if !reflect.DeepEqual(got[2], before[2]) || !reflect.DeepEqual(got[3], before[3]) {
		t.Fatal("untagged or same-plane geometry changed in a mixed receiver scene")
	}
	if !reflect.DeepEqual(ts, before) || first != .08 || second != -.31 || ground != -.56 {
		t.Fatal("receiver normalization mutated the caller's mesh or receiver pointers")
	}
	got[0].V[0].Position.X += 10
	if ts[0].V[0].Position != before[0].V[0].Position {
		t.Fatal("normalized mesh retained the caller's vertex storage")
	}
}

func TestGroundReceiversKeepIndependentRootShadowsStable(t *testing.T) {
	root, nested := -.02, -.22
	ts := []Triangle{groundReceiverTestTriangle(&root), groundReceiverTestTriangle(&nested)}
	for j := range ts[1].V {
		ts[1].V[j].Position.X += 8
	}
	view := nativeViewDirection()
	camera := rasterFit(ts, view, 640, 480, 1.08)
	before := rasterGroundTriangles(ts, -.22, view)
	// An unrelated branch introduces a lower common raster plane. Its own
	// receiver changes too, while the first root retains its existing plane.
	deeper := -.52
	changed := append([]Triangle(nil), ts...)
	changed[1].ShadowGround = &deeper
	after := rasterGroundTriangles(changed, -.52, view)
	for j := range ts[0].V {
		groundReceiverTestSameProjection(t, camera,
			groundReceiverTestShadow(before[0].V[j].Position, -.22),
			groundReceiverTestShadow(after[0].V[j].Position, -.52))
	}
	changedProjection := false
	for j := range ts[1].V {
		a := camera.project(groundReceiverTestShadow(before[1].V[j].Position, -.22))
		b := camera.project(groundReceiverTestShadow(after[1].V[j].Position, -.52))
		changedProjection = changedProjection || math.Abs(a.x-b.x) > 1e-3 || math.Abs(a.y-b.y) > 1e-3
		groundReceiverTestSameProjection(t, camera,
			groundReceiverTestShadow(ts[1].V[j].Position, deeper),
			groundReceiverTestShadow(after[1].V[j].Position, -.52))
	}
	if !changedProjection {
		t.Fatal("changing the nested receiver did not affect its own shadow")
	}
	if ts[1].ShadowGround != &nested || nested != -.22 || root != -.02 {
		t.Fatal("independent receiver normalization changed the source domain")
	}
}

func TestHierarchyShadowReceiversFollowIndependentVisibleRoots(t *testing.T) {
	boards := []d2isometric.Board{
		{ID: "layout", Kind: "ungrouped"},
		{ID: "a", ParentID: "layout", Kind: "platform"},
		{ID: "a.wrapper", ParentID: "a", Kind: "ungrouped"},
		{ID: "a.child", ParentID: "a.wrapper", Kind: "terrace"},
		{ID: "b", ParentID: "layout", Kind: "platform"},
		{ID: "b.child", ParentID: "b", Kind: "terrace"},
	}
	ts := make([]Triangle, len(boards)+1)
	spans := make([]hierarchyShadowSpan, len(boards))
	for i, board := range boards {
		ts[i] = groundReceiverTestTriangle(nil)
		bottom := .07
		switch board.ID {
		case "a":
			bottom = -.102
		case "b":
			bottom = -.302
		case "b.child":
			bottom = -.102
		}
		ts[i].V[0].Position.Y = bottom
		spans[i] = hierarchyShadowSpan{first: i, last: i + 1, board: board.ID}
	}
	ts[len(boards)] = groundReceiverTestTriangle(nil) // edges are outside all object spans
	before := append([]Triangle(nil), ts...)
	hierarchyShadowReceivers(ts, boards, spans)
	for i, want := range []float64{.08, -.022, -.022, -.022, -.222, -.222} {
		if ts[i].ShadowGround == nil || math.Abs(*ts[i].ShadowGround-want) > 1e-12 {
			t.Fatalf("wrong visible-root receiver for %s: %v, want %g", boards[i].ID, ts[i].ShadowGround, want)
		}
	}
	if ts[1].ShadowGround != ts[2].ShadowGround || ts[1].ShadowGround != ts[3].ShadowGround || ts[4].ShadowGround != ts[5].ShadowGround || ts[1].ShadowGround == ts[4].ShadowGround || ts[0].ShadowGround == ts[1].ShadowGround {
		t.Fatal("shadow receiver identity does not follow independent visible-root containment")
	}
	for i := range ts {
		if ts[i].V != before[i].V || ts[i].Material != before[i].Material {
			t.Fatal("receiver assignment changed source geometry or paint")
		}
	}
	if ts[len(boards)].ShadowGround != nil {
		t.Fatal("receiver assignment tagged geometry outside every source domain")
	}
	// Only the second root descends further; the shared invisible layout
	// wrapper must not let that change pull down the first root's receiver.
	ts[4].V[0].Position.Y -= .2
	hierarchyShadowReceivers(ts, boards, spans)
	if math.Abs(*ts[1].ShadowGround+.022) > 1e-12 || math.Abs(*ts[2].ShadowGround+.022) > 1e-12 || math.Abs(*ts[3].ShadowGround+.022) > 1e-12 || math.Abs(*ts[4].ShadowGround+.422) > 1e-12 || math.Abs(*ts[5].ShadowGround+.422) > 1e-12 {
		t.Fatal("lowering one visible root changed another root's ground receiver")
	}
}

func TestGroundReceiversKeepUnchangedSlice(t *testing.T) {
	ground := -.24
	for _, receiver := range []*float64{nil, &ground} {
		ts := []Triangle{groundReceiverTestTriangle(receiver)}
		got := rasterGroundTriangles(ts, ground, nativeViewDirection())
		if len(got) != 1 || &got[0] != &ts[0] {
			t.Fatal("unchanged receiver needlessly copied source geometry")
		}
	}
	if got := rasterGroundTriangles(nil, ground, nativeViewDirection()); got != nil {
		t.Fatal("empty receiver normalization changed nil input")
	}
}

func TestGroundReceiverSnapshotOwnsReceiverPointers(t *testing.T) {
	receiver := -.23
	ts := []Triangle{groundReceiverTestTriangle(&receiver), groundReceiverTestTriangle(&receiver), groundReceiverTestTriangle(nil)}
	frozen, err := rasterSnapshot(context.Background(), ts)
	if err != nil {
		t.Fatal(err)
	}
	for _, triangle := range frozen[:2] {
		if triangle.ShadowGround == nil || triangle.ShadowGround == &receiver || *triangle.ShadowGround != -.23 {
			t.Fatal("snapshot retained caller-owned receiver storage or changed its value")
		}
	}
	if frozen[2].ShadowGround != nil {
		t.Fatal("snapshot invented a receiver for an untagged triangle")
	}
	receiver = -.61
	for _, triangle := range frozen[:2] {
		if *triangle.ShadowGround != -.23 {
			t.Fatal("changing caller receiver moved an immutable deferred snapshot")
		}
	}
	*frozen[0].ShadowGround = .04
	if receiver != -.61 || *ts[0].ShadowGround != -.61 || *ts[1].ShadowGround != -.61 {
		t.Fatal("changing snapshot receiver leaked into caller state")
	}
}

func TestGroundReceiverValidationRejectsInvalidPlanes(t *testing.T) {
	ctx := context.Background()
	for _, receiver := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), rasterCoordinateLimit + 1, -rasterCoordinateLimit - 1} {
		triangle := groundReceiverTestTriangle(&receiver)
		if err := rasterValidate(ctx, []Triangle{triangle}); err == nil {
			t.Fatalf("accepted invalid shadow receiver %g", receiver)
		}
		if _, err := rasterSnapshot(ctx, []Triangle{triangle}); err == nil {
			t.Fatalf("snapshotted invalid shadow receiver %g", receiver)
		}
	}
	for _, receiver := range []float64{0, -.6, .08, -rasterCoordinateLimit, rasterCoordinateLimit} {
		if err := rasterValidate(ctx, []Triangle{groundReceiverTestTriangle(&receiver)}); err != nil {
			t.Fatalf("rejected bounded finite shadow receiver %g: %v", receiver, err)
		}
	}
}
