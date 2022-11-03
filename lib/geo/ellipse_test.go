package geo

import (
	"math"
	"testing"
)

func TestEllipseLineIntersections(t *testing.T) {
	e := NewEllipse(NewPoint(0, 0), 11, 11)

	intersections := e.Intersections(Segment{
		Start: NewPoint(0, 20),
		End:   NewPoint(0, -20),
	})
	if len(intersections) != 2 ||
		!intersections[0].Equals(NewPoint(0, 11)) ||
		!intersections[1].Equals(NewPoint(0, -11)) {
		t.Fatalf("vertical intersection check failed [%v,%v]", intersections[0].ToString(), intersections[1].ToString())
	}

	intersections = e.Intersections(Segment{
		Start: NewPoint(0, 2),
		End:   NewPoint(0, -2),
	})
	if len(intersections) != 0 {
		t.Fatalf("(vertical) line intersects but segment shouldn't")
	}

	intersections = e.Intersections(Segment{
		Start: NewPoint(2, 2),
		End:   NewPoint(5, 5),
	})
	if len(intersections) != 0 {
		t.Fatalf("line intersects but segment shouldn't")
	}

	intersections = e.Intersections(Segment{
		Start: NewPoint(2, 2),
		End:   NewPoint(50, 50),
	})
	x := math.Sqrt2 / 2 * 11
	expected := NewPoint(x, x)
	if len(intersections) != 1 || !intersections[0].Equals(expected) {
		t.Fatalf("intersection check failed with %v expected %v", intersections[0].ToString(), expected.ToString())
	}

	// test with cx,cy offset
	e = NewEllipse(NewPoint(100, 200), 21, 21)
	intersections = e.Intersections(Segment{
		Start: NewPoint(0, 0),
		End:   NewPoint(100, 150),
	})
	if len(intersections) != 0 {
		t.Fatalf("shouldn't intersect with offset")
	}
	intersections = e.Intersections(Segment{
		Start: NewPoint(50, 150),
		End:   NewPoint(200, 250),
	})
	if len(intersections) != 2 {
		t.Fatalf("should intersect with offset")
	}

	// tangent
	intersections = e.Intersections(Segment{
		Start: NewPoint(0, 221),
		End:   NewPoint(200, 221),
	})
	if len(intersections) != 1 {
		t.Fatalf("should intersect horizontal tangent")
	}
	intersections = e.Intersections(Segment{
		Start: NewPoint(121, 100),
		End:   NewPoint(121, 300),
	})
	if len(intersections) != 1 {
		t.Fatalf("should intersect vertical tangent")
	}
	intersections = NewEllipse(
		NewPoint(1, 1),
		2/math.Sqrt2,
		2/math.Sqrt2,
	).Intersections(Segment{
		Start: NewPoint(1, 3),
		End:   NewPoint(3, 1),
	})
	if len(intersections) == 0 {
		// Note: due to floating point accuracy, there are two intersections instead of one
		t.Fatalf("should intersect tangent")
	}
}
