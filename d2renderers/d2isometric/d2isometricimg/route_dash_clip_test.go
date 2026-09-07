package d2isometricimg

import (
	"math"
	"reflect"
	"testing"
)

func TestEndpointClippingPreservesOriginalDashPhaseAndBends(t *testing.T) {
	points := []Vec{nv(0, .08, 0), nv(2, .08, 0), nv(2, .08, 3), nv(7, .08, 3)}
	lengths := routeLengths(points)
	full := nativeRouteDashes(points, lengths, 0, 1, 4, 8000)
	// A different marker may consume a different amount at either end. Every
	// complete interior dash, including its corner vertices, must stay fixed.
	for _, span := range [][2]float64{{.17, .89}, {.27, .73}, {.05, .98}} {
		clipped := nativeRouteDashes(points, lengths, span[0], span[1], 4, 8000)
		if len(clipped) == 0 {
			t.Fatal("endpoint clipping removed the entire long dashed connection")
		}
		distance := func(p Vec) float64 {
			if p.Z == 0 {
				return p.X
			}
			if p.X == 2 {
				return 2 + p.Z
			}
			return 5 + p.X - 2
		}
		interior := 0
		for _, dash := range full {
			if distance(dash[0]) <= span[0]*10 || distance(dash[len(dash)-1]) >= span[1]*10 {
				continue
			}
			found := false
			for _, candidate := range clipped {
				found = found || reflect.DeepEqual(dash, candidate)
			}
			if !found {
				t.Fatal("endpoint clearance changed an interior dash or routed bend")
			}
			interior++
		}
		if interior < 5 {
			t.Fatal("fixture did not exercise enough complete interior dashes")
		}
		for _, dash := range clipped {
			if distance(dash[0]) < span[0]*10-1e-9 || distance(dash[len(dash)-1]) > span[1]*10+1e-9 {
				t.Fatal("wire still protrudes into the clipped marker region")
			}
			for _, p := range dash {
				if p.Y != .08 || math.IsNaN(p.X) || math.IsNaN(p.Z) {
					t.Fatal("clipping moved a dash off the flat route")
				}
			}
		}
	}
}
