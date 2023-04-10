package pptx

import (
	"archive/zip"
	"bytes"
	_ "embed"
	"fmt"
	"strings"
	"time"
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

func addFile(zipFile *zip.Writer, filePath, content string) error {
	w, err := zipFile.Create(filePath)
	if err != nil {
		return err
	}
	w.Write([]byte(content))
	return nil
}

//go:embed templates/slide.xml.rels
var RELS_SLIDE_XML string

func getRelsSlideXml(imageID string) string {
	return fmt.Sprintf(RELS_SLIDE_XML, imageID, imageID)
}

//go:embed templates/slide.xml
var SLIDE_XML string

func getSlideXml(boardPath []string, imageID string, top, left, width, height int) string {
	var slideTitle string
	boardName := boardPath[len(boardPath)-1]
	prefixPath := boardPath[:len(boardPath)-1]
	if len(prefixPath) > 0 {
		prefix := strings.Join(prefixPath, "  /  ") + "  /  "
		slideTitle = fmt.Sprintf(`<a:r><a:t>%s</a:t></a:r><a:r><a:rPr b="1" /><a:t>%s</a:t></a:r>`, prefix, boardName)
	} else {
		slideTitle = fmt.Sprintf(`<a:r><a:rPr b="1" /><a:t>%s</a:t></a:r>`, boardName)
	}
	slideDescription := strings.Join(boardPath, " / ")
	top += HEADER_HEIGHT
	return fmt.Sprintf(SLIDE_XML, slideDescription, slideDescription, imageID, left, top, width, height, slideDescription, HEADER_HEIGHT, slideTitle)
}

//go:embed templates/rels_presentation.xml
var RELS_PRESENTATION_XML string

func getPresentationXmlRels(slideFileNames []string) string {
	var builder strings.Builder
	for _, name := range slideFileNames {
		builder.WriteString(fmt.Sprintf(
			`<Relationship Id="%s" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide" Target="slides/%s.xml" />`, name, name,
		))
	}

	return fmt.Sprintf(RELS_PRESENTATION_XML, builder.String())
}

//go:embed templates/content_types.xml
var CONTENT_TYPES_XML string

func getContentTypesXml(slideFileNames []string) string {
	var builder strings.Builder
	for _, name := range slideFileNames {
		builder.WriteString(fmt.Sprintf(
			`<Override PartName="/ppt/slides/%s.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml" />`, name,
		))
	}

	return fmt.Sprintf(CONTENT_TYPES_XML, builder.String())
}

//go:embed templates/presentation.xml
var PRESENTATION_XML string

func getPresentationXml(slideFileNames []string) string {
	var builder strings.Builder
	for i, name := range slideFileNames {
		// in the exported presentation, the first slide ID was 256, so keeping it here for compatibility
		builder.WriteString(fmt.Sprintf(`<p:sldId id="%d" r:id="%s" />`, 256+i, name))
	}
	return fmt.Sprintf(PRESENTATION_XML, builder.String(), SLIDE_WIDTH, SLIDE_HEIGHT)
}

//go:embed templates/core.xml
var CORE_XML string

func getCoreXml(title, subject, description, creator string) string {
	dateTime := time.Now().Format(time.RFC3339)
	return fmt.Sprintf(
		CORE_XML,
		title,
		subject,
		creator,
		description,
		creator,
		dateTime,
		dateTime,
	)
}

//go:embed templates/app.xml
var APP_XML string

func getAppXml(slides []*Slide, d2version string) string {
	var builder strings.Builder
	for _, slide := range slides {
		builder.WriteString(fmt.Sprintf(`<vt:lpstr>%s</vt:lpstr>`, strings.Join(slide.BoardPath, "/")))
	}
	return fmt.Sprintf(
		APP_XML,
		len(slides),
		len(slides),
		len(slides)+3, // number of entries, len(slides) + Office Theme + 2 Fonts
		builder.String(),
		d2version,
	)
}
