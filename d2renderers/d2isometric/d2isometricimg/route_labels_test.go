package d2isometricimg

import (
	"math"
	"reflect"
	"testing"
)

func TestRouteCaptionsSeparateSharedCorridor(t *testing.T) {
	points := []Vec{{X: 0, Y: .08}, {X: 14, Y: .08}}
	place := func() []labelSurface {
		p := newRouteCaptionPlacer()
		var labels []labelSurface
		for i := 0; i < 8; i++ {
			s := p.Place(points, .5, 2.2, .38)
			if s.width != 2.2 || s.depth != .38 || math.Abs(s.center.Y-.086) > 1e-12 || s.angle != 0 {
				t.Fatalf("print dimensions, plane or angle changed: %+v", s)
			}
			if s.center.X-s.width/2 < 0 || s.center.X+s.width/2 > 14 || math.Abs(s.center.Z)-s.depth/2 < .06 {
				t.Fatalf("caption left the straight leg or covers its wire: %+v", s)
			}
			for _, prior := range labels {
				// Axis-aligned intervals provide an independent overlap check.
				if math.Abs(prior.center.X-s.center.X) < (prior.width+s.width)/2+.04 && math.Abs(prior.center.Z-s.center.Z) < (prior.depth+s.depth)/2+.04 {
					t.Fatalf("corridor captions overlap: %+v and %+v", prior, s)
				}
			}
			labels = append(labels, s)
		}
		return labels
	}
	if !reflect.DeepEqual(place(), place()) {
		t.Fatal("caption placement is nondeterministic")
	}
}

func TestRouteCaptionsKeepEndpointIdentityAndReadableAngle(t *testing.T) {
	points := []Vec{{Y: .08}, {X: 10, Y: .08}}
	p := newRouteCaptionPlacer()
	source := p.Place(points, .08, 2, .3)
	main := p.Place(points, .5, 2, .3)
	target := p.Place(points, .92, 2, .3)
	if source.center.X >= 4 || main.center.X <= source.center.X || main.center.X >= target.center.X || target.center.X <= 6 {
		t.Fatalf("source/main/target ordering changed: %+v %+v %+v", source, main, target)
	}
	reverse := newRouteCaptionPlacer().Place([]Vec{points[1], points[0]}, .5, 2, .3)
	if reverse.angle != main.angle || reverse.center != main.center {
		t.Fatalf("reversing an edge turned or shifted its caption: %+v %+v", reverse, main)
	}
	for _, s := range []labelSurface{source, main, target} {
		if s.width != 2 || s.depth != .3 {
			t.Fatal("caption was shrunk")
		}
	}
}

func TestRouteCaptionsAvoidComponentsAndUseOtherLegs(t *testing.T) {
	p := newRouteCaptionPlacer()
	// Both sides of the first leg are occupied. The second leg has room.
	p.Avoid(Vec{X: 4, Z: 0}, 9, 3)
	points := []Vec{{Y: .08}, {X: 8, Y: .08}, {X: 8, Y: .08, Z: 8}}
	s := p.Place(points, .5, 2, .4)
	if s.center.Z-s.depth/2 <= 1.5 || math.Abs(s.angle-math.Pi/2) > 1e-12 {
		t.Fatalf("did not move to a clear straight leg: %+v", s)
	}
	if s.width != 2 || s.depth != .4 || math.Abs(s.center.Y-.086) > 1e-12 {
		t.Fatal("physical label changed")
	}
}

func TestRouteCaptionOrientedOverlap(t *testing.T) {
	a := captionRect(labelSurface{width: 4, depth: .2, angle: math.Pi / 4}, false)
	b := captionRect(labelSurface{center: Vec{Z: 1.5}, width: 4, depth: .2, angle: math.Pi / 4}, false)
	if a.right <= b.left || b.right <= a.left || a.back <= b.front || b.back <= a.front {
		t.Fatal("test rectangles should have overlapping broad-phase bounds")
	}
	if captionOverlap(a, b) {
		t.Fatal("separate diagonal captions falsely overlap")
	}
	b = captionRect(labelSurface{width: 4, depth: .2, angle: -math.Pi / 4}, false)
	if !captionOverlap(a, b) {
		t.Fatal("crossed captions were not detected")
	}
}

func TestRouteCaptionBudgetsAndDegenerateRoutes(t *testing.T) {
	points := []Vec{{Y: .08}, {Y: .08}, {X: 10, Y: .08}}
	p := newRouteCaptionPlacer()
	s := p.Place(points, .5, 2, .3)
	if s.width != 2 || !captionFinite(s.center.X, s.center.Y, s.center.Z, s.angle) {
		t.Fatal("duplicate route points broke placement")
	}
	p.work = maxRouteCaptionWork
	fallback := p.Place(points, .5, 2, .3)
	if p.work != maxRouteCaptionWork || fallback.width != 2 || fallback.depth != .3 {
		t.Fatal("exhaustion exceeded work or hid/shrank a caption")
	}
	p = newRouteCaptionPlacer()
	for i := 0; i < maxRouteCaptionRects+10; i++ {
		p.Avoid(Vec{}, 1e8, 1e8)
	}
	if len(p.rects) != maxRouteCaptionRects || p.refs > maxRouteCaptionGridRefs {
		t.Fatal("rectangle or grid budget exceeded")
	}
	if p.Place(points, .5, 2, .3).width != 2 {
		t.Fatal("rectangle exhaustion hid a caption")
	}
	if newRouteCaptionPlacer().Place(points, .5, math.Inf(1), .3).width != 0 {
		t.Fatal("nonfinite print dimensions admitted")
	}
	if newRouteCaptionPlacer().Place([]Vec{{}, {}}, .5, 2, .3).width != 0 {
		t.Fatal("zero-length route produced invalid orientation")
	}
}
