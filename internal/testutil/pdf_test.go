package testutil

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/internal/testutil/imagediff"
	"github.com/d2lang/d2/lib/pdf"
)

func TestInspectD2PDFDecodesRasterPagesAndLinks(t *testing.T) {
	t.Parallel()

	expectedPNG := pdfTestPNG(t)
	document := pdf.Init()
	links := []d2target.Shape{
		{ID: "external", Pos: d2target.Point{X: 2, Y: 3}, Width: 5, Height: 4, Link: "https://example.com/a_(b)"},
		{ID: "internal", Pos: d2target.Point{X: 10, Y: 12}, Width: 5, Height: 4, Link: "root.layers.next"},
	}
	pageMap := map[string]int{"root": 0, "root.layers.next": 1}
	if err := document.AddPDFPage(expectedPNG, []pdf.BoardTitle{{Name: "root", BoardID: "root"}}, 0, "#ffffff", links, 0, 0, 0, pageMap, true); err != nil {
		t.Fatal(err)
	}
	if err := document.AddPDFPage(expectedPNG, []pdf.BoardTitle{{Name: "next", BoardID: "root.layers.next"}}, 0, "#ffffff", nil, 0, 0, 0, pageMap, true); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "test.pdf")
	if err := document.Export(path); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	inspection, err := InspectD2PDF(content)
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Pages) != 2 || inspection.Pages[0].Width != 576 || inspection.Pages[0].Height != 648 {
		t.Fatalf("PDF pages = %+v", inspection.Pages)
	}
	if len(inspection.Images) != 1 || inspection.Images[0].Width != 3 || inspection.Images[0].Height != 2 {
		t.Fatalf("PDF images = %+v", inspection.Images)
	}
	if len(inspection.ExternalLinks) != 1 || inspection.ExternalLinks[0] != "https://example.com/a_(b)" || inspection.InternalLinks == 0 {
		t.Fatalf("PDF links = %q/%d", inspection.ExternalLinks, inspection.InternalLinks)
	}
	if _, err := ComparePDFImages(content, [][]byte{expectedPNG}, imagediff.Options{}); err != nil {
		t.Fatal(err)
	}

	wrong := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	for pixel := range wrong.Pix {
		wrong.Pix[pixel] = 255
	}
	var encodedWrong bytes.Buffer
	if err := png.Encode(&encodedWrong, wrong); err != nil {
		t.Fatal(err)
	}
	comparison, err := ComparePDFImages(content, [][]byte{encodedWrong.Bytes()}, imagediff.Options{})
	if err == nil || comparison == nil || comparison.Page != 1 || comparison.Result == nil {
		t.Fatalf("PDF pixel mismatch = %#v/%v", comparison, err)
	}
	var mismatch *imagediff.MismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("PDF pixel error = %v", err)
	}
}

func TestInspectD2PDFKeepsSameTitleBoardsDistinct(t *testing.T) {
	t.Parallel()

	red := solidPDFTestPNG(t, color.NRGBA{R: 255, A: 255})
	blue := solidPDFTestPNG(t, color.NRGBA{B: 255, A: 255})
	document := pdf.Init()
	pageMap := map[string]int{"root.layers.same": 0, "root.scenarios.same": 1}
	for _, page := range []struct {
		id  string
		png []byte
	}{
		{id: "root.layers.same", png: red},
		{id: "root.scenarios.same", png: blue},
	} {
		if err := document.AddPDFPage(
			page.png,
			[]pdf.BoardTitle{{Name: "same", BoardID: page.id}},
			0,
			"#ffffff",
			nil,
			0,
			0,
			0,
			pageMap,
			false,
		); err != nil {
			t.Fatal(err)
		}
	}
	var output bytes.Buffer
	if err := document.ExportTo(&output); err != nil {
		t.Fatal(err)
	}
	if _, err := ComparePDFImages(output.Bytes(), [][]byte{red, blue}, imagediff.Options{}); err != nil {
		t.Fatal(err)
	}
}

func TestPDFPageFillDoesNotDependOnNavigation(t *testing.T) {
	for _, fill := range []struct {
		name, value, operator, textOperator string
	}{
		{"dark", "#174ea6", "0.090 0.306 0.651 rg", "1.000 g"},
		{"light", "#f2cf9b", "0.949 0.812 0.608 rg", "0.039 0.059 0.145 rg"},
		{"empty", "", "1.000 g", "0.039 0.059 0.145 rg"},
		{"transparent", "TrAnSpArEnT", "1.000 g", "0.039 0.059 0.145 rg"},
	} {
		for _, includeNav := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/nav=%t", fill.name, includeNav), func(t *testing.T) {
				t.Parallel()
				// The tiny, partly transparent image leaves most of the page
				// uncovered. Its own pixels cannot supply the page background.
				pngBytes := pdfTestPNG(t)
				document := pdf.Init()
				parent := pdf.BoardTitle{Name: "Parent", BoardID: "root"}
				child := pdf.BoardTitle{Name: "Child", BoardID: "root.layers.child"}
				pageMap := map[string]int{parent.BoardID: 0, child.BoardID: 1}
				for _, path := range [][]pdf.BoardTitle{{parent}, {parent, child}} {
					if err := document.AddPDFPage(pngBytes, path, 0, fill.value, nil, 0, 0, 0, pageMap, includeNav); err != nil {
						t.Fatal(err)
					}
				}
				var output bytes.Buffer
				if err := document.ExportTo(&output); err != nil {
					t.Fatal(err)
				}
				inspection, err := InspectD2PDF(output.Bytes())
				if err != nil {
					t.Fatal(err)
				}
				if len(inspection.Pages) != 2 {
					t.Fatalf("page count = %d, want 2", len(inspection.Pages))
				}
				if _, err := ComparePDFImages(output.Bytes(), [][]byte{pngBytes}, imagediff.Options{}); err != nil {
					t.Fatal(err)
				}
				objects, err := splitPDFObjects(output.Bytes())
				if err != nil {
					t.Fatal(err)
				}
				byNumber := make(map[int]pdfObject, len(objects))
				for _, object := range objects {
					byNumber[object.number] = object
				}
				height := 576.0
				if includeNav {
					height += 72
				}
				for index, page := range inspection.Pages {
					if page.Width != 576 || page.Height != height {
						t.Fatalf("page %d dimensions = %.2fx%.2f, want 576x%.2f", index+1, page.Width, page.Height, height)
					}
					pageObject := byNumber[page.ObjectNumber]
					contentNumber, err := pdfDictionaryInteger(pageObject.body, "Contents")
					if err != nil {
						t.Fatal(err)
					}
					content, err := decodePDFContentStream(byNumber[contentNumber])
					if err != nil {
						t.Fatal(err)
					}
					// Check the actual exported page stream, not the writer's
					// remembered fill color. Paint the whole page once, below
					// the image so transparent pixels reveal the selected fill.
					rectangle := []byte(fmt.Sprintf("0.00 %.2f 576.00 -%.2f re f", height, height))
					background := append([]byte(fill.operator+"\n"), rectangle...)
					fillIndex := bytes.Index(content, background)
					imageUse := pdfImageUseRE.FindIndex(content)
					if fillIndex < 0 || bytes.Count(content, rectangle) != 1 || imageUse == nil || fillIndex >= imageUse[0] {
						t.Fatalf("page %d must paint %q once before its image; stream:\n%s", index+1, background, content)
					}
					if includeNav {
						if !bytes.Contains(content, []byte("\x00P\x00a\x00r\x00e\x00n\x00t")) ||
							(index == 1 && !bytes.Contains(content, []byte("\x00C\x00h\x00i\x00l\x00d"))) {
							t.Fatalf("page %d lost its navigation text", index+1)
						}
						if !bytes.Contains(content, []byte("q "+fill.textOperator+" BT")) {
							t.Fatalf("page %d lost contrasting header text color %q", index+1, fill.textOperator)
						}
						if page.InternalLinks != index {
							t.Fatalf("page %d internal links = %d, want %d", index+1, page.InternalLinks, index)
						}
						if index == 1 && !bytes.Contains(pageObject.body, []byte(fmt.Sprintf("/Dest [%d 0 R", inspection.Pages[0].ObjectNumber))) {
							t.Fatal("child breadcrumb no longer targets the physical parent page")
						}
					} else if page.InternalLinks != 0 || bytes.Contains(content, []byte("Tj")) || bytes.Contains(content, []byte("TJ")) {
						t.Fatalf("page %d gained a navigation header or link", index+1)
					}
				}
			})
		}
	}
}

func pdfTestPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	values := []color.NRGBA{
		{R: 255, A: 255}, {G: 255, A: 128}, {B: 255, A: 64},
		{R: 10, G: 20, B: 30, A: 40}, {R: 50, G: 60, B: 70, A: 80}, {R: 90, G: 100, B: 110, A: 120},
	}
	for index, value := range values {
		img.SetNRGBA(index%3, index/3, value)
	}
	var output bytes.Buffer
	if err := png.Encode(&output, img); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func solidPDFTestPNG(t *testing.T, fill color.NRGBA) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			img.SetNRGBA(x, y, fill)
		}
	}
	var output bytes.Buffer
	if err := png.Encode(&output, img); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}
