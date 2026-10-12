package d2isometricimg

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2target"
)

func TestPresentationStackedPersonCaptionsRetainSourceClearance(t *testing.T) {
	// This is the compiled shape pattern used by queue-worker diagrams: close
	// rows of multiple person silhouettes, with bold outside-bottom captions.
	// These 99x81 px bodies, 84x36 px labels and 152 px row pitch come from
	// the compiled queue-workers source, translated to a local origin. Keep
	// that source clearance rather than giving the renderer extra room.
	var nodes []d2isometric.Node
	for i, name := range []string{"User01", "User02"} {
		s := *d2target.BaseShape()
		s.ID, s.Type, s.Label = name, d2target.ShapePerson, name
		s.Pos, s.Width, s.Height = d2target.NewPoint(0, i*152), 99, 81
		s.Multiple, s.Bold, s.FontSize = true, true, 28
		s.LabelPosition, s.LabelWidth, s.LabelHeight = "OUTSIDE_BOTTOM_CENTER", 84, 36
		s.Fill, s.Stroke = "#f1f4fa", "#193bac"
		nodes = append(nodes, d2isometric.Node{ID: name, Type: s.Type, Label: name,
			Position: nv(.495, .07+1.35/2, .405+float64(i)*1.52), Size: nv(.99, 1.35, .81),
			Fill: s.Fill, Stroke: s.Stroke, StrokeWidth: s.StrokeWidth, Opacity: 1,
			FillExplicit: true, Metadata: d2isometric.NodeMetadata{Original: s}})
	}
	original := append([]d2isometric.Node(nil), nodes...)
	build := func(oldSculptureRelief bool) [][]Triangle {
		t.Helper()
		painter, err := newTextPainter(context.Background(), len(nodes))
		if err != nil {
			t.Fatal(err)
		}
		meshes := make([][]Triangle, len(nodes))
		for i, n := range nodes {
			b := &meshBuilder{ctx: context.Background(), scale: .01, text: painter}
			if oldSculptureRelief {
				// Negative control: the former sculpture relief displaced the
				// first full-sized caption into the next source silhouette.
				// Recreate that old final height independently of the selected
				// relief symbol's new footprint-based body height.
				factor := .55 * math.Max(.10, n.Size.Y*1.15) / nativeCanonicalHeight(n, .01)
				b.node(n, "#849ebc", factor)
				floor := n.Position.Y - n.Size.Y/2
				for j := range b.triangles {
					for k := range b.triangles[j].V {
						p := &b.triangles[j].V[k].Position
						p.Y = floor + (p.Y-floor)*factor
					}
				}
			} else {
				b.hierarchyNode(n, "#849ebc")
			}
			if b.err != nil || len(b.triangles) < 4 {
				t.Fatalf("person %s did not produce body and caption: %v", n.ID, b.err)
			}
			meshes[i] = b.triangles
		}
		return meshes
	}
	caption := func(mesh []Triangle) labelSurface {
		t.Helper()
		a, c := mesh[len(mesh)-2], mesh[len(mesh)-1]
		if !a.NoDepthWrite || !c.NoDepthWrite || a.Material.Texture == nil || a.Material != c.Material {
			t.Fatal("final two triangles are not the complete printed caption")
		}
		lo, hi := a.V[0].Position, a.V[0].Position
		for _, tri := range []Triangle{a, c} {
			for _, v := range tri.V {
				p := v.Position
				lo.X, lo.Y, lo.Z = min(lo.X, p.X), min(lo.Y, p.Y), min(lo.Z, p.Z)
				hi.X, hi.Y, hi.Z = max(hi.X, p.X), max(hi.Y, p.Y), max(hi.Z, p.Z)
			}
		}
		if math.Abs(hi.X-lo.X-.84) > 1e-9 || math.Abs(hi.Z-lo.Z-.36) > 1e-9 || hi.Y-lo.Y > 1e-9 {
			t.Fatalf("source caption changed size or ceased to be flat: %+v..%+v", lo, hi)
		}
		return labelSurface{center: nmul(nadd(lo, hi), .5), width: hi.X - lo.X, depth: hi.Z - lo.Z}
	}
	meshes := build(false)
	labels := []labelSurface{caption(meshes[0]), caption(meshes[1])}
	printed := []labelSurface{presentationCaptionInk(labels[0], meshes[0]), presentationCaptionInk(labels[1], meshes[1])}
	if projectedTestSurfacesOverlap(labels[0], labels[1]) {
		t.Fatal("adjacent source captions overlap in the final camera")
	}
	for i, mesh := range meshes {
		for labelIndex, s := range printed {
			if presentationCaptionIntersectsBody(s, mesh[:len(mesh)-2]) {
				t.Fatalf("printed caption %d intersects final person %d body", labelIndex, i)
			}
		}
		// Also check the neighbor's original footprint independently of the
		// mesh collision index, using the final camera's screen-space SAT.
		floor := nodes[i].Position.Y - nodes[i].Size.Y/2
		footprint := labelSurface{center: nv(nodes[i].Position.X, floor, nodes[i].Position.Z), width: nodes[i].Size.X, depth: nodes[i].Size.Z}
		if projectedTestSurfacesOverlap(labels[1-i], footprint) {
			t.Fatal("caption entered its neighbor's original source footprint")
		}
	}
	bad := build(true)
	if !presentationCaptionIntersectsBody(presentationCaptionInk(caption(bad[0]), bad[0]), bad[1][:len(bad[1])-2]) {
		t.Fatal("fixture no longer reproduces the former sculpture-height overlap")
	}
	if !reflect.DeepEqual(nodes, original) {
		t.Fatal("presentation changed original source positions, dimensions or label metrics")
	}
}

func presentationCaptionInk(s labelSurface, mesh []Triangle) labelSurface {
	tex := mesh[len(mesh)-1].Material.Texture
	bounds := tex.Bounds()
	x0, y0, x1, y1 := bounds.Max.X, bounds.Max.Y, bounds.Min.X, bounds.Min.Y
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, alpha := tex.At(x, y).RGBA()
			if alpha > 0 {
				x0, y0, x1, y1 = min(x0, x), min(y0, y), max(x1, x+1), max(y1, y+1)
			}
		}
	}
	sx, sy := s.width/float64(bounds.Dx()), s.depth/float64(bounds.Dy())
	s.center.X += (float64(x0+x1-2*bounds.Min.X)/2 - float64(bounds.Dx())/2) * sx
	s.center.Z += (float64(y0+y1-2*bounds.Min.Y)/2 - float64(bounds.Dy())/2) * sy
	s.width, s.depth = float64(x1-x0)*sx, float64(y1-y0)*sy
	return s
}

// Independently intersect the final caption quad and actual opaque sidewall
// triangles in screen space. Do not use the route planner's safety margins or
// transparent cap texture bounds as evidence that visible content overlaps.
func presentationCaptionIntersectsBody(s labelSurface, mesh []Triangle) bool {
	direction := nunit(nativeViewDirection())
	right := nunit(ncross(nv(0, 1, 0), direction))
	up := ncross(direction, right)
	project := func(v Vec) [2]float64 { return [2]float64{ndot(v, right), ndot(v, up)} }
	quad := make([][2]float64, 4)
	for i, sign := range [4][2]float64{{-1, -1}, {1, -1}, {1, 1}, {-1, 1}} {
		quad[i] = project(nadd(s.center, nv(sign[0]*s.width/2, 0, sign[1]*s.depth/2)))
	}
	for _, tri := range mesh {
		if tri.Material == nil || tri.Material.Color.A == 0 || tri.Material.Texture != nil {
			continue
		}
		points := [][2]float64{project(tri.V[0].Position), project(tri.V[1].Position), project(tri.V[2].Position)}
		area := (points[1][0]-points[0][0])*(points[2][1]-points[0][1]) - (points[1][1]-points[0][1])*(points[2][0]-points[0][0])
		if math.Abs(area) < 1e-12 {
			continue
		}
		separate := false
		for _, polygon := range [][][2]float64{quad, points} {
			for i, a := range polygon {
				b := polygon[(i+1)%len(polygon)]
				ax, ay := b[1]-a[1], a[0]-b[0]
				if math.Hypot(ax, ay) < 1e-12 {
					continue
				}
				bounds := func(p [][2]float64) (float64, float64) {
					lo, hi := math.Inf(1), math.Inf(-1)
					for _, q := range p {
						v := q[0]*ax + q[1]*ay
						lo, hi = min(lo, v), max(hi, v)
					}
					return lo, hi
				}
				a0, a1 := bounds(quad)
				b0, b1 := bounds(points)
				separate = separate || a1 <= b0+1e-12 || b1 <= a0+1e-12
			}
		}
		if !separate {
			return true
		}
	}
	return false
}
