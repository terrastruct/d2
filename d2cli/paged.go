package d2cli

import (
	"context"
	"fmt"
	"image"
	"io"
	"math"

	"github.com/d2lang/d2/d2plugin"
	"github.com/d2lang/d2/d2renderers/d2raster"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/d2scenebuild"
	"github.com/d2lang/d2/d2renderers/d2svg"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/pdf"
	"github.com/d2lang/d2/lib/png"
	"github.com/d2lang/d2/lib/pptx"
	"github.com/d2lang/d2/lib/textmeasure"
)

const (
	pagedMaxBoards                    = 1_024
	pagedMaxTreeNodes                 = 4_096
	pagedMaxTotalPixels         int64 = 256 * 1024 * 1024
	pagedMaxEncodedBytes        int64 = 512 * 1024 * 1024
	pagedMaxLinkRegions               = 65_536
	pagedMaxLinkStringBytes           = 64 * 1024 * 1024
	pagedPerBoardMaxLinkRegions       = 4_096
	pagedPerBoardMaxLinkBytes         = 1 * 1024 * 1024
)

type pagedRenderer struct {
	ctx              context.Context
	plugin           d2plugin.Plugin
	opts             d2svg.RenderOpts
	assets           *d2scenebuild.AssetOptions
	fonts            *d2scenebuild.FontFallbackOptions
	session          *d2raster.RenderSession
	workspace        d2raster.RenderWorkspace
	pngEncoder       rasterPNGEncoder
	totalBoards      int
	boardIDToPage    map[string]int
	linkBudget       d2scenebuild.LinkBudget
	renderedBoards   int
	remainingPixels  int64
	remainingEncoded int64
}

func (r *pagedRenderer) close() {
	if r == nil {
		return
	}
	r.pngEncoder.close()
	r.workspace.Reset()
}

type pagedBoard struct {
	png []byte
	// links are mapped into the encoded PNG's source-pixel coordinate space.
	// PDF divides these coordinates by png.SCALE; PPTX consumes them directly.
	links   []d2scene.LinkRegion
	preview []byte
}

// Both raster renderers feed the same board traversal, navigation and typed
// link adapters. Images and link regions are already in final pixel space.
type pagedBoardRenderer interface {
	render(*d2target.Diagram, bool) (*pagedBoard, error)
	close()
	info() (context.Context, map[string]int, int)
}

func (r *pagedRenderer) info() (context.Context, map[string]int, int) {
	return r.ctx, r.boardIDToPage, r.totalBoards
}

func newPagedRenderer(ctx context.Context, plugin d2plugin.Plugin, inputPath string, cacheImages bool, diagram *d2target.Diagram, opts d2svg.RenderOpts) (*pagedRenderer, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	boardIDToPage, boardCount, err := indexPagedBoards(ctx, diagram)
	if err != nil {
		return nil, err
	}
	if boardCount == 0 {
		return nil, fmt.Errorf("paged export requires at least one renderable board")
	}
	linkBudget, err := pagedLinkBudget(boardCount)
	if err != nil {
		return nil, err
	}
	assets, err := pagedSceneAssetOptions(inputPath, cacheImages, boardCount)
	if err != nil {
		return nil, err
	}
	fonts, err := newFontFallbackOptions(boardCount)
	if err != nil {
		return nil, err
	}
	session, err := d2raster.NewRenderSession(d2raster.RenderSessionOptions{
		MaxCacheEntries:    rasterMaxAssets,
		MaxCacheBytes:      pagedRenderCacheBytes,
		MaxConcurrentLoads: 1,
	})
	if err != nil {
		return nil, fmt.Errorf("paged render session: %w", err)
	}
	return &pagedRenderer{
		ctx:              ctx,
		plugin:           plugin,
		opts:             opts,
		assets:           assets,
		fonts:            fonts,
		session:          session,
		totalBoards:      boardCount,
		boardIDToPage:    boardIDToPage,
		linkBudget:       linkBudget,
		remainingPixels:  pagedMaxTotalPixels,
		remainingEncoded: pagedMaxEncodedBytes,
	}, nil
}

func (r *pagedRenderer) render(diagram *d2target.Diagram, wantsPreview bool) (*pagedBoard, error) {
	if err := r.ctx.Err(); err != nil {
		return nil, err
	}
	if diagram == nil {
		return nil, fmt.Errorf("paged export contains a nil diagram")
	}
	if diagram.IsFolderOnly {
		return nil, fmt.Errorf("paged export cannot render a folder-only board")
	}
	if r.renderedBoards >= r.totalBoards {
		return nil, fmt.Errorf("paged export rendered more than its %d preflighted boards", r.totalBoards)
	}

	board := *diagram
	board.Layers = nil
	board.Scenarios = nil
	board.Steps = nil
	board.Root.Fill = "transparent"
	renderOpts := rasterRenderOptions(r.opts)
	preview, err := renderRasterSVG(r.ctx, r.plugin, &board, renderOpts, wantsPreview, true)
	if err != nil {
		return nil, fmt.Errorf("paged board %d: %w", r.renderedBoards, err)
	}

	frameOptions, err := pagedFrameOptions(r.totalBoards, r.remainingPixels)
	if err != nil {
		return nil, err
	}
	document, err := buildSceneWithResourcesAndLinks(r.ctx, &board, renderOpts, r.assets, r.fonts, r.linkBudget, sceneAdmissionLimitsForFrame(frameOptions))
	if err != nil {
		return nil, fmt.Errorf("paged board %d scene: %w", r.renderedBoards, err)
	}
	var (
		links      []d2scene.LinkRegion
		pixels     int64
		encoded    []byte
		consumeErr error
	)
	renderErr := r.workspace.Render(r.ctx, r.session, document, frameOptions, func(frame *image.NRGBA) error {
		if err := validateRenderSession("paged export", r.session); err != nil {
			consumeErr = err
			return err
		}
		var err error
		links, err = mapPagedLinks(r.ctx, document, frame, frameOptions.Scale)
		if err != nil {
			consumeErr = fmt.Errorf("paged board %d links: %w", r.renderedBoards, err)
			return consumeErr
		}
		pixels, err = imagePixelCount(frame)
		if err != nil {
			consumeErr = fmt.Errorf("paged board %d frame: %w", r.renderedBoards, err)
			return consumeErr
		}
		if pixels > r.remainingPixels {
			consumeErr = fmt.Errorf("paged pixel work exceeds the %d-pixel operation limit", pagedMaxTotalPixels)
			return consumeErr
		}
		encoded, err = r.pngEncoder.encodeGeneric(r.ctx, frame)
		if err != nil {
			consumeErr = fmt.Errorf("paged board %d PNG: %w", r.renderedBoards, err)
			return consumeErr
		}
		if int64(len(encoded)) > r.remainingEncoded {
			consumeErr = fmt.Errorf("paged encoded PNG bytes exceed the %d-byte operation limit", pagedMaxEncodedBytes)
			return consumeErr
		}
		return nil
	})
	if consumeErr != nil {
		return nil, consumeErr
	}
	if renderErr != nil {
		return nil, fmt.Errorf("paged board %d frame: %w", r.renderedBoards, renderErr)
	}
	if len(preview) != 0 {
		preview, err = bundleRasterPreview(r.ctx, r.assets.Resolver, preview)
		if err != nil {
			return nil, fmt.Errorf("paged watch preview: %w", err)
		}
	}

	r.remainingPixels -= pixels
	r.remainingEncoded -= int64(len(encoded))
	r.renderedBoards++
	return &pagedBoard{png: encoded, links: links, preview: preview}, nil
}

func pagedLinkBudget(totalBoards int) (d2scenebuild.LinkBudget, error) {
	if totalBoards <= 0 {
		return d2scenebuild.LinkBudget{}, fmt.Errorf("paged export requires a positive board count for link budgets")
	}
	return d2scenebuild.LinkBudget{
		MaxRegions:     min(pagedPerBoardMaxLinkRegions, max(1, pagedMaxLinkRegions/totalBoards)),
		MaxStringBytes: min(pagedPerBoardMaxLinkBytes, max(1, pagedMaxLinkStringBytes/totalBoards)),
	}, nil
}

func pagedFrameOptions(totalBoards int, remainingPixels int64) (d2raster.FrameOptions, error) {
	if totalBoards <= 0 || remainingPixels <= 0 {
		return d2raster.FrameOptions{}, fmt.Errorf("paged export exhausted its operation budget")
	}
	options := rasterFrameOptions(png.SCALE, 0)
	options.Background = nil
	options.MaxPixels = min(rasterMaxPixels, remainingPixels)
	options.MaxNodes = max(1, rasterMaxNodes/totalBoards)
	options.MaxPathCommands = max(1, rasterMaxPathCommands/totalBoards)
	options.MaxTextCoverageChecks = max(int64(1), rasterMaxTextCoverageChecks/int64(totalBoards))
	options.MaxTextShapingRuns = max(1, rasterMaxTextShapingRuns/totalBoards)
	options.MaxAnimationTracks = max(1, rasterMaxAnimationTracks/totalBoards)
	options.MaxAnimationKeyframes = max(1, rasterMaxAnimationKeyframes/totalBoards)
	options.MaxEvenOddClipWork = max(int64(1), rasterMaxEvenOddClipWork/int64(totalBoards))
	options.MaxScanlineWork = max(int64(1), rasterMaxScanlineWork/int64(totalBoards))
	return options, nil
}

func pagedSceneAssetOptions(inputPath string, cacheImages bool, boardCount int) (*d2scenebuild.AssetOptions, error) {
	budget, err := divideSVGImportBudget(svgImportBudget(), boardCount)
	if err != nil {
		return nil, err
	}
	return newSceneAssetOptions(inputPath, cacheImages, assetSessionLimits{
		maxDecodedPixels:          rasterMaxPixels,
		maxAssets:                 imageAssetMaxCount,
		maxCumulativeEncodedBytes: imageAssetMaxCumulativeEncodedBytes,
		maxCumulativeDecodedBytes: imageAssetMaxCumulativeDecodedBytes,
		svgImportBudget:           budget,
	})
}

func imagePixelCount(frame image.Image) (int64, error) {
	if frame == nil {
		return 0, fmt.Errorf("renderer returned a nil frame")
	}
	bounds := frame.Bounds()
	if bounds.Empty() {
		return 0, fmt.Errorf("renderer returned an empty frame")
	}
	maxInt64 := int64(^uint64(0) >> 1)
	if int64(bounds.Dx()) > maxInt64/int64(bounds.Dy()) {
		return 0, fmt.Errorf("frame pixel count overflows int64")
	}
	return int64(bounds.Dx()) * int64(bounds.Dy()), nil
}

// mapPagedLinks applies the same viewbox-to-frame mapping as d2raster.
// Keeping annotations in source-PNG pixels makes their later PDF/PPTX fitting
// independent of logical scale, centering, and fractional output dimensions.
func mapPagedLinks(ctx context.Context, document *d2scene.Document, frame image.Image, scale float64) ([]d2scene.LinkRegion, error) {
	if document == nil {
		return nil, fmt.Errorf("cannot map links for a nil document")
	}
	if len(document.Links) == 0 {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if frame == nil || frame.Bounds().Empty() || frame.Bounds().Min.X != 0 || frame.Bounds().Min.Y != 0 {
		return nil, fmt.Errorf("renderer returned an invalid source-pixel frame")
	}
	widthFloat := document.LogicalWidth * scale
	heightFloat := document.LogicalHeight * scale
	if math.IsNaN(widthFloat) || math.IsInf(widthFloat, 0) || math.IsNaN(heightFloat) || math.IsInf(heightFloat, 0) ||
		widthFloat <= 0 || heightFloat <= 0 || document.ViewBox.Width <= 0 || document.ViewBox.Height <= 0 {
		return nil, fmt.Errorf("document has invalid viewport dimensions")
	}
	scaleX := widthFloat / document.ViewBox.Width
	scaleY := heightFloat / document.ViewBox.Height
	offsetX, offsetY := 0.0, 0.0
	if document.ViewportFit == d2scene.ViewportMeet {
		uniformScale := math.Min(float64(frame.Bounds().Dx())/document.ViewBox.Width, float64(frame.Bounds().Dy())/document.ViewBox.Height)
		scaleX, scaleY = uniformScale, uniformScale
		if document.ViewportAlign == d2scene.ViewportAlignXMidYMid {
			offsetX = (float64(frame.Bounds().Dx()) - document.ViewBox.Width*uniformScale) / 2
			offsetY = (float64(frame.Bounds().Dy()) - document.ViewBox.Height*uniformScale) / 2
		}
	}
	mapping := d2scene.Matrix{
		A: scaleX, D: scaleY,
		E: offsetX - document.ViewBox.X*scaleX,
		F: offsetY - document.ViewBox.Y*scaleY,
	}
	if !mapping.IsFinite() {
		return nil, fmt.Errorf("document link mapping is non-finite")
	}
	result := make([]d2scene.LinkRegion, 0, len(document.Links))
	for index, link := range document.Links {
		if index&255 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		topLeft := mapping.Point(d2scene.Point{X: link.Box.X, Y: link.Box.Y})
		bottomRight := mapping.Point(d2scene.Point{X: link.Box.X + link.Box.Width, Y: link.Box.Y + link.Box.Height})
		mapped := link
		mapped.Box = d2scene.Box{
			X: topLeft.X, Y: topLeft.Y,
			Width: bottomRight.X - topLeft.X, Height: bottomRight.Y - topLeft.Y,
		}
		if math.IsNaN(mapped.Box.X) || math.IsInf(mapped.Box.X, 0) ||
			math.IsNaN(mapped.Box.Y) || math.IsInf(mapped.Box.Y, 0) ||
			math.IsNaN(mapped.Box.Width) || math.IsInf(mapped.Box.Width, 0) ||
			math.IsNaN(mapped.Box.Height) || math.IsInf(mapped.Box.Height, 0) ||
			mapped.Box.Width <= 0 || mapped.Box.Height <= 0 {
			return nil, fmt.Errorf("link %d maps to an invalid source-pixel box", index)
		}
		result = append(result, mapped)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// indexPagedBoards is the canonical page index for PDF/PPTX.
// Folder-only nodes remain in board IDs but never consume a page number, so
// neither body links nor PPTX breadcrumb relationships can name a nonexistent
// page. The traversal order is exactly the render order below.
func indexPagedBoards(ctx context.Context, root *d2target.Diagram) (map[string]int, int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if root == nil {
		return nil, 0, fmt.Errorf("paged export: nil diagram")
	}
	active := make(map[*d2target.Diagram]struct{})
	boardIDToPage := make(map[string]int)
	visited, boards := 0, 0
	var walk func(*d2target.Diagram, string, int) error
	walk = func(diagram *d2target.Diagram, boardID string, depth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if diagram == nil {
			return fmt.Errorf("paged board tree contains a nil diagram")
		}
		if depth > rasterMaxDepth {
			return fmt.Errorf("paged board tree exceeds depth %d", rasterMaxDepth)
		}
		visited++
		if visited > pagedMaxTreeNodes {
			return fmt.Errorf("paged board tree exceeds %d total nodes", pagedMaxTreeNodes)
		}
		if _, ok := active[diagram]; ok {
			return fmt.Errorf("paged board tree contains a cycle")
		}
		active[diagram] = struct{}{}
		defer delete(active, diagram)
		if !diagram.IsFolderOnly {
			if _, exists := boardIDToPage[boardID]; exists {
				return fmt.Errorf("paged board tree contains duplicate board ID %q", boardID)
			}
			boardIDToPage[boardID] = boards
			boards++
			if boards > pagedMaxBoards {
				return fmt.Errorf("paged board count exceeds %d", pagedMaxBoards)
			}
		}
		for _, group := range []struct {
			name     string
			children []*d2target.Diagram
		}{{LAYERS, diagram.Layers}, {SCENARIOS, diagram.Scenarios}, {STEPS, diagram.Steps}} {
			for _, child := range group.children {
				childName := ""
				if child != nil {
					childName = child.Name
				}
				childID := boardID + "." + group.name + "." + childName
				if err := walk(child, childID, depth+1); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(root, "root", 1); err != nil {
		return nil, 0, err
	}
	return boardIDToPage, boards, nil
}

func renderPDFWithStatus(ctx context.Context, plugin d2plugin.Plugin, opts d2svg.RenderOpts, inputPath, outputPath string, cacheImages bool, ruler *textmeasure.Ruler, diagram *d2target.Diagram, rootPath []pdf.BoardTitle, includeNav, wantPreview bool) ([]byte, bool, error) {
	return renderPDFWithExporter(ctx, plugin, opts, inputPath, cacheImages, ruler, diagram, rootPath, includeNav, wantPreview, func(document *pdf.GoFPDF) (bool, error) {
		return document.ExportWithStatus(outputPath)
	})
}

func renderPDFTo(ctx context.Context, plugin d2plugin.Plugin, opts d2svg.RenderOpts, inputPath string, output io.Writer, cacheImages bool, ruler *textmeasure.Ruler, diagram *d2target.Diagram, rootPath []pdf.BoardTitle, includeNav, wantPreview bool) ([]byte, error) {
	preview, _, err := renderPDFWithExporter(ctx, plugin, opts, inputPath, cacheImages, ruler, diagram, rootPath, includeNav, wantPreview, func(document *pdf.GoFPDF) (bool, error) {
		return false, document.ExportTo(output)
	})
	return preview, err
}

func renderPDFWithExporter(ctx context.Context, plugin d2plugin.Plugin, opts d2svg.RenderOpts, inputPath string, cacheImages bool, ruler *textmeasure.Ruler, diagram *d2target.Diagram, rootPath []pdf.BoardTitle, includeNav, wantPreview bool, export func(*pdf.GoFPDF) (bool, error)) ([]byte, bool, error) {
	renderer, err := newPagedRenderer(ctx, plugin, inputPath, cacheImages, diagram, opts)
	if err != nil {
		return nil, false, err
	}
	return renderPDFWithRenderer(renderer, opts, ruler, diagram, rootPath, includeNav, wantPreview, export)
}

func renderPDFWithRenderer(renderer pagedBoardRenderer, opts d2svg.RenderOpts, ruler *textmeasure.Ruler, diagram *d2target.Diagram, rootPath []pdf.BoardTitle, includeNav, wantPreview bool, export func(*pdf.GoFPDF) (bool, error)) ([]byte, bool, error) {
	defer renderer.close()
	ctx, boardIDToPage, totalBoards := renderer.info()
	renderedBoards := 0
	var err error
	rootPath, err = pdfRootPath(rootPath, diagram.IsFolderOnly)
	if err != nil {
		return nil, false, err
	}
	document := pdf.Init()
	var preview []byte
	var walk func(*d2target.Diagram, string, []pdf.BoardTitle, bool) error
	walk = func(board *d2target.Diagram, boardID string, boardPath []pdf.BoardTitle, wantsPreview bool) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !board.IsFolderOnly {
			if page, ok := boardIDToPage[boardID]; !ok || page != renderedBoards {
				return fmt.Errorf("PDF board %q has page index %d/%v, want %d", boardID, page, ok, renderedBoards)
			}
			rootFill := board.Root.Fill
			rendered, err := renderer.render(board, wantsPreview)
			if err != nil {
				return err
			}
			if wantsPreview {
				preview = rendered.preview
			}
			linkShapes, err := pdfLinkShapes(ctx, rendered.links, boardIDToPage)
			if err != nil {
				return err
			}
			if err := document.AddPDFPage(rendered.png, boardPath, themeID(opts), rootFill, linkShapes, padding(opts), 0, 0, boardIDToPage, includeNav); err != nil {
				return err
			}
			renderedBoards++
		}
		for _, group := range []struct {
			name     string
			children []*d2target.Diagram
		}{{LAYERS, board.Layers}, {SCENARIOS, board.Scenarios}, {STEPS, board.Steps}} {
			for _, child := range group.children {
				childID := boardID + "." + group.name + "." + child.Name
				path := boardPath
				if !child.IsFolderOnly {
					path = appendPDFBoardTitle(boardPath, pdf.BoardTitle{Name: child.Root.Label, BoardID: childID})
				}
				if err := walk(child, childID, path, false); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(diagram, "root", rootPath, wantPreview && !diagram.IsFolderOnly); err != nil {
		return preview, false, err
	}
	if renderedBoards != totalBoards {
		return preview, false, fmt.Errorf("paged export rendered %d boards, want %d", renderedBoards, totalBoards)
	}
	if wantPreview {
		preview, err = appendRasterPreview(diagram, opts, ruler, preview)
		if err != nil {
			return nil, false, err
		}
	}
	touched, err := runStatusFinalizer(ctx, func() (bool, error) { return export(document) })
	return preview, touched, err
}

func renderPPTX(ctx context.Context, presentation *pptx.Presentation, plugin d2plugin.Plugin, opts d2svg.RenderOpts, inputPath string, cacheImages bool, ruler *textmeasure.Ruler, diagram *d2target.Diagram, rootPath []pptx.BoardTitle, wantPreview bool) ([]byte, error) {
	if presentation == nil {
		return nil, fmt.Errorf("PPTX export requires a presentation")
	}
	renderer, err := newPagedRenderer(ctx, plugin, inputPath, cacheImages, diagram, opts)
	if err != nil {
		return nil, err
	}
	return renderPPTXWithRenderer(renderer, presentation, opts, ruler, diagram, rootPath, wantPreview)
}

func renderPPTXWithRenderer(renderer pagedBoardRenderer, presentation *pptx.Presentation, opts d2svg.RenderOpts, ruler *textmeasure.Ruler, diagram *d2target.Diagram, rootPath []pptx.BoardTitle, wantPreview bool) ([]byte, error) {
	defer renderer.close()
	ctx, boardIDToPage, totalBoards := renderer.info()
	renderedBoards := 0
	if presentation == nil {
		return nil, fmt.Errorf("PPTX export requires a presentation")
	}
	var err error
	rootPath, err = pptxRootPath(rootPath, diagram.IsFolderOnly, boardIDToPage)
	if err != nil {
		return nil, err
	}
	var preview []byte
	var walk func(*d2target.Diagram, string, []pptx.BoardTitle, bool) error
	walk = func(board *d2target.Diagram, boardID string, boardPath []pptx.BoardTitle, wantsPreview bool) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !board.IsFolderOnly {
			if page, ok := boardIDToPage[boardID]; !ok || page != renderedBoards {
				return fmt.Errorf("PPTX board %q has page index %d/%v, want %d", boardID, page, ok, renderedBoards)
			}
			rendered, err := renderer.render(board, wantsPreview)
			if err != nil {
				return err
			}
			if wantsPreview {
				preview = rendered.preview
			}
			slide, err := presentation.AddSlide(rendered.png, boardPath)
			if err != nil {
				return err
			}
			if err := addPPTXLinks(ctx, slide, rendered.links, boardIDToPage); err != nil {
				return err
			}
			renderedBoards++
		}
		for _, group := range []struct {
			name     string
			children []*d2target.Diagram
		}{{LAYERS, board.Layers}, {SCENARIOS, board.Scenarios}, {STEPS, board.Steps}} {
			for _, child := range group.children {
				childID := boardID + "." + group.name + "." + child.Name
				path := boardPath
				if !child.IsFolderOnly {
					page, ok := boardIDToPage[childID]
					if !ok {
						return fmt.Errorf("PPTX renderable board %q has no preflighted page", childID)
					}
					path = appendPPTXBoardTitle(boardPath, pptx.BoardTitle{Name: child.Name, BoardID: childID, LinkToSlide: page + 1})
				}
				if err := walk(child, childID, path, false); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(diagram, "root", rootPath, wantPreview && !diagram.IsFolderOnly); err != nil {
		return preview, err
	}
	if renderedBoards != totalBoards {
		return preview, fmt.Errorf("paged export rendered %d boards, want %d", renderedBoards, totalBoards)
	}
	if !wantPreview {
		return nil, nil
	}
	return appendRasterPreview(diagram, opts, ruler, preview)
}

// pdfLinkShapes adapts typed scene links to lib/pdf's shape-only
// annotation API. PDF destinations are preserved; tooltip text is painted in
// the static appendix because the annotation API cannot encode it, and
// tooltip-only regions are not turned into behavior-changing self-links. An
// internal target without a renderable page is an error rather than a silently
// omitted annotation. Pixel boxes are rounded outward to whole PDF points
// because d2target.Shape stores integer geometry.
func pdfLinkShapes(ctx context.Context, links []d2scene.LinkRegion, boardIDToPage map[string]int) ([]d2target.Shape, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	result := make([]d2target.Shape, 0, len(links))
	for index, region := range links {
		if index&255 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		destination := region.URL
		if region.Target != "" {
			if _, ok := boardIDToPage[region.Target]; !ok {
				return nil, fmt.Errorf("PDF link %d target %q has no renderable page", index, region.Target)
			}
			destination = region.Target
		}
		if destination == "" {
			continue
		}
		left := int(math.Floor(region.Box.X / png.SCALE))
		top := int(math.Floor(region.Box.Y / png.SCALE))
		right := int(math.Ceil((region.Box.X + region.Box.Width) / png.SCALE))
		bottom := int(math.Ceil((region.Box.Y + region.Box.Height) / png.SCALE))
		if right <= left || bottom <= top {
			return nil, fmt.Errorf("PDF link %d has an empty rounded annotation box", index)
		}
		result = append(result, d2target.Shape{
			Pos: d2target.Point{X: left, Y: top}, Width: right - left, Height: bottom - top,
			Link: destination,
		})
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// addPPTXLinks preserves explicit tooltip text on destination links.
// lib/pptx requires every tooltip shape to own a relationship, so tooltip-only
// regions are represented by the static appendix rather than receiving
// a click-changing self-link. Links are validated into a temporary slice first
// so an invalid internal target cannot partially mutate the slide.
func addPPTXLinks(ctx context.Context, slide *pptx.Slide, links []d2scene.LinkRegion, boardIDToPage map[string]int) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if slide == nil {
		return fmt.Errorf("PPTX cannot add links to a nil slide")
	}
	prepared := make([]*pptx.Link, 0, len(links))
	for index, region := range links {
		if index&255 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		link := &pptx.Link{Tooltip: region.Tooltip}
		switch {
		case region.Target != "":
			page, ok := boardIDToPage[region.Target]
			if !ok {
				return fmt.Errorf("PPTX link %d target %q has no renderable slide", index, region.Target)
			}
			link.SlideIndex = page + 1
		case region.URL != "":
			// Markdown links retain URL provenance so relative URLs such as
			// root.html are not rejected as missing boards. An exact rendered
			// board ID remains navigable within the presentation.
			if page, ok := boardIDToPage[region.URL]; ok {
				link.SlideIndex = page + 1
			} else {
				link.ExternalUrl = region.URL
			}
		default:
			continue
		}
		left := int(math.Floor(region.Box.X))
		top := int(math.Floor(region.Box.Y))
		right := int(math.Ceil(region.Box.X + region.Box.Width))
		bottom := int(math.Ceil(region.Box.Y + region.Box.Height))
		if right <= left || bottom <= top {
			return fmt.Errorf("PPTX link %d has an empty rounded annotation box", index)
		}
		link.Left, link.Top = left, top
		link.Width, link.Height = right-left, bottom-top
		prepared = append(prepared, link)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, link := range prepared {
		slide.AddLink(link)
	}
	return nil
}

func pdfRootPath(rootPath []pdf.BoardTitle, folderOnly bool) ([]pdf.BoardTitle, error) {
	if len(rootPath) != 1 || rootPath[0].BoardID != "root" {
		return nil, fmt.Errorf("PDF root path must contain exactly the root board")
	}
	if folderOnly {
		return nil, nil
	}
	result := append([]pdf.BoardTitle(nil), rootPath...)
	return result, nil
}

func pptxRootPath(rootPath []pptx.BoardTitle, folderOnly bool, boardIDToPage map[string]int) ([]pptx.BoardTitle, error) {
	if len(rootPath) != 1 || rootPath[0].BoardID != "root" {
		return nil, fmt.Errorf("PPTX root path must contain exactly the root board")
	}
	if folderOnly {
		return nil, nil
	}
	page, ok := boardIDToPage["root"]
	if !ok {
		return nil, fmt.Errorf("PPTX root board has no preflighted page")
	}
	result := append([]pptx.BoardTitle(nil), rootPath...)
	result[0].LinkToSlide = page + 1
	return result, nil
}

func themeID(opts d2svg.RenderOpts) int64 {
	if opts.ThemeID == nil {
		return 0
	}
	return *opts.ThemeID
}

func padding(opts d2svg.RenderOpts) int64 {
	if opts.Pad == nil {
		return d2scenebuild.DefaultPadding
	}
	return *opts.Pad
}

func appendPDFBoardTitle(path []pdf.BoardTitle, title pdf.BoardTitle) []pdf.BoardTitle {
	result := make([]pdf.BoardTitle, len(path), len(path)+1)
	copy(result, path)
	return append(result, title)
}

func appendPPTXBoardTitle(path []pptx.BoardTitle, title pptx.BoardTitle) []pptx.BoardTitle {
	result := make([]pptx.BoardTitle, len(path), len(path)+1)
	copy(result, path)
	return append(result, title)
}
