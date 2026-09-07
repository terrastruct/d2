package d2cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"html"
	"image"
	"image/color"
	"math"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/d2lang/d2/d2plugin"
	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/d2renderers/d2raster"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/d2scenebuild"
	"github.com/d2lang/d2/d2renderers/d2svg"
	"github.com/d2lang/d2/d2renderers/d2svgimport"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/imageasset"
	"github.com/d2lang/d2/lib/xgif"
)

func validateRasterPostProcessor(ctx context.Context, plugin d2plugin.Plugin, sourceSVG []byte) error {
	postProcessor, ok := plugin.(d2plugin.PostProcessor)
	if !ok {
		return nil
	}
	// A PostProcessor may mutate its argument and return the same slice. Give it
	// an owned input so the pristine source remains a trustworthy comparison.
	processed, err := postProcessor.PostProcess(ctx, bytes.Clone(sourceSVG))
	if err != nil {
		return fmt.Errorf("raster postprocessor validation: %w", err)
	}
	if !bytes.Equal(sourceSVG, processed) {
		return fmt.Errorf("raster export cannot apply SVG changes made by the layout plugin postprocessor; disable the postprocessor for PNG, GIF, PDF, and PPTX output")
	}
	return nil
}

func renderRasterSVG(ctx context.Context, plugin d2plugin.Plugin, diagram *d2target.Diagram, opts d2svg.RenderOpts, returnSVG, checkPostProcessor bool) ([]byte, error) {
	_, checksPostProcessor := plugin.(d2plugin.PostProcessor)
	if !returnSVG && (!checkPostProcessor || !checksPostProcessor) {
		return nil, nil
	}
	sourceSVG, err := d2svg.Render(diagram, &opts)
	if err != nil {
		return nil, err
	}
	if checkPostProcessor && checksPostProcessor {
		if err := validateRasterPostProcessor(ctx, plugin, sourceSVG); err != nil {
			if returnSVG {
				return sourceSVG, err
			}
			return nil, err
		}
	}
	if returnSVG {
		return sourceSVG, nil
	}
	return nil, nil
}

const (
	// These production safety ceilings bound raster work independently
	// of the caller's input source.
	// Admit long, narrow diagrams while rasterMaxPixels bounds complete-frame
	// allocations. Streaming PNG uses these dimensions with a bounded strip.
	rasterMaxDimension                = 32_768
	rasterMaxPixels             int64 = 64 * 1024 * 1024
	rasterMaxNodes                    = 1_000_000
	rasterMaxDepth                    = 1_024
	rasterMaxPathCommands             = 10_000_000
	rasterMaxTextRunesPerRun          = 100_000
	rasterMaxFontFacesPerText         = fontAssetReserve
	rasterMaxTextCoverageChecks int64 = 10_000_000
	rasterMaxTextShapingRuns          = 100_000
	rasterMaxAnimationTracks          = 1_000_000
	rasterMaxAnimationKeyframes       = 10_000_000
	rasterMaxOffscreenBytes     int64 = 512 * 1024 * 1024
	rasterMaxEvenOddClipWork    int64 = 250_000_000
	// Scanline work counts conservative edge-row visits, horizontal crossings,
	// and painted/cleared spans across the complete export.
	rasterMaxScanlineWork int64 = 4_000_000_000

	// GIF rendering keeps only indexed-color frames between samples. Bound its
	// schedule, retained pixels, transient quantizer work, and encoded output
	// independently of the one-frame renderer ceilings.
	gifMaxFrames            = 1_800
	gifMaxBoardNodes        = 4_096
	gifMaxFramePixels int64 = 2 * 1024 * 1024
	// Median-cut quantization has substantial non-cancellable scratch storage.
	// Render and quantize one full-color frame at a time, while retaining only
	// its smaller indexed-color result.
	gifMaxFrameOffscreenBytes int64 = 128 * 1024 * 1024
	// Animated SVG clips repeat their work for every sample. This GIF-specific
	// aggregate admits complex animation while retaining a total-work ceiling;
	// the smaller static export allowance remains unchanged.
	gifMaxEvenOddClipWork int64 = 768 * 1024 * 1024
	gifMaxScanlineWork    int64 = 8_000_000_000
	// GIF normalizes every frame to max(width) x max(height). This ceiling is
	// charged against the retained indexed-color canvas, not the smaller source
	// rectangles. The encoder output has a separate bound because compressed
	// bytes and buffer capacity are not represented by this pixel accounting.
	gifMaxNormalizedPixels int64 = 160 * 1024 * 1024
	gifMaxEncodedBytes     int64 = 64 * 1024 * 1024
	// The watch preview is generated from trusted d2svg output, but it can still
	// multiply one asset across many image elements. Bound both the input scan
	// and the fully expanded data-URI output before allocating the result.
	rasterPreviewMaxSourceBytes           = 32 * 1024 * 1024
	rasterPreviewMaxImageReferences       = 4_096
	rasterPreviewMaxOutputBytes     int64 = 192 * 1024 * 1024

	rasterMaxAssets             = 4_096
	rasterMaxAssetBytes   int64 = 512 * 1024 * 1024
	rasterMaxDecodedBytes int64 = 512 * 1024 * 1024
	// Document.Assets also retains bundled font faces. Keep image resolution
	// below the renderer's all-asset ceiling so a boundary-valid image set does
	// not fail only after fonts have been added.
	fontAssetReserve                 = 64
	fontAssetByteReserve       int64 = 64 * 1024 * 1024
	fontMaxRunesPerText              = rasterMaxTextRunesPerRun
	fontMaxTotalRunes                = rasterMaxPathCommands
	fontMaxCoverageChecks      int64 = 50_000_000
	fontSearchDirectoryEntries       = 20_000
	fontSearchFiles                  = 4_096
	fontSearchFaces                  = 8_192
	fontSearchRequestedRunes         = fontMaxRunesPerText
	fontSearchCoverageChecks         = 10_000_000
	fontSearchFileBytes        int64 = 32 * 1024 * 1024
	fontSearchScannedBytes     int64 = 256 * 1024 * 1024
	// Resolvers return owned bytes for each style bucket, including content and
	// face pairs already retained by an earlier bucket. Bound cumulative
	// resolver output separately from retained scene bytes.
	fontSearchResolvedBytes int64 = 2 * fontAssetByteReserve
	// The export-scoped resolver retains one immutable copy of the bundled
	// color face and shares it across every board scene. Keep that copy under
	// an independent bound in addition to the downstream operation ceiling.
	fontBundledCopyBytes     int64 = 8 * 1024 * 1024
	fontBundledResolvedBytes       = fontBundledCopyBytes + fontSearchResolvedBytes
	gifMaxAssets                   = 1_024
	gifMaxAssetBytes         int64 = 128 * 1024 * 1024
	gifMaxDecodedAssetBytes  int64 = 128 * 1024 * 1024
	// A GIF render session retains the full immutable asset-key memo (up to
	// 128 MiB), decoded raster pixels (up to 128 MiB), parsed font sources (up
	// to the 64 MiB reserve), and bounded entry metadata. Keeping this above
	// that arithmetic ceiling makes every repeated frame a cache hit rather
	// than silently hashing, parsing, or decoding the same assets again.
	gifRenderCacheBytes   int64 = 321 * 1024 * 1024
	gifImageAssetMaxCount       = gifMaxAssets - fontAssetReserve
	gifImageEncodedBytes  int64 = gifMaxAssetBytes - fontAssetByteReserve
	gifImageDecodedBytes  int64 = gifMaxDecodedAssetBytes

	imageAssetMaxBytes                  int64 = 32 * 1024 * 1024
	imageAssetMaxCount                        = rasterMaxAssets - fontAssetReserve
	imageAssetMaxCumulativeEncodedBytes int64 = rasterMaxAssetBytes - fontAssetByteReserve
	imageAssetMaxCumulativeDecodedBytes int64 = rasterMaxDecodedBytes
	// Paged exports render each board once, so earlier per-board key memos may be
	// evicted. This cap still retains the complete 512 MiB decoded-raster
	// budget, 64 MiB of parsed font sources, one maximum-size 32 MiB key memo,
	// and charged entry metadata without an admission skip.
	pagedRenderCacheBytes int64 = 612 * 1024 * 1024
	assetCacheMaxBytes    int64 = 512 * 1024 * 1024
	assetCacheNamespace         = "d2cli/imageasset/v1/default-http"

	// Static raster exports paint at most two appendix items (tooltip and
	// link) per typed shape or connection region; each Markdown link adds at
	// most one title row. Keep the metadata strings and resulting scene nodes
	// bounded even for PNG/GIF callers that do not consume typed PDF/PPTX
	// annotations.
	rasterMaxLinkRegions     = 4_096
	rasterMaxLinkStringBytes = 1 * 1024 * 1024

	// Measured from the checked-in icon and frozen MathJax fixtures.
	// The largest distinct board used 202 SVG elements and 1,786 path commands;
	// the largest formula used 122 elements and 1,943 commands. These ceilings
	// retain user-SVG headroom while bounding the work for each import.
	svgMaxBytes              = 64 * 1024
	svgMaxDepth              = 32
	svgMaxElements           = 256
	svgMaxAttributes         = 512
	svgMaxAttributeBytes     = 64 * 1024
	svgMaxPathCommands       = 4_096
	svgMaxTransformFunctions = 128
	svgMaxUseDepth           = 8
	svgMaxResources          = 64

	svgDocumentMaxSourceBytes          = 1 * 1024 * 1024
	svgDocumentMaxElements             = 4_096
	svgDocumentMaxAttributes           = 8_192
	svgDocumentMaxAttributeBytes       = 1 * 1024 * 1024
	svgDocumentMaxPathCommands         = 65_536
	svgDocumentMaxTransformFunctions   = 2_048
	svgDocumentMaxDeclaredResources    = 1_024
	svgDocumentMaxExpandedUseInstances = 1_024

	assetHTTPMaxIdleConns          = 32
	assetHTTPMaxIdleConnsPerHost   = 16
	assetHTTPMaxConnsPerHost       = 32
	assetHTTPMaxResponseHeaderSize = 64 << 10
)

var (
	assetCacheOnce sync.Once
	assetCache     *imageasset.MemoryCache
	assetCacheErr  error

	// A process-owned client lets watch mode and repeated uncached renders reuse
	// connections instead of leaving one idle pool behind per Resolver.
	assetHTTPTransport = newAssetHTTPTransport()
	assetHTTPClient    = &http.Client{
		Transport: assetHTTPTransport,
		Timeout:   time.Minute,
	}
)

func newAssetHTTPTransport() *http.Transport {
	if base, ok := http.DefaultTransport.(*http.Transport); ok {
		transport := base.Clone()
		transport.MaxIdleConns = assetHTTPMaxIdleConns
		transport.MaxIdleConnsPerHost = assetHTTPMaxIdleConnsPerHost
		transport.MaxConnsPerHost = assetHTTPMaxConnsPerHost
		transport.MaxResponseHeaderBytes = assetHTTPMaxResponseHeaderSize
		return transport
	}
	return &http.Transport{
		Proxy:                  http.ProxyFromEnvironment,
		DialContext:            (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:      true,
		MaxIdleConns:           assetHTTPMaxIdleConns,
		MaxIdleConnsPerHost:    assetHTTPMaxIdleConnsPerHost,
		MaxConnsPerHost:        assetHTTPMaxConnsPerHost,
		IdleConnTimeout:        90 * time.Second,
		TLSHandshakeTimeout:    10 * time.Second,
		ExpectContinueTimeout:  1 * time.Second,
		MaxResponseHeaderBytes: assetHTTPMaxResponseHeaderSize,
	}
}

func renderPNGWithEncoder(ctx context.Context, inputPath string, cacheImages bool, diagram *d2target.Diagram, opts d2svg.RenderOpts, encoder *rasterPNGEncoder) ([]byte, error) {
	document, err := buildScene(ctx, inputPath, cacheImages, diagram, opts)
	if err != nil {
		return nil, err
	}
	frame, err := d2raster.Render(ctx, document, rasterFrameOptions(2, 0))
	if err != nil {
		return nil, err
	}
	var localEncoder rasterPNGEncoder
	if encoder == nil {
		encoder = &localEncoder
		defer localEncoder.close()
	}
	return encoder.encode(ctx, frame)
}

func buildScene(ctx context.Context, inputPath string, cacheImages bool, diagram *d2target.Diagram, opts d2svg.RenderOpts) (*d2scene.Document, error) {
	if diagram == nil {
		return nil, fmt.Errorf("raster export: nil diagram")
	}
	assetOptions, err := sceneAssetOptions(inputPath, cacheImages)
	if err != nil {
		return nil, err
	}
	return buildSceneWithAssets(ctx, diagram, opts, assetOptions)
}

func buildSceneWithAssets(ctx context.Context, diagram *d2target.Diagram, opts d2svg.RenderOpts, assetOptions *d2scenebuild.AssetOptions) (*d2scene.Document, error) {
	fontOptions, err := newFontFallbackOptions(1)
	if err != nil {
		return nil, err
	}
	return buildSceneWithResourcesAndLinks(ctx, diagram, opts, assetOptions, fontOptions, d2scenebuild.LinkBudget{
		MaxRegions: rasterMaxLinkRegions, MaxStringBytes: rasterMaxLinkStringBytes,
	}, sceneAdmissionLimits{
		maxNodes: rasterMaxNodes, maxPathCommands: rasterMaxPathCommands,
	})
}

func newFontFallbackOptions(totalBoards int) (*d2scenebuild.FontFallbackOptions, error) {
	if totalBoards <= 0 {
		return nil, fmt.Errorf("raster export: font fallback budget requires a positive board count")
	}
	systemFontResolver, err := d2fonts.NewSystemFallbackResolver(d2fonts.SystemFallbackLimits{
		MaxDirectoryEntries: fontSearchDirectoryEntries,
		MaxFiles:            fontSearchFiles,
		MaxFaces:            fontSearchFaces,
		MaxRequestedRunes:   fontSearchRequestedRunes,
		MaxCoverageChecks:   fontSearchCoverageChecks,
		MaxFileBytes:        fontSearchFileBytes,
		MaxScannedBytes:     fontSearchScannedBytes,
		MaxResolvedBytes:    fontSearchResolvedBytes,
	})
	if err != nil {
		return nil, fmt.Errorf("raster export: initialize font fallback resolver: %w", err)
	}
	fontResolver, err := d2fonts.NewBundledFallbackResolver(systemFontResolver, d2fonts.BundledFallbackLimits{
		MaxRequestedRunes: fontSearchRequestedRunes,
		MaxBundledBytes:   fontBundledCopyBytes,
		MaxResolvedBytes:  fontBundledResolvedBytes,
	})
	if err != nil {
		return nil, fmt.Errorf("raster export: initialize bundled font fallback resolver: %w", err)
	}
	return &d2scenebuild.FontFallbackOptions{
		Resolver: fontResolver, MaxAssets: fontAssetReserve, MaxBytes: fontAssetByteReserve,
		MaxRunesPerText:     fontMaxRunesPerText,
		MaxTotalRunes:       max(1, fontMaxTotalRunes/totalBoards),
		MaxCoverageChecks:   max(int64(1), fontMaxCoverageChecks/int64(totalBoards)),
		MaxFontFacesPerText: rasterMaxFontFacesPerText,
		MaxShapingRuns:      max(1, rasterMaxTextShapingRuns/totalBoards),
		MaxShapedGlyphs:     max(1, rasterMaxPathCommands/totalBoards),
	}, nil
}

type sceneAdmissionLimits struct {
	maxNodes        int
	maxPathCommands int
}

func sceneAdmissionLimitsForFrame(options d2raster.FrameOptions) sceneAdmissionLimits {
	return sceneAdmissionLimits{maxNodes: options.MaxNodes, maxPathCommands: options.MaxPathCommands}
}

func buildSceneWithResourcesAndLinks(ctx context.Context, diagram *d2target.Diagram, opts d2svg.RenderOpts, assetOptions *d2scenebuild.AssetOptions, fontOptions *d2scenebuild.FontFallbackOptions, linkBudget d2scenebuild.LinkBudget, limits sceneAdmissionLimits) (*d2scene.Document, error) {
	if diagram == nil {
		return nil, fmt.Errorf("raster export: nil diagram")
	}
	// render() walks boards independently. Keep the scene for this board free
	// of child-board metadata so d2scenebuild does not mistake it for a request
	// to compose multiple boards into one frame.
	board := *diagram
	board.Layers = nil
	board.Scenarios = nil
	board.Steps = nil
	sketch := opts.Sketch != nil && *opts.Sketch
	document, err := d2scenebuild.Build(ctx, &board, d2scenebuild.Options{
		Pad:             opts.Pad,
		Scale:           opts.Scale,
		Center:          opts.Center,
		ThemeID:         opts.ThemeID,
		ThemeOverrides:  opts.ThemeOverrides,
		MaxNodes:        limits.maxNodes,
		MaxPathCommands: limits.maxPathCommands,
		Sketch:          sketch,
		SketchBudget: d2scenebuild.SketchBudget{
			MaxOperationSets: rasterMaxNodes,
			MaxOperations:    rasterMaxPathCommands,
			MaxPathCommands:  rasterMaxPathCommands,
		},
		Assets:     assetOptions,
		Fonts:      fontOptions,
		LinkBudget: linkBudget,
		Appendix:   true,
	})
	if err != nil {
		return nil, err
	}
	return document, nil
}

func rasterFrameOptions(scale float64, timestamp time.Duration) d2raster.FrameOptions {
	return d2raster.FrameOptions{
		Scale:                 scale,
		Time:                  timestamp,
		Background:            color.White,
		MaxWidth:              rasterMaxDimension,
		MaxHeight:             rasterMaxDimension,
		MaxPixels:             rasterMaxPixels,
		MaxNodes:              rasterMaxNodes,
		MaxDepth:              rasterMaxDepth,
		MaxPathCommands:       rasterMaxPathCommands,
		MaxTextRunesPerRun:    rasterMaxTextRunesPerRun,
		MaxFontFacesPerText:   rasterMaxFontFacesPerText,
		MaxTextCoverageChecks: rasterMaxTextCoverageChecks,
		MaxTextShapingRuns:    rasterMaxTextShapingRuns,
		MaxAnimationTracks:    rasterMaxAnimationTracks,
		MaxAnimationKeyframes: rasterMaxAnimationKeyframes,
		MaxAssets:             rasterMaxAssets,
		MaxAssetBytes:         rasterMaxAssetBytes,
		MaxDecodedAssetBytes:  rasterMaxDecodedBytes,
		MaxImportDepth:        rasterMaxDepth,
		MaxOffscreenBytes:     rasterMaxOffscreenBytes,
		MaxEvenOddClipWork:    rasterMaxEvenOddClipWork,
		MaxScanlineWork:       rasterMaxScanlineWork,
	}
}

func gifFrameOptions(timestamp time.Duration, totalFrames int) d2raster.FrameOptions {
	options := rasterFrameOptions(1, timestamp)
	options.MaxPixels = gifMaxFramePixels
	// Render preflight repeats for every frame. Divide aggregate work ceilings
	// across the known schedule so many tiny frames cannot multiply a full
	// one-document allowance.
	divisor := max(1, totalFrames)
	options.MaxNodes = max(1, rasterMaxNodes/divisor)
	options.MaxPathCommands = max(1, rasterMaxPathCommands/divisor)
	options.MaxTextCoverageChecks = max(int64(1), rasterMaxTextCoverageChecks/int64(divisor))
	options.MaxTextShapingRuns = max(1, rasterMaxTextShapingRuns/divisor)
	options.MaxAnimationTracks = max(1, rasterMaxAnimationTracks/divisor)
	options.MaxAnimationKeyframes = max(1, rasterMaxAnimationKeyframes/divisor)
	options.MaxEvenOddClipWork = max(int64(1), gifMaxEvenOddClipWork/int64(divisor))
	options.MaxScanlineWork = max(int64(1), gifMaxScanlineWork/int64(divisor))
	options.MaxAssets = gifMaxAssets
	options.MaxAssetBytes = gifMaxAssetBytes
	options.MaxDecodedAssetBytes = gifMaxDecodedAssetBytes
	// GIF frames render serially, so each receives this fixed offscreen ceiling.
	options.MaxOffscreenBytes = gifMaxFrameOffscreenBytes
	return options
}

type gifFrameRenderer func(context.Context, int, time.Duration, d2raster.FrameOptions, func(image.Image) error) error

type gifFrameConsumer func(frameIndex int, frame image.Image) error

type gifBoardPlanConsumer func(totalBoards, totalFrames int, bounds image.Rectangle) error

// renderGIFBoardFrames paints and consumes one frame at a time. The consumer
// can quantize and release each full-color image before the next render begins.
func renderGIFBoardFrames(
	ctx context.Context,
	firstFrame, frameCount, totalFrames int,
	render gifFrameRenderer,
	consume gifFrameConsumer,
) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if firstFrame < 0 || frameCount <= 0 || totalFrames <= 0 || firstFrame > totalFrames || frameCount > totalFrames-firstFrame {
		return fmt.Errorf("GIF frame schedule must contain a positive board count within the total frame count")
	}
	if render == nil {
		return fmt.Errorf("GIF frame renderer is nil")
	}
	if consume == nil {
		return fmt.Errorf("GIF frame consumer is nil")
	}
	for offset := 0; offset < frameCount; offset++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		frameIndex := firstFrame + offset
		timestamp, err := xgif.AnimationFrameTime(frameIndex)
		if err != nil {
			return err
		}
		emitted := false
		var emitErr error
		err = render(ctx, frameIndex, timestamp, gifFrameOptions(timestamp, totalFrames), func(frame image.Image) error {
			if emitted {
				emitErr = fmt.Errorf("GIF frame %d was rendered more than once", frameIndex)
				return emitErr
			}
			emitted = true
			if frame == nil {
				emitErr = fmt.Errorf("GIF frame %d was not rendered", frameIndex)
				return emitErr
			}
			emitErr = consume(frameIndex, frame)
			return emitErr
		})
		if emitErr != nil {
			return emitErr
		}
		if err != nil {
			return fmt.Errorf("GIF frame %d: %w", frameIndex, err)
		}
		if !emitted {
			return fmt.Errorf("GIF frame %d was not rendered", frameIndex)
		}
	}
	return nil
}

func renderGIF(ctx context.Context, plugin d2plugin.Plugin, inputPath string, cacheImages bool, diagram *d2target.Diagram, opts d2svg.RenderOpts, intervalMs int, wantPreview bool) (encoded, previewSVG []byte, err error) {
	session, err := newGIFRenderSession()
	if err != nil {
		return nil, nil, err
	}
	var workspace d2raster.RenderWorkspace
	defer workspace.Reset()
	var quantizationWorkspace xgif.OpaqueQuantizationWorkspace
	palettedFrames := make([]*image.Paletted, 0)
	var incrementalEncoder *xgif.OpaquePalettedAnimationEncoder
	var incrementalBounds image.Rectangle
	summary, err := renderGIFWithSession(
		ctx, plugin, inputPath, cacheImages, diagram, opts, intervalMs, session, &workspace, wantPreview,
		func(totalBoards, totalFrames int, bounds image.Rectangle) error {
			if totalBoards != 1 {
				return nil
			}
			encoder, encoderErr := xgif.NewOpaquePalettedAnimationEncoder(
				ctx, bounds.Dx(), bounds.Dy(), totalFrames, intervalMs, gifMaxEncodedBytes,
			)
			if encoderErr != nil {
				return encoderErr
			}
			incrementalEncoder = encoder
			incrementalBounds = bounds
			return nil
		},
		func(frameIndex int, frame image.Image) error {
			if incrementalEncoder != nil {
				var consumeErr error
				quantizeErr := quantizationWorkspace.Quantize(ctx, frame, func(paletted *image.Paletted) error {
					normalized, normalizeErr := xgif.NormalizePalettedImage(ctx, paletted, incrementalBounds.Dx(), incrementalBounds.Dy())
					if normalizeErr != nil {
						consumeErr = fmt.Errorf("GIF frame %d normalization: %w", frameIndex, normalizeErr)
						return consumeErr
					}
					if encodeErr := incrementalEncoder.WriteFrame(normalized); encodeErr != nil {
						consumeErr = fmt.Errorf("GIF frame %d encoding: %w", frameIndex, encodeErr)
						return consumeErr
					}
					return nil
				})
				if consumeErr != nil {
					return consumeErr
				}
				if quantizeErr != nil {
					return fmt.Errorf("GIF frame %d quantization: %w", frameIndex, quantizeErr)
				}
				return nil
			}
			paletted, quantizeErr := xgif.QuantizeImage(ctx, frame)
			if quantizeErr != nil {
				return fmt.Errorf("GIF frame %d quantization: %w", frameIndex, quantizeErr)
			}
			palettedFrames = append(palettedFrames, paletted)
			return nil
		},
	)
	if err != nil {
		return nil, nil, err
	}
	if incrementalEncoder != nil {
		encoded, err = incrementalEncoder.Finish()
		if err != nil {
			return nil, nil, err
		}
		return encoded, summary.previewSVG, nil
	}
	if len(palettedFrames) != summary.totalFrames {
		return nil, nil, fmt.Errorf("GIF encoded frame count %d differs from render schedule %d", len(palettedFrames), summary.totalFrames)
	}
	encoded, err = xgif.AnimateCenteredOpaquePalettedImagesWithLimit(
		ctx, palettedFrames, summary.maxWidth, summary.maxHeight, intervalMs, gifMaxEncodedBytes,
	)
	if err != nil {
		return nil, nil, err
	}
	return encoded, summary.previewSVG, nil
}

func newGIFRenderSession() (*d2raster.RenderSession, error) {
	session, err := d2raster.NewRenderSession(d2raster.RenderSessionOptions{
		MaxCacheEntries:    gifMaxAssets,
		MaxCacheBytes:      gifRenderCacheBytes,
		MaxConcurrentLoads: 1,
	})
	if err != nil {
		return nil, fmt.Errorf("GIF render session: %w", err)
	}
	return session, nil
}

type gifRenderSummary struct {
	previewSVG          []byte
	totalFrames         int
	maxWidth, maxHeight int
}

func renderGIFWithSession(
	ctx context.Context,
	plugin d2plugin.Plugin,
	inputPath string,
	cacheImages bool,
	diagram *d2target.Diagram,
	opts d2svg.RenderOpts,
	intervalMs int,
	session *d2raster.RenderSession,
	workspace *d2raster.RenderWorkspace,
	wantPreview bool,
	plan gifBoardPlanConsumer,
	consume gifFrameConsumer,
) (gifRenderSummary, error) {
	var summary gifRenderSummary
	if ctx == nil {
		ctx = context.Background()
	}
	if session == nil {
		return summary, fmt.Errorf("GIF requires a render session")
	}
	if consume == nil {
		return summary, fmt.Errorf("GIF frame consumer is nil")
	}
	framesPerBoard, err := xgif.AnimationFrameCount(intervalMs)
	if err != nil {
		return summary, err
	}
	if framesPerBoard > gifMaxFrames {
		return summary, fmt.Errorf("GIF requires %d frames per board, exceeding the %d-frame limit", framesPerBoard, gifMaxFrames)
	}
	boards, err := collectGIFBoards(ctx, diagram, gifMaxFrames/framesPerBoard)
	if err != nil {
		return summary, err
	}
	if len(boards) == 0 {
		return summary, fmt.Errorf("GIF animation requires at least one renderable board")
	}
	assetOptions, err := gifSceneAssetOptions(inputPath, cacheImages, len(boards))
	if err != nil {
		return summary, err
	}
	fontOptions, err := newFontFallbackOptions(len(boards))
	if err != nil {
		return summary, err
	}
	linkBudget, err := gifLinkBudget(len(boards))
	if err != nil {
		return summary, err
	}
	totalFrames := len(boards) * framesPerBoard
	summary.totalFrames = totalFrames
	renderedFrameCount := 0
	for boardIndex, board := range boards {
		if err := ctx.Err(); err != nil {
			return summary, err
		}
		boardOpts := rasterRenderOptions(opts)
		needsPreview := wantPreview && board == diagram && !diagram.IsFolderOnly
		sourceSVG, err := renderRasterSVG(ctx, plugin, board, boardOpts, needsPreview, true)
		if err != nil {
			return summary, fmt.Errorf("GIF board %d: %w", boardIndex, err)
		}
		if needsPreview {
			summary.previewSVG = sourceSVG
		}
		frameOptions := gifFrameOptions(0, totalFrames)
		document, err := buildSceneWithResourcesAndLinks(ctx, board, boardOpts, assetOptions, fontOptions, linkBudget, sceneAdmissionLimitsForFrame(frameOptions))
		if err != nil {
			return summary, fmt.Errorf("GIF board %d scene: %w", boardIndex, err)
		}
		plannedBounds, err := gifDocumentFrameBounds(document)
		if err != nil {
			return summary, fmt.Errorf("GIF board %d: %w", boardIndex, err)
		}
		summary.maxWidth = max(summary.maxWidth, plannedBounds.Dx())
		summary.maxHeight = max(summary.maxHeight, plannedBounds.Dy())
		if err := validateGIFNormalizedFootprint(summary.maxWidth, summary.maxHeight, totalFrames); err != nil {
			return summary, fmt.Errorf("GIF board %d: %w", boardIndex, err)
		}
		if plan != nil {
			if err := plan(len(boards), totalFrames, plannedBounds); err != nil {
				return summary, fmt.Errorf("GIF board %d: %w", boardIndex, err)
			}
		}
		err = renderGIFBoardFrames(
			ctx, 0, framesPerBoard, totalFrames,
			func(renderCtx context.Context, _ int, _ time.Duration, frameOptions d2raster.FrameOptions, emit func(image.Image) error) error {
				if workspace == nil {
					frame, renderErr := session.Render(renderCtx, document, frameOptions)
					if renderErr != nil {
						return renderErr
					}
					return emit(frame)
				}
				return workspace.Render(renderCtx, session, document, frameOptions, func(frame *image.NRGBA) error {
					return emit(frame)
				})
			},
			func(frameIndex int, frame image.Image) error {
				if frame.Bounds() != plannedBounds {
					return fmt.Errorf("GIF frame %d bounds %v differ from planned bounds %v", frameIndex, frame.Bounds(), plannedBounds)
				}
				if err := validateRenderSession("GIF", session); err != nil {
					return err
				}
				if err := consume(frameIndex, frame); err != nil {
					return err
				}
				renderedFrameCount++
				return nil
			},
		)
		if err != nil {
			return summary, fmt.Errorf("GIF board %d: %w", boardIndex, err)
		}
	}
	if renderedFrameCount != totalFrames {
		return summary, fmt.Errorf("GIF rendered frame count %d differs from schedule %d", renderedFrameCount, totalFrames)
	}
	if len(summary.previewSVG) != 0 {
		summary.previewSVG, err = bundleRasterPreview(ctx, assetOptions.Resolver, summary.previewSVG)
		if err != nil {
			return summary, fmt.Errorf("GIF watch preview: %w", err)
		}
	}
	return summary, nil
}

func gifDocumentFrameBounds(document *d2scene.Document) (image.Rectangle, error) {
	if document == nil {
		return image.Rectangle{}, fmt.Errorf("GIF requires a scene document")
	}
	widthFloat, heightFloat := document.LogicalWidth, document.LogicalHeight
	if math.IsNaN(widthFloat) || math.IsInf(widthFloat, 0) || widthFloat <= 0 ||
		math.IsNaN(heightFloat) || math.IsInf(heightFloat, 0) || heightFloat <= 0 {
		return image.Rectangle{}, fmt.Errorf("GIF scene has invalid logical dimensions")
	}
	widthCeil, heightCeil := math.Ceil(widthFloat), math.Ceil(heightFloat)
	if widthCeil > rasterMaxDimension || heightCeil > rasterMaxDimension {
		return image.Rectangle{}, fmt.Errorf("GIF scene dimensions %.0fx%.0f exceed the %d-pixel dimension limit", widthCeil, heightCeil, rasterMaxDimension)
	}
	width, height := int(widthCeil), int(heightCeil)
	if int64(width) > gifMaxFramePixels/int64(height) {
		return image.Rectangle{}, fmt.Errorf("GIF frame pixels exceed the %d-pixel limit", gifMaxFramePixels)
	}
	return image.Rect(0, 0, width, height), nil
}

func validateGIFNormalizedFootprint(width, height, totalFrames int) error {
	if width <= 0 || height <= 0 || totalFrames <= 0 {
		return fmt.Errorf("GIF normalized frame footprint requires positive dimensions and frame count")
	}
	maxPixelsPerFrame := gifMaxNormalizedPixels / int64(totalFrames)
	if int64(width) > maxPixelsPerFrame/int64(height) {
		return fmt.Errorf("GIF normalized frame footprint exceeds the %d cumulative-pixel limit", gifMaxNormalizedPixels)
	}
	return nil
}

func gifLinkBudget(totalBoards int) (d2scenebuild.LinkBudget, error) {
	if totalBoards <= 0 {
		return d2scenebuild.LinkBudget{}, fmt.Errorf("GIF requires a positive board count for link budgets")
	}
	return d2scenebuild.LinkBudget{
		MaxRegions:     max(1, rasterMaxLinkRegions/totalBoards),
		MaxStringBytes: max(1, rasterMaxLinkStringBytes/totalBoards),
	}, nil
}

func validateRenderSession(export string, session *d2raster.RenderSession) error {
	if session == nil {
		return fmt.Errorf("%s requires a render session", export)
	}
	stats := session.Stats()
	if stats.SkippedOversize != 0 || stats.MemoSkipped != 0 {
		return fmt.Errorf(
			"%s render cache rejected bounded state (asset skips %d, memo skips %d, retained bytes %d); reduce asset size or count",
			export, stats.SkippedOversize, stats.MemoSkipped, stats.RetainedBytes,
		)
	}
	return nil
}

var rasterImageHrefPrefix = []byte(`<image href="`)

type rasterPreviewReference struct {
	start  int
	end    int
	source string
}

type rasterPreviewResource struct {
	resource *imageasset.Resource
	uriBytes int
}

// bundleRasterPreview embeds external image references from the same
// resolver session used to build the frames. Resolver memoization is
// important here: local files and remote responses cannot change between the
// rendered GIF and its watch preview without adding a second I/O path.
func bundleRasterPreview(ctx context.Context, resolver *imageasset.Resolver, sourceSVG []byte) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if int64(len(sourceSVG)) > rasterPreviewMaxSourceBytes {
		return nil, fmt.Errorf("source bytes %d exceed limit %d", len(sourceSVG), rasterPreviewMaxSourceBytes)
	}
	if resolver == nil {
		return nil, fmt.Errorf("image resolver is required")
	}

	references := make([]rasterPreviewReference, 0)
	searchAt := 0
	imageCount := 0
	for {
		relative := bytes.Index(sourceSVG[searchAt:], rasterImageHrefPrefix)
		if relative < 0 {
			break
		}
		start := searchAt + relative + len(rasterImageHrefPrefix)
		relativeEnd := bytes.IndexByte(sourceSVG[start:], '"')
		if relativeEnd < 0 {
			return nil, fmt.Errorf("unterminated image href in generated SVG")
		}
		end := start + relativeEnd
		imageCount++
		if imageCount > rasterPreviewMaxImageReferences {
			return nil, fmt.Errorf("image references %d exceed limit %d", imageCount, rasterPreviewMaxImageReferences)
		}
		source := string(sourceSVG[start:end])
		if !strings.HasPrefix(strings.ToLower(source), "data:") {
			references = append(references, rasterPreviewReference{start: start, end: end, source: source})
		}
		searchAt = end + 1
	}
	if len(references) == 0 {
		return sourceSVG, nil
	}

	resources := make(map[string]rasterPreviewResource)
	outputBytes := int64(len(sourceSVG))
	for _, reference := range references {
		previewResource, ok := resources[reference.source]
		if !ok {
			resolved, err := resolver.Resolve(ctx, html.UnescapeString(reference.source))
			if err != nil {
				return nil, err
			}
			if resolved == nil {
				return nil, fmt.Errorf("image resolver returned nil for preview source")
			}
			encodedBytes := resolved.EncodedBytes()
			if encodedBytes < 0 || encodedBytes > int64(^uint(0)>>1) {
				return nil, fmt.Errorf("image encoded byte count %d is not representable", encodedBytes)
			}
			uriBytes := len("data:") + len(resolved.MIMEType()) + len(";base64,") + base64.StdEncoding.EncodedLen(int(encodedBytes))
			previewResource = rasterPreviewResource{resource: resolved, uriBytes: uriBytes}
			resources[reference.source] = previewResource
		}
		delta := int64(previewResource.uriBytes) - int64(reference.end-reference.start)
		if delta > rasterPreviewMaxOutputBytes-outputBytes {
			return nil, fmt.Errorf("expanded output bytes exceed limit %d", rasterPreviewMaxOutputBytes)
		}
		outputBytes += delta
	}
	if outputBytes < 0 || outputBytes > rasterPreviewMaxOutputBytes {
		return nil, fmt.Errorf("expanded output bytes %d exceed limit %d", outputBytes, rasterPreviewMaxOutputBytes)
	}

	encodedResources := make(map[string][]byte, len(resources))
	for source, previewResource := range resources {
		data, err := previewResource.resource.BytesContext(ctx)
		if err != nil {
			return nil, err
		}
		uri, err := rasterDataURI(ctx, previewResource.resource.MIMEType(), data, previewResource.uriBytes)
		if err != nil {
			return nil, err
		}
		encodedResources[source] = uri
	}

	result := make([]byte, 0, int(outputBytes))
	copyAt := 0
	for _, reference := range references {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		result = append(result, sourceSVG[copyAt:reference.start]...)
		result = append(result, encodedResources[reference.source]...)
		copyAt = reference.end
	}
	result = append(result, sourceSVG[copyAt:]...)
	if int64(len(result)) != outputBytes {
		return nil, fmt.Errorf("expanded output size %d differs from preflight size %d", len(result), outputBytes)
	}
	return result, nil
}

func rasterDataURI(ctx context.Context, mimeType string, data []byte, outputBytes int) ([]byte, error) {
	prefix := []byte("data:" + mimeType + ";base64,")
	if outputBytes != len(prefix)+base64.StdEncoding.EncodedLen(len(data)) {
		return nil, fmt.Errorf("data URI size preflight mismatch")
	}
	result := make([]byte, outputBytes)
	copy(result, prefix)
	writeAt := len(prefix)
	// A multiple of three lets every non-final chunk encode independently.
	const chunkBytes = 48 * 1024
	for readAt := 0; readAt < len(data); readAt += chunkBytes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		end := min(readAt+chunkBytes, len(data))
		base64.StdEncoding.Encode(result[writeAt:], data[readAt:end])
		writeAt += base64.StdEncoding.EncodedLen(end - readAt)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if writeAt != len(result) {
		return nil, fmt.Errorf("data URI encoded size %d differs from preflight size %d", writeAt, len(result))
	}
	return result, nil
}

func rasterRenderOptions(opts d2svg.RenderOpts) d2svg.RenderOpts {
	scale := opts.Scale
	if scale == nil {
		one := 1.0
		scale = &one
	}
	return d2svg.RenderOpts{
		Pad:                opts.Pad,
		Sketch:             opts.Sketch,
		Center:             opts.Center,
		Scale:              scale,
		ThemeID:            opts.ThemeID,
		DarkThemeID:        opts.DarkThemeID,
		ThemeOverrides:     opts.ThemeOverrides,
		DarkThemeOverrides: opts.DarkThemeOverrides,
		OmitVersion:        opts.OmitVersion,
	}
}

func collectGIFBoards(ctx context.Context, root *d2target.Diagram, maxBoards int) ([]*d2target.Diagram, error) {
	if root == nil {
		return nil, fmt.Errorf("GIF: nil diagram")
	}
	if maxBoards <= 0 {
		return nil, fmt.Errorf("GIF board count exceeds the %d-frame limit", gifMaxFrames)
	}
	boards := make([]*d2target.Diagram, 0, min(maxBoards, 16))
	active := make(map[*d2target.Diagram]struct{})
	visited := 0
	var walk func(*d2target.Diagram, int) error
	walk = func(diagram *d2target.Diagram, depth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if diagram == nil {
			return fmt.Errorf("GIF board tree contains a nil diagram")
		}
		if depth > rasterMaxDepth {
			return fmt.Errorf("GIF board tree exceeds depth %d", rasterMaxDepth)
		}
		visited++
		if visited > gifMaxBoardNodes {
			return fmt.Errorf("GIF board tree exceeds %d total nodes", gifMaxBoardNodes)
		}
		if _, ok := active[diagram]; ok {
			return fmt.Errorf("GIF board tree contains a cycle")
		}
		active[diagram] = struct{}{}
		defer delete(active, diagram)
		if !diagram.IsFolderOnly {
			if len(boards) == maxBoards {
				return fmt.Errorf("GIF board count exceeds the %d-frame limit", gifMaxFrames)
			}
			boards = append(boards, diagram)
		}
		for _, children := range [][]*d2target.Diagram{diagram.Layers, diagram.Scenarios, diagram.Steps} {
			for _, child := range children {
				if err := walk(child, depth+1); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(root, 1); err != nil {
		return nil, err
	}
	return boards, nil
}

type assetSessionLimits struct {
	maxDecodedPixels          int64
	maxAssets                 int
	maxCumulativeEncodedBytes int64
	maxCumulativeDecodedBytes int64
	svgImportBudget           d2scenebuild.SVGImportBudget
}

func sceneAssetOptions(inputPath string, cacheImages bool) (*d2scenebuild.AssetOptions, error) {
	return newSceneAssetOptions(inputPath, cacheImages, assetSessionLimits{
		maxDecodedPixels:          rasterMaxPixels,
		maxAssets:                 imageAssetMaxCount,
		maxCumulativeEncodedBytes: imageAssetMaxCumulativeEncodedBytes,
		maxCumulativeDecodedBytes: imageAssetMaxCumulativeDecodedBytes,
		svgImportBudget:           svgImportBudget(),
	})
}

func gifSceneAssetOptions(inputPath string, cacheImages bool, boardCount int) (*d2scenebuild.AssetOptions, error) {
	budget, err := divideSVGImportBudget(svgImportBudget(), boardCount)
	if err != nil {
		return nil, err
	}
	return newSceneAssetOptions(inputPath, cacheImages, assetSessionLimits{
		maxDecodedPixels:          gifMaxFramePixels,
		maxAssets:                 gifImageAssetMaxCount,
		maxCumulativeEncodedBytes: gifImageEncodedBytes,
		maxCumulativeDecodedBytes: gifImageDecodedBytes,
		svgImportBudget:           budget,
	})
}

func newSceneAssetOptions(inputPath string, cacheImages bool, limits assetSessionLimits) (*d2scenebuild.AssetOptions, error) {
	baseDir := ""
	if inputPath != "" && inputPath != "-" {
		baseDir = filepath.Dir(inputPath)
	}
	var cache imageasset.Cache
	if cacheImages {
		assetCacheOnce.Do(func() {
			assetCache, assetCacheErr = imageasset.NewMemoryCache(imageAssetMaxCount, assetCacheMaxBytes)
		})
		if assetCacheErr != nil {
			return nil, fmt.Errorf("raster export: initialize image cache: %w", assetCacheErr)
		}
		cache = assetCache
	}
	cacheNamespace := ""
	if cache != nil {
		cacheNamespace = assetCacheNamespace
	}
	resolver, err := imageasset.New(imageasset.Options{
		BaseDir:        baseDir,
		HTTPClient:     assetHTTPClient,
		Cache:          cache,
		CacheNamespace: cacheNamespace,
		Limits: imageasset.Limits{
			MaxFetchedBytes:           min(imageAssetMaxBytes, limits.maxCumulativeEncodedBytes),
			MaxEncodedBytes:           min(imageAssetMaxBytes, limits.maxCumulativeEncodedBytes),
			MaxDecompressedBytes:      min(imageAssetMaxBytes, limits.maxCumulativeEncodedBytes),
			MaxSVGBytes:               svgMaxBytes,
			MaxDecodedWidth:           rasterMaxDimension,
			MaxDecodedHeight:          rasterMaxDimension,
			MaxDecodedPixels:          limits.maxDecodedPixels,
			MaxAssets:                 limits.maxAssets,
			MaxCumulativeEncodedBytes: limits.maxCumulativeEncodedBytes,
			MaxCumulativeDecodedBytes: limits.maxCumulativeDecodedBytes,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("raster export: initialize image resolver: %w", err)
	}
	return &d2scenebuild.AssetOptions{
		Resolver:        resolver,
		SVGImportLimits: svgImportLimits(),
		SVGImportBudget: limits.svgImportBudget,
	}, nil
}

func svgImportLimits() d2svgimport.Limits {
	return d2svgimport.Limits{
		MaxBytes:              svgMaxBytes,
		MaxDepth:              svgMaxDepth,
		MaxElements:           svgMaxElements,
		MaxAttributes:         svgMaxAttributes,
		MaxAttributeBytes:     svgMaxAttributeBytes,
		MaxPathCommands:       svgMaxPathCommands,
		MaxTransformFunctions: svgMaxTransformFunctions,
		MaxUseDepth:           svgMaxUseDepth,
		MaxResources:          svgMaxResources,
	}
}

func svgImportBudget() d2scenebuild.SVGImportBudget {
	return d2scenebuild.SVGImportBudget{
		MaxSourceBytes:          svgDocumentMaxSourceBytes,
		MaxElements:             svgDocumentMaxElements,
		MaxAttributes:           svgDocumentMaxAttributes,
		MaxAttributeBytes:       svgDocumentMaxAttributeBytes,
		MaxPathCommands:         svgDocumentMaxPathCommands,
		MaxTransformFunctions:   svgDocumentMaxTransformFunctions,
		MaxDeclaredResources:    svgDocumentMaxDeclaredResources,
		MaxExpandedUseInstances: svgDocumentMaxExpandedUseInstances,
	}
}

func divideSVGImportBudget(total d2scenebuild.SVGImportBudget, boardCount int) (d2scenebuild.SVGImportBudget, error) {
	if boardCount <= 0 {
		return d2scenebuild.SVGImportBudget{}, fmt.Errorf("GIF requires a positive board count")
	}
	divided := d2scenebuild.SVGImportBudget{
		MaxSourceBytes:          total.MaxSourceBytes / boardCount,
		MaxElements:             total.MaxElements / boardCount,
		MaxAttributes:           total.MaxAttributes / boardCount,
		MaxAttributeBytes:       total.MaxAttributeBytes / boardCount,
		MaxPathCommands:         total.MaxPathCommands / boardCount,
		MaxTransformFunctions:   total.MaxTransformFunctions / boardCount,
		MaxDeclaredResources:    total.MaxDeclaredResources / boardCount,
		MaxExpandedUseInstances: total.MaxExpandedUseInstances / boardCount,
	}
	if divided.MaxSourceBytes <= 0 || divided.MaxElements <= 0 || divided.MaxAttributes <= 0 || divided.MaxAttributeBytes <= 0 ||
		divided.MaxPathCommands <= 0 || divided.MaxTransformFunctions <= 0 || divided.MaxDeclaredResources <= 0 || divided.MaxExpandedUseInstances <= 0 {
		return d2scenebuild.SVGImportBudget{}, fmt.Errorf("GIF has too many boards for the operation-wide SVG import budget")
	}
	return divided, nil
}
