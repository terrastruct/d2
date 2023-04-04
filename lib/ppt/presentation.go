package ppt

import (
	"archive/zip"
	"bytes"
	_ "embed"
	"fmt"
	"image/png"
	"os"
)

type Pptx struct {
	Slides []*Slide
}

type Slide struct {
	Image  []byte
	Width  int
	Height int
}

func NewPresentation() *Pptx {
	return &Pptx{}
}

func (p *Pptx) AddSlide(pngContent []byte) error {
	src, err := png.Decode(bytes.NewReader(pngContent))
	if err != nil {
		return fmt.Errorf("error decoding PNG image: %v", err)
	}

	srcSize := src.Bounds().Size()
	height := int(float64(SLIDE_WIDTH) * (float64(srcSize.X) / float64(srcSize.Y)))

	p.Slides = append(p.Slides, &Slide{
		Image:  pngContent,
		Width:  SLIDE_WIDTH,
		Height: height,
	})

	return nil
}

func (p *Pptx) SaveTo(filePath string) error {
	// TODO: update core files with metadata

	f, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer f.Close()
	zipFile := zip.NewWriter(f)
	defer zipFile.Close()

	copyPptxTemplateTo(zipFile)

	var slideFileNames []string
	for i, slide := range p.Slides {
		imageId := fmt.Sprintf("slide%dImage", i+1)
		slideFileName := fmt.Sprintf("slide%d", i+1)
		slideFileNames = append(slideFileNames, slideFileName)

		imageWriter, err := zipFile.Create(fmt.Sprintf("ppt/media/%s.png", imageId))
		if err != nil {
			return err
		}
		_, err = imageWriter.Write(slide.Image)
		if err != nil {
			return err
		}

		err = addFile(zipFile, fmt.Sprintf("ppt/slides/_rels/%s.xml.rels", slideFileName), getRelsSlideXml(imageId))
		if err != nil {
			return err
		}

		// TODO: center the image?
		err = addFile(zipFile, fmt.Sprintf("ppt/slides/%s.xml", slideFileName), getSlideXml(imageId, imageId, 0, 0, slide.Width, slide.Height))
		if err != nil {
			return err
		}
	}

	err = addFile(zipFile, "[Content_Types].xml", getContentTypesXml(slideFileNames))
	if err != nil {
		return err
	}

	err = addFile(zipFile, "ppt/_rels/presentation.xml.rels", getPresentationXmlRels(slideFileNames))
	if err != nil {
		return err
	}

	err = addFile(zipFile, "ppt/presentation.xml", getPresentationXml(slideFileNames))
	if err != nil {
		return err
	}

	return nil
}
