package routing

import (
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

func TestOVGPortDirectionFilterMatchesLinearScan(t *testing.T) {
	node := layoutgraph.NewNode(1, 40, 60)
	node.TopLeft = geo.NewPoint(0, 0)
	ports := []*OVGNode{
		NewOVGNode(geo.NewPoint(0, 30)), NewOVGNode(geo.NewPoint(40, 30)),
		NewOVGNode(geo.NewPoint(20, 0)), NewOVGNode(geo.NewPoint(20, 60)),
	}
	points := []*geo.Point{geo.NewPoint(-50, 30), geo.NewPoint(90, 30), geo.NewPoint(20, -50), geo.NewPoint(20, 90), geo.NewPoint(20, 30)}
	for _, port := range ports {
		points = append(points, port.Point)
	}
	for direction := geo.TopLeft; direction <= geo.NONE; direction++ {
		for _, from := range points {
			for _, to := range points {
				for _, selected := range [][]*OVGNode{nil, ports[:1], ports, append(ports, ports...)} {
					guard := newOVGBuildGuardForTest(t.Context(), t)
					got, err := guard.passesThroughAllowingPorts(node, from, to, direction, selected)
					if err != nil {
						t.Fatal(err)
					}
					want, err := guard.passesThroughAllowingPortsLinearReference(node, from, to, direction, selected)
					if err != nil || got != want {
						t.Fatalf("direction=%v from=%v to=%v ports=%d got=%v want=%v err=%v", direction, from, to, len(selected), got, want, err)
					}
				}
			}
		}
	}
}

func TestOVGPortDirectionFilterEmptyPortsPreservesNilPoints(t *testing.T) {
	node := layoutgraph.NewNode(1, 40, 40)
	node.TopLeft = geo.NewPoint(0, 0)
	for direction := geo.TopLeft; direction <= geo.NONE; direction++ {
		for _, pair := range [][2]*geo.Point{{nil, nil}, {nil, geo.NewPoint(20, 20)}, {geo.NewPoint(20, 20), nil}} {
			guard := newOVGBuildGuardForTest(t.Context(), t)
			got, err := guard.passesThroughAllowingPorts(node, pair[0], pair[1], direction, nil)
			want, wantErr := guard.passesThroughAllowingPortsLinearReference(node, pair[0], pair[1], direction, nil)
			if got != want || err != wantErr {
				t.Fatalf("direction=%v got=%v/%v want=%v/%v", direction, got, err, want, wantErr)
			}
		}
	}
}

func TestOVGPortDirectionFilterChecksCancellation(t *testing.T) {
	node := layoutgraph.NewNode(1, 40, 40)
	node.TopLeft = geo.NewPoint(0, 0)
	for _, direction := range []geo.Orientation{geo.NONE, geo.Right} {
		guard := newOVGBuildGuardForTest(t.Context(), t)
		guard.ctx = canceledContext()
		guard.done = guard.ctx.Done()
		_, err := guard.passesThroughAllowingPorts(node, geo.NewPoint(-50, 20), geo.NewPoint(90, 20), direction, []*OVGNode{NewOVGNode(geo.NewPoint(0, 20))})
		requireCanceledAt(t, err, "EdgeRouting")
	}
}

func BenchmarkOVGPortDirectionFilter(b *testing.B) {
	node := layoutgraph.NewNode(1, 40, 40)
	node.TopLeft = geo.NewPoint(0, 0)
	var ports []*OVGNode
	for i := range 64 {
		ports = append(ports, NewOVGNode(geo.NewPoint(0, float64(i))))
	}
	from, to := geo.NewPoint(-50, 20), geo.NewPoint(90, 20)
	for _, tc := range []struct {
		name      string
		direction geo.Orientation
	}{{"none", geo.NONE}, {"wrong", geo.Left}, {"outwards", geo.Right}} {
		for _, linear := range []bool{true, false} {
			name := tc.name + "/indexed"
			if linear {
				name = tc.name + "/linear"
			}
			b.Run(name, func(b *testing.B) {
				guard, err := newOVGBuildGuard(b.Context(), defaultOVGBuildLimits())
				if err != nil {
					b.Fatal(err)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					guard.work = 0
					if linear {
						_, err = guard.passesThroughAllowingPortsLinearReference(node, from, to, tc.direction, ports)
					} else {
						_, err = guard.passesThroughAllowingPorts(node, from, to, tc.direction, ports)
					}
					if err != nil {
						b.Fatal(err)
					}
				}
				b.ReportMetric(float64(guard.work), "work/op")
			})
		}
	}
}

func (guard *ovgBuildGuard) passesThroughAllowingPortsLinearReference(node *layoutgraph.Node, p1, p2 *geo.Point, direction geo.Orientation, ports []*OVGNode) (bool, error) {
	for _, port := range ports {
		if err := guard.step(); err != nil {
			return false, err
		}
		if !nonNilEquals(port.Point, p1) && !nonNilEquals(port.Point, p2) {
			continue
		}
		switch direction {
		case geo.Top:
			if p1.X == p2.X && p1.Y > p2.Y {
				return false, nil
			}
		case geo.Bottom:
			if p1.X == p2.X && p1.Y < p2.Y {
				return false, nil
			}
		case geo.Left:
			if p1.Y == p2.Y && p1.X > p2.X {
				return false, nil
			}
		case geo.Right:
			if p1.Y == p2.Y && p1.X < p2.X {
				return false, nil
			}
		}
	}
	return node.PassesThrough(p1, p2), guard.check()
}
