package d2isometricimg

import (
	"context"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2isometric"
)

func TestSVGVisibilityClassRowCapCoversItsRearWall(t *testing.T) {
	ctx := nativeVectorContext(context.Background())
	d := sourcePanelFixture(t, "stable/class/elk/board.exp.json")
	scene, err := d2isometric.BuildScene(d, &d2isometric.RenderOpts{})
	if err != nil {
		t.Fatal(err)
	}
	native, err := newNativeSceneWithOptions(ctx, scene, 1200, 1200, nil, nil, nativeSceneOptions{deferRaster: true, vector: true, fitContent: true, outputDensity: sceneOutputDensity(scene, 1200, 1200, nil)})
	if err != nil {
		t.Fatal(err)
	}
	var faces []svgVisibilityFace
	for i, tr := range native.triangles {
		face := svgVisibilityFace{order: i, group: tr.OpacityGroup, opaque: !tr.NoDepthWrite && tr.Material.Texture == nil && tr.Material.Color.A == 255 && !tr.Material.Multiply}
		for _, vertex := range tr.V {
			p := native.camera.project(vertex.Position)
			face.points = append(face.points, svgPoint{p.x, p.y, p.z + tr.DepthBias})
		}
		faces = append(faces, face)
	}
	visible, err := svgVisibleFaces(ctx, faces)
	if err != nil {
		t.Fatal(err)
	}
	p := svgPoint{x: 950, y: 374}
	// Identify each source surface by its geometry and depth at the failed
	// point. Mesh indices can change when independent renderer features grow.
	occluded := 0
	for i, face := range faces {
		if svgValidFragment(face.points) == nil {
			continue
		}
		if svgPolygonArea(face.points) < 0 {
			face.points = []svgPoint{face.points[2], face.points[1], face.points[0]}
		}
		plane, valid := svgPolygonPlane(face.points)
		if !valid {
			continue
		}
		want := svgVisibilityContains(face.points, p)
		if want {
			for j, other := range faces {
				if i == j || !other.opaque || (other.group != nil && other.group != face.group) || svgValidFragment(other.points) == nil {
					continue
				}
				if svgPolygonArea(other.points) < 0 {
					other.points = []svgPoint{other.points[2], other.points[1], other.points[0]}
				}
				if !svgVisibilityContains(other.points, p) {
					continue
				}
				op, valid := svgPolygonPlane(other.points)
				if !valid {
					continue
				}
				difference := op.at(p) - plane.at(p)
				if difference > 1e-9 || (difference >= -1e-9 && j > i) {
					want = false
					occluded++
					break
				}
			}
		}
		got := false
		for _, fragment := range visible[i] {
			got = got || svgVisibilityContains(fragment, p)
		}
		if got != want {
			t.Fatalf("class face %d visibility=%v, want %v from source geometry/depth", i, got, want)
		}
	}
	if occluded == 0 {
		t.Fatal("fixture no longer exercises overlapping row geometry at the probe")
	}
}
