package d2raster

import (
	"context"
	"errors"
	"fmt"
	"image"
	"math"

	"github.com/d2lang/d2/d2renderers/d2raster/internal/scanline"
	"github.com/d2lang/d2/d2renderers/d2scene"
)

const evenOddSamplesPerAxis = 4

type renderResources struct {
	peakOffscreenBytes int64
	evenOddClipWork    int64
	scanlineWork       int64
	rasterizerBytes    int64
	rasterizerWidth    int
	rasterizerHeight   int
	rasterizerEdges    int
}

type rasterizerResourcePlanner struct {
	counter *scanline.Rasterizer
}

type offscreenBudget struct {
	limit int64
	live  int64
	peak  int64
}

func (b *offscreenBudget) reserve(bounds image.Rectangle, bytesPerPixel int64, purpose string) (int64, error) {
	bytes, err := pixelStorageBytes(bounds, bytesPerPixel)
	if err != nil {
		return 0, fmt.Errorf("d2raster: %s: %w", purpose, err)
	}
	return b.reserveBytes(bytes, purpose)
}

func (b *offscreenBudget) reserveBytes(bytes int64, purpose string) (int64, error) {
	if bytes > b.limit-b.live {
		return 0, fmt.Errorf(
			"d2raster: offscreen %s requires %d bytes with %d bytes already live, exceeding limit %d",
			purpose, bytes, b.live, b.limit,
		)
	}
	b.live += bytes
	if b.live > b.peak {
		b.peak = b.live
	}
	return bytes, nil
}

func (b *offscreenBudget) release(bytes int64) {
	b.live -= bytes
}

func pixelStorageBytes(bounds image.Rectangle, bytesPerPixel int64) (int64, error) {
	if bounds.Empty() {
		return 0, nil
	}
	pixels, ok := checkedMultiply(int64(bounds.Dx()), int64(bounds.Dy()))
	if !ok {
		return 0, fmt.Errorf("offscreen pixel storage exceeds the int64 domain")
	}
	bytes, ok := checkedMultiply(pixels, bytesPerPixel)
	if !ok || bytes > platformMaxInt() {
		return 0, fmt.Errorf("offscreen pixel storage exceeds the platform integer domain")
	}
	return bytes, nil
}

func platformMaxInt() int64 {
	return int64(int(^uint(0) >> 1))
}

func checkedMultiply(left, right int64) (int64, bool) {
	if left < 0 || right < 0 || left != 0 && right > math.MaxInt64/left {
		return 0, false
	}
	return left * right, true
}

func checkedAdd(left, right int64) (int64, bool) {
	if left < 0 || right < 0 || right > math.MaxInt64-left {
		return 0, false
	}
	return left + right, true
}

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

func addResourceWork(resources *renderResources, work int64) error {
	total, ok := checkedAdd(resources.evenOddClipWork, work)
	if !ok {
		return fmt.Errorf("d2raster: even-odd clip work exceeds the int64 domain")
	}
	resources.evenOddClipWork = total
	return nil
}

func addScanlineWork(resources *renderResources, work int64) error {
	total, ok := checkedAdd(resources.scanlineWork, work)
	if !ok {
		return fmt.Errorf("d2raster: scanline work exceeds the int64 domain")
	}
	resources.scanlineWork = total
	return nil
}

func planRenderResources(ctx context.Context, node *preparedNode, dst image.Rectangle, patterns []*preparedPatternTile, maxOffscreenBytes int64) (renderResources, error) {
	planner := &rasterizerResourcePlanner{
		counter: scanline.NewCounter(0, 0, scanline.MaxEdgesForBytes(maxOffscreenBytes)),
	}
	resources, err := planNodeResources(ctx, node, dst, planner)
	if err != nil {
		return renderResources{}, err
	}
	scenePeak := resources.peakOffscreenBytes
	livePatternBytes := int64(0)
	patternPhasePeak := int64(0)
	for _, pattern := range patterns {
		if err := ctx.Err(); err != nil {
			return renderResources{}, err
		}
		if pattern == nil {
			return renderResources{}, fmt.Errorf("d2raster: internal nil prepared pattern")
		}
		patternResources, err := planNodeResources(ctx, pattern.root, pattern.bounds, planner)
		if err != nil {
			return renderResources{}, err
		}
		duringTile, ok := checkedAdd(livePatternBytes, pattern.tileBytes)
		if !ok {
			return renderResources{}, fmt.Errorf("d2raster: pattern tile storage exceeds the int64 domain")
		}
		duringTile, ok = checkedAdd(duringTile, patternResources.peakOffscreenBytes)
		if !ok {
			return renderResources{}, fmt.Errorf("d2raster: peak offscreen pixel storage exceeds the int64 domain")
		}
		patternPhasePeak = maxInt64(patternPhasePeak, duringTile)
		mergeRasterizerRequirements(&resources, patternResources)
		if err := addScanlineWork(&resources, patternResources.scanlineWork); err != nil {
			return renderResources{}, err
		}
		if err := addResourceWork(&resources, patternResources.evenOddClipWork); err != nil {
			return renderResources{}, err
		}
		livePatternBytes, ok = checkedAdd(livePatternBytes, pattern.tileBytes)
		if !ok {
			return renderResources{}, fmt.Errorf("d2raster: pattern tile storage exceeds the int64 domain")
		}
	}
	scenePeak, ok := checkedAdd(scenePeak, livePatternBytes)
	if !ok {
		return renderResources{}, fmt.Errorf("d2raster: peak offscreen pixel storage exceeds the int64 domain")
	}
	resources.peakOffscreenBytes = maxInt64(patternPhasePeak, scenePeak)
	resources.peakOffscreenBytes, ok = checkedAdd(resources.peakOffscreenBytes, resources.rasterizerBytes)
	if !ok {
		return renderResources{}, fmt.Errorf("d2raster: peak offscreen pixel storage exceeds the int64 domain")
	}
	return resources, nil
}

// planNodeResources computes the exact peak live RGBA/Alpha backing storage
// for the prepared tree's render order. It runs during preflight, before the
// final canvas is allocated. Parent effect layers and mask targets remain live
// while descendants render, whereas sequential children reuse the same budget.
// Rasterizer capacities are collected separately because the scratch object
// retains them for the complete traversal.
func planNodeResources(ctx context.Context, node *preparedNode, dst image.Rectangle, planner *rasterizerResourcePlanner) (renderResources, error) {
	var resources renderResources
	if node == nil || node.opacity == 0 {
		return resources, nil
	}
	if err := ctx.Err(); err != nil {
		return resources, err
	}
	visibleBounds := node.bounds.Intersect(dst)
	if visibleBounds.Empty() {
		return resources, nil
	}

	usesFilters := len(node.filters) != 0
	usesEffectLayer := node.isolated || node.opacity < 1 || node.blend != d2scene.BlendNormal || usesFilters || node.clip != nil || node.mask != nil
	baseBytes := int64(0)
	finalLayerBytes := int64(0)
	paintBounds := dst
	if usesFilters {
		var err error
		baseBytes, err = pixelStorageBytes(node.contentBounds, 4)
		if err != nil {
			return resources, fmt.Errorf("d2raster: filter input layer: %w", err)
		}
		resources.peakOffscreenBytes, finalLayerBytes, err = planFilterResources(node.filters, node.contentBounds)
		if err != nil {
			return resources, err
		}
		paintBounds = node.contentBounds
	} else if usesEffectLayer {
		var err error
		baseBytes, err = pixelStorageBytes(visibleBounds, 4)
		if err != nil {
			return resources, fmt.Errorf("d2raster: effect layer: %w", err)
		}
		finalLayerBytes = baseBytes
		resources.peakOffscreenBytes = baseBytes
		paintBounds = visibleBounds
	}

	var primitiveResources renderResources
	if node.primitive != nil && !node.primitive.bounds.Intersect(paintBounds).Empty() {
		var err error
		primitiveResources, err = planPrimitiveResources(ctx, node.primitive, paintBounds, planner)
		if err != nil {
			return resources, err
		}
	}
	withPrimitive, ok := checkedAdd(baseBytes, primitiveResources.peakOffscreenBytes)
	if !ok {
		return resources, fmt.Errorf("d2raster: peak offscreen pixel storage exceeds the int64 domain")
	}
	resources.peakOffscreenBytes = maxInt64(resources.peakOffscreenBytes, withPrimitive)
	mergeRasterizerRequirements(&resources, primitiveResources)
	if err := addScanlineWork(&resources, primitiveResources.scanlineWork); err != nil {
		return resources, err
	}
	if err := addResourceWork(&resources, primitiveResources.evenOddClipWork); err != nil {
		return resources, err
	}

	for _, child := range node.children {
		if err := ctx.Err(); err != nil {
			return resources, err
		}
		childResources, err := planNodeResources(ctx, child, paintBounds, planner)
		if err != nil {
			return resources, err
		}
		withChild, ok := checkedAdd(baseBytes, childResources.peakOffscreenBytes)
		if !ok {
			return resources, fmt.Errorf("d2raster: peak offscreen pixel storage exceeds the int64 domain")
		}
		resources.peakOffscreenBytes = maxInt64(resources.peakOffscreenBytes, withChild)
		mergeRasterizerRequirements(&resources, childResources)
		if err := addScanlineWork(&resources, childResources.scanlineWork); err != nil {
			return resources, err
		}
		if err := addResourceWork(&resources, childResources.evenOddClipWork); err != nil {
			return resources, err
		}
	}

	if node.clip != nil {
		clipBytes, err := pixelStorageBytes(visibleBounds, 1)
		if err != nil {
			return resources, fmt.Errorf("d2raster: clip mask: %w", err)
		}
		withClip, ok := checkedAdd(finalLayerBytes, clipBytes)
		if !ok {
			return resources, fmt.Errorf("d2raster: peak offscreen pixel storage exceeds the int64 domain")
		}
		resources.peakOffscreenBytes = maxInt64(resources.peakOffscreenBytes, withClip)
		if node.clip.fillRule != d2scene.EvenOdd && !node.clip.bounds.Intersect(visibleBounds).Empty() {
			target := image.Rect(0, 0, visibleBounds.Dx(), visibleBounds.Dy())
			shifted := d2scene.Translate(-float64(visibleBounds.Min.X), -float64(visibleBounds.Min.Y))
			if err := planner.recordFill(ctx, &resources, target, node.clip.subpaths, shifted, "clip"); err != nil {
				return resources, err
			}
		}
		if node.clip.fillRule == d2scene.EvenOdd {
			work, err := evenOddMaskWork(visibleBounds, node.clip.edges)
			if err != nil {
				return resources, err
			}
			if err := addResourceWork(&resources, work); err != nil {
				return resources, err
			}
		}
	}

	if node.mask != nil {
		maskBytes, err := pixelStorageBytes(visibleBounds, 4)
		if err != nil {
			return resources, fmt.Errorf("d2raster: scene mask: %w", err)
		}
		maskResources, err := planNodeResources(ctx, node.mask.root, visibleBounds, planner)
		if err != nil {
			return resources, err
		}
		withMask, ok := checkedAdd(finalLayerBytes, maskBytes)
		if ok {
			withMask, ok = checkedAdd(withMask, maskResources.peakOffscreenBytes)
		}
		if !ok {
			return resources, fmt.Errorf("d2raster: peak offscreen pixel storage exceeds the int64 domain")
		}
		resources.peakOffscreenBytes = maxInt64(resources.peakOffscreenBytes, withMask)
		mergeRasterizerRequirements(&resources, maskResources)
		if err := addScanlineWork(&resources, maskResources.scanlineWork); err != nil {
			return resources, err
		}
		if err := addResourceWork(&resources, maskResources.evenOddClipWork); err != nil {
			return resources, err
		}
	}
	return resources, nil
}

func evenOddMaskWork(bounds image.Rectangle, edges int64) (int64, error) {
	if bounds.Empty() || edges == 0 {
		return 0, nil
	}
	pixels, ok := checkedMultiply(int64(bounds.Dx()), int64(bounds.Dy()))
	if !ok {
		return 0, fmt.Errorf("d2raster: even-odd clip work exceeds the int64 domain")
	}
	work, ok := checkedMultiply(pixels, evenOddSamplesPerAxis*evenOddSamplesPerAxis)
	if ok {
		work, ok = checkedMultiply(work, edges)
	}
	if !ok {
		return 0, fmt.Errorf("d2raster: even-odd clip work exceeds the int64 domain")
	}
	return work, nil
}

func planPrimitiveResources(ctx context.Context, primitive *preparedPrimitive, dst image.Rectangle, planner *rasterizerResourcePlanner) (renderResources, error) {
	var resources renderResources
	if primitive == nil {
		return resources, nil
	}
	if primitive.image != nil {
		// Decoded assets are retained once per document under their dedicated
		// preflight limit. Image sampling writes directly into dst without a
		// per-image layer or temporary pixel storage.
		return resources, nil
	}
	if primitive.vector != nil {
		return planNodeResources(ctx, primitive.vector, dst, planner)
	}
	if primitive.fill != nil {
		bounds := subpathPixelBounds(primitive.subpaths, primitive.transform, 0, dst)
		if primitive.fillRule == d2scene.EvenOdd {
			bytes, err := pixelStorageBytes(bounds, 1)
			if err != nil {
				return resources, fmt.Errorf("d2raster: even-odd fill mask: %w", err)
			}
			resources.peakOffscreenBytes = bytes
			work, err := evenOddMaskWork(bounds, primitive.evenOddEdges)
			if err != nil {
				return resources, err
			}
			if err := addResourceWork(&resources, work); err != nil {
				return resources, err
			}
		} else if primitive.fill.kind == preparedSolidPaint {
			shifted := d2scene.Translate(-float64(dst.Min.X), -float64(dst.Min.Y)).Mul(primitive.transform)
			if err := planner.recordFill(ctx, &resources, dst, primitive.subpaths, shifted, "solid fill"); err != nil {
				return resources, err
			}
		} else {
			bytes, err := pixelStorageBytes(bounds, 1)
			if err != nil {
				return resources, fmt.Errorf("d2raster: gradient fill mask: %w", err)
			}
			resources.peakOffscreenBytes = bytes
			if !bounds.Empty() {
				target := image.Rect(0, 0, bounds.Dx(), bounds.Dy())
				shifted := d2scene.Translate(-float64(bounds.Min.X), -float64(bounds.Min.Y)).Mul(primitive.transform)
				if err := planner.recordFill(ctx, &resources, target, primitive.subpaths, shifted, "gradient fill"); err != nil {
					return resources, err
				}
			}
		}
	}
	if primitive.stroke != nil {
		if primitive.stroke.paint.kind == preparedSolidPaint {
			shifted := d2scene.Translate(-float64(dst.Min.X), -float64(dst.Min.Y)).Mul(primitive.transform)
			if err := planner.recordStroke(ctx, &resources, dst, primitive.strokeRuns, shifted, primitive.stroke, "solid stroke"); err != nil {
				return resources, err
			}
		} else {
			bounds := paintedStrokePixelBounds(primitive.strokeRuns, primitive.transform, primitive.stroke, dst)
			bytes, err := pixelStorageBytes(bounds, 1)
			if err != nil {
				return resources, fmt.Errorf("d2raster: gradient stroke mask: %w", err)
			}
			resources.peakOffscreenBytes = maxInt64(resources.peakOffscreenBytes, bytes)
			if !bounds.Empty() {
				target := image.Rect(0, 0, bounds.Dx(), bounds.Dy())
				shifted := d2scene.Translate(-float64(bounds.Min.X), -float64(bounds.Min.Y)).Mul(primitive.transform)
				if err := planner.recordStroke(ctx, &resources, target, primitive.strokeRuns, shifted, primitive.stroke, "gradient stroke"); err != nil {
					return resources, err
				}
			}
		}
	}
	return resources, nil
}

func mergeRasterizerRequirements(destination *renderResources, source renderResources) {
	destination.rasterizerWidth = max(destination.rasterizerWidth, source.rasterizerWidth)
	destination.rasterizerHeight = max(destination.rasterizerHeight, source.rasterizerHeight)
	destination.rasterizerEdges = max(destination.rasterizerEdges, source.rasterizerEdges)
	updateRasterizerBytes(destination)
}

func recordRasterizerRequirement(resources *renderResources, bounds image.Rectangle) {
	if bounds.Empty() {
		return
	}
	resources.rasterizerWidth = max(resources.rasterizerWidth, bounds.Dx())
	resources.rasterizerHeight = max(resources.rasterizerHeight, bounds.Dy())
	updateRasterizerBytes(resources)
}

func recordRasterizerEdges(resources *renderResources, edges int) {
	resources.rasterizerEdges = max(resources.rasterizerEdges, edges)
	updateRasterizerBytes(resources)
}

func updateRasterizerBytes(resources *renderResources) {
	bytes, ok := scanline.RetainedBytes(resources.rasterizerWidth, resources.rasterizerHeight, resources.rasterizerEdges)
	if !ok {
		resources.rasterizerBytes = math.MaxInt64
		return
	}
	resources.rasterizerBytes = bytes
}

func (p *rasterizerResourcePlanner) recordFill(ctx context.Context, resources *renderResources, target image.Rectangle, paths []subpath, transform d2scene.Matrix, purpose string) error {
	recordRasterizerRequirement(resources, target)
	p.counter.Reset(target.Dx(), target.Dy())
	for index, path := range paths {
		if index&255 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		addFillSubpath(p.counter, path, transform)
		if err := p.counter.Err(); err != nil {
			return rasterizerPlanError(purpose, err)
		}
	}
	edges := p.counter.EdgeCount()
	recordRasterizerEdges(resources, edges)
	work, ok := p.counter.WorkBound()
	if !ok {
		return fmt.Errorf("d2raster: %s scanline work exceeds the int64 domain", purpose)
	}
	return addScanlineWork(resources, work)
}

func (p *rasterizerResourcePlanner) recordStroke(ctx context.Context, resources *renderResources, target image.Rectangle, runs []strokeRun, transform d2scene.Matrix, stroke *preparedStroke, purpose string) error {
	recordRasterizerRequirement(resources, target)
	p.counter.Reset(target.Dx(), target.Dy())
	for _, run := range runs {
		if err := addStrokeRun(ctx, p.counter, run, transform, stroke); err != nil {
			return err
		}
		if err := p.counter.Err(); err != nil {
			return rasterizerPlanError(purpose, err)
		}
	}
	edges := p.counter.EdgeCount()
	recordRasterizerEdges(resources, edges)
	work, ok := p.counter.WorkBound()
	if !ok {
		return fmt.Errorf("d2raster: %s scanline work exceeds the int64 domain", purpose)
	}
	return addScanlineWork(resources, work)
}

func rasterizerPlanError(purpose string, err error) error {
	if errors.Is(err, scanline.ErrEdgeLimit) {
		return fmt.Errorf("d2raster: %s scanline edges exceed the offscreen storage limit: %w", purpose, err)
	}
	return fmt.Errorf("d2raster: %s geometry: %w", purpose, err)
}

func unionRect(left, right image.Rectangle) image.Rectangle {
	if left.Empty() {
		return right
	}
	if right.Empty() {
		return left
	}
	return left.Union(right)
}

// renderEffectNode preserves SVG group-effect ordering. The complete node
// subtree is painted into an ink-bounds-sized layer, then clipped, masked,
// opacity-scaled, and finally source-over composited into its parent.
func renderEffectNode(ctx context.Context, dst *image.RGBA, node *preparedNode, scratch *rasterScratch) error {
	if len(node.filters) != 0 {
		return renderFilteredEffectNode(ctx, dst, node, scratch)
	}
	bounds := node.bounds.Intersect(dst.Bounds())
	if bounds.Empty() {
		return nil
	}
	layerBytes, err := scratch.offscreen.reserve(bounds, 4, "effect layer")
	if err != nil {
		return err
	}
	defer scratch.offscreen.release(layerBytes)
	layer := image.NewRGBA(bounds)
	if node.primitive != nil && !node.primitive.bounds.Intersect(bounds).Empty() {
		if err := drawPrimitive(ctx, layer, node.primitive, scratch); err != nil {
			return err
		}
	}
	for _, child := range node.children {
		if err := renderNode(ctx, layer, child, scratch); err != nil {
			return err
		}
	}
	if node.clip != nil {
		if err := applyClip(ctx, layer, node.clip, scratch); err != nil {
			return err
		}
	}
	if node.mask != nil {
		if err := applyMask(ctx, layer, node.mask, scratch); err != nil {
			return err
		}
	}
	return compositeLayer(ctx, dst, layer, node.opacity, node.blend)
}

func renderFilteredEffectNode(ctx context.Context, dst *image.RGBA, node *preparedNode, scratch *rasterScratch) error {
	visibleBounds := node.bounds.Intersect(dst.Bounds())
	if visibleBounds.Empty() || node.contentBounds.Empty() {
		return nil
	}
	current, err := reserveRGBA(scratch, node.contentBounds, "filter input layer")
	if err != nil {
		return err
	}
	defer func() { current.release() }()
	if node.primitive != nil && !node.primitive.bounds.Intersect(node.contentBounds).Empty() {
		if err := drawPrimitive(ctx, current.image, node.primitive, scratch); err != nil {
			return err
		}
	}
	for _, child := range node.children {
		if err := renderNode(ctx, current.image, child, scratch); err != nil {
			return err
		}
	}
	for _, filter := range node.filters {
		next, err := applyPreparedFilter(ctx, current, filter, scratch)
		if err != nil {
			return err
		}
		current = next
	}
	layer := current.image
	if layer.Bounds() != visibleBounds {
		layer = layer.SubImage(visibleBounds).(*image.RGBA)
	}
	if node.clip != nil {
		if err := applyClip(ctx, layer, node.clip, scratch); err != nil {
			return err
		}
	}
	if node.mask != nil {
		if err := applyMask(ctx, layer, node.mask, scratch); err != nil {
			return err
		}
	}
	return compositeLayer(ctx, dst, layer, node.opacity, node.blend)
}

func applyClip(ctx context.Context, layer *image.RGBA, clip *preparedClip, scratch *rasterScratch) error {
	bounds := layer.Bounds()
	maskBounds := image.Rect(0, 0, bounds.Dx(), bounds.Dy())
	maskBytes, err := scratch.offscreen.reserve(maskBounds, 1, "clip Alpha mask")
	if err != nil {
		return err
	}
	defer scratch.offscreen.release(maskBytes)
	mask := image.NewAlpha(maskBounds)
	if !clip.bounds.Intersect(bounds).Empty() {
		if clip.fillRule == d2scene.EvenOdd {
			if err := rasterizeEvenOddMask(ctx, mask, bounds.Min, clip.subpaths); err != nil {
				return err
			}
		} else {
			rasterizer := scratch.reset(mask.Bounds())
			shifted := d2scene.Translate(-float64(bounds.Min.X), -float64(bounds.Min.Y))
			for index, path := range clip.subpaths {
				if index&255 == 0 {
					if err := ctx.Err(); err != nil {
						return err
					}
				}
				addFillSubpath(rasterizer, path, shifted)
			}
			if err := rasterizer.WriteAlpha(ctx, scratch.workBudget(), mask); err != nil {
				return fmt.Errorf("d2raster: clip: %w", err)
			}
		}
	}
	return multiplyLayerByAlpha(ctx, layer, mask)
}

// Even-odd fills and clips use bounded 4x4 supersampling over already-flattened
// device-space paths. Ordinary non-zero geometry uses the scanline rasterizer.
func rasterizeEvenOddMask(ctx context.Context, mask *image.Alpha, origin image.Point, paths []subpath) error {
	const sampleCount = evenOddSamplesPerAxis * evenOddSamplesPerAxis
	var edgeEvaluations uint64
	for y := 0; y < mask.Bounds().Dy(); y++ {
		if y&31 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		row := y * mask.Stride
		for x := 0; x < mask.Bounds().Dx(); x++ {
			if x&255 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			inside := 0
			for sampleY := 0; sampleY < evenOddSamplesPerAxis; sampleY++ {
				py := float64(origin.Y+y) + (float64(sampleY)+0.5)/evenOddSamplesPerAxis
				for sampleX := 0; sampleX < evenOddSamplesPerAxis; sampleX++ {
					px := float64(origin.X+x) + (float64(sampleX)+0.5)/evenOddSamplesPerAxis
					isInside, err := pointInEvenOddPath(ctx, paths, px, py, &edgeEvaluations)
					if err != nil {
						return err
					}
					if isInside {
						inside++
					}
				}
			}
			mask.Pix[row+x] = uint8((inside*255 + sampleCount/2) / sampleCount)
		}
	}
	return ctx.Err()
}

func pointInEvenOddPath(ctx context.Context, paths []subpath, x, y float64, edgeEvaluations *uint64) (bool, error) {
	inside := false
	for _, path := range paths {
		if len(path.points) < 2 {
			continue
		}
		previous := path.points[len(path.points)-1]
		for _, current := range path.points {
			*edgeEvaluations = *edgeEvaluations + 1
			if *edgeEvaluations&255 == 0 {
				if err := ctx.Err(); err != nil {
					return false, err
				}
			}
			if (current.Y > y) != (previous.Y > y) &&
				x < (previous.X-current.X)*(y-current.Y)/(previous.Y-current.Y)+current.X {
				inside = !inside
			}
			previous = current
		}
	}
	return inside, nil
}

func multiplyLayerByAlpha(ctx context.Context, layer *image.RGBA, mask *image.Alpha) error {
	width, height := layer.Bounds().Dx(), layer.Bounds().Dy()
	for y := 0; y < height; y++ {
		if y&31 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		layerOffset := layer.PixOffset(layer.Bounds().Min.X, layer.Bounds().Min.Y+y)
		maskOffset := mask.PixOffset(mask.Bounds().Min.X, mask.Bounds().Min.Y+y)
		for x := 0; x < width; x++ {
			if x&4095 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			scalePremultiplied(layer.Pix[layerOffset+x*4:layerOffset+x*4+4], mask.Pix[maskOffset+x])
		}
	}
	return ctx.Err()
}

func applyMask(ctx context.Context, layer *image.RGBA, mask *preparedMask, scratch *rasterScratch) error {
	maskBytes, err := scratch.offscreen.reserve(layer.Bounds(), 4, "scene mask RGBA layer")
	if err != nil {
		return err
	}
	defer scratch.offscreen.release(maskBytes)
	maskLayer := image.NewRGBA(layer.Bounds())
	if err := renderNode(ctx, maskLayer, mask.root, scratch); err != nil {
		return err
	}
	width, height := layer.Bounds().Dx(), layer.Bounds().Dy()
	for y := 0; y < height; y++ {
		if y&31 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		layerOffset := layer.PixOffset(layer.Bounds().Min.X, layer.Bounds().Min.Y+y)
		maskOffset := maskLayer.PixOffset(maskLayer.Bounds().Min.X, maskLayer.Bounds().Min.Y+y)
		for x := 0; x < width; x++ {
			if x&4095 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			i := maskOffset + x*4
			coverage := maskLayer.Pix[i+3]
			if mask.kind == d2scene.MaskLuminance {
				// RGBA stores premultiplied channels, so Rec.709 luminance of
				// these channels is already luminance multiplied by alpha, as
				// required by SVG luminance masks.
				coverage = uint8((2126*uint32(maskLayer.Pix[i]) +
					7152*uint32(maskLayer.Pix[i+1]) +
					722*uint32(maskLayer.Pix[i+2]) + 5000) / 10000)
			}
			j := layerOffset + x*4
			scalePremultiplied(layer.Pix[j:j+4], coverage)
		}
	}
	return ctx.Err()
}

func scalePremultiplied(pixel []byte, alpha uint8) {
	if alpha == 255 {
		return
	}
	if alpha == 0 {
		pixel[0], pixel[1], pixel[2], pixel[3] = 0, 0, 0, 0
		return
	}
	for channel := range 4 {
		pixel[channel] = uint8((uint32(pixel[channel])*uint32(alpha) + 127) / 255)
	}
}

func supportedBlendMode(mode d2scene.BlendMode) bool {
	switch mode {
	case d2scene.BlendNormal, d2scene.BlendMultiply, d2scene.BlendDarken,
		d2scene.BlendColorBurn, d2scene.BlendOverlay, d2scene.BlendLighten:
		return true
	default:
		return false
	}
}

func supportedPreparedBlendMode(mode d2scene.BlendMode) bool {
	return supportedBlendMode(mode) || mode == preparedCOLRv1SoftLight
}

func compositeLayer(ctx context.Context, dst, layer *image.RGBA, opacity float64, mode d2scene.BlendMode) error {
	if mode == d2scene.BlendNormal {
		// Use deterministic integer source-over arithmetic for normal blending.
		return compositeLayerOver(ctx, dst, layer, opacity)
	}
	if !supportedPreparedBlendMode(mode) {
		return fmt.Errorf("d2raster: invalid or unsupported blend mode %d", mode)
	}
	if mode == preparedCOLRv1SoftLight {
		return compositeCOLRv1SoftLightLayer(ctx, dst, layer, opacity)
	}
	return compositeBlendLayer(ctx, dst, layer, opacity, mode)
}

// compositeCOLRv1SoftLightLayer performs the W3C soft-light operation in the
// linear-light sRGB space required by OpenType COLRv1. image.RGBA stores
// premultiplied, transfer-encoded channels, so each pixel is decoded before
// blending and encoded after source-over compositing.
func compositeCOLRv1SoftLightLayer(ctx context.Context, dst, layer *image.RGBA, opacity float64) error {
	bounds := layer.Bounds().Intersect(dst.Bounds())
	if bounds.Empty() || opacity == 0 {
		return nil
	}
	opacityScale := math.Round(opacity*255) / 255
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		if (y-bounds.Min.Y)&31 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		destinationOffset := dst.PixOffset(bounds.Min.X, y)
		sourceOffset := layer.PixOffset(bounds.Min.X, y)
		for x := 0; x < bounds.Dx(); x++ {
			if x&4095 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			si := sourceOffset + x*4
			di := destinationOffset + x*4
			storedSourceAlpha := float64(layer.Pix[si+3]) / 255
			sourceAlpha := storedSourceAlpha * opacityScale
			if sourceAlpha == 0 {
				continue
			}
			backdropAlpha := float64(dst.Pix[di+3]) / 255
			outputAlpha := sourceAlpha + backdropAlpha*(1-sourceAlpha)
			outputAlphaByte := roundedByte(outputAlpha * 255)
			for channel := range 3 {
				sourceEncoded := float64(layer.Pix[si+channel]) / float64(layer.Pix[si+3])
				backdropEncoded := 0.0
				if dst.Pix[di+3] != 0 {
					backdropEncoded = float64(dst.Pix[di+channel]) / float64(dst.Pix[di+3])
				}
				sourceLinear := srgbToLinear(sourceEncoded)
				backdropLinear := srgbToLinear(backdropEncoded)
				mixedLinear := softLight(backdropLinear, sourceLinear)
				outputPremultipliedLinear := sourceAlpha*(1-backdropAlpha)*sourceLinear +
					sourceAlpha*backdropAlpha*mixedLinear +
					backdropAlpha*(1-sourceAlpha)*backdropLinear
				encoded := 0.0
				if outputAlpha > 0 {
					encoded = linearToSRGB(outputPremultipliedLinear / outputAlpha)
				}
				value := roundedByte(encoded * outputAlpha * 255)
				if value > outputAlphaByte {
					value = outputAlphaByte
				}
				dst.Pix[di+channel] = value
			}
			dst.Pix[di+3] = outputAlphaByte
		}
	}
	return ctx.Err()
}

// compositeBlendLayer implements the separable blend/source-over formula from
// W3C Compositing and Blending Level 1. RGBA stores premultiplied channels, but
// blend functions operate on unpremultiplied sRGB components. Group opacity is
// applied after the isolated layer and before blending with the backdrop.
func compositeBlendLayer(ctx context.Context, dst, layer *image.RGBA, opacity float64, mode d2scene.BlendMode) error {
	bounds := layer.Bounds().Intersect(dst.Bounds())
	if bounds.Empty() || opacity == 0 {
		return nil
	}
	opacityByte := math.Round(opacity * 255)
	opacityScale := opacityByte / 255
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		if (y-bounds.Min.Y)&31 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		destinationOffset := dst.PixOffset(bounds.Min.X, y)
		sourceOffset := layer.PixOffset(bounds.Min.X, y)
		for x := 0; x < bounds.Dx(); x++ {
			if x&4095 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			si := sourceOffset + x*4
			di := destinationOffset + x*4
			sourceAlphaStored := float64(layer.Pix[si+3]) / 255
			sourceAlpha := sourceAlphaStored * opacityScale
			if sourceAlpha == 0 {
				continue
			}
			backdropAlpha := float64(dst.Pix[di+3]) / 255
			outputAlpha := sourceAlpha + backdropAlpha*(1-sourceAlpha)
			outputAlphaByte := roundedByte(outputAlpha * 255)
			for channel := 0; channel < 3; channel++ {
				sourcePremultiplied := float64(layer.Pix[si+channel]) / 255 * opacityScale
				backdropPremultiplied := float64(dst.Pix[di+channel]) / 255
				sourceColor := float64(layer.Pix[si+channel]) / float64(layer.Pix[si+3])
				backdropColor := 0.0
				if dst.Pix[di+3] != 0 {
					backdropColor = float64(dst.Pix[di+channel]) / float64(dst.Pix[di+3])
				}
				mixed := blendComponent(mode, backdropColor, sourceColor)
				outputPremultiplied := sourcePremultiplied*(1-backdropAlpha) +
					sourceAlpha*backdropAlpha*mixed +
					backdropPremultiplied*(1-sourceAlpha)
				value := roundedByte(outputPremultiplied * 255)
				if value > outputAlphaByte {
					value = outputAlphaByte
				}
				dst.Pix[di+channel] = value
			}
			dst.Pix[di+3] = outputAlphaByte
		}
	}
	return ctx.Err()
}

func blendComponent(mode d2scene.BlendMode, backdrop, source float64) float64 {
	switch mode {
	case d2scene.BlendMultiply:
		return backdrop * source
	case d2scene.BlendDarken:
		return math.Min(backdrop, source)
	case d2scene.BlendColorBurn:
		if backdrop >= 1 {
			return 1
		}
		if source <= 0 {
			return 0
		}
		return 1 - math.Min(1, (1-backdrop)/source)
	case d2scene.BlendOverlay:
		if backdrop <= .5 {
			return 2 * backdrop * source
		}
		return 1 - 2*(1-backdrop)*(1-source)
	case d2scene.BlendLighten:
		return math.Max(backdrop, source)
	case preparedCOLRv1SoftLight:
		return softLight(backdrop, source)
	default:
		return source
	}
}

func softLight(backdrop, source float64) float64 {
	if source <= .5 {
		return backdrop - (1-2*source)*backdrop*(1-backdrop)
	}
	d := math.Sqrt(backdrop)
	if backdrop <= .25 {
		d = ((16*backdrop-12)*backdrop + 4) * backdrop
	}
	return backdrop + (2*source-1)*(d-backdrop)
}

func roundedByte(value float64) uint8 {
	if value <= 0 {
		return 0
	}
	if value >= 255 {
		return 255
	}
	return uint8(math.Round(value))
}

func compositeLayerOver(ctx context.Context, dst, layer *image.RGBA, opacity float64) error {
	bounds := layer.Bounds().Intersect(dst.Bounds())
	if bounds.Empty() || opacity == 0 {
		return nil
	}
	opacityByte := uint32(math.Round(opacity * 255))
	mul255 := func(left, right uint32) uint32 { return (left*right + 127) / 255 }
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		if (y-bounds.Min.Y)&31 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		destinationOffset := dst.PixOffset(bounds.Min.X, y)
		sourceOffset := layer.PixOffset(bounds.Min.X, y)
		for x := 0; x < bounds.Dx(); x++ {
			if x&4095 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			si := sourceOffset + x*4
			di := destinationOffset + x*4
			sourceAlpha := mul255(uint32(layer.Pix[si+3]), opacityByte)
			if sourceAlpha == 0 {
				continue
			}
			inverseAlpha := 255 - sourceAlpha
			for channel := 0; channel < 3; channel++ {
				source := mul255(uint32(layer.Pix[si+channel]), opacityByte)
				value := source + mul255(uint32(dst.Pix[di+channel]), inverseAlpha)
				if value > 255 {
					value = 255
				}
				dst.Pix[di+channel] = uint8(value)
			}
			alpha := sourceAlpha + mul255(uint32(dst.Pix[di+3]), inverseAlpha)
			if alpha > 255 {
				alpha = 255
			}
			dst.Pix[di+3] = uint8(alpha)
		}
	}
	return ctx.Err()
}
