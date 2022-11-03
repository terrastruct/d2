package geo

import (
	"testing"
)

func TestPointDistanceTo(t *testing.T) {
	p1 := &Point{0, 0}
	p2 := &Point{100, 0}

	p := &Point{50, 70}

	d := p.DistanceToLine(p1, p2)

	if d != 70.0 {
		t.Fatalf("Expected 70.0 and got %v", d)
	}
}

func TestAddVector(t *testing.T) {
	start := &Point{1.5, 5.3}
	c := NewVector(-3.5, -2.3)
	p2 := start.AddVector(c)

	if p2.X != -2 || p2.Y != 3 {
		t.Fatalf("Expected resulting point to be (-2, 3), got %+v", p2)
	}
}

func TestToVector(t *testing.T) {
	p := &Point{3.5, 6.7}
	v := p.toVector()

	if v[0] != p.X || v[1] != p.Y {
		t.Fatalf("Expected Vector (%v) coordinates to match the point (%v)", p, v)
	}

	if len(v) != 2 {
		t.Fatal("Expected the Vector to have 2 components")
	}
}

func TestVectorTo(t *testing.T) {
	p1 := &Point{1.5, 5.3}
	p2 := &Point{-2, 3}
	c := p1.VectorTo(p2)
	if !c.equals(NewVector(-3.5, -2.3)) {
		t.Fatalf("Expected Vector to be (-3.5, -2.3), got %v", c)
	}

	p1 = &Point{1.5, 5.3}
	p2 = &Point{-2, 3}
	c = p2.VectorTo(p1)
	if !c.equals(NewVector(3.5, 2.3)) {
		t.Fatalf("Expected Vector to be (3.5, 2.3), got %v", c)
	}
}
