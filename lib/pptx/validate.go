package pptx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strings"
)

func Validate(pptxContent []byte, nSlides int) error {
	reader := bytes.NewReader(pptxContent)
	zipReader, err := zip.NewReader(reader, reader.Size())
	if err != nil {
		return fmt.Errorf("error reading pptx content: %v", err)
	}

	if err := validateZipStructure(zipReader); err != nil {
		return fmt.Errorf("invalid ZIP structure: %v", err)
	}

	if err := checkFile(zipReader, "_rels/.rels"); err != nil {
		return fmt.Errorf("missing root relationship file: %v", err)
	}

	if err := validateRootRels(zipReader); err != nil {
		return fmt.Errorf("invalid root relationships: %v", err)
	}

	if err := checkFile(zipReader, "ppt/_rels/presentation.xml.rels"); err != nil {
		return fmt.Errorf("missing presentation relationship file: %v", err)
	}

	if err := checkFile(zipReader, "ppt/slideMasters/_rels/slideMaster1.xml.rels"); err != nil {
		return fmt.Errorf("missing slideMaster relationship file: %v", err)
	}

	if err := validatePresentationXml(zipReader); err != nil {
		return fmt.Errorf("invalid presentation.xml: %v", err)
	}

	if err := validatePresentationRels(zipReader); err != nil {
		return fmt.Errorf("invalid presentation relationships: %v", err)
	}

	for i := 0; i < nSlides; i++ {
		slideNum := i + 1
		if err := checkFile(zipReader, fmt.Sprintf("ppt/slides/slide%d.xml", slideNum)); err != nil {
			return fmt.Errorf("missing slide %d: %v", slideNum, err)
		}
		if err := checkFile(zipReader, fmt.Sprintf("ppt/slides/_rels/slide%d.xml.rels", slideNum)); err != nil {
			return fmt.Errorf("missing slide %d relationship file: %v", slideNum, err)
		}
		if err := checkFile(zipReader, fmt.Sprintf("ppt/media/slide%dImage.png", slideNum)); err != nil {
			return fmt.Errorf("missing slide %d image: %v", slideNum, err)
		}

		if err := validateSlideXml(zipReader, slideNum); err != nil {
			return fmt.Errorf("invalid slide %d: %v", slideNum, err)
		}

	}

	if err := validateCoreXml(zipReader); err != nil {
		return fmt.Errorf("invalid core.xml: %v", err)
	}

	for _, file := range zipReader.File {
		if !strings.Contains(file.Name, ".xml") {
			continue
		}
		if err := validateXmlWellFormed(file); err != nil {
			return fmt.Errorf("invalid XML in %s: %v", file.Name, err)
		}
	}

	expectedCount := getExpectedPptxFileCount(nSlides)
	if len(zipReader.File) != expectedCount {
		return fmt.Errorf("expected %d files, got %d", expectedCount, len(zipReader.File))
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

func validateZipStructure(reader *zip.Reader) error {
	if len(reader.File) == 0 {
		return fmt.Errorf("empty ZIP archive")
	}

	if reader.File[0].Name != "[Content_Types].xml" {
		return fmt.Errorf("first ZIP entry must be [Content_Types].xml, found: %s", reader.File[0].Name)
	}

	fileNames := make(map[string]int)
	for _, file := range reader.File {
		fileNames[file.Name]++
	}

	var duplicates []string
	for fileName, count := range fileNames {
		if count > 1 {
			duplicates = append(duplicates, fileName)
		}
	}

	if len(duplicates) > 0 {
		return fmt.Errorf("duplicate ZIP entries found: %v", duplicates)
	}

	return nil
}

func validateRootRels(reader *zip.Reader) error {
	f, err := reader.Open("_rels/.rels")
	if err != nil {
		return fmt.Errorf("error opening root .rels: %v", err)
	}
	defer f.Close()

	content, err := io.ReadAll(f)
	if err != nil {
		return fmt.Errorf("error reading root .rels: %v", err)
	}

	contentStr := string(content)
	requiredRels := []string{
		`Target="ppt/presentation.xml"`,
		`Target="docProps/core.xml"`,
		`Target="docProps/app.xml"`,
	}

	for _, rel := range requiredRels {
		if !strings.Contains(contentStr, rel) {
			return fmt.Errorf("missing required relationship: %s", rel)
		}
	}

	return nil
}

func validatePresentationXml(reader *zip.Reader) error {
	f, err := reader.Open("ppt/presentation.xml")
	if err != nil {
		return fmt.Errorf("error opening presentation.xml: %v", err)
	}
	defer f.Close()

	content, err := io.ReadAll(f)
	if err != nil {
		return fmt.Errorf("error reading presentation.xml: %v", err)
	}

	contentStr := string(content)

	if strings.Contains(contentStr, `cx="9144000" cy="5143500"`) {
		if !strings.Contains(contentStr, `type="screen16x9"`) {
			return fmt.Errorf("slide size type mismatch: 16:9 dimensions should use type=\"screen16x9\"")
		}
	}

	rIdPattern := regexp.MustCompile(`r:id="(rId\d+)"`)
	matches := rIdPattern.FindAllStringSubmatch(contentStr, -1)
	if len(matches) == 0 {
		return fmt.Errorf("no valid r:id references found")
	}

	for _, match := range matches {
		if !strings.HasPrefix(match[1], "rId") {
			return fmt.Errorf("invalid r:id format: %s (should start with 'rId')", match[1])
		}
	}

	return nil
}

func validatePresentationRels(reader *zip.Reader) error {
	f, err := reader.Open("ppt/_rels/presentation.xml.rels")
	if err != nil {
		return fmt.Errorf("error opening presentation.xml.rels: %v", err)
	}
	defer f.Close()

	content, err := io.ReadAll(f)
	if err != nil {
		return fmt.Errorf("error reading presentation.xml.rels: %v", err)
	}

	contentStr := string(content)

	rIdPattern := regexp.MustCompile(`Id="(rId\d+)"`)
	matches := rIdPattern.FindAllStringSubmatch(contentStr, -1)

	rIdCounts := make(map[string]int)
	for _, match := range matches {
		rId := match[1]
		rIdCounts[rId]++
	}

	var duplicates []string
	for rId, count := range rIdCounts {
		if count > 1 {
			duplicates = append(duplicates, rId)
		}
	}

	if len(duplicates) > 0 {
		return fmt.Errorf("duplicate relationship IDs found in presentation.xml.rels: %v", duplicates)
	}

	return nil
}

func validateSlideXml(reader *zip.Reader, slideNum int) error {
	fileName := fmt.Sprintf("ppt/slides/slide%d.xml", slideNum)
	f, err := reader.Open(fileName)
	if err != nil {
		return fmt.Errorf("error opening %s: %v", fileName, err)
	}
	defer f.Close()

	content, err := io.ReadAll(f)
	if err != nil {
		return fmt.Errorf("error reading %s: %v", fileName, err)
	}

	contentStr := string(content)

	zeroHeightTxtBoxPattern := regexp.MustCompile(`<p:sp>[\s\S]*?txBox="1"[\s\S]*?<a:ext[^>]*cy="0"`)
	if zeroHeightTxtBoxPattern.MatchString(contentStr) {
		return fmt.Errorf("found zero height text box shape which violates OOXML schema")
	}

	return nil
}

func validateCoreXml(reader *zip.Reader) error {
	f, err := reader.Open("docProps/core.xml")
	if err != nil {
		return fmt.Errorf("error opening core.xml: %v", err)
	}
	defer f.Close()

	return nil
}

func validateXmlWellFormed(file *zip.File) error {
	f, err := file.Open()
	if err != nil {
		return fmt.Errorf("error opening %s: %v", file.Name, err)
	}
	defer f.Close()

	decoder := xml.NewDecoder(f)
	for {
		if err := decoder.Decode(new(interface{})); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("error parsing xml content in %s: %v", file.Name, err)
		}
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
	skippedFiles := 2
	generatedFiles := 7
	slideFiles := 3 * nSlides

	return baseFiles - skippedFiles + generatedFiles + slideFiles
}
