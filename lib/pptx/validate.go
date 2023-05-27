package pptx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

func Validate(pptxContent []byte, nSlides int) error {
	reader := bytes.NewReader(pptxContent)
	zipReader, err := zip.NewReader(reader, reader.Size())
	if err != nil {
		fmt.Printf("error reading pptx content: %v", err)
	}

	expectedCount := getExpectedPptxFileCount(nSlides)
	if len(zipReader.File) != expectedCount {
		return fmt.Errorf("expected %d files, got %d", expectedCount, len(zipReader.File))
	}

	for i := 0; i < nSlides; i++ {
		if err := checkFile(zipReader, fmt.Sprintf("ppt/slides/slide%d.xml", i+1)); err != nil {
			return err
		}
		if err := checkFile(zipReader, fmt.Sprintf("ppt/slides/_rels/slide%d.xml.rels", i+1)); err != nil {
			return err
		}
		if err := checkFile(zipReader, fmt.Sprintf("ppt/media/slide%dImage.png", i+1)); err != nil {
			return err
		}
	}

	for _, file := range zipReader.File {
		if !strings.Contains(file.Name, ".xml") {
			continue
		}
		// checks if the XML content is valid
		f, err := file.Open()
		if err != nil {
			return fmt.Errorf("error opening %s: %v", file.Name, err)
		}
		decoder := xml.NewDecoder(f)
		for {
			if err := decoder.Decode(new(interface{})); err != nil {
				if err == io.EOF {
					break
				}
				return fmt.Errorf("error parsing xml content in %s: %v", file.Name, err)
			}
		}
		defer f.Close()
	}

	return nil
}

func checkFile(reader *zip.Reader, fname string) error {
	f, err := reader.Open(fname)
	if err != nil {
		return fmt.Errorf("error opening file %s: %v", fname, err)
	}
	defer f.Close()
	if _, err = f.Stat(); err != nil {
		return fmt.Errorf("error getting file info %s: %v", fname, err)
	}
	return nil
}

func getExpectedPptxFileCount(nSlides int) int {
	reader := bytes.NewReader(PPTX_TEMPLATE)
	zipReader, err := zip.NewReader(reader, reader.Size())
	if err != nil {
		return -1
	}
	baseFiles := len(zipReader.File)
	presentationFiles := 5    // presentation, rels, app, core, content types
	slideFiles := 3 * nSlides // slides, rels, images
	return baseFiles + presentationFiles + slideFiles
}
