package imagediff

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompareZeroToleranceMismatchProducesDeterministicArtifacts(t *testing.T) {
	t.Parallel()

	expected := solidImage(image.Rect(0, 0, 2, 2), color.NRGBA{R: 10, G: 20, B: 30, A: 255})
	actual := solidImage(image.Rect(0, 0, 2, 2), color.NRGBA{R: 10, G: 20, B: 30, A: 255})
	actual.SetNRGBA(1, 0, color.NRGBA{R: 20, G: 20, B: 30, A: 255})

	expectedBytes := mustEncodePNG(t, expected)
	actualBytes := mustEncodePNG(t, actual)
	result, err := Compare(expectedBytes, actualBytes, Options{})
	if err == nil {
		t.Fatal("Compare() error = nil, want zero-tolerance mismatch")
	}
	var mismatch *MismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("Compare() error type = %T, want *MismatchError", err)
	}
	if result == nil {
		t.Fatal("Compare() result = nil on mismatch")
	}
	if result.Passed {
		t.Fatal("Compare() Passed = true on mismatch")
	}
	if result.Metrics.ChangedPixels != 1 {
		t.Fatalf("ChangedPixels = %d, want 1", result.Metrics.ChangedPixels)
	}
	if result.Metrics.MaxChannelDelta != 10 {
		t.Fatalf("MaxChannelDelta = %d, want 10", result.Metrics.MaxChannelDelta)
	}
	if result.Metrics.RMSE != 2.5 {
		t.Fatalf("RMSE = %v, want 2.5", result.Metrics.RMSE)
	}

	artifacts := map[string][]byte{
		"expected": result.ExpectedPNG,
		"actual":   result.ActualPNG,
		"overlay":  result.OverlayPNG,
		"heatmap":  result.HeatmapPNG,
	}
	for name, artifact := range artifacts {
		if len(artifact) == 0 {
			t.Fatalf("%s artifact is empty", name)
		}
		decoded, err := png.Decode(bytes.NewReader(artifact))
		if err != nil {
			t.Fatalf("decode %s artifact: %v", name, err)
		}
		if got := decoded.Bounds().Size(); got != (image.Point{X: 2, Y: 2}) {
			t.Fatalf("%s artifact size = %v, want 2x2", name, got)
		}
	}
	if got := toNRGBA(mustDecodePNG(t, result.OverlayPNG).At(1, 0)); got != (color.NRGBA{R: 15, G: 20, B: 30, A: 255}) {
		t.Fatalf("overlay changed pixel = %#v", got)
	}
	if got := toNRGBA(mustDecodePNG(t, result.HeatmapPNG).At(1, 0)); got != (color.NRGBA{R: 255, G: 245, A: 255}) {
		t.Fatalf("heatmap changed pixel = %#v", got)
	}
	if got := strings.Count(string(result.ReportHTML), "data:image/png;base64,"); got != 4 {
		t.Fatalf("embedded PNG count = %d, want 4", got)
	}
	if !bytes.Contains(result.ReportHTML, []byte("<td>1</td><td>0</td>")) {
		t.Fatalf("report does not contain changed-pixel metric: %s", result.ReportHTML)
	}

	reportPath := filepath.Join(t.TempDir(), "nested", "comparison.html")
	if err := result.WriteReport(reportPath); err != nil {
		t.Fatalf("WriteReport() error: %v", err)
	}
	written, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	if !bytes.Equal(written, result.ReportHTML) {
		t.Fatal("written report differs from Result.ReportHTML")
	}

	second, secondErr := Compare(expectedBytes, actualBytes, Options{})
	if secondErr == nil {
		t.Fatal("second Compare() error = nil, want mismatch")
	}
	if !bytes.Equal(result.ExpectedPNG, second.ExpectedPNG) ||
		!bytes.Equal(result.ActualPNG, second.ActualPNG) ||
		!bytes.Equal(result.OverlayPNG, second.OverlayPNG) ||
		!bytes.Equal(result.HeatmapPNG, second.HeatmapPNG) ||
		!bytes.Equal(result.ReportHTML, second.ReportHTML) {
		t.Fatal("comparison artifacts are not deterministic")
	}
}

func TestCompareExactMatchPassesZeroTolerance(t *testing.T) {
	t.Parallel()

	img := solidImage(image.Rect(5, 7, 8, 9), color.NRGBA{R: 1, G: 2, B: 3, A: 4})
	result, err := CompareImages(img, img, Options{})
	if err != nil {
		t.Fatalf("CompareImages() error: %v", err)
	}
	if !result.Passed {
		t.Fatal("CompareImages() Passed = false for identical images")
	}
	if result.Metrics.ChangedPixels != 0 || result.Metrics.MaxChannelDelta != 0 || result.Metrics.RMSE != 0 {
		t.Fatalf("identical-image metrics = %+v", result.Metrics)
	}
	if !bytes.Contains(result.ReportHTML, []byte(">PASS<")) {
		t.Fatal("passing report does not include PASS status")
	}
}

func TestCompareToleranceIsInclusive(t *testing.T) {
	t.Parallel()

	expected := solidImage(image.Rect(0, 0, 2, 2), color.NRGBA{A: 255})
	actual := solidImage(image.Rect(0, 0, 2, 2), color.NRGBA{A: 255})
	actual.SetNRGBA(0, 0, color.NRGBA{R: 10, A: 255})

	result, err := CompareImages(expected, actual, Options{
		Tolerance: Tolerance{
			MaxChangedPixels: 1,
			MaxChannelDelta:  10,
			MaxRMSE:          2.5,
		},
	})
	if err != nil {
		t.Fatalf("CompareImages() error: %v", err)
	}
	if !result.Passed {
		t.Fatal("comparison at exact tolerance boundary did not pass")
	}
}

func TestCompareRejectsDimensionMismatch(t *testing.T) {
	t.Parallel()

	expected := solidImage(image.Rect(0, 0, 2, 2), color.NRGBA{})
	actual := solidImage(image.Rect(0, 0, 3, 2), color.NRGBA{})
	result, err := Compare(mustEncodePNG(t, expected), mustEncodePNG(t, actual), Options{})
	if result == nil {
		t.Fatal("CompareImages() result = nil, want diagnostics for dimension mismatch")
	}
	if result.Passed {
		t.Fatal("CompareImages() Passed = true for dimension mismatch")
	}
	if result.Metrics.ExpectedWidth != 2 || result.Metrics.ExpectedHeight != 2 ||
		result.Metrics.ActualWidth != 3 || result.Metrics.ActualHeight != 2 {
		t.Fatalf("dimension metrics = %+v", result.Metrics)
	}
	if strings.Count(string(result.ReportHTML), "data:image/png;base64,") != 4 ||
		!bytes.Contains(result.ReportHTML, []byte("expected 2x2; actual 3x2")) {
		t.Fatalf("dimension mismatch report is incomplete: %s", result.ReportHTML)
	}
	var mismatch *DimensionMismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("CompareImages() error = %v, want *DimensionMismatchError", err)
	}
	if mismatch.Expected != (image.Point{X: 2, Y: 2}) || mismatch.Actual != (image.Point{X: 3, Y: 2}) {
		t.Fatalf("dimension mismatch = %+v", mismatch)
	}
}

func TestCompareRejectsInvalidTolerance(t *testing.T) {
	t.Parallel()

	img := solidImage(image.Rect(0, 0, 1, 1), color.NRGBA{})
	tests := []Tolerance{
		{MaxChangedPixels: -1},
		{MaxChannelDelta: -1},
		{MaxChannelDelta: 256},
		{MaxRMSE: -1},
		{MaxRMSE: math.NaN()},
	}
	for _, tolerance := range tests {
		if _, err := CompareImages(img, img, Options{Tolerance: tolerance}); err == nil {
			t.Fatalf("CompareImages() accepted invalid tolerance %+v", tolerance)
		}
	}
}

func solidImage(bounds image.Rectangle, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

func mustEncodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func mustDecodePNG(t *testing.T, data []byte) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return img
}
