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

func TestSegmentIntersectCircle(t *testing.T) {
	origin := NewPoint(0, 0)

	// segment passing through the origin → 2 intersections at (-r, 0) and (r, 0)
	s := NewSegment(NewPoint(-10, 0), NewPoint(10, 0))
	pts := s.IntersectCircle(origin, 5)
	assert.Equal(t, 2, len(pts))
	assert.InDelta(t, -5, pts[0].X, 1e-9)
	assert.InDelta(t, 0, pts[0].Y, 1e-9)
	assert.InDelta(t, 5, pts[1].X, 1e-9)
	assert.InDelta(t, 0, pts[1].Y, 1e-9)

	// segment crossing the circle once with one endpoint inside
	s = NewSegment(NewPoint(0, 0), NewPoint(10, 0))
	pts = s.IntersectCircle(origin, 5)
	assert.Equal(t, 1, len(pts))
	assert.InDelta(t, 5, pts[0].X, 1e-9)
	assert.InDelta(t, 0, pts[0].Y, 1e-9)

	// segment entirely outside the circle, no crossing
	s = NewSegment(NewPoint(10, 10), NewPoint(20, 20))
	pts = s.IntersectCircle(origin, 5)
	assert.Equal(t, 0, len(pts))

	// segment entirely inside the circle, no crossing
	s = NewSegment(NewPoint(-1, 0), NewPoint(1, 0))
	pts = s.IntersectCircle(origin, 5)
	assert.Equal(t, 0, len(pts))

	// segment endpoint on the circle
	s = NewSegment(NewPoint(0, 0), NewPoint(5, 0))
	pts = s.IntersectCircle(origin, 5)
	assert.Equal(t, 1, len(pts))
	assert.InDelta(t, 5, pts[0].X, 1e-9)
	assert.InDelta(t, 0, pts[0].Y, 1e-9)

	// vertical segment chord intersecting the circle at two points
	s = NewSegment(NewPoint(3, -10), NewPoint(3, 10))
	pts = s.IntersectCircle(origin, 5)
	assert.Equal(t, 2, len(pts))
	assert.InDelta(t, 3, pts[0].X, 1e-9)
	assert.InDelta(t, -4, pts[0].Y, 1e-9)
	assert.InDelta(t, 3, pts[1].X, 1e-9)
	assert.InDelta(t, 4, pts[1].Y, 1e-9)

	// circle centered off origin
	c := NewPoint(10, 10)
	s = NewSegment(NewPoint(0, 10), NewPoint(20, 10))
	pts = s.IntersectCircle(c, 5)
	assert.Equal(t, 2, len(pts))
	assert.InDelta(t, 5, pts[0].X, 1e-9)
	assert.InDelta(t, 10, pts[0].Y, 1e-9)
	assert.InDelta(t, 15, pts[1].X, 1e-9)
	assert.InDelta(t, 10, pts[1].Y, 1e-9)

	// tangent contact: the segment grazes the circle at a single point and
	// must not be reported twice (regression test for the duplicated-root
	// case when the discriminant is zero).
	s = NewSegment(NewPoint(-10, 5), NewPoint(10, 5))
	pts = s.IntersectCircle(origin, 5)
	assert.Equal(t, 1, len(pts))
	assert.InDelta(t, 0, pts[0].X, 1e-9)
	assert.InDelta(t, 5, pts[0].Y, 1e-9)

	// degenerate zero-length segment returns nil
	s = NewSegment(NewPoint(7, 7), NewPoint(7, 7))
	pts = s.IntersectCircle(origin, 5)
	assert.Nil(t, pts)
}
