package d2isometricimg

import (
	"reflect"
	"strings"
	"testing"
)

func TestSVGPolygonPathOmitsSubprecisionCollinearSplits(t *testing.T) {
	// The same pattern-face clip on ARM64 and amd64 placed one redundant
	// intersection at different positions along an otherwise identical edge.
	// Both rounded positions are less than one serialized coordinate unit
	// from that edge. They must not change its SVG or its silhouette.
	polygon := []svgPoint{
		{316.141043, 347.158271, 0}, {318.065866, 344.688423, 0},
		{318.661261, 344.688423, 0}, {318.661261, 347.986151, 0},
		{318.032125, 348.793430, 0}, {317.690070, 348.793430, 0},
		{316.141043, 347.481102, 0},
	}
	want := svgPolygonPath([][]svgPoint{polygon})
	for _, split := range []svgPoint{
		{317.423128, 348.567277, 0},
		{317.423670, 348.567737, 0},
	} {
		points := append([]svgPoint(nil), polygon[:6]...)
		points = append(points, split, polygon[6])
		original := append([]svgPoint(nil), points...)
		for repeat := 0; repeat < 2; repeat++ {
			if got := svgPolygonPath([][]svgPoint{points}); got != want {
				t.Fatalf("subprecision clip split changed its serialized silhouette:\n%s\nwant %s", got, want)
			}
		}
		if !reflect.DeepEqual(points, original) {
			t.Fatal("SVG serialization mutated source visibility geometry")
		}
		// The redundant vertex can also straddle the cyclic loop boundary.
		for offset := range points {
			rotated := append(append([]svgPoint(nil), points[offset:]...), points[:offset]...)
			if got := svgPolygonPath([][]svgPoint{rotated}); got != want {
				t.Fatalf("rotating a clip split changed its serialized silhouette: %s", got)
			}
		}
	}
}

func TestSVGPolygonPathKeepsRealCornersAndTriangles(t *testing.T) {
	for _, polygon := range [][]svgPoint{
		{{0, 0, 0}, {2, .00001, 0}, {4, 0, 0}, {4, 4, 0}, {0, 4, 0}},
		{{0, 0, 0}, {2, .000001, 0}, {4, 0, 0}},
	} {
		path := svgPolygonPath([][]svgPoint{polygon})
		if got := strings.Count(path, "L") + strings.Count(path, "M"); got != len(polygon) {
			t.Fatalf("serialization erased a real corner or triangle: %s", path)
		}
	}
}
