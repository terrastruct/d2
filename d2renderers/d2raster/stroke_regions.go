package d2raster

import (
	"context"
	"image"
	"math"

	"github.com/d2lang/d2/d2renderers/d2scene"
)

// effectRenderBounds tightens an effect layer around the actual stroke
// segments crossing this destination. A long connection can have a wide full
// frame bounding box while occupying only a narrow part of a horizontal band.
// Fills and filtered nodes retain their established conservative bounds.
func effectRenderBounds(ctx context.Context, node *preparedNode, destination image.Rectangle) (image.Rectangle, error) {
	if err := ctx.Err(); err != nil {
		return image.Rectangle{}, err
	}
	if node == nil || node.opacity == 0 {
		return image.Rectangle{}, nil
	}
	visible := node.bounds.Intersect(destination)
	if visible.Empty() || len(node.filters) != 0 {
		return visible, nil
	}
	var bounds image.Rectangle
	if primitive := node.primitive; primitive != nil {
		if primitive.vector != nil {
			var err error
			bounds, err = effectRenderBounds(ctx, primitive.vector, visible)
			if err != nil {
				return image.Rectangle{}, err
			}
		} else if primitive.fill == nil && primitive.stroke != nil && primitive.image == nil {
			var err error
			bounds, err = strokeRegionBounds(ctx, primitive, visible)
			if err != nil {
				return image.Rectangle{}, err
			}
		} else {
			bounds = primitive.bounds.Intersect(visible)
		}
	}
	for _, child := range node.children {
		childBounds, err := effectRenderBounds(ctx, child, visible)
		if err != nil {
			return image.Rectangle{}, err
		}
		bounds = unionRect(bounds, childBounds)
	}
	return bounds.Intersect(visible), nil
}

func strokeRegionBounds(ctx context.Context, primitive *preparedPrimitive, destination image.Rectangle) (image.Rectangle, error) {
	fallback := primitive.bounds.Intersect(destination)
	if fallback.Empty() || math.Abs(float64(primitive.referenceBounds.Min.X)) > 1<<20 || math.Abs(float64(primitive.referenceBounds.Min.Y)) > 1<<20 {
		return fallback, nil
	}
	expansion := strokeExtent(primitive.stroke, primitive.transform) + 1
	if !finite(expansion) || expansion > 1<<20 {
		return fallback, nil
	}
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	lowerY := float64(destination.Min.Y) - expansion
	upperY := float64(destination.Max.Y) + expansion
	segments := 0
	for _, run := range primitive.strokeRuns {
		count := len(run.points) - 1
		if run.closed {
			count++
		}
		for index := 0; index < count; index++ {
			if segments&255 == 0 {
				if err := ctx.Err(); err != nil {
					return image.Rectangle{}, err
				}
			}
			segments++
			from := primitive.transform.Point(run.points[index])
			to := primitive.transform.Point(run.points[(index+1)%len(run.points)])
			if !boundedRegionPoint(from) || !boundedRegionPoint(to) {
				return fallback, nil
			}
			if max(from.Y, to.Y) < lowerY || min(from.Y, to.Y) > upperY {
				continue
			}
			first, last := 0.0, 1.0
			dx, dy := to.X-from.X, to.Y-from.Y
			if dy != 0 {
				t0, t1 := (lowerY-from.Y)/dy, (upperY-from.Y)/dy
				first, last = max(0, min(t0, t1)), min(1, max(t0, t1))
			}
			x0, x1 := from.X+first*dx, from.X+last*dx
			y0, y1 := from.Y+first*dy, from.Y+last*dy
			minX, minY = min(minX, x0, x1), min(minY, y0, y1)
			maxX, maxY = max(maxX, x0, x1), max(maxY, y0, y1)
		}
	}
	if math.IsInf(minX, 1) {
		return image.Rectangle{}, nil
	}
	minX = max(minX-expansion, float64(destination.Min.X))
	minY = max(minY-expansion, float64(destination.Min.Y))
	maxX = min(maxX+expansion, float64(destination.Max.X))
	maxY = min(maxY+expansion, float64(destination.Max.Y))
	if minX >= maxX || minY >= maxY {
		return image.Rectangle{}, nil
	}
	return image.Rect(int(math.Floor(minX)), int(math.Floor(minY)), int(math.Ceil(maxX)), int(math.Ceil(maxY))).Intersect(destination), nil
}

func boundedRegionPoint(point d2scene.Point) bool {
	return finitePoint(point) && math.Abs(point.X) <= 1<<20 && math.Abs(point.Y) <= 1<<20
}
