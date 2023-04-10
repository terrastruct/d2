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
	"strings"
	"time"
)

type Presentation struct {
	Title       string
	Description string
	Subject     string
	Creator     string
	// D2Version can't have letters, only numbers (`[0-9]`) and `.`
	// Otherwise, it may fail to open in PowerPoint
	D2Version string

	Slides []*Slide
}

type Slide struct {
	BoardPath   []string
	Image       []byte
	ImageWidth  int
	ImageHeight int
	ImageTop    int
	ImageLeft   int
}

func NewPresentation(title, description, subject, creator, d2Version string) *Presentation {
	return &Presentation{
		Title:       title,
		Description: description,
		Subject:     subject,
		Creator:     creator,
		D2Version:   d2Version,
	}
}

func (p *Presentation) AddSlide(pngContent []byte, boardPath []string) error {
	src, err := png.Decode(bytes.NewReader(pngContent))
	if err != nil {
		return fmt.Errorf("error decoding PNG image: %v", err)
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
	if srcWidth/srcHeight >= IMAGE_ASPECT_RATIO {
		// here, the image aspect ratio is, at least, equal to the slide aspect ratio
		// so, it makes sense to expand the image horizontally to use as much as space as possible
		width = SLIDE_WIDTH
		height = int(float64(width) * (srcHeight / srcWidth))
		// first, try to make the image as wide as the slide
		// but, if this results in a tall image, use only the
		// image adjusted width to avoid overlapping with the header
		if height > IMAGE_HEIGHT {
			width = IMAGE_WIDTH
			height = int(float64(width) * (srcHeight / srcWidth))
		}
	} else {
		// here, the aspect ratio could be 4x3, in which the image is still wider than taller,
		// but expanding horizontally would result in an overflow
		// so, we expand to make it fit the available vertical space
		height = IMAGE_HEIGHT
		width = int(float64(height) * (srcWidth / srcHeight))
	}
	top := (IMAGE_HEIGHT - height) / 2
	left := (SLIDE_WIDTH - width) / 2

	slide := &Slide{
		BoardPath:   make([]string, len(boardPath)),
		Image:       pngContent,
		ImageWidth:  width,
		ImageHeight: height,
		ImageTop:    top,
		ImageLeft:   left,
	}
	// it must copy the board path to avoid slice reference issues
	copy(slide.BoardPath, boardPath)

	p.Slides = append(p.Slides, slide)
	return nil
}

func (p *Presentation) SaveTo(filePath string) error {
	f, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer f.Close()
	zipWriter := zip.NewWriter(f)
	defer zipWriter.Close()

	if err = copyPptxTemplateTo(zipWriter); err != nil {
		return err
	}

	var slideFileNames []string
	for i, slide := range p.Slides {
		imageID := fmt.Sprintf("slide%dImage", i+1)
		slideFileName := fmt.Sprintf("slide%d", i+1)
		slideFileNames = append(slideFileNames, slideFileName)

		imageWriter, err := zipWriter.Create(fmt.Sprintf("ppt/media/%s.png", imageID))
		if err != nil {
			return err
		}
		_, err = imageWriter.Write(slide.Image)
		if err != nil {
			return err
		}

		err = addFileFromTemplate(zipWriter, fmt.Sprintf("ppt/slides/_rels/%s.xml.rels", slideFileName), RELS_SLIDE_XML, RelsSlideXmlContent{
			FileName:       imageID,
			RelationshipID: imageID,
		})
		if err != nil {
			return err
		}

		err = addFileFromTemplate(zipWriter, fmt.Sprintf("ppt/slides/%s.xml", slideFileName), SLIDE_XML, getSlideXmlContent(imageID, slide))
		if err != nil {
			return err
		}
	}

	err = addFileFromTemplate(zipWriter, "[Content_Types].xml", CONTENT_TYPES_XML, ContentTypesXmlContent{
		FileNames: slideFileNames,
	})
	if err != nil {
		return err
	}

	err = addFileFromTemplate(zipWriter, "ppt/_rels/presentation.xml.rels", RELS_PRESENTATION_XML, getRelsPresentationXmlContent(slideFileNames))
	if err != nil {
		return err
	}

	err = addFileFromTemplate(zipWriter, "ppt/presentation.xml", PRESENTATION_XML, getPresentationXmlContent(slideFileNames))
	if err != nil {
		return err
	}

	dateTime := time.Now().Format(time.RFC3339)
	err = addFileFromTemplate(zipWriter, "docProps/core.xml", CORE_XML, CoreXmlContent{
		Creator:        p.Creator,
		Subject:        p.Subject,
		Description:    p.Description,
		LastModifiedBy: p.Creator,
		Title:          p.Title,
		Created:        dateTime,
		Modified:       dateTime,
	})
	if err != nil {
		return err
	}

	titles := make([]string, 0, len(p.Slides))
	for _, slide := range p.Slides {
		titles = append(titles, strings.Join(slide.BoardPath, "/"))
	}
	err = addFileFromTemplate(zipWriter, "docProps/app.xml", APP_XML, AppXmlContent{
		SlideCount:         len(p.Slides),
		TitlesOfPartsCount: len(p.Slides) + 3,
		D2Version:          p.D2Version,
		Titles:             titles,
	})
	if err != nil {
		return err
	}

	return nil
}
