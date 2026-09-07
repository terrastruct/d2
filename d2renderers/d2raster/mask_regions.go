package d2raster

import (
	"context"
	"image"
	"math"

	"github.com/d2lang/d2/d2renderers/d2scene"
)

// maskRenderBounds omits the unchanged white area of a luminance mask whose
// first child is an opaque rectangular backdrop. D2's connection-label mask
// uses exactly this representation: a diagram-wide white rectangle followed by
// small label cutouts. Rendering that white area for every connection would
// otherwise scale with the connections' bounding-box areas.
//
// Reference-coordinate rendering is required because a smaller destination
// must not change the float32 rounding of antialiased mask geometry. Complete
// frames retain their existing path; band rendering already supplies stable
// reference origins for every node, including masks.
func maskRenderBounds(ctx context.Context, mask *preparedMask, destination image.Rectangle, referenceCoordinates bool) (image.Rectangle, error) {
	if err := ctx.Err(); err != nil {
		return image.Rectangle{}, err
	}
	if !referenceCoordinates || mask == nil || mask.kind != d2scene.MaskLuminance || !plainMaskNode(mask.root) || mask.root.primitive != nil || len(mask.root.children) == 0 {
		return destination, nil
	}
	base := mask.root.children[0]
	if !plainMaskNode(base) || len(base.children) != 0 || base.primitive == nil {
		return destination, nil
	}
	primitive := base.primitive
	if primitive.fill == nil || primitive.fill.kind != preparedSolidPaint || primitive.fill.solid.R != 255 || primitive.fill.solid.G != 255 || primitive.fill.solid.B != 255 || primitive.fill.solid.A != 255 || primitive.stroke != nil || primitive.image != nil || primitive.vector != nil || len(primitive.subpaths) != 1 {
		return destination, nil
	}
	// A large translation that cancels local coordinates can round differently
	// when the renderer subtracts its reference origin before transformation.
	if math.Abs(float64(primitive.referenceBounds.Min.X)) > 1<<20 || math.Abs(float64(primitive.referenceBounds.Min.Y)) > 1<<20 || !boundedRegionPoint(d2scene.Point{X: primitive.transform.E, Y: primitive.transform.F}) {
		return destination, nil
	}
	points := primitive.subpaths[0].points
	if len(points) != 4 {
		return destination, nil
	}
	var corners [4]d2scene.Point
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for index, point := range points {
		point = primitive.transform.Point(point)
		// The one-pixel inset below is deliberately conservative. Within this
		// coordinate range float32 endpoint rounding cannot consume the inset.
		if !finitePoint(point) || math.Abs(point.X) > 1<<20 || math.Abs(point.Y) > 1<<20 {
			return destination, nil
		}
		corners[index] = point
		minX, minY = min(minX, point.X), min(minY, point.Y)
		maxX, maxY = max(maxX, point.X), max(maxY, point.Y)
	}
	seenCorners := uint8(0)
	for index, point := range corners {
		next := corners[(index+1)%len(corners)]
		if point.X != minX && point.X != maxX || point.Y != minY && point.Y != maxY || point == next || (point.X == next.X) == (point.Y == next.Y) {
			return destination, nil
		}
		corner := uint8(0)
		if point.X == maxX {
			corner |= 1
		}
		if point.Y == maxY {
			corner |= 2
		}
		seenCorners |= 1 << corner
	}
	if seenCorners != 0xf {
		return destination, nil
	}
	interior := image.Rect(int(math.Ceil(minX))+1, int(math.Ceil(minY))+1, int(math.Floor(maxX))-1, int(math.Floor(maxY))-1)
	if !destination.In(interior) {
		return destination, nil
	}
	var changed image.Rectangle
	for index, child := range mask.root.children[1:] {
		if index&255 == 0 {
			if err := ctx.Err(); err != nil {
				return image.Rectangle{}, err
			}
		}
		if child != nil && child.opacity != 0 {
			changed = unionRect(changed, child.bounds.Intersect(destination))
		}
	}
	return changed, nil
}

func plainMaskNode(node *preparedNode) bool {
	return node != nil && node.opacity == 1 && node.blend == d2scene.BlendNormal && !node.isolated && len(node.filters) == 0 && node.clip == nil && node.mask == nil
}
