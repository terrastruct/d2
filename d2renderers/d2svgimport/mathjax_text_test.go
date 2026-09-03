package d2svgimport

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"image/color"
	"math"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2latex"
	"github.com/d2lang/d2/d2renderers/d2raster"
	"github.com/d2lang/d2/d2renderers/d2scene"
)

const minimalFrozenMathJaxTextSVG = `<svg style="vertical-align: -0.1ex;" xmlns="http://www.w3.org/2000/svg" width="2ex" height="2ex" role="img" focusable="false" viewBox="0 -900 1000 1000"><g stroke="currentColor" fill="currentColor" stroke-width="0" transform="scale(1,-1)"><g data-mml-node="math"><g data-mml-node="mi" class=" MathML-Unit"><text data-variant="normal" transform="scale(1,-1)" font-size="884px" font-family="serif">µ</text></g></g></g></svg>`

func TestImportNodeFrozenMathJaxTextFallbackPixelsAndTopology(t *testing.T) {
	source, err := d2latex.Render(`\lambda = 10.6\,\micro\mathrm{m}`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := ImportNode(context.Background(), "mathjax-micro.svg", []byte(source), generousImportLimits())
	if err != nil {
		t.Fatal(err)
	}

	microPaths := mathJaxTextPaths(result.Root)
	if len(microPaths) != 1 {
		t.Fatalf("scale(1,-1) glyph paths = %d, want 1", len(microPaths))
	}
	if got := len(microPaths[0].Commands); got != 16 || got > result.Metrics.EmittedPathCommands {
		t.Fatalf("micro outline command count = %d, want pinned topology of 16 over metrics %+v", got, result.Metrics)
	}
	if !pathCommandsFinite(microPaths[0].Commands) {
		t.Fatal("micro outline contains non-finite coordinates")
	}
	if hasTextPrimitive(result.Root) {
		t.Fatal("MathJax platform-font fallback leaked a TextRun into the imported scene")
	}

	viewport := d2scene.NewNode(nil)
	viewport.Transform = result.ViewportTransform
	viewport.Children = []*d2scene.Node{result.Root}
	document := d2scene.NewDocument(d2scene.Box{Width: result.Width, Height: result.Height}, viewport)
	width, height := int(math.Ceil(result.Width)), int(math.Ceil(result.Height))
	frame, err := d2raster.Render(context.Background(), document, d2raster.FrameOptions{
		Scale: 1, MaxWidth: width, MaxHeight: height, MaxPixels: int64(width * height),
		MaxNodes: result.Metrics.EmittedElements + 2, MaxDepth: 100,
		MaxPathCommands:    10_000,
		MaxAnimationTracks: 1, MaxAnimationKeyframes: 1,
		MaxAssets: 1, MaxAssetBytes: 1, MaxDecodedAssetBytes: 1, MaxImportDepth: 10,
		MaxOffscreenBytes: 4 << 20, MaxEvenOddClipWork: 1 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(frame.Pix)
	if got, want := fmt.Sprintf("%x", digest), "8e540710580ddc8f8e28e99ea4dfa341278445bfb0e4c3aaea88decded5bc556"; got != want {
		t.Fatalf("frozen MathJax micro frame SHA-256 = %s, want %s", got, want)
	}
}

func TestImportNodeFrozenMathJaxTextCurrentColor(t *testing.T) {
	source, err := d2latex.Render(`\micro+\textcolor{red}{\micro}`)
	if err != nil {
		t.Fatal(err)
	}
	purple := color.NRGBA{R: 0x63, G: 0x2c, B: 0x9a, A: 0xff}
	result, err := ImportNodeWithOptions(context.Background(), "mathjax-color.svg", []byte(source), generousImportLimits(), ImportOptions{CurrentColor: &purple})
	if err != nil {
		t.Fatal(err)
	}
	paths := mathJaxTextPaths(result.Root)
	if len(paths) != 2 {
		t.Fatalf("micro glyph paths = %d, want 2", len(paths))
	}
	colors := make(map[color.NRGBA]int)
	for _, path := range paths {
		paint, ok := path.Fill.(d2scene.SolidPaint)
		if !ok {
			t.Fatalf("micro fill = %T, want solid", path.Fill)
		}
		colors[paint.Color]++
	}
	if colors[purple] != 1 || colors[(color.NRGBA{R: 0xff, A: 0xff})] != 1 || len(colors) != 2 {
		t.Fatalf("micro colors = %#v, want caller purple and explicit red", colors)
	}

	svg, err := ImportNode(context.Background(), "mathjax-color.svg", []byte(source), generousImportLimits())
	if err != nil {
		t.Fatal(err)
	}
	svgColors := make(map[color.NRGBA]int)
	for _, path := range mathJaxTextPaths(svg.Root) {
		svgColors[path.Fill.(d2scene.SolidPaint).Color]++
	}
	if svgColors[(color.NRGBA{A: 0xff})] != 1 || svgColors[(color.NRGBA{R: 0xff, A: 0xff})] != 1 {
		t.Fatalf("svg ImportNode colors = %#v, want initial black and explicit red", svgColors)
	}
}

func TestImportNodeFrozenMathJaxTextStrictSubset(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{"outside MathJax", `<svg width="1" height="1"><text data-variant="normal" transform="scale(1,-1)" font-size="884px" font-family="serif">µ</text></svg>`, "verified frozen MathJax root"},
		{"empty namespace", strings.Replace(minimalFrozenMathJaxTextSVG, ` xmlns="http://www.w3.org/2000/svg"`, "", 1), "verified frozen MathJax root"},
		{"missing role", strings.Replace(minimalFrozenMathJaxTextSVG, ` role="img"`, "", 1), "verified frozen MathJax root"},
		{"wrong root shape", strings.Replace(minimalFrozenMathJaxTextSVG, ` viewBox="0 -900 1000 1000"`, ` preserveAspectRatio="none" viewBox="0 -900 1000 1000"`, 1), "verified frozen MathJax root"},
		{"wrong wrapper fill", strings.Replace(minimalFrozenMathJaxTextSVG, `fill="currentColor"`, `fill="black"`, 1), "verified frozen MathJax root"},
		{"wrong math node", strings.Replace(minimalFrozenMathJaxTextSVG, `data-mml-node="math"`, `data-mml-node="mrow"`, 1), "verified frozen MathJax root"},
		{"wrong unit node", strings.Replace(minimalFrozenMathJaxTextSVG, `data-mml-node="mi"`, `data-mml-node="mo"`, 1), "sole child"},
		{"wrong unit class", strings.Replace(minimalFrozenMathJaxTextSVG, `class=" MathML-Unit"`, `class="unit"`, 1), "sole child"},
		{"Greek mu", strings.Replace(minimalFrozenMathJaxTextSVG, `>µ</text>`, `>μ</text>`, 1), "U+00B5"},
		{"two characters", strings.Replace(minimalFrozenMathJaxTextSVG, `>µ</text>`, `>µµ</text>`, 1), "one U+00B5"},
		{"wrong variant", strings.Replace(minimalFrozenMathJaxTextSVG, `data-variant="normal"`, `data-variant="italic"`, 1), "data-variant"},
		{"wrong transform", strings.Replace(minimalFrozenMathJaxTextSVG, `transform="scale(1,-1)" font-size="884px"`, `transform="translate(1)" font-size="884px"`, 1), "must use scale"},
		{"wrong size", strings.Replace(minimalFrozenMathJaxTextSVG, `font-size="884px"`, `font-size="16px"`, 1), "must use scale"},
		{"wrong family", strings.Replace(minimalFrozenMathJaxTextSVG, `font-family="serif"`, `font-family="sans-serif"`, 1), "must use scale"},
		{"position attribute", strings.Replace(minimalFrozenMathJaxTextSVG, `<text `, `<text x="0" `, 1), "unsupported attribute"},
		{"paint attribute", strings.Replace(minimalFrozenMathJaxTextSVG, `<text `, `<text fill="red" `, 1), "unsupported layout"},
		{"nested child", strings.Replace(minimalFrozenMathJaxTextSVG, `>µ</text>`, `><path d="M0 0"/></text>`, 1), "cannot contain child"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := ImportNode(context.Background(), "mathjax-adversarial.svg", []byte(test.source), generousImportLimits())
			if err == nil || result != nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ImportNode() = %#v, %v; want %q", result, err, test.want)
			}
		})
	}
}

func TestImportNodeFrozenMathJaxTextPathLimitAndCancellation(t *testing.T) {
	data := []byte(minimalFrozenMathJaxTextSVG)
	result, err := ImportNode(context.Background(), "mathjax-limit.svg", data, generousImportLimits())
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.ParsedPathCommands != 0 || result.Metrics.EmittedPathCommands <= 0 {
		t.Fatalf("text fallback metrics = %+v", result.Metrics)
	}
	limits := generousImportLimits()
	limits.MaxPathCommands = result.Metrics.EmittedPathCommands
	if exact, err := ImportNode(context.Background(), "mathjax-limit.svg", data, limits); err != nil || exact == nil {
		t.Fatalf("exact path limit = %#v, %v", exact, err)
	}
	limits.MaxPathCommands--
	if over, err := ImportNode(context.Background(), "mathjax-limit.svg", data, limits); err == nil || over != nil || !strings.Contains(err.Error(), "path command count") {
		t.Fatalf("limit+1 = %#v, %v", over, err)
	}
	for _, test := range []struct {
		name  string
		exact int
		set   func(*Limits, int)
		want  string
	}{
		{"bytes", len(data), func(limits *Limits, value int) { limits.MaxBytes = value }, "bytes"},
		{"elements", result.Metrics.ParsedElements, func(limits *Limits, value int) { limits.MaxElements = value }, "element count"},
	} {
		t.Run(test.name, func(t *testing.T) {
			exactLimits := generousImportLimits()
			test.set(&exactLimits, test.exact)
			if exact, err := ImportNode(context.Background(), "mathjax-limit.svg", data, exactLimits); err != nil || exact == nil {
				t.Fatalf("exact limit = %#v, %v", exact, err)
			}
			test.set(&exactLimits, test.exact-1)
			if over, err := ImportNode(context.Background(), "mathjax-limit.svg", data, exactLimits); err == nil || over != nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("limit+1 = %#v, %v; want %q", over, err, test.want)
			}
		})
	}

	for remaining := int64(1); remaining < 512; remaining++ {
		ctx := &cancelAfterContext{remaining: remaining}
		got, err := ImportNode(ctx, "mathjax-cancel.svg", data, generousImportLimits())
		if errors.Is(err, context.Canceled) {
			if got != nil {
				t.Fatalf("canceled import returned partial result at checkpoint %d", remaining)
			}
			continue
		}
		if err != nil {
			t.Fatalf("checkpoint %d returned %v", remaining, err)
		}
		if got == nil {
			t.Fatalf("checkpoint %d returned nil success", remaining)
		}
		return
	}
	t.Fatal("frozen MathJax text import never reached success across cancellation checkpoints")
}

func mathJaxTextPaths(root *d2scene.Node) []d2scene.Path {
	var paths []d2scene.Path
	stack := []*d2scene.Node{root}
	wantTransform := d2scene.Scale(1, -1)
	for len(stack) != 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if node == nil {
			continue
		}
		if path, ok := node.Primitive.(d2scene.Path); ok && node.Transform == wantTransform {
			paths = append(paths, path)
		}
		stack = append(stack, node.Children...)
	}
	return paths
}

func hasTextPrimitive(root *d2scene.Node) bool {
	stack := []*d2scene.Node{root}
	for len(stack) != 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if node == nil {
			continue
		}
		switch node.Primitive.(type) {
		case d2scene.TextRun, *d2scene.TextRun:
			return true
		}
		stack = append(stack, node.Children...)
	}
	return false
}

func pathCommandsFinite(commands []d2scene.PathCommand) bool {
	for _, command := range commands {
		values := []float64{
			command.P1.X, command.P1.Y, command.P2.X, command.P2.Y, command.P3.X, command.P3.Y,
			command.RadiusX, command.RadiusY, command.Rotation,
		}
		for _, value := range values {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return false
			}
		}
	}
	return true
}

func TestFrozenMathJaxMicroOutlineTopologyAndDigest(t *testing.T) {
	wantKinds := [...]d2scene.PathCommandKind{
		d2scene.MoveCommand,
		d2scene.LineCommand,
		d2scene.LineCommand,
		d2scene.CubicCommand,
		d2scene.CubicCommand,
		d2scene.LineCommand,
		d2scene.LineCommand,
		d2scene.LineCommand,
		d2scene.CubicCommand,
		d2scene.LineCommand,
		d2scene.CubicCommand,
		d2scene.CubicCommand,
		d2scene.CubicCommand,
		d2scene.LineCommand,
		d2scene.LineCommand,
		d2scene.CloseCommand,
	}
	if len(frozenMathJaxMicroOutline) != len(wantKinds) {
		t.Fatalf("micro outline command count = %d, want %d", len(frozenMathJaxMicroOutline), len(wantKinds))
	}
	for index, want := range wantKinds {
		if got := frozenMathJaxMicroOutline[index].Kind; got != want {
			t.Fatalf("micro outline command %d kind = %d, want %d", index, got, want)
		}
	}
	if got, want := pathCommandDigest(frozenMathJaxMicroOutline[:]), "3b7104c5e83e5d8821d1e9a2e93a5856769c76a8f5075d7d6d656072518df651"; got != want {
		t.Fatalf("micro outline SHA-256 = %s, want %s", got, want)
	}
}

func pathCommandDigest(commands []d2scene.PathCommand) string {
	digest := sha256.New()
	for _, command := range commands {
		fmt.Fprintf(digest, "%d/%016x/%016x/%016x/%016x/%016x/%016x/%016x/%016x/%016x/%t/%t;",
			command.Kind,
			math.Float64bits(command.P1.X), math.Float64bits(command.P1.Y),
			math.Float64bits(command.P2.X), math.Float64bits(command.P2.Y),
			math.Float64bits(command.P3.X), math.Float64bits(command.P3.Y),
			math.Float64bits(command.RadiusX), math.Float64bits(command.RadiusY), math.Float64bits(command.Rotation),
			command.LargeArc, command.Sweep,
		)
	}
	return fmt.Sprintf("%x", digest.Sum(nil))
}
