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

func reserveRGBA(scratch *rasterScratch, bounds image.Rectangle, purpose string) (ownedRGBA, error) {
	buffer, reservation, err := scratch.offscreen.newRGBA(bounds, purpose)
	if err != nil {
		return ownedRGBA{}, err
	}
	return ownedRGBA{image: buffer, reservation: reservation, scratch: scratch}, nil
}

func (layer *ownedRGBA) release() {
	if layer == nil || layer.reservation == 0 {
		return
	}
	layer.scratch.offscreen.recycleRGBA(layer.image, layer.reservation)
	layer.reservation = 0
	layer.image = nil
}

type ownedAlpha struct {
	image       *image.Alpha
	reservation int64
	scratch     *rasterScratch
}

func reserveAlpha(scratch *rasterScratch, bounds image.Rectangle, purpose string) (ownedAlpha, error) {
	buffer, reservation, err := scratch.offscreen.newAlpha(bounds, purpose)
	if err != nil {
		return ownedAlpha{}, err
	}
	return ownedAlpha{image: buffer, reservation: reservation, scratch: scratch}, nil
}

func (layer *ownedAlpha) release() {
	if layer == nil || layer.reservation == 0 {
		return
	}
	layer.scratch.offscreen.recycleAlpha(layer.image, layer.reservation)
	layer.reservation = 0
	layer.image = nil
}

func applyPreparedFilter(ctx context.Context, input *ownedRGBA, filter preparedFilter, scratch *rasterScratch) error {
	switch filter.kind {
	case preparedGaussianBlur:
		return applyGaussianFilter(ctx, input, filter, scratch)
	case preparedDropShadow:
		return applyDropShadowFilter(ctx, input, filter, scratch)
	default:
		input.release()
		return fmt.Errorf("d2raster: internal unknown prepared filter %d", filter.kind)
	}
}

func applyGaussianFilter(ctx context.Context, current *ownedRGBA, filter preparedFilter, scratch *rasterScratch) error {
	for index, pass := range filter.passes {
		output, err := reserveRGBA(scratch, pass.bounds, "Gaussian blur pass")
		if err != nil {
			current.release()
			return err
		}
		if err := boxBlurRGBA(ctx, output.image, current.image, pass); err != nil {
			output.release()
			current.release()
			return fmt.Errorf("d2raster: Gaussian blur pass %d: %w", index, err)
		}
		current.release()
		*current = output
	}
	return nil
}

func applyDropShadowFilter(ctx context.Context, input *ownedRGBA, filter preparedFilter, scratch *rasterScratch) error {
	var alpha ownedAlpha
	for index, pass := range filter.passes {
		next, err := reserveAlpha(scratch, pass.bounds, "drop-shadow blur pass")
		if err != nil {
			alpha.release()
			input.release()
			return err
		}
		if alpha.image == nil {
			err = boxBlurAlphaFromRGBA(ctx, next.image, input.image, pass)
		} else {
			err = boxBlurAlpha(ctx, next.image, alpha.image, pass)
		}
		if err != nil {
			next.release()
			alpha.release()
			input.release()
			return fmt.Errorf("d2raster: drop-shadow blur pass %d: %w", index, err)
		}
		alpha.release()
		alpha = next
	}

	output, err := reserveRGBA(scratch, filter.output, "drop-shadow output layer")
	if err != nil {
		alpha.release()
		input.release()
		return err
	}
	if err := paintDropShadow(ctx, output.image, input.image, alphaImage(&alpha), filter.offsetX, filter.offsetY, filter.shadowColor); err != nil {
		output.release()
		alpha.release()
		input.release()
		return err
	}
	if err := compositeLayerOver(ctx, output.image, input.image, 1); err != nil {
		output.release()
		alpha.release()
		input.release()
		return err
	}
	alpha.release()
	input.release()
	*input = output
	return nil
}

func alphaImage(alpha *ownedAlpha) *image.Alpha {
	if alpha == nil {
		return nil
	}
	return alpha.image
}

func boxBlurRGBA(ctx context.Context, destination, source *image.RGBA, pass blurPass) error {
	if expandedBlurPass(destination.Bounds(), source.Bounds(), pass) {
		return boxBlurRGBAExpanded(ctx, destination, source, pass)
	}
	return boxBlurRGBAGeneral(ctx, destination, source, pass)
}

// expandedBlurPass recognizes the layout produced by prepareBlurPasses. In
// that layout every row (horizontal pass) or column (vertical pass) has source
// pixels, and the moving window enters and leaves the source at fixed relative
// offsets. Keeping those facts out of the per-pixel loop materially reduces
// the cost of the six blur passes used by filters while the general kernel
// remains available for independently constructed images and tests.
func expandedBlurPass(destination, source image.Rectangle, pass blurPass) bool {
	if pass.radius <= 0 || destination.Empty() || source.Empty() {
		return false
	}
	radius := int64(pass.radius)
	switch pass.axis {
	case blurHorizontal:
		return destination.Min.Y == source.Min.Y && destination.Max.Y == source.Max.Y &&
			int64(destination.Min.X) == int64(source.Min.X)-radius &&
			int64(destination.Max.X) == int64(source.Max.X)+radius
	case blurVertical:
		return destination.Min.X == source.Min.X && destination.Max.X == source.Max.X &&
			int64(destination.Min.Y) == int64(source.Min.Y)-radius &&
			int64(destination.Max.Y) == int64(source.Max.Y)+radius
	default:
		return false
	}
}

func boxBlurRGBAExpanded(ctx context.Context, destination, source *image.RGBA, pass blurPass) error {
	window := int64(pass.radius)*2 + 1
	halfWindow := window / 2
	sourceBounds := source.Bounds()
	destinationBounds := destination.Bounds()
	twoRadius := pass.radius * 2
	if pass.axis == blurHorizontal {
		sourceWidth := sourceBounds.Dx()
		destinationWidth := destinationBounds.Dx()
		for row := 0; row < destinationBounds.Dy(); row++ {
			sourceOffset := source.PixOffset(sourceBounds.Min.X, sourceBounds.Min.Y+row)
			destinationOffset := destination.PixOffset(destinationBounds.Min.X, destinationBounds.Min.Y+row)
			var sums [4]int64
			sums[0] = int64(source.Pix[sourceOffset])
			sums[1] = int64(source.Pix[sourceOffset+1])
			sums[2] = int64(source.Pix[sourceOffset+2])
			sums[3] = int64(source.Pix[sourceOffset+3])
			for x := 0; x < destinationWidth; x++ {
				if x&255 == 0 {
					if err := ctx.Err(); err != nil {
						return err
					}
				}
				destination.Pix[destinationOffset] = uint8((sums[0] + halfWindow) / window)
				destination.Pix[destinationOffset+1] = uint8((sums[1] + halfWindow) / window)
				destination.Pix[destinationOffset+2] = uint8((sums[2] + halfWindow) / window)
				destination.Pix[destinationOffset+3] = uint8((sums[3] + halfWindow) / window)
				destinationOffset += 4

				if x >= twoRadius {
					remove := x - twoRadius
					offset := sourceOffset + remove*4
					sums[0] -= int64(source.Pix[offset])
					sums[1] -= int64(source.Pix[offset+1])
					sums[2] -= int64(source.Pix[offset+2])
					sums[3] -= int64(source.Pix[offset+3])
				}
				add := x + 1
				if add < sourceWidth {
					offset := sourceOffset + add*4
					sums[0] += int64(source.Pix[offset])
					sums[1] += int64(source.Pix[offset+1])
					sums[2] += int64(source.Pix[offset+2])
					sums[3] += int64(source.Pix[offset+3])
				}
			}
		}
		return ctx.Err()
	}

	sourceHeight := sourceBounds.Dy()
	destinationHeight := destinationBounds.Dy()
	for column := 0; column < destinationBounds.Dx(); column++ {
		sourceOffset := source.PixOffset(sourceBounds.Min.X+column, sourceBounds.Min.Y)
		destinationOffset := destination.PixOffset(destinationBounds.Min.X+column, destinationBounds.Min.Y)
		var sums [4]int64
		sums[0] = int64(source.Pix[sourceOffset])
		sums[1] = int64(source.Pix[sourceOffset+1])
		sums[2] = int64(source.Pix[sourceOffset+2])
		sums[3] = int64(source.Pix[sourceOffset+3])
		for y := 0; y < destinationHeight; y++ {
			if y&255 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			destination.Pix[destinationOffset] = uint8((sums[0] + halfWindow) / window)
			destination.Pix[destinationOffset+1] = uint8((sums[1] + halfWindow) / window)
			destination.Pix[destinationOffset+2] = uint8((sums[2] + halfWindow) / window)
			destination.Pix[destinationOffset+3] = uint8((sums[3] + halfWindow) / window)
			destinationOffset += destination.Stride

			if y >= twoRadius {
				remove := y - twoRadius
				offset := sourceOffset + remove*source.Stride
				sums[0] -= int64(source.Pix[offset])
				sums[1] -= int64(source.Pix[offset+1])
				sums[2] -= int64(source.Pix[offset+2])
				sums[3] -= int64(source.Pix[offset+3])
			}
			add := y + 1
			if add < sourceHeight {
				offset := sourceOffset + add*source.Stride
				sums[0] += int64(source.Pix[offset])
				sums[1] += int64(source.Pix[offset+1])
				sums[2] += int64(source.Pix[offset+2])
				sums[3] += int64(source.Pix[offset+3])
			}
		}
	}
	return ctx.Err()
}

func boxBlurRGBAGeneral(ctx context.Context, destination, source *image.RGBA, pass blurPass) error {
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
			sourceRow := -1
			if y >= sourceBounds.Min.Y && y < sourceBounds.Max.Y {
				sourceRow = (y - sourceBounds.Min.Y) * source.Stride
				for sampleX := sampleMin; sampleX <= sampleMax; sampleX++ {
					offset := sourceRow + (int(sampleX)-sourceBounds.Min.X)*4
					sums[0] += int64(source.Pix[offset])
					sums[1] += int64(source.Pix[offset+1])
					sums[2] += int64(source.Pix[offset+2])
					sums[3] += int64(source.Pix[offset+3])
				}
			}
			destinationOffset := (y - destinationBounds.Min.Y) * destination.Stride
			for x := destinationBounds.Min.X; x < destinationBounds.Max.X; x++ {
				if (x-destinationBounds.Min.X)&255 == 0 {
					if err := ctx.Err(); err != nil {
						return err
					}
				}
				offset := destinationOffset + (x-destinationBounds.Min.X)*4
				destination.Pix[offset] = uint8((sums[0] + window/2) / window)
				destination.Pix[offset+1] = uint8((sums[1] + window/2) / window)
				destination.Pix[offset+2] = uint8((sums[2] + window/2) / window)
				destination.Pix[offset+3] = uint8((sums[3] + window/2) / window)
				removeX, addX := int64(x)-int64(pass.radius), int64(x)+int64(pass.radius)+1
				if sourceRow >= 0 && removeX >= int64(sourceBounds.Min.X) && removeX < int64(sourceBounds.Max.X) {
					sourceOffset := sourceRow + (int(removeX)-sourceBounds.Min.X)*4
					sums[0] -= int64(source.Pix[sourceOffset])
					sums[1] -= int64(source.Pix[sourceOffset+1])
					sums[2] -= int64(source.Pix[sourceOffset+2])
					sums[3] -= int64(source.Pix[sourceOffset+3])
				}
				if sourceRow >= 0 && addX >= int64(sourceBounds.Min.X) && addX < int64(sourceBounds.Max.X) {
					sourceOffset := sourceRow + (int(addX)-sourceBounds.Min.X)*4
					sums[0] += int64(source.Pix[sourceOffset])
					sums[1] += int64(source.Pix[sourceOffset+1])
					sums[2] += int64(source.Pix[sourceOffset+2])
					sums[3] += int64(source.Pix[sourceOffset+3])
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
		sourceColumn := -1
		if x >= sourceBounds.Min.X && x < sourceBounds.Max.X {
			sourceColumn = (x - sourceBounds.Min.X) * 4
			for sampleY := sampleMin; sampleY <= sampleMax; sampleY++ {
				offset := (int(sampleY)-sourceBounds.Min.Y)*source.Stride + sourceColumn
				sums[0] += int64(source.Pix[offset])
				sums[1] += int64(source.Pix[offset+1])
				sums[2] += int64(source.Pix[offset+2])
				sums[3] += int64(source.Pix[offset+3])
			}
		}
		destinationColumn := (x - destinationBounds.Min.X) * 4
		for y := destinationBounds.Min.Y; y < destinationBounds.Max.Y; y++ {
			if (y-destinationBounds.Min.Y)&255 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			offset := (y-destinationBounds.Min.Y)*destination.Stride + destinationColumn
			destination.Pix[offset] = uint8((sums[0] + window/2) / window)
			destination.Pix[offset+1] = uint8((sums[1] + window/2) / window)
			destination.Pix[offset+2] = uint8((sums[2] + window/2) / window)
			destination.Pix[offset+3] = uint8((sums[3] + window/2) / window)
			removeY, addY := int64(y)-int64(pass.radius), int64(y)+int64(pass.radius)+1
			if sourceColumn >= 0 && removeY >= int64(sourceBounds.Min.Y) && removeY < int64(sourceBounds.Max.Y) {
				sourceOffset := (int(removeY)-sourceBounds.Min.Y)*source.Stride + sourceColumn
				sums[0] -= int64(source.Pix[sourceOffset])
				sums[1] -= int64(source.Pix[sourceOffset+1])
				sums[2] -= int64(source.Pix[sourceOffset+2])
				sums[3] -= int64(source.Pix[sourceOffset+3])
			}
			if sourceColumn >= 0 && addY >= int64(sourceBounds.Min.Y) && addY < int64(sourceBounds.Max.Y) {
				sourceOffset := (int(addY)-sourceBounds.Min.Y)*source.Stride + sourceColumn
				sums[0] += int64(source.Pix[sourceOffset])
				sums[1] += int64(source.Pix[sourceOffset+1])
				sums[2] += int64(source.Pix[sourceOffset+2])
				sums[3] += int64(source.Pix[sourceOffset+3])
			}
		}
	}
	return ctx.Err()
}

func boxBlurAlphaFromRGBA(ctx context.Context, destination *image.Alpha, source *image.RGBA, pass blurPass) error {
	return boxBlurAlphaPixels(ctx, destination, source.Pix, source.Stride, source.Bounds(), 4, 3, pass)
}

func boxBlurAlpha(ctx context.Context, destination, source *image.Alpha, pass blurPass) error {
	return boxBlurAlphaPixels(ctx, destination, source.Pix, source.Stride, source.Bounds(), 1, 0, pass)
}

func boxBlurAlphaPixels(ctx context.Context, destination *image.Alpha, sourcePixels []uint8, sourceStride int, sourceBounds image.Rectangle, sourceStep, sourceChannel int, pass blurPass) error {
	if expandedBlurPass(destination.Bounds(), sourceBounds, pass) {
		return boxBlurAlphaPixelsExpanded(ctx, destination, sourcePixels, sourceStride, sourceBounds, sourceStep, sourceChannel, pass)
	}
	return boxBlurAlphaPixelsGeneral(ctx, destination, sourcePixels, sourceStride, sourceBounds, sourceStep, sourceChannel, pass)
}

func boxBlurAlphaPixelsExpanded(ctx context.Context, destination *image.Alpha, sourcePixels []uint8, sourceStride int, sourceBounds image.Rectangle, sourceStep, sourceChannel int, pass blurPass) error {
	window := int64(pass.radius)*2 + 1
	halfWindow := window / 2
	destinationBounds := destination.Bounds()
	twoRadius := pass.radius * 2
	if pass.axis == blurHorizontal {
		sourceWidth := sourceBounds.Dx()
		destinationWidth := destinationBounds.Dx()
		for row := 0; row < destinationBounds.Dy(); row++ {
			sourceOffset := row*sourceStride + sourceChannel
			destinationOffset := row * destination.Stride
			sum := int64(sourcePixels[sourceOffset])
			for x := 0; x < destinationWidth; x++ {
				if x&255 == 0 {
					if err := ctx.Err(); err != nil {
						return err
					}
				}
				destination.Pix[destinationOffset] = uint8((sum + halfWindow) / window)
				destinationOffset++
				if x >= twoRadius {
					remove := x - twoRadius
					sum -= int64(sourcePixels[sourceOffset+remove*sourceStep])
				}
				add := x + 1
				if add < sourceWidth {
					sum += int64(sourcePixels[sourceOffset+add*sourceStep])
				}
			}
		}
		return ctx.Err()
	}

	sourceHeight := sourceBounds.Dy()
	destinationHeight := destinationBounds.Dy()
	for column := 0; column < destinationBounds.Dx(); column++ {
		sourceOffset := column*sourceStep + sourceChannel
		destinationOffset := column
		sum := int64(sourcePixels[sourceOffset])
		for y := 0; y < destinationHeight; y++ {
			if y&255 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			destination.Pix[destinationOffset] = uint8((sum + halfWindow) / window)
			destinationOffset += destination.Stride
			if y >= twoRadius {
				remove := y - twoRadius
				sum -= int64(sourcePixels[sourceOffset+remove*sourceStride])
			}
			add := y + 1
			if add < sourceHeight {
				sum += int64(sourcePixels[sourceOffset+add*sourceStride])
			}
		}
	}
	return ctx.Err()
}

func boxBlurAlphaPixelsGeneral(ctx context.Context, destination *image.Alpha, sourcePixels []uint8, sourceStride int, sourceBounds image.Rectangle, sourceStep, sourceChannel int, pass blurPass) error {
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
				sourceRow := (y-sourceBounds.Min.Y)*sourceStride + sourceChannel
				for sampleX := sampleMin; sampleX <= sampleMax; sampleX++ {
					sum += int64(sourcePixels[sourceRow+(int(sampleX)-sourceBounds.Min.X)*sourceStep])
				}
			}
			destinationOffset := (y - destinationBounds.Min.Y) * destination.Stride
			for x := destinationBounds.Min.X; x < destinationBounds.Max.X; x++ {
				if (x-destinationBounds.Min.X)&255 == 0 {
					if err := ctx.Err(); err != nil {
						return err
					}
				}
				destination.Pix[destinationOffset+x-destinationBounds.Min.X] = uint8((sum + window/2) / window)
				removeX, addX := int64(x)-int64(pass.radius), int64(x)+int64(pass.radius)+1
				if y >= sourceBounds.Min.Y && y < sourceBounds.Max.Y && removeX >= int64(sourceBounds.Min.X) && removeX < int64(sourceBounds.Max.X) {
					offset := (y-sourceBounds.Min.Y)*sourceStride + (int(removeX)-sourceBounds.Min.X)*sourceStep + sourceChannel
					sum -= int64(sourcePixels[offset])
				}
				if y >= sourceBounds.Min.Y && y < sourceBounds.Max.Y && addX >= int64(sourceBounds.Min.X) && addX < int64(sourceBounds.Max.X) {
					offset := (y-sourceBounds.Min.Y)*sourceStride + (int(addX)-sourceBounds.Min.X)*sourceStep + sourceChannel
					sum += int64(sourcePixels[offset])
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
			sourceColumn := (x-sourceBounds.Min.X)*sourceStep + sourceChannel
			for sampleY := sampleMin; sampleY <= sampleMax; sampleY++ {
				sum += int64(sourcePixels[(int(sampleY)-sourceBounds.Min.Y)*sourceStride+sourceColumn])
			}
		}
		destinationColumn := x - destinationBounds.Min.X
		for y := destinationBounds.Min.Y; y < destinationBounds.Max.Y; y++ {
			if (y-destinationBounds.Min.Y)&255 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			destination.Pix[(y-destinationBounds.Min.Y)*destination.Stride+destinationColumn] = uint8((sum + window/2) / window)
			removeY, addY := int64(y)-int64(pass.radius), int64(y)+int64(pass.radius)+1
			if x >= sourceBounds.Min.X && x < sourceBounds.Max.X && removeY >= int64(sourceBounds.Min.Y) && removeY < int64(sourceBounds.Max.Y) {
				offset := (int(removeY)-sourceBounds.Min.Y)*sourceStride + (x-sourceBounds.Min.X)*sourceStep + sourceChannel
				sum -= int64(sourcePixels[offset])
			}
			if x >= sourceBounds.Min.X && x < sourceBounds.Max.X && addY >= int64(sourceBounds.Min.Y) && addY < int64(sourceBounds.Max.Y) {
				offset := (int(addY)-sourceBounds.Min.Y)*sourceStride + (x-sourceBounds.Min.X)*sourceStep + sourceChannel
				sum += int64(sourcePixels[offset])
			}
		}
	}
	return ctx.Err()
}

func paintDropShadow(ctx context.Context, destination, source *image.RGBA, blurred *image.Alpha, offsetX, offsetY float64, shadow color.NRGBA) error {
	if finite(offsetX) && finite(offsetY) && offsetX == math.Trunc(offsetX) && offsetY == math.Trunc(offsetY) {
		return paintDropShadowInteger(ctx, destination, source, blurred, offsetX, offsetY, shadow)
	}
	for y := destination.Bounds().Min.Y; y < destination.Bounds().Max.Y; y++ {
		if (y-destination.Bounds().Min.Y)&31 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		sampleY := float64(y) - offsetY
		for x := destination.Bounds().Min.X; x < destination.Bounds().Max.X; x++ {
			if (x-destination.Bounds().Min.X)&1023 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			var alpha uint8
			if blurred == nil {
				alpha = sampleShadowAlphaRGBA(source, float64(x)-offsetX, sampleY)
			} else {
				alpha = sampleShadowAlphaImage(blurred, float64(x)-offsetX, sampleY)
			}
			if alpha == 0 {
				continue
			}
			pixel := premultipliedShadowPixel(shadow, alpha)
			offset := destination.PixOffset(x, y)
			destination.Pix[offset] = pixel[0]
			destination.Pix[offset+1] = pixel[1]
			destination.Pix[offset+2] = pixel[2]
			destination.Pix[offset+3] = pixel[3]
		}
	}
	return ctx.Err()
}

func paintDropShadowInteger(ctx context.Context, destination, source *image.RGBA, blurred *image.Alpha, offsetX, offsetY float64, shadow color.NRGBA) error {
	sourcePixels := source.Pix
	sourceStride := source.Stride
	sourceBounds := source.Bounds()
	sourceStep, sourceChannel := 4, 3
	if blurred != nil {
		sourcePixels = blurred.Pix
		sourceStride = blurred.Stride
		sourceBounds = blurred.Bounds()
		sourceStep, sourceChannel = 1, 0
	}
	// Computing translated integer bounds costs more than it saves for a single
	// destination pixel. Keep that degenerate case scalar while preserving the
	// same three cancellation checkpoints as the row kernel.
	destinationBounds := destination.Bounds()
	if destinationBounds.Dx() == 1 && destinationBounds.Dy() == 1 {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		x, y := destinationBounds.Min.X, destinationBounds.Min.Y
		sampleXFloat, sampleYFloat := float64(x)-offsetX, float64(y)-offsetY
		if sampleXFloat >= float64(sourceBounds.Min.X) && sampleXFloat < float64(sourceBounds.Max.X) &&
			sampleYFloat >= float64(sourceBounds.Min.Y) && sampleYFloat < float64(sourceBounds.Max.Y) {
			sampleX, sampleY := int(sampleXFloat), int(sampleYFloat)
			alpha := sourcePixels[(sampleY-sourceBounds.Min.Y)*sourceStride+(sampleX-sourceBounds.Min.X)*sourceStep+sourceChannel]
			if alpha != 0 {
				pixel := premultipliedShadowPixel(shadow, alpha)
				offset := destination.PixOffset(x, y)
				destination.Pix[offset] = pixel[0]
				destination.Pix[offset+1] = pixel[1]
				destination.Pix[offset+2] = pixel[2]
				destination.Pix[offset+3] = pixel[3]
			}
		}
		return ctx.Err()
	}
	shiftedSourceBounds, ok := exactIntegerTranslatedBounds(sourceBounds, offsetX, offsetY)
	if !ok || !rectangleWithinExactFloatIntegerDomain(destinationBounds) {
		return paintDropShadowIntegerGeneral(ctx, destination, sourcePixels, sourceStride, sourceBounds, sourceStep, sourceChannel, offsetX, offsetY, shadow)
	}
	for y := destinationBounds.Min.Y; y < destinationBounds.Max.Y; y++ {
		if (y-destinationBounds.Min.Y)&31 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		sampleYInBounds := y >= shiftedSourceBounds.Min.Y && y < shiftedSourceBounds.Max.Y
		sourceRow := 0
		if sampleYInBounds {
			sourceY := sourceBounds.Min.Y + y - shiftedSourceBounds.Min.Y
			sourceRow = (sourceY-sourceBounds.Min.Y)*sourceStride + sourceChannel
		}
		destinationOffset := destination.PixOffset(destinationBounds.Min.X, y)
		for x := destinationBounds.Min.X; x < destinationBounds.Max.X; x++ {
			if (x-destinationBounds.Min.X)&1023 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			if !sampleYInBounds || x < shiftedSourceBounds.Min.X || x >= shiftedSourceBounds.Max.X {
				destinationOffset += 4
				continue
			}
			sourceX := sourceBounds.Min.X + x - shiftedSourceBounds.Min.X
			alpha := sourcePixels[sourceRow+(sourceX-sourceBounds.Min.X)*sourceStep]
			if alpha != 0 {
				pixel := premultipliedShadowPixel(shadow, alpha)
				destination.Pix[destinationOffset] = pixel[0]
				destination.Pix[destinationOffset+1] = pixel[1]
				destination.Pix[destinationOffset+2] = pixel[2]
				destination.Pix[destinationOffset+3] = pixel[3]
			}
			destinationOffset += 4
		}
	}
	return ctx.Err()
}

func rectangleWithinExactFloatIntegerDomain(bounds image.Rectangle) bool {
	const maxExactInteger = int64(1) << 53
	return int64(bounds.Min.X) >= -maxExactInteger && int64(bounds.Min.X) <= maxExactInteger &&
		int64(bounds.Min.Y) >= -maxExactInteger && int64(bounds.Min.Y) <= maxExactInteger &&
		int64(bounds.Max.X) >= -maxExactInteger && int64(bounds.Max.X) <= maxExactInteger &&
		int64(bounds.Max.Y) >= -maxExactInteger && int64(bounds.Max.Y) <= maxExactInteger
}

func exactIntegerTranslatedBounds(bounds image.Rectangle, offsetX, offsetY float64) (image.Rectangle, bool) {
	// The general sampler converts each destination coordinate to float64 before
	// subtracting the offset. Outside float64's exact integer domain, adjacent
	// coordinates can alias even when translating the rectangle endpoints happens
	// to preserve its width. Only use the direct-index kernel when every operand
	// and translated endpoint is exactly representable, so it remains byte-for-byte
	// equivalent to that sampler.
	const maxExactInteger = int64(1) << 53
	if !rectangleWithinExactFloatIntegerDomain(bounds) ||
		offsetX < -float64(maxExactInteger) || offsetX > float64(maxExactInteger) ||
		offsetY < -float64(maxExactInteger) || offsetY > float64(maxExactInteger) {
		return image.Rectangle{}, false
	}
	maxPlatform := float64(platformMaxInt())
	if platformMaxInt() > 1<<53 {
		maxPlatform = math.Nextafter(maxPlatform, 0)
	}
	minPlatform := -maxPlatform - 1
	minX := float64(bounds.Min.X) + offsetX
	minY := float64(bounds.Min.Y) + offsetY
	maxX := float64(bounds.Max.X) + offsetX
	maxY := float64(bounds.Max.Y) + offsetY
	if minX < -float64(maxExactInteger) || minY < -float64(maxExactInteger) ||
		maxX > float64(maxExactInteger) || maxY > float64(maxExactInteger) ||
		minX < minPlatform || minY < minPlatform || maxX > maxPlatform || maxY > maxPlatform ||
		minX != math.Trunc(minX) || minY != math.Trunc(minY) || maxX != math.Trunc(maxX) || maxY != math.Trunc(maxY) {
		return image.Rectangle{}, false
	}
	translated := image.Rect(int(minX), int(minY), int(maxX), int(maxY))
	if translated.Dx() != bounds.Dx() || translated.Dy() != bounds.Dy() {
		return image.Rectangle{}, false
	}
	return translated, true
}

func paintDropShadowIntegerGeneral(ctx context.Context, destination *image.RGBA, sourcePixels []byte, sourceStride int, sourceBounds image.Rectangle, sourceStep, sourceChannel int, offsetX, offsetY float64, shadow color.NRGBA) error {
	for y := destination.Bounds().Min.Y; y < destination.Bounds().Max.Y; y++ {
		if (y-destination.Bounds().Min.Y)&31 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		sampleYFloat := float64(y) - offsetY
		sampleYInBounds := sampleYFloat >= float64(sourceBounds.Min.Y) && sampleYFloat < float64(sourceBounds.Max.Y)
		sampleY := 0
		if sampleYInBounds {
			sampleY = int(sampleYFloat)
		}
		for x := destination.Bounds().Min.X; x < destination.Bounds().Max.X; x++ {
			if (x-destination.Bounds().Min.X)&1023 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			if !sampleYInBounds {
				continue
			}
			sampleXFloat := float64(x) - offsetX
			if sampleXFloat < float64(sourceBounds.Min.X) || sampleXFloat >= float64(sourceBounds.Max.X) {
				continue
			}
			sampleX := int(sampleXFloat)
			alpha := sourcePixels[(sampleY-sourceBounds.Min.Y)*sourceStride+(sampleX-sourceBounds.Min.X)*sourceStep+sourceChannel]
			if alpha == 0 {
				continue
			}
			pixel := premultipliedShadowPixel(shadow, alpha)
			offset := destination.PixOffset(x, y)
			destination.Pix[offset] = pixel[0]
			destination.Pix[offset+1] = pixel[1]
			destination.Pix[offset+2] = pixel[2]
			destination.Pix[offset+3] = pixel[3]
		}
	}
	return ctx.Err()
}

func premultipliedShadowPixel(shadow color.NRGBA, alpha uint8) [4]uint8 {
	shadowAlpha := uint8((uint32(alpha)*uint32(shadow.A) + 127) / 255)
	return [4]uint8{
		uint8((uint32(shadow.R)*uint32(shadowAlpha) + 127) / 255),
		uint8((uint32(shadow.G)*uint32(shadowAlpha) + 127) / 255),
		uint8((uint32(shadow.B)*uint32(shadowAlpha) + 127) / 255),
		shadowAlpha,
	}
}

func sampleShadowAlphaRGBA(source *image.RGBA, x, y float64) uint8 {
	bounds := source.Bounds()
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
		if x0 < bounds.Min.X || x0 >= bounds.Max.X || y0 < bounds.Min.Y || y0 >= bounds.Max.Y {
			return 0
		}
		return source.Pix[source.PixOffset(x0, y0)+3]
	}
	if x0 >= bounds.Min.X && x0+1 < bounds.Max.X && y0 >= bounds.Min.Y && y0+1 < bounds.Max.Y {
		offset := source.PixOffset(x0, y0) + 3
		return interpolateShadowAlpha(
			source.Pix[offset], source.Pix[offset+4],
			source.Pix[offset+source.Stride], source.Pix[offset+source.Stride+4],
			fx, fy,
		)
	}
	return interpolateShadowAlpha(
		rgbaAlphaAt(source, x0, y0), rgbaAlphaAt(source, x0+1, y0),
		rgbaAlphaAt(source, x0, y0+1), rgbaAlphaAt(source, x0+1, y0+1),
		fx, fy,
	)
}

func sampleShadowAlphaImage(source *image.Alpha, x, y float64) uint8 {
	bounds := source.Bounds()
	// Keep the same pre-conversion guard as the RGBA path.
	if !finite(x) || !finite(y) ||
		x < float64(bounds.Min.X)-1 || x >= float64(bounds.Max.X) ||
		y < float64(bounds.Min.Y)-1 || y >= float64(bounds.Max.Y) {
		return 0
	}
	x0, y0 := int(math.Floor(x)), int(math.Floor(y))
	fx, fy := x-float64(x0), y-float64(y0)
	if fx == 0 && fy == 0 {
		if x0 < bounds.Min.X || x0 >= bounds.Max.X || y0 < bounds.Min.Y || y0 >= bounds.Max.Y {
			return 0
		}
		return source.Pix[source.PixOffset(x0, y0)]
	}
	if x0 >= bounds.Min.X && x0+1 < bounds.Max.X && y0 >= bounds.Min.Y && y0+1 < bounds.Max.Y {
		offset := source.PixOffset(x0, y0)
		return interpolateShadowAlpha(
			source.Pix[offset], source.Pix[offset+1],
			source.Pix[offset+source.Stride], source.Pix[offset+source.Stride+1],
			fx, fy,
		)
	}
	return interpolateShadowAlpha(
		alphaAt(source, x0, y0), alphaAt(source, x0+1, y0),
		alphaAt(source, x0, y0+1), alphaAt(source, x0+1, y0+1),
		fx, fy,
	)
}

func interpolateShadowAlpha(a00, a10, a01, a11 uint8, fx, fy float64) uint8 {
	top := float64(a00) + (float64(a10)-float64(a00))*fx
	bottom := float64(a01) + (float64(a11)-float64(a01))*fx
	return roundedByte(top + (bottom-top)*fy)
}

func rgbaAlphaAt(source *image.RGBA, x, y int) uint8 {
	if !image.Pt(x, y).In(source.Bounds()) {
		return 0
	}
	return source.Pix[source.PixOffset(x, y)+3]
}

func alphaAt(source *image.Alpha, x, y int) uint8 {
	if !image.Pt(x, y).In(source.Bounds()) {
		return 0
	}
	return source.Pix[source.PixOffset(x, y)]
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
