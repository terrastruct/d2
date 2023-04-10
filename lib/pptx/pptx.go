package pptx

import (
	"archive/zip"
	"bytes"
	_ "embed"
	"fmt"
	"strings"
	"text/template"
)

// Measurements in OOXML are made in English Metric Units (EMUs) where 1 inch = 914,400 EMUs
// The intent is to have a measurement unit that doesn't require floating points when dealing with centimeters, inches, points (DPI).
// Office Open XML (OOXML) http://officeopenxml.com/prPresentation.php
// https://startbigthinksmall.wordpress.com/2010/01/04/points-inches-and-emus-measuring-units-in-office-open-xml/
const SLIDE_WIDTH = 9_144_000
const SLIDE_HEIGHT = 5_143_500
const HEADER_HEIGHT = 392_471

const IMAGE_HEIGHT = SLIDE_HEIGHT - HEADER_HEIGHT

// keep the right aspect ratio: SLIDE_WIDTH / SLIDE_HEIGHT = IMAGE_WIDTH / IMAGE_HEIGHT
const IMAGE_WIDTH = 8_446_273
const IMAGE_ASPECT_RATIO = float64(IMAGE_WIDTH) / float64(IMAGE_HEIGHT)

//go:embed template.pptx
var PPTX_TEMPLATE []byte

func copyPptxTemplateTo(w *zip.Writer) error {
	reader := bytes.NewReader(PPTX_TEMPLATE)
	zipReader, err := zip.NewReader(reader, reader.Size())
	if err != nil {
		fmt.Printf("error creating zip reader: %v", err)
	}

	for _, f := range zipReader.File {
		if err := w.Copy(f); err != nil {
			return fmt.Errorf("error copying %s: %v", f.Name, err)
		}
	}
	return nil
}

//go:embed templates/slide.xml.rels
var RELS_SLIDE_XML string

type RelsSlideXmlContent struct {
	FileName       string
	RelationshipID string
}

//go:embed templates/slide.xml
var SLIDE_XML string

type SlideXmlContent struct {
	Title        string
	TitlePrefix  string
	Description  string
	HeaderHeight int
	ImageID      string
	ImageLeft    int
	ImageTop     int
	ImageWidth   int
	ImageHeight  int
}

func getSlideXmlContent(imageID string, slide *Slide) SlideXmlContent {
	boardPath := slide.BoardPath
	boardName := boardPath[len(boardPath)-1]
	prefixPath := boardPath[:len(boardPath)-1]
	var prefix string
	if len(prefixPath) > 0 {
		prefix = strings.Join(prefixPath, "  /  ") + "  /  "
	}
	return SlideXmlContent{
		Title:        boardName,
		TitlePrefix:  prefix,
		Description:  strings.Join(boardPath, " / "),
		HeaderHeight: HEADER_HEIGHT,
		ImageID:      imageID,
		ImageLeft:    slide.ImageLeft,
		ImageTop:     slide.ImageTop + HEADER_HEIGHT,
		ImageWidth:   slide.ImageWidth,
		ImageHeight:  slide.ImageHeight,
	}
}

//go:embed templates/rels_presentation.xml
var RELS_PRESENTATION_XML string

type RelsPresentationSlideXmlContent struct {
	RelationshipID string
	FileName       string
}

type RelsPresentationXmlContent struct {
	Slides []RelsPresentationSlideXmlContent
}

func getRelsPresentationXmlContent(slideFileNames []string) RelsPresentationXmlContent {
	var content RelsPresentationXmlContent
	for _, name := range slideFileNames {
		content.Slides = append(content.Slides, RelsPresentationSlideXmlContent{
			RelationshipID: name,
			FileName:       name,
		})
	}

	return content
}

//go:embed templates/content_types.xml
var CONTENT_TYPES_XML string

type ContentTypesXmlContent struct {
	FileNames []string
}

//go:embed templates/presentation.xml
var PRESENTATION_XML string

type PresentationSlideXmlContent struct {
	ID             int
	RelationshipID string
}

type PresentationXmlContent struct {
	SlideWidth  int
	SlideHeight int
	Slides      []PresentationSlideXmlContent
}

func getPresentationXmlContent(slideFileNames []string) PresentationXmlContent {
	content := PresentationXmlContent{
		SlideWidth:  SLIDE_WIDTH,
		SlideHeight: SLIDE_HEIGHT,
	}
	for i, name := range slideFileNames {
		content.Slides = append(content.Slides, PresentationSlideXmlContent{
			// in the exported presentation, the first slide ID was 256, so keeping it here for compatibility
			ID:             256 + i,
			RelationshipID: name,
		})
	}
	return content
}

//go:embed templates/core.xml
var CORE_XML string

type CoreXmlContent struct {
	Title          string
	Subject        string
	Creator        string
	Description    string
	LastModifiedBy string
	Created        string
	Modified       string
}

//go:embed templates/app.xml
var APP_XML string

type AppXmlContent struct {
	SlideCount         int
	TitlesOfPartsCount int
	Titles             []string
	D2Version          string
}

func addFileFromTemplate(zipFile *zip.Writer, filePath, templateContent string, templateData interface{}) error {
	w, err := zipFile.Create(filePath)
	if err != nil {
		return err
	}

	tmpl, err := template.New(filePath).Parse(templateContent)
	if err != nil {
		return err
	}
	return tmpl.Execute(w, templateData)
}
