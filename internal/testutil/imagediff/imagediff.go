// Package imagediff compares rendered images and produces deterministic visual
// diagnostics suitable for test failures.
package imagediff

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html/template"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

// Metrics describes the pixel differences between two equal-sized images.
// RMSE is calculated across all four non-premultiplied RGBA channels.
type Metrics struct {
	// Width and Height are the comparison-canvas dimensions. ExpectedWidth,
	// ExpectedHeight, ActualWidth, and ActualHeight retain the source sizes so a
	// crop regression still produces a useful report instead of a nil Result.
	Width          int
	Height         int
	ExpectedWidth  int
	ExpectedHeight int
	ActualWidth    int
	ActualHeight   int

	ChangedPixels   int
	MaxChannelDelta int
	RMSE            float64
}

// Tolerance contains the inclusive limits for a passing comparison. Its zero
// value requires an exact pixel match.
type Tolerance struct {
	MaxChangedPixels int
	MaxChannelDelta  int
	MaxRMSE          float64
}

// Options controls a comparison and the labels in its HTML report.
type Options struct {
	Tolerance    Tolerance
	ExpectedName string
	ActualName   string
}

// Result contains normalized PNG artifacts and a self-contained HTML report.
// The report embeds all four PNGs as data URLs and has no external resources.
type Result struct {
	Metrics   Metrics
	Tolerance Tolerance
	Passed    bool

	ExpectedPNG []byte
	ActualPNG   []byte
	OverlayPNG  []byte
	HeatmapPNG  []byte
	ReportHTML  []byte
}

// MismatchError reports a comparison that exceeded at least one tolerance.
// Compare still returns a complete Result alongside this error.
type MismatchError struct {
	Metrics   Metrics
	Tolerance Tolerance
}

func (e *MismatchError) Error() string {
	return fmt.Sprintf(
		"image comparison failed: changed pixels %d (limit %d), max channel delta %d (limit %d), RMSE %.6f (limit %.6f)",
		e.Metrics.ChangedPixels,
		e.Tolerance.MaxChangedPixels,
		e.Metrics.MaxChannelDelta,
		e.Tolerance.MaxChannelDelta,
		e.Metrics.RMSE,
		e.Tolerance.MaxRMSE,
	)
}

// DimensionMismatchError reports images whose pixel dimensions differ.
type DimensionMismatchError struct {
	Expected image.Point
	Actual   image.Point
}

func (e *DimensionMismatchError) Error() string {
	return fmt.Sprintf(
		"image dimensions differ: expected %dx%d, actual %dx%d",
		e.Expected.X,
		e.Expected.Y,
		e.Actual.X,
		e.Actual.Y,
	)
}

// Compare decodes two encoded images, compares them, and creates diagnostics.
// PNG, JPEG, and GIF inputs are registered by this package.
func Compare(expected, actual []byte, opts Options) (*Result, error) {
	expectedImage, _, err := image.Decode(bytes.NewReader(expected))
	if err != nil {
		return nil, fmt.Errorf("decode expected image: %w", err)
	}
	actualImage, _, err := image.Decode(bytes.NewReader(actual))
	if err != nil {
		return nil, fmt.Errorf("decode actual image: %w", err)
	}
	return CompareImages(expectedImage, actualImage, opts)
}

// CompareImages compares two decoded images and creates diagnostics. Image
// bounds may have different origins, but their dimensions must match exactly.
func CompareImages(expected, actual image.Image, opts Options) (*Result, error) {
	if expected == nil {
		return nil, fmt.Errorf("expected image is nil")
	}
	if actual == nil {
		return nil, fmt.Errorf("actual image is nil")
	}
	if err := validateTolerance(opts.Tolerance); err != nil {
		return nil, err
	}
	expectedBounds := expected.Bounds()
	actualBounds := actual.Bounds()
	expectedSize, actualSize := expectedBounds.Size(), actualBounds.Size()
	if expectedSize.X <= 0 || expectedSize.Y <= 0 {
		return nil, fmt.Errorf("expected image dimensions must be positive, got %dx%d", expectedSize.X, expectedSize.Y)
	}
	if actualSize.X <= 0 || actualSize.Y <= 0 {
		return nil, fmt.Errorf("actual image dimensions must be positive, got %dx%d", actualSize.X, actualSize.Y)
	}

	expectedRect := image.Rect(0, 0, expectedSize.X, expectedSize.Y)
	actualRect := image.Rect(0, 0, actualSize.X, actualSize.Y)
	comparisonSize := image.Point{X: max(expectedSize.X, actualSize.X), Y: max(expectedSize.Y, actualSize.Y)}
	comparisonRect := image.Rect(0, 0, comparisonSize.X, comparisonSize.Y)
	expectedNRGBA := image.NewNRGBA(expectedRect)
	actualNRGBA := image.NewNRGBA(actualRect)
	overlay := image.NewNRGBA(comparisonRect)
	heatmap := image.NewNRGBA(comparisonRect)

	metrics := Metrics{
		Width:          comparisonSize.X,
		Height:         comparisonSize.Y,
		ExpectedWidth:  expectedSize.X,
		ExpectedHeight: expectedSize.Y,
		ActualWidth:    actualSize.X,
		ActualHeight:   actualSize.Y,
	}
	var squaredDeltaSum float64

	for y := 0; y < comparisonSize.Y; y++ {
		for x := 0; x < comparisonSize.X; x++ {
			var expectedColor, actualColor color.NRGBA
			if x < expectedSize.X && y < expectedSize.Y {
				expectedColor = toNRGBA(expected.At(expectedBounds.Min.X+x, expectedBounds.Min.Y+y))
				expectedNRGBA.SetNRGBA(x, y, expectedColor)
			}
			if x < actualSize.X && y < actualSize.Y {
				actualColor = toNRGBA(actual.At(actualBounds.Min.X+x, actualBounds.Min.Y+y))
				actualNRGBA.SetNRGBA(x, y, actualColor)
			}

			deltas := [4]int{
				absDiff(expectedColor.R, actualColor.R),
				absDiff(expectedColor.G, actualColor.G),
				absDiff(expectedColor.B, actualColor.B),
				absDiff(expectedColor.A, actualColor.A),
			}
			pixelMax := 0
			for _, delta := range deltas {
				if delta > pixelMax {
					pixelMax = delta
				}
				if delta > metrics.MaxChannelDelta {
					metrics.MaxChannelDelta = delta
				}
				squaredDeltaSum += float64(delta * delta)
			}
			if pixelMax != 0 {
				metrics.ChangedPixels++
				heatmap.SetNRGBA(x, y, color.NRGBA{
					R: 255,
					G: uint8(255 - pixelMax),
					A: 255,
				})
			}
			overlay.SetNRGBA(x, y, color.NRGBA{
				R: roundedAverage(expectedColor.R, actualColor.R),
				G: roundedAverage(expectedColor.G, actualColor.G),
				B: roundedAverage(expectedColor.B, actualColor.B),
				A: roundedAverage(expectedColor.A, actualColor.A),
			})
		}
	}

	channelCount := float64(comparisonSize.X) * float64(comparisonSize.Y) * 4
	metrics.RMSE = math.Sqrt(squaredDeltaSum / channelCount)
	dimensionsMatch := expectedSize == actualSize
	passed := dimensionsMatch && withinTolerance(metrics, opts.Tolerance)

	result := &Result{
		Metrics:   metrics,
		Tolerance: opts.Tolerance,
		Passed:    passed,
	}
	var err error
	result.ExpectedPNG, err = encodePNG(expectedNRGBA)
	if err != nil {
		return nil, fmt.Errorf("encode expected PNG: %w", err)
	}
	result.ActualPNG, err = encodePNG(actualNRGBA)
	if err != nil {
		return nil, fmt.Errorf("encode actual PNG: %w", err)
	}
	result.OverlayPNG, err = encodePNG(overlay)
	if err != nil {
		return nil, fmt.Errorf("encode overlay PNG: %w", err)
	}
	result.HeatmapPNG, err = encodePNG(heatmap)
	if err != nil {
		return nil, fmt.Errorf("encode heatmap PNG: %w", err)
	}
	result.ReportHTML, err = buildReport(result, opts)
	if err != nil {
		return nil, fmt.Errorf("build HTML report: %w", err)
	}

	if !dimensionsMatch {
		return result, &DimensionMismatchError{Expected: expectedSize, Actual: actualSize}
	}
	if !passed {
		return result, &MismatchError{
			Metrics:   metrics,
			Tolerance: opts.Tolerance,
		}
	}
	return result, nil
}

// WriteReport writes the self-contained HTML report, creating parent
// directories as needed.
func (r *Result) WriteReport(path string) error {
	if r == nil {
		return fmt.Errorf("image comparison result is nil")
	}
	if len(r.ReportHTML) == 0 {
		return fmt.Errorf("image comparison report is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create report directory: %w", err)
	}
	if err := os.WriteFile(path, r.ReportHTML, 0o644); err != nil {
		return fmt.Errorf("write image comparison report: %w", err)
	}
	return nil
}

func validateTolerance(tolerance Tolerance) error {
	if tolerance.MaxChangedPixels < 0 {
		return fmt.Errorf("maximum changed pixels must be non-negative")
	}
	if tolerance.MaxChannelDelta < 0 || tolerance.MaxChannelDelta > 255 {
		return fmt.Errorf("maximum channel delta must be between 0 and 255")
	}
	if math.IsNaN(tolerance.MaxRMSE) || tolerance.MaxRMSE < 0 {
		return fmt.Errorf("maximum RMSE must be non-negative")
	}
	return nil
}

func withinTolerance(metrics Metrics, tolerance Tolerance) bool {
	return metrics.ChangedPixels <= tolerance.MaxChangedPixels &&
		metrics.MaxChannelDelta <= tolerance.MaxChannelDelta &&
		metrics.RMSE <= tolerance.MaxRMSE
}

func toNRGBA(c color.Color) color.NRGBA {
	return color.NRGBAModel.Convert(c).(color.NRGBA)
}

func absDiff(a, b uint8) int {
	if a >= b {
		return int(a - b)
	}
	return int(b - a)
}

func roundedAverage(a, b uint8) uint8 {
	return uint8((uint16(a) + uint16(b) + 1) / 2)
}

func encodePNG(img image.Image) ([]byte, error) {
	var out bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := encoder.Encode(&out, img); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

type reportData struct {
	StatusClass   string
	Status        string
	ZeroTolerance bool
	Metrics       Metrics
	Tolerance     Tolerance
	RMSE          string
	MaxRMSE       string
	Images        []reportImage
}

type reportImage struct {
	Name    string
	DataURL template.URL
}

func buildReport(result *Result, opts Options) ([]byte, error) {
	expectedName := opts.ExpectedName
	if expectedName == "" {
		expectedName = "Expected"
	}
	actualName := opts.ActualName
	if actualName == "" {
		actualName = "Actual"
	}
	statusClass := "fail"
	status := "FAIL"
	if result.Passed {
		statusClass = "pass"
		status = "PASS"
	}
	data := reportData{
		StatusClass:   statusClass,
		Status:        status,
		ZeroTolerance: result.Tolerance == (Tolerance{}),
		Metrics:       result.Metrics,
		Tolerance:     result.Tolerance,
		RMSE:          fmt.Sprintf("%.6f", result.Metrics.RMSE),
		MaxRMSE:       fmt.Sprintf("%.6f", result.Tolerance.MaxRMSE),
		Images: []reportImage{
			{Name: expectedName, DataURL: pngDataURL(result.ExpectedPNG)},
			{Name: actualName, DataURL: pngDataURL(result.ActualPNG)},
			{Name: "50% overlay", DataURL: pngDataURL(result.OverlayPNG)},
			{Name: "Difference heatmap", DataURL: pngDataURL(result.HeatmapPNG)},
		},
	}

	var out bytes.Buffer
	if err := reportTemplate.Execute(&out, data); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func pngDataURL(data []byte) template.URL {
	return template.URL("data:image/png;base64," + base64.StdEncoding.EncodeToString(data))
}

var reportTemplate = template.Must(template.New("image-diff-report").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Image comparison report</title>
<style>
:root { color-scheme: light dark; font-family: ui-sans-serif, system-ui, sans-serif; }
body { margin: 0; padding: 24px; background: #111827; color: #e5e7eb; }
h1 { margin: 0 0 16px; font-size: 24px; }
.status { display: inline-block; margin-left: 8px; padding: 3px 8px; border-radius: 999px; font-size: 13px; }
.status.pass { background: #065f46; color: #d1fae5; }
.status.fail { background: #991b1b; color: #fee2e2; }
table { border-collapse: collapse; margin-bottom: 24px; }
th, td { border: 1px solid #4b5563; padding: 7px 10px; text-align: right; }
th:first-child, td:first-child { text-align: left; }
.images { display: grid; grid-template-columns: repeat(auto-fit, minmax(280px, 1fr)); gap: 16px; }
figure { margin: 0; padding: 12px; border: 1px solid #4b5563; border-radius: 8px; background: #1f2937; }
figcaption { margin-bottom: 8px; font-weight: 600; }
.checker { overflow: auto; padding: 8px; background-color: #fff; background-image: linear-gradient(45deg, #ddd 25%, transparent 25%), linear-gradient(-45deg, #ddd 25%, transparent 25%), linear-gradient(45deg, transparent 75%, #ddd 75%), linear-gradient(-45deg, transparent 75%, #ddd 75%); background-size: 16px 16px; background-position: 0 0, 0 8px, 8px -8px, -8px 0; }
img { display: block; max-width: none; image-rendering: pixelated; }
</style>
</head>
<body class="{{.StatusClass}}">
<h1>Pixel comparison: <span class="status {{.StatusClass}}">{{.Status}}</span>{{if .ZeroTolerance}} — zero tolerance{{end}}</h1>
<table>
<thead><tr><th>Metric</th><th>Observed</th><th>Allowed</th></tr></thead>
<tbody>
<tr><td>Dimensions</td><td>expected {{.Metrics.ExpectedWidth}}x{{.Metrics.ExpectedHeight}}; actual {{.Metrics.ActualWidth}}x{{.Metrics.ActualHeight}}</td><td>exact</td></tr>
<tr><td>Changed pixels</td><td>{{.Metrics.ChangedPixels}}</td><td>{{.Tolerance.MaxChangedPixels}}</td></tr>
<tr><td>Maximum channel delta</td><td>{{.Metrics.MaxChannelDelta}}</td><td>{{.Tolerance.MaxChannelDelta}}</td></tr>
<tr><td>RGBA RMSE</td><td>{{.RMSE}}</td><td>{{.MaxRMSE}}</td></tr>
</tbody>
</table>
<section class="images">
{{range .Images}}<figure><figcaption>{{.Name}}</figcaption><div class="checker"><img alt="{{.Name}}" src="{{.DataURL}}"></div></figure>{{end}}
</section>
</body>
</html>
`))
