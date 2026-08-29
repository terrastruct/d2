package d2scenebuild

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image/color"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2raster"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/internal/patternassets"
	"github.com/d2lang/d2/d2target"
)

func TestBuildBuiltinFillPatternDefinitions(t *testing.T) {
	tests := []struct {
		name         string
		tile         float64
		rootOpacity  float64
		rootChildren int
		assets       int
	}{
		{name: "dots", tile: 15, rootOpacity: .1, rootChildren: 9},
		{name: "lines", tile: 15, rootOpacity: .05, rootChildren: 3},
		{name: "grain", tile: 300, rootOpacity: .8, rootChildren: 1, assets: 1},
		{name: "paper", tile: 75, rootOpacity: 1, assets: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := buildPatternDocument(t, test.name, 0, 0, int(test.tile), int(test.tile))
			shapeNode := findSceneNode(t, document.Root, "pattern")
			if len(shapeNode.Children) != 2 {
				t.Fatalf("pattern children = %d, want base and overlay", len(shapeNode.Children))
			}
			base, overlay := shapeNode.Children[0], shapeNode.Children[1]
			if overlay.ID != "pattern:fill-pattern:0" || !reflect.DeepEqual(overlay.Classes, []string{test.name + "-overlay"}) {
				t.Fatalf("overlay identity = %q/%v", overlay.ID, overlay.Classes)
			}
			if overlay.Blend != d2scene.BlendMultiply || overlay.Opacity != base.Opacity {
				t.Fatalf("overlay compositing = blend %d opacity %v, want multiply and %v", overlay.Blend, overlay.Opacity, base.Opacity)
			}
			baseRect := base.Primitive.(d2scene.Rect)
			overlayRect := overlay.Primitive.(d2scene.Rect)
			if baseRect.Box != overlayRect.Box || overlayRect.Stroke != nil {
				t.Fatalf("overlay geometry/stroke = %+v/%+v", overlayRect.Box, overlayRect.Stroke)
			}
			pattern, ok := overlayRect.Fill.(d2scene.PatternPaint)
			if !ok {
				t.Fatalf("overlay fill = %T, want PatternPaint", overlayRect.Fill)
			}
			if pattern.Tile != (d2scene.Box{Width: test.tile, Height: test.tile}) ||
				pattern.Units != d2scene.UserSpaceOnUse || pattern.Transform != d2scene.Identity() || pattern.Root == nil {
				t.Fatalf("pattern geometry = %+v units=%d transform=%+v root=%p", pattern.Tile, pattern.Units, pattern.Transform, pattern.Root)
			}
			if pattern.Root.Opacity != test.rootOpacity || len(pattern.Root.Children) != test.rootChildren {
				t.Fatalf("pattern root opacity/children = %v/%d, want %v/%d", pattern.Root.Opacity, len(pattern.Root.Children), test.rootOpacity, test.rootChildren)
			}
			if len(document.Assets) != test.assets {
				t.Fatalf("document assets = %d, want %d", len(document.Assets), test.assets)
			}

			switch test.name {
			case "dots":
				assertDotTile(t, pattern.Root)
			case "lines":
				assertLineTile(t, pattern.Root)
			case "grain":
				assertGrainPattern(t, document, pattern.Root)
			case "paper":
				assertPaperPattern(t, document, pattern.Root)
			}
		})
	}
}

func TestBuildFillPatternAttachmentOrder(t *testing.T) {
	t.Run("root double border patterns both layers", func(t *testing.T) {
		diagram := d2target.NewDiagram()
		diagram.Root.Fill, diagram.Root.Stroke, diagram.Root.StrokeWidth = "#ffffff", "none", 0
		diagram.Root.FillPattern, diagram.Root.DoubleBorder = "dots", true
		diagram.Shapes = []d2target.Shape{patternTestShape("shape", d2target.ShapeRectangle, 0, 0, 40, 20, "")}
		pad := int64(0)
		document, err := Build(context.Background(), diagram, Options{Pad: &pad})
		if err != nil {
			t.Fatal(err)
		}
		assertChildIDs(t, document.Root, []string{
			"root:double-border:outer", "root:double-border:outer:fill-pattern",
			"root:background", "root:background:fill-pattern", "shape",
		})
		inner := document.Root.Children[2].Primitive.(d2scene.Rect)
		if alphaOf(t, inner.Fill) != 0 {
			t.Fatal("double-root inner base must stay transparent beneath its pattern overlay")
		}
	})

	t.Run("ordinary multipath interleaves", func(t *testing.T) {
		document := buildPatternShapeDocument(t, patternTestShape("cylinder", d2target.ShapeCylinder, 0, 0, 80, 60, "dots"))
		node := findSceneNode(t, document.Root, "cylinder")
		if len(node.Children) != 4 {
			t.Fatalf("cylinder children = %d, want base/overlay for two paths", len(node.Children))
		}
		for index := 0; index < 4; index += 2 {
			base := node.Children[index].Primitive.(d2scene.Path)
			overlay := node.Children[index+1].Primitive.(d2scene.Path)
			if !reflect.DeepEqual(base.Commands, overlay.Commands) || overlay.Stroke != nil {
				t.Fatalf("path pair %d did not preserve geometry and clear stroke", index/2)
			}
		}
	})

	t.Run("3d patterns main face only", func(t *testing.T) {
		shape := patternTestShape("three", d2target.ShapeRectangle, 0, 10, 80, 50, "dots")
		shape.ThreeDee = true
		document := buildPatternShapeDocument(t, shape)
		assertChildIDs(t, findSceneNode(t, document.Root, shape.ID), []string{
			"three:3d-main", "three:3d-main:fill-pattern", "three:3d-sides", "three:3d-border",
		})
	})

	t.Run("multiple patterns front only", func(t *testing.T) {
		shape := patternTestShape("multiple", d2target.ShapeDiamond, 0, 10, 80, 50, "dots")
		shape.Multiple = true
		document := buildPatternShapeDocument(t, shape)
		assertChildIDs(t, findSceneNode(t, document.Root, shape.ID), []string{
			"multiple:multiple", "multiple:main", "multiple:main:fill-pattern",
		})
	})

	t.Run("double border patterns outer only", func(t *testing.T) {
		shape := patternTestShape("double", d2target.ShapeRectangle, 0, 0, 80, 50, "dots")
		shape.DoubleBorder = true
		document := buildPatternShapeDocument(t, shape)
		assertChildIDs(t, findSceneNode(t, document.Root, shape.ID), []string{
			"double:double-border:outer", "double:double-border:outer:fill-pattern", "double:double-border:inner",
		})
	})

	t.Run("multiple double rectangle patterns rear outer", func(t *testing.T) {
		shape := patternTestShape("double", d2target.ShapeRectangle, 0, 10, 80, 50, "dots")
		shape.Multiple, shape.DoubleBorder = true, true
		document := buildPatternShapeDocument(t, shape)
		assertChildIDs(t, findSceneNode(t, document.Root, shape.ID), []string{
			"double:multiple:outer", "double:multiple:outer:fill-pattern", "double:multiple:inner",
			"double:double-border:outer", "double:double-border:outer:fill-pattern", "double:double-border:inner",
		})
	})

	t.Run("multiple double oval leaves rear unpatterned", func(t *testing.T) {
		shape := patternTestShape("double", d2target.ShapeOval, 0, 10, 80, 50, "dots")
		shape.Multiple, shape.DoubleBorder = true, true
		document := buildPatternShapeDocument(t, shape)
		assertChildIDs(t, findSceneNode(t, document.Root, shape.ID), []string{
			"double:multiple:outer", "double:multiple:inner",
			"double:double-border:outer", "double:double-border:outer:fill-pattern", "double:double-border:inner",
		})
	})

	for _, shapeType := range []string{d2target.ShapeClass, d2target.ShapeSQLTable} {
		shapeType := shapeType
		t.Run(shapeType+" patterns outline and header", func(t *testing.T) {
			shape := structuredTestShape("structured", shapeType, 0, 0, 100, 60)
			shape.FillPattern = "dots"
			document := buildPatternShapeDocument(t, shape)
			children := findSceneNode(t, document.Root, shape.ID).Children
			wantPrefix := []string{"structured:outline", "structured:outline:fill-pattern"}
			if shapeType == d2target.ShapeClass {
				wantPrefix = append(wantPrefix, "structured:class-header", "structured:class-header:fill-pattern")
			} else {
				wantPrefix = append(wantPrefix, "structured:table-header", "structured:table-header:fill-pattern")
			}
			if got := childIDs(children[:len(wantPrefix)]); !reflect.DeepEqual(got, wantPrefix) {
				t.Fatalf("structured prefix = %v, want %v", got, wantPrefix)
			}
		})
	}
}

func TestBuildFillPatternsAreNoOpForImageTextAndCode(t *testing.T) {
	tests := []struct {
		name    string
		shape   d2target.Shape
		options Options
	}{
		{
			name: "image",
			shape: func() d2target.Shape {
				shape := patternTestShape("noop", d2target.ShapeImage, 0, 0, 20, 10, "paper")
				shape.Icon = testRasterAssetURL(t)
				return shape
			}(),
			options: Options{Assets: testAssetOptions(t)},
		},
		{name: "text", shape: patternTestShape("noop", d2target.ShapeText, 0, 0, 20, 10, "paper")},
		{name: "code", shape: patternTestShape("noop", d2target.ShapeCode, 0, 0, 20, 10, "paper")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			diagram := d2target.NewDiagram()
			diagram.Root.Fill, diagram.Root.Stroke, diagram.Root.StrokeWidth = "#ffffff", "none", 0
			diagram.Shapes = []d2target.Shape{test.shape}
			pad := int64(0)
			test.options.Pad = &pad
			document, err := Build(context.Background(), diagram, test.options)
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := document.Assets[paperPatternAssetID]; ok {
				t.Fatalf("%s shape materialized unused paper asset", test.name)
			}
			for _, child := range findSceneNode(t, document.Root, "noop").Children {
				if strings.Contains(child.ID, "fill-pattern") {
					t.Fatalf("%s shape emitted pattern overlay %q", test.name, child.ID)
				}
			}
		})
	}
}

func TestBuildRejectsUnknownFillPatternsExplicitly(t *testing.T) {
	uppercase := buildPatternDocument(t, "DoTs", 0, 0, 15, 15)
	overlay := findSceneNode(t, uppercase.Root, "pattern").Children[1]
	if !reflect.DeepEqual(overlay.Classes, []string{"dots-overlay"}) {
		t.Fatalf("compiler-accepted mixed-case pattern was not canonicalized: %v", overlay.Classes)
	}

	tests := []struct {
		name string
		edit func(*d2target.Diagram)
	}{
		{name: "root", edit: func(diagram *d2target.Diagram) { diagram.Root.FillPattern = "hatch" }},
		{name: "shape", edit: func(diagram *d2target.Diagram) { diagram.Shapes[0].FillPattern = "hatch" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			diagram := d2target.NewDiagram()
			diagram.Shapes = []d2target.Shape{patternTestShape("shape", d2target.ShapeRectangle, 0, 0, 20, 10, "")}
			test.edit(diagram)
			_, err := Build(context.Background(), diagram, Options{})
			if err == nil || !strings.Contains(err.Error(), `field "fillPattern"`) || !strings.Contains(err.Error(), "dots, lines, grain, or paper") {
				t.Fatalf("Build() error = %v, want explicit fillPattern enum error", err)
			}
		})
	}

	animated := d2target.NewDiagram()
	animated.Shapes = []d2target.Shape{patternTestShape("animated", d2target.ShapeRectangle, 0, 0, 20, 10, "dots")}
	animated.Shapes[0].Animated = true
	document, err := Build(context.Background(), animated, Options{})
	if err != nil {
		t.Fatalf("animated patterned shape Build() error = %v", err)
	}
	node := findSceneNode(t, document.Root, "animated")
	if len(node.Animations) != 3 || len(node.Filters) != 2 {
		t.Fatalf("animated patterned shape tracks/filters = %d/%d, want 3/2", len(node.Animations), len(node.Filters))
	}
}

func TestBuiltinPatternSourcesAndAssetsHaveStableHashes(t *testing.T) {
	if paperPatternAssetID != d2scene.AssetID("builtin:fill-pattern:paper:"+paperPatternSourceSHA256) ||
		grainPatternAssetID != d2scene.AssetID("builtin:fill-pattern:grain:"+grainPatternPNGSourceSHA256) {
		t.Fatal("built-in pattern asset IDs are not derived from canonical content hashes")
	}
	paperSource := patternassets.PaperSVG()
	if got := sha256Hex([]byte(paperSource)); got != paperPatternSourceSHA256 {
		t.Fatalf("paper source SHA-256 = %s, want %s", got, paperPatternSourceSHA256)
	}
	parsedPaper, err := parsePaperPatternSource(context.Background(), paperSource)
	if err != nil {
		t.Fatal(err)
	}
	asset, err := newPaperPatternAsset(context.Background(), parsedPaper)
	if err != nil {
		t.Fatal(err)
	}
	assertPaperVectorAsset(t, asset)

	grainPNG := patternassets.GrainPNG()
	if got := sha256Hex(grainPNG); got != grainPatternPNGSourceSHA256 || len(grainPNG) != grainPatternPNGBytes {
		t.Fatalf("grain PNG = SHA %s bytes %d, want %s/%d", got, len(grainPNG), grainPatternPNGSourceSHA256, grainPatternPNGBytes)
	}
	grain, err := parseGrainPatternSource(context.Background(), grainPNG)
	if err != nil {
		t.Fatal(err)
	}
	if grain.pixelWidth != grainPatternPixelWidth || grain.pixelHeight != grainPatternPixelHeight || grain.decodedBytes != grainPatternDecodedBytes {
		t.Fatalf("grain metadata = %dx%d/%d", grain.pixelWidth, grain.pixelHeight, grain.decodedBytes)
	}
}

func TestBuiltinPatternSourceCancellationAndCorruption(t *testing.T) {
	t.Run("cancellation is not cached", func(t *testing.T) {
		var cache patternSourceCache[*paperPatternSource]
		ctx, cancel := context.WithCancel(context.Background())
		_, err := cache.load(ctx, func(ctx context.Context) (*paperPatternSource, error) {
			cancel()
			return nil, ctx.Err()
		})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("first cache load error = %v, want context.Canceled", err)
		}
		source, err := cache.load(context.Background(), func(ctx context.Context) (*paperPatternSource, error) {
			return parsePaperPatternSource(ctx, patternassets.PaperSVG())
		})
		if err != nil || source == nil {
			t.Fatalf("cache retry = %p, %v", source, err)
		}
	})

	t.Run("waiting caller can cancel", func(t *testing.T) {
		var cache patternSourceCache[*paperPatternSource]
		started := make(chan struct{})
		release := make(chan struct{})
		loaded := make(chan error, 1)
		go func() {
			_, err := cache.load(context.Background(), func(context.Context) (*paperPatternSource, error) {
				close(started)
				<-release
				return &paperPatternSource{}, nil
			})
			loaded <- err
		}()
		<-started
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := cache.load(ctx, func(context.Context) (*paperPatternSource, error) {
			t.Fatal("waiting caller unexpectedly became the loader")
			return nil, nil
		}); !errors.Is(err, context.Canceled) {
			t.Fatalf("waiting cache load error = %v, want context.Canceled", err)
		}
		close(release)
		if err := <-loaded; err != nil {
			t.Fatal(err)
		}
	})

	t.Run("corrupt canonical sources", func(t *testing.T) {
		paper := patternassets.PaperSVG()
		paper = paper[:len(paper)/2] + "x" + paper[len(paper)/2+1:]
		if _, err := parsePaperPatternSource(context.Background(), paper); err == nil {
			t.Fatal("parsePaperPatternSource() accepted corrupt source")
		}
		grain := append([]byte(nil), patternassets.GrainPNG()...)
		grain[len(grain)/2] ^= 1
		if _, err := parseGrainPatternSource(context.Background(), grain); err == nil {
			t.Fatal("parseGrainPatternSource() accepted corrupt source")
		}
	})

	for _, pattern := range []string{"dots", "lines", "grain", "paper"} {
		diagram := d2target.NewDiagram()
		diagram.Root.FillPattern = pattern
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := Build(ctx, diagram, Options{}); !errors.Is(err, context.Canceled) {
			t.Fatalf("pre-canceled %s Build() error = %v, want context.Canceled", pattern, err)
		}
	}
}

func TestBuiltinPatternAssetsAreConcurrentAndDocumentOwned(t *testing.T) {
	const workers = 4
	type result struct {
		document *d2scene.Document
		err      error
	}
	results := make(chan result, workers)
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			diagram := d2target.NewDiagram()
			diagram.Root.Fill, diagram.Root.Stroke, diagram.Root.StrokeWidth = "#ffffff", "none", 0
			diagram.Shapes = []d2target.Shape{
				patternTestShape("paper-1", d2target.ShapeRectangle, 0, 0, 75, 75, "paper"),
				patternTestShape("paper-2", d2target.ShapeRectangle, 80, 0, 75, 75, "paper"),
				patternTestShape("grain-1", d2target.ShapeRectangle, 160, 0, 300, 300, "grain"),
				patternTestShape("grain-2", d2target.ShapeRectangle, 465, 0, 300, 300, "grain"),
			}
			pad := int64(0)
			document, err := Build(context.Background(), diagram, Options{Pad: &pad})
			results <- result{document: document, err: err}
		}()
	}
	wait.Wait()
	close(results)

	paperRoots := make(map[*d2scene.Node]bool, workers)
	grainPointers := make(map[*byte]bool, workers)
	for result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}
		if len(result.document.Assets) != 2 {
			t.Fatalf("document assets = %d, want paper and grain", len(result.document.Assets))
		}
		paper := result.document.Assets[paperPatternAssetID].(d2scene.VectorAsset)
		if paperRoots[paper.Root] {
			t.Fatal("concurrent documents share mutable paper vector nodes")
		}
		paperRoots[paper.Root] = true
		grain := result.document.Assets[grainPatternAssetID].(d2scene.RasterAsset)
		pointer := &grain.Data[0]
		if grainPointers[pointer] {
			t.Fatal("concurrent documents share mutable grain bytes")
		}
		grainPointers[pointer] = true
		if sha256Hex(grain.Data) != grainPatternPNGSourceSHA256 {
			t.Fatal("concurrent grain asset hash changed")
		}
	}
}

func TestBuiltinFillPatternPixelsRepeatWithGlobalUserSpacePhase(t *testing.T) {
	for _, test := range []struct {
		name string
		tile int
	}{
		{name: "dots", tile: 15},
		{name: "lines", tile: 15},
		{name: "paper", tile: 75},
		{name: "grain", tile: 300},
	} {
		t.Run(test.name, func(t *testing.T) {
			document := buildPatternDocument(t, test.name, 0, 0, test.tile*2, test.tile)
			frame, err := d2raster.Render(context.Background(), document, patternFrameOptions())
			if err != nil {
				t.Fatal(err)
			}
			nonWhite := false
			for y := 0; y < test.tile; y++ {
				for x := 0; x < test.tile; x++ {
					left, right := frame.NRGBAAt(x, y), frame.NRGBAAt(x+test.tile, y)
					if left != right {
						t.Fatalf("repeat mismatch at (%d,%d): left=%v right=%v", x, y, left, right)
					}
					if left != (color.NRGBA{R: 255, G: 255, B: 255, A: 255}) {
						nonWhite = true
					}
				}
			}
			if !nonWhite {
				t.Fatal("pattern rendered no visible pixels over white base")
			}
		})
	}

	t.Run("phase does not restart at object bounds", func(t *testing.T) {
		diagram := d2target.NewDiagram()
		diagram.Root.Fill, diagram.Root.Stroke, diagram.Root.StrokeWidth = "#ffffff", "none", 0
		diagram.Shapes = []d2target.Shape{
			patternTestShape("first", d2target.ShapeRectangle, 0, 0, 20, 15, "dots"),
			patternTestShape("second", d2target.ShapeRectangle, 21, 0, 15, 15, "dots"),
		}
		pad := int64(0)
		document, err := Build(context.Background(), diagram, Options{Pad: &pad})
		if err != nil {
			t.Fatal(err)
		}
		frame, err := d2raster.Render(context.Background(), document, patternFrameOptions())
		if err != nil {
			t.Fatal(err)
		}
		white := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
		if frame.NRGBAAt(2, 2) == white {
			t.Fatal("expected absolute user-space dot at (2,2)")
		}
		if got := frame.NRGBAAt(23, 2); got != white {
			t.Fatalf("second shape restarted pattern at local (2,2): got %v", got)
		}
		if frame.NRGBAAt(17, 2) != frame.NRGBAAt(2, 2) {
			t.Fatal("dot tile does not repeat at the 15px seam")
		}
	})
}

func TestBuiltinFillPatternShapeOpacityAppliesOnce(t *testing.T) {
	baseShape := patternTestShape("pattern", d2target.ShapeRectangle, 0, 0, 15, 15, "")
	baseShape.Fill, baseShape.Opacity = "#6699cc", .5
	patternShape := baseShape
	patternShape.FillPattern = "dots"
	base := buildPatternShapeDocument(t, baseShape)
	pattern := buildPatternShapeDocument(t, patternShape)
	patternGroup := findSceneNode(t, pattern.Root, "pattern")
	if patternGroup.Opacity != .5 || patternGroup.Children[1].Opacity != 1 {
		t.Fatalf("shape/overlay opacity = %v/%v, want .5/1", patternGroup.Opacity, patternGroup.Children[1].Opacity)
	}
	baseFrame, err := d2raster.Render(context.Background(), base, patternFrameOptions())
	if err != nil {
		t.Fatal(err)
	}
	patternFrame, err := d2raster.Render(context.Background(), pattern, patternFrameOptions())
	if err != nil {
		t.Fatal(err)
	}
	if got, want := patternFrame.NRGBAAt(3, 3), baseFrame.NRGBAAt(3, 3); got != want {
		t.Fatalf("away from dot pattern opacity changed base: got %v want %v", got, want)
	}
	if got, basePixel := patternFrame.NRGBAAt(2, 2), baseFrame.NRGBAAt(2, 2); got == basePixel {
		t.Fatalf("dot disappeared under group opacity: patterned/base = %v", got)
	}
}

func TestBuiltinFillPatternLimitsAreExactAndInclusive(t *testing.T) {
	document := buildPatternDocument(t, "dots", 0, 0, 15, 15)
	options := patternFrameOptions()
	options.MaxNodes = 14
	if _, err := d2raster.Render(context.Background(), document, options); err == nil || !strings.Contains(err.Error(), "node count exceeds limit 14") {
		t.Fatalf("MaxNodes limit-1 error = %v", err)
	}
	options.MaxNodes = 15
	low, high := int64(1), options.MaxOffscreenBytes
	for low < high {
		middle := low + (high-low)/2
		candidate := options
		candidate.MaxOffscreenBytes = middle
		if _, err := d2raster.Render(context.Background(), document, candidate); err != nil {
			low = middle + 1
		} else {
			high = middle
		}
	}
	minimumOffscreenBytes := low
	options.MaxOffscreenBytes = minimumOffscreenBytes - 1
	if _, err := d2raster.Render(context.Background(), document, options); err == nil {
		t.Fatalf("MaxOffscreenBytes limit-1 error = %v", err)
	}
	options.MaxOffscreenBytes = minimumOffscreenBytes
	if _, err := d2raster.Render(context.Background(), document, options); err != nil {
		t.Fatalf("inclusive pattern limits failed: %v", err)
	}

	grain := buildPatternDocument(t, "grain", 0, 0, 300, 300)
	options = patternFrameOptions()
	options.MaxAssetBytes = int64(grainPatternPNGBytes - 1)
	if _, err := d2raster.Render(context.Background(), grain, options); err == nil || !strings.Contains(err.Error(), "retained asset bytes") {
		t.Fatalf("grain encoded limit-1 error = %v", err)
	}
	options.MaxAssetBytes = int64(grainPatternPNGBytes)
	options.MaxDecodedAssetBytes = grainPatternDecodedBytes - 1
	if _, err := d2raster.Render(context.Background(), grain, options); err == nil || !strings.Contains(err.Error(), "decoded storage") {
		t.Fatalf("grain decoded limit-1 error = %v", err)
	}
	options.MaxDecodedAssetBytes = grainPatternDecodedBytes
	if _, err := d2raster.Render(context.Background(), grain, options); err != nil {
		t.Fatalf("inclusive grain asset limits failed: %v", err)
	}

	paper := buildPatternDocument(t, "paper", 0, 0, 75, 75)
	options = patternFrameOptions()
	options.MaxNodes = 2091
	if _, err := d2raster.Render(context.Background(), paper, options); err == nil || !strings.Contains(err.Error(), "node count exceeds limit 2091") {
		t.Fatalf("paper MaxNodes limit-1 error = %v", err)
	}
	options.MaxNodes = 2092
	// The canonical definition charges its 10,926 typed commands once. Its
	// one visible placement then charges device-flattened path work together
	// with the three host rectangles, for an exact 23,983-command ceiling.
	options.MaxPathCommands = 23982
	if _, err := d2raster.Render(context.Background(), paper, options); err == nil || !strings.Contains(err.Error(), "path command count exceeds limit 23982") {
		t.Fatalf("paper MaxPathCommands limit-1 error = %v", err)
	}
	options.MaxPathCommands = 23983
	if _, err := d2raster.Render(context.Background(), paper, options); err != nil {
		t.Fatalf("inclusive paper structural limits failed: %v", err)
	}
}

func assertDotTile(t *testing.T, root *d2scene.Node) {
	t.Helper()
	if root.Blend != d2scene.BlendMultiply {
		t.Fatalf("dot tile blend = %d, want multiply", root.Blend)
	}
	want := map[[2]float64]bool{}
	for _, y := range []float64{2, 7, 12} {
		for _, x := range []float64{2, 7, 12} {
			want[[2]float64{x, y}] = true
		}
	}
	for _, child := range root.Children {
		rect := child.Primitive.(d2scene.Rect)
		if rect.Box.Width != 1 || rect.Box.Height != 1 || !want[[2]float64{rect.Box.X, rect.Box.Y}] {
			t.Fatalf("unexpected dot %+v", rect.Box)
		}
		delete(want, [2]float64{rect.Box.X, rect.Box.Y})
		assertPatternInk(t, rect.Fill)
	}
	if len(want) != 0 {
		t.Fatalf("missing dots: %v", want)
	}
}

func assertLineTile(t *testing.T, root *d2scene.Node) {
	t.Helper()
	if root.Blend != d2scene.BlendMultiply {
		t.Fatalf("line tile blend = %d, want multiply", root.Blend)
	}
	for index, y := range []float64{2, 7, 12} {
		rect := root.Children[index].Primitive.(d2scene.Rect)
		if rect.Box != (d2scene.Box{Y: y, Width: 15, Height: 1}) {
			t.Fatalf("line %d = %+v", index, rect.Box)
		}
		assertPatternInk(t, rect.Fill)
	}
}

func assertPatternInk(t *testing.T, paint d2scene.Paint) {
	t.Helper()
	solid, ok := paint.(d2scene.SolidPaint)
	if !ok || solid.Color != (color.NRGBA{R: 0x0a, G: 0x0f, B: 0x25, A: 0xff}) {
		t.Fatalf("pattern ink = %#v", paint)
	}
}

func assertGrainPattern(t *testing.T, document *d2scene.Document, root *d2scene.Node) {
	t.Helper()
	imageNode := root.Children[0]
	imagePrimitive, ok := imageNode.Primitive.(d2scene.Image)
	if !ok || imageNode.Opacity != .9 || imagePrimitive.Asset != grainPatternAssetID ||
		imagePrimitive.Box != (d2scene.Box{Width: 300, Height: 300}) || imagePrimitive.Aspect.Align != d2scene.AlignNone {
		t.Fatalf("grain image = node opacity %v primitive %+v", imageNode.Opacity, imagePrimitive)
	}
	asset, ok := document.Assets[grainPatternAssetID].(d2scene.RasterAsset)
	if !ok || asset.MIMEType != "image/png" || asset.PixelWidth != 466 || asset.PixelHeight != 349 ||
		asset.DecodedBytes != grainPatternDecodedBytes || len(asset.Data) != grainPatternPNGBytes || sha256Hex(asset.Data) != grainPatternPNGSourceSHA256 {
		t.Fatalf("grain asset = %#v", asset)
	}
}

func assertPaperPattern(t *testing.T, document *d2scene.Document, root *d2scene.Node) {
	t.Helper()
	imagePrimitive, ok := root.Primitive.(d2scene.Image)
	if !ok || imagePrimitive.Asset != paperPatternAssetID || imagePrimitive.Box != (d2scene.Box{Width: 75, Height: 75}) || imagePrimitive.Aspect.Align != d2scene.AlignNone {
		t.Fatalf("paper image = %+v", imagePrimitive)
	}
	asset, ok := document.Assets[paperPatternAssetID].(d2scene.VectorAsset)
	if !ok {
		t.Fatalf("paper asset = %T", document.Assets[paperPatternAssetID])
	}
	assertPaperVectorAsset(t, asset)
}

func assertPaperVectorAsset(t *testing.T, asset d2scene.VectorAsset) {
	t.Helper()
	if asset.ViewBox != (d2scene.Box{Width: 75, Height: 75}) || asset.Root == nil ||
		asset.Root.Opacity != .3 || asset.Root.Blend != d2scene.BlendMultiply || len(asset.Root.Children) != paperPatternPathCount {
		t.Fatalf("paper asset topology = viewbox %+v root opacity/blend/children %v/%d/%d", asset.ViewBox, asset.Root.Opacity, asset.Root.Blend, len(asset.Root.Children))
	}
	commands := 0
	for index, child := range asset.Root.Children {
		path, ok := child.Primitive.(d2scene.Path)
		if !ok || path.Fill == nil || path.Stroke != nil || len(path.Commands) == 0 {
			t.Fatalf("paper path %d = %#v", index, child.Primitive)
		}
		commands += len(path.Commands)
	}
	if commands != paperPatternCommandCount {
		t.Fatalf("paper commands = %d, want %d", commands, paperPatternCommandCount)
	}
}

func buildPatternDocument(t *testing.T, pattern string, x, y, width, height int) *d2scene.Document {
	t.Helper()
	return buildPatternShapeDocument(t, patternTestShape("pattern", d2target.ShapeRectangle, x, y, width, height, pattern))
}

func buildPatternShapeDocument(t *testing.T, shape d2target.Shape) *d2scene.Document {
	t.Helper()
	diagram := d2target.NewDiagram()
	diagram.Root.Fill, diagram.Root.Stroke, diagram.Root.StrokeWidth = "#ffffff", "none", 0
	diagram.Shapes = []d2target.Shape{shape}
	pad := int64(0)
	document, err := Build(context.Background(), diagram, Options{Pad: &pad})
	if err != nil {
		t.Fatal(err)
	}
	return document
}

func patternTestShape(id, shapeType string, x, y, width, height int, pattern string) d2target.Shape {
	return d2target.Shape{
		ID: id, Type: shapeType, Pos: d2target.Point{X: x, Y: y}, Width: width, Height: height,
		Fill: "#ffffff", FillPattern: pattern, Stroke: "none", StrokeWidth: 0, Opacity: 1,
	}
}

func patternFrameOptions() d2raster.FrameOptions {
	return d2raster.FrameOptions{
		Scale: 1, Background: color.White,
		MaxWidth: 4096, MaxHeight: 4096, MaxPixels: 16_777_216,
		MaxNodes: 100_000, MaxDepth: 128, MaxPathCommands: 1_000_000,
		MaxAnimationTracks: 100_000, MaxAnimationKeyframes: 1_000_000,
		MaxAssets: 100, MaxAssetBytes: 64 << 20, MaxDecodedAssetBytes: 64 << 20, MaxImportDepth: 128,
		MaxOffscreenBytes: 64 << 20, MaxEvenOddClipWork: 1_000_000_000,
	}
}

func sha256Hex(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
