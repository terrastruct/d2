package d2sketch

import (
	"context"
	"fmt"

	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2target"
	rough "github.com/d2lang/rough-go"
)

const (
	connectionCurveSteps           = 8
	connectionOperationsPerSegment = 4
	connectionSegmentsPerSet       = 256
)

// ConnectionDrawable converts a typed connection path into deterministic
// rough-go operations without serializing or parsing SVG path data. Cubics are
// flattened at a fixed resolution before roughening; this keeps generation
// work predictable while retaining the route's rounded/curved silhouette.
// limits bound the rough intermediate before any expanded point slice is
// allocated. Long routes are generated in fixed-size sets so cancellation is
// observed between bounded rough-go calls.
func ConnectionDrawable(ctx context.Context, path d2scene.Path, limits SceneLimits) (rough.Drawable, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if limits.MaxSets <= 0 || limits.MaxOps <= 0 {
		return rough.Drawable{}, fmt.Errorf("d2sketch: connection scene limits must be positive")
	}
	if err := ctx.Err(); err != nil {
		return rough.Drawable{}, err
	}

	segments := 0
	haveMove := false
	for index, command := range path.Commands {
		if index&255 == 0 {
			if err := ctx.Err(); err != nil {
				return rough.Drawable{}, err
			}
		}
		switch command.Kind {
		case d2scene.MoveCommand:
			if haveMove {
				return rough.Drawable{}, fmt.Errorf("d2sketch: connection path has multiple subpaths")
			}
			haveMove = true
		case d2scene.LineCommand:
			if !haveMove {
				return rough.Drawable{}, fmt.Errorf("d2sketch: connection path command %d is before its move", index)
			}
			segments++
		case d2scene.CubicCommand:
			if !haveMove {
				return rough.Drawable{}, fmt.Errorf("d2sketch: connection path command %d is before its move", index)
			}
			segments += connectionCurveSteps
		default:
			return rough.Drawable{}, fmt.Errorf("d2sketch: connection path command %d has unsupported kind %d", index, command.Kind)
		}
		if segments > limits.MaxOps/connectionOperationsPerSegment {
			return rough.Drawable{}, fmt.Errorf("d2sketch: connection input expands beyond operation limit %d", limits.MaxOps)
		}
	}
	if !haveMove || segments == 0 {
		return rough.Drawable{}, fmt.Errorf("d2sketch: connection path has no painted segments")
	}
	requiredSets := (segments + connectionSegmentsPerSet - 1) / connectionSegmentsPerSet
	if requiredSets > limits.MaxSets {
		return rough.Drawable{}, fmt.Errorf("d2sketch: connection input expands beyond operation set limit %d", limits.MaxSets)
	}

	points := make([]rough.Point, 0, segments+1)
	var current d2scene.Point
	for index, command := range path.Commands {
		if index&255 == 0 {
			if err := ctx.Err(); err != nil {
				return rough.Drawable{}, err
			}
		}
		switch command.Kind {
		case d2scene.MoveCommand:
			current = command.P1
			points = append(points, rough.Point{current.X, current.Y})
		case d2scene.LineCommand:
			current = command.P1
			points = append(points, rough.Point{current.X, current.Y})
		case d2scene.CubicCommand:
			start := current
			for step := 1; step <= connectionCurveSteps; step++ {
				t := float64(step) / connectionCurveSteps
				point := cubicPoint(start, command.P1, command.P2, command.P3, t)
				points = append(points, rough.Point{point.X, point.Y})
			}
			current = command.P3
		}
	}
	if err := ctx.Err(); err != nil {
		return rough.Drawable{}, err
	}
	generator := newGenerator()
	var combined rough.Drawable
	for firstSegment, setIndex := 0, 0; firstSegment < segments; firstSegment, setIndex = firstSegment+connectionSegmentsPerSet, setIndex+1 {
		if err := ctx.Err(); err != nil {
			return rough.Drawable{}, err
		}
		lastSegment := firstSegment + connectionSegmentsPerSet
		if lastSegment > segments {
			lastSegment = segments
		}
		drawable := generator.LinearPath(points[firstSegment:lastSegment+1], &rough.Options{
			Roughness: rough.Float64(.5),
			Seed:      rough.Float64(1 + float64(setIndex)),
		})
		if combined.Options == nil {
			combined.Shape = "connection"
			combined.Options = drawable.Options
		}
		combined.Sets = append(combined.Sets, drawable.Sets...)
	}
	return combined, nil
}

func cubicPoint(start, control1, control2, end d2scene.Point, t float64) d2scene.Point {
	lerp := func(a, b d2scene.Point) d2scene.Point {
		return d2scene.Point{X: a.X + (b.X-a.X)*t, Y: a.Y + (b.Y-a.Y)*t}
	}
	a := lerp(start, control1)
	b := lerp(control1, control2)
	c := lerp(control2, end)
	d := lerp(a, b)
	e := lerp(b, c)
	return lerp(d, e)
}

// ArrowheadDrawables returns the bounded, structured rough-go geometry for one
// D2 arrowhead. Every generator input has a fixed number of points, and each
// endpoint gets a fresh seeded generator so output is independent of object or
// frame traversal order.
func ArrowheadDrawables(arrowhead d2target.Arrowhead, stroke string, strokeWidth int) ([]rough.Drawable, bool) {
	generator := newGenerator()
	line := func(points ...rough.Point) rough.Drawable {
		return generator.LinearPath(points, arrowOptions(stroke, strokeWidth, 3))
	}
	polygon := func(points []rough.Point, fill string, seed float64) rough.Drawable {
		return generator.Polygon(points, solidArrowOptions(stroke, fill, strokeWidth, -1, seed))
	}

	switch arrowhead {
	case d2target.ArrowArrowhead, d2target.LineArrowhead:
		return []rough.Drawable{line(rough.Point{-10, -4}, rough.Point{0, 0}, rough.Point{-10, 4})}, true
	case d2target.TriangleArrowhead:
		return []rough.Drawable{polygon([]rough.Point{{-10, -4}, {0, 0}, {-10, 4}}, stroke, 2)}, true
	case d2target.UnfilledTriangleArrowhead:
		return []rough.Drawable{polygon([]rough.Point{{-10, -4}, {0, 0}, {-10, 4}}, BG_COLOR, 2)}, true
	case d2target.DiamondArrowhead:
		return []rough.Drawable{polygon([]rough.Point{{-20, 0}, {-10, 5}, {0, 0}, {-10, -5}, {-20, 0}}, BG_COLOR, 1)}, true
	case d2target.FilledDiamondArrowhead:
		options := solidArrowOptions(stroke, stroke, strokeWidth, 4, 1)
		options.FillStyle = rough.String("zigzag")
		return []rough.Drawable{generator.Polygon([]rough.Point{{-20, 0}, {-10, 5}, {0, 0}, {-10, -5}, {-20, 0}}, options)}, true
	case d2target.FilledCircleArrowhead:
		return []rough.Drawable{generator.Circle(-2, -1, 8, solidArrowOptions(stroke, stroke, strokeWidth, 1, 5))}, true
	case d2target.CircleArrowhead:
		return []rough.Drawable{generator.Circle(-2, -1, 8, solidArrowOptions(stroke, BG_COLOR, strokeWidth, 1, 5))}, true
	case d2target.CrossArrowhead:
		return []rough.Drawable{line(rough.Point{-6, -6}, rough.Point{6, 6}, rough.Point{0, 0}, rough.Point{-6, 6}, rough.Point{0, 0}, rough.Point{6, -6})}, true
	case d2target.FilledBoxArrowhead:
		return []rough.Drawable{polygon([]rough.Point{{0, -10}, {0, 10}, {-20, 10}, {-20, -10}}, stroke, 1)}, true
	case d2target.BoxArrowhead:
		return []rough.Drawable{polygon([]rough.Point{{0, -10}, {0, 10}, {-20, 10}, {-20, -10}}, BG_COLOR, 1)}, true
	case d2target.CfManyRequired:
		return []rough.Drawable{
			generator.Line(-15, -10, -15, 10, arrowOptions(stroke, strokeWidth, 2)),
			generator.LinearPath([]rough.Point{{0, 10}, {-15, 0}, {0, -10}}, arrowOptions(stroke, strokeWidth, 2)),
		}, true
	case d2target.CfMany:
		return []rough.Drawable{
			generator.LinearPath([]rough.Point{{0, 10}, {-15, 0}, {0, -10}}, arrowOptions(stroke, strokeWidth, 8)),
			generator.Circle(-20, 0, 8, solidArrowOptions(stroke, BG_COLOR, strokeWidth, 1, 4)),
		}, true
	case d2target.CfOneRequired:
		return []rough.Drawable{
			generator.Line(-15, -10, -15, 10, arrowOptions(stroke, strokeWidth, 2)),
			generator.Line(-10, -10, -10, 10, arrowOptions(stroke, strokeWidth, 2)),
		}, true
	case d2target.CfOne:
		return []rough.Drawable{
			generator.Line(-10, -10, -10, 10, arrowOptions(stroke, strokeWidth, 3)),
			generator.Circle(-20, 0, 8, solidArrowOptions(stroke, BG_COLOR, strokeWidth, 1, 5)),
		}, true
	default:
		return nil, false
	}
}
