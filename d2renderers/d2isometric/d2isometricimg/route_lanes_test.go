package d2isometricimg

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2target"
)

func TestNativeLanesDistinctStableFlatAndOwned(t *testing.T) {
	edges := []d2isometric.Edge{}
	for _, id := range []string{"a", "b", "c", "d"} {
		edges = append(edges, bridgeTestEdge(id, nv(0, .08, 0), nv(5, .08, 0)))
	}
	// Direction does not reverse the stable physical ordering of lanes.
	edges[2].Points[0], edges[2].Points[1] = edges[2].Points[1], edges[2].Points[0]
	before, _ := json.Marshal(edges)
	paths, err := nativeLaneRoutes(context.Background(), edges, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	positions := []float64{}
	for i, path := range paths {
		if path[0] != edges[i].Points[0] || path[len(path)-1] != edges[i].Points[1] {
			t.Fatal("lane moved a port")
		}
		for _, p := range path {
			if p.Y != .08 || math.Abs(p.Z) > nativeLaneMaxOffset || p.X < 0 || p.X > 5 {
				t.Fatalf("lane escaped its local flat corridor: %+v", p)
			}
		}
		mid := pathPoint(path, routeLengths(path), .5)
		for _, z := range positions {
			if math.Abs(z-mid.Z) < 3.3*nativeRouteRadius(edges[i])+.03 {
				t.Fatalf("shared run remains indistinguishable: %v and %+v", positions, mid)
			}
		}
		positions = append(positions, mid.Z)
	}
	if positions[0] != 0 || positions[1] <= 0 || positions[2] >= 0 {
		t.Fatalf("unexpected canonical lane order: %v", positions)
	}
	for _, perm := range [][]int{{3, 1, 2, 0}, {2, 0, 3, 1}} {
		reordered := make([]d2isometric.Edge, len(edges))
		for i, j := range perm {
			reordered[i] = edges[j]
		}
		got, err := nativeLaneRoutes(context.Background(), reordered, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		for i, j := range perm {
			if !reflect.DeepEqual(paths[j], got[i]) {
				t.Fatalf("edge %s changed with input order", edges[j].ID)
			}
		}
	}
	after, _ := json.Marshal(edges)
	if string(before) != string(after) {
		t.Fatal("lane resolution mutated scene metadata or points")
	}
	paths[0][0].X = 100
	if edges[0].Points[0].X != 0 {
		t.Fatal("resolved route aliases source memory")
	}
}

func TestNativeLanesLocalPartialOverlap(t *testing.T) {
	edges := []d2isometric.Edge{
		bridgeTestEdge("a", nv(4, .08, 0), nv(6, .08, 0)),
		bridgeTestEdge("z", nv(0, .08, 0), nv(10, .08, 0)),
	}
	paths, err := nativeLaneRoutes(context.Background(), edges, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	shifted := false
	for _, p := range paths[1] {
		if p.Z != 0 {
			shifted = true
			if p.X < 3.5 || p.X > 6.5 {
				t.Fatalf("nonshared distant run was displaced: %+v", p)
			}
		}
	}
	if !shifted {
		t.Fatal("partial collinear overlap was not separated")
	}
	if !reflect.DeepEqual(paths[0], edges[0].Points) {
		t.Fatal("reference lane moved")
	}
	// Identical diagonal runs are separated in their common perpendicular,
	// without relying on an axis-only special case.
	edges = []d2isometric.Edge{bridgeTestEdge("a", nv(0, .08, 0), nv(3, .08, 4)), bridgeTestEdge("b", nv(0, .08, 0), nv(3, .08, 4))}
	paths, err = nativeLaneRoutes(context.Background(), edges, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	shift := nsub(pathPoint(paths[1], routeLengths(paths[1]), .5), nv(1.5, .08, 2))
	if math.Abs(ndot(shift, nv(.6, 0, .8))) > 1e-9 || nlen(shift) < .1 {
		t.Fatalf("diagonal lane did not use a local perpendicular: %+v", shift)
	}
}

func TestNativeLanesAvoidModulesHeadersAndNeighbors(t *testing.T) {
	edges := []d2isometric.Edge{bridgeTestEdge("a", nv(0, .08, 0), nv(5, .08, 0)), bridgeTestEdge("b", nv(0, .08, 0), nv(5, .08, 0))}
	block := d2isometric.Node{ID: "block", Position: nv(2.5, .6, .45), Size: nv(3, 1, .3), Opacity: 1}
	paths, err := nativeLaneRoutes(context.Background(), edges, []d2isometric.Node{block}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if mid := pathPoint(paths[1], routeLengths(paths[1]), .5); mid.Z >= 0 {
		t.Fatalf("lane moved through the nearby module instead of using the open side: %+v", mid)
	}
	// Check the exact rounded candidate by dense independent point samples.
	curve := nativeRoundedRoute(paths[1])
	pad := 1.65*nativeRouteRadius(edges[1]) + .02
	for i := 1; i < len(curve); i++ {
		for j := 0; j <= 100; j++ {
			p := nlerp(curve[i-1], curve[i], float64(j)/100)
			if math.Abs(p.X-block.Position.X) < block.Size.X/2+.08+pad && math.Abs(p.Z-block.Position.Z) < block.Size.Z/2+.17+pad {
				t.Fatal("rounded lane cuts a physical module footprint")
			}
		}
	}
	other := block
	other.ID, other.Position.Z = "other-block", -.45
	blocked, err := nativeLaneRoutes(context.Background(), edges, []d2isometric.Node{block, other}, nil)
	if err != nil || !reflect.DeepEqual(blocked[1], edges[1].Points) {
		t.Fatalf("closed aisle should retain original route: %v, %v", blocked, err)
	}
	board := d2isometric.Board{ID: "board", Label: "Reserved title", Position: nv(2.5, 0, 1.06), Size: nv(6, .14, 2), HeaderDepth: .4}
	paths, err = nativeLaneRoutes(context.Background(), edges, nil, []d2isometric.Board{board})
	if err != nil {
		t.Fatal(err)
	}
	if mid := pathPoint(paths[1], routeLengths(paths[1]), .5); mid.Z >= 0 {
		t.Fatal("lane entered a reserved board title strip")
	}
	neighbor := bridgeTestEdge("neighbor", nv(0, .08, .2), nv(5, .08, .2))
	paths, err = nativeLaneRoutes(context.Background(), append(edges, neighbor), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if mid := pathPoint(paths[1], routeLengths(paths[1]), .5); mid.Z >= 0 {
		t.Fatal("lane collapsed an existing nearby parallel route")
	}
	if !reflect.DeepEqual(paths[2], neighbor.Points) {
		t.Fatal("unshared neighbor was changed")
	}
}

func TestNativeLanesLeaveNonoverlapsAndInvisibleRoutes(t *testing.T) {
	base := bridgeTestEdge("a", nv(0, .08, 0), nv(5, .08, 0))
	for name, other := range map[string]d2isometric.Edge{
		"touch only":  bridgeTestEdge("b", nv(5, .08, 0), nv(8, .08, 0)),
		"cross only":  bridgeTestEdge("b", nv(2, .08, -2), nv(2, .08, 2)),
		"other plane": bridgeTestEdge("b", nv(0, .5, 0), nv(5, .5, 0)),
		"parallel":    bridgeTestEdge("b", nv(0, .08, 1), nv(5, .08, 1)),
	} {
		t.Run(name, func(t *testing.T) {
			edges := []d2isometric.Edge{base, other}
			paths, err := nativeLaneRoutes(context.Background(), edges, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			for i := range edges {
				if !reflect.DeepEqual(paths[i], edges[i].Points) {
					t.Fatal("nonoverlap was changed")
				}
			}
		})
	}
	for _, mode := range []string{"opacity", "paint", "width"} {
		invisible := base
		invisible.ID = "b"
		switch mode {
		case "opacity":
			invisible.Opacity = 0
		case "width":
			invisible.StrokeWidth = 0
		case "paint":
			invisible.StrokeExplicit = true
			invisible.Stroke = "transparent"
		}
		paths, err := nativeLaneRoutes(context.Background(), []d2isometric.Edge{base, invisible}, nil, nil)
		if err != nil || !reflect.DeepEqual(paths[0], base.Points) || !reflect.DeepEqual(paths[1], invisible.Points) {
			t.Fatalf("invisible %s altered the visible path", mode)
		}
	}
}

func TestNativeLanesPipelineUsesSamePacketGeometry(t *testing.T) {
	edges := []d2isometric.Edge{
		bridgeTestEdge("a", nv(0, .08, 0), nv(5, .08, 0)),
		bridgeTestEdge("b", nv(0, .08, 0), nv(5, .08, 0)),
		bridgeTestEdge("z", nv(2.5, .08, -2), nv(2.5, .08, 2)),
	}
	for i := range edges {
		edges[i].Metadata.Original.Animated = true
	}
	edges[1].StrokeDash = 3
	lanes, err := nativeLaneRoutes(context.Background(), edges, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	resolved := append([]d2isometric.Edge(nil), edges...)
	for i := range resolved {
		resolved[i].Points = lanes[i]
	}
	want, err := nativeBridgeRoutes(context.Background(), resolved)
	if err != nil {
		t.Fatal(err)
	}
	b := &meshBuilder{ctx: context.Background()}
	packets := b.edges(edges, newRouteCaptionPlacer())
	if b.err != nil || len(packets) != len(edges) {
		t.Fatalf("integrated routes failed: %v", b.err)
	}
	for i := range packets {
		if !reflect.DeepEqual(packets[i].points, want[i]) {
			t.Fatal("wire pipeline and traffic use different routes")
		}
	}
	if pathPoint(packets[1].points, packets[1].lengths, .5).Z == 0 {
		t.Fatal("integrated dashed route lost its lane")
	}
	peak := .08
	for _, p := range packets[2].points {
		peak = max(peak, p.Y)
	}
	if peak <= .08 {
		t.Fatal("crossing detection did not run after lane resolution")
	}
}

func TestNativeLanesAdmissionAndCancellation(t *testing.T) {
	edges := []d2isometric.Edge{bridgeTestEdge("a", nv(0, .08, 0), nv(5, .08, 0)), bridgeTestEdge("b", nv(0, .08, 0), nv(5, .08, 0))}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := nativeLaneRoutes(ctx, edges, nil, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("lost cancellation: %v", err)
	}
	if _, err := nativeResolveLaneRoutes(context.Background(), edges, nil, nil, 10); err == nil || !strings.Contains(err.Error(), "work limit") {
		t.Fatalf("work limit not enforced: %v", err)
	}
	if _, err := nativeLaneRoutes(nil, edges, nil, nil); err == nil {
		t.Fatal("nil context accepted")
	}
	edges[0].Points[0].X = math.NaN()
	if _, err := nativeLaneRoutes(context.Background(), edges, nil, nil); err == nil {
		t.Fatal("NaN accepted")
	}
	edges[0].Points = make([]Vec, 10001)
	if _, err := nativeLaneRoutes(context.Background(), edges, nil, nil); err == nil {
		t.Fatal("oversized route accepted")
	}
}

func TestNativeLanesContinueThroughSharedBends(t *testing.T) {
	for _, turn := range []float64{-1, 1} {
		edges := []d2isometric.Edge{}
		for _, id := range []string{"a", "b", "c"} {
			edges = append(edges, bridgeTestEdge(id, nv(0, .08, 0), nv(4, .08, 0), nv(4, .08, turn*4)))
		}
		paths, err := nativeLaneRoutes(context.Background(), edges, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		for i := 1; i < len(paths); i++ {
			if paths[i][0] != edges[i].Points[0] || paths[i][len(paths[i])-1] != edges[i].Points[2] {
				t.Fatal("continuous corner changed a port")
			}
			nearCorner := false
			for _, p := range paths[i] {
				if p.Y != .08 {
					t.Fatal("corner left plane")
				}
				if nlen(nsub(p, nv(4, .08, 0))) < .6 {
					nearCorner = true
					if math.Abs(p.X-4) < .05 || math.Abs(p.Z) < .05 {
						t.Fatalf("lane pinched onto centerline at shared bend: %+v", p)
					}
				}
			}
			if !nearCorner {
				t.Fatal("shared bend was skipped")
			}
			curve := nativeRoundedRoute(paths[i])
			for k := 1; k < len(curve); k++ {
				for s := 0; s <= 30; s++ {
					p := nlerp(curve[k-1], curve[k], float64(s)/30)
					if nlen(nsub(p, nv(4, .08, 0))) < .5 && math.Abs(p.Z) < .04 && p.X < 4-.04 {
						t.Fatal("lane exchanged order across original horizontal run")
					}
				}
			}
		}
		reversed, err := nativeLaneRoutes(context.Background(), []d2isometric.Edge{edges[2], edges[0], edges[1]}, nil, nil)
		if err != nil || !reflect.DeepEqual(reversed[0], paths[2]) || !reflect.DeepEqual(reversed[1], paths[0]) || !reflect.DeepEqual(reversed[2], paths[1]) {
			t.Fatal("continuous corners depend on input order")
		}
	}
}

func TestNativeLanesDoNotEnterEndpointOnLaterRun(t *testing.T) {
	node := d2isometric.Node{ID: "own", Position: nv(2.5, .6, .45), Size: nv(3, 1, .3), Opacity: 1}
	edges := []d2isometric.Edge{
		bridgeTestEdge("a", nv(0, .08, 0), nv(5, .08, 0)),
		bridgeTestEdge("b", nv(1, .08, .45), nv(.5, .08, .45), nv(.5, .08, -1), nv(0, .08, -1), nv(0, .08, 0), nv(5, .08, 0)),
	}
	edges[1].Source = "own"
	paths, err := nativeLaneRoutes(context.Background(), edges, []d2isometric.Node{node}, nil)
	if err != nil {
		t.Fatal(err)
	}
	shifted := false
	for i := 1; i < len(paths[1]); i++ {
		for step := 0; step <= 50; step++ {
			p := nlerp(paths[1][i-1], paths[1][i], float64(step)/50)
			if p.X > 1.5 && p.X < 4 && p.Z != 0 {
				shifted = true
				if p.Z > 0 {
					t.Fatal("later lane entered its own source component")
				}
			}
		}
	}
	if !shifted {
		t.Fatal("open side of the later run was not used")
	}
}

func TestNativeEdgeIconOnlyAndTextCaption(t *testing.T) {
	for _, label := range []string{"", "Request"} {
		ctx := context.Background()
		icons, err := newSurfaceIconPainter(ctx, 1, nil)
		if err != nil {
			t.Fatal(err)
		}
		painter, err := newTextPainter(ctx, 3)
		if err != nil {
			t.Fatal(err)
		}
		u := iconData(t, "image/svg+xml", []byte(surfaceTestSVG))
		edge := bridgeTestEdge("icon-edge", nv(0, .08, 0), nv(5, .08, 0))
		edge.Label, edge.Icon = label, u.String()
		edge.Metadata.Original.Icon = u
		edge.Metadata.Original.Animated = true
		edge.Metadata.Original.Text = d2target.Text{Label: label, LabelWidth: 90, LabelHeight: 20, FontSize: 16}
		before, _ := json.Marshal(edge)
		b := &meshBuilder{ctx: ctx, scale: .01, text: painter, icons: icons}
		packets := b.edges([]d2isometric.Edge{edge}, newRouteCaptionPlacer())
		if b.err != nil || len(packets) != 1 {
			t.Fatalf("edge icon render failed: %v", b.err)
		}
		textures := map[*Material]bool{}
		for _, tri := range b.triangles {
			if tri.Material.Texture != nil {
				textures[tri.Material] = true
			}
		}
		want := 1
		if label != "" {
			want++
		}
		if len(textures) != want {
			t.Fatalf("label=%q: got %d textured faces, want %d", label, len(textures), want)
		}
		after, _ := json.Marshal(edge)
		if string(before) != string(after) {
			t.Fatal("icon integration mutated edge metadata")
		}
	}
}
