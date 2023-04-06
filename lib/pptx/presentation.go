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

	// compute the size and position to fit the slide
	if srcSize.X > srcSize.Y {
		width = IMAGE_WIDTH
		height = int(float64(width) * (float64(srcSize.Y) / float64(srcSize.X)))
	} else {
		height = IMAGE_HEIGHT
		width = int(float64(height) * (float64(srcSize.X) / float64(srcSize.Y)))
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
		imageId := fmt.Sprintf("slide%dImage", i+1)
		slideFileName := fmt.Sprintf("slide%d", i+1)
		slideFileNames = append(slideFileNames, slideFileName)

		imageWriter, err := zipWriter.Create(fmt.Sprintf("ppt/media/%s.png", imageId))
		if err != nil {
			return err
		}
		_, err = imageWriter.Write(slide.Image)
		if err != nil {
			return err
		}

		err = addFile(zipWriter, fmt.Sprintf("ppt/slides/_rels/%s.xml.rels", slideFileName), getRelsSlideXml(imageId))
		if err != nil {
			return err
		}

		err = addFile(
			zipWriter,
			fmt.Sprintf("ppt/slides/%s.xml", slideFileName),
			getSlideXml(slide.BoardPath, imageId, slide.ImageTop, slide.ImageLeft, slide.ImageWidth, slide.ImageHeight),
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
