package geo

import (
	"fmt"
	"math"
)

type Intersectable interface {
	Intersections(segment Segment) []*Point
}

type Segment struct {
	Start *Point
	End   *Point
}

func NewSegment(from, to *Point) *Segment {
	return &Segment{from, to}
}

func (s Segment) Overlaps(otherS Segment, isHorizontal bool, buffer float64) bool {
	if isHorizontal {
		if s.Start.Y-otherS.End.Y >= buffer {
			return false
		}
		if otherS.Start.Y-s.End.Y >= buffer {
			return false
		}
		return true
	} else {
		if s.Start.X-otherS.End.X >= buffer {
			return false
		}
		if otherS.Start.X-s.End.X >= buffer {
			return false
		}
		return true
	}
}

func (segment Segment) Intersects(otherSegment Segment) bool {
	return IntersectionPoint(segment.Start, segment.End, otherSegment.Start, otherSegment.End) != nil
}

//nolint:unused
func (s Segment) ToString() string {
	return fmt.Sprintf("%v -> %v", s.Start.ToString(), s.End.ToString())
}

func (segment Segment) Intersections(otherSegment Segment) []*Point {
	point := IntersectionPoint(segment.Start, segment.End, otherSegment.Start, otherSegment.End)
	if point == nil {
		return nil
	}
	return []*Point{point}
}

// getBounds takes a segment and returns the floor and ceil of where it can shift to
// If there is no floor or ceiling, negative or positive infinity is used, respectively
// The direction is inferred, e.g. b/c the passed in segment is vertical, it's inferred we want horizontal bounds
// buffer says how close the segment can be, on both axes, to other segments given
//    │              │
//    │              │
//    │              │
//    │              │
//    │           non-overlap
//    │
//    │
//    │
//    │     segment
//    │       │
//    │       │         ceil
//    │       │            │
//            │            │
// floor      │            │
//                         │
//                         │
//                         │
//                         │
// NOTE: the assumption is that all segments given are orthogonal
func (segment *Segment) GetBounds(segments []*Segment, buffer float64) (float64, float64) {
	ceil := math.Inf(1)
	floor := math.Inf(-1)
	if segment.Start.X == segment.End.X && segment.Start.Y == segment.End.Y {
		// single point, no segment
		return floor, ceil
	}
	isHorizontal := segment.Start.X == segment.End.X
	for _, otherSegment := range segments {
		if isHorizontal {
			// Exclude segments that don't overlap (non-overlap in above diagram)
			if otherSegment.End.Y < segment.Start.Y-buffer {
				continue
			}
			if otherSegment.Start.Y > segment.End.Y+buffer {
				continue
			}
			if otherSegment.Start.X <= segment.Start.X {
				floor = math.Max(floor, otherSegment.Start.X)
			}
			if otherSegment.Start.X > segment.Start.X {
				ceil = math.Min(ceil, otherSegment.Start.X)
			}
		} else {
			if otherSegment.End.X < segment.Start.X-buffer {
				continue
			}
			if otherSegment.Start.X > segment.End.X+buffer {
				continue
			}
			if otherSegment.Start.Y <= segment.Start.Y {
				floor = math.Max(floor, otherSegment.Start.Y)
			}
			if otherSegment.Start.Y > segment.Start.Y {
				ceil = math.Min(ceil, otherSegment.Start.Y)
			}
		}
	}
	return floor, ceil
}

func (segment Segment) Length() float64 {
	return EuclideanDistance(segment.Start.X, segment.Start.Y, segment.End.X, segment.End.Y)
}

func (segment Segment) ToVector() Vector {
	return NewVector(segment.End.X-segment.Start.X, segment.End.Y-segment.Start.Y)
}
