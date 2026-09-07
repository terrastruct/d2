package d2isometricimg

import (
	"context"
	"image"
	"image/color"
	"math"
	"reflect"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2target"
)

// This independent check uses the final raster camera's viewing axes. Four
// corners in screen coordinates form a parallelogram; SAT is exact for these
// flat print surfaces even when they occupy different world-space elevations.
func projectedTestSurfacesOverlap(a, b labelSurface) bool {
	direction := nunit(nativeViewDirection())
	right := nunit(ncross(nv(0, 1, 0), direction))
	up := ncross(direction, right)
	points := func(s labelSurface) [4][2]float64 {
		u := nmul(nv(math.Cos(s.angle), 0, math.Sin(s.angle)), s.width/2)
		v := nmul(nv(-math.Sin(s.angle), 0, math.Cos(s.angle)), s.depth/2)
		var result [4][2]float64
		for i, signs := range [4][2]float64{{-1, -1}, {1, -1}, {1, 1}, {-1, 1}} {
			p := nadd(s.center, nadd(nmul(u, signs[0]), nmul(v, signs[1])))
			result[i] = [2]float64{ndot(p, right), ndot(p, up)}
		}
		return result
	}
	pa, pb := points(a), points(b)
	for _, polygon := range [][4][2]float64{pa, pb} {
		for i, p := range polygon {
			q := polygon[(i+1)%4]
			axis := [2]float64{q[1] - p[1], p[0] - q[0]}
			bounds := func(points [4][2]float64) (float64, float64) {
				lo, hi := math.Inf(1), math.Inf(-1)
				for _, point := range points {
					distance := point[0]*axis[0] + point[1]*axis[1]
					lo, hi = min(lo, distance), max(hi, distance)
				}
				return lo, hi
			}
			alo, ahi := bounds(pa)
			blo, bhi := bounds(pb)
			if ahi <= blo || bhi <= alo {
				return false
			}
		}
	}
	return true
}

func TestProjectedCaptionClearanceAcrossElevations(t *testing.T) {
	points := []Vec{nv(0, .08, 0), nv(12, .08, 0)}
	before := newRouteCaptionPlacer().Place(points, .5, 2, .4)
	raised := before
	raised.center = nadd(raised.center, nmul(nativeViewDirection(), 2))
	if math.Abs(raised.center.Z-before.center.Z) < before.depth || !projectedTestSurfacesOverlap(before, raised) {
		t.Fatal("fixture must be separate on the board but coincident in the final projection")
	}
	place := func() labelSurface {
		p := newRouteCaptionPlacer()
		p.Avoid(raised.center, raised.width, raised.depth)
		return p.Place(points, .5, before.width, before.depth)
	}
	after := place()
	if projectedTestSurfacesOverlap(after, raised) {
		t.Fatal("automatic caption still overlaps the raised label in the final camera")
	}
	if after.width != before.width || after.depth != before.depth || after.center.Y != before.center.Y || after.angle != before.angle {
		t.Fatal("projection clearance changed print size, plane, or route angle")
	}
	if after != place() {
		t.Fatal("projected placement is nondeterministic")
	}
}

func TestProjectedCaptionMeshIncludesFinalOutsideSurfaces(t *testing.T) {
	points := []Vec{nv(0, .08, 0), nv(12, .08, 0)}
	before := newRouteCaptionPlacer().Place(points, .5, 2, .4)
	out := before
	out.center = nadd(out.center, nmul(nativeViewDirection(), 2))
	b := &meshBuilder{ctx: context.Background()}
	body := &Material{Color: color.NRGBA{70, 90, 110, 255}}
	b.box(nv(-5, .4, 0), nv(1, .8, 1), body, 0)
	b.surfaceTexture(image.NewRGBA(image.Rect(0, 0, 1, 1)), out, 1)
	original := append([]Triangle(nil), b.triangles...)
	p := newRouteCaptionPlacer()
	p.AvoidMesh(b.triangles)
	after := p.Place(points, .5, 2, .4)
	if projectedTestSurfacesOverlap(after, out) {
		t.Fatal("completed outside caption was omitted from node obstacles")
	}
	if !reflect.DeepEqual(b.triangles, original) {
		t.Fatal("collision indexing changed the source component geometry or caption anchor")
	}
	// The separate body and outside caption must not block the empty space
	// between them, as one combined component bounding box would.
	if score, complete := p.score(captionRect(labelSurface{center: nv(0, 0, 0), width: .2, depth: .2}, false)); !complete || score != 0 {
		t.Fatal("joining body and outside caption reserved an empty corridor")
	}
}

func TestProjectedCaptionAvoidsLaterCrossingWire(t *testing.T) {
	points := []Vec{nv(0, .08, 0), nv(12, .08, 0)}
	before := newRouteCaptionPlacer().Place(points, .5, 2, .4)
	wire := []Vec{nv(6, .08, -3), nv(6, .08, 3)}
	wireSurface := labelSurface{center: nv(6, .08, 0), width: 6, depth: .08, angle: math.Pi / 2}
	if !projectedTestSurfacesOverlap(before, wireSurface) {
		t.Fatal("fixture's automatic caption does not cross the wire")
	}
	p := newRouteCaptionPlacer()
	p.AvoidRoute(wire, .04)
	after := p.Place(points, .5, 2, .4)
	if projectedTestSurfacesOverlap(after, wireSurface) {
		t.Fatal("automatic caption covers a crossing wire")
	}
	if !reflect.DeepEqual(wire, []Vec{nv(6, .08, -3), nv(6, .08, 3)}) {
		t.Fatal("caption clearance moved the route")
	}
}

func TestProjectedCaptionMeshKeepsClearSilhouetteCorners(t *testing.T) {
	// An actual triangular body leaves the upper-right corner of its source
	// bounds empty. Captions can occupy that corner without crossing its mesh.
	triangles := []Triangle{{V: [3]Vertex{{Position: nv(0, 0, 0)}, {Position: nv(4, 0, 0)}, {Position: nv(0, 0, 4)}}, Material: &Material{Color: color.NRGBA{70, 90, 110, 255}}}}
	p := newRouteCaptionPlacer()
	p.AvoidMesh(triangles)
	if score, complete := p.score(captionRect(labelSurface{center: nv(3, 0, 3), width: .4, depth: .4}, false)); !complete || score != 0 {
		t.Fatal("source bounding box falsely blocked the body's empty projected corner")
	}
	if score, complete := p.score(captionRect(labelSurface{center: nv(1, 0, 1), width: .4, depth: .4}, false)); !complete || score == 0 {
		t.Fatal("actual body silhouette was not reserved")
	}
	// Work exhaustion must retain a finite, full-sized caption rather than
	// turning expensive polygon checks into an unbounded search or hidden text.
	p.work = maxRouteCaptionWork - 8
	surface := p.Place([]Vec{nv(0, 0, 1), nv(4, 0, 1)}, .5, 2, .4)
	if p.work > maxRouteCaptionWork || surface.width != 2 || surface.depth != .4 || !captionFinite(surface.center.X, surface.center.Y, surface.center.Z, surface.angle) {
		t.Fatal("mesh collision budget changed or hid the fallback caption")
	}
}

func TestProjectedCaptionReservesLaterAuthoredAnchor(t *testing.T) {
	ctx := context.Background()
	painter, err := newTextPainter(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	style := d2target.Text{Label: "label", LabelWidth: 100, LabelHeight: 25, FontSize: 20}
	auto := d2isometric.Edge{ID: "auto", Label: style.Label, Points: []Vec{nv(0, .08, 0), nv(10, .08, 0)}, Opacity: 1}
	auto.Metadata.Original.Text = style
	fixed := auto
	fixed.ID = "fixed"
	fixed.Points = []Vec{nv(0, .08, -.25), nv(10, .08, -.25)}
	fixed.Metadata.Original.LabelPosition = "INSIDE_MIDDLE_CENTER"
	width, depth := nativeRouteCaptionSize(fixed, .5, style, false, .01)
	anchor, ok := nativeConnectionCaptionSurface(fixed.Points, fixed.Metadata.Original, width, depth, .01)
	if !ok {
		t.Fatal("authored anchor was not recognized")
	}
	anchor.center.Y += nativeRouteRadius(fixed)
	before := newRouteCaptionPlacer().Place(auto.Points, .5, width, depth)
	if !projectedTestSurfacesOverlap(before, anchor) {
		t.Fatal("fixture must overlap before reserving later authored anchors")
	}
	b := &meshBuilder{ctx: ctx, text: painter, scale: .01}
	b.edges([]d2isometric.Edge{auto, fixed}, newRouteCaptionPlacer())
	if b.err != nil {
		t.Fatal(b.err)
	}
	var surfaces []labelSurface
	for i := 0; i < len(b.triangles); i++ {
		t := b.triangles[i]
		if t.Material.Texture == nil {
			continue
		}
		// surfaceTexture's first triangle has opposite corners at V[0],V[2].
		surfaces = append(surfaces, labelSurface{center: nmul(nadd(t.V[0].Position, t.V[2].Position), .5), width: width, depth: depth})
		i++
	}
	if len(surfaces) != 2 {
		t.Fatalf("expected two intact text surfaces, got %d", len(surfaces))
	}
	if projectedTestSurfacesOverlap(surfaces[0], surfaces[1]) {
		t.Fatal("earlier automatic label occupied the later authored anchor")
	}
	if nlen(nsub(surfaces[1].center, anchor.center)) > 1e-9 {
		t.Fatalf("authored caption moved from %+v to %+v", anchor.center, surfaces[1].center)
	}
}
