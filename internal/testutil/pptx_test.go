package testutil

import (
	"archive/zip"
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/d2lang/d2/internal/testutil/imagediff"
)

func TestValidatePPTXRejectsNonZIP(t *testing.T) {
	t.Parallel()

	assert.EqualError(t, ValidatePPTX([]byte("not a ZIP archive"), nil, 1), "error reading pptx content: zip: not a valid zip file")
}

func TestExtractAndComparePPTXImages(t *testing.T) {
	t.Parallel()

	red := encodePPTXTestPNG(t, color.NRGBA{R: 255, A: 255})
	blue := encodePPTXTestPNG(t, color.NRGBA{B: 255, A: 255})
	presentation := pptxImageArchive(t, map[string][]byte{
		"ppt/media/slide2Image.png": blue,
		"ppt/media/slide1Image.png": red,
	})
	images, err := ExtractPPTXImages(presentation)
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 2 || !bytes.Equal(images[0], red) || !bytes.Equal(images[1], blue) {
		t.Fatal("PPTX images were not extracted in slide order")
	}
	if _, err := ComparePPTXImages(presentation, [][]byte{red, blue}, imagediff.Options{}); err != nil {
		t.Fatal(err)
	}

	comparison, err := ComparePPTXImages(presentation, [][]byte{blue, red}, imagediff.Options{})
	if err == nil || comparison == nil || comparison.Page != 1 || comparison.Result == nil {
		t.Fatalf("PPTX pixel mismatch = %#v/%v", comparison, err)
	}
	var mismatch *imagediff.MismatchError
	if !errors.As(err, &mismatch) || strings.Count(string(comparison.Result.ReportHTML), "data:image/png;base64,") != 4 {
		t.Fatalf("PPTX pixel mismatch diagnostics = %v", err)
	}
}

func TestExtractPPTXImagesRejectsGaps(t *testing.T) {
	t.Parallel()

	presentation := pptxImageArchive(t, map[string][]byte{
		"ppt/media/slide2Image.png": encodePPTXTestPNG(t, color.NRGBA{A: 255}),
	})
	if _, err := ExtractPPTXImages(presentation); err == nil || !strings.Contains(err.Error(), "slide 1") {
		t.Fatalf("PPTX image gap error = %v", err)
	}
}

func pptxImageArchive(t *testing.T, members map[string][]byte) []byte {
	t.Helper()
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	for name, content := range members {
		member, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := member.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func encodePPTXTestPNG(t *testing.T, fill color.NRGBA) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for index := 0; index < len(img.Pix); index += 4 {
		img.Pix[index], img.Pix[index+1], img.Pix[index+2], img.Pix[index+3] = fill.R, fill.G, fill.B, fill.A
	}
	var output bytes.Buffer
	if err := png.Encode(&output, img); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func TestExpectedPPTXFileCount(t *testing.T) {
	t.Parallel()

	var template bytes.Buffer
	w := zip.NewWriter(&template)
	for _, name := range []string{"one", "two", "three"} {
		_, err := w.Create(name)
		assert.NoError(t, err)
	}
	assert.NoError(t, w.Close())

	assert.Equal(t, 8, getExpectedPptxFileCount(template.Bytes(), 0))
	assert.Equal(t, 20, getExpectedPptxFileCount(template.Bytes(), 4))
	assert.Equal(t, -1, getExpectedPptxFileCount([]byte("invalid"), 1))
}
