package d2cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/d2lang/d2/d2renderers/d2isometric/d2isometricimg"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/d2svg"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/pdf"
	"github.com/d2lang/d2/lib/pptx"
	"github.com/d2lang/d2/lib/version"
	"github.com/d2lang/util-go/xmain"
)

// Native pages share the regular paged exporters' operation budgets and asset
// resolver. Each scene is released before the next is built; only encoded page
// images and annotations remain in the PDF or presentation writer.
type isometricPagedRenderer struct {
	ctx                                context.Context
	opts                               d2isometricimg.Options
	boardIDToPage                      map[string]int
	sourceRootID                       string
	totalBoards, renderedBoards        int
	remainingPixels, remainingEncoded  int64
	remainingLinks, remainingLinkBytes int
}

func newIsometricPagedRenderer(ctx context.Context, ms *xmain.State, diagram *d2target.Diagram, render d2svg.RenderOpts, input, sourceRootID string) (*isometricPagedRenderer, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	pages, count, err := indexPagedBoards(ctx, diagram)
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, fmt.Errorf("isometric paged export requires at least one renderable board")
	}
	o, err := configuredIsometricImageOptions(ms, render, input, isometricPNG)
	if err != nil {
		return nil, err
	}
	// Admit the maximum requested canvas before allocating a scene. FitContent
	// can shrink it, but never permits the operation to exceed this upper bound.
	if int64(o.Width)*int64(o.Height)*int64(count) > pagedMaxTotalPixels {
		return nil, fmt.Errorf("isometric paged export exceeds the %d-pixel operation limit; reduce --scale or select fewer boards", pagedMaxTotalPixels)
	}
	o.Assets, err = pagedSceneAssetOptions(input, ms.Env.Getenv("IMG_CACHE") == "1", count)
	if err != nil {
		return nil, err
	}
	o.Fonts, err = newFontFallbackOptions(count)
	if err != nil {
		return nil, err
	}
	links, err := pagedLinkBudget(count)
	if err != nil {
		return nil, err
	}
	o.LinkBudget = &links
	return &isometricPagedRenderer{ctx: ctx, opts: o, boardIDToPage: pages, sourceRootID: sourceRootID, totalBoards: count,
		remainingPixels: pagedMaxTotalPixels, remainingEncoded: pagedMaxEncodedBytes,
		remainingLinks: pagedMaxLinkRegions, remainingLinkBytes: pagedMaxLinkStringBytes}, nil
}

func (r *isometricPagedRenderer) close() {}
func (r *isometricPagedRenderer) info() (context.Context, map[string]int, int) {
	return r.ctx, r.boardIDToPage, r.totalBoards
}

func (r *isometricPagedRenderer) render(diagram *d2target.Diagram, wantsPreview bool) (*pagedBoard, error) {
	if err := r.ctx.Err(); err != nil {
		return nil, err
	}
	if wantsPreview {
		return nil, fmt.Errorf("isometric paged exports do not produce SVG previews")
	}
	if diagram == nil || diagram.IsFolderOnly {
		return nil, fmt.Errorf("isometric paged export requires a drawable board")
	}
	if r.renderedBoards >= r.totalBoards {
		return nil, fmt.Errorf("isometric paged export exceeded its preflighted board count")
	}
	board := *diagram
	board.Layers, board.Scenarios, board.Steps = nil, nil, nil
	o := r.opts
	page, err := d2isometricimg.RenderPage(r.ctx, &board, &o)
	if err != nil {
		return nil, fmt.Errorf("isometric page %d: %w", r.renderedBoards+1, err)
	}
	if err := r.admitPage(page); err != nil {
		return nil, err
	}
	links, err := rebaseIsometricPageLinks(page.Links, r.sourceRootID, r.boardIDToPage)
	if err != nil {
		return nil, err
	}
	r.renderedBoards++
	return &pagedBoard{png: page.PNG, links: links}, nil
}

func rebaseIsometricPageLinks(links []d2scene.LinkRegion, sourceRootID string, pages map[string]int) ([]d2scene.LinkRegion, error) {
	if sourceRootID == "" || sourceRootID == "root" {
		return links, nil
	}
	result := append([]d2scene.LinkRegion(nil), links...)
	for i := range result {
		link := &result[i]
		if link.Target != "" {
			if link.Target != sourceRootID && !strings.HasPrefix(link.Target, sourceRootID+".") {
				return nil, fmt.Errorf("isometric page link target %q lies outside selected board %q", link.Target, sourceRootID)
			}
			link.Target = "root" + strings.TrimPrefix(link.Target, sourceRootID)
		} else if link.URL == sourceRootID || strings.HasPrefix(link.URL, sourceRootID+".") {
			local := "root" + strings.TrimPrefix(link.URL, sourceRootID)
			if _, ok := pages[local]; ok {
				link.Target, link.URL = local, ""
			}
		}
	}
	return result, nil
}

func (r *isometricPagedRenderer) admitPage(page *d2isometricimg.Page) error {
	if page == nil || page.Width <= 0 || page.Height <= 0 || page.Width > r.opts.Width || page.Height > r.opts.Height {
		return fmt.Errorf("isometric page has invalid image bounds")
	}
	pixels := int64(page.Width) * int64(page.Height)
	if pixels > r.remainingPixels {
		return fmt.Errorf("isometric paged export exhausted its pixel budget")
	}
	if len(page.PNG) == 0 || int64(len(page.PNG)) > r.remainingEncoded {
		return fmt.Errorf("isometric paged export exhausted its encoded image budget")
	}
	if len(page.Links) > r.remainingLinks {
		return fmt.Errorf("isometric paged export exhausted its link-region budget")
	}
	linkBytes := 0
	for i, link := range page.Links {
		if i&255 == 0 {
			if err := r.ctx.Err(); err != nil {
				return err
			}
		}
		box := link.Box
		for _, v := range []float64{box.X, box.Y, box.Width, box.Height} {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return fmt.Errorf("isometric page link has non-finite coordinates")
			}
		}
		if box.X < 0 || box.Y < 0 || box.Width <= 0 || box.Height <= 0 || box.X+box.Width > float64(page.Width)+1e-6 || box.Y+box.Height > float64(page.Height)+1e-6 {
			return fmt.Errorf("isometric page link lies outside the rendered image")
		}
		for _, s := range []string{link.URL, link.Target, link.Tooltip} {
			if len(s) > r.remainingLinkBytes-linkBytes {
				return fmt.Errorf("isometric paged export exhausted its link-string budget")
			}
			linkBytes += len(s)
		}
	}
	r.remainingPixels -= pixels
	r.remainingEncoded -= int64(len(page.PNG))
	r.remainingLinks -= len(page.Links)
	r.remainingLinkBytes -= linkBytes
	return nil
}

func renderIsometricPaged(ctx context.Context, ms *xmain.State, diagram *d2target.Diagram, render d2svg.RenderOpts, input, output string, ext exportExtension, sourceRootID string) ([]byte, bool, error) {
	if ext != isometricPDF && ext != isometricPPTX {
		return nil, false, fmt.Errorf("isometric paged export requires PDF or PPTX")
	}
	renderer, err := newIsometricPagedRenderer(ctx, ms, diagram, render, input, sourceRootID)
	if err != nil {
		return nil, false, err
	}
	var encoded bytes.Buffer
	writer := &isometricDocumentWriter{ctx: renderer.ctx, output: &encoded, remaining: pagedMaxEncodedBytes}
	includeNav := diagram.Root.Label != ""
	if ext == isometricPDF {
		_, _, err = renderPDFWithRenderer(renderer, render, nil, diagram,
			[]pdf.BoardTitle{{Name: diagram.Root.Label, BoardID: "root"}}, includeNav, false,
			func(document *pdf.GoFPDF) (bool, error) { return false, document.ExportTo(writer) })
	} else {
		name := getFileName(output)
		if output == "-" {
			name = getFileName(input)
			if input == "-" {
				name = "stdin"
			}
		}
		presentation := pptx.NewPresentation(name, "Presentation generated with D2 - https://d2lang.com", name, "", version.OnlyNumbers(), includeNav)
		_, err = renderPPTXWithRenderer(renderer, presentation, render, nil, diagram,
			[]pptx.BoardTitle{{Name: "root", BoardID: "root"}}, false)
		if err == nil {
			err = presentation.ExportTo(writer)
		}
	}
	if err != nil {
		return nil, false, err
	}
	return writeIsometricImage(renderer.ctx, ms, output, encoded.Bytes())
}

// Encoding completes into bounded memory before stdout or an existing file is
// touched. Cancellation and writer failures never publish a partial document.
type isometricDocumentWriter struct {
	ctx       context.Context
	output    io.Writer
	remaining int64
}

func (w *isometricDocumentWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	if int64(len(p)) > w.remaining {
		return 0, fmt.Errorf("isometric paged document exceeds the %d-byte output limit", pagedMaxEncodedBytes)
	}
	n, err := w.output.Write(p)
	w.remaining -= int64(n)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	return n, err
}
