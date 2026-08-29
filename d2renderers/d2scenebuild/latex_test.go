package d2scenebuild

import (
	"context"
	"image/color"
	"math"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2latex"
	"github.com/d2lang/d2/d2renderers/d2raster"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2target"
)

func TestBuildLatexShapeAndConnectionLabels(t *testing.T) {
	formula := `a + b = c`
	labelWidth, labelHeight, err := d2latex.Measure(formula)
	if err != nil {
		t.Fatal(err)
	}
	diagram := validDiagram()
	shape := &diagram.Shapes[0]
	shape.Text = d2target.Text{
		Label: formula, Language: "latex",
		LabelWidth: labelWidth, LabelHeight: labelHeight,
		LabelFill: "#ff0000", Underline: true, Bold: true,
	}
	shape.LabelPosition = "INSIDE_MIDDLE_CENTER"
	shape.Stroke = "#000000"
	connection := &diagram.Connections[0]
	connection.Text = d2target.Text{
		Label: formula, Language: "latex",
		LabelWidth: labelWidth, LabelHeight: labelHeight, FontSize: 16,
		LabelFill: "#ff0000", Underline: true,
	}
	connection.LabelPosition = "INSIDE_MIDDLE_CENTER"
	connection.LabelPercentage = .5
	connection.Color = "#000000"
	connection.Fill = "#00ff00"

	document, err := Build(context.Background(), diagram, Options{Assets: testAssetOptions(t)})
	if err != nil {
		t.Fatal(err)
	}
	shapeLabel := findSceneNode(t, document.Root, "a:label:0")
	shapeImage, ok := shapeLabel.Primitive.(d2scene.Image)
	if !ok {
		t.Fatalf("shape LaTeX label = %T, want Image", shapeLabel.Primitive)
	}
	shapeTopLeft := latexShapeLabelTopLeft(*shape)
	if !nearSceneValue(shapeImage.Box.X, shapeTopLeft.X) || !nearSceneValue(shapeImage.Box.Y, shapeTopLeft.Y) ||
		!nearSceneValue(shapeImage.Box.Width, 71.44) || !nearSceneValue(shapeImage.Box.Height, 14.048) {
		t.Fatalf("shape LaTeX image = %+v, top-left %+v", shapeImage, shapeTopLeft)
	}
	connectionLabel := findSceneNode(t, document.Root, "a-b:label:0")
	connectionImage, ok := connectionLabel.Primitive.(d2scene.Image)
	if !ok {
		t.Fatalf("connection LaTeX label = %T, want Image", connectionLabel.Primitive)
	}
	connectionTopLeft := connection.GetLabelTopLeft()
	if !nearSceneValue(connectionImage.Box.X, math.Round(connectionTopLeft.X)) ||
		!nearSceneValue(connectionImage.Box.Y, math.Round(connectionTopLeft.Y)) {
		t.Fatalf("connection LaTeX image = %+v, top-left %+v", connectionImage, connectionTopLeft)
	}
	if shapeImage.Asset != connectionImage.Asset {
		t.Fatalf("identical formula/currentColor was not deduplicated: %q != %q", shapeImage.Asset, connectionImage.Asset)
	}
	asset, ok := document.Assets[shapeImage.Asset].(d2scene.VectorAsset)
	if !ok || asset.ViewBox != (d2scene.Box{Width: 71.44, Height: 14.048}) || asset.Root == nil || asset.Root.Clip == nil || len(asset.Root.Children) != 1 {
		t.Fatalf("LaTeX vector asset = %#v", document.Assets[shapeImage.Asset])
	}
	if findOptionalSceneNode(document.Root, "a:label-fill") != nil || findOptionalSceneNode(document.Root, "a-b:label-fill") != nil {
		t.Fatal("LaTeX branch incorrectly emitted ordinary label-fill visuals")
	}
}

func TestBuildLatexUsesIntrinsicViewportAndPlacement(t *testing.T) {
	formula := `\frac{x^2 + 1}{\sqrt{y}}`
	width, height, err := d2latex.Measure(formula)
	if err != nil {
		t.Fatal(err)
	}
	diagram := d2target.NewDiagram()
	diagram.Shapes = []d2target.Shape{{
		ID: "formula", Type: d2target.ShapeRectangle,
		Pos: d2target.Point{X: 20, Y: 30}, Width: 160, Height: 100,
		Fill: "#fff", Stroke: "#000", StrokeWidth: 2, Opacity: 1,
		ThreeDee: true,
		Text: d2target.Text{
			Label: formula, Language: "latex", LabelWidth: width, LabelHeight: height,
		},
		LabelPosition: "OUTSIDE_TOP_CENTER",
	}}
	document, err := Build(context.Background(), diagram, Options{Assets: testAssetOptions(t)})
	if err != nil {
		t.Fatal(err)
	}
	image := findSceneNode(t, document.Root, "formula:label:0").Primitive.(d2scene.Image)
	wantTopLeft := latexShapeLabelTopLeft(diagram.Shapes[0])
	if image.Box.X != wantTopLeft.X || image.Box.Y != wantTopLeft.Y {
		t.Fatalf("LaTeX placement = %+v, want origin %+v", image.Box, wantTopLeft)
	}
	asset := document.Assets[image.Asset].(d2scene.VectorAsset)
	if image.Box.Width != asset.ViewBox.Width || image.Box.Height != asset.ViewBox.Height ||
		int(math.Ceil(image.Box.Width)) != width || int(math.Ceil(image.Box.Height)) != height {
		t.Fatalf("intrinsic viewport image=%+v asset=%+v measured=%dx%d", image.Box, asset.ViewBox, width, height)
	}
}

func TestBuildLatexResolvesCurrentColorWithoutOverridingExplicitColor(t *testing.T) {
	formula := `\textcolor{red}{y} = x`
	width, height, err := d2latex.Measure(formula)
	if err != nil {
		t.Fatal(err)
	}
	makeShape := func(id string, x int, stroke string) d2target.Shape {
		return d2target.Shape{
			ID: id, Type: d2target.ShapeText, Pos: d2target.Point{X: x}, Width: width, Height: height,
			Opacity: 1, Fill: "none", Stroke: stroke,
			Text:          d2target.Text{Label: formula, Language: "latex", LabelWidth: width, LabelHeight: height},
			LabelPosition: "INSIDE_MIDDLE_CENTER",
		}
	}
	diagram := d2target.NewDiagram()
	diagram.Shapes = []d2target.Shape{
		makeShape("first", 0, "#112233"),
		makeShape("second", width+20, "#445566"),
	}
	document, err := Build(context.Background(), diagram, Options{Assets: testAssetOptions(t)})
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Assets) != 2 {
		t.Fatalf("color-specialized LaTeX assets = %d, want 2", len(document.Assets))
	}
	for _, test := range []struct {
		id      string
		current color.NRGBA
	}{
		{"first", color.NRGBA{R: 0x11, G: 0x22, B: 0x33, A: 0xff}},
		{"second", color.NRGBA{R: 0x44, G: 0x55, B: 0x66, A: 0xff}},
	} {
		image := findSceneNode(t, document.Root, test.id+":label:0").Primitive.(d2scene.Image)
		asset := document.Assets[image.Asset].(d2scene.VectorAsset)
		colors := vectorSolidColors(asset.Root)
		if !colors[test.current] || !colors[(color.NRGBA{R: 0xff, A: 0xff})] {
			t.Fatalf("%s LaTeX colors = %+v; want current %+v and explicit red", test.id, colors, test.current)
		}
	}
}

func TestBuildLatexInputAndImportBudgets(t *testing.T) {
	makeDiagram := func(formula string) *d2target.Diagram {
		diagram := d2target.NewDiagram()
		diagram.Shapes = []d2target.Shape{{
			ID: "formula", Type: d2target.ShapeText, Width: 100, Height: 20,
			Opacity: 1, Fill: "none", Stroke: "#000",
			Text:          d2target.Text{Label: formula, Language: "latex", LabelWidth: 100, LabelHeight: 20},
			LabelPosition: "INSIDE_MIDDLE_CENTER",
		}}
		return diagram
	}
	_, err := Build(context.Background(), makeDiagram("x"), Options{})
	if err == nil || !strings.Contains(err.Error(), "label language latex without configured SVG import limits") {
		t.Fatalf("missing import options error = %v", err)
	}
	_, err = Build(context.Background(), makeDiagram(strings.Repeat("x", maxLatexInputBytes+1)), Options{Assets: testAssetOptions(t)})
	if err == nil || !strings.Contains(err.Error(), "latex input is 4097 bytes, exceeding limit 4096") {
		t.Fatalf("input limit error = %v", err)
	}
	options := testAssetOptions(t)
	options.SVGImportBudget.MaxSourceBytes = 1
	_, err = Build(context.Background(), makeDiagram("x"), Options{Assets: options})
	if err == nil || !strings.Contains(err.Error(), "exceeding limit 1") {
		t.Fatalf("shared SVG import budget error = %v", err)
	}
}

func TestBuildLatexConnectionMask(t *testing.T) {
	formula := `x^2`
	width, height, err := d2latex.Measure(formula)
	if err != nil {
		t.Fatal(err)
	}
	diagram := validDiagram()
	diagram.Connections[0].Text = d2target.Text{
		Label: formula, Language: "latex", LabelWidth: width, LabelHeight: height, FontSize: 16,
	}
	diagram.Connections[0].LabelPosition = "INSIDE_MIDDLE_CENTER"
	diagram.Connections[0].LabelPercentage = .5
	document, err := Build(context.Background(), diagram, Options{Assets: testAssetOptions(t)})
	if err != nil {
		t.Fatal(err)
	}
	geometry := findSceneNode(t, document.Root, "a-b:geometry")
	if geometry.Mask == nil || findOptionalSceneNode(geometry.Mask.Root, "a-b:label-mask-hole") == nil {
		t.Fatal("LaTeX connection label did not preserve the diagram-wide route mask")
	}
	labelNode := findSceneNode(t, document.Root, "a-b:label:0")
	image := labelNode.Primitive.(d2scene.Image)
	if _, ok := document.Assets[image.Asset].(d2scene.VectorAsset); !ok {
		t.Fatalf("LaTeX image asset = %T", document.Assets[image.Asset])
	}
}

func TestBuildLatexLabelRendersPixels(t *testing.T) {
	formula := `x^2 + y^2`
	width, height, err := d2latex.Measure(formula)
	if err != nil {
		t.Fatal(err)
	}
	diagram := d2target.NewDiagram()
	diagram.Shapes = []d2target.Shape{{
		ID: "formula", Type: d2target.ShapeText,
		Width: width, Height: height, Opacity: 1,
		Fill: "none", Stroke: "#0055cc",
		Text: d2target.Text{
			Label: formula, Language: "latex", LabelWidth: width, LabelHeight: height,
		},
		LabelPosition: "INSIDE_MIDDLE_CENTER",
	}}
	pad := int64(2)
	document, err := Build(context.Background(), diagram, Options{Pad: &pad, Assets: testAssetOptions(t)})
	if err != nil {
		t.Fatal(err)
	}
	frame, err := d2raster.Render(context.Background(), document, d2raster.FrameOptions{
		Scale:    4,
		MaxWidth: 1_000, MaxHeight: 1_000, MaxPixels: 1_000_000,
		MaxNodes: 10_000, MaxDepth: 100, MaxPathCommands: 1_000_000,
		MaxAnimationTracks: 10_000, MaxAnimationKeyframes: 100_000,
		MaxAssets: 100, MaxAssetBytes: 64 * 1024 * 1024,
		MaxDecodedAssetBytes: 64 * 1024 * 1024, MaxImportDepth: 100,
		MaxOffscreenBytes: 64 * 1024 * 1024, MaxEvenOddClipWork: 1_000_000_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	painted, inheritedBlue := 0, 0
	for y := frame.Bounds().Min.Y; y < frame.Bounds().Max.Y; y++ {
		for x := frame.Bounds().Min.X; x < frame.Bounds().Max.X; x++ {
			pixel := frame.NRGBAAt(x, y)
			if pixel.A == 0 {
				continue
			}
			painted++
			if pixel.B > pixel.G && pixel.G > pixel.R {
				inheritedBlue++
			}
		}
	}
	if painted == 0 || inheritedBlue == 0 {
		t.Fatalf("raster LaTeX frame has painted=%d inherited-blue=%d pixels in %v", painted, inheritedBlue, frame.Bounds())
	}
}

func nearSceneValue(left, right float64) bool {
	return math.Abs(left-right) < 1e-9
}

func vectorSolidColors(root *d2scene.Node) map[color.NRGBA]bool {
	colors := make(map[color.NRGBA]bool)
	stack := []*d2scene.Node{root}
	for len(stack) != 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if node == nil {
			continue
		}
		if path, ok := node.Primitive.(d2scene.Path); ok {
			if fill, ok := path.Fill.(d2scene.SolidPaint); ok {
				colors[fill.Color] = true
			}
		}
		stack = append(stack, node.Children...)
	}
	return colors
}
