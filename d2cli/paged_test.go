package d2cli

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d2lang/util-go/go2"

	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/d2svg"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/internal/testutil"
	"github.com/d2lang/d2/internal/testutil/imagediff"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/pdf"
	d2png "github.com/d2lang/d2/lib/png"
	d2pptx "github.com/d2lang/d2/lib/pptx"
	"github.com/d2lang/d2/lib/textmeasure"
)

func TestPagedContainersEmbedEquivalentPixels(t *testing.T) {
	root := simpleRasterDiagramWithLabel()
	root.Name = "root"
	opts := d2svg.RenderOpts{Pad: go2.Pointer(int64(0)), Scale: go2.Pointer(1.0)}
	expectedRenderer, err := newPagedRenderer(context.Background(), nil, "-", false, root, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(expectedRenderer.close)
	expected, err := expectedRenderer.render(root, false)
	if err != nil {
		t.Fatal(err)
	}

	presentation := d2pptx.NewPresentation("pixels", "", "", "", "1", true)
	preview, err := renderPPTX(
		context.Background(), presentation, nil, opts, "-", false, nil, root,
		[]d2pptx.BoardTitle{{Name: "root", BoardID: "root", LinkToSlide: 1}},
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview) != 0 {
		t.Fatalf("unrequested PPTX preview = %q", preview)
	}
	pptxPath := filepath.Join(t.TempDir(), "pixels.pptx")
	if err := presentation.SaveTo(pptxPath); err != nil {
		t.Fatal(err)
	}
	pptxContent, err := os.ReadFile(pptxPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := d2pptx.Validate(pptxContent, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := testutil.ComparePPTXImages(pptxContent, [][]byte{expected.png}, imagediff.Options{}); err != nil {
		t.Fatal(err)
	}

	ruler, err := textmeasure.NewRuler()
	if err != nil {
		t.Fatal(err)
	}
	pdfPath := filepath.Join(t.TempDir(), "pixels.pdf")
	if _, _, err := renderPDFWithStatus(
		context.Background(), nil, opts, "-", pdfPath, false, ruler, root,
		[]pdf.BoardTitle{{Name: "root", BoardID: "root"}}, true, true,
	); err != nil {
		t.Fatal(err)
	}
	pdfContent, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testutil.ComparePDFImages(pdfContent, [][]byte{expected.png}, imagediff.Options{}); err != nil {
		t.Fatal(err)
	}
	inspection, err := testutil.InspectD2PDF(pdfContent)
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Pages) != 1 || len(inspection.Images) != 1 ||
		inspection.Images[0].Width != 20 || inspection.Images[0].Height != 20 ||
		inspection.Pages[0].Width != 576 || inspection.Pages[0].Height != 648 {
		t.Fatalf("PDF structure = pages %+v, images %+v", inspection.Pages, inspection.Images)
	}
}

func TestPagedRendererPreservesPreorderScaleAndPreview(t *testing.T) {
	root := simpleRasterDiagramWithLabel()
	root.Root.Fill = "#abcdef"
	layer := simpleRasterDiagramWithLabel()
	root.Layers = []*d2target.Diagram{layer}
	renderer, err := newPagedRenderer(
		context.Background(), nil, "-", false, root,
		d2svg.RenderOpts{Pad: go2.Pointer(int64(0)), Scale: go2.Pointer(1.0)},
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(renderer.close)
	if renderer.totalBoards != 2 {
		t.Fatalf("preflighted boards = %d, want 2", renderer.totalBoards)
	}
	first, err := renderer.render(root, true)
	if err != nil {
		t.Fatal(err)
	}
	firstEncoderBuffer := renderer.pngEncoder.generic.buffer
	if firstEncoderBuffer == nil || renderer.pngEncoder.genericImage.Pix != nil {
		t.Fatal("first paged board did not retain only reusable PNG encoder scratch")
	}
	second, err := renderer.render(layer, false)
	if err != nil {
		t.Fatal(err)
	}
	if renderer.pngEncoder.generic.buffer != firstEncoderBuffer || renderer.pngEncoder.genericImage.Pix != nil {
		t.Fatal("second paged board did not reuse PNG scratch or retained its source frame")
	}
	for index, board := range []*pagedBoard{first, second} {
		decoded, err := png.Decode(bytes.NewReader(board.png))
		if err != nil {
			t.Fatal(err)
		}
		if decoded.Bounds().Dx() != 20 || decoded.Bounds().Dy() != 20 {
			t.Fatalf("board %d PNG bounds = %v, want 20x20", index, decoded.Bounds())
		}
	}
	if !bytes.Contains(first.preview, []byte("<svg")) || len(second.preview) != 0 {
		t.Fatalf("root/child preview lengths = %d/%d", len(first.preview), len(second.preview))
	}
	if renderer.renderedBoards != 2 {
		t.Fatalf("rendered boards = %d, want 2", renderer.renderedBoards)
	}
	stats := renderer.session.Stats()
	if stats.Misses == 0 || stats.Hits == 0 {
		t.Fatalf("paged render cache misses/hits = %d/%d, want both positive", stats.Misses, stats.Hits)
	}
	if stats.SkippedOversize != 0 || stats.MemoSkipped != 0 {
		t.Fatalf("paged render cache skipped state: %+v", stats)
	}
	if root.Root.Fill != "#abcdef" {
		t.Fatalf("paged render mutated root fill to %q", root.Root.Fill)
	}
}

func TestPagedRendererBoundsAggregatePixelsAndEncoding(t *testing.T) {
	newRenderer := func(t *testing.T) *pagedRenderer {
		t.Helper()
		renderer, err := newPagedRenderer(
			context.Background(), nil, "-", false, simpleRasterDiagram(),
			d2svg.RenderOpts{Pad: go2.Pointer(int64(0)), Scale: go2.Pointer(1.0)},
		)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(renderer.close)
		return renderer
	}

	pixelLimited := newRenderer(t)
	pixelLimited.remainingPixels = 399
	if _, err := pixelLimited.render(simpleRasterDiagram(), false); err == nil || !strings.Contains(err.Error(), "frame pixels exceed limit 399") {
		t.Fatalf("aggregate pixel limit error = %v", err)
	}

	encodedLimited := newRenderer(t)
	encodedLimited.remainingEncoded = 1
	if _, err := encodedLimited.render(simpleRasterDiagram(), false); err == nil || !strings.Contains(err.Error(), "encoded PNG bytes") {
		t.Fatalf("aggregate encoded limit error = %v", err)
	}
}

func TestPagedFrameOptionsDivideAggregateWork(t *testing.T) {
	options, err := pagedFrameOptions(4, 1234)
	if err != nil {
		t.Fatal(err)
	}
	if options.Scale != d2png.SCALE || options.Background != nil || options.MaxPixels != 1234 ||
		options.MaxNodes != rasterMaxNodes/4 || options.MaxPathCommands != rasterMaxPathCommands/4 ||
		options.MaxTextCoverageChecks != rasterMaxTextCoverageChecks/4 ||
		options.MaxTextShapingRuns != rasterMaxTextShapingRuns/4 ||
		options.MaxAnimationTracks != rasterMaxAnimationTracks/4 ||
		options.MaxAnimationKeyframes != rasterMaxAnimationKeyframes/4 ||
		options.MaxEvenOddClipWork != rasterMaxEvenOddClipWork/4 ||
		options.MaxScanlineWork != rasterMaxScanlineWork/4 {
		t.Fatalf("paged frame options = %+v", options)
	}
	if _, err := pagedFrameOptions(0, 1); err == nil {
		t.Fatal("paged frame options accepted zero boards")
	}
	if _, err := pagedFrameOptions(1, 0); err == nil {
		t.Fatal("paged frame options accepted exhausted pixels")
	}
}

func TestPagedLinkBudgetDividesAggregateWork(t *testing.T) {
	one, err := pagedLinkBudget(1)
	if err != nil {
		t.Fatal(err)
	}
	if one.MaxRegions != pagedPerBoardMaxLinkRegions || one.MaxStringBytes != pagedPerBoardMaxLinkBytes {
		t.Fatalf("single-board link budget = %+v", one)
	}
	many, err := pagedLinkBudget(pagedMaxBoards)
	if err != nil {
		t.Fatal(err)
	}
	if many.MaxRegions != pagedMaxLinkRegions/pagedMaxBoards || many.MaxStringBytes != pagedMaxLinkStringBytes/pagedMaxBoards {
		t.Fatalf("max-board link budget = %+v", many)
	}
	if _, err := pagedLinkBudget(0); err == nil {
		t.Fatal("paged link budget accepted zero boards")
	}
}

func TestMapPagedLinksUsesRenderedViewport(t *testing.T) {
	document := d2scene.NewDocument(
		d2scene.Box{X: 10, Y: 20, Width: 20, Height: 10},
		d2scene.NewNode(nil),
	)
	document.LogicalWidth = 31.25
	document.LogicalHeight = 18.75
	document.ViewportFit = d2scene.ViewportMeet
	document.ViewportAlign = d2scene.ViewportAlignXMidYMid
	document.Links = []d2scene.LinkRegion{{
		Box: d2scene.Box{X: 15, Y: 22, Width: 4, Height: 3},
		URL: "https://example.com", Tooltip: "open",
	}}
	frame := image.NewNRGBA(image.Rect(0, 0, 63, 38))
	links, err := mapPagedLinks(context.Background(), document, frame, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 || links[0].URL != document.Links[0].URL || links[0].Tooltip != document.Links[0].Tooltip {
		t.Fatalf("mapped links = %#v", links)
	}
	want := d2scene.Box{X: 15.75, Y: 9.55, Width: 12.6, Height: 9.45}
	got := links[0].Box
	if !near(got.X, want.X) || !near(got.Y, want.Y) || !near(got.Width, want.Width) || !near(got.Height, want.Height) {
		t.Fatalf("mapped link box = %+v, want %+v", got, want)
	}
}

func TestCountPagedBoardsRejectsCyclesNilAndLimits(t *testing.T) {
	root := simpleRasterDiagram()
	root.Layers = []*d2target.Diagram{simpleRasterDiagram()}
	root.Scenarios = []*d2target.Diagram{{IsFolderOnly: true, Steps: []*d2target.Diagram{simpleRasterDiagram()}}}
	if _, got, err := indexPagedBoards(context.Background(), root); err != nil || got != 3 {
		t.Fatalf("countPagedBoards = %d/%v, want 3/nil", got, err)
	}

	cyclic := simpleRasterDiagram()
	cyclic.Layers = []*d2target.Diagram{cyclic}
	if _, _, err := indexPagedBoards(context.Background(), cyclic); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cycle error = %v", err)
	}
	nilChild := simpleRasterDiagram()
	nilChild.Layers = []*d2target.Diagram{nil}
	if _, _, err := indexPagedBoards(context.Background(), nilChild); err == nil || !strings.Contains(err.Error(), "nil diagram") {
		t.Fatalf("nil-child error = %v", err)
	}

	tooMany := &d2target.Diagram{IsFolderOnly: true, Layers: make([]*d2target.Diagram, pagedMaxBoards+1)}
	for index := range tooMany.Layers {
		tooMany.Layers[index] = simpleRasterDiagram()
		tooMany.Layers[index].Name = fmt.Sprint(index)
	}
	if _, _, err := indexPagedBoards(context.Background(), tooMany); err == nil || !strings.Contains(err.Error(), fmt.Sprint(pagedMaxBoards)) {
		t.Fatalf("board-count error = %v", err)
	}
}

func TestIndexPagedBoardsSkipsFoldersAndRejectsDuplicateIDs(t *testing.T) {
	root := d2target.NewDiagram()
	root.Name = "root"
	root.IsFolderOnly = true
	folder := d2target.NewDiagram()
	folder.Name = "folder"
	folder.IsFolderOnly = true
	leaf := simpleRasterDiagram()
	leaf.Name = "leaf"
	scenario := simpleRasterDiagram()
	scenario.Name = "scenario"
	folder.Steps = []*d2target.Diagram{leaf}
	root.Layers = []*d2target.Diagram{folder}
	root.Scenarios = []*d2target.Diagram{scenario}

	indices, count, err := indexPagedBoards(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int{
		"root.layers.folder.steps.leaf": 0,
		"root.scenarios.scenario":       1,
	}
	if count != len(want) || len(indices) != len(want) {
		t.Fatalf("page count/map = %d/%v, want %d/%v", count, indices, len(want), want)
	}
	for boardID, page := range want {
		if indices[boardID] != page {
			t.Fatalf("page %q = %d, want %d", boardID, indices[boardID], page)
		}
	}
	if _, ok := indices["root"]; ok {
		t.Fatal("folder-only root received a page")
	}
	if _, ok := indices["root.layers.folder"]; ok {
		t.Fatal("folder-only intermediate board received a page")
	}

	duplicateRoot := d2target.NewDiagram()
	first, second := simpleRasterDiagram(), simpleRasterDiagram()
	first.Name, second.Name = "same", "same"
	duplicateRoot.Layers = []*d2target.Diagram{first, second}
	if _, _, err := indexPagedBoards(context.Background(), duplicateRoot); err == nil || !strings.Contains(err.Error(), "duplicate board ID") {
		t.Fatalf("duplicate board ID error = %v", err)
	}
}

func TestPagedRendererRejectsMutatingPostprocessor(t *testing.T) {
	plugin := &inPlacePostProcessPlugin{}
	renderer, err := newPagedRenderer(
		context.Background(), plugin, "-", false, simpleRasterDiagram(),
		d2svg.RenderOpts{Pad: go2.Pointer(int64(0)), Scale: go2.Pointer(1.0)},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := renderer.render(simpleRasterDiagram(), false); err == nil || !strings.Contains(err.Error(), "postprocessor") {
		t.Fatalf("mutating postprocessor error = %v", err)
	}
	if !plugin.called || renderer.renderedBoards != 0 {
		t.Fatalf("postprocessor called/rendered = %v/%d, want true/0", plugin.called, renderer.renderedBoards)
	}
}

func TestRenderPPTXPreservesRenderablePreorder(t *testing.T) {
	root := simpleRasterDiagram()
	root.Name = "root"
	layer := simpleRasterDiagram()
	layer.Name = "layer"
	scenario := simpleRasterDiagram()
	scenario.Name = "scenario"
	step := simpleRasterDiagram()
	step.Name = "step"
	root.Layers = []*d2target.Diagram{layer}
	root.Scenarios = []*d2target.Diagram{scenario}
	root.Steps = []*d2target.Diagram{step}

	indices, _, err := indexPagedBoards(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	presentation := d2pptx.NewPresentation("test", "", "", "", "1", true)
	preview, err := renderPPTX(
		context.Background(), presentation, nil,
		d2svg.RenderOpts{Pad: go2.Pointer(int64(0)), Scale: go2.Pointer(1.0)},
		"-", false, nil, root,
		[]d2pptx.BoardTitle{{Name: "root", BoardID: "root", LinkToSlide: indices["root"] + 1}},
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(preview, []byte("<svg")) {
		t.Fatalf("PPTX preview does not contain SVG: %q", preview)
	}
	wantIDs := []string{"root", "root.layers.layer", "root.scenarios.scenario", "root.steps.step"}
	if len(presentation.Slides) != len(wantIDs) {
		t.Fatalf("PPTX slides = %d, want %d", len(presentation.Slides), len(wantIDs))
	}
	for index, wantID := range wantIDs {
		path := presentation.Slides[index].BoardTitle
		if len(path) == 0 || path[len(path)-1].BoardID != wantID {
			t.Fatalf("PPTX slide %d path = %+v, want final board ID %q", index, path, wantID)
		}
	}
}

func TestPagedTypedLinksExportToPDFAndPPTX(t *testing.T) {
	root := pagedMetadataDiagram()
	child := simpleRasterDiagram()
	child.Name = "next"
	root.Layers = []*d2target.Diagram{child}
	opts := d2svg.RenderOpts{Pad: go2.Pointer(int64(0)), Scale: go2.Pointer(1.25)}
	ruler, err := textmeasure.NewRuler()
	if err != nil {
		t.Fatal(err)
	}
	nonPaged, err := buildScene(context.Background(), "-", false, root, opts)
	if err != nil {
		t.Fatalf("non-paged metadata scene: %v", err)
	}
	appendix := findPagedNode(nonPaged.Root, "appendix")
	if len(nonPaged.Links) != 5 || appendix == nil {
		t.Fatalf("non-paged metadata links/appendix = %d/%v, want 5/painted", len(nonPaged.Links), appendix != nil)
	}
	appendixText := pagedNodeText(appendix)
	for _, want := range []string{"open website", "open child", "hover only", "relative documentation", "edge details"} {
		if !strings.Contains(appendixText, want) {
			t.Fatalf("paged appendix text = %q, want tooltip %q", appendixText, want)
		}
	}

	renderer, err := newPagedRenderer(context.Background(), nil, "-", false, root, opts)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := renderer.render(root, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rendered.links) != 5 {
		t.Fatalf("typed rendered links = %#v, want all 5 shape/Markdown/connection regions", rendered.links)
	}

	presentation := d2pptx.NewPresentation("test", "", "", "", "1", true)
	preview, err := renderPPTX(
		context.Background(), presentation, nil, opts, "-", false, ruler, root,
		[]d2pptx.BoardTitle{{Name: "root", BoardID: "root"}},
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(preview, []byte("<svg")) || len(presentation.Slides) != 2 {
		t.Fatalf("PPTX preview/slides = %d/%d", len(preview), len(presentation.Slides))
	}
	rootLinks := presentation.Slides[0].Links
	if len(rootLinks) != 4 {
		t.Fatalf("PPTX destination links = %#v, want external, internal, relative Markdown, and connection links", rootLinks)
	}
	linksByDestination := make(map[string]*d2pptx.Link)
	for _, link := range rootLinks {
		destination := link.ExternalUrl
		if link.SlideIndex != 0 {
			destination = fmt.Sprintf("slide:%d", link.SlideIndex)
		}
		linksByDestination[destination] = link
	}
	if link := linksByDestination["https://example.com"]; link == nil || link.Tooltip != "open website" {
		t.Fatalf("PPTX external link = %#v", link)
	}
	if link := linksByDestination["slide:2"]; link == nil || link.ExternalUrl != "" || link.Tooltip != "open child" {
		t.Fatalf("PPTX internal link = %#v", link)
	}
	if link := linksByDestination["root.html"]; link == nil || link.SlideIndex != 0 || link.Tooltip != "relative documentation" {
		t.Fatalf("PPTX relative Markdown link = %#v", link)
	}
	if link := linksByDestination["https://example.com/edge"]; link == nil || link.Tooltip != "edge details" {
		t.Fatalf("PPTX connection link = %#v", link)
	}

	pptxPath := filepath.Join(t.TempDir(), "links.pptx")
	if err := presentation.SaveTo(pptxPath); err != nil {
		t.Fatal(err)
	}
	pptxBytes, err := os.ReadFile(pptxPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := d2pptx.Validate(pptxBytes, 2); err != nil {
		t.Fatal(err)
	}
	rels := readZipMember(t, pptxBytes, "ppt/slides/_rels/slide1.xml.rels")
	slideXML := readZipMember(t, pptxBytes, "ppt/slides/slide1.xml")
	if !bytes.Contains(rels, []byte(`Target="https://example.com"`)) ||
		!bytes.Contains(rels, []byte(`Target="slide2.xml"`)) ||
		!bytes.Contains(rels, []byte(`Target="root.html"`)) ||
		bytes.Contains(rels, []byte("slide0.xml")) {
		t.Fatalf("PPTX relationships = %s", rels)
	}
	for _, tooltip := range [][]byte{[]byte(`tooltip="open website"`), []byte(`tooltip="open child"`), []byte(`tooltip="relative documentation"`), []byte(`tooltip="edge details"`)} {
		if !bytes.Contains(slideXML, tooltip) {
			t.Fatalf("PPTX slide is missing %q: %s", tooltip, slideXML)
		}
	}

	pdfPath := filepath.Join(t.TempDir(), "links.pdf")
	preview, _, err = renderPDFWithStatus(
		context.Background(), nil, opts, "-", pdfPath, false, ruler, root,
		[]pdf.BoardTitle{{Name: "root", BoardID: "root"}}, true, true,
	)
	if err != nil {
		t.Fatal(err)
	}
	pdfBytes, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(pdfBytes, []byte("%PDF-")) || !bytes.Contains(pdfBytes, []byte("https://example.com")) || !bytes.Contains(pdfBytes, []byte("root.html")) {
		t.Fatalf("PDF output does not contain its typed external destination")
	}

	shapes, err := pdfLinkShapes(context.Background(), rendered.links, renderer.boardIDToPage)
	if err != nil {
		t.Fatal(err)
	}
	if len(shapes) != 4 {
		t.Fatalf("PDF destination adapters = %#v", shapes)
	}
	destinations := make(map[string]bool, len(shapes))
	for _, shape := range shapes {
		if shape.Tooltip != "" {
			t.Fatalf("PDF adapter unexpectedly retained unsupported tooltip %q", shape.Tooltip)
		}
		destinations[shape.Link] = true
	}
	for _, destination := range []string{"https://example.com", "root.layers.next", "root.html", "https://example.com/edge"} {
		if !destinations[destination] {
			t.Fatalf("PDF destination adapters = %#v, missing %q", shapes, destination)
		}
	}
}

func TestPagedRejectsUnrenderableLinkTargetsWithoutPartialAnnotations(t *testing.T) {
	for _, target := range []string{"root.layers.missing", "root.layers.folder"} {
		t.Run(target, func(t *testing.T) {
			links := []d2scene.LinkRegion{
				{Box: d2scene.Box{Width: 10, Height: 10}, URL: "https://example.com"},
				{Box: d2scene.Box{X: 20, Width: 10, Height: 10}, Target: target},
			}
			pages := map[string]int{"root": 0, "root.layers.next": 1}

			shapes, err := pdfLinkShapes(context.Background(), links, pages)
			if err == nil || !strings.Contains(err.Error(), target) || !strings.Contains(err.Error(), "no renderable page") {
				t.Fatalf("PDF target error = %v", err)
			}
			if shapes != nil {
				t.Fatalf("PDF returned partial annotations: %#v", shapes)
			}

			slide := &d2pptx.Slide{ImageScaleFactor: 1}
			err = addPPTXLinks(context.Background(), slide, links, pages)
			if err == nil || !strings.Contains(err.Error(), target) || !strings.Contains(err.Error(), "no renderable slide") {
				t.Fatalf("PPTX target error = %v", err)
			}
			if len(slide.Links) != 0 {
				t.Fatalf("PPTX retained partial annotations: %#v", slide.Links)
			}
		})
	}
}

func TestPagedMarkdownURLsResolveOnlyExactBoardTargets(t *testing.T) {
	links := []d2scene.LinkRegion{
		{Box: d2scene.Box{Width: 10, Height: 10}, URL: "root.html", Tooltip: "relative"},
		{Box: d2scene.Box{X: 20, Width: 10, Height: 10}, URL: "root.layers.next", Tooltip: "board"},
	}
	pages := map[string]int{"root": 0, "root.layers.next": 1}

	shapes, err := pdfLinkShapes(context.Background(), links, pages)
	if err != nil {
		t.Fatal(err)
	}
	if len(shapes) != 2 || shapes[0].Link != "root.html" || shapes[1].Link != "root.layers.next" {
		t.Fatalf("PDF Markdown destinations = %#v", shapes)
	}

	slide := &d2pptx.Slide{ImageScaleFactor: 1}
	if err := addPPTXLinks(context.Background(), slide, links, pages); err != nil {
		t.Fatal(err)
	}
	if len(slide.Links) != 2 || slide.Links[0].ExternalUrl != "root.html" || slide.Links[0].SlideIndex != 0 ||
		slide.Links[1].ExternalUrl != "" || slide.Links[1].SlideIndex != 2 {
		t.Fatalf("PPTX Markdown destinations = %#v", slide.Links)
	}
}

func TestRenderPPTXFolderOnlyPathsNeverTargetMissingSlides(t *testing.T) {
	root := d2target.NewDiagram()
	root.Name = "root"
	root.IsFolderOnly = true
	folder := d2target.NewDiagram()
	folder.Name = "folder"
	folder.IsFolderOnly = true
	leaf := simpleRasterDiagram()
	leaf.Name = "leaf"
	sibling := simpleRasterDiagram()
	sibling.Name = "sibling"
	folder.Steps = []*d2target.Diagram{leaf}
	root.Layers = []*d2target.Diagram{folder}
	root.Scenarios = []*d2target.Diagram{sibling}

	presentation := d2pptx.NewPresentation("folders", "", "", "", "1", true)
	_, err := renderPPTX(
		context.Background(), presentation, nil,
		d2svg.RenderOpts{Pad: go2.Pointer(int64(0)), Scale: go2.Pointer(1.0)},
		"-", false, nil, root,
		[]d2pptx.BoardTitle{{Name: "root", BoardID: "root"}},
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(presentation.Slides) != 2 {
		t.Fatalf("folder-only PPTX slides = %d, want 2", len(presentation.Slides))
	}
	wantIDs := []string{"root.layers.folder.steps.leaf", "root.scenarios.sibling"}
	for index, slide := range presentation.Slides {
		if len(slide.BoardTitle) != 1 || slide.BoardTitle[0].BoardID != wantIDs[index] || slide.BoardTitle[0].LinkToSlide != index+1 {
			t.Fatalf("folder-only PPTX slide %d path = %+v", index, slide.BoardTitle)
		}
	}

	path := filepath.Join(t.TempDir(), "folders.pptx")
	if err := presentation.SaveTo(path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for index := range presentation.Slides {
		rels := readZipMember(t, data, fmt.Sprintf("ppt/slides/_rels/slide%d.xml.rels", index+1))
		if bytes.Contains(rels, []byte("slide0.xml")) {
			t.Fatalf("folder-only PPTX slide %d targets slide0: %s", index+1, rels)
		}
	}
}

func pagedMetadataDiagram() *d2target.Diagram {
	diagram := simpleRasterDiagramWithSize(80, 80)
	diagram.Name = "root"
	base := diagram.Shapes[0]
	shape := func(id string, x int, link, tooltip string) d2target.Shape {
		result := base
		result.ID = id
		result.Pos = d2target.Point{X: x, Y: 5}
		result.Width, result.Height = 10, 10
		result.Link, result.Tooltip = link, tooltip
		return result
	}
	diagram.Shapes = []d2target.Shape{
		shape("external", 0, "https://example.com", "open website"),
		shape("internal", 15, "root.layers.next", "open child"),
		shape("tooltip", 30, "", "hover only"),
	}
	markdown := shape("markdown", 40, "", "")
	markdown.Pos.Y = 25
	markdown.Width, markdown.Height = 35, 20
	markdown.Text = d2target.Text{
		Label: "[docs](root.html \"relative documentation\")", Language: "markdown",
		FontSize: 8, FontFamily: "default", LabelWidth: 30, LabelHeight: 12,
	}
	diagram.Shapes = append(diagram.Shapes, markdown)
	connection := *d2target.BaseConnection()
	connection.ID = "edge"
	connection.Route = []*geo.Point{{X: 5, Y: 60}, {X: 70, Y: 60}}
	connection.Label = "edge"
	connection.LabelPosition = "INSIDE_MIDDLE_CENTER"
	connection.LabelPercentage = .5
	connection.LabelWidth = 24
	connection.LabelHeight = 10
	connection.FontSize = 8
	connection.FontFamily = "default"
	connection.Link = "https://example.com/edge"
	connection.Tooltip = "edge details"
	diagram.Connections = []d2target.Connection{connection}
	return diagram
}

func findPagedNode(root *d2scene.Node, id string) *d2scene.Node {
	stack := []*d2scene.Node{root}
	for len(stack) != 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if node == nil {
			continue
		}
		if node.ID == id {
			return node
		}
		stack = append(stack, node.Children...)
	}
	return nil
}

func pagedNodeText(root *d2scene.Node) string {
	var text strings.Builder
	stack := []*d2scene.Node{root}
	for len(stack) != 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if node == nil {
			continue
		}
		if run, ok := node.Primitive.(d2scene.TextRun); ok {
			if text.Len() != 0 {
				text.WriteByte('\n')
			}
			text.WriteString(run.Text)
		}
		for index := len(node.Children) - 1; index >= 0; index-- {
			stack = append(stack, node.Children[index])
		}
	}
	return text.String()
}

func readZipMember(t *testing.T, data []byte, name string) []byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range reader.File {
		if file.Name != name {
			continue
		}
		stream, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, readErr := io.ReadAll(stream)
		closeErr := stream.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
		return content
	}
	t.Fatalf("PPTX member %q is missing", name)
	return nil
}

func near(left, right float64) bool {
	const epsilon = 1e-9
	return left > right-epsilon && left < right+epsilon
}
