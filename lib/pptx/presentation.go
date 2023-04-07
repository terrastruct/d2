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

	"oss.terrastruct.com/d2/d2target"
)

type Presentation struct {
	Title       string
	Description string
	Subject     string
	Creator     string
	D2Version   string

	Slides []*Slide
}

type Slide struct {
	BoardPath        []string
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
	link.Id = fmt.Sprintf("link%d", len(s.Links))
	link.Height *= int(s.ImageScaleFactor)
	link.Width *= int(s.ImageScaleFactor)
	link.Top = s.ImageTop + int(float64(link.Top)*s.ImageScaleFactor)
	link.Left = s.ImageLeft + int(float64(link.Left)*s.ImageScaleFactor)
}

type Link struct {
	Id          string
	Index       int
	Top         int
	Left        int
	Width       int
	Height      int
	SlideIndex  int
	ExternalUrl string
	Tooltip     string
}

func (l *Link) isExternal() bool {
	return l.ExternalUrl != ""
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

func (p *Presentation) AddSlide(pngContent []byte, diagram *d2target.Diagram, boardPath []string) (*Slide, error) {
	src, err := png.Decode(bytes.NewReader(pngContent))
	if err != nil {
		return nil, fmt.Errorf("error decoding PNG image: %v", err)
	}
	srcSize := src.Bounds().Size()
	srcWidth, srcHeight := float64(srcSize.X), float64(srcSize.Y)

	var width, height int

	// compute the size and position to fit the slide
	// if the image is wider than taller and its aspect ratio is, at least, the same as the available image space aspect ratio
	// then, set the image width to the available space and compute the height
	if srcWidth/srcHeight >= IMAGE_ASPECT_RATIO {
		width = SLIDE_WIDTH
		height = int(float64(width) * (srcHeight / srcWidth))
		if height > IMAGE_HEIGHT {
			// this would overflow with the title, so we need to adjust to use only IMAGE_WIDTH
			width = IMAGE_WIDTH
			height = int(float64(width) * (srcHeight / srcWidth))
		}
	} else {
		// otherwise, this image could overflow the slide height/header
		height = IMAGE_HEIGHT
		width = int(float64(height) * (srcWidth / srcHeight))
	}
	top := HEADER_HEIGHT + ((IMAGE_HEIGHT - height) / 2)
	left := (SLIDE_WIDTH - width) / 2

	slide := &Slide{
		BoardPath:        make([]string, len(boardPath)),
		ImageId:          fmt.Sprintf("slide%dImage", len(p.Slides)+1),
		Image:            pngContent,
		ImageWidth:       width,
		ImageHeight:      height,
		ImageTop:         top,
		ImageLeft:        left,
		ImageScaleFactor: float64(width) / srcWidth,
	}
	// it must copy the board path to avoid slice reference issues
	copy(slide.BoardPath, boardPath)

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

	if err = copyPptxTemplateTo(zipWriter); err != nil {
		return err
	}

	var slideFileNames []string
	for i, slide := range p.Slides {
		slideFileName := fmt.Sprintf("slide%d", i+1)
		slideFileNames = append(slideFileNames, slideFileName)

		imageWriter, err := zipWriter.Create(fmt.Sprintf("ppt/media/%s.png", slide.ImageId))
		if err != nil {
			return err
		}
		_, err = imageWriter.Write(slide.Image)
		if err != nil {
			return err
		}

		err = addFile(zipWriter, fmt.Sprintf("ppt/slides/_rels/%s.xml.rels", slideFileName), getRelsSlideXml(slide))
		if err != nil {
			return err
		}

		err = addFile(
			zipWriter,
			fmt.Sprintf("ppt/slides/%s.xml", slideFileName),
			getSlideXml(slide),
		)
		if err != nil {
			return err
		}
	}

	err = addFile(zipWriter, "[Content_Types].xml", getContentTypesXml(slideFileNames))
	if err != nil {
		return err
	}

	err = addFile(zipWriter, "ppt/_rels/presentation.xml.rels", getPresentationXmlRels(slideFileNames))
	if err != nil {
		return err
	}

	err = addFile(zipWriter, "ppt/presentation.xml", getPresentationXml(slideFileNames))
	if err != nil {
		return err
	}

	err = addFile(zipWriter, "docProps/core.xml", getCoreXml(p.Title, p.Subject, p.Description, p.Creator))
	if err != nil {
		return err
	}

	err = addFile(zipWriter, "docProps/app.xml", getAppXml(p.Slides, p.D2Version))
	if err != nil {
		return err
	}

	return nil
}
