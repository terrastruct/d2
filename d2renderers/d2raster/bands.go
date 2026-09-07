package d2raster

import (
	"context"
	"fmt"
	"image"
	"image/draw"

	"github.com/d2lang/d2/d2renderers/d2raster/internal/scanline"
	"github.com/d2lang/d2/d2renderers/d2scene"
)

// RenderBands renders a frame from top to bottom using a reusable canvas of at
// most bandHeight rows. Each NRGBA band has absolute frame coordinates, full
// frame width, and consecutive rows. Its pixels are borrowed only until consume
// returns; a consumer that retains a band must copy it.
//
// Geometry and assets are prepared once. All resource limits, including the
// aggregate scanline and even-odd work of every band and filter overlap, are
// checked before the first callback. MaxPixels still bounds total output pixels;
// only the band canvas, rather than the full frame, must fit platform storage.
// MaxOffscreenBytes separately bounds retained pattern tiles and rendering
// scratch. The final band may have fewer rows. On any error, consume receives no
// further bands. A callback must not modify document or its assets.
func RenderBands(ctx context.Context, document *d2scene.Document, options FrameOptions, bandHeight int, consume func(*image.NRGBA) error) error {
	return renderBands(ctx, document, options, nil, bandHeight, consume)
}

// RenderBands reuses the session's bounded font and raster-asset cache while
// rendering consecutive borrowed bands. Its contract is otherwise RenderBands'.
func (s *RenderSession) RenderBands(ctx context.Context, document *d2scene.Document, options FrameOptions, bandHeight int, consume func(*image.NRGBA) error) error {
	if s == nil {
		return fmt.Errorf("d2raster: nil render session")
	}
	return renderBands(ctx, document, options, s, bandHeight, consume)
}

func renderBands(ctx context.Context, document *d2scene.Document, options FrameOptions, session *RenderSession, bandHeight int, consume func(*image.NRGBA) error) error {
	if bandHeight <= 0 {
		return fmt.Errorf("d2raster: band height must be positive")
	}
	if consume == nil {
		return fmt.Errorf("d2raster: nil band consumer")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	prepared, err := prepareWithSessionBands(ctx, document, options, session, bandHeight)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	scratch := &rasterScratch{
		regionFilters:     true,
		rasterizerEdges:   prepared.resources.rasterizerEdges,
		rasterizerBounded: true,
		offscreen:         offscreenBudget{limit: options.MaxOffscreenBytes},
		scanlineWork:      scanline.NewWorkBudget(prepared.resources.scanlineWork),
		scanlineWorkSet:   true,
	}
	defer scratch.releasePatternTiles()
	reservation, err := scratch.offscreen.reserveBytes(prepared.resources.rasterizerBytes, "scanline rasterizer working storage")
	if err != nil {
		return err
	}
	defer scratch.offscreen.release(reservation)
	for _, pattern := range prepared.patterns {
		if err := pattern.render(ctx, scratch); err != nil {
			return err
		}
	}

	bandHeight = min(bandHeight, prepared.height)
	canvas := image.NewRGBA(image.Rect(0, 0, prepared.width, bandHeight))
	for top := 0; top < prepared.height; {
		if err := ctx.Err(); err != nil {
			return err
		}
		bottom := top + min(bandHeight, prepared.height-top)
		canvas.Rect = image.Rect(0, top, prepared.width, bottom)
		canvas.Pix = canvas.Pix[:canvas.Stride*(bottom-top)]
		if prepared.background != nil && prepared.background.A == 0xff {
			fillOpaquePixels(canvas.Pix, *prepared.background)
		} else if prepared.background != nil && prepared.background.A != 0 {
			draw.Draw(canvas, canvas.Bounds(), image.NewUniform(*prepared.background), image.Point{}, draw.Src)
		} else {
			clear(canvas.Pix)
		}
		if err := renderNode(ctx, canvas, prepared.root, scratch); err != nil {
			return err
		}
		expectedLive := reservation + scratch.patternBytes
		if scratch.offscreen.live != expectedLive {
			return fmt.Errorf("d2raster: internal band offscreen reservation leak: %d bytes live, want %d", scratch.offscreen.live, expectedLive)
		}
		if scratch.offscreen.peak > prepared.resources.peakOffscreenBytes {
			return fmt.Errorf("d2raster: internal band offscreen peak %d exceeds preflight plan %d", scratch.offscreen.peak, prepared.resources.peakOffscreenBytes)
		}
		conversionBounds := canvas.Bounds()
		if prepared.background == nil || prepared.background.A == 0 {
			conversionBounds = prepared.root.bounds.Intersect(canvas.Bounds())
		}
		band, err := rgbaToNRGBAInPlaceBounds(ctx, canvas, conversionBounds, prepared.background != nil && prepared.background.A == 0xff)
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := consume(band); err != nil {
			return err
		}
		top = bottom
	}
	return ctx.Err()
}

func planBandResources(ctx context.Context, root *preparedNode, bounds image.Rectangle, patterns []*preparedPatternTile, options FrameOptions, bandHeight int) (renderResources, error) {
	// Patterns are rendered and retained once, not re-created for every band.
	resources, err := planRenderResourcesRegion(ctx, nil, image.Rectangle{}, patterns, options.MaxOffscreenBytes, true)
	if err != nil {
		return renderResources{}, err
	}
	pixelPeak := resources.peakOffscreenBytes - resources.rasterizerBytes
	var patternBytes int64
	for _, pattern := range patterns {
		var ok bool
		patternBytes, ok = checkedAdd(patternBytes, pattern.tileBytes)
		if !ok {
			return renderResources{}, fmt.Errorf("d2raster: pattern tile storage exceeds the int64 domain")
		}
	}
	for top := bounds.Min.Y; top < bounds.Max.Y; {
		if err := ctx.Err(); err != nil {
			return renderResources{}, err
		}
		bottom := top + min(bandHeight, bounds.Max.Y-top)
		band := image.Rect(bounds.Min.X, top, bounds.Max.X, bottom)
		bandResources, err := planRenderResourcesRegion(ctx, root, band, nil, options.MaxOffscreenBytes, true)
		if err != nil {
			return renderResources{}, err
		}
		bandPixelPeak, ok := checkedAdd(patternBytes, bandResources.peakOffscreenBytes-bandResources.rasterizerBytes)
		if !ok {
			return renderResources{}, fmt.Errorf("d2raster: peak offscreen pixel storage exceeds the int64 domain")
		}
		pixelPeak = maxInt64(pixelPeak, bandPixelPeak)
		mergeRasterizerRequirements(&resources, bandResources)
		if err := addScanlineWork(&resources, bandResources.scanlineWork); err != nil {
			return renderResources{}, err
		}
		if err := addResourceWork(&resources, bandResources.evenOddClipWork); err != nil {
			return renderResources{}, err
		}
		resources.peakOffscreenBytes, ok = checkedAdd(pixelPeak, resources.rasterizerBytes)
		if !ok {
			return renderResources{}, fmt.Errorf("d2raster: peak offscreen pixel storage exceeds the int64 domain")
		}
		// Reject as soon as monotonic totals exceed their limits rather than
		// continuing to plan a potentially very tall rejected image.
		if resources.peakOffscreenBytes > options.MaxOffscreenBytes {
			return renderResources{}, fmt.Errorf("d2raster: peak offscreen pixel storage %d bytes exceeds limit %d", resources.peakOffscreenBytes, options.MaxOffscreenBytes)
		}
		if resources.evenOddClipWork > options.MaxEvenOddClipWork {
			return renderResources{}, fmt.Errorf("d2raster: even-odd clip work %d exceeds limit %d", resources.evenOddClipWork, options.MaxEvenOddClipWork)
		}
		if resources.scanlineWork > options.MaxScanlineWork {
			return renderResources{}, fmt.Errorf("d2raster: scanline work %d exceeds limit %d", resources.scanlineWork, options.MaxScanlineWork)
		}
		top = bottom
	}
	return resources, nil
}

// setBandReferenceBounds records the canvas origins used by an ordinary full
// render. Scanline inputs are rounded to float32 relative to those same origins
// before translating into each band, avoiding changes to antialiased pixels.
func setBandReferenceBounds(ctx context.Context, node *preparedNode, dst image.Rectangle) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if node == nil || node.opacity == 0 {
		return nil
	}
	visible := node.bounds.Intersect(dst)
	if visible.Empty() {
		return nil
	}
	paintBounds := dst
	if len(node.filters) != 0 {
		paintBounds = node.contentBounds
	} else if node.isolated || node.opacity < 1 || node.blend != d2scene.BlendNormal || node.clip != nil || node.mask != nil {
		paintBounds = visible
	}
	if node.primitive != nil {
		node.primitive.referenceBounds = paintBounds
		if node.primitive.stroke != nil {
			node.primitive.stroke.referenceBounds = paintBounds
		}
		if node.primitive.vector != nil {
			if err := setBandReferenceBounds(ctx, node.primitive.vector, paintBounds); err != nil {
				return err
			}
		}
	}
	for _, child := range node.children {
		if err := setBandReferenceBounds(ctx, child, paintBounds); err != nil {
			return err
		}
	}
	if node.clip != nil {
		node.clip.referenceBounds = visible
	}
	if node.mask != nil {
		return setBandReferenceBounds(ctx, node.mask.root, visible)
	}
	return nil
}

func setRasterizerReference(rasterizer *scanline.Rasterizer, actualOrigin, referenceOrigin image.Point, enabled bool) image.Point {
	if !enabled {
		return actualOrigin
	}
	rasterizer.SetOriginOffset(float64(referenceOrigin.X)-float64(actualOrigin.X), float64(referenceOrigin.Y)-float64(actualOrigin.Y))
	return referenceOrigin
}
