// pptx is a package to create slide presentations in pptx (Microsoft Power Point) format.
// A `.pptx` file is just a bunch of zip compressed `.xml` files following the Office Open XML (OOXML) format.
// To see its content, you can just `unzip <path/to/file>.pptx -d <folder>`.
// With this package, it is possible to create a `Presentation` and add `Slide`s to it.
// Then, when saving the presentation, it will generate the required `.xml` files, compress them and write to the disk.
// Note that this isn't a full implementation of the OOXML format, but a wrapper around it.
// There's a base template with common files to the presentation and then when saving, the package generate only the slides and relationships.
// The base template and slide templates were generated using https://python-pptx.readthedocs.io/en/latest/
// For more information about OOXML, check http://officeopenxml.com/index.php
package pptx

import (
	"archive/zip"
	"bytes"
	_ "embed"
	"fmt"
	"image/png"
	"os"
	"text/template"
)

type BoardTitle struct {
	LinkID      string
	Name        string
	BoardID     string
	LinkToSlide int
}

type Presentation struct {
	Title       string
	Description string
	Subject     string
	Creator     string
	// D2Version can't have letters, only numbers (`[0-9]`) and `.`
	// Otherwise, it may fail to open in PowerPoint
	D2Version  string
	includeNav bool

	Slides []*Slide
}

type Slide struct {
	BoardTitle       []BoardTitle
	Links            []*Link
	Image            []byte
	ImageId          string
	ImageWidth       int
	ImageHeight      int
	ImageTop         int
	ImageLeft        int
	ImageScaleFactor float64
}

func (s *Slide) AddLink(link *Link) {
	link.Index = len(s.Links)
	s.Links = append(s.Links, link)
	link.ID = fmt.Sprintf("link%d", len(s.Links))
	link.Height *= int(s.ImageScaleFactor)
	link.Width *= int(s.ImageScaleFactor)
	link.Top = s.ImageTop + int(float64(link.Top)*s.ImageScaleFactor)
	link.Left = s.ImageLeft + int(float64(link.Left)*s.ImageScaleFactor)
}

type Link struct {
	ID          string
	Index       int
	Top         int
	Left        int
	Width       int
	Height      int
	SlideIndex  int
	ExternalUrl string
	Tooltip     string
}

func NewPresentation(title, description, subject, creator, d2Version string, includeNav bool) *Presentation {
	return &Presentation{
		Title:       title,
		Description: description,
		Subject:     subject,
		Creator:     creator,
		D2Version:   d2Version,
		includeNav:  includeNav,
	}
}

func (p *Presentation) headerHeight() int {
	if p.includeNav {
		return HEADER_HEIGHT
	}
	return 0
}

func (p *Presentation) height() int {
	return SLIDE_HEIGHT - p.headerHeight()
}

func (p *Presentation) aspectRatio() float64 {
	return float64(IMAGE_WIDTH) / float64(p.height())
}

func (p *Presentation) AddSlide(pngContent []byte, titlePath []BoardTitle) (*Slide, error) {
	src, err := png.Decode(bytes.NewReader(pngContent))
	if err != nil {
		return nil, fmt.Errorf("error decoding PNG image: %v", err)
	}

	var width, height int
	srcSize := src.Bounds().Size()
	srcWidth, srcHeight := float64(srcSize.X), float64(srcSize.Y)

	// compute the size and position to fit the slide
	// if the image is wider than taller and its aspect ratio is, at least, the same as the available image space aspect ratio
	// then, set the image width to the available space and compute the height
	// ┌──────────────────────────────────────────────────┐   ─┬─
	// │  HEADER                                          │    │
	// ├──┬────────────────────────────────────────────┬──┤    │         ─┬─
	// │  │                                            │  │    │          │
	// │  │                                            │  │  SLIDE        │
	// │  │                                            │  │  HEIGHT       │
	// │  │                                            │  │    │        IMAGE
	// │  │                                            │  │    │        HEIGHT
	// │  │                                            │  │    │          │
	// │  │                                            │  │    │          │
	// │  │                                            │  │    │          │
	// │  │                                            │  │    │          │
	// └──┴────────────────────────────────────────────┴──┘   ─┴─        ─┴─
	// ├────────────────────SLIDE WIDTH───────────────────┤
	//    ├─────────────────IMAGE WIDTH────────────────┤
	if srcWidth/srcHeight >= p.aspectRatio() {
		// here, the image aspect ratio is, at least, equal to the slide aspect ratio
		// so, it makes sense to expand the image horizontally to use as much as space as possible
		width = SLIDE_WIDTH
		height = int(float64(width) * (srcHeight / srcWidth))
		// first, try to make the image as wide as the slide
		// but, if this results in a tall image, use only the
		// image adjusted width to avoid overlapping with the header
		if height > p.height() {
			width = IMAGE_WIDTH
			height = int(float64(width) * (srcHeight / srcWidth))
		}
	} else {
		// here, the aspect ratio could be 4x3, in which the image is still wider than taller,
		// but expanding horizontally would result in an overflow
		// so, we expand to make it fit the available vertical space
		height = p.height()
		width = int(float64(height) * (srcWidth / srcHeight))
	}
	top := p.headerHeight() + ((p.height() - height) / 2)
	left := (SLIDE_WIDTH - width) / 2

	slide := &Slide{
		BoardTitle:       make([]BoardTitle, len(titlePath)),
		ImageId:          fmt.Sprintf("slide%dImage", len(p.Slides)+1),
		Image:            pngContent,
		ImageWidth:       width,
		ImageHeight:      height,
		ImageTop:         top,
		ImageLeft:        left,
		ImageScaleFactor: float64(width) / srcWidth,
	}
	// it must copy the board path to avoid slice reference issues
	for i := 0; i < len(titlePath); i++ {
		titlePath[i].LinkID = fmt.Sprintf("navLink%d", i)
		slide.BoardTitle[i] = titlePath[i]
	}

	p.Slides = append(p.Slides, slide)
	return slide, nil
}

func (p *Presentation) SaveTo(filePath string) error {
	f, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer f.Close()
	zipWriter := zip.NewWriter(f)
	defer zipWriter.Close()

	var slideFileNames []string
	for i := range p.Slides {
		slideFileName := fmt.Sprintf("slide%d", i+1)
		slideFileNames = append(slideFileNames, slideFileName)
	}

	err = addFileFromTemplate(zipWriter, "[Content_Types].xml", CONTENT_TYPES_XML, ContentTypesXmlContent{
		FileNames: slideFileNames,
	})
	if err != nil {
		return err
	}

	if err = copyPptxTemplateTo(zipWriter); err != nil {
		return err
	}

	err = addFileFromTemplate(zipWriter, "_rels/.rels", ROOT_RELS_XML, nil)
	if err != nil {
		return err
	}

	err = addFileFromTemplate(zipWriter, "ppt/slideMasters/_rels/slideMaster1.xml.rels", SLIDEMASTER_RELS_XML, nil)
	if err != nil {
		return err
	}

	for i, slide := range p.Slides {
		imageID := fmt.Sprintf("slide%dImage", i+1)
		slideFileName := fmt.Sprintf("slide%d", i+1)

		imageWriter, err := zipWriter.Create(fmt.Sprintf("ppt/media/%s.png", imageID))
		if err != nil {
			return err
		}
		_, err = imageWriter.Write(slide.Image)
		if err != nil {
			return err
		}

		err = addFileFromTemplate(zipWriter, fmt.Sprintf("ppt/slides/_rels/%s.xml.rels", slideFileName), RELS_SLIDE_XML, getSlideXmlRelsContent(imageID, slide, i+1))
		if err != nil {
			return err
		}

		err = addFileFromTemplate(zipWriter, fmt.Sprintf("ppt/slides/%s.xml", slideFileName), SLIDE_XML, p.getSlideXmlContent(imageID, slide))
		if err != nil {
			return err
		}
	}

	err = addFileFromTemplate(zipWriter, "ppt/_rels/presentation.xml.rels", RELS_PRESENTATION_XML, getRelsPresentationXmlContent(slideFileNames))
	if err != nil {
		return err
	}

	err = addFileFromTemplate(zipWriter, "ppt/presentation.xml", PRESENTATION_XML, getPresentationXmlContent(slideFileNames))
	if err != nil {
		return err
	}

	err = addFileFromTemplate(zipWriter, "docProps/core.xml", CORE_XML, CoreXmlContent{
		Creator:        p.Creator,
		Subject:        p.Subject,
		Description:    p.Description,
		LastModifiedBy: p.Creator,
		Title:          p.Title,
	})
	if err != nil {
		return err
	}

	titles := make([]string, 0, len(p.Slides))
	for _, slide := range p.Slides {
		titles = append(titles, slide.BoardTitle[len(slide.BoardTitle)-1].BoardID)
	}
	err = addFileFromTemplate(zipWriter, "docProps/app.xml", APP_XML, AppXmlContent{
		SlideCount:         len(p.Slides),
		TitlesOfPartsCount: len(p.Slides) + 3, // + 3 for fonts and theme
		D2Version:          p.D2Version,
		Titles:             titles,
	})
	if err != nil {
		return err
	}

	return nil
}

// Measurements in OOXML are made in English Metric Units (EMUs) where 1 inch = 914,400 EMUs
// The intent is to have a measurement unit that doesn't require floating points when dealing with centimeters, inches, points (DPI).
// Office Open XML (OOXML) http://officeopenxml.com/prPresentation.php
// https://startbigthinksmall.wordpress.com/2010/01/04/points-inches-and-emus-measuring-units-in-office-open-xml/
const SLIDE_WIDTH = 9_144_000
const SLIDE_HEIGHT = 5_143_500
const HEADER_HEIGHT = 392_471

// keep the right aspect ratio: SLIDE_WIDTH / SLIDE_HEIGHT = IMAGE_WIDTH / IMAGE_HEIGHT
const IMAGE_WIDTH = 8_446_273

//go:embed template.pptx
var PPTX_TEMPLATE []byte

func copyPptxTemplateTo(w *zip.Writer) error {
	reader := bytes.NewReader(PPTX_TEMPLATE)
	zipReader, err := zip.NewReader(reader, reader.Size())
	if err != nil {
		fmt.Printf("error creating zip reader: %v", err)
	}

	skipFiles := map[string]bool{
		"_rels/.rels": true,
		"ppt/slideMasters/_rels/slideMaster1.xml.rels": true,
	}

	for _, f := range zipReader.File {
		if skipFiles[f.Name] {
			continue
		}
		if err := w.Copy(f); err != nil {
			return fmt.Errorf("error copying %s: %v", f.Name, err)
		}
	}
	return nil
}

//go:embed templates/slide.xml.rels
var RELS_SLIDE_XML string

type RelsSlideXmlLinkContent struct {
	RelationshipID string
	ExternalUrl    string
	SlideIndex     int
}

type RelsSlideXmlContent struct {
	FileName       string
	RelationshipID string
	Links          []RelsSlideXmlLinkContent
}

func getSlideXmlRelsContent(imageID string, slide *Slide, currentSlideNum int) RelsSlideXmlContent {
	content := RelsSlideXmlContent{
		FileName:       imageID,
		RelationshipID: imageID,
	}

	for _, link := range slide.Links {
		content.Links = append(content.Links, RelsSlideXmlLinkContent{
			RelationshipID: link.ID,
			ExternalUrl:    link.ExternalUrl,
			SlideIndex:     link.SlideIndex,
		})
	}

	for _, t := range slide.BoardTitle {
		content.Links = append(content.Links, RelsSlideXmlLinkContent{
			RelationshipID: t.LinkID,
			SlideIndex:     t.LinkToSlide,
		})
	}

	return content
}

//go:embed templates/slide.xml
var SLIDE_XML string

type SlideLinkXmlContent struct {
	ID             int
	RelationshipID string
	Name           string
	Action         string
	Left           int
	Top            int
	Width          int
	Height         int
}

type SlideXmlTitlePathContent struct {
	Name           string
	RelationshipID string
}

type SlideXmlContent struct {
	Title        string
	TitlePrefix  []SlideXmlTitlePathContent
	Description  string
	HeaderHeight int
	ImageID      string
	ImageLeft    int
	ImageTop     int
	ImageWidth   int
	ImageHeight  int

	Links []SlideLinkXmlContent
}

func (p *Presentation) getSlideXmlContent(imageID string, slide *Slide) SlideXmlContent {
	title := make([]SlideXmlTitlePathContent, len(slide.BoardTitle)-1)
	for i := 0; i < len(slide.BoardTitle)-1; i++ {
		t := slide.BoardTitle[i]
		title[i] = SlideXmlTitlePathContent{
			Name:           t.Name,
			RelationshipID: t.LinkID,
		}
	}
	content := SlideXmlContent{
		Description:  slide.BoardTitle[len(slide.BoardTitle)-1].BoardID,
		HeaderHeight: p.headerHeight(),
		ImageID:      imageID,
		ImageLeft:    slide.ImageLeft,
		ImageTop:     slide.ImageTop,
		ImageWidth:   slide.ImageWidth,
		ImageHeight:  slide.ImageHeight,
	}
	if p.includeNav {
		content.Title = slide.BoardTitle[len(slide.BoardTitle)-1].Name
		content.TitlePrefix = title
	}

	for _, link := range slide.Links {
		var action string
		if link.ExternalUrl == "" {
			action = "ppaction://hlinksldjump"
		}
		content.Links = append(content.Links, SlideLinkXmlContent{
			ID:             link.Index,
			RelationshipID: link.ID,
			Name:           link.Tooltip,
			Action:         action,
			Left:           link.Left,
			Top:            link.Top,
			Width:          link.Width,
			Height:         link.Height,
		})
	}

	return content
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
	for i, name := range slideFileNames {
		content.Slides = append(content.Slides, RelsPresentationSlideXmlContent{
			RelationshipID: fmt.Sprintf("rId%d", i+2),
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
	for i := range slideFileNames {
		content.Slides = append(content.Slides, PresentationSlideXmlContent{
			ID:             256 + i,
			RelationshipID: fmt.Sprintf("rId%d", i+2),
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
}

//go:embed templates/app.xml
var APP_XML string

//go:embed templates/root_rels.xml
var ROOT_RELS_XML string

//go:embed templates/slidemaster_rels.xml
var SLIDEMASTER_RELS_XML string

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
