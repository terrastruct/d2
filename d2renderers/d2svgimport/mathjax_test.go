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

func TestImportNodeFrozenMathJaxPathCorpus(t *testing.T) {
	formulas := []string{
		`a + b = c`,
		`\frac{x^2 + 1}{\sqrt{y}}`,
		`\sum_{i=0}^{n} i = \frac{n(n+1)}{2}`,
		`\begin{bmatrix}a & b \\ c & d\end{bmatrix}`,
		`\int_{-\infty}^{\infty} e^{-x^2}\,dx = \sqrt{\pi}`,
		`\Huge{\frac{\alpha g^2}{\omega^5} e^{[ -0.74\bigl\{\frac{\omega U_\omega 19.5}{g}\bigr\}^{\!-4}\,]}}`,
		`gibberish\; math:\sum_{i=0}^\infty i^2`,
		`\min_{ \mathclap{\substack{ x \in \mathbb{R}^n \\ x \geq 0 \\ Ax \leq b }}} c^T x`,
		`\lim_{h \rightarrow 0 } \frac{f(x+h)-f(x)}{h}`,
		`\begin{equation} \label{eq1}\begin{split}A & = \frac{\pi r^2}{2} \\ & = \frac{1}{2} \pi r^2\end{split}\end{equation}`,
		`\bra{a}\ket{b}`,
		`\cancel{Culture + 5}`,
		`\textcolor{red}{y} = \textcolor{green}{\sin} x`,
		`\lambda = 10.6\,\micro\mathrm{m}`,
		`\ce{SO4^2- + Ba^2+ -> BaSO4 v}`,
		`\var{F[g(x)]}\dd(\cos\theta)`,
		`\displaylines{x = a + b \\ y = b + c}\sum_{k=1}^{n} h_{k} \int_{0}^{1} \bigl(\partial_{k} f(x_{k-1}+t h_{k} e_{k}) -\partial_{k} f(a)\bigr) \,dt`,
	}
	for _, formula := range formulas {
		t.Run(formula, func(t *testing.T) {
			source, err := d2latex.Render(formula)
			if err != nil {
				t.Fatal(err)
			}
			width, height, err := d2latex.Measure(formula)
			if err != nil {
				t.Fatal(err)
			}
			result, err := ImportNode(context.Background(), "mathjax.svg", []byte(source), generousImportLimits())
			if err != nil {
				t.Fatalf("ImportNode frozen MathJax output: %v", err)
			}
			if result.Root == nil || result.Metrics.SourceBytes != len(source) || result.Metrics.ParsedPathCommands == 0 || result.Metrics.EmittedPathCommands == 0 {
				t.Fatalf("MathJax result/metrics = root %p, %+v", result.Root, result.Metrics)
			}
			if int(math.Ceil(result.Width)) != width || int(math.Ceil(result.Height)) != height {
				t.Fatalf("MathJax viewport %gx%g does not match measured %dx%d", result.Width, result.Height, width, height)
			}
		})
	}
}

func TestImportNodeFrozenMathJaxViewportAndPixels(t *testing.T) {
	source, err := d2latex.Render(`a + b = c`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := ImportNode(context.Background(), "mathjax.svg", []byte(source), generousImportLimits())
	if err != nil {
		t.Fatal(err)
	}
	if !nearFloat(result.Width, 71.44) || !nearFloat(result.Height, 14.048) || result.ViewBox != (d2scene.Box{X: 0, Y: -694, Width: 3947, Height: 776}) {
		t.Fatalf("simple MathJax viewport = %gx%g over %+v", result.Width, result.Height, result.ViewBox)
	}
	viewport := d2scene.NewNode(nil)
	viewport.Transform = result.ViewportTransform
	viewport.Children = []*d2scene.Node{result.Root}
	document := d2scene.NewDocument(d2scene.Box{Width: result.Width, Height: result.Height}, viewport)
	frame, err := d2raster.Render(context.Background(), document, d2raster.FrameOptions{
		Scale: 1, MaxWidth: 72, MaxHeight: 15, MaxPixels: 72 * 15,
		MaxNodes: 100, MaxDepth: 100, MaxPathCommands: 1000,
		MaxAnimationTracks: 1, MaxAnimationKeyframes: 1,
		MaxAssets: 1, MaxAssetBytes: 1, MaxDecodedAssetBytes: 1, MaxImportDepth: 10,
		MaxOffscreenBytes: 1 << 20, MaxEvenOddClipWork: 1 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	painted := 0
	for y := frame.Bounds().Min.Y; y < frame.Bounds().Max.Y; y++ {
		for x := frame.Bounds().Min.X; x < frame.Bounds().Max.X; x++ {
			if color.NRGBAModel.Convert(frame.At(x, y)).(color.NRGBA).A != 0 {
				painted++
			}
		}
	}
	if painted == 0 {
		t.Fatal("frozen MathJax scene rasterized to transparent pixels")
	}
}

func TestImportNodeRestrictsMathJaxRelativeLengths(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{"ex without signature", `<svg width="2ex" height="1" viewBox="0 0 2 1"/>`, "invalid width"},
		{"invalid baseline keyword", `<svg style="vertical-align:middle" width="2ex" height="1ex" viewBox="0 0 2 1"/>`, "invalid MathJax vertical-align"},
		{"invalid baseline unit", `<svg style="vertical-align:-1em" width="2ex" height="1ex" viewBox="0 0 2 1"/>`, "invalid MathJax vertical-align"},
		{"non-root baseline", `<svg width="2" height="1"><g style="vertical-align:-1ex"/></svg>`, "outside the root"},
		{"child ex geometry", `<svg style="vertical-align:-1ex" width="2ex" height="1ex" viewBox="0 0 2 1"><rect width="1ex" height="1"/></svg>`, "invalid width"},
		{"stylesheet baseline", `<svg width="2" height="1"><style>.x{vertical-align:-1ex}</style><g class="x"/></svg>`, `unsupported stylesheet property "vertical-align"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := ImportNode(context.Background(), "relative.svg", []byte(test.source), generousImportLimits())
			if err == nil || result != nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ImportNode() = result %v, error %v; want %q", result, err, test.want)
			}
		})
	}
	accepted := mustImport(t, `<svg style="vertical-align: -0.25ex;" width="2.5ex" height="1.25ex" viewBox="0 0 10 5"><path d="M0 0H10V5H0Z"/></svg>`)
	if !nearFloat(accepted.Width, 20) || !nearFloat(accepted.Height, 10) {
		t.Fatalf("bounded MathJax ex viewport = %gx%g, want 20x10", accepted.Width, accepted.Height)
	}
}
