// Package d2raster renders the supported d2scene subset with a pure-Go raster
// kernel. Unsupported scene features are rejected during preflight, before the
// final output canvas is allocated.
package d2raster

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"math"
	"sort"
	"time"

	"github.com/d2lang/d2/d2renderers/d2raster/internal/scanline"
	"github.com/d2lang/d2/d2renderers/d2scene"
)

// FrameOptions defines the pixel mapping and hard resource ceilings for one
// render. Every limit and Scale must be positive; callers select the limits so
// production ceilings are explicit rather than hidden package defaults.
type FrameOptions struct {
	Scale      float64
	Time       time.Duration
	Background color.Color

	MaxWidth  int
	MaxHeight int
	MaxPixels int64
	// Stored vector definitions are charged once by structural graph
	// validation. Every rendered vector Image then charges its instantiated
	// subtree again. Animation limits follow the same retained-plus-instantiated
	// policy and are charged before per-node animation bookkeeping is allocated.
	// Retained MaxDepth validation includes the unavoidable host Image at depth
	// one, so an intrinsically unplaceable definition is rejected even if unused.
	// MaxNodes also bounds aggregate filter entries and synthesized color-glyph
	// layers as separate structural dimensions, preventing arbitrarily long
	// identity-filter chains or color-font layer lists.
	MaxNodes        int
	MaxDepth        int
	MaxPathCommands int
	// MaxTextRunesPerRun bounds one non-interruptible segmentation/shaping
	// call. Zero selects the default; negative values are invalid.
	// Aggregate shaped glyphs remain bounded by MaxPathCommands.
	MaxTextRunesPerRun int
	// MaxFontFacesPerText bounds primary plus fallback face selection for one
	// TextRun. MaxTextCoverageChecks and MaxTextShapingRuns are aggregate frame
	// work ceilings. Zero selects bounded defaults.
	MaxFontFacesPerText   int
	MaxTextCoverageChecks int64
	MaxTextShapingRuns    int
	MaxAnimationTracks    int
	MaxAnimationKeyframes int
	MaxAssets             int
	MaxAssetBytes         int64
	// MaxDecodedAssetBytes bounds the cumulative resolver-declared decoded
	// footprint of all retained raster assets. MaxAssetBytes separately bounds
	// their encoded bytes together with retained font bytes.
	MaxDecodedAssetBytes int64
	// MaxImportDepth bounds active vector-asset nesting. Retained vector roots
	// and visible vector Image instances start at depth one; raster images do
	// not consume this budget.
	MaxImportDepth int

	// MaxOffscreenBytes bounds the peak live pixel backing storage used by
	// retained pattern tiles, temporary RGBA effect/filter layers, Alpha blur
	// and effect masks, and retained scanline-rasterizer scratch. It excludes
	// decoded assets (bounded separately), the final frame canvas, and returned
	// image.
	MaxOffscreenBytes int64
	// MaxEvenOddClipWork bounds aggregate point-in-edge evaluations used by
	// supersampled even-odd fills and clips.
	MaxEvenOddClipWork int64
	// MaxScanlineWork bounds aggregate operation units used by non-zero fills,
	// strokes, gradient masks, and clips. Zero selects a bounded default;
	// negative values are invalid.
	MaxScanlineWork int64
}

type preparedDocument struct {
	width      int
	height     int
	background *color.NRGBA
	root       *preparedNode
	patterns   []*preparedPatternTile
	resources  renderResources
}

type preparedNode struct {
	opacity       float64
	blend         d2scene.BlendMode
	isolated      bool
	primitive     *preparedPrimitive
	children      []*preparedNode
	filters       []preparedFilter
	clip          *preparedClip
	mask          *preparedMask
	contentBounds image.Rectangle
	bounds        image.Rectangle
}

type preparedPrimitive struct {
	subpaths        []subpath
	strokeRuns      []strokeRun
	transform       d2scene.Matrix
	fill            *preparedPaint
	fillRule        d2scene.FillRule
	evenOddSubpaths []subpath
	evenOddEdges    int64
	stroke          *preparedStroke
	image           *preparedImage
	vector          *preparedNode
	bounds          image.Rectangle
}

type preparedClip struct {
	subpaths []subpath
	fillRule d2scene.FillRule
	bounds   image.Rectangle
	edges    int64
}

type preparedMask struct {
	kind d2scene.MaskType
	root *preparedNode
}

type preparedStroke struct {
	paint      *preparedPaint
	width      float64
	cap        d2scene.LineCap
	join       d2scene.LineJoin
	miterLimit float64
	dashes     []float64
	dashOffset float64
}

type animationOverrides struct {
	fillColor   *color.NRGBA
	strokeColor *color.NRGBA
	dashOffset  *float64
	dropShadows map[int]d2scene.DropShadow
}

type preflight struct {
	ctx                context.Context
	document           *d2scene.Document
	options            FrameOptions
	viewToPixel        d2scene.Matrix
	frameBounds        image.Rectangle
	nodes              int
	pathSegments       int
	textRunes          int
	textCoverageChecks int64
	textShapingRuns    int
	shapedGlyphs       int
	animationTracks    int
	animationKeyframes int
	filters            int
	active             map[*d2scene.Node]bool
	activeAssets       map[d2scene.AssetID]bool
	fonts              map[d2scene.AssetID]*preparedFont
	rasters            map[d2scene.AssetID]*preparedRasterAsset
	vectors            map[d2scene.AssetID]d2scene.VectorAsset
	patternTiles       map[preparedPatternTileKey]*preparedPatternTile
	assetBytes         int64
	decodedBytes       int64
	session            *RenderSession
}

type rasterScratch struct {
	rasterizer        *scanline.Rasterizer
	rasterizerEdges   int
	rasterizerBounded bool
	offscreen         offscreenBudget
	patternTiles      []*preparedPatternTile
	patternBytes      int64
	scanlineWork      scanline.WorkBudget
	scanlineWorkSet   bool
}

func (s *rasterScratch) reset(bounds image.Rectangle) *scanline.Rasterizer {
	if s.rasterizer == nil {
		s.rasterizer = scanline.NewRasterizer(bounds.Dx(), bounds.Dy())
		if s.rasterizerBounded {
			s.rasterizer.ReserveEdges(s.rasterizerEdges)
		}
	} else {
		s.rasterizer.Reset(bounds.Dx(), bounds.Dy())
	}
	return s.rasterizer
}

func (s *rasterScratch) workBudget() *scanline.WorkBudget {
	if !s.scanlineWorkSet {
		s.scanlineWork = scanline.NewWorkBudget(defaultMaxScanlineWork)
		s.scanlineWorkSet = true
	}
	return &s.scanlineWork
}

// Render preflights document and returns a newly allocated NRGBA frame. The
// viewbox is mapped to LogicalWidth x LogicalHeight and then multiplied by
// FrameOptions.Scale.
func Render(ctx context.Context, document *d2scene.Document, options FrameOptions) (*image.NRGBA, error) {
	return render(ctx, document, options, nil)
}

// Render preflights and renders one frame while reusing this session's bounded
// parsed-font and decoded-raster cache.
func (s *RenderSession) Render(ctx context.Context, document *d2scene.Document, options FrameOptions) (*image.NRGBA, error) {
	if s == nil {
		return nil, fmt.Errorf("d2raster: nil render session")
	}
	return render(ctx, document, options, s)
}

func render(ctx context.Context, document *d2scene.Document, options FrameOptions, session *RenderSession) (*image.NRGBA, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	prepared, err := prepareWithSession(ctx, document, options, session)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	scratch := &rasterScratch{
		rasterizerEdges:   prepared.resources.rasterizerEdges,
		rasterizerBounded: true,
		offscreen:         offscreenBudget{limit: options.MaxOffscreenBytes},
		scanlineWork:      scanline.NewWorkBudget(prepared.resources.scanlineWork),
		scanlineWorkSet:   true,
	}
	defer scratch.releasePatternTiles()
	rasterizerReservation, err := scratch.offscreen.reserveBytes(prepared.resources.rasterizerBytes, "scanline rasterizer working storage")
	if err != nil {
		return nil, err
	}
	defer func() {
		if rasterizerReservation != 0 {
			scratch.offscreen.release(rasterizerReservation)
		}
	}()
	for _, pattern := range prepared.patterns {
		if err := pattern.render(ctx, scratch); err != nil {
			return nil, err
		}
	}
	canvas := image.NewRGBA(image.Rect(0, 0, prepared.width, prepared.height))
	if prepared.background != nil {
		draw.Draw(canvas, canvas.Bounds(), image.NewUniform(*prepared.background), image.Point{}, draw.Src)
	}
	if err := renderNode(ctx, canvas, prepared.root, scratch); err != nil {
		return nil, err
	}
	expectedLive := rasterizerReservation + scratch.patternBytes
	if scratch.offscreen.live != expectedLive {
		return nil, fmt.Errorf(
			"d2raster: internal offscreen reservation leak: %d bytes live, want retained rasterizer and patterns %d",
			scratch.offscreen.live, expectedLive,
		)
	}
	if scratch.offscreen.peak > prepared.resources.peakOffscreenBytes {
		return nil, fmt.Errorf(
			"d2raster: internal offscreen peak %d exceeds preflight plan %d",
			scratch.offscreen.peak, prepared.resources.peakOffscreenBytes,
		)
	}
	scratch.releasePatternTiles()
	scratch.rasterizer = nil
	scratch.offscreen.release(rasterizerReservation)
	rasterizerReservation = 0
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result := image.NewNRGBA(canvas.Bounds())
	draw.Draw(result, result.Bounds(), canvas, image.Point{}, draw.Src)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// EncodePNG deterministically encodes img as PNG.
func EncodePNG(ctx context.Context, img image.Image) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if img == nil {
		return nil, fmt.Errorf("d2raster: cannot encode a nil image")
	}
	var out bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := encoder.Encode(contextWriter{ctx: ctx, output: &out}, img); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, contextErr
		}
		return nil, fmt.Errorf("d2raster: encode PNG: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

type contextWriter struct {
	ctx    context.Context
	output io.Writer
}

func (w contextWriter) Write(data []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	written, err := w.output.Write(data)
	if err != nil {
		return written, err
	}
	if contextErr := w.ctx.Err(); contextErr != nil {
		return written, contextErr
	}
	return written, nil
}

func prepareWithSession(ctx context.Context, document *d2scene.Document, options FrameOptions, session *RenderSession) (*preparedDocument, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if document == nil {
		return nil, fmt.Errorf("d2raster: nil document")
	}
	if options.MaxTextRunesPerRun == 0 {
		options.MaxTextRunesPerRun = min(options.MaxPathCommands, defaultMaxTextRunesPerRun)
	}
	if options.MaxFontFacesPerText == 0 {
		options.MaxFontFacesPerText = min(options.MaxAssets, defaultMaxFontFacesPerText)
	}
	if options.MaxTextCoverageChecks == 0 {
		options.MaxTextCoverageChecks = defaultMaxTextCoverageChecks
	}
	if options.MaxTextShapingRuns == 0 {
		options.MaxTextShapingRuns = min(options.MaxPathCommands, defaultMaxTextShapingRuns)
	}
	if options.MaxScanlineWork == 0 {
		options.MaxScanlineWork = defaultMaxScanlineWork
	}
	if err := validateOptions(options); err != nil {
		return nil, err
	}
	if err := validateBox(document.ViewBox); err != nil || document.ViewBox.Width == 0 || document.ViewBox.Height == 0 {
		return nil, fmt.Errorf("d2raster: invalid viewbox")
	}
	if !finite(document.LogicalWidth) || !finite(document.LogicalHeight) || document.LogicalWidth <= 0 || document.LogicalHeight <= 0 {
		return nil, fmt.Errorf("d2raster: invalid logical dimensions")
	}
	if document.ViewportFit > d2scene.ViewportMeet {
		return nil, fmt.Errorf("d2raster: invalid viewport fit %d", document.ViewportFit)
	}
	if document.ViewportAlign > d2scene.ViewportAlignXMidYMid {
		return nil, fmt.Errorf("d2raster: invalid viewport alignment %d", document.ViewportAlign)
	}

	widthFloat := document.LogicalWidth * options.Scale
	heightFloat := document.LogicalHeight * options.Scale
	if !finite(widthFloat) || !finite(heightFloat) || widthFloat <= 0 || heightFloat <= 0 {
		return nil, fmt.Errorf("d2raster: invalid scaled dimensions")
	}
	widthCeil := math.Ceil(widthFloat)
	heightCeil := math.Ceil(heightFloat)
	if widthCeil > float64(options.MaxWidth) {
		return nil, fmt.Errorf("d2raster: frame width %.0f exceeds limit %d", widthCeil, options.MaxWidth)
	}
	if heightCeil > float64(options.MaxHeight) {
		return nil, fmt.Errorf("d2raster: frame height %.0f exceeds limit %d", heightCeil, options.MaxHeight)
	}
	maxInt := int(^uint(0) >> 1)
	if widthCeil >= float64(maxInt) || heightCeil >= float64(maxInt) {
		return nil, fmt.Errorf("d2raster: scaled dimensions exceed the platform integer domain")
	}
	width := int(widthCeil)
	height := int(heightCeil)
	if int64(width) > options.MaxPixels/int64(height) {
		return nil, fmt.Errorf("d2raster: frame pixels exceed limit %d", options.MaxPixels)
	}
	if _, err := finalFrameStorageBytes(width, height); err != nil {
		return nil, err
	}
	if document.Root == nil {
		return nil, fmt.Errorf("d2raster: document has no root node")
	}

	sx := widthFloat / document.ViewBox.Width
	sy := heightFloat / document.ViewBox.Height
	offsetX, offsetY := 0.0, 0.0
	if document.ViewportFit == d2scene.ViewportMeet {
		uniformScale := math.Min(float64(width)/document.ViewBox.Width, float64(height)/document.ViewBox.Height)
		sx, sy = uniformScale, uniformScale
		if document.ViewportAlign == d2scene.ViewportAlignXMidYMid {
			offsetX = (float64(width) - document.ViewBox.Width*uniformScale) / 2
			offsetY = (float64(height) - document.ViewBox.Height*uniformScale) / 2
		}
	}
	viewToPixel := d2scene.Matrix{
		A: sx,
		D: sy,
		E: offsetX - document.ViewBox.X*sx,
		F: offsetY - document.ViewBox.Y*sy,
	}
	if !viewToPixel.IsFinite() {
		return nil, fmt.Errorf("d2raster: viewbox mapping is non-finite")
	}
	p := &preflight{
		ctx:          ctx,
		document:     document,
		options:      options,
		viewToPixel:  viewToPixel,
		frameBounds:  image.Rect(0, 0, width, height),
		active:       make(map[*d2scene.Node]bool),
		activeAssets: make(map[d2scene.AssetID]bool),
		fonts:        make(map[d2scene.AssetID]*preparedFont),
		rasters:      make(map[d2scene.AssetID]*preparedRasterAsset),
		vectors:      make(map[d2scene.AssetID]d2scene.VectorAsset),
		patternTiles: make(map[preparedPatternTileKey]*preparedPatternTile),
		session:      session,
	}
	if err := p.assets(); err != nil {
		return nil, err
	}
	root, err := p.node(document.Root, viewToPixel, 1, 0)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	patterns, err := collectPreparedPatterns(ctx, root, p.frameBounds)
	if err != nil {
		return nil, err
	}
	resources, err := planRenderResources(ctx, root, p.frameBounds, patterns, options.MaxOffscreenBytes)
	if err != nil {
		return nil, err
	}
	if resources.peakOffscreenBytes > options.MaxOffscreenBytes {
		return nil, fmt.Errorf(
			"d2raster: peak offscreen pixel storage %d bytes exceeds limit %d",
			resources.peakOffscreenBytes, options.MaxOffscreenBytes,
		)
	}
	if resources.evenOddClipWork > options.MaxEvenOddClipWork {
		return nil, fmt.Errorf(
			"d2raster: even-odd clip work %d exceeds limit %d",
			resources.evenOddClipWork, options.MaxEvenOddClipWork,
		)
	}
	if resources.scanlineWork > options.MaxScanlineWork {
		return nil, fmt.Errorf(
			"d2raster: scanline work %d exceeds limit %d",
			resources.scanlineWork, options.MaxScanlineWork,
		)
	}

	prepared := &preparedDocument{width: width, height: height, root: root, patterns: patterns, resources: resources}
	if options.Background != nil {
		background := color.NRGBAModel.Convert(options.Background).(color.NRGBA)
		prepared.background = &background
	}
	return prepared, nil
}

const (
	defaultMaxTextRunesPerRun          = 100_000
	defaultMaxFontFacesPerText         = 64
	defaultMaxTextCoverageChecks       = int64(10_000_000)
	defaultMaxTextShapingRuns          = 100_000
	defaultMaxScanlineWork       int64 = 4_000_000_000
)

func finalFrameStorageBytes(width, height int) (int64, error) {
	pixels, ok := checkedMultiply(int64(width), int64(height))
	if !ok {
		return 0, fmt.Errorf("d2raster: final frame storage exceeds the int64 domain")
	}
	// Render retains the premultiplied RGBA canvas while allocating the
	// returned NRGBA image, so both four-byte backing stores must fit together.
	bytes, ok := checkedMultiply(pixels, 8)
	if !ok || bytes > platformMaxInt() {
		return 0, fmt.Errorf("d2raster: final frame storage exceeds the platform integer domain")
	}
	return bytes, nil
}

// preparationBounds keeps off-viewport source pixels available to bounded
// filters that can move them back into the destination. It remains a strict
// platform-sized domain; actual allocation is still charged from node-local
// ink/filter bounds before rendering.
func (p *preflight) preparationBounds() image.Rectangle {
	maxPlatform := platformMaxInt()
	minPlatform := -maxPlatform - 1
	margin := maxPlatform / 4
	minX, minY := minPlatform, minPlatform
	if int64(p.frameBounds.Min.X) >= minPlatform+margin {
		minX = int64(p.frameBounds.Min.X) - margin
	}
	if int64(p.frameBounds.Min.Y) >= minPlatform+margin {
		minY = int64(p.frameBounds.Min.Y) - margin
	}
	maxX, maxY := maxPlatform, maxPlatform
	if int64(p.frameBounds.Max.X) <= maxPlatform-margin {
		maxX = int64(p.frameBounds.Max.X) + margin
	}
	if int64(p.frameBounds.Max.Y) <= maxPlatform-margin {
		maxY = int64(p.frameBounds.Max.Y) + margin
	}
	return image.Rect(int(minX), int(minY), int(maxX), int(maxY))
}

func validateOptions(options FrameOptions) error {
	if !finite(options.Scale) || options.Scale <= 0 {
		return fmt.Errorf("d2raster: scale must be finite and positive")
	}
	if options.Time < 0 {
		return fmt.Errorf("d2raster: frame time must not be negative")
	}
	if options.MaxWidth <= 0 || options.MaxHeight <= 0 || options.MaxPixels <= 0 ||
		options.MaxNodes <= 0 || options.MaxDepth <= 0 || options.MaxPathCommands <= 0 || options.MaxTextRunesPerRun <= 0 ||
		options.MaxFontFacesPerText <= 0 || options.MaxTextCoverageChecks <= 0 || options.MaxTextShapingRuns <= 0 ||
		options.MaxAnimationTracks <= 0 || options.MaxAnimationKeyframes <= 0 ||
		options.MaxAssets <= 0 || options.MaxAssetBytes <= 0 || options.MaxDecodedAssetBytes <= 0 || options.MaxImportDepth <= 0 ||
		options.MaxOffscreenBytes <= 0 || options.MaxEvenOddClipWork <= 0 || options.MaxScanlineWork <= 0 {
		return fmt.Errorf("d2raster: every frame resource limit must be positive")
	}
	return nil
}

func (p *preflight) assets() error {
	if len(p.document.Assets) > p.options.MaxAssets {
		return fmt.Errorf("d2raster: asset count %d exceeds limit %d", len(p.document.Assets), p.options.MaxAssets)
	}
	ids := make([]string, 0, len(p.document.Assets))
	for id := range p.document.Assets {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)
	for _, rawID := range ids {
		if err := p.ctx.Err(); err != nil {
			return err
		}
		id := d2scene.AssetID(rawID)
		if id == "" {
			return fmt.Errorf("d2raster: empty asset ID")
		}
		asset := p.document.Assets[id]
		switch asset := asset.(type) {
		case d2scene.FontAsset:
			if err := p.addFontAsset(id, asset); err != nil {
				return err
			}
		case *d2scene.FontAsset:
			if asset == nil {
				return fmt.Errorf("d2raster: font asset %q is nil", id)
			}
			if err := p.addFontAsset(id, *asset); err != nil {
				return err
			}
		case d2scene.RasterAsset:
			if err := p.addRasterAsset(id, asset); err != nil {
				return err
			}
		case *d2scene.RasterAsset:
			if asset == nil {
				return fmt.Errorf("d2raster: raster asset %q is nil", id)
			}
			if err := p.addRasterAsset(id, *asset); err != nil {
				return err
			}
		case d2scene.VectorAsset:
			if err := p.addVectorAsset(id, asset); err != nil {
				return err
			}
		case *d2scene.VectorAsset:
			if asset == nil {
				return fmt.Errorf("d2raster: vector asset %q is nil", id)
			}
			if err := p.addVectorAsset(id, *asset); err != nil {
				return err
			}
		case nil:
			return fmt.Errorf("d2raster: asset %q is nil", id)
		default:
			return fmt.Errorf("d2raster: asset %q has unsupported type %T", id, asset)
		}
	}
	return p.validateRetainedVectorAssets(ids)
}

func (p *preflight) addFontAsset(id d2scene.AssetID, asset d2scene.FontAsset) error {
	if err := p.addAssetBytes(id, len(asset.Data)); err != nil {
		return err
	}
	var (
		font *preparedFont
		err  error
	)
	if p.session == nil {
		font, err = parsePreparedFont(asset.Data, asset.FaceIndex)
	} else {
		font, err = p.session.font(p.ctx, p.document, id, asset)
	}
	if err != nil {
		return fmt.Errorf("d2raster: font asset %q: %w", id, err)
	}
	p.fonts[id] = font
	return nil
}

func (p *preflight) addRasterAsset(id d2scene.AssetID, asset d2scene.RasterAsset) error {
	if err := p.addAssetBytes(id, len(asset.Data)); err != nil {
		return err
	}
	availableBytes := p.options.MaxDecodedAssetBytes - p.decodedBytes
	var (
		prepared     *preparedRasterAsset
		decodedBytes int64
		err          error
	)
	if p.session == nil {
		prepared, decodedBytes, err = prepareRasterAsset(p.ctx, id, asset, availableBytes)
	} else {
		prepared, decodedBytes, err = p.session.raster(p.ctx, p.document, id, asset, availableBytes)
	}
	if err != nil {
		return err
	}
	p.decodedBytes += decodedBytes
	p.rasters[id] = prepared
	return nil
}

func (p *preflight) addVectorAsset(id d2scene.AssetID, asset d2scene.VectorAsset) error {
	if err := validateBox(asset.ViewBox); err != nil || asset.ViewBox.Width == 0 || asset.ViewBox.Height == 0 {
		return fmt.Errorf("d2raster: vector asset %q has invalid viewbox", id)
	}
	if asset.Root == nil {
		return fmt.Errorf("d2raster: vector asset %q has no root node", id)
	}
	// Retain the immutable scene handle during the collection pass. A second,
	// placement-independent pass validates each definition exactly once after
	// forward references are available. Visible Image occurrences are still
	// instantiated independently.
	p.vectors[id] = asset
	return nil
}

func (p *preflight) addAssetBytes(id d2scene.AssetID, encodedBytes int) error {
	encoded := int64(encodedBytes)
	if encoded > p.options.MaxAssetBytes-p.assetBytes {
		return fmt.Errorf("d2raster: asset %q causes retained asset bytes to exceed limit %d", id, p.options.MaxAssetBytes)
	}
	p.assetBytes += encoded
	return nil
}

func (p *preflight) node(node *d2scene.Node, parent d2scene.Matrix, depth, importDepth int) (*preparedNode, error) {
	if err := p.ctx.Err(); err != nil {
		return nil, err
	}
	if node == nil {
		return nil, nil
	}
	if depth > p.options.MaxDepth {
		return nil, fmt.Errorf("d2raster: node depth %d exceeds limit %d", depth, p.options.MaxDepth)
	}
	if p.active[node] {
		return nil, fmt.Errorf("d2raster: node cycle at %q", node.ID)
	}
	if err := p.addPreparedNodes(1); err != nil {
		return nil, err
	}
	if !node.Transform.IsFinite() || !finite(node.Opacity) || node.Opacity < 0 || node.Opacity > 1 {
		return nil, fmt.Errorf("d2raster: node %q has invalid transform or opacity", node.ID)
	}
	if !supportedBlendMode(node.Blend) {
		return nil, fmt.Errorf("d2raster: node %q uses invalid or unsupported blend mode %d", node.ID, node.Blend)
	}
	if err := p.chargeFilterWork(node.ID, node.Filters); err != nil {
		return nil, err
	}
	if err := p.chargeAnimationWork(node.ID, node.Animations); err != nil {
		return nil, err
	}
	p.active[node] = true
	defer delete(p.active, node)

	nodeTransform := node.Transform
	opacity := node.Opacity
	var overrides animationOverrides
	seenTracks := make(map[[2]int]bool, len(node.Animations))
	for trackIndex, track := range node.Animations {
		if err := p.ctx.Err(); err != nil {
			return nil, err
		}
		key := [2]int{int(track.Property), track.TargetIndex}
		if seenTracks[key] {
			return nil, fmt.Errorf("d2raster: node %q has duplicate animation target property %d index %d", node.ID, track.Property, track.TargetIndex)
		}
		seenTracks[key] = true
		value, err := p.animationValueAt(track)
		if err != nil {
			return nil, fmt.Errorf("d2raster: node %q animation %d: %w", node.ID, trackIndex, err)
		}
		if track.Property != d2scene.AnimateDropShadow && track.TargetIndex != 0 {
			return nil, fmt.Errorf("d2raster: node %q animation %d uses non-zero target index for scalar property %d", node.ID, trackIndex, track.Property)
		}
		switch track.Property {
		case d2scene.AnimateOpacity:
			if value.Number < 0 || value.Number > 1 {
				return nil, fmt.Errorf("d2raster: node %q animation %d resolves opacity outside [0,1]", node.ID, trackIndex)
			}
			opacity = value.Number
		case d2scene.AnimateTransform:
			nodeTransform = value.Transform
		case d2scene.AnimateStrokeDashOffset:
			offset := value.Number
			overrides.dashOffset = &offset
		case d2scene.AnimateFillColor:
			animatedColor := value.Color
			overrides.fillColor = &animatedColor
		case d2scene.AnimateStrokeColor:
			animatedColor := value.Color
			overrides.strokeColor = &animatedColor
		case d2scene.AnimateDropShadow:
			if overrides.dropShadows == nil {
				overrides.dropShadows = make(map[int]d2scene.DropShadow)
			}
			overrides.dropShadows[track.TargetIndex] = value.Shadow
		default:
			return nil, fmt.Errorf("d2raster: node %q animation %d uses unknown property %d", node.ID, trackIndex, track.Property)
		}
	}

	transform := parent.Mul(nodeTransform)
	if !transform.IsFinite() {
		return nil, fmt.Errorf("d2raster: node %q has a non-finite composed transform", node.ID)
	}
	normalizedFilters, err := normalizeNodeFilters(p.ctx, node.ID, node.Filters, overrides.dropShadows)
	if err != nil {
		return nil, err
	}
	primitive, err := p.primitive(node.ID, node.Primitive, transform, overrides, depth, importDepth)
	if err != nil {
		return nil, err
	}
	if primitive == nil && (overrides.fillColor != nil || overrides.strokeColor != nil || overrides.dashOffset != nil) {
		return nil, fmt.Errorf("d2raster: node %q has a paint or stroke animation but no primitive", node.ID)
	}
	prepared := &preparedNode{opacity: opacity, blend: node.Blend, primitive: primitive}
	if primitive != nil {
		prepared.bounds = primitive.bounds
	}
	if node.Clip != nil {
		clip, err := p.clip(node.ID, node.Clip, transform)
		if err != nil {
			return nil, err
		}
		prepared.clip = clip
	}
	if node.Mask != nil {
		if !node.Mask.Transform.IsFinite() || node.Mask.Type > d2scene.MaskLuminance || node.Mask.Root == nil {
			return nil, fmt.Errorf("d2raster: node %q has invalid mask", node.ID)
		}
		// Scene masks use node-local user space: the node's complete composed
		// transform is applied before the mask's own transform.
		maskTransform := transform.Mul(node.Mask.Transform)
		if !maskTransform.IsFinite() {
			return nil, fmt.Errorf("d2raster: node %q has a non-finite composed mask transform", node.ID)
		}
		maskRoot, err := p.node(node.Mask.Root, maskTransform, depth+1, importDepth)
		if err != nil {
			return nil, fmt.Errorf("d2raster: node %q mask: %w", node.ID, err)
		}
		prepared.mask = &preparedMask{kind: node.Mask.Type, root: maskRoot}
	}
	for childIndex, child := range node.Children {
		if err := p.ctx.Err(); err != nil {
			return nil, err
		}
		// Document, vector-asset, and mask roots are checked explicitly by
		// their owners. Inside a node, every child slot must identify an
		// actual node; otherwise arbitrarily large nil slices could evade both
		// work accounting and useful malformed-scene errors.
		if child == nil {
			return nil, fmt.Errorf("d2raster: node %q child %d is nil", node.ID, childIndex)
		}
		preparedChild, err := p.node(child, transform, depth+1, importDepth)
		if err != nil {
			return nil, err
		}
		if preparedChild != nil {
			prepared.children = append(prepared.children, preparedChild)
			prepared.bounds = unionRect(prepared.bounds, preparedChild.bounds)
		}
	}
	prepared.contentBounds = prepared.bounds
	prepared.filters, prepared.bounds, err = p.prepareFilters(node.ID, normalizedFilters, transform, prepared.bounds)
	if err != nil {
		return nil, err
	}
	if prepared.clip != nil {
		prepared.bounds = prepared.bounds.Intersect(prepared.clip.bounds)
	}
	if prepared.mask != nil && prepared.mask.root != nil {
		prepared.bounds = prepared.bounds.Intersect(prepared.mask.root.bounds)
	}
	return prepared, nil
}

func (p *preflight) addPreparedNodes(count int) error {
	if count <= 0 {
		return fmt.Errorf("d2raster: internal prepared node charge must be positive")
	}
	if count > p.options.MaxNodes-p.nodes {
		return fmt.Errorf("d2raster: node count exceeds limit %d", p.options.MaxNodes)
	}
	p.nodes += count
	return nil
}

func (p *preflight) clip(nodeID string, clip *d2scene.Clip, transform d2scene.Matrix) (*preparedClip, error) {
	if clip == nil {
		return nil, nil
	}
	if !clip.Transform.IsFinite() {
		return nil, fmt.Errorf("d2raster: node %q has invalid clip transform", nodeID)
	}
	if clip.Path.FillRule > d2scene.EvenOdd {
		return nil, fmt.Errorf("d2raster: node %q clip has invalid fill rule %d", nodeID, clip.Path.FillRule)
	}
	// Clip.Path is geometry-only. Its Fill and Stroke paints do not contribute
	// pixels or coverage, consistent with SVG clipping-path geometry.
	clipTransform := transform.Mul(clip.Transform)
	if !clipTransform.IsFinite() {
		return nil, fmt.Errorf("d2raster: node %q has a non-finite composed clip transform", nodeID)
	}
	paths, err := flattenScenePath(p.ctx, clip.Path, clipTransform, p.addPathSegment)
	if err != nil {
		return nil, fmt.Errorf("d2raster: node %q clip: %w", nodeID, err)
	}
	devicePaths := make([]subpath, len(paths))
	var edges int64
	for pathIndex, path := range paths {
		devicePaths[pathIndex].closed = path.closed
		devicePaths[pathIndex].points = make([]d2scene.Point, len(path.points))
		for pointIndex, point := range path.points {
			devicePoint := clipTransform.Point(point)
			if err := validateRasterPoint(devicePoint); err != nil {
				return nil, fmt.Errorf("d2raster: node %q clip geometry: %w", nodeID, err)
			}
			devicePaths[pathIndex].points[pointIndex] = devicePoint
		}
		if len(path.points) >= 2 {
			edges += int64(len(path.points))
		}
	}
	return &preparedClip{
		subpaths: devicePaths,
		fillRule: clip.Path.FillRule,
		bounds:   subpathPixelBounds(devicePaths, d2scene.Identity(), 0, p.preparationBounds()),
		edges:    edges,
	}, nil
}

func (p *preflight) primitive(nodeID string, primitive d2scene.Primitive, transform d2scene.Matrix, animation animationOverrides, depth, importDepth int) (*preparedPrimitive, error) {
	if primitive == nil {
		return nil, nil
	}
	switch primitive := primitive.(type) {
	case d2scene.Rect:
		return p.rect(nodeID, primitive, transform, animation, importDepth)
	case *d2scene.Rect:
		if primitive == nil {
			return nil, nil
		}
		return p.rect(nodeID, *primitive, transform, animation, importDepth)
	case d2scene.Ellipse:
		return p.ellipse(nodeID, primitive, transform, animation, importDepth)
	case *d2scene.Ellipse:
		if primitive == nil {
			return nil, nil
		}
		return p.ellipse(nodeID, *primitive, transform, animation, importDepth)
	case d2scene.Path:
		return p.path(nodeID, primitive, transform, animation, importDepth)
	case *d2scene.Path:
		if primitive == nil {
			return nil, nil
		}
		return p.path(nodeID, *primitive, transform, animation, importDepth)
	case d2scene.TextRun:
		return p.text(nodeID, primitive, transform, animation, importDepth)
	case *d2scene.TextRun:
		if primitive == nil {
			return nil, nil
		}
		return p.text(nodeID, *primitive, transform, animation, importDepth)
	case d2scene.Image:
		return p.image(nodeID, primitive, transform, animation, depth, importDepth)
	case *d2scene.Image:
		if primitive == nil {
			return nil, nil
		}
		return p.image(nodeID, *primitive, transform, animation, depth, importDepth)
	default:
		return nil, fmt.Errorf("d2raster: node %q uses unsupported primitive %T", nodeID, primitive)
	}
}

func (p *preflight) rect(nodeID string, rect d2scene.Rect, transform d2scene.Matrix, animation animationOverrides, importDepth int) (*preparedPrimitive, error) {
	if err := validateBox(rect.Box); err != nil {
		return nil, fmt.Errorf("d2raster: node %q rectangle: %w", nodeID, err)
	}
	if !finite(rect.RadiusX) || !finite(rect.RadiusY) || rect.RadiusX < 0 || rect.RadiusY < 0 {
		return nil, fmt.Errorf("d2raster: node %q rectangle has invalid corner radius", nodeID)
	}
	subpaths, err := roundedRectSubpaths(p.ctx, rect, flattenTolerance(transform), p.addPathSegment)
	if err != nil {
		return nil, fmt.Errorf("d2raster: node %q rectangle: %w", nodeID, err)
	}
	objectBounds := rect.Box
	fill, err := p.prepareAnimatedPaint(rect.Fill, animation.fillColor, objectBounds, transform, importDepth)
	if err != nil {
		return nil, fmt.Errorf("d2raster: node %q rectangle fill: %w", nodeID, err)
	}
	stroke, err := p.prepareAnimatedStroke(rect.Stroke, animation.strokeColor, animation.dashOffset, objectBounds, transform, importDepth)
	if err != nil {
		return nil, fmt.Errorf("d2raster: node %q rectangle stroke: %w", nodeID, err)
	}
	return p.finishPrimitive(nodeID, subpaths, transform, fill, stroke)
}

func (p *preflight) ellipse(nodeID string, ellipse d2scene.Ellipse, transform d2scene.Matrix, animation animationOverrides, importDepth int) (*preparedPrimitive, error) {
	if !finitePoint(ellipse.Center) || !finite(ellipse.RadiusX) || !finite(ellipse.RadiusY) || ellipse.RadiusX < 0 || ellipse.RadiusY < 0 {
		return nil, fmt.Errorf("d2raster: node %q ellipse has invalid geometry", nodeID)
	}
	subpaths, err := ellipseSubpaths(p.ctx, ellipse.Center, ellipse.RadiusX, ellipse.RadiusY, flattenTolerance(transform), p.addPathSegment)
	if err != nil {
		return nil, fmt.Errorf("d2raster: node %q ellipse: %w", nodeID, err)
	}
	objectBounds := d2scene.Box{
		X: ellipse.Center.X - ellipse.RadiusX, Y: ellipse.Center.Y - ellipse.RadiusY,
		Width: 2 * ellipse.RadiusX, Height: 2 * ellipse.RadiusY,
	}
	fill, err := p.prepareAnimatedPaint(ellipse.Fill, animation.fillColor, objectBounds, transform, importDepth)
	if err != nil {
		return nil, fmt.Errorf("d2raster: node %q ellipse fill: %w", nodeID, err)
	}
	stroke, err := p.prepareAnimatedStroke(ellipse.Stroke, animation.strokeColor, animation.dashOffset, objectBounds, transform, importDepth)
	if err != nil {
		return nil, fmt.Errorf("d2raster: node %q ellipse stroke: %w", nodeID, err)
	}
	return p.finishPrimitive(nodeID, subpaths, transform, fill, stroke)
}

func (p *preflight) path(nodeID string, path d2scene.Path, transform d2scene.Matrix, animation animationOverrides, importDepth int) (*preparedPrimitive, error) {
	if path.FillRule > d2scene.EvenOdd {
		return nil, fmt.Errorf("d2raster: node %q path has invalid fill rule %d", nodeID, path.FillRule)
	}
	subpaths, err := flattenScenePath(p.ctx, path, transform, p.addPathSegment)
	if err != nil {
		return nil, fmt.Errorf("d2raster: node %q path: %w", nodeID, err)
	}
	geometryBounds, err := path.GeometryBounds()
	if err != nil {
		return nil, fmt.Errorf("d2raster: node %q path bounds: %w", nodeID, err)
	}
	objectBounds := geometryBounds.Box()
	fill, err := p.prepareAnimatedPaint(path.Fill, animation.fillColor, objectBounds, transform, importDepth)
	if err != nil {
		return nil, fmt.Errorf("d2raster: node %q path fill: %w", nodeID, err)
	}
	stroke, err := p.prepareAnimatedStroke(path.Stroke, animation.strokeColor, animation.dashOffset, objectBounds, transform, importDepth)
	if err != nil {
		return nil, fmt.Errorf("d2raster: node %q path stroke: %w", nodeID, err)
	}
	prepared, err := p.finishPrimitive(nodeID, subpaths, transform, fill, stroke)
	if err != nil {
		return nil, err
	}
	prepared.fillRule = path.FillRule
	if path.FillRule == d2scene.EvenOdd && fill != nil {
		prepared.evenOddSubpaths = make([]subpath, len(subpaths))
		for pathIndex, path := range subpaths {
			if err := p.ctx.Err(); err != nil {
				return nil, err
			}
			prepared.evenOddSubpaths[pathIndex].closed = path.closed
			prepared.evenOddSubpaths[pathIndex].points = make([]d2scene.Point, len(path.points))
			for pointIndex, point := range path.points {
				if pointIndex&255 == 0 {
					if err := p.ctx.Err(); err != nil {
						return nil, err
					}
				}
				prepared.evenOddSubpaths[pathIndex].points[pointIndex] = transform.Point(point)
			}
			if len(path.points) >= 2 {
				prepared.evenOddEdges += int64(len(path.points))
			}
		}
	}
	return prepared, nil
}

func (p *preflight) finishPrimitive(nodeID string, paths []subpath, transform d2scene.Matrix, fill *preparedPaint, stroke *preparedStroke) (*preparedPrimitive, error) {
	for _, path := range paths {
		for _, point := range path.points {
			if err := p.ctx.Err(); err != nil {
				return nil, err
			}
			if err := validateRasterPoint(transform.Point(point)); err != nil {
				return nil, fmt.Errorf("d2raster: node %q geometry: %w", nodeID, err)
			}
		}
	}
	prepared := &preparedPrimitive{subpaths: paths, transform: transform, fill: fill, stroke: stroke}
	if fill != nil {
		prepared.bounds = subpathPixelBounds(paths, transform, 0, p.preparationBounds())
	}
	if stroke == nil {
		return prepared, nil
	}
	if !finite(stroke.width * transform.MaxScale()) {
		return nil, fmt.Errorf("d2raster: node %q stroke has non-finite transformed width", nodeID)
	}
	for _, path := range paths {
		if err := validateStrokeCenterline(path); err != nil {
			return nil, fmt.Errorf("d2raster: node %q stroke geometry: %w", nodeID, err)
		}
		runs, err := makeStrokeRuns(p.ctx, path, stroke.dashes, stroke.dashOffset, p.addPathSegment)
		if err != nil {
			return nil, fmt.Errorf("d2raster: node %q stroke: %w", nodeID, err)
		}
		prepared.strokeRuns = append(prepared.strokeRuns, runs...)
	}
	halfWidth := stroke.width / 2
	for _, run := range prepared.strokeRuns {
		for _, point := range run.points {
			for _, offset := range [...]d2scene.Point{
				{X: -halfWidth, Y: -halfWidth},
				{X: halfWidth, Y: -halfWidth},
				{X: halfWidth, Y: halfWidth},
				{X: -halfWidth, Y: halfWidth},
			} {
				if err := validateRasterPoint(transform.Point(d2scene.Point{X: point.X + offset.X, Y: point.Y + offset.Y})); err != nil {
					return nil, fmt.Errorf("d2raster: node %q stroke geometry: %w", nodeID, err)
				}
			}
		}
		points := cleanPoints(run.points, run.closed)
		if stroke.join != d2scene.JoinRound {
			err := forEachStrokeJoin(p.ctx, points, run.closed, func(previous, vertex, next d2scene.Point) error {
				polygon, ok := strokeJoinPolygon(previous, vertex, next, halfWidth, stroke.join, stroke.miterLimit)
				if !ok {
					return nil
				}
				for _, point := range polygon {
					if err := validateRasterPoint(transform.Point(point)); err != nil {
						return err
					}
				}
				return nil
			})
			if err != nil {
				return nil, fmt.Errorf("d2raster: node %q stroke join geometry: %w", nodeID, err)
			}
		}
	}
	expansion := stroke.width * transform.MaxScale() / 2
	factor := 1.0
	if stroke.cap == d2scene.CapSquare {
		factor = math.Sqrt2
	}
	if stroke.join == d2scene.JoinMiter && stroke.miterLimit > factor {
		factor = stroke.miterLimit
	}
	prepared.bounds = unionRect(prepared.bounds, strokeRunPixelBounds(prepared.strokeRuns, transform, expansion*factor, p.preparationBounds()))
	return prepared, nil
}

func (p *preflight) addPathSegment() error {
	p.pathSegments++
	if p.pathSegments > p.options.MaxPathCommands {
		return fmt.Errorf("path command count exceeds limit %d", p.options.MaxPathCommands)
	}
	return nil
}

func (p *preflight) chargeAnimationWork(nodeID string, tracks []d2scene.Track) error {
	if err := p.ctx.Err(); err != nil {
		return err
	}
	if len(tracks) > p.options.MaxAnimationTracks-p.animationTracks {
		return fmt.Errorf(
			"d2raster: node %q causes animation track count to exceed limit %d",
			nodeID, p.options.MaxAnimationTracks,
		)
	}
	p.animationTracks += len(tracks)
	for trackIndex, track := range tracks {
		if err := p.ctx.Err(); err != nil {
			return err
		}
		if len(track.Keyframes) > p.options.MaxAnimationKeyframes-p.animationKeyframes {
			return fmt.Errorf(
				"d2raster: node %q animation %d causes keyframe count to exceed limit %d",
				nodeID, trackIndex, p.options.MaxAnimationKeyframes,
			)
		}
		p.animationKeyframes += len(track.Keyframes)
	}
	return nil
}

// MaxNodes also caps filter entries as a separate structural-work dimension.
// This keeps the public options stable while preventing one node from carrying
// an unbounded identity-filter slice that evades node and path accounting.
func (p *preflight) chargeFilterWork(nodeID string, filters []d2scene.Filter) error {
	if err := p.ctx.Err(); err != nil {
		return err
	}
	if len(filters) > p.options.MaxNodes-p.filters {
		return fmt.Errorf("d2raster: node %q causes filter count to exceed structural limit %d", nodeID, p.options.MaxNodes)
	}
	p.filters += len(filters)
	return nil
}

func (p *preflight) prepareAnimatedStroke(stroke *d2scene.Stroke, animatedColor *color.NRGBA, animatedDashOffset *float64, objectBounds d2scene.Box, objectToDevice d2scene.Matrix, importDepth int) (*preparedStroke, error) {
	return prepareAnimatedStrokeWithPaint(stroke, animatedColor, animatedDashOffset, objectBounds, objectToDevice, func(paint d2scene.Paint, animatedColor *color.NRGBA, objectBounds d2scene.Box, objectToDevice d2scene.Matrix) (*preparedPaint, error) {
		return p.prepareAnimatedPaint(paint, animatedColor, objectBounds, objectToDevice, importDepth)
	})
}

type animatedPaintPreparer func(d2scene.Paint, *color.NRGBA, d2scene.Box, d2scene.Matrix) (*preparedPaint, error)

func prepareAnimatedStrokeWithPaint(stroke *d2scene.Stroke, animatedColor *color.NRGBA, animatedDashOffset *float64, objectBounds d2scene.Box, objectToDevice d2scene.Matrix, preparePaint animatedPaintPreparer) (*preparedStroke, error) {
	if stroke == nil {
		if animatedColor != nil || animatedDashOffset != nil {
			return nil, fmt.Errorf("stroke animation targets missing stroke")
		}
		return nil, nil
	}
	if !finite(stroke.Width) || stroke.Width < 0 || !finite(stroke.MiterLimit) || !finite(stroke.DashOffset) {
		return nil, fmt.Errorf("invalid stroke")
	}
	paint, err := preparePaint(stroke.Paint, animatedColor, objectBounds, objectToDevice)
	if err != nil {
		return nil, err
	}
	dashTotal := 0.0
	for _, dash := range stroke.Dashes {
		if !finite(dash) || dash <= 0 {
			return nil, fmt.Errorf("invalid stroke dash")
		}
		dashTotal += dash
		if !finite(dashTotal) {
			return nil, fmt.Errorf("invalid stroke dash total")
		}
	}
	if stroke.Width == 0 || paint == nil {
		return nil, nil
	}
	switch stroke.Cap {
	case d2scene.CapButt, d2scene.CapRound, d2scene.CapSquare:
	default:
		return nil, fmt.Errorf("unsupported line cap %d", stroke.Cap)
	}
	switch stroke.Join {
	case d2scene.JoinMiter, d2scene.JoinRound, d2scene.JoinBevel:
	default:
		return nil, fmt.Errorf("unsupported line join %d", stroke.Join)
	}
	miterLimit := stroke.MiterLimit
	if stroke.Join == d2scene.JoinMiter {
		if miterLimit == 0 {
			miterLimit = 4
		}
		if miterLimit < 1 {
			return nil, fmt.Errorf("invalid miter limit %g", stroke.MiterLimit)
		}
	}
	dashOffset := stroke.DashOffset
	if animatedDashOffset != nil {
		dashOffset = *animatedDashOffset
	}
	return &preparedStroke{
		paint:      paint,
		width:      stroke.Width,
		cap:        stroke.Cap,
		join:       stroke.Join,
		miterLimit: miterLimit,
		dashes:     append([]float64(nil), stroke.Dashes...),
		dashOffset: dashOffset,
	}, nil
}

func renderNode(ctx context.Context, dst *image.RGBA, node *preparedNode, scratch *rasterScratch) error {
	if node == nil || node.opacity == 0 || node.bounds.Empty() {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if node.isolated || node.opacity < 1 || node.blend != d2scene.BlendNormal || len(node.filters) != 0 || node.clip != nil || node.mask != nil {
		return renderEffectNode(ctx, dst, node, scratch)
	}
	if node.primitive != nil && !node.primitive.bounds.Intersect(dst.Bounds()).Empty() {
		if err := drawPrimitive(ctx, dst, node.primitive, scratch); err != nil {
			return err
		}
	}
	for _, child := range node.children {
		if err := renderNode(ctx, dst, child, scratch); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func drawPrimitive(ctx context.Context, dst *image.RGBA, primitive *preparedPrimitive, scratch *rasterScratch) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if primitive.image != nil {
		return drawPreparedImage(ctx, dst, primitive.image)
	}
	if primitive.vector != nil {
		return renderNode(ctx, dst, primitive.vector, scratch)
	}
	if primitive.fill != nil {
		if primitive.fillRule == d2scene.EvenOdd {
			bounds := subpathPixelBounds(primitive.subpaths, primitive.transform, 0, dst.Bounds())
			err := drawPaintMask(ctx, dst, bounds, primitive.fill, scratch, "even-odd fill Alpha mask", func(mask *image.Alpha) error {
				return rasterizeEvenOddMask(ctx, mask, bounds.Min, primitive.evenOddSubpaths)
			})
			if err != nil {
				return err
			}
		} else if primitive.fill.kind == preparedSolidPaint {
			rasterizer := scratch.reset(dst.Bounds())
			shifted := d2scene.Translate(-float64(dst.Bounds().Min.X), -float64(dst.Bounds().Min.Y)).Mul(primitive.transform)
			for _, path := range primitive.subpaths {
				addFillSubpath(rasterizer, path, shifted)
			}
			if err := rasterizer.DrawRGBA(ctx, scratch.workBudget(), dst, primitive.fill.solid); err != nil {
				return fmt.Errorf("d2raster: solid fill: %w", err)
			}
		} else {
			bounds := subpathPixelBounds(primitive.subpaths, primitive.transform, 0, dst.Bounds())
			err := drawGradientMask(ctx, dst, bounds, primitive.fill, scratch, func(mask *image.Alpha) error {
				rasterizer := scratch.reset(mask.Bounds())
				shifted := d2scene.Translate(-float64(bounds.Min.X), -float64(bounds.Min.Y)).Mul(primitive.transform)
				for _, path := range primitive.subpaths {
					addFillSubpath(rasterizer, path, shifted)
				}
				if err := rasterizer.WriteAlpha(ctx, scratch.workBudget(), mask); err != nil {
					return fmt.Errorf("d2raster: gradient fill: %w", err)
				}
				return ctx.Err()
			})
			if err != nil {
				return err
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	if primitive.stroke != nil {
		if err := drawStroke(ctx, dst, primitive.strokeRuns, primitive.transform, primitive.stroke, scratch); err != nil {
			return err
		}
	}
	return nil
}

func addFillSubpath(rasterizer *scanline.Rasterizer, path subpath, transform d2scene.Matrix) {
	if len(path.points) < 2 {
		return
	}
	first := transform.Point(path.points[0])
	rasterizer.MoveTo(float32(first.X), float32(first.Y))
	for _, point := range path.points[1:] {
		point = transform.Point(point)
		rasterizer.LineTo(float32(point.X), float32(point.Y))
	}
	// SVG fill closes open subpaths implicitly.
	rasterizer.ClosePath()
}

func validateBox(box d2scene.Box) error {
	if !finite(box.X) || !finite(box.Y) || !finite(box.Width) || !finite(box.Height) {
		return fmt.Errorf("non-finite box")
	}
	if box.Width < 0 || box.Height < 0 {
		return fmt.Errorf("negative box size")
	}
	return nil
}

func finite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

func finitePoint(point d2scene.Point) bool {
	return finite(point.X) && finite(point.Y)
}

func validateRasterPoint(point d2scene.Point) error {
	if !finitePoint(point) || math.Abs(point.X) > math.MaxFloat32 || math.Abs(point.Y) > math.MaxFloat32 {
		return fmt.Errorf("point is outside the finite float32 raster domain")
	}
	return nil
}

func validateStrokeCenterline(path subpath) error {
	points := cleanPoints(path.points, path.closed)
	edgeCount := len(points) - 1
	if path.closed && len(points) > 1 {
		edgeCount = len(points)
	}
	for edge := 0; edge < edgeCount; edge++ {
		start := points[edge]
		end := points[(edge+1)%len(points)]
		dx, dy := end.X-start.X, end.Y-start.Y
		if !finite(dx) || !finite(dy) || !finite(math.Hypot(dx, dy)) {
			return fmt.Errorf("centerline delta is non-finite")
		}
	}
	return nil
}
