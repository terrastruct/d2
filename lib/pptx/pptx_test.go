package pptx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestPresentationExportToWritesCompleteArchive(t *testing.T) {
	p := testPresentation(t)
	var output bytes.Buffer
	err := p.ExportTo(&output)
	if err != nil {
		t.Fatal(err)
	}
	if output.Len() == 0 {
		t.Fatal("ExportTo produced empty output")
	}
	if err := Validate(output.Bytes(), 1); err != nil {
		t.Fatalf("ExportTo produced invalid PPTX: %v", err)
	}
}

func TestPresentationExportToPropagatesZIPCloseError(t *testing.T) {
	wantErr := errors.New("ZIP close failed")
	output := &zipCloseErrorWriter{err: wantErr}
	err := testPresentation(t).ExportTo(output)
	if err != wantErr {
		t.Fatalf("ExportTo error = %v, want %v", err, wantErr)
	}
}

func TestPresentationExportToPropagatesWriteError(t *testing.T) {
	wantErr := errors.New("write failed")
	err := testPresentation(t).ExportTo(errorWriter{err: wantErr})
	if !errors.Is(err, wantErr) {
		t.Fatalf("ExportTo error = %v, want %v", err, wantErr)
	}
	err = testPresentation(t).ExportTo(partialErrorWriter{err: wantErr})
	if !errors.Is(err, wantErr) {
		t.Fatalf("ExportTo partial-write error = %v, want %v", err, wantErr)
	}
}

func TestPresentationSaveToPropagatesFileCloseError(t *testing.T) {
	wantErr := errors.New("file close failed")
	output := &closeErrorWriter{closeErr: wantErr}
	err := testPresentation(t).writeToAndClose(output)
	if err != wantErr {
		t.Fatalf("saveTo error = %v, want %v", err, wantErr)
	}
	if output.Len() == 0 {
		t.Fatal("writeToAndClose wrote no PPTX bytes")
	}
	if err := Validate(output.Bytes(), 1); err != nil {
		t.Fatalf("saveTo did not finish the archive before closing the file: %v", err)
	}
}

func TestPresentationSaveToPropagatesWriteAndFileCloseErrors(t *testing.T) {
	writeErr := errors.New("write failed")
	closeErr := errors.New("file close failed")
	err := testPresentation(t).writeToAndClose(writeCloseErrorWriter{writeErr: writeErr, closeErr: closeErr})
	if !errors.Is(err, writeErr) || !errors.Is(err, closeErr) {
		t.Fatalf("saveTo error = %v, want both %v and %v", err, writeErr, closeErr)
	}
}

func TestPresentationSaveToWithStatusReportsTouchedTarget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "presentation.pptx")
	touched, err := testPresentation(t).SaveToWithStatus(path)
	if err != nil {
		t.Fatal(err)
	}
	if !touched {
		t.Fatal("SaveToWithStatus reported an untouched target after creating it")
	}

	missingParentPath := filepath.Join(t.TempDir(), "missing", "presentation.pptx")
	touched, err = testPresentation(t).SaveToWithStatus(missingParentPath)
	if err == nil {
		t.Fatal("SaveToWithStatus unexpectedly created a target in a missing directory")
	}
	if touched {
		t.Fatal("SaveToWithStatus reported a touched target when os.Create failed")
	}
}

type zipCloseErrorWriter struct {
	bytes.Buffer
	err error
}

func (w *zipCloseErrorWriter) Write(p []byte) (int, error) {
	if bytes.Contains(p, []byte("PK\x05\x06")) {
		return 0, w.err
	}
	return w.Buffer.Write(p)
}

type closeErrorWriter struct {
	bytes.Buffer
	closeErr error
}

type errorWriter struct {
	err error
}

func (w errorWriter) Write([]byte) (int, error) {
	return 0, w.err
}

type partialErrorWriter struct {
	err error
}

func (w partialErrorWriter) Write(data []byte) (int, error) {
	return min(7, len(data)), w.err
}

type writeCloseErrorWriter struct {
	writeErr error
	closeErr error
}

func (w writeCloseErrorWriter) Write([]byte) (int, error) {
	return 0, w.writeErr
}

func (w writeCloseErrorWriter) Close() error {
	return w.closeErr
}

func (w *closeErrorWriter) Close() error {
	return w.closeErr
}

func testPresentation(t *testing.T) *Presentation {
	t.Helper()

	var pngContent bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.Black)
	if err := png.Encode(&pngContent, img); err != nil {
		t.Fatal(err)
	}
	p := NewPresentation("test", "", "", "", "1", true)
	if _, err := p.AddSlide(pngContent.Bytes(), []BoardTitle{{
		Name:        "root",
		BoardID:     "root",
		LinkToSlide: 1,
	}}); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPresentationEscapesXML(t *testing.T) {
	t.Parallel()

	special := `A & B < C > D "quoted" 'apostrophe'`
	externalURL := `https://example.com/search?q=a%20b&next=%3Ctag%3E&quote=%22double%22&single=%27apostrophe%27`
	p := NewPresentation(special, special, special, special, "1.2.3", true)

	var pngContent bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.Black)
	if err := png.Encode(&pngContent, img); err != nil {
		t.Fatal(err)
	}

	slide, err := p.AddSlide(pngContent.Bytes(), []BoardTitle{{
		Name:        special,
		BoardID:     special,
		LinkToSlide: 1,
	}})
	if err != nil {
		t.Fatal(err)
	}
	slide.AddLink(&Link{
		ExternalUrl: externalURL,
		Tooltip:     special,
		Width:       1,
		Height:      1,
	})
	slide.AddLink(&Link{
		SlideIndex: 1,
		Tooltip:    special,
		Width:      1,
		Height:     1,
	})

	path := filepath.Join(t.TempDir(), "A & B < C > D.pptx")
	if err := p.SaveTo(path); err != nil {
		t.Fatal(err)
	}

	reader, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	for _, file := range reader.File {
		if !strings.HasSuffix(file.Name, ".xml") && !strings.HasSuffix(file.Name, ".rels") {
			continue
		}
		if _, err := parseXMLPart(file); err != nil {
			t.Errorf("invalid XML in %s: %v", file.Name, err)
		}
	}

	core := mustParseXMLPart(t, reader.File, "docProps/core.xml")
	for _, element := range []string{"title", "subject", "creator", "description", "lastModifiedBy"} {
		if !slices.Contains(core.text[element], special) {
			t.Errorf("%s does not round-trip: %q", element, core.text[element])
		}
	}

	app := mustParseXMLPart(t, reader.File, "docProps/app.xml")
	if !slices.Contains(app.text["lpstr"], special) {
		t.Errorf("slide title does not round-trip: %q", app.text["lpstr"])
	}

	slideXML := mustParseXMLPart(t, reader.File, "ppt/slides/slide1.xml")
	if !slices.Contains(slideXML.text["t"], special) {
		t.Errorf("board title does not round-trip: %q", slideXML.text["t"])
	}
	for _, attribute := range []string{"name", "descr", "tooltip"} {
		if !slices.Contains(slideXML.attrs[attribute], special) {
			t.Errorf("%s attribute does not round-trip: %q", attribute, slideXML.attrs[attribute])
		}
	}
	if !slices.Contains(slideXML.attrs["action"], "ppaction://hlinksldjump") {
		t.Errorf("internal slide action changed: %q", slideXML.attrs["action"])
	}

	rels := mustParseXMLPart(t, reader.File, "ppt/slides/_rels/slide1.xml.rels")
	if !slices.Contains(rels.attrs["Target"], externalURL) {
		t.Errorf("external URL does not round-trip: %q", rels.attrs["Target"])
	}
}

type parsedXML struct {
	text  map[string][]string
	attrs map[string][]string
}

func TestParseXMLRejectsCharacterDataBeforeRoot(t *testing.T) {
	t.Parallel()

	_, err := parseXML(strings.NewReader(`&lt;?xml version="1.0"?> <root />`))
	if err == nil {
		t.Fatal("expected escaped XML declaration to be rejected")
	}
}

func parseXMLPart(file *zip.File) (*parsedXML, error) {
	r, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return parseXML(r)
}

func parseXML(r io.Reader) (*parsedXML, error) {
	parsed := &parsedXML{
		text:  make(map[string][]string),
		attrs: make(map[string][]string),
	}
	decoder := xml.NewDecoder(r)
	var elements []string
	rootSeen := false
	rootClosed := false
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			if !rootSeen || !rootClosed {
				return nil, fmt.Errorf("missing or unclosed root element")
			}
			return parsed, nil
		}
		if err != nil {
			return nil, err
		}
		switch token := token.(type) {
		case xml.StartElement:
			if rootClosed {
				return nil, fmt.Errorf("multiple root elements")
			}
			rootSeen = true
			elements = append(elements, token.Name.Local)
			for _, attr := range token.Attr {
				parsed.attrs[attr.Name.Local] = append(parsed.attrs[attr.Name.Local], attr.Value)
			}
		case xml.CharData:
			if (!rootSeen || rootClosed) && strings.TrimSpace(string(token)) != "" {
				return nil, fmt.Errorf("character data outside root element: %q", token)
			}
			if len(elements) > 0 {
				value := strings.TrimSpace(string(token))
				if value != "" {
					name := elements[len(elements)-1]
					parsed.text[name] = append(parsed.text[name], value)
				}
			}
		case xml.EndElement:
			elements = elements[:len(elements)-1]
			if len(elements) == 0 {
				rootClosed = true
			}
		}
	}
}

func mustParseXMLPart(t *testing.T, files []*zip.File, name string) *parsedXML {
	t.Helper()
	for _, file := range files {
		if file.Name != name {
			continue
		}
		parsed, err := parseXMLPart(file)
		if err != nil {
			t.Fatal(err)
		}
		return parsed
	}
	t.Fatalf("%s not found", name)
	return nil
}
