package layoutgraph

import (
	"context"
	"fmt"

	"github.com/d2lang/d2/lib/geo"
)

const scoringCancellationCheckInterval = 64

// CrossingSegment is an oriented line segment used by crossing scoring.
type CrossingSegment struct {
	Start geo.Point
	End   geo.Point
}

func scoringCancellationError(ctx context.Context, iteration int) error {
	if iteration%scoringCancellationCheckInterval != 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("EdgeLength: %w", err)
	}
	return nil
}

// CountSegmentCrossings counts intersections between segments while ignoring
// pairs with equal starts or equal ends. It is the common geometric kernel used
// by layout cost evaluation and hierarchy rank optimization.
func CountSegmentCrossings(segments []CrossingSegment) int64 {
	crossings, _ := countSegmentCrossingsWithCheck(segments, nil)
	return crossings
}

// CountSegmentCrossingsContext is CountSegmentCrossings with the layout
// engine's bounded cancellation checks.
func CountSegmentCrossingsContext(ctx context.Context, segments []CrossingSegment) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("EdgeLength: %w", err)
	}
	crossings, err := countSegmentCrossingsWithCheck(segments, func(iteration int) error {
		return scoringCancellationError(ctx, iteration)
	})
	if err != nil {
		return 0, err
	}
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("EdgeLength: %w", err)
	}
	return crossings, nil
}

func countSegmentCrossingsWithCheck(segments []CrossingSegment, checkCanceled func(iteration int) error) (int64, error) {
	var crossings int64
	var iStart, iEnd, jStart, jEnd *geo.Point
	var minX, maxX, minY, maxY float64
	var j int
	for i := 0; i < len(segments)-1; i++ {
		if checkCanceled != nil {
			if err := checkCanceled(i); err != nil {
				return 0, err
			}
		}
		iStart = &segments[i].Start
		iEnd = &segments[i].End
		minX, maxX = iStart.X, iEnd.X
		if maxX < minX {
			minX, maxX = maxX, minX
		}
		minY, maxY = iStart.Y, iEnd.Y
		if maxY < minY {
			minY, maxY = maxY, minY
		}
		for j = i + 1; j < len(segments); j++ {
			if checkCanceled != nil {
				if err := checkCanceled(j - i - 1); err != nil {
					return 0, err
				}
			}
			jStart = &segments[j].Start
			jEnd = &segments[j].End
			if jStart.X < jEnd.X {
				if jEnd.X < minX {
					continue
				}
				if maxX < jStart.X {
					continue
				}
			} else {
				if jStart.X < minX {
					continue
				}
				if maxX < jEnd.X {
					continue
				}
			}
			if jStart.Y < jEnd.Y {
				if jEnd.Y < minY {
					continue
				}
				if maxY < jStart.Y {
					continue
				}
			} else {
				if jStart.Y < minY {
					continue
				}
				if maxY < jEnd.Y {
					continue
				}
			}

			if nonNilEquals(iStart, jStart) || nonNilEquals(iEnd, jEnd) {
				continue
			}
			if SegmentsCross(iStart, iEnd, jStart, jEnd) {
				crossings++
			}
		}
	}
	return crossings, nil
}

// SegmentsCross reports whether two finite segments intersect. Parallel and
// overlapping segments do not count as crossings.
func SegmentsCross(u0, u1, v0, v1 *geo.Point) bool {
	denom := ((u1.Y-u0.Y)*(v1.X-v0.X) - (u1.X-u0.X)*(v1.Y-v0.Y))
	if denom == 0 {
		return false
	}
	s := ((v1.X-v0.X)*(v0.Y-u0.Y) - (v1.Y-v0.Y)*(v0.X-u0.X)) / denom
	if s < 0 || s > 1 {
		return false
	}
	t := ((u1.X-u0.X)*(v0.Y-u0.Y) - (u1.Y-u0.Y)*(v0.X-u0.X)) / denom
	return t >= 0 && t <= 1
}
