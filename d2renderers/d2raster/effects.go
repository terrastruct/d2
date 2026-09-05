package d2raster

import (
	"context"
	"errors"
	"fmt"
	"image"
	"math"
	"math/bits"

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

type offscreenPixelKind uint8

const (
	offscreenRGBA offscreenPixelKind = iota
	offscreenAlpha
)

type cachedOffscreenImage struct {
	rgba  *image.RGBA
	alpha *image.Alpha
}

func (entry cachedOffscreenImage) byteLen() int {
	if entry.rgba != nil {
		return len(entry.rgba.Pix)
	}
	if entry.alpha != nil {
		return len(entry.alpha.Pix)
	}
	return 0
}

func (entry cachedOffscreenImage) isKind(kind offscreenPixelKind) bool {
	if kind == offscreenRGBA {
		return entry.rgba != nil
	}
	return entry.alpha != nil
}

type offscreenBudget struct {
	limit       int64
	live        int64
	peak        int64
	cachedBytes int64
	cache       cachedOffscreenImage
}

func (b *offscreenBudget) reserveBytes(bytes int64, purpose string) (int64, error) {
	if bytes > b.limit-b.live {
		return 0, fmt.Errorf(
			"d2raster: offscreen %s requires %d bytes with %d bytes already live, exceeding limit %d",
			purpose, bytes, b.live, b.limit,
		)
	}
	// Cached pixel buffers remain part of the exact offscreen working-set peak,
	// not merely under the caller's (potentially much larger) configured limit.
	// This ensures reuse never retains more pixel storage than the unpooled
	// traversal already needed simultaneously. Logical live/peak retain their
	// existing meaning and remain exactly comparable with the preflight plan.
	newLive := b.live + bytes
	retainedLimit := maxInt64(b.peak, newLive)
	b.trimCache(retainedLimit - newLive)
	b.addLive(bytes)
	return bytes, nil
}

func (b *offscreenBudget) addLive(bytes int64) {
	b.live += bytes
	if b.live > b.peak {
		b.peak = b.live
	}
}

func (b *offscreenBudget) release(bytes int64) {
	b.live -= bytes
}

func (b *offscreenBudget) newRGBA(bounds image.Rectangle, purpose string) (*image.RGBA, int64, error) {
	bytes, err := pixelStorageBytes(bounds, 4)
	if err != nil {
		return nil, 0, fmt.Errorf("d2raster: %s: %w", purpose, err)
	}
	entry, reservation, err := b.reserveImage(bytes, offscreenRGBA, purpose)
	if err != nil {
		return nil, 0, err
	}
	if entry.rgba == nil {
		return image.NewRGBA(bounds), reservation, nil
	}
	clear(entry.rgba.Pix)
	entry.rgba.Stride = 4 * bounds.Dx()
	entry.rgba.Rect = bounds
	return entry.rgba, reservation, nil
}

func (b *offscreenBudget) newAlpha(bounds image.Rectangle, purpose string) (*image.Alpha, int64, error) {
	bytes, err := pixelStorageBytes(bounds, 1)
	if err != nil {
		return nil, 0, fmt.Errorf("d2raster: %s: %w", purpose, err)
	}
	entry, reservation, err := b.reserveImage(bytes, offscreenAlpha, purpose)
	if err != nil {
		return nil, 0, err
	}
	if entry.alpha == nil {
		return image.NewAlpha(bounds), reservation, nil
	}
	clear(entry.alpha.Pix)
	entry.alpha.Stride = bounds.Dx()
	entry.alpha.Rect = bounds
	return entry.alpha, reservation, nil
}

func (b *offscreenBudget) reserveImage(bytes int64, kind offscreenPixelKind, purpose string) (cachedOffscreenImage, int64, error) {
	if bytes > b.limit-b.live {
		return cachedOffscreenImage{}, 0, fmt.Errorf(
			"d2raster: offscreen %s requires %d bytes with %d bytes already live, exceeding limit %d",
			purpose, bytes, b.live, b.limit,
		)
	}
	if bytes != 0 && b.cache.isKind(kind) && int64(b.cache.byteLen()) == bytes {
		entry := b.cache
		b.dropCached()
		b.addLive(bytes)
		return entry, bytes, nil
	}
	// Only the most recently released exact-sized image is a strong reuse
	// candidate. Drop a mismatched candidate before allocating so a run of
	// one-use sizes has the same retained pixel storage as the unpooled path.
	// Repeated effect/filter sizes still reuse immediately without allocation.
	if b.cachedBytes != 0 {
		b.dropCached()
	}
	reservation, err := b.reserveBytes(bytes, purpose)
	return cachedOffscreenImage{}, reservation, err
}

func (b *offscreenBudget) recycleRGBA(buffer *image.RGBA, reservation int64) {
	if buffer == nil {
		b.release(reservation)
		return
	}
	b.recycleImage(cachedOffscreenImage{rgba: buffer}, reservation)
}

func (b *offscreenBudget) recycleAlpha(buffer *image.Alpha, reservation int64) {
	if buffer == nil {
		b.release(reservation)
		return
	}
	b.recycleImage(cachedOffscreenImage{alpha: buffer}, reservation)
}

func (b *offscreenBudget) recycleImage(entry cachedOffscreenImage, reservation int64) {
	b.release(reservation)
	if reservation == 0 || int64(entry.byteLen()) != reservation {
		return
	}
	if b.cachedBytes != 0 {
		b.dropCached()
	}
	b.cache = entry
	b.cachedBytes += reservation
}

func (b *offscreenBudget) trimCache(maxBytes int64) {
	if b.cachedBytes > maxBytes {
		b.dropCached()
	}
}

func (b *offscreenBudget) dropCached() {
	b.cache = cachedOffscreenImage{}
	b.cachedBytes = 0
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
		if node.clip.fillRule == d2scene.EvenOdd {
			clipBytes, err := pixelStorageBytes(visibleBounds, 1)
			if err != nil {
				return resources, fmt.Errorf("d2raster: clip mask: %w", err)
			}
			withClip, ok := checkedAdd(finalLayerBytes, clipBytes)
			if !ok {
				return resources, fmt.Errorf("d2raster: peak offscreen pixel storage exceeds the int64 domain")
			}
			resources.peakOffscreenBytes = maxInt64(resources.peakOffscreenBytes, withClip)
			work, err := evenOddMaskWork(visibleBounds, node.clip.edges)
			if err != nil {
				return resources, err
			}
			if err := addResourceWork(&resources, work); err != nil {
				return resources, err
			}
		} else if !node.clip.bounds.Intersect(visibleBounds).Empty() {
			target := image.Rect(0, 0, visibleBounds.Dx(), visibleBounds.Dy())
			shifted := d2scene.Translate(-float64(visibleBounds.Min.X), -float64(visibleBounds.Min.Y))
			if err := planner.recordFill(ctx, &resources, target, node.clip.subpaths, shifted, "clip"); err != nil {
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
	layer, layerBytes, err := scratch.offscreen.newRGBA(bounds, "effect layer")
	if err != nil {
		return err
	}
	defer scratch.offscreen.recycleRGBA(layer, layerBytes)
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
		if err := applyPreparedFilter(ctx, &current, filter, scratch); err != nil {
			return err
		}
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
	if clip.fillRule != d2scene.EvenOdd {
		return applyNonZeroClip(ctx, layer, clip, scratch)
	}
	bounds := layer.Bounds()
	maskBounds := image.Rect(0, 0, bounds.Dx(), bounds.Dy())
	mask, maskBytes, err := scratch.offscreen.newAlpha(maskBounds, "clip Alpha mask")
	if err != nil {
		return err
	}
	defer scratch.offscreen.recycleAlpha(mask, maskBytes)
	if !clip.bounds.Intersect(bounds).Empty() {
		if err := rasterizeEvenOddMask(ctx, mask, bounds.Min, clip.subpaths); err != nil {
			return err
		}
	}
	return multiplyLayerByAlpha(ctx, layer, mask)
}

// applyNonZeroClip consumes analytic coverage one row at a time. Clip coverage
// is not reused after scaling the effect layer, so retaining a canvas-sized
// Alpha image would add storage and a second memory pass without preserving
// useful state.
func applyNonZeroClip(ctx context.Context, layer *image.RGBA, clip *preparedClip, scratch *rasterScratch) error {
	bounds := layer.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width == 0 || height == 0 {
		return ctx.Err()
	}
	if clip.bounds.Intersect(bounds).Empty() {
		return clearLayerRows(ctx, layer, 0, height)
	}

	rasterizer := scratch.reset(image.Rect(0, 0, width, height))
	shifted := d2scene.Translate(-float64(bounds.Min.X), -float64(bounds.Min.Y))
	for index, path := range clip.subpaths {
		if index&255 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		addFillSubpath(rasterizer, path, shifted)
	}

	nextRow := 0
	var callbackErr error
	err := rasterizer.WalkCoverage(ctx, scratch.workBudget(), func(y, minX int, partial, difference []float32) error {
		if err := ctx.Err(); err != nil {
			callbackErr = err
			return err
		}
		if err := clearLayerRows(ctx, layer, nextRow, y); err != nil {
			callbackErr = err
			return err
		}
		rowOffset := layer.PixOffset(bounds.Min.X, bounds.Min.Y+y)
		row := layer.Pix[rowOffset : rowOffset+width*4]
		if err := clearLayerPixelRange(ctx, row, 0, minX); err != nil {
			callbackErr = err
			return err
		}

		var winding float32
		for index := 0; index < len(partial); {
			if index != 0 && index&4095 == 0 {
				if err := ctx.Err(); err != nil {
					callbackErr = err
					return err
				}
			}
			if partial[index] == 0 && difference[index] == 0 {
				runEnd := index + 1
				for runEnd < len(partial) && partial[runEnd] == 0 && difference[runEnd] == 0 {
					runEnd++
				}
				if err := scaleLayerCoverageSpan(ctx, row, minX+index, minX+runEnd, scanline.QuantizeCoverage(winding)); err != nil {
					callbackErr = err
					return err
				}
				index = runEnd
				continue
			}
			winding += difference[index]
			coverage := scanline.QuantizeCoverage(partial[index] + winding)
			if err := scaleLayerCoverageSpan(ctx, row, minX+index, minX+index+1, coverage); err != nil {
				callbackErr = err
				return err
			}
			index++
		}
		maxX := minX + len(partial)
		if err := clearLayerPixelRange(ctx, row, maxX, width); err != nil {
			callbackErr = err
			return err
		}
		nextRow = y + 1
		return nil
	})
	if err != nil {
		// Rasterizer and work-budget errors retain the established clip context.
		// Errors from scaling the already-rasterized row correspond to the old
		// mask-application phase and remain unwrapped.
		if callbackErr != nil && err == callbackErr {
			return err
		}
		return fmt.Errorf("d2raster: clip: %w", err)
	}
	return clearLayerRows(ctx, layer, nextRow, height)
}

func clearLayerRows(ctx context.Context, layer *image.RGBA, first, last int) error {
	if first >= last {
		return nil
	}
	width := layer.Bounds().Dx()
	for y := first; y < last; y++ {
		if y == first || (y-first)&31 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		rowOffset := layer.PixOffset(layer.Bounds().Min.X, layer.Bounds().Min.Y+y)
		row := layer.Pix[rowOffset : rowOffset+width*4]
		if err := clearLayerPixelRange(ctx, row, 0, width); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func clearLayerPixelRange(ctx context.Context, row []byte, first, last int) error {
	for start := first; start < last; {
		if start != first && (start-first)&4095 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		end := min(start+4096, last)
		clear(row[start*4 : end*4])
		start = end
	}
	return nil
}

func scaleLayerCoverageSpan(ctx context.Context, row []byte, first, last int, coverage uint8) error {
	switch coverage {
	case 0:
		return clearLayerPixelRange(ctx, row, first, last)
	case 0xff:
		return nil
	}
	for x := first; x < last; x++ {
		if x != first && (x-first)&4095 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		offset := x * 4
		scalePremultiplied(row[offset:offset+4], coverage)
	}
	return nil
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
		pixelY := float64(origin.Y + y)
		sampleY0 := pixelY + 0.5/evenOddSamplesPerAxis
		sampleY1 := pixelY + 1.5/evenOddSamplesPerAxis
		sampleY2 := pixelY + 2.5/evenOddSamplesPerAxis
		sampleY3 := pixelY + 3.5/evenOddSamplesPerAxis
		for x := 0; x < mask.Bounds().Dx(); x++ {
			if x&255 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			pixelX := float64(origin.X + x)
			sampleX0 := pixelX + 0.5/evenOddSamplesPerAxis
			sampleX1 := pixelX + 1.5/evenOddSamplesPerAxis
			sampleX2 := pixelX + 2.5/evenOddSamplesPerAxis
			sampleX3 := pixelX + 3.5/evenOddSamplesPerAxis
			inside, err := countEvenOddPathSamples(
				ctx, paths,
				sampleX0, sampleX1, sampleX2, sampleX3,
				sampleY0, sampleY1, sampleY2, sampleY3,
				&edgeEvaluations,
			)
			if err != nil {
				return err
			}
			mask.Pix[row+x] = uint8((inside*255 + sampleCount/2) / sampleCount)
		}
	}
	return ctx.Err()
}

// countEvenOddPathSamples shares each edge intersection across the four
// horizontal samples at each sample Y. It retains the independent point test's
// crossing expression and strict threshold semantics, while edgeEvaluations
// continues to count that logical work and checks cancellation every 256
// evaluations.
func countEvenOddPathSamples(
	ctx context.Context,
	paths []subpath,
	x0, x1, x2, x3 float64,
	y0, y1, y2, y3 float64,
	edgeEvaluations *uint64,
) (int, error) {
	inside := 0
	for _, y := range [...]float64{y0, y1, y2, y3} {
		var parity uint8
		for _, path := range paths {
			if len(path.points) < 2 {
				continue
			}
			previous := path.points[len(path.points)-1]
			for _, current := range path.points {
				*edgeEvaluations += evenOddSamplesPerAxis
				if *edgeEvaluations&255 == 0 {
					if err := ctx.Err(); err != nil {
						return 0, err
					}
				}
				if (current.Y > y) != (previous.Y > y) {
					crossingX := (previous.X-current.X)*(y-current.Y)/(previous.Y-current.Y) + current.X
					switch {
					case x3 < crossingX:
						parity ^= 0b1111
					case x2 < crossingX:
						parity ^= 0b0111
					case x1 < crossingX:
						parity ^= 0b0011
					case x0 < crossingX:
						parity ^= 0b0001
					}
				}
				previous = current
			}
		}
		inside += bits.OnesCount8(parity)
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
		if width == 0 {
			continue
		}
		layerRow := layer.Pix[layerOffset : layerOffset+width*4]
		maskRow := mask.Pix[maskOffset : maskOffset+width]
		for x := 0; x < width; {
			if x&4095 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			// Do not let a run cross the existing cancellation boundary. This
			// retains the exact Err-call cadence while allowing the common fully
			// transparent and fully opaque mask spans to be handled in bulk.
			chunkEnd := min((x|4095)+1, width)
			// Widely spaced probes keep mixed or continuously varying masks on
			// the tight scalar loop. Binary clip masks have long interior spans,
			// and all probes ordinarily land on 0 or 255.
			useRuns := chunkEnd-x >= 64
			if useRuns {
				for probe := range 16 {
					alpha := maskRow[x+(chunkEnd-x)*probe/16]
					if alpha != 0 && alpha != 0xff {
						useRuns = false
						break
					}
				}
			}
			if !useRuns {
				for ; x < chunkEnd; x++ {
					pixelOffset := x * 4
					scalePremultiplied(layerRow[pixelOffset:pixelOffset+4], maskRow[x])
				}
				continue
			}
			alpha := maskRow[x]
			switch alpha {
			case 0:
				runStart := x
				x++
				for x < chunkEnd && maskRow[x] == 0 {
					x++
				}
				clear(layerRow[runStart*4 : x*4])
			case 0xff:
				x++
				for x < chunkEnd && maskRow[x] == 0xff {
					x++
				}
			default:
				pixelOffset := x * 4
				scalePremultiplied(layerRow[pixelOffset:pixelOffset+4], alpha)
				x++
			}
		}
	}
	return ctx.Err()
}

func applyMask(ctx context.Context, layer *image.RGBA, mask *preparedMask, scratch *rasterScratch) error {
	maskLayer, maskBytes, err := scratch.offscreen.newRGBA(layer.Bounds(), "scene mask RGBA layer")
	if err != nil {
		return err
	}
	defer scratch.offscreen.recycleRGBA(maskLayer, maskBytes)
	if err := renderNode(ctx, maskLayer, mask.root, scratch); err != nil {
		return err
	}
	if mask.kind == d2scene.MaskAlpha {
		return multiplyLayerByRGBAAlpha(ctx, layer, maskLayer)
	}
	return multiplyLayerByRGBALuminance(ctx, layer, maskLayer)
}

func multiplyLayerByRGBALuminance(ctx context.Context, layer, mask *image.RGBA) error {
	width, height := layer.Bounds().Dx(), layer.Bounds().Dy()
	for y := 0; y < height; y++ {
		if y&31 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		layerOffset := layer.PixOffset(layer.Bounds().Min.X, layer.Bounds().Min.Y+y)
		maskOffset := mask.PixOffset(mask.Bounds().Min.X, mask.Bounds().Min.Y+y)
		layerRow := layer.Pix[layerOffset : layerOffset+width*4]
		maskRow := mask.Pix[maskOffset : maskOffset+width*4]
		if width < 64 || !binaryLuminancePixel(maskRow, 0) {
			for x := 0; x < width; x++ {
				if x&4095 == 0 {
					if err := ctx.Err(); err != nil {
						return err
					}
				}
				i := maskOffset + x*4
				coverage := uint8((2126*uint32(mask.Pix[i]) +
					7152*uint32(mask.Pix[i+1]) +
					722*uint32(mask.Pix[i+2]) + 5000) / 10000)
				j := layerOffset + x*4
				scalePremultiplied(layer.Pix[j:j+4], coverage)
			}
			continue
		}
		for x := 0; x < width; {
			if x&4095 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			chunkEnd := min((x|4095)+1, width)
			if chunkEnd-x < 64 || x != 0 && !binaryLuminancePixel(maskRow, x) {
				for ; x < chunkEnd; x++ {
					pixelOffset := x * 4
					scalePremultiplied(layerRow[pixelOffset:pixelOffset+4], rgbaLuminance(maskRow, pixelOffset))
				}
				continue
			}

			// Sample the rest of the cancellation-bounded chunk before
			// selecting the binary-mask path. Continuously varying masks keep
			// the branch-free scalar loop, while solid black and white SVG masks
			// can clear or preserve whole spans.
			binarySamples := 1
			span := chunkEnd - x
			for probe := 1; probe < 16; probe++ {
				if binaryLuminancePixel(maskRow, x+(span-1)*probe/15) {
					binarySamples++
				}
			}
			if binarySamples < 7 {
				for ; x < chunkEnd; x++ {
					pixelOffset := x * 4
					scalePremultiplied(layerRow[pixelOffset:pixelOffset+4], rgbaLuminance(maskRow, pixelOffset))
				}
				continue
			}

			// Consume a uniform prefix in bulk. Solid mask rows normally cover
			// the entire chunk; checkerboards only pay for one failed comparison
			// before continuing through the direct binary loop below.
			runStart := x
			if blackLuminancePixel(maskRow, x*4) {
				x++
				for x < chunkEnd && blackLuminancePixel(maskRow, x*4) {
					x++
				}
				clear(layerRow[runStart*4 : x*4])
			} else {
				x++
				for x < chunkEnd && whiteLuminancePixel(maskRow, x*4) {
					x++
				}
			}
			for x < chunkEnd {
				pixelOffset := x * 4
				switch rgbaRGB(maskRow, pixelOffset) {
				case 0:
					layerRow[pixelOffset], layerRow[pixelOffset+1], layerRow[pixelOffset+2], layerRow[pixelOffset+3] = 0, 0, 0, 0
				case 0xffffff:
				default:
					scalePremultiplied(layerRow[pixelOffset:pixelOffset+4], rgbaLuminance(maskRow, pixelOffset))
				}
				x++
			}
		}
	}
	return ctx.Err()
}

func rgbaLuminance(pixels []byte, offset int) uint8 {
	// RGBA stores premultiplied channels, so Rec.709 luminance of these
	// channels is already luminance multiplied by alpha, as required by SVG
	// luminance masks.
	return uint8((2126*uint32(pixels[offset]) +
		7152*uint32(pixels[offset+1]) +
		722*uint32(pixels[offset+2]) + 5000) / 10000)
}

func binaryLuminancePixel(pixels []byte, pixel int) bool {
	rgb := rgbaRGB(pixels, pixel*4)
	return rgb == 0 || rgb == 0xffffff
}

func blackLuminancePixel(pixels []byte, offset int) bool {
	return rgbaRGB(pixels, offset) == 0
}

func whiteLuminancePixel(pixels []byte, offset int) bool {
	return rgbaRGB(pixels, offset) == 0xffffff
}

func rgbaRGB(pixels []byte, offset int) uint32 {
	return uint32(pixels[offset]) | uint32(pixels[offset+1])<<8 | uint32(pixels[offset+2])<<16
}

func multiplyLayerByRGBAAlpha(ctx context.Context, layer, mask *image.RGBA) error {
	width, height := layer.Bounds().Dx(), layer.Bounds().Dy()
	for y := 0; y < height; y++ {
		if y&31 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		layerOffset := layer.PixOffset(layer.Bounds().Min.X, layer.Bounds().Min.Y+y)
		maskOffset := mask.PixOffset(mask.Bounds().Min.X, mask.Bounds().Min.Y+y)
		if width == 0 {
			continue
		}
		layerRow := layer.Pix[layerOffset : layerOffset+width*4]
		maskRow := mask.Pix[maskOffset : maskOffset+width*4]
		if width < 64 || maskRow[3] != 0 && maskRow[3] != 0xff {
			for x := 0; x < width; x++ {
				if x&4095 == 0 {
					if err := ctx.Err(); err != nil {
						return err
					}
				}
				pixelOffset := layerOffset + x*4
				scalePremultiplied(layer.Pix[pixelOffset:pixelOffset+4], mask.Pix[maskOffset+x*4+3])
			}
			continue
		}
		for x := 0; x < width; {
			if x&4095 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			chunkEnd := min((x|4095)+1, width)
			firstAlpha := mask.Pix[maskOffset+x*4+3]
			useRuns := chunkEnd-x >= 64 && (firstAlpha == 0 || firstAlpha == 0xff)
			if useRuns {
				for probe := range 16 {
					alpha := mask.Pix[maskOffset+(x+probe)*4+3]
					if alpha != 0 && alpha != 0xff {
						useRuns = false
						break
					}
				}
			}
			if !useRuns {
				for ; x < chunkEnd; x++ {
					pixelOffset := layerOffset + x*4
					scalePremultiplied(layer.Pix[pixelOffset:pixelOffset+4], mask.Pix[maskOffset+x*4+3])
				}
				continue
			}
			alpha := maskRow[x*4+3]
			switch alpha {
			case 0:
				runStart := x
				x++
				for x < chunkEnd && maskRow[x*4+3] == 0 {
					x++
				}
				clear(layerRow[runStart*4 : x*4])
			case 0xff:
				x++
				for x < chunkEnd && maskRow[x*4+3] == 0xff {
					x++
				}
			default:
				pixelOffset := x * 4
				scalePremultiplied(layerRow[pixelOffset:pixelOffset+4], alpha)
				x++
			}
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
		if opaquePartialOverPrefixEligible(dst, layer, opacity) {
			return compositeLayerOverOpaquePartialPrefix(ctx, dst, layer, uint32(math.Round(opacity*255)))
		}
		return compositeLayerOver(ctx, dst, layer, opacity)
	}
	if !supportedPreparedBlendMode(mode) {
		return fmt.Errorf("d2raster: invalid or unsupported blend mode %d", mode)
	}
	if mode == preparedCOLRv1SoftLight {
		return compositeCOLRv1SoftLightLayer(ctx, dst, layer, opacity)
	}
	if opaqueBlendPrefixEligible(dst, layer, opacity) {
		return compositeBlendLayerOpaquePrefix(ctx, dst, layer, mode)
	}
	return compositeBlendLayer(ctx, dst, layer, opacity, mode)
}

func opaquePartialOverPrefixEligible(dst, layer *image.RGBA, opacity float64) bool {
	opacityByte := math.Round(opacity * 255)
	if !(opacityByte > 0 && opacityByte < 255) {
		return false
	}
	bounds := layer.Bounds().Intersect(dst.Bounds())
	if bounds.Empty() {
		return false
	}
	firstSource := layer.PixOffset(bounds.Min.X, bounds.Min.Y)
	firstDestination := dst.PixOffset(bounds.Min.X, bounds.Min.Y)
	return layer.Pix[firstSource+3] == 255 && dst.Pix[firstDestination+3] == 255
}

func opaqueBlendPrefixEligible(dst, layer *image.RGBA, opacity float64) bool {
	if opacity == 0 || math.Round(opacity*255) != 255 {
		return false
	}
	bounds := layer.Bounds().Intersect(dst.Bounds())
	if bounds.Empty() {
		return false
	}
	firstSource := layer.PixOffset(bounds.Min.X, bounds.Min.Y)
	firstDestination := dst.PixOffset(bounds.Min.X, bounds.Min.Y)
	return layer.Pix[firstSource+3] == 255 && dst.Pix[firstDestination+3] == 255
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
		x := 0
		if layer.Pix[sourceOffset+3] == 0 {
			for ; x < bounds.Dx(); x++ {
				if layer.Pix[sourceOffset+x*4+3] != 0 {
					break
				}
				if x&4095 == 0 {
					if err := ctx.Err(); err != nil {
						return err
					}
				}
			}
		}
		for ; x < bounds.Dx(); x++ {
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
				sourceLinear := premultipliedSRGBByteToLinear(layer.Pix[si+channel], layer.Pix[si+3])
				backdropLinear := premultipliedSRGBByteToLinear(dst.Pix[di+channel], dst.Pix[di+3])
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

func compositeBlendLayerOpaquePrefix(ctx context.Context, dst, layer *image.RGBA, mode d2scene.BlendMode) error {
	bounds := layer.Bounds().Intersect(dst.Bounds())
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		if (y-bounds.Min.Y)&31 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		destinationOffset := dst.PixOffset(bounds.Min.X, y)
		sourceOffset := layer.PixOffset(bounds.Min.X, y)
		x := 0
		if layer.Pix[sourceOffset+3] == 0 {
			for ; x < bounds.Dx(); x++ {
				if layer.Pix[sourceOffset+x*4+3] != 0 {
					break
				}
				if x&4095 == 0 {
					if err := ctx.Err(); err != nil {
						return err
					}
				}
			}
		} else if layer.Pix[sourceOffset+3] == 255 && dst.Pix[destinationOffset+3] == 255 {
			// Consume an opaque prefix with byte-exact blend arithmetic. Stop at
			// the first partial-alpha pixel so mixed rows retain the original
			// branch-light floating-point loop for their entire remainder.
			for ; x < bounds.Dx(); x++ {
				si := sourceOffset + x*4
				di := destinationOffset + x*4
				if layer.Pix[si+3] != 255 || dst.Pix[di+3] != 255 {
					break
				}
				if x&4095 == 0 {
					if err := ctx.Err(); err != nil {
						return err
					}
				}
				destination := dst.Pix[di : di+4]
				source := layer.Pix[si : si+4]
				switch mode {
				case d2scene.BlendMultiply:
					destination[0] = multiplyOpaqueByte(destination[0], source[0])
					destination[1] = multiplyOpaqueByte(destination[1], source[1])
					destination[2] = multiplyOpaqueByte(destination[2], source[2])
				case d2scene.BlendDarken:
					destination[0] = min(destination[0], source[0])
					destination[1] = min(destination[1], source[1])
					destination[2] = min(destination[2], source[2])
				case d2scene.BlendColorBurn:
					for channel := range 3 {
						backdropColor := float64(destination[channel]) / 255
						sourceColor := float64(source[channel]) / 255
						switch {
						case backdropColor >= 1:
							destination[channel] = 255
						case sourceColor <= 0:
							destination[channel] = 0
						default:
							destination[channel] = roundedByte((1 - math.Min(1, (1-backdropColor)/sourceColor)) * 255)
						}
					}
				case d2scene.BlendOverlay:
					destination[0] = overlayOpaqueByte(destination[0], source[0])
					destination[1] = overlayOpaqueByte(destination[1], source[1])
					destination[2] = overlayOpaqueByte(destination[2], source[2])
				case d2scene.BlendLighten:
					destination[0] = max(destination[0], source[0])
					destination[1] = max(destination[1], source[1])
					destination[2] = max(destination[2], source[2])
				}
				destination[3] = 255
			}
		}
		for ; x < bounds.Dx(); x++ {
			if x&4095 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			si := sourceOffset + x*4
			di := destinationOffset + x*4
			sourceAlphaStored := float64(layer.Pix[si+3]) / 255
			if sourceAlphaStored == 0 {
				continue
			}
			backdropAlpha := float64(dst.Pix[di+3]) / 255
			outputAlpha := sourceAlphaStored + backdropAlpha*(1-sourceAlphaStored)
			outputAlphaByte := roundedByte(outputAlpha * 255)
			for channel := 0; channel < 3; channel++ {
				sourcePremultiplied := float64(layer.Pix[si+channel]) / 255
				backdropPremultiplied := float64(dst.Pix[di+channel]) / 255
				sourceColor := float64(layer.Pix[si+channel]) / float64(layer.Pix[si+3])
				backdropColor := 0.0
				if dst.Pix[di+3] != 0 {
					backdropColor = float64(dst.Pix[di+channel]) / float64(dst.Pix[di+3])
				}
				mixed := blendComponent(mode, backdropColor, sourceColor)
				outputPremultiplied := sourcePremultiplied*(1-backdropAlpha) +
					sourceAlphaStored*backdropAlpha*mixed +
					backdropPremultiplied*(1-sourceAlphaStored)
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

func multiplyOpaqueByte(backdrop, source byte) byte {
	return byte((uint32(backdrop)*uint32(source) + 127) / 255)
}

func overlayOpaqueByte(backdrop, source byte) byte {
	b, s := uint32(backdrop), uint32(source)
	if b <= 127 {
		return byte((2*b*s + 127) / 255)
	}
	return byte(255 - (2*(255-b)*(255-s)+127)/255)
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

func compositeLayerOverOpaquePartialPrefix(ctx context.Context, dst, layer *image.RGBA, opacityByte uint32) error {
	bounds := layer.Bounds().Intersect(dst.Bounds())
	mul255 := func(left, right uint32) uint32 { return (left*right + 127) / 255 }
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		if (y-bounds.Min.Y)&31 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		destinationOffset := dst.PixOffset(bounds.Min.X, y)
		sourceOffset := layer.PixOffset(bounds.Min.X, y)
		x := 0
		if layer.Pix[sourceOffset+3] == 255 && dst.Pix[destinationOffset+3] == 255 {
			for ; x < bounds.Dx(); x++ {
				si := sourceOffset + x*4
				di := destinationOffset + x*4
				if layer.Pix[si+3] != 255 || dst.Pix[di+3] != 255 {
					break
				}
				if x&4095 == 0 {
					if err := ctx.Err(); err != nil {
						return err
					}
				}
				dst.Pix[di] = compositeOpaquePartialOverByte(layer.Pix[si], dst.Pix[di], opacityByte)
				dst.Pix[di+1] = compositeOpaquePartialOverByte(layer.Pix[si+1], dst.Pix[di+1], opacityByte)
				dst.Pix[di+2] = compositeOpaquePartialOverByte(layer.Pix[si+2], dst.Pix[di+2], opacityByte)
				dst.Pix[di+3] = 255
			}
		}
		for ; x < bounds.Dx(); x++ {
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
			for channel := range 3 {
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

func compositeOpaquePartialOverByte(source, destination byte, opacity uint32) byte {
	scaledSource := (uint32(source)*opacity + 127) / 255
	inverseAlpha := 255 - opacity
	value := scaledSource + (uint32(destination)*inverseAlpha+127)/255
	if value > 255 {
		value = 255
	}
	return byte(value)
}

func compositeLayerOver(ctx context.Context, dst, layer *image.RGBA, opacity float64) error {
	bounds := layer.Bounds().Intersect(dst.Bounds())
	if bounds.Empty() || opacity == 0 {
		return nil
	}
	opacityByte := uint32(math.Round(opacity * 255))
	mul255 := func(left, right uint32) uint32 { return (left*right + 127) / 255 }
	if opacityByte == 0xff {
		// At full group opacity, transparent source blocks are no-ops and
		// opaque source blocks replace the destination byte-for-byte. Probe
		// small blocks only on rows that begin with one of those common alpha
		// values; partial-alpha rows retain a branch-light general loop.
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			if (y-bounds.Min.Y)&31 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			destinationOffset := dst.PixOffset(bounds.Min.X, y)
			sourceOffset := layer.PixOffset(bounds.Min.X, y)
			blockPixels := 4096
			detectUniformBlocks := layer.Pix[sourceOffset+3] == 0 || layer.Pix[sourceOffset+3] == 0xff
			if detectUniformBlocks {
				blockPixels = 64
			}
			for x := 0; x < bounds.Dx(); {
				if x&4095 == 0 {
					if err := ctx.Err(); err != nil {
						return err
					}
				}
				end := min(x+blockPixels, bounds.Dx())
				blockSource := sourceOffset + x*4
				blockDestination := destinationOffset + x*4
				blockAlpha := layer.Pix[blockSource+3]
				if detectUniformBlocks && end-x == blockPixels && (blockAlpha == 0 || blockAlpha == 0xff) && layer.Pix[blockSource+(blockPixels-1)*4+3] == blockAlpha {
					uniformPixels := blockPixels
					for blockX := 1; blockX < blockPixels-1; blockX++ {
						if layer.Pix[blockSource+blockX*4+3] != blockAlpha {
							uniformPixels = blockX
							break
						}
					}
					if uniformPixels == blockPixels {
						if blockAlpha == 0xff {
							copy(dst.Pix[blockDestination:blockDestination+blockPixels*4], layer.Pix[blockSource:blockSource+blockPixels*4])
						}
						x = end
						continue
					}
					// The alpha probe already proved this prefix is uniform. Consume
					// it rather than visiting the same pixels again in the general
					// loop. Keep very short opaque prefixes in the scalar loop since
					// a variable-sized memmove costs more than a few assignments.
					if blockAlpha == 0 || uniformPixels >= 8 {
						if blockAlpha == 0xff {
							copy(dst.Pix[blockDestination:blockDestination+uniformPixels*4], layer.Pix[blockSource:blockSource+uniformPixels*4])
						}
						x += uniformPixels
					}
				}
				for ; x < end; x++ {
					si := sourceOffset + x*4
					di := destinationOffset + x*4
					sourceAlpha := uint32(layer.Pix[si+3])
					if sourceAlpha == 0 {
						continue
					}
					if sourceAlpha == 0xff {
						dst.Pix[di], dst.Pix[di+1], dst.Pix[di+2], dst.Pix[di+3] = layer.Pix[si], layer.Pix[si+1], layer.Pix[si+2], 0xff
						continue
					}
					inverseAlpha := 255 - sourceAlpha
					for channel := 0; channel < 3; channel++ {
						value := uint32(layer.Pix[si+channel]) + mul255(uint32(dst.Pix[di+channel]), inverseAlpha)
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
		}
		return ctx.Err()
	}
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
