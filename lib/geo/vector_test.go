package geo

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtendVerticalLineSegments(t *testing.T) {
	p1 := &Point{0, 0}
	p2 := &Point{0, 1}

	v := p1.VectorTo(p2)
	v = v.Multiply(2)
	p2New := p1.AddVector(v)
	expected := Point{0, 2}
	assert.Equal(t, expected, *p2New)

	v = p2.VectorTo(p1)
	v = v.Multiply(2)
	p1New := p2.AddVector(v)
	expected = Point{0, -1}
	assert.Equal(t, expected, *p1New)
}

func TestExtendHorizontalLineSegment(t *testing.T) {
	p1 := &Point{0, 0}
	p2 := &Point{1, 0}

	v := p1.VectorTo(p2)
	v = v.Multiply(1.5)
	p2New := p1.AddVector(v)
	expected := Point{1.5, 0}
	assert.Equal(t, expected, *p2New)

	v = p2.VectorTo(p1)
	v = v.Multiply(1.5)
	p1New := p2.AddVector(v)
	expected = Point{-0.5, 0}
	assert.Equal(t, expected, *p1New)
}

func TestExtendDiagonalLineSegment(t *testing.T) {
	p1 := &Point{0, 0}
	p2 := &Point{3, 1}

	v := p1.VectorTo(p2)
	v = v.Multiply(2)
	p2New := p1.AddVector(v)
	expected := Point{6, 2}
	assert.Equal(t, expected, *p2New)

	v = p2.VectorTo(p1)
	v = v.Multiply(2)
	p1New := p2.AddVector(v)
	expected = Point{-3, -1}
	assert.Equal(t, expected, *p1New)
}

func TestVectorAdd(t *testing.T) {
	a := NewVector(1, 2)
	b := NewVector(3, 4)

	c := a.Add(b)

	assert.Truef(t, c.equals(NewVector(4, 6)), "Expected Vector %v to be (4, 6)", c)
}

func TestVectorMinus(t *testing.T) {
	a := NewVector(1, 2)
	b := NewVector(3, 4)

	c := a.Minus(b)

	assert.Truef(t, c.equals(NewVector(-2, -2)), "Expected Vector %v to be (-2, -2)", c)
}

func TestVectorMultiply(t *testing.T) {
	a := NewVector(1, 2)

	c := a.Multiply(3)

	assert.Truef(t, c.equals(NewVector(3, 6)), "Expected Vector %v to be (3, 6)", c)
}

func TestVectorLength(t *testing.T) {
	a := NewVector(3, 4)

	assert.Equal(t, 5.0, a.Length())
}

func TestNewVectorFromProperties(t *testing.T) {
	a := NewVectorFromProperties(3, math.Pi/3) // 60 degrees
	if !a.equals(NewVector(2.59807621135, 1.5)) {
		t.Errorf("expected Vector to be close to (2.59807, 1.5), got %v", a)
	}

	b := NewVectorFromProperties(3, -math.Pi/3) // -60 degrees
	if !b.equals(NewVector(-2.59807621135, 1.5)) {
		t.Errorf("expected Vector to be close to (-2.59807, 1.5), got %v", b)
	}

	c := NewVectorFromProperties(3, math.Pi*2/3) // 120 degrees
	if !c.equals(NewVector(2.59807621135, -1.5)) {
		t.Errorf("expected Vector to be close to (2.59807, -1.5), got %v", c)
	}

	d := NewVectorFromProperties(3, -math.Pi*2/3) // -120 degrees
	if !d.equals(NewVector(-2.59807621135, -1.5)) {
		t.Errorf("expected Vector to be close to (-2.59807, -1.5), got %v", c)
	}
}

func TestVectorUnit(t *testing.T) {
	a := NewVector(3, 4).Unit()
	expected := NewVector(3.0/5, 4.0/5)
	assert.Truef(t, a.equals(expected), "Expected (%f, %f) Vector, got %v", 3/5.0, 4/5.0, a)
}

func TestVectorAddLength(t *testing.T) {
	a := NewVector(3, 4)
	b := a.AddLength(8)

	if PrecisionCompare(b.Length(), 13.0, PRECISION) != 0 {
		t.Fatalf("Expected new Vector to have length 13, got %v", b.Length())
	}
}

func TestVectorEquals(t *testing.T) {
	a := NewVector(1, 2)
	assert.True(t, a.equals(a), "Expected Vector to be equal to itself")
	assert.True(t, a.equals(NewVector(1.0, 2.0)), "Expected Vector to be equal to different Vector with same components")
	assert.False(t, a.equals(NewVector(1, 2, 3)), "Expected Vector to be different if different component count")
	assert.False(t, a.equals(NewVector(2, 2)), "Expected Vector to be different if components are different")
}

func TestVectorToPoint(t *testing.T) {
	v := NewVector(3.789, -0.731)
	p := v.ToPoint()

	assert.Equal(t, v[0], p.X)
	assert.Equal(t, v[1], p.Y)
}

func (a Vector) equals(other Vector) bool {
	if len(a) != len(other) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if PrecisionCompare(a[i], other[i], PRECISION) != 0 {
			return false
		}
	}
	return true
}
