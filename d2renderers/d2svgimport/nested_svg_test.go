package d2svgimport

import (
	"context"
	"image/color"
	"math"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2latex"
	"github.com/d2lang/d2/d2renderers/d2raster"
	"github.com/d2lang/d2/d2renderers/d2scene"
)

func TestImportNodeNestedSVGViewportTopologyAndPixels(t *testing.T) {
	result := mustImport(t, `<svg width="20" height="15" viewBox="0 0 20 15">
  <svg id="nested" class="viewport" x="5" y="3" width="10" height="8" viewBox="0 0 10 10" preserveAspectRatio="none" opacity=".5">
    <rect x="-10" y="-10" width="30" height="30" fill="#ff0000"/>
  </svg>
</svg>`)
	if len(result.Root.Children) != 1 {
		t.Fatalf("root children = %d, want 1", len(result.Root.Children))
	}
	viewport := result.Root.Children[0]
	if viewport.ID != "nested" || len(viewport.Classes) != 1 || viewport.Classes[0] != "viewport" ||
		viewport.Transform != d2scene.Translate(5, 3) || !nearFloat(viewport.Opacity, .5) {
		t.Fatalf("nested viewport node = %+v", viewport)
	}
	if viewport.Clip == nil || viewport.Clip.Transform != d2scene.Identity() ||
		viewport.Clip.Path.FillRule != d2scene.NonZero || len(viewport.Clip.Path.Commands) != 5 {
		t.Fatalf("nested viewport clip = %+v", viewport.Clip)
	}
	if len(viewport.Children) != 1 || viewport.Children[0].Transform != d2scene.Scale(1, .8) ||
		len(viewport.Children[0].Children) != 1 {
		t.Fatalf("nested viewport content = %+v", viewport.Children)
	}
	if result.Metrics.ParsedElements != 3 || result.Metrics.EmittedElements != 4 || result.Metrics.EmittedPathCommands != 5 {
		t.Fatalf("nested viewport metrics = %+v", result.Metrics)
	}

	document := d2scene.NewDocument(d2scene.Box{Width: 20, Height: 15}, result.Root)
	frame, err := d2raster.Render(context.Background(), document, d2raster.FrameOptions{
		Scale: 1, MaxWidth: 20, MaxHeight: 15, MaxPixels: 300,
		MaxNodes: 10, MaxDepth: 10, MaxPathCommands: 20,
		MaxAnimationTracks: 1, MaxAnimationKeyframes: 1,
		MaxAssets: 1, MaxAssetBytes: 1, MaxDecodedAssetBytes: 1, MaxImportDepth: 10,
		MaxOffscreenBytes: 1 << 20, MaxEvenOddClipWork: 1 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	inside := color.NRGBAModel.Convert(frame.At(7, 5)).(color.NRGBA)
	if inside.R < 250 || inside.A < 120 || inside.A > 135 {
		t.Fatalf("inside nested viewport = %+v", inside)
	}
	for _, point := range []struct{ x, y int }{{4, 5}, {15, 5}, {7, 2}, {7, 11}} {
		if pixel := color.NRGBAModel.Convert(frame.At(point.x, point.y)).(color.NRGBA); pixel.A != 0 {
			t.Fatalf("pixel outside nested viewport at %d,%d = %+v", point.x, point.y, pixel)
		}
	}
}

func TestImportNodeFrozenMathJaxNestedViewports(t *testing.T) {
	formula := `\begin{CD} B @>{\text{very long label}}>> C S^{{\mathcal{W}}_\Lambda}\otimes T @>j>> T\\ @VVV V \end{CD}`
	source, err := d2latex.Render(formula)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(source, "<svg") < 2 {
		t.Fatal("frozen commutative-diagram output no longer contains a nested SVG viewport")
	}
	result, err := ImportNode(context.Background(), "mathjax-amscd.svg", []byte(source), generousImportLimits())
	if err != nil {
		t.Fatalf("ImportNode frozen MathJax commutative diagram: %v", err)
	}
	width, height, err := d2latex.Measure(formula)
	if err != nil {
		t.Fatal(err)
	}
	if int(math.Ceil(result.Width)) != width || int(math.Ceil(result.Height)) != height {
		t.Fatalf("MathJax viewport %gx%g does not match measured %dx%d", result.Width, result.Height, width, height)
	}
	viewportClips := 0
	stack := []*d2scene.Node{result.Root}
	for len(stack) != 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if node.Clip != nil && len(node.Clip.Path.Commands) == 5 {
			viewportClips++
		}
		stack = append(stack, node.Children...)
	}
	if viewportClips == 0 || result.Metrics.EmittedPathCommands < viewportClips*5 {
		t.Fatalf("nested viewport clips = %d, metrics = %+v", viewportClips, result.Metrics)
	}
}

func TestImportNodeRejectsUnsupportedNestedSVGViewports(t *testing.T) {
	tests := []struct {
		name   string
		nested string
		want   string
	}{
		{"missing viewBox", `<svg width="1" height="1"/>`, "requires an explicit viewBox"},
		{"missing width", `<svg height="1" viewBox="0 0 1 1"/>`, "requires explicit width and height"},
		{"missing height", `<svg width="1" viewBox="0 0 1 1"/>`, "requires explicit width and height"},
		{"zero width", `<svg width="0" height="1" viewBox="0 0 1 1"/>`, "width and height must be positive"},
		{"relative x", `<svg x="1ex" width="1" height="1" viewBox="0 0 1 1"/>`, "invalid x"},
		{"relative dimensions", `<svg width="1ex" height="1" viewBox="0 0 1 1"/>`, "invalid width"},
		{"version", `<svg version="1.1" width="1" height="1" viewBox="0 0 1 1"/>`, "version is unsupported"},
		{"enable background attribute", `<svg enable-background="new 0 0 1 1" width="1" height="1" viewBox="0 0 1 1"/>`, "enable-background is unsupported"},
		{"enable background style", `<svg style="enable-background:new 0 0 1 1" width="1" height="1" viewBox="0 0 1 1"/>`, "outside the root"},
		{"vertical align", `<svg style="vertical-align:-1ex" width="1" height="1" viewBox="0 0 1 1"/>`, "outside the root"},
		{"transform", `<svg transform="translate(1 1)" width="1" height="1" viewBox="0 0 1 1"/>`, "transform is unsupported"},
		{"clip path", `<svg clip-path="none" width="1" height="1" viewBox="0 0 1 1"/>`, "clip-path is unsupported"},
		{"bad aspect", `<svg width="1" height="1" viewBox="0 0 1 1" preserveAspectRatio="defer xMidYMid"/>`, "defer is unsupported"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := `<svg width="2" height="2" viewBox="0 0 2 2">` + test.nested + `</svg>`
			result, err := ImportNode(context.Background(), "nested.svg", []byte(source), generousImportLimits())
			if err == nil || result != nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ImportNode() = result %v, error %v; want %q", result, err, test.want)
			}
		})
	}
}

func TestImportNodeNestedSVGViewportLimits(t *testing.T) {
	source := []byte(`<svg width="2" height="2" viewBox="0 0 2 2"><svg width="1" height="1" viewBox="0 0 1 1"><rect width="1" height="1"/></svg></svg>`)
	limits := generousImportLimits()
	limits.MaxPathCommands = 4
	result, err := ImportNode(context.Background(), "nested.svg", source, limits)
	if err == nil || result != nil || !strings.Contains(err.Error(), "emitted path command count exceeds limit 4") {
		t.Fatalf("path-command limit = result %v, error %v", result, err)
	}
	limits = generousImportLimits()
	limits.MaxElements = 3
	result, err = ImportNode(context.Background(), "nested.svg", source, limits)
	if err == nil || result != nil || !strings.Contains(err.Error(), "emitted element count exceeds limit 3") {
		t.Fatalf("emitted-element limit = result %v, error %v", result, err)
	}
}
