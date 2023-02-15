package pdf

import (
	"bytes"
	"math"
	"strings"

	"github.com/jung-kurt/gofpdf"
)

type GoFPDF struct {
	pdf *gofpdf.Fpdf
}

func Init() *GoFPDF {
	newGofPDF := gofpdf.NewCustom(&gofpdf.InitType{
		UnitStr:    "in",
		FontDirStr: "./d2renderers/d2fonts/ttf",
	})

	newGofPDF.AddUTF8Font("source", "", "SourceSansPro-Regular.ttf")
	newGofPDF.AddUTF8Font("source", "B", "SourceSansPro-Bold.ttf")
	newGofPDF.SetAutoPageBreak(false, 0)
	newGofPDF.SetLineWidth(0.05)
	newGofPDF.SetMargins(0, 0, 0)

	fpdf := GoFPDF{
		pdf: newGofPDF,
	}

	return &fpdf
}

func (g *GoFPDF) AddPDFPage(png []byte, boardPath []string) error {
	var opt gofpdf.ImageOptions
	opt.ImageType = "png"
	imageInfo := g.pdf.RegisterImageOptionsReader(strings.Join(boardPath, "/"), opt, bytes.NewReader(png))
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
	headerMargin := 0.3
	headerWidth := g.pdf.GetStringWidth(pathString) + 2*headerMargin

	minPageDimension := 6.0
	pageWidth = math.Max(math.Max(minPageDimension, imageWidth), headerWidth)
	pageHeight = math.Max(minPageDimension, imageHeight)

	// Add page
	headerHeight := 0.75
	g.pdf.AddPageFormat("", gofpdf.SizeType{Wd: pageWidth, Ht: pageHeight + headerHeight})

	// Draw header
	g.pdf.SetFillColor(255, 255, 255)
	g.pdf.Rect(0, 0, pageWidth, pageHeight, "F")
	g.pdf.SetTextColor(10, 15, 37) // steel-900
	g.pdf.SetFont("source", "", 14)

	// Draw board path prefix
	var prefixWidth float64
	prefixPath := boardPath[:len(boardPath)-1]
	if len(prefixPath) > 0 {
		prefix := strings.Join(boardPath[:len(boardPath)-1], "  /  ") + "  /  "
		prefixWidth = g.pdf.GetStringWidth(prefix)

		g.pdf.SetXY(headerMargin, 0)
		g.pdf.CellFormat(prefixWidth, headerHeight, prefix, "", 0, "", false, 0, "")
	}

	// Draw board name
	boardName := boardPath[len(boardPath)-1]
	g.pdf.SetFont("source", "B", 14)
	g.pdf.SetXY(prefixWidth+headerMargin, 0)
	g.pdf.CellFormat(pageWidth-prefixWidth-headerMargin, headerHeight, boardName, "", 0, "", false, 0, "")

	// Draw image
	g.pdf.ImageOptions(strings.Join(boardPath, "/"), (pageWidth-imageWidth)/2, headerHeight+(pageHeight-imageHeight)/2, imageWidth, imageHeight, false, opt, 0, "")

	// Draw header/img seperator
	g.pdf.SetXY(headerMargin, headerHeight)
	g.pdf.SetLineWidth(0.01)
	g.pdf.SetDrawColor(10, 15, 37) // steel-900
	g.pdf.CellFormat(pageWidth-(headerMargin*2), 0.01, "", "T", 0, "", false, 0, "")

	return nil
}

func (g *GoFPDF) Export(outputPath string) error {
	return g.pdf.OutputFileAndClose(outputPath)
}
