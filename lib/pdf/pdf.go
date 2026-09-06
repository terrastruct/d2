package pdf

import (
	"bytes"
	"errors"
	"io"
	"math"
	"os"
	"strings"

	"codeberg.org/go-pdf/fpdf"

	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/d2themes"
	"github.com/d2lang/d2/d2themes/d2themescatalog"
	"github.com/d2lang/d2/lib/color"
)

const TITLE_SEP = "  /  "

type GoFPDF struct {
	pdf *fpdf.Fpdf
}

type BoardTitle struct {
	Name    string
	BoardID string
}

func Init() *GoFPDF {
	newGofPDF := fpdf.NewCustom(&fpdf.InitType{
		UnitStr: "pt",
	})

	newGofPDF.AddUTF8FontFromBytes("source", "", d2fonts.FontFaces.Get(d2fonts.SourceSansPro.Font(0, d2fonts.FONT_STYLE_REGULAR)))
	newGofPDF.AddUTF8FontFromBytes("source", "B", d2fonts.FontFaces.Get(d2fonts.SourceSansPro.Font(0, d2fonts.FONT_STYLE_BOLD)))
	newGofPDF.SetAutoPageBreak(false, 0)
	newGofPDF.SetLineWidth(2)
	newGofPDF.SetMargins(0, 0, 0)

	fpdf := GoFPDF{
		pdf: newGofPDF,
	}

	return &fpdf
}

func (g *GoFPDF) GetFillRGB(themeID int64, fill string) (color.RGB, error) {
	if fill == "" || strings.ToLower(fill) == "transparent" {
		return color.RGB{
			Red:   255,
			Green: 255,
			Blue:  255,
		}, nil
	}

	if color.IsThemeColor(fill) {
		theme := d2themescatalog.Find(themeID)
		fill = d2themes.ResolveThemeColor(theme, fill)
	} else {
		rgb := color.Name2RGB(fill)
		if (rgb != color.RGB{}) {
			return rgb, nil
		}
	}

	return color.Hex2RGB(fill)
}

func (g *GoFPDF) AddPDFPage(png []byte, titlePath []BoardTitle, themeID int64, fill string, shapes []d2target.Shape, pad int64, viewboxX, viewboxY float64, pageMap map[string]int, includeNav bool) error {
	if len(titlePath) == 0 {
		return errors.New("PDF page requires a board title path")
	}
	boardID := titlePath[len(titlePath)-1].BoardID
	if boardID == "" {
		return errors.New("PDF page requires a board ID")
	}
	imageKey := "d2-board:" + boardID
	var opt fpdf.ImageOptions
	opt.ImageType = "png"
	boardPath := make([]string, len(titlePath))
	for i, t := range titlePath {
		boardPath[i] = t.Name
	}
	imageInfo := g.pdf.RegisterImageOptionsReader(imageKey, opt, bytes.NewReader(png))
	if g.pdf.Err() {
		return g.pdf.Error()
	}
	imageWidth := imageInfo.Width() / 2
	imageHeight := imageInfo.Height() / 2

	// calculate page dimensions
	var pageWidth float64
	var pageHeight float64

	g.pdf.SetFont("source", "B", 14)
	pathString := strings.Join(boardPath, "  /  ")
	headerMargin := 28.0
	headerWidth := g.pdf.GetStringWidth(pathString) + 2*headerMargin
	headerHeight := 72.0
	if !includeNav {
		headerMargin = 0.
		headerWidth = 0.
		headerHeight = 0.
	}

	minPageDimension := 576.0
	pageWidth = math.Max(math.Max(minPageDimension, imageWidth), headerWidth)
	pageHeight = math.Max(minPageDimension, imageHeight)

	fillRGB, err := g.GetFillRGB(themeID, fill)
	if err != nil {
		return err
	}

	// Add page
	g.pdf.AddPageFormat("", fpdf.SizeType{Wd: pageWidth, Ht: pageHeight + headerHeight})

	if includeNav {
		// Draw header
		g.pdf.SetFillColor(int(fillRGB.Red), int(fillRGB.Green), int(fillRGB.Blue))
		g.pdf.Rect(0, 0, pageWidth, pageHeight+headerHeight, "F")
		if fillRGB.IsLight() {
			g.pdf.SetTextColor(10, 15, 37) // steel-900
		} else {
			g.pdf.SetTextColor(255, 255, 255)
		}
		g.pdf.SetFont("source", "", 14)

		// Draw board path prefix
		prefixWidth := headerMargin
		if len(titlePath) > 1 {
			for _, t := range titlePath[:len(titlePath)-1] {
				g.pdf.SetXY(prefixWidth, 0)
				w := g.pdf.GetStringWidth(t.Name)
				var linkID int
				if pageNum, ok := pageMap[t.BoardID]; ok {
					linkID = g.pdf.AddLink()
					g.pdf.SetLink(linkID, 0, pageNum+1)
				}
				g.pdf.CellFormat(w, headerHeight, t.Name, "", 0, "", false, linkID, "")
				prefixWidth += w

				g.pdf.SetXY(prefixWidth, 0)
				w = g.pdf.GetStringWidth(TITLE_SEP)
				g.pdf.CellFormat(prefixWidth, headerHeight, TITLE_SEP, "", 0, "", false, 0, "")
				prefixWidth += w
			}
		}

		// Draw board name
		boardName := boardPath[len(boardPath)-1]
		g.pdf.SetFont("source", "B", 14)
		g.pdf.SetXY(prefixWidth, 0)
		g.pdf.CellFormat(pageWidth-prefixWidth-headerMargin, headerHeight, boardName, "", 0, "", false, 0, "")
	}

	// Draw image
	imageX := (pageWidth - imageWidth) / 2
	imageY := headerHeight + (pageHeight-imageHeight)/2
	g.pdf.ImageOptions(imageKey, imageX, imageY, imageWidth, imageHeight, false, opt, 0, "")

	// Draw links
	for _, shape := range shapes {
		if shape.Link == "" {
			continue
		}

		linkX := imageX + float64(shape.Pos.X) - viewboxX - float64(shape.StrokeWidth)
		linkY := imageY + float64(shape.Pos.Y) - viewboxY - float64(shape.StrokeWidth)
		linkWidth := float64(shape.Width) + float64(shape.StrokeWidth*2)
		linkHeight := float64(shape.Height) + float64(shape.StrokeWidth*2)

		if pageNum, ok := pageMap[shape.Link]; ok {
			linkID := g.pdf.AddLink()
			g.pdf.SetLink(linkID, 0, pageNum+1)
			g.pdf.Link(linkX, linkY, linkWidth, linkHeight, linkID)
		} else {
			g.pdf.LinkString(linkX, linkY, linkWidth, linkHeight, shape.Link)
		}
	}

	// Draw header/img separator
	g.pdf.SetXY(headerMargin, headerHeight)
	g.pdf.SetLineWidth(1)
	if fillRGB.IsLight() {
		g.pdf.SetDrawColor(10, 15, 37) // steel-900
	} else {
		g.pdf.SetDrawColor(255, 255, 255)
	}
	g.pdf.CellFormat(pageWidth-(headerMargin*2), 1, "", "T", 0, "", false, 0, "")

	return nil
}

func (g *GoFPDF) Export(outputPath string) error {
	_, err := g.ExportWithStatus(outputPath)
	return err
}

// ExportWithStatus writes the PDF document to outputPath and reports whether
// the target was created or truncated. A true result therefore means callers
// must treat an accompanying error as a partial output failure, even when the
// failure happened before the first byte was written.
func (g *GoFPDF) ExportWithStatus(outputPath string) (touched bool, err error) {
	// Match fpdf.OutputFileAndClose: a document-generation error must not touch
	// an existing target.
	if err := g.pdf.Error(); err != nil {
		return false, err
	}

	output, err := os.Create(outputPath)
	if err != nil {
		g.pdf.SetError(err)
		return false, err
	}
	err = g.exportToAndClose(output)
	return true, err
}

// ExportTo writes the complete PDF document to output. The caller retains
// ownership of output.
func (g *GoFPDF) ExportTo(output io.Writer) error {
	return g.pdf.Output(output)
}

func (g *GoFPDF) exportToAndClose(output io.WriteCloser) error {
	return errors.Join(g.ExportTo(output), output.Close())
}
