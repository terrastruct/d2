package d2raster

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"

	"github.com/d2lang/d2/d2renderers/d2scene"
)

type preparedFilterKind uint8

const (
	preparedGaussianBlur preparedFilterKind = iota
	preparedDropShadow
)

type blurAxis uint8

const (
	blurHorizontal blurAxis = iota
	blurVertical
)

type blurPass struct {
	axis   blurAxis
	radius int
	bounds image.Rectangle
}

type preparedFilter struct {
	kind        preparedFilterKind
	passes      []blurPass
	output      image.Rectangle
	offsetX     float64
	offsetY     float64
	shadowColor color.NRGBA
}

type normalizedFilter struct {
	kind        preparedFilterKind
	sigmaX      float64
	sigmaY      float64
	offsetX     float64
	offsetY     float64
	shadowColor color.NRGBA
	sourceIndex int
}

func normalizeNodeFilters(ctx context.Context, nodeID string, filters []d2scene.Filter, animated map[int]d2scene.DropShadow) ([]normalizedFilter, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(animated) != 0 {
		indices := make([]int, 0, len(animated))
		for index := range animated {
			indices = append(indices, index)
		}
		sort.Ints(indices)
		for _, index := range indices {
			if index < 0 || index >= len(filters) {
				return nil, fmt.Errorf("d2raster: node %q drop-shadow animation target index %d is outside %d filters", nodeID, index, len(filters))
			}
		}
	}

	result := make([]normalizedFilter, 0, len(filters))
	for index, raw := range filters {
		if index&255 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		var filter normalizedFilter
		filter.sourceIndex = index
		switch value := raw.(type) {
		case d2scene.GaussianBlur:
			filter.kind, filter.sigmaX, filter.sigmaY = preparedGaussianBlur, value.SigmaX, value.SigmaY
		case *d2scene.GaussianBlur:
			if value == nil {
				return nil, fmt.Errorf("d2raster: node %q filter %d is a nil Gaussian blur", nodeID, index)
			}
			filter.kind, filter.sigmaX, filter.sigmaY = preparedGaussianBlur, value.SigmaX, value.SigmaY
		case d2scene.DropShadow:
			filter.kind = preparedDropShadow
			filter.sigmaX, filter.sigmaY = value.SigmaX, value.SigmaY
			filter.offsetX, filter.offsetY, filter.shadowColor = value.OffsetX, value.OffsetY, value.Color
		case *d2scene.DropShadow:
			if value == nil {
				return nil, fmt.Errorf("d2raster: node %q filter %d is a nil drop shadow", nodeID, index)
			}
			filter.kind = preparedDropShadow
			filter.sigmaX, filter.sigmaY = value.SigmaX, value.SigmaY
			filter.offsetX, filter.offsetY, filter.shadowColor = value.OffsetX, value.OffsetY, value.Color
		default:
			return nil, fmt.Errorf("d2raster: node %q filter %d has unsupported type %T", nodeID, index, raw)
		}
		if err := validateNormalizedFilter(nodeID, index, filter); err != nil {
			return nil, err
		}
		if override, ok := animated[index]; ok {
			if filter.kind != preparedDropShadow {
				return nil, fmt.Errorf("d2raster: node %q drop-shadow animation target index %d does not identify a drop shadow", nodeID, index)
			}
			filter.sigmaX, filter.sigmaY = override.SigmaX, override.SigmaY
			filter.offsetX, filter.offsetY, filter.shadowColor = override.OffsetX, override.OffsetY, override.Color
			if err := validateNormalizedFilter(nodeID, index, filter); err != nil {
				return nil, err
			}
		}
		result = append(result, filter)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func validateNormalizedFilter(nodeID string, index int, filter normalizedFilter) error {
	if filter.sigmaX < 0 || filter.sigmaY < 0 || !finite(filter.sigmaX) || !finite(filter.sigmaY) {
		return fmt.Errorf("d2raster: node %q filter %d has invalid Gaussian deviation", nodeID, index)
	}
	if filter.kind == preparedDropShadow && (!finite(filter.offsetX) || !finite(filter.offsetY)) {
		return fmt.Errorf("d2raster: node %q filter %d has an invalid drop-shadow offset", nodeID, index)
	}
	return nil
}

func (p *preflight) prepareFilters(nodeID string, filters []normalizedFilter, transform d2scene.Matrix, input image.Rectangle) ([]preparedFilter, image.Rectangle, error) {
	current := input
	// Every supported filter maps transparent black to transparent black. The
	// normalized pass above has already validated all declared values, so an
	// empty subtree needs neither transformed-kernel arithmetic nor scratch.
	if current.Empty() {
		return nil, current, p.ctx.Err()
	}
	result := make([]preparedFilter, 0, len(filters))
	for index, filter := range filters {
		if err := p.ctx.Err(); err != nil {
			return nil, image.Rectangle{}, err
		}
		// A fully transparent shadow is an identity operation even when its
		// otherwise valid finite deviation would exceed the raster work domain.
		if filter.kind == preparedDropShadow && filter.shadowColor.A == 0 {
			continue
		}
		sigmaX := math.Hypot(transform.A*filter.sigmaX, transform.C*filter.sigmaY)
		sigmaY := math.Hypot(transform.B*filter.sigmaX, transform.D*filter.sigmaY)
		if !finite(sigmaX) || !finite(sigmaY) {
			return nil, image.Rectangle{}, fmt.Errorf("d2raster: node %q filter %d has non-finite transformed deviation", nodeID, filter.sourceIndex)
		}
		radiusX, err := blurSupportRadius(sigmaX)
		if err != nil {
			return nil, image.Rectangle{}, fmt.Errorf("d2raster: node %q filter %d horizontal deviation: %w", nodeID, filter.sourceIndex, err)
		}
		radiusY, err := blurSupportRadius(sigmaY)
		if err != nil {
			return nil, image.Rectangle{}, fmt.Errorf("d2raster: node %q filter %d vertical deviation: %w", nodeID, filter.sourceIndex, err)
		}

		prepared := preparedFilter{kind: filter.kind, shadowColor: filter.shadowColor}
		switch filter.kind {
		case preparedGaussianBlur:
			if current.Empty() || radiusX == 0 && radiusY == 0 {
				continue
			}
			prepared.passes, prepared.output, err = prepareBlurPasses(current, radiusX, radiusY)
			if err != nil {
				return nil, image.Rectangle{}, fmt.Errorf("d2raster: node %q filter %d: %w", nodeID, filter.sourceIndex, err)
			}
			current = prepared.output
		case preparedDropShadow:
			offset := transform.Vector(d2scene.Point{X: filter.offsetX, Y: filter.offsetY})
			if !finitePoint(offset) {
				return nil, image.Rectangle{}, fmt.Errorf("d2raster: node %q filter %d has a non-finite transformed shadow offset", nodeID, filter.sourceIndex)
			}
			prepared.offsetX, prepared.offsetY = offset.X, offset.Y
			blurred := current
			prepared.passes, blurred, err = prepareBlurPasses(current, radiusX, radiusY)
			if err != nil {
				return nil, image.Rectangle{}, fmt.Errorf("d2raster: node %q filter %d: %w", nodeID, filter.sourceIndex, err)
			}
			shadowBounds, err := translatedFilterBoundsUnbounded(blurred, offset.X, offset.Y)
			if err != nil {
				return nil, image.Rectangle{}, fmt.Errorf("d2raster: node %q filter %d: %w", nodeID, filter.sourceIndex, err)
			}
			prepared.output, err = unionFilterBounds(current, shadowBounds)
			if err != nil {
				return nil, image.Rectangle{}, fmt.Errorf("d2raster: node %q filter %d: %w", nodeID, filter.sourceIndex, err)
			}
			current = prepared.output
		default:
			return nil, image.Rectangle{}, fmt.Errorf("d2raster: node %q filter %d has unknown prepared kind %d", nodeID, index, filter.kind)
		}
		result = append(result, prepared)
	}
	return result, current, nil
}

const maxBlurSupportRadius = 1 << 30

func blurSupportRadius(sigma float64) (int, error) {
	if sigma == 0 {
		return 0, nil
	}
	expansion := math.Ceil(3 * sigma)
	if !finite(expansion) || expansion > maxBlurSupportRadius {
		return 0, fmt.Errorf("three-sigma support exceeds the bounded raster domain")
	}
	return int(expansion), nil
}

func prepareBlurPasses(input image.Rectangle, radiusX, radiusY int) ([]blurPass, image.Rectangle, error) {
	xRadii := splitBlurRadius(radiusX)
	yRadii := splitBlurRadius(radiusY)
	current := input
	passes := make([]blurPass, 0, 6)
	for index := range 3 {
		if xRadii[index] != 0 {
			var err error
			current, err = expandFilterBounds(current, xRadii[index], 0)
			if err != nil {
				return nil, image.Rectangle{}, err
			}
			passes = append(passes, blurPass{axis: blurHorizontal, radius: xRadii[index], bounds: current})
		}
		if yRadii[index] != 0 {
			var err error
			current, err = expandFilterBounds(current, 0, yRadii[index])
			if err != nil {
				return nil, image.Rectangle{}, err
			}
			passes = append(passes, blurPass{axis: blurVertical, radius: yRadii[index], bounds: current})
		}
	}
	return passes, current, nil
}

func splitBlurRadius(radius int) [3]int {
	// Three successive boxes converge toward a Gaussian while retaining an
	// O(pixel-count) sliding-window implementation. Splitting the conservative
	// three-sigma support across the passes makes the actual finite support
	// exactly match the bounds charged during preflight.
	result := [3]int{radius / 3, radius / 3, radius / 3}
	for index := 0; index < radius%3; index++ {
		result[index]++
	}
	return result
}

func expandFilterBounds(bounds image.Rectangle, x, y int) (image.Rectangle, error) {
	if bounds.Empty() {
		return image.Rectangle{}, nil
	}
	maxPlatform := platformMaxInt()
	minPlatform := -maxPlatform - 1
	if int64(bounds.Min.X) < minPlatform+int64(x) || int64(bounds.Min.Y) < minPlatform+int64(y) ||
		int64(bounds.Max.X) > maxPlatform-int64(x) || int64(bounds.Max.Y) > maxPlatform-int64(y) {
		return image.Rectangle{}, fmt.Errorf("filter bounds exceed the platform integer domain")
	}
	minX, minY := int64(bounds.Min.X)-int64(x), int64(bounds.Min.Y)-int64(y)
	maxX, maxY := int64(bounds.Max.X)+int64(x), int64(bounds.Max.Y)+int64(y)
	if uint64(maxX)-uint64(minX) > uint64(maxPlatform) || uint64(maxY)-uint64(minY) > uint64(maxPlatform) {
		return image.Rectangle{}, fmt.Errorf("filter bounds exceed the platform integer domain")
	}
	return image.Rect(int(minX), int(minY), int(maxX), int(maxY)), nil
}

func translatedFilterBoundsUnbounded(bounds image.Rectangle, offsetX, offsetY float64) (image.Rectangle, error) {
	if bounds.Empty() {
		return image.Rectangle{}, nil
	}
	minX := math.Floor(float64(bounds.Min.X) + offsetX)
	minY := math.Floor(float64(bounds.Min.Y) + offsetY)
	maxX := math.Ceil(float64(bounds.Max.X) + offsetX)
	maxY := math.Ceil(float64(bounds.Max.Y) + offsetY)
	maxPlatform := platformMaxInt()
	minPlatform := -maxPlatform - 1
	maxCoordinate := float64(maxPlatform)
	if maxPlatform > 1<<53 {
		maxCoordinate = math.Nextafter(maxCoordinate, 0)
	}
	if !finite(minX) || !finite(minY) || !finite(maxX) || !finite(maxY) ||
		minX < float64(minPlatform) || minY < float64(minPlatform) ||
		maxX > maxCoordinate || maxY > maxCoordinate || maxX <= minX || maxY <= minY {
		return image.Rectangle{}, fmt.Errorf("translated filter bounds exceed the platform integer domain")
	}
	result := image.Rect(int(minX), int(minY), int(maxX), int(maxY))
	if uint64(int64(result.Max.X))-uint64(int64(result.Min.X)) > uint64(maxPlatform) ||
		uint64(int64(result.Max.Y))-uint64(int64(result.Min.Y)) > uint64(maxPlatform) {
		return image.Rectangle{}, fmt.Errorf("translated filter bounds exceed the platform integer domain")
	}
	return result, nil
}

func unionFilterBounds(left, right image.Rectangle) (image.Rectangle, error) {
	result := unionRect(left, right)
	if result.Empty() {
		return result, nil
	}
	maxPlatform := uint64(platformMaxInt())
	if uint64(int64(result.Max.X))-uint64(int64(result.Min.X)) > maxPlatform ||
		uint64(int64(result.Max.Y))-uint64(int64(result.Min.Y)) > maxPlatform {
		return image.Rectangle{}, fmt.Errorf("filter bounds exceed the platform integer domain")
	}
	return result, nil
}

type ownedRGBA struct {
	image       *image.RGBA
	reservation int64
	scratch     *rasterScratch
}

func reserveRGBA(scratch *rasterScratch, bounds image.Rectangle, purpose string) (*ownedRGBA, error) {
	reservation, err := scratch.offscreen.reserve(bounds, 4, purpose)
	if err != nil {
		return nil, err
	}
	return &ownedRGBA{image: image.NewRGBA(bounds), reservation: reservation, scratch: scratch}, nil
}

func (layer *ownedRGBA) release() {
	if layer == nil || layer.reservation == 0 {
		return
	}
	layer.scratch.offscreen.release(layer.reservation)
	layer.reservation = 0
	layer.image = nil
}

type ownedAlpha struct {
	image       *image.Alpha
	reservation int64
	scratch     *rasterScratch
}

func reserveAlpha(scratch *rasterScratch, bounds image.Rectangle, purpose string) (*ownedAlpha, error) {
	reservation, err := scratch.offscreen.reserve(bounds, 1, purpose)
	if err != nil {
		return nil, err
	}
	return &ownedAlpha{image: image.NewAlpha(bounds), reservation: reservation, scratch: scratch}, nil
}

func (layer *ownedAlpha) release() {
	if layer == nil || layer.reservation == 0 {
		return
	}
	layer.scratch.offscreen.release(layer.reservation)
	layer.reservation = 0
	layer.image = nil
}

func applyPreparedFilter(ctx context.Context, input *ownedRGBA, filter preparedFilter, scratch *rasterScratch) (*ownedRGBA, error) {
	switch filter.kind {
	case preparedGaussianBlur:
		return applyGaussianFilter(ctx, input, filter, scratch)
	case preparedDropShadow:
		return applyDropShadowFilter(ctx, input, filter, scratch)
	default:
		input.release()
		return nil, fmt.Errorf("d2raster: internal unknown prepared filter %d", filter.kind)
	}
}

func applyGaussianFilter(ctx context.Context, input *ownedRGBA, filter preparedFilter, scratch *rasterScratch) (*ownedRGBA, error) {
	current := input
	for index, pass := range filter.passes {
		output, err := reserveRGBA(scratch, pass.bounds, "Gaussian blur pass")
		if err != nil {
			current.release()
			return nil, err
		}
		if err := boxBlurRGBA(ctx, output.image, current.image, pass); err != nil {
			output.release()
			current.release()
			return nil, fmt.Errorf("d2raster: Gaussian blur pass %d: %w", index, err)
		}
		current.release()
		current = output
	}
	return current, nil
}

func applyDropShadowFilter(ctx context.Context, input *ownedRGBA, filter preparedFilter, scratch *rasterScratch) (*ownedRGBA, error) {
	var alpha *ownedAlpha
	for index, pass := range filter.passes {
		next, err := reserveAlpha(scratch, pass.bounds, "drop-shadow blur pass")
		if err != nil {
			alpha.release()
			input.release()
			return nil, err
		}
		if alpha == nil {
			err = boxBlurAlphaFromRGBA(ctx, next.image, input.image, pass)
		} else {
			err = boxBlurAlpha(ctx, next.image, alpha.image, pass)
		}
		if err != nil {
			next.release()
			alpha.release()
			input.release()
			return nil, fmt.Errorf("d2raster: drop-shadow blur pass %d: %w", index, err)
		}
		alpha.release()
		alpha = next
	}

	output, err := reserveRGBA(scratch, filter.output, "drop-shadow output layer")
	if err != nil {
		alpha.release()
		input.release()
		return nil, err
	}
	if err := paintDropShadow(ctx, output.image, input.image, alphaImage(alpha), filter.offsetX, filter.offsetY, filter.shadowColor); err != nil {
		output.release()
		alpha.release()
		input.release()
		return nil, err
	}
	if err := compositeLayerOver(ctx, output.image, input.image, 1); err != nil {
		output.release()
		alpha.release()
		input.release()
		return nil, err
	}
	alpha.release()
	input.release()
	return output, nil
}

func alphaImage(alpha *ownedAlpha) *image.Alpha {
	if alpha == nil {
		return nil
	}
	return alpha.image
}

func boxBlurRGBA(ctx context.Context, destination, source *image.RGBA, pass blurPass) error {
	window := int64(pass.radius)*2 + 1
	destinationBounds, sourceBounds := destination.Bounds(), source.Bounds()
	if pass.axis == blurHorizontal {
		for y := destinationBounds.Min.Y; y < destinationBounds.Max.Y; y++ {
			if (y-destinationBounds.Min.Y)&15 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			var sums [4]int64
			x0 := destinationBounds.Min.X
			sampleMin := max(int64(x0)-int64(pass.radius), int64(sourceBounds.Min.X))
			sampleMax := min(int64(x0)+int64(pass.radius), int64(sourceBounds.Max.X)-1)
			for sampleX := sampleMin; sampleX <= sampleMax; sampleX++ {
				if y >= sourceBounds.Min.Y && y < sourceBounds.Max.Y {
					offset := source.PixOffset(int(sampleX), y)
					for channel := range 4 {
						sums[channel] += int64(source.Pix[offset+channel])
					}
				}
			}
			for x := destinationBounds.Min.X; x < destinationBounds.Max.X; x++ {
				if (x-destinationBounds.Min.X)&255 == 0 {
					if err := ctx.Err(); err != nil {
						return err
					}
				}
				offset := destination.PixOffset(x, y)
				for channel := range 4 {
					destination.Pix[offset+channel] = uint8((sums[channel] + window/2) / window)
				}
				removeX, addX := int64(x)-int64(pass.radius), int64(x)+int64(pass.radius)+1
				if y >= sourceBounds.Min.Y && y < sourceBounds.Max.Y && removeX >= int64(sourceBounds.Min.X) && removeX < int64(sourceBounds.Max.X) {
					sourceOffset := source.PixOffset(int(removeX), y)
					for channel := range 4 {
						sums[channel] -= int64(source.Pix[sourceOffset+channel])
					}
				}
				if y >= sourceBounds.Min.Y && y < sourceBounds.Max.Y && addX >= int64(sourceBounds.Min.X) && addX < int64(sourceBounds.Max.X) {
					sourceOffset := source.PixOffset(int(addX), y)
					for channel := range 4 {
						sums[channel] += int64(source.Pix[sourceOffset+channel])
					}
				}
			}
		}
		return ctx.Err()
	}

	for x := destinationBounds.Min.X; x < destinationBounds.Max.X; x++ {
		if (x-destinationBounds.Min.X)&31 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		var sums [4]int64
		y0 := destinationBounds.Min.Y
		sampleMin := max(int64(y0)-int64(pass.radius), int64(sourceBounds.Min.Y))
		sampleMax := min(int64(y0)+int64(pass.radius), int64(sourceBounds.Max.Y)-1)
		for sampleY := sampleMin; sampleY <= sampleMax; sampleY++ {
			if x >= sourceBounds.Min.X && x < sourceBounds.Max.X {
				offset := source.PixOffset(x, int(sampleY))
				for channel := range 4 {
					sums[channel] += int64(source.Pix[offset+channel])
				}
			}
		}
		for y := destinationBounds.Min.Y; y < destinationBounds.Max.Y; y++ {
			if (y-destinationBounds.Min.Y)&255 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			offset := destination.PixOffset(x, y)
			for channel := range 4 {
				destination.Pix[offset+channel] = uint8((sums[channel] + window/2) / window)
			}
			removeY, addY := int64(y)-int64(pass.radius), int64(y)+int64(pass.radius)+1
			if x >= sourceBounds.Min.X && x < sourceBounds.Max.X && removeY >= int64(sourceBounds.Min.Y) && removeY < int64(sourceBounds.Max.Y) {
				sourceOffset := source.PixOffset(x, int(removeY))
				for channel := range 4 {
					sums[channel] -= int64(source.Pix[sourceOffset+channel])
				}
			}
			if x >= sourceBounds.Min.X && x < sourceBounds.Max.X && addY >= int64(sourceBounds.Min.Y) && addY < int64(sourceBounds.Max.Y) {
				sourceOffset := source.PixOffset(x, int(addY))
				for channel := range 4 {
					sums[channel] += int64(source.Pix[sourceOffset+channel])
				}
			}
		}
	}
	return ctx.Err()
}

func boxBlurAlphaFromRGBA(ctx context.Context, destination *image.Alpha, source *image.RGBA, pass blurPass) error {
	return boxBlurAlphaValues(ctx, destination, source.Bounds(), func(x, y int) uint8 {
		return source.Pix[source.PixOffset(x, y)+3]
	}, pass)
}

func boxBlurAlpha(ctx context.Context, destination, source *image.Alpha, pass blurPass) error {
	return boxBlurAlphaValues(ctx, destination, source.Bounds(), func(x, y int) uint8 {
		return source.Pix[source.PixOffset(x, y)]
	}, pass)
}

func boxBlurAlphaValues(ctx context.Context, destination *image.Alpha, sourceBounds image.Rectangle, sample func(int, int) uint8, pass blurPass) error {
	window := int64(pass.radius)*2 + 1
	destinationBounds := destination.Bounds()
	if pass.axis == blurHorizontal {
		for y := destinationBounds.Min.Y; y < destinationBounds.Max.Y; y++ {
			if (y-destinationBounds.Min.Y)&15 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			sum := int64(0)
			x0 := destinationBounds.Min.X
			if y >= sourceBounds.Min.Y && y < sourceBounds.Max.Y {
				sampleMin := max(int64(x0)-int64(pass.radius), int64(sourceBounds.Min.X))
				sampleMax := min(int64(x0)+int64(pass.radius), int64(sourceBounds.Max.X)-1)
				for sampleX := sampleMin; sampleX <= sampleMax; sampleX++ {
					sum += int64(sample(int(sampleX), y))
				}
			}
			for x := destinationBounds.Min.X; x < destinationBounds.Max.X; x++ {
				if (x-destinationBounds.Min.X)&255 == 0 {
					if err := ctx.Err(); err != nil {
						return err
					}
				}
				destination.Pix[destination.PixOffset(x, y)] = uint8((sum + window/2) / window)
				removeX, addX := int64(x)-int64(pass.radius), int64(x)+int64(pass.radius)+1
				if y >= sourceBounds.Min.Y && y < sourceBounds.Max.Y && removeX >= int64(sourceBounds.Min.X) && removeX < int64(sourceBounds.Max.X) {
					sum -= int64(sample(int(removeX), y))
				}
				if y >= sourceBounds.Min.Y && y < sourceBounds.Max.Y && addX >= int64(sourceBounds.Min.X) && addX < int64(sourceBounds.Max.X) {
					sum += int64(sample(int(addX), y))
				}
			}
		}
		return ctx.Err()
	}

	for x := destinationBounds.Min.X; x < destinationBounds.Max.X; x++ {
		if (x-destinationBounds.Min.X)&31 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		sum := int64(0)
		y0 := destinationBounds.Min.Y
		if x >= sourceBounds.Min.X && x < sourceBounds.Max.X {
			sampleMin := max(int64(y0)-int64(pass.radius), int64(sourceBounds.Min.Y))
			sampleMax := min(int64(y0)+int64(pass.radius), int64(sourceBounds.Max.Y)-1)
			for sampleY := sampleMin; sampleY <= sampleMax; sampleY++ {
				sum += int64(sample(x, int(sampleY)))
			}
		}
		for y := destinationBounds.Min.Y; y < destinationBounds.Max.Y; y++ {
			if (y-destinationBounds.Min.Y)&255 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			destination.Pix[destination.PixOffset(x, y)] = uint8((sum + window/2) / window)
			removeY, addY := int64(y)-int64(pass.radius), int64(y)+int64(pass.radius)+1
			if x >= sourceBounds.Min.X && x < sourceBounds.Max.X && removeY >= int64(sourceBounds.Min.Y) && removeY < int64(sourceBounds.Max.Y) {
				sum -= int64(sample(x, int(removeY)))
			}
			if x >= sourceBounds.Min.X && x < sourceBounds.Max.X && addY >= int64(sourceBounds.Min.Y) && addY < int64(sourceBounds.Max.Y) {
				sum += int64(sample(x, int(addY)))
			}
		}
	}
	return ctx.Err()
}

func paintDropShadow(ctx context.Context, destination, source *image.RGBA, blurred *image.Alpha, offsetX, offsetY float64, shadow color.NRGBA) error {
	for y := destination.Bounds().Min.Y; y < destination.Bounds().Max.Y; y++ {
		if (y-destination.Bounds().Min.Y)&31 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		for x := destination.Bounds().Min.X; x < destination.Bounds().Max.X; x++ {
			if (x-destination.Bounds().Min.X)&1023 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			alpha := sampleShadowAlpha(source, blurred, float64(x)-offsetX, float64(y)-offsetY)
			if alpha == 0 {
				continue
			}
			shadowAlpha := uint8((uint32(alpha)*uint32(shadow.A) + 127) / 255)
			offset := destination.PixOffset(x, y)
			destination.Pix[offset] = uint8((uint32(shadow.R)*uint32(shadowAlpha) + 127) / 255)
			destination.Pix[offset+1] = uint8((uint32(shadow.G)*uint32(shadowAlpha) + 127) / 255)
			destination.Pix[offset+2] = uint8((uint32(shadow.B)*uint32(shadowAlpha) + 127) / 255)
			destination.Pix[offset+3] = shadowAlpha
		}
	}
	return ctx.Err()
}

func sampleShadowAlpha(source *image.RGBA, blurred *image.Alpha, x, y float64) uint8 {
	var bounds image.Rectangle
	var sample func(int, int) uint8
	if blurred == nil {
		bounds = source.Bounds()
		sample = func(px, py int) uint8 {
			if px < bounds.Min.X || px >= bounds.Max.X || py < bounds.Min.Y || py >= bounds.Max.Y {
				return 0
			}
			return source.Pix[source.PixOffset(px, py)+3]
		}
	} else {
		bounds = blurred.Bounds()
		sample = func(px, py int) uint8 {
			if px < bounds.Min.X || px >= bounds.Max.X || py < bounds.Min.Y || py >= bounds.Max.Y {
				return 0
			}
			return blurred.Pix[blurred.PixOffset(px, py)]
		}
	}
	// Reject non-contributing coordinates before float-to-int conversion. This
	// also makes finite but enormous offsets deterministic across architectures.
	if !finite(x) || !finite(y) ||
		x < float64(bounds.Min.X)-1 || x >= float64(bounds.Max.X) ||
		y < float64(bounds.Min.Y)-1 || y >= float64(bounds.Max.Y) {
		return 0
	}
	x0, y0 := int(math.Floor(x)), int(math.Floor(y))
	fx, fy := x-float64(x0), y-float64(y0)
	if fx == 0 && fy == 0 {
		return sample(x0, y0)
	}
	a00 := float64(sample(x0, y0))
	a10 := float64(sample(x0+1, y0))
	a01 := float64(sample(x0, y0+1))
	a11 := float64(sample(x0+1, y0+1))
	top := a00 + (a10-a00)*fx
	bottom := a01 + (a11-a01)*fx
	return roundedByte(top + (bottom-top)*fy)
}

func planFilterResources(filters []preparedFilter, input image.Rectangle) (peak, finalBytes int64, err error) {
	currentBytes, err := pixelStorageBytes(input, 4)
	if err != nil {
		return 0, 0, fmt.Errorf("d2raster: filter input layer: %w", err)
	}
	peak = currentBytes
	for _, filter := range filters {
		switch filter.kind {
		case preparedGaussianBlur:
			for _, pass := range filter.passes {
				nextBytes, err := pixelStorageBytes(pass.bounds, 4)
				if err != nil {
					return 0, 0, fmt.Errorf("d2raster: Gaussian blur pass: %w", err)
				}
				live, ok := checkedAdd(currentBytes, nextBytes)
				if !ok {
					return 0, 0, fmt.Errorf("d2raster: peak offscreen pixel storage exceeds the int64 domain")
				}
				peak = maxInt64(peak, live)
				currentBytes = nextBytes
			}
		case preparedDropShadow:
			alphaBytes := int64(0)
			for _, pass := range filter.passes {
				nextBytes, err := pixelStorageBytes(pass.bounds, 1)
				if err != nil {
					return 0, 0, fmt.Errorf("d2raster: drop-shadow blur pass: %w", err)
				}
				live, ok := checkedAdd(currentBytes, alphaBytes)
				if ok {
					live, ok = checkedAdd(live, nextBytes)
				}
				if !ok {
					return 0, 0, fmt.Errorf("d2raster: peak offscreen pixel storage exceeds the int64 domain")
				}
				peak = maxInt64(peak, live)
				alphaBytes = nextBytes
			}
			outputBytes, err := pixelStorageBytes(filter.output, 4)
			if err != nil {
				return 0, 0, fmt.Errorf("d2raster: drop-shadow output layer: %w", err)
			}
			live, ok := checkedAdd(currentBytes, alphaBytes)
			if ok {
				live, ok = checkedAdd(live, outputBytes)
			}
			if !ok {
				return 0, 0, fmt.Errorf("d2raster: peak offscreen pixel storage exceeds the int64 domain")
			}
			peak = maxInt64(peak, live)
			currentBytes = outputBytes
		default:
			return 0, 0, fmt.Errorf("d2raster: internal unknown prepared filter %d", filter.kind)
		}
	}
	return peak, currentBytes, nil
}
