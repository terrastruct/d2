package pdf

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestExportToWritesPDFAndPropagatesWriterErrors(t *testing.T) {
	document := testDocument(t)
	var output bytes.Buffer
	err := document.ExportTo(&output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(output.Bytes(), []byte("%PDF-")) || !bytes.Contains(output.Bytes(), []byte("%%EOF")) {
		t.Fatalf("ExportTo output is not a complete PDF: %q", output.Bytes())
	}

	wantErr := errors.New("write failed")
	err = testDocument(t).ExportTo(errorWriter{err: wantErr})
	if !errors.Is(err, wantErr) {
		t.Fatalf("ExportTo error = %v, want %v", err, wantErr)
	}
	err = testDocument(t).ExportTo(partialErrorWriter{err: wantErr})
	if !errors.Is(err, wantErr) {
		t.Fatalf("ExportTo partial-write error = %v, want %v", err, wantErr)
	}
}

func TestExportToAndClosePropagatesCloseError(t *testing.T) {
	wantErr := errors.New("close failed")
	output := &closeErrorWriter{closeErr: wantErr}
	err := testDocument(t).exportToAndClose(output)
	if output.Len() == 0 {
		t.Fatal("exportToAndClose wrote no PDF bytes")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("exportToAndClose error = %v, want %v", err, wantErr)
	}
}

func TestExportWithStatusReportsTouchedTarget(t *testing.T) {
	path := t.TempDir() + "/document.pdf"
	touched, err := testDocument(t).ExportWithStatus(path)
	if err != nil {
		t.Fatal(err)
	}
	if !touched {
		t.Fatal("ExportWithStatus reported an untouched target after creating it")
	}

	missingParentPath := t.TempDir() + "/missing/document.pdf"
	touched, err = testDocument(t).ExportWithStatus(missingParentPath)
	if err == nil {
		t.Fatal("ExportWithStatus unexpectedly created a target in a missing directory")
	}
	if touched {
		t.Fatal("ExportWithStatus reported a touched target when os.Create failed")
	}
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

type closeErrorWriter struct {
	bytes.Buffer
	closeErr error
}

func (w *closeErrorWriter) Close() error {
	return w.closeErr
}

func testDocument(t *testing.T) *GoFPDF {
	t.Helper()

	imageBuffer := &bytes.Buffer{}
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	if err := png.Encode(imageBuffer, img); err != nil {
		t.Fatal(err)
	}

	document := Init()
	if err := document.AddPDFPage(
		imageBuffer.Bytes(),
		[]BoardTitle{{Name: "root", BoardID: "root"}},
		0,
		"#ffffff",
		nil,
		0,
		0,
		0,
		map[string]int{"root": 0},
		true,
	); err != nil {
		t.Fatal(err)
	}
	return document
}
