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
)

func bridgeTestEdge(id string, points ...Vec) d2isometric.Edge {
	return d2isometric.Edge{ID: id, Source: id + "-source", Target: id + "-target", StrokeWidth: 2, Opacity: 1, Points: points}
}

func TestNativeBridgeCrossingClearanceAndOwnership(t *testing.T) {
	for _, width := range []int{1, 2, 15} {
		edges := []d2isometric.Edge{
			bridgeTestEdge("a-under", nv(-2, .08, 0), nv(2, .08, 0)),
			bridgeTestEdge("b-over", nv(0, .08, -2), nv(0, .08, 2)),
		}
		for i := range edges {
			edges[i].StrokeWidth = width
		}
		before, _ := json.Marshal(edges)
		paths, err := nativeBridgeRoutes(context.Background(), edges)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(paths[0], edges[0].Points) {
			t.Fatal("underpass moved")
		}
		upper := paths[1]
		if upper[0] != edges[1].Points[0] || upper[len(upper)-1] != edges[1].Points[1] {
			t.Fatal("a port moved")
		}
		if upper[1].Y != .08 || upper[len(upper)-2].Y != .08 {
			t.Fatal("zero-lift bridge boundaries left the exact connection plane")
		}
		peak := .08
		for i, p := range upper {
			if p.X != 0 || p.Z < -2 || p.Z > 2 || p.Y < .08 {
				t.Fatalf("bridge escaped the original XZ corridor: %+v", p)
			}
			peak = max(peak, p.Y)
			if i == 0 {
				continue
			}
			for j := 0; j <= 100; j++ {
				q := nlerp(upper[i-1], p, float64(j)/100)
				if d := math.Hypot(q.Z, q.Y-.08); d < 2*nativeRouteRadius(edges[0])+.005 {
					t.Fatalf("thickness %d: raised tube intersects underpass: distance=%g", width, d)
				}
			}
		}
		want := .08 + 2*nativeRouteRadius(edges[0]) + .05
		if math.Abs(peak-want) > 1e-10 || len(upper) != 11 {
			t.Fatalf("unexpected adaptive bridge: peak=%g want=%g points=%d", peak, want, len(upper))
		}
		lengths := routeLengths(upper)
		packet := pathPoint(upper, lengths, .5)
		if math.Abs(packet.Y-peak) > 1e-10 {
			t.Fatal("traffic interpolation missed bridge summit")
		}
		after, _ := json.Marshal(edges)
		if string(before) != string(after) {
			t.Fatal("input metadata or routes were mutated")
		}
		paths[0][0].Y = 55
		if edges[0].Points[0].Y != .08 {
			t.Fatal("resolved path aliases source points")
		}
	}
}

func TestNativeBridgeOnlyTrueCrossings(t *testing.T) {
	for name, pair := range map[string][2][]Vec{
		"disjoint":          {{nv(-2, .08, 0), nv(2, .08, 0)}, {nv(-2, .08, 1), nv(2, .08, 1)}},
		"collinear overlap": {{nv(-2, .08, 0), nv(2, .08, 0)}, {nv(-1, .08, 0), nv(3, .08, 0)}},
		"shared endpoint":   {{nv(-2, .08, 0), nv(0, .08, 0)}, {nv(0, .08, 0), nv(0, .08, 2)}},
		"tangent endpoint":  {{nv(-2, .08, 0), nv(2, .08, 0)}, {nv(0, .08, 0), nv(0, .08, 2)}},
		"different planes":  {{nv(-2, .08, 0), nv(2, .08, 0)}, {nv(0, 1, -2), nv(0, 1, 2)}},
		"near port":         {{nv(-2, .08, 0), nv(2, .08, 0)}, {nv(0, .08, -.2), nv(0, .08, 2)}},
		"shallow angle":     {{nv(-2, .08, 0), nv(2, .08, 0)}, {nv(-2, .08, -.2), nv(2, .08, .2)}},
	} {
		t.Run(name, func(t *testing.T) {
			edges := []d2isometric.Edge{bridgeTestEdge("a", pair[0]...), bridgeTestEdge("b", pair[1]...)}
			paths, err := nativeBridgeRoutes(context.Background(), edges)
			if err != nil {
				t.Fatal(err)
			}
			for i := range paths {
				if !reflect.DeepEqual(paths[i], edges[i].Points) {
					t.Fatalf("non-crossing path changed: %v", paths[i])
				}
			}
		})
	}
}

func TestNativeBridgeVisibilityAndDash(t *testing.T) {
	base := []d2isometric.Edge{
		bridgeTestEdge("a", nv(-2, .08, 0), nv(2, .08, 0)),
		bridgeTestEdge("b", nv(0, .08, -2), nv(0, .08, 2)),
	}
	want, err := nativeBridgeRoutes(context.Background(), base)
	if err != nil {
		t.Fatal(err)
	}
	base[1].StrokeDash = 4
	dashed, err := nativeBridgeRoutes(context.Background(), base)
	if err != nil || !reflect.DeepEqual(dashed, want) {
		t.Fatal("dashed wire and its packets need the same resolved crossing path")
	}
	for _, field := range []string{"opacity", "width", "paint"} {
		edges := append([]d2isometric.Edge(nil), base...)
		switch field {
		case "opacity":
			edges[0].Opacity = 0
		case "width":
			edges[0].StrokeWidth = 0
		case "paint":
			edges[0].StrokeExplicit, edges[0].Stroke = true, "transparent"
		}
		paths, err := nativeBridgeRoutes(context.Background(), edges)
		if err != nil || !reflect.DeepEqual(paths[1], edges[1].Points) {
			t.Fatalf("invisible %s produced a bridge", field)
		}
	}
}

func TestNativeBridgeStableOrderAndNearbyPlateau(t *testing.T) {
	edges := []d2isometric.Edge{
		bridgeTestEdge("z-over", nv(-3, .08, 0), nv(3, .08, 0)),
		bridgeTestEdge("b-under", nv(.15, .08, -2), nv(.15, .08, 2)),
		bridgeTestEdge("a-under", nv(-.15, .08, -2), nv(-.15, .08, 2)),
	}
	paths, err := nativeBridgeRoutes(context.Background(), edges)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths[0]) != 12 {
		t.Fatalf("nearby crossings did not share one plateau: %v", paths[0])
	}
	peak := .08 + 2*nativeRouteRadius(edges[0]) + .05
	for _, p := range paths[0] {
		if p.X >= -.15 && p.X <= .15 && math.Abs(p.Y-peak) > 1e-10 {
			t.Fatal("bridge dipped between neighboring crossings")
		}
	}
	for _, permutation := range [][]int{{2, 0, 1}, {1, 2, 0}, {2, 1, 0}} {
		reordered := []d2isometric.Edge{edges[permutation[0]], edges[permutation[1]], edges[permutation[2]]}
		got, err := nativeBridgeRoutes(context.Background(), reordered)
		if err != nil {
			t.Fatal(err)
		}
		for i, original := range permutation {
			if !reflect.DeepEqual(got[i], paths[original]) {
				t.Fatalf("path for %q changed with edge order", reordered[i].ID)
			}
		}
	}
}

func TestNativeBridgeMultiwayAndRemoteSharedSource(t *testing.T) {
	// Three mutually transverse routes cross at one point. Each higher ID
	// clears the actual lower path, including its earlier bridge.
	edges := make([]d2isometric.Edge, 3)
	for i, id := range []string{"a", "b", "c"} {
		angle := float64(i) * math.Pi / 3
		dir := nv(2*math.Cos(angle), 0, 2*math.Sin(angle))
		edges[i] = bridgeTestEdge(id, nv(-dir.X, .08, -dir.Z), nv(dir.X, .08, dir.Z))
	}
	paths, err := nativeBridgeRoutes(context.Background(), edges)
	if err != nil {
		t.Fatal(err)
	}
	clearance := 2*nativeRouteRadius(edges[0]) + .05
	for i, path := range paths {
		mid := pathPoint(path, routeLengths(path), .5)
		if math.Abs(mid.Y-(.08+float64(i)*clearance)) > 1e-8 {
			t.Fatalf("multi-way bridge %d does not clear its lower route: %+v", i, mid)
		}
	}
	// Shared semantic endpoints do not make a remote geometric crossing a
	// junction. Only the actual endpoint intersection is excluded.
	edges = []d2isometric.Edge{
		bridgeTestEdge("a", nv(-2, .08, 0), nv(2, .08, 0)),
		bridgeTestEdge("b", nv(-2, .08, 0), nv(-2, .08, -2), nv(0, .08, -2), nv(0, .08, 2)),
	}
	edges[0].Source, edges[1].Source = "same-source", "same-source"
	paths, err = nativeBridgeRoutes(context.Background(), edges)
	if err != nil {
		t.Fatal(err)
	}
	peak := .08
	for _, p := range paths[1] {
		peak = max(peak, p.Y)
		if p.Y > .08 && (p.X != 0 || math.Abs(p.Z) > .3) {
			t.Fatalf("shared-source bridge modified the corner or endpoint: %+v", p)
		}
	}
	if peak == .08 {
		t.Fatal("remote crossing was incorrectly suppressed by a shared endpoint ID")
	}
}

func TestNativeBridgeValidationCancellationAndBudget(t *testing.T) {
	edges := []d2isometric.Edge{bridgeTestEdge("a", nv(-2, .08, 0), nv(2, .08, 0)), bridgeTestEdge("b", nv(0, .08, -2), nv(0, .08, 2))}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := nativeBridgeRoutes(ctx, edges); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
	if _, err := nativeResolveBridgeRoutes(context.Background(), edges, 8); err == nil || !strings.Contains(err.Error(), "work limit") {
		t.Fatalf("work budget not enforced: %v", err)
	}
	if _, err := nativeBridgeRoutes(nil, edges); err == nil {
		t.Fatal("nil context accepted")
	}
	for _, value := range []float64{math.NaN(), math.Inf(1), 1e10} {
		edges[0].Points[0].X = value
		if _, err := nativeBridgeRoutes(context.Background(), edges); err == nil {
			t.Fatalf("invalid coordinate accepted: %g", value)
		}
	}
	edges[0].Points = make([]Vec, 10001)
	if _, err := nativeBridgeRoutes(context.Background(), edges); err == nil || !strings.Contains(err.Error(), "point limit") {
		t.Fatalf("oversized route accepted: %v", err)
	}
}
