package geo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSegmentIntersections(t *testing.T) {
	// mid intersection
	s1 := NewSegment(NewPoint(0, 0), NewPoint(10, 10))
	s2 := NewSegment(NewPoint(0, 10), NewPoint(10, 0))
	intersections := s1.Intersections(*s2)
	assert.Equal(t, len(intersections), 1)
	assert.True(t, intersections[0].Equals(NewPoint(5, 5)))

	// intersection at the end
	s3 := NewSegment(NewPoint(10, 10), NewPoint(10, 0))
	intersections = s1.Intersections(*s3)
	assert.Equal(t, len(intersections), 1)
	assert.True(t, intersections[0].Equals(NewPoint(10, 10)))

	// intersection at the beginning
	s4 := NewSegment(NewPoint(0, 0), NewPoint(0, 10))
	intersections = s1.Intersections(*s4)
	assert.Equal(t, len(intersections), 1)
	assert.True(t, intersections[0].Equals(NewPoint(0, 0)))

	// no intersection
	s5 := NewSegment(NewPoint(3, 8), NewPoint(2, 15))
	intersections = s1.Intersections(*s5)
	assert.Equal(t, len(intersections), 0)
}
