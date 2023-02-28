package pdf

import (
	"bytes"
	"math"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/jung-kurt/gofpdf"

	"oss.terrastruct.com/d2/d2renderers/d2fonts"
	"oss.terrastruct.com/d2/d2renderers/d2svg/appendix"
	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/d2themes"
	"oss.terrastruct.com/d2/d2themes/d2themescatalog"
	"oss.terrastruct.com/d2/lib/color"
)

type GoFPDF struct {
	pdf *gofpdf.Fpdf
}

func Init() *GoFPDF {
	newGofPDF := gofpdf.NewCustom(&gofpdf.InitType{
		UnitStr: "pt",
	})

	newGofPDF.AddUTF8FontFromBytes("source", "", d2fonts.FontFaces[d2fonts.SourceSansPro.Font(0, d2fonts.FONT_STYLE_REGULAR)])
	newGofPDF.AddUTF8FontFromBytes("source", "B", d2fonts.FontFaces[d2fonts.SourceSansPro.Font(0, d2fonts.FONT_STYLE_BOLD)])
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

func (g *GoFPDF) AddPDFPage(png []byte, boardPath []string, themeID int64, fill string, shapes []d2target.Shape, pad int64) error {
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
	headerMargin := 28.0
	headerWidth := g.pdf.GetStringWidth(pathString) + 2*headerMargin

	minPageDimension := 576.0
	pageWidth = math.Max(math.Max(minPageDimension, imageWidth), headerWidth)
	pageHeight = math.Max(minPageDimension, imageHeight)

	fillRGB, err := g.GetFillRGB(themeID, fill)
	if err != nil {
		return err
	}

	// Add page
	headerHeight := 72.0
	g.pdf.AddPageFormat("", gofpdf.SizeType{Wd: pageWidth, Ht: pageHeight + headerHeight})

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

	// Draw external links
	for _, shape := range shapes {
		if shape.Link != "" {
			linkX := (pageWidth-imageWidth)/2 + float64(pad/2) + float64(shape.Pos.X)
			linkY := (pageHeight-imageHeight)/2 + float64(pad/2) + float64(shape.Pos.Y) + appendix.ICON_RADIUS/2 + headerHeight
			linkWidth := float64(shape.Width)
			linkHeight := float64(shape.Height)
			spew.Dump("X", linkX, "Y", linkY)
			spew.Dump("w", linkWidth, "h", linkHeight)
			spew.Dump("pw", pageWidth, "pwh", pageHeight)
			g.pdf.LinkString(linkX, linkY, linkWidth, linkHeight, shape.Link)
		}
	}

	// Draw header/img seperator
	g.pdf.SetXY(headerMargin, headerHeight)
	g.pdf.SetLineWidth(0.01)
	if fillRGB.IsLight() {
		g.pdf.SetDrawColor(10, 15, 37) // steel-900
	} else {
		g.pdf.SetDrawColor(255, 255, 255)
	}
	g.pdf.CellFormat(pageWidth-(headerMargin*2), 0.01, "", "T", 0, "", false, 0, "")

	return nil
}

func (g *GoFPDF) Export(outputPath string) error {
	return g.pdf.OutputFileAndClose(outputPath)
}
