package d2scenebuild

import (
	"bytes"
	"context"
	"errors"
	"image/color"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2raster"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/internal/patternassets"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/d2themes/d2themescatalog"
)

func TestSketchOrdinaryShapeFamiliesUseTypedRoughPathsDeterministically(t *testing.T) {
	t.Parallel()

	shapeTypes := []string{
		"", d2target.ShapeRectangle, d2target.ShapeSquare,
		d2target.ShapeOval, d2target.ShapeCircle,
		d2target.ShapePage, d2target.ShapeParallelogram, d2target.ShapeDocument,
		d2target.ShapeCylinder, d2target.ShapeQueue, d2target.ShapePackage,
		d2target.ShapeStep, d2target.ShapeCallout, d2target.ShapeStoredData,
		d2target.ShapePerson, d2target.ShapeC4Person, d2target.ShapeDiamond,
		d2target.ShapeHexagon, d2target.ShapeCloud,
		d2target.ShapeSequenceDiagram, d2target.ShapeHierarchy,
	}
	for _, shapeType := range shapeTypes {
		name := shapeType
		if name == "" {
			name = "empty rectangle alias"
		}
		t.Run(name, func(t *testing.T) {
			target := sketchTestShape("shape", shapeType, 10, 20, 140, 90)
			firstBuilder := newSketchShapeTestBuilder(context.Background(), d2target.NewDiagram(), largeSketchTestBudget())
			first, err := firstBuilder.buildShape(target)
			if err != nil {
				t.Fatalf("buildShape() error = %v", err)
			}
			secondBuilder := newSketchShapeTestBuilder(context.Background(), d2target.NewDiagram(), largeSketchTestBudget())
			second, err := secondBuilder.buildShape(target)
			if err != nil {
				t.Fatalf("second buildShape() error = %v", err)
			}
			if !reflect.DeepEqual(first, second) {
				t.Fatal("seeded sketch shape is not deterministic")
			}
			paths, streaks := 0, 0
			walkSketchTestNodes(first, func(node *d2scene.Node) {
				if _, ok := node.Primitive.(d2scene.Path); ok {
					paths++
				}
				if hasSketchTestClass(node, "sketch-streak-overlay") {
					streaks++
				}
			})
			if paths == 0 || streaks == 0 {
				t.Fatalf("rough paths/streaks = %d/%d, want both non-zero", paths, streaks)
			}
			if firstBuilder.sketchOperationSets == 0 || firstBuilder.sketchOperations == 0 || firstBuilder.sketchPathCommands <= sketchStreakCommandCount {
				t.Fatalf("sketch accounting = sets %d ops %d commands %d", firstBuilder.sketchOperationSets, firstBuilder.sketchOperations, firstBuilder.sketchPathCommands)
			}
		})
	}
}

func TestSketchStructuredShapesKeepTextAndRoughenGeometry(t *testing.T) {
	t.Parallel()

	class := structuredTestShape("class", d2target.ShapeClass, 10, 20, 220, 140)
	class.Label, class.LabelWidth, class.LabelHeight = "Person", 60, 20
	class.FillPattern = "dots"
	class.Fields = []d2target.ClassField{{Name: "name", Type: "string", Visibility: "private"}}
	class.Methods = []d2target.ClassMethod{{Name: "save()", Return: "error", Visibility: "public"}}
	table := structuredTestShape("table", d2target.ShapeSQLTable, 260, 20, 220, 140)
	table.Label, table.LabelWidth, table.LabelHeight = "users", 50, 20
	table.FillPattern = "dots"
	table.Columns = []d2target.SQLColumn{{
		Name:       d2target.Text{Label: "id", LabelWidth: 20},
		Type:       d2target.Text{Label: "uuid", LabelWidth: 35},
		Constraint: []string{"primary_key"},
	}}

	diagram := d2target.NewDiagram()
	b := newSketchShapeTestBuilder(context.Background(), diagram, largeSketchTestBudget())
	classNode, err := b.buildShape(class)
	if err != nil {
		t.Fatalf("class buildShape() error = %v", err)
	}
	tableNode, err := b.buildShape(table)
	if err != nil {
		t.Fatalf("table buildShape() error = %v", err)
	}
	for _, test := range []struct {
		name     string
		node     *d2scene.Node
		wantText int
	}{
		{name: "class", node: classNode, wantText: 7},
		{name: "table", node: tableNode, wantText: 4},
	} {
		paths, texts, streaks, builtinPatterns := 0, 0, 0, 0
		walkSketchTestNodes(test.node, func(node *d2scene.Node) {
			switch node.Primitive.(type) {
			case d2scene.Path:
				paths++
			case d2scene.TextRun:
				texts++
			}
			if hasSketchTestClass(node, "sketch-streak-overlay") {
				streaks++
			}
			if hasSketchTestClass(node, "dots-overlay") {
				builtinPatterns++
			}
		})
		if paths < 4 || texts != test.wantText || streaks != 1 {
			t.Fatalf("%s paths/texts/streaks = %d/%d/%d", test.name, paths, texts, streaks)
		}
		if builtinPatterns == 0 {
			t.Fatalf("%s sketch lost the configured fill-pattern overlays", test.name)
		}
		separatorPattern := false
		walkSketchTestNodes(test.node, func(node *d2scene.Node) {
			separatorPattern = separatorPattern ||
				(strings.Contains(node.ID, "separator") && strings.HasSuffix(node.ID, ":fill-pattern"))
		})
		if !separatorPattern {
			t.Fatalf("%s sketch separator lost its svg fill-pattern overlay", test.name)
		}
	}
}

func TestSketchShapeEffectsAndCodeImageSeparation(t *testing.T) {
	t.Parallel()

	b := newSketchShapeTestBuilder(context.Background(), d2target.NewDiagram(), largeSketchTestBudget())
	tests := []struct {
		name        string
		shape       d2target.Shape
		wantRough   bool
		wantStreak  bool
		wantPrecise bool
	}{
		{name: "3d", shape: func() d2target.Shape {
			s := sketchTestShape("three", d2target.ShapeRectangle, 10, 20, 120, 80)
			s.ThreeDee = true
			return s
		}(), wantPrecise: true},
		{name: "multiple", shape: func() d2target.Shape {
			s := sketchTestShape("multiple", d2target.ShapeDiamond, 10, 20, 120, 80)
			s.Multiple = true
			return s
		}(), wantRough: true, wantStreak: true, wantPrecise: true},
		{name: "double border", shape: func() d2target.Shape {
			s := sketchTestShape("double", d2target.ShapeOval, 10, 20, 120, 80)
			s.DoubleBorder = true
			return s
		}(), wantRough: true, wantStreak: true},
		{name: "code", shape: func() d2target.Shape {
			s := sketchTestShape("code", d2target.ShapeCode, 10, 20, 160, 90)
			s.Label, s.Language, s.FontSize = "package main", "go", 16
			return s
		}(), wantPrecise: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			node, err := b.buildShape(test.shape)
			if err != nil {
				t.Fatalf("buildShape() error = %v", err)
			}
			roughPaths, streaks, precise := 0, 0, 0
			walkSketchTestNodes(node, func(child *d2scene.Node) {
				isRough := strings.Contains(child.ID, ":sketch:set:") || strings.Contains(child.ID, ":sketch:path:")
				isStreak := hasSketchTestClass(child, "sketch-streak-overlay")
				if isRough {
					roughPaths++
				}
				if isStreak {
					streaks++
				}
				switch child.Primitive.(type) {
				case d2scene.Rect, d2scene.Ellipse, d2scene.Path, d2scene.Image:
					if !isRough && !isStreak {
						precise++
					}
				}
			})
			if (roughPaths != 0) != test.wantRough || (streaks != 0) != test.wantStreak || (precise != 0) != test.wantPrecise {
				t.Fatalf("rough/streak/precise = %d/%d/%d", roughPaths, streaks, precise)
			}
		})
	}

	imageURL, err := url.Parse("https://example.invalid/image.png")
	if err != nil {
		t.Fatal(err)
	}
	imageShape := sketchTestShape("image", d2target.ShapeImage, 10, 20, 120, 80)
	imageShape.Icon = imageURL
	assetID := d2scene.AssetID("test:image")
	b.sourceAssetIDs[imageURL.String()] = assetID
	b.assets[assetID] = d2scene.RasterAsset{MIMEType: "image/png", Data: []byte{1}, PixelWidth: 1, PixelHeight: 1, DecodedBytes: 4}
	imageNode, err := b.buildShape(imageShape)
	if err != nil {
		t.Fatalf("image buildShape() error = %v", err)
	}
	if len(imageNode.Children) != 1 {
		t.Fatalf("image children = %d, want one precise image", len(imageNode.Children))
	}
	if _, ok := imageNode.Children[0].Primitive.(d2scene.Image); !ok {
		t.Fatalf("image primitive = %T", imageNode.Children[0].Primitive)
	}
}

func TestSketchStreakPatternsChargeOncePerLuminanceCategory(t *testing.T) {
	t.Parallel()

	b := newSketchShapeTestBuilder(context.Background(), d2target.NewDiagram(), largeSketchTestBudget())
	target := sketchTestShape("shape", d2target.ShapeRectangle, 0, 0, 20, 20)
	primitive := []d2scene.Primitive{d2scene.Rect{Box: d2scene.Box{Width: 20, Height: 20}}}
	first, err := b.buildSketchStreakOverlays(target, "#ffffff", "first", primitive)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || first[0].Blend != d2scene.BlendDarken || b.sketchPathCommands != sketchStreakCommandCount {
		t.Fatalf("first streak = nodes %d blend %d commands %d", len(first), first[0].Blend, b.sketchPathCommands)
	}
	if _, err := b.buildSketchStreakOverlays(target, "#eeeeee", "same", primitive); err != nil {
		t.Fatal(err)
	}
	if b.sketchPathCommands != sketchStreakCommandCount {
		t.Fatalf("same-category cached commands = %d", b.sketchPathCommands)
	}
	second, err := b.buildSketchStreakOverlays(target, "#aaaaaa", "second", primitive)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 1 || second[0].Blend != d2scene.BlendColorBurn || b.sketchPathCommands != 2*sketchStreakCommandCount {
		t.Fatalf("second category = nodes %d blend %d commands %d", len(second), second[0].Blend, b.sketchPathCommands)
	}

	pathBuilder := newSketchShapeTestBuilder(context.Background(), d2target.NewDiagram(), largeSketchTestBudget())
	pathPrimitive := d2scene.Path{Commands: []d2scene.PathCommand{d2scene.MoveTo(0, 0), d2scene.LineTo(20, 20)}}
	if _, err := pathBuilder.buildSketchStreakOverlays(target, "#ffffff", "path", []d2scene.Primitive{pathPrimitive}); err != nil {
		t.Fatal(err)
	}
	if got, want := pathBuilder.sketchPathCommands, sketchStreakCommandCount+len(pathPrimitive.Commands); got != want {
		t.Fatalf("streak path overlay command charge = %d, want %d", got, want)
	}

	patternBuilder := newSketchShapeTestBuilder(context.Background(), d2target.NewDiagram(), largeSketchTestBudget())
	patternTarget := target
	patternTarget.FillPattern = "dots"
	base := d2scene.NewNode(pathPrimitive)
	if _, err := patternBuilder.appendSketchBuiltinPattern(patternTarget, []*d2scene.Node{base}, "pattern"); err != nil {
		t.Fatal(err)
	}
	if got, want := patternBuilder.sketchPathCommands, len(pathPrimitive.Commands); got != want {
		t.Fatalf("fill-pattern path overlay command charge = %d, want %d", got, want)
	}
}

func TestSketchStreakStylesUseMultiplyBlend(t *testing.T) {
	t.Parallel()

	tests := []struct {
		category string
		blend    d2scene.BlendMode
		ink      color.NRGBA
	}{
		{category: "bright", blend: d2scene.BlendDarken, ink: color.NRGBA{A: 26}},
		{category: "normal", blend: d2scene.BlendColorBurn, ink: color.NRGBA{A: 41}},
		{category: "dark", blend: d2scene.BlendOverlay, ink: color.NRGBA{A: 82}},
		{category: "darker", blend: d2scene.BlendLighten, ink: color.NRGBA{R: 255, G: 255, B: 255, A: 61}},
	}
	for _, test := range tests {
		t.Run(test.category, func(t *testing.T) {
			blend, ink, err := sketchStreakStyle(test.category)
			if err != nil {
				t.Fatal(err)
			}
			if blend != test.blend || ink != test.ink {
				t.Fatalf("style = blend %d ink %#v, want blend %d ink %#v", blend, ink, test.blend, test.ink)
			}
		})
	}
}

func TestSketchShapeBudgetsCancellationAndTopLevelIntegration(t *testing.T) {
	t.Parallel()

	target := sketchTestShape("shape", d2target.ShapeRectangle, 0, 0, 120, 80)
	tooSmall := largeSketchTestBudget()
	tooSmall.MaxOperations = 1
	b := newSketchShapeTestBuilder(context.Background(), d2target.NewDiagram(), tooSmall)
	if _, err := b.buildShape(target); err == nil || !strings.Contains(err.Error(), "operation count exceeds limit") {
		t.Fatalf("small operation budget error = %v", err)
	}

	streakSmall := largeSketchTestBudget()
	streakSmall.MaxPathCommands = sketchStreakCommandCount - 1
	b = newSketchShapeTestBuilder(context.Background(), d2target.NewDiagram(), streakSmall)
	_, err := b.buildSketchStreakOverlays(target, "#ffffff", "shape", []d2scene.Primitive{d2scene.Rect{Box: d2scene.Box{Width: 20, Height: 20}}})
	if err == nil || !strings.Contains(err.Error(), "sketch path command count exceeds limit") {
		t.Fatalf("small streak budget error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	b = newSketchShapeTestBuilder(ctx, d2target.NewDiagram(), largeSketchTestBudget())
	if _, err := b.buildShape(target); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled buildShape() error = %v", err)
	}
	if _, err := parseSketchStreakSource(ctx, patternassets.StreakPathData()); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled streak parse error = %v", err)
	}

	diagram := d2target.NewDiagram()
	diagram.Shapes = []d2target.Shape{target}
	document, err := Build(context.Background(), diagram, Options{Sketch: true, SketchBudget: largeSketchTestBudget()})
	if err != nil {
		t.Fatalf("top-level sketch shape Build() error = %v", err)
	}
	shapeNode := findSceneNode(t, document.Root, target.ID)
	roughPaths := 0
	walkSketchTestNodes(shapeNode, func(node *d2scene.Node) {
		if strings.Contains(node.ID, ":sketch:set:") {
			roughPaths++
		}
	})
	if roughPaths == 0 {
		t.Fatal("top-level sketch Build did not route shape geometry through typed rough paths")
	}
}

func TestSketchStreakCanonicalGeometryAndRasterSmoke(t *testing.T) {
	t.Parallel()

	commands, err := parseSketchStreakSource(context.Background(), patternassets.StreakPathData())
	if err != nil {
		t.Fatal(err)
	}
	if len(commands) != sketchStreakCommandCount || len(commands) == 0 {
		t.Fatalf("decoded streak commands = %d, want %d", len(commands), sketchStreakCommandCount)
	}

	diagram := d2target.NewDiagram()
	b := newSketchShapeTestBuilder(context.Background(), diagram, largeSketchTestBudget())
	root := d2scene.NewNode(nil)
	for _, target := range []d2target.Shape{
		func() d2target.Shape {
			s := sketchTestShape("rect", d2target.ShapeRectangle, 20, 20, 140, 90)
			s.FillPattern = "dots"
			return s
		}(),
		sketchTestShape("cloud", d2target.ShapeCloud, 190, 20, 150, 90),
	} {
		node, err := b.buildShape(target)
		if err != nil {
			t.Fatal(err)
		}
		root.Children = append(root.Children, node)
	}
	document := d2scene.NewDocument(d2scene.Box{Width: 380, Height: 140}, root)
	document.Assets = b.assets
	frameOptions := patternFrameOptions()
	first, err := d2raster.Render(context.Background(), document, frameOptions)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	second, err := d2raster.Render(context.Background(), document, frameOptions)
	if err != nil {
		t.Fatalf("second Render() error = %v", err)
	}
	if !bytes.Equal(first.Pix, second.Pix) {
		t.Fatal("sketch raster output is not deterministic")
	}
	painted := 0
	for offset := 0; offset+3 < len(first.Pix); offset += 4 {
		if first.Pix[offset] != 255 || first.Pix[offset+1] != 255 || first.Pix[offset+2] != 255 || first.Pix[offset+3] != 255 {
			painted++
		}
	}
	if painted == 0 {
		t.Fatal("sketch raster smoke produced no painted pixels")
	}
}

func TestSketchStreakLocalSubpathsMatchCanonicalEvenOddTileWithinProductionWork(t *testing.T) {
	commands, err := parseSketchStreakSource(context.Background(), patternassets.StreakPathData())
	if err != nil {
		t.Fatal(err)
	}
	_, ink, err := sketchStreakStyle("bright")
	if err != nil {
		t.Fatal(err)
	}
	localRoot, err := sketchStreakRoot(context.Background(), commands, ink, "bright")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(localRoot.Children), sketchStreakSubpathCount-sketchStreakCompoundSubpathSpan+1; got != want {
		t.Fatalf("local streak primitives = %d, want %d", got, want)
	}
	canonicalRoot := d2scene.NewNode(d2scene.Path{
		Commands: commands,
		FillRule: d2scene.EvenOdd,
		Fill:     d2scene.SolidPaint{Color: ink},
	})
	patternDocument := func(patternRoot *d2scene.Node) *d2scene.Document {
		paint := d2scene.PatternPaint{
			Tile: d2scene.Box{Width: 100, Height: 100}, Root: patternRoot,
			Units: d2scene.UserSpaceOnUse, Transform: d2scene.Identity(),
		}
		return d2scene.NewDocument(
			d2scene.Box{Width: 100, Height: 100},
			d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 100, Height: 100}, Fill: paint}),
		)
	}

	options := patternFrameOptions()
	options.Scale = 2
	canonical, err := d2raster.Render(context.Background(), patternDocument(canonicalRoot), options)
	if err != nil {
		t.Fatalf("canonical even-odd Render() error = %v", err)
	}
	local, err := d2raster.Render(context.Background(), patternDocument(localRoot), options)
	if err != nil {
		t.Fatalf("local-subpath Render() error = %v", err)
	}
	if !bytes.Equal(canonical.Pix, local.Pix) {
		t.Fatal("local streak subpaths differ from the canonical even-odd tile at 2x scale")
	}

	options.MaxEvenOddClipWork = 250_000_000
	if _, err := d2raster.Render(context.Background(), patternDocument(localRoot), options); err != nil {
		t.Fatalf("local streak tile exceeds the production even-odd work ceiling: %v", err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := sketchStreakRoot(canceled, commands, ink, "bright"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled sketchStreakRoot() error = %v", err)
	}
	if _, err := sketchStreakRoot(context.Background(), commands[1:], ink, "bright"); err == nil || !strings.Contains(err.Error(), "unexpected subpath topology") {
		t.Fatalf("malformed sketchStreakRoot() error = %v", err)
	}
}

func newSketchShapeTestBuilder(ctx context.Context, diagram *d2target.Diagram, budget SketchBudget) *builder {
	return &builder{
		ctx: ctx, diagram: diagram,
		options:        Options{Sketch: true, SketchBudget: budget},
		theme:          d2themescatalog.Find(d2themescatalog.NeutralDefault.ID),
		assets:         make(map[d2scene.AssetID]d2scene.Asset),
		assetIDs:       make(map[[32]byte]d2scene.AssetID),
		sourceAssetIDs: make(map[string]d2scene.AssetID),
		idToShape:      make(map[string]d2target.Shape),
	}
}

func largeSketchTestBudget() SketchBudget {
	return SketchBudget{MaxOperationSets: 100_000, MaxOperations: 1_000_000, MaxPathCommands: 1_000_000}
}

func sketchTestShape(id, shapeType string, x, y, width, height int) d2target.Shape {
	return d2target.Shape{
		ID: id, Type: shapeType, Pos: d2target.Point{X: x, Y: y}, Width: width, Height: height,
		Fill: "#ffffff", Stroke: "#112233", StrokeWidth: 2, Opacity: 1,
		Text: d2target.Text{FontSize: 16, FontFamily: "default", Color: "#112233"},
	}
}

func walkSketchTestNodes(node *d2scene.Node, visit func(*d2scene.Node)) {
	if node == nil {
		return
	}
	visit(node)
	for _, child := range node.Children {
		walkSketchTestNodes(child, visit)
	}
}

func hasSketchTestClass(node *d2scene.Node, class string) bool {
	if node == nil {
		return false
	}
	for _, candidate := range node.Classes {
		if candidate == class {
			return true
		}
	}
	return false
}
