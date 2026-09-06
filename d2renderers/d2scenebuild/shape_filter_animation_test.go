package d2scenebuild

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/d2lang/d2/d2renderers/d2raster"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2target"
)

func TestBuildStaticShapeShadowFilterAndScope(t *testing.T) {
	t.Parallel()

	shape := effectTestShape("shadow", d2target.ShapeRectangle, 10, 20, 40, 30)
	shape.Shadow = true
	shape.Text = d2target.Text{
		Label: "outside", FontSize: 16, FontFamily: "default", LabelWidth: 50, LabelHeight: 19,
	}
	document := buildEffectsDocument(t, shape)
	outer := findSceneNode(t, document.Root, shape.ID)
	assertChildIDs(t, outer, []string{"shadow:shape", "shadow:label:0"})
	inner := outer.Children[0]
	if len(inner.Filters) != 1 {
		t.Fatalf("inner filters = %d, want one svg drop shadow", len(inner.Filters))
	}
	shadow, ok := inner.Filters[0].(d2scene.DropShadow)
	if !ok {
		t.Fatalf("inner filter = %T, want DropShadow", inner.Filters[0])
	}
	want := d2scene.DropShadow{
		OffsetX: 3, OffsetY: 5,
		Color: color.NRGBA{R: 0x3d, G: 0x45, B: 0x74, A: 102},
	}
	if shadow != want {
		t.Fatalf("static shadow = %+v, want sharp svg composite %+v", shadow, want)
	}
	if shadow.SigmaX != 0 || shadow.SigmaY != 0 {
		t.Fatal("svg filter's unreferenced feGaussianBlur must not blur the visible shadow")
	}
	if len(inner.Children) != 1 {
		t.Fatalf("inner geometry children = %d, want outline only", len(inner.Children))
	}
	if _, ok := inner.Children[0].Primitive.(d2scene.Rect); !ok {
		t.Fatalf("inner geometry = %T, want Rect", inner.Children[0].Primitive)
	}
	if _, ok := outer.Children[1].Primitive.(d2scene.TextRun); !ok {
		t.Fatalf("outer sibling = %T, want label outside static filter", outer.Children[1].Primitive)
	}
}

func TestBuildStaticShapeShadowExclusions(t *testing.T) {
	t.Parallel()

	shapes := []d2target.Shape{
		effectTestShape("text", d2target.ShapeText, 0, 0, 80, 40),
		effectTestShape("code", d2target.ShapeCode, 0, 0, 80, 40),
		structuredTestShape("class", d2target.ShapeClass, 0, 0, 80, 40),
		structuredTestShape("sql", d2target.ShapeSQLTable, 0, 0, 80, 40),
	}
	for _, shape := range shapes {
		shape := shape
		t.Run(shape.Type, func(t *testing.T) {
			shape.Shadow = true
			document := buildEffectsDocument(t, shape)
			node := findSceneNode(t, document.Root, shape.ID)
			if sceneNodeHasFilter(node) {
				t.Fatalf("excluded %s shape unexpectedly contains a filter", shape.Type)
			}
		})
	}
}

func TestBuildShapeBlendInnerGroup(t *testing.T) {
	t.Parallel()

	shape := effectTestShape("blend", d2target.ShapeRectangle, 0, 0, 40, 30)
	shape.Blend = true
	shape.Opacity = .8
	shape.Text = d2target.Text{
		Label: "outside", FontSize: 16, FontFamily: "default", LabelWidth: 50, LabelHeight: 19,
	}
	document := buildEffectsDocument(t, shape)
	outer := findSceneNode(t, document.Root, shape.ID)
	if outer.Opacity != .8 || outer.Blend != d2scene.BlendNormal {
		t.Fatalf("outer opacity/blend = %v/%d, want target opacity and normal blend", outer.Opacity, outer.Blend)
	}
	assertChildIDs(t, outer, []string{"blend:shape", "blend:label:0"})
	inner := outer.Children[0]
	if inner.Opacity != .5 || inner.Blend != d2scene.BlendMultiply {
		t.Fatalf("inner opacity/blend = %v/%d, want .5/multiply", inner.Opacity, inner.Blend)
	}
	if len(inner.Filters) != 0 || len(inner.Children) != 1 {
		t.Fatalf("inner filters/children = %d/%d, want geometry-only blend group", len(inner.Filters), len(inner.Children))
	}

	// Code shapes close an empty inner group before syntax-highlighted content,
	// so blend is intentionally a visual no-op.
	code := effectTestShape("code", d2target.ShapeCode, 0, 0, 80, 40)
	code.Blend = true
	code.Text = d2target.Text{Label: "x", LabelWidth: 10, LabelHeight: 19, FontSize: 16, FontFamily: "mono"}
	codeDocument := buildEffectsDocument(t, code)
	codeOuter := findSceneNode(t, codeDocument.Root, code.ID)
	if len(codeOuter.Children) < 2 || codeOuter.Children[0].ID != "code:shape" || len(codeOuter.Children[0].Children) != 0 {
		t.Fatalf("code blend topology = %#v, want empty inner group before plain label", codeOuter.Children)
	}
	if codeOuter.Children[0].Blend != d2scene.BlendMultiply || codeOuter.Children[0].Opacity != .5 {
		t.Fatalf("code inner opacity/blend = %v/%d", codeOuter.Children[0].Opacity, codeOuter.Children[0].Blend)
	}
}

func TestBuildAnimatedShapeTracksAndCombinedOrder(t *testing.T) {
	t.Parallel()

	shape := effectTestShape("effects", d2target.ShapeRectangle, 20, 30, 50, 40)
	shape.Animated, shape.Shadow, shape.Blend = true, true, true
	shape.Opacity = .75
	shape.Text = d2target.Text{
		Label: "outer", FontSize: 16, FontFamily: "default", LabelWidth: 40, LabelHeight: 19,
	}
	document := buildEffectsDocument(t, shape)
	outer := findSceneNode(t, document.Root, shape.ID)
	if outer.Opacity != .75 || len(outer.Filters) != 2 || len(outer.Animations) != 3 {
		t.Fatalf("outer effects = opacity %v filters %d tracks %d", outer.Opacity, len(outer.Filters), len(outer.Animations))
	}
	if outer.Clip == nil || len(outer.Clip.Path.Commands) != 5 {
		t.Fatalf("animated outer isolation clip = %#v, want bounded rectangular stacking context", outer.Clip)
	}
	for index, filter := range outer.Filters {
		shadow, ok := filter.(d2scene.DropShadow)
		if !ok || shadow != animatedShapeFilterDeclarations[index] || shadow.Color.A != 0 {
			t.Fatalf("declared animated filter %d = %#v, want transparent maximum-work shadow", index, filter)
		}
	}

	transformTrack := outer.Animations[0]
	assertShapeEffectTrack(t, transformTrack, d2scene.AnimateTransform, 0)
	assertAnimationKeyframe(t, transformTrack, 0, d2scene.TransformValue(d2scene.Identity()))
	assertAnimationKeyframe(t, transformTrack, 1, d2scene.TransformValue(d2scene.Translate(0, -4)))
	assertAnimationKeyframe(t, transformTrack, 2, d2scene.TransformValue(d2scene.Identity()))

	for index, midpoint := range animatedShapeShadowMidpoints {
		track := outer.Animations[index+1]
		assertShapeEffectTrack(t, track, d2scene.AnimateDropShadow, index)
		assertAnimationKeyframe(t, track, 0, d2scene.ShadowValue(animatedShapeShadowEndpoints[index]))
		assertAnimationKeyframe(t, track, 1, d2scene.ShadowValue(midpoint))
		assertAnimationKeyframe(t, track, 2, d2scene.ShadowValue(animatedShapeShadowEndpoints[index]))
	}
	if animatedShapeShadowMidpoints[0].SigmaX != 25.2 || animatedShapeShadowMidpoints[1].SigmaX != 15.12 {
		t.Fatal("CSS drop-shadow standard deviations were not mapped directly to scene sigma")
	}

	assertChildIDs(t, outer, []string{"effects:shape", "effects:label:0"})
	inner := outer.Children[0]
	if inner.Opacity != .5 || inner.Blend != d2scene.BlendMultiply || len(inner.Filters) != 1 {
		t.Fatalf("inner static effects = opacity %v blend %d filters %d", inner.Opacity, inner.Blend, len(inner.Filters))
	}
	if inner.Filters[0] != svgStaticShapeShadow {
		t.Fatalf("inner static filter = %#v, want svg shadow", inner.Filters[0])
	}
	// This nesting encodes the svg order: inner subtree -> static shadow ->
	// inner .5/multiply; outer subtree (including label) -> animated shadows ->
	// target opacity. Node-local clip/mask, when present, remain renderer-ordered
	// after filters and before opacity/blend.
}

func TestBuildShapeEffectsRasterPixelsAtStaticMidpointAndLoopEnd(t *testing.T) {
	t.Parallel()

	t.Run("sharp static shadow", func(t *testing.T) {
		shape := effectTestShape("shadow", d2target.ShapeRectangle, 0, 0, 20, 20)
		shape.Fill, shape.Stroke, shape.StrokeWidth, shape.Shadow = "#ffffff", "none", 0, true
		document := buildShapeEffectPixelDocument(t, shape)
		frame := renderShapeEffectFrame(t, document, 0, shapeEffectFrameOptions())
		if got := frame.NRGBAAt(102, 102); got == (color.NRGBA{R: 255, G: 255, B: 255, A: 255}) {
			t.Fatalf("static shadow pixel = %+v, want svg offset flood", got)
		}
		if got := frame.NRGBAAt(104, 102); got != (color.NRGBA{R: 255, G: 255, B: 255, A: 255}) {
			t.Fatalf("pixel beyond sharp shadow = %+v, want unblurred white background", got)
		}
	})

	t.Run("animated midpoint and loop endpoint", func(t *testing.T) {
		shape := effectTestShape("animated", d2target.ShapeRectangle, 0, 0, 20, 20)
		shape.Fill, shape.Stroke, shape.StrokeWidth, shape.Animated = "#ff0000", "none", 0, true
		document := buildShapeEffectPixelDocument(t, shape)
		options := shapeEffectFrameOptions()
		start := renderShapeEffectFrame(t, document, 0, options)
		midpoint := renderShapeEffectFrame(t, document, 500*time.Millisecond, options)
		end := renderShapeEffectFrame(t, document, time.Second, options)
		if !bytes.Equal(start.Pix, end.Pix) {
			t.Fatal("one-second repeating animation endpoint differs from its start")
		}
		white := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
		if got := start.NRGBAAt(90, 77); got != white {
			t.Fatalf("start pixel above shape = %+v, want white", got)
		}
		if got := midpoint.NRGBAAt(90, 77); got.R < 240 || got.G > 20 || got.B > 20 {
			t.Fatalf("midpoint translated fill pixel = %+v, want red shape shifted up four pixels", got)
		}
		if got := midpoint.NRGBAAt(90, 125); got == white {
			t.Fatalf("midpoint shadow pixel = %+v, want blurred animated shadow", got)
		}
	})

	t.Run("blend multiply and half opacity", func(t *testing.T) {
		shape := effectTestShape("blend", d2target.ShapeRectangle, 0, 0, 20, 20)
		shape.Fill, shape.Stroke, shape.StrokeWidth, shape.Blend = "#ff0000", "none", 0, true
		document := buildShapeEffectPixelDocument(t, shape)
		// Replace the white diagram backdrop with blue to distinguish multiply
		// from an ordinary half-opacity source-over composite.
		background := document.Root.Children[0].Primitive.(d2scene.Rect)
		background.Fill = d2scene.SolidPaint{Color: color.NRGBA{B: 255, A: 255}}
		document.Root.Children[0].Primitive = background
		frame := renderShapeEffectFrame(t, document, 0, shapeEffectFrameOptions())
		got := frame.NRGBAAt(90, 90)
		if got.R > 5 || got.G > 5 || got.B < 120 || got.B > 135 || got.A != 255 {
			t.Fatalf("multiply/.5 pixel = %+v, want half-black over blue", got)
		}
	})

	t.Run("transparent animation filter still isolates blend", func(t *testing.T) {
		shape := effectTestShape("isolated", d2target.ShapeRectangle, 0, 0, 20, 20)
		shape.Fill, shape.Stroke, shape.StrokeWidth = "#ff0000", "none", 0
		shape.Animated, shape.Blend = true, true
		document := buildShapeEffectPixelDocument(t, shape)
		background := document.Root.Children[0].Primitive.(d2scene.Rect)
		background.Fill = d2scene.SolidPaint{Color: color.NRGBA{B: 255, A: 255}}
		document.Root.Children[0].Primitive = background
		frame := renderShapeEffectFrame(t, document, 0, shapeEffectFrameOptions())
		got := frame.NRGBAAt(90, 90)
		if got.R < 120 || got.R > 135 || got.G > 5 || got.B < 120 || got.B > 135 || got.A != 255 {
			t.Fatalf("isolated animated blend pixel = %+v, want half-red source over blue", got)
		}
	})
}

func TestBuildAnimatedShapeResourceLimitsAndCancellation(t *testing.T) {
	t.Parallel()

	shape := effectTestShape("limited", d2target.ShapeRectangle, 0, 0, 20, 20)
	shape.Animated = true
	document := buildShapeEffectPixelDocument(t, shape)
	options := shapeEffectFrameOptions()
	options.Time = 500 * time.Millisecond
	if _, err := d2raster.Render(context.Background(), document, options); err != nil {
		t.Fatalf("render at exact adequate limits: %v", err)
	}
	options.MaxAnimationTracks = 2
	if _, err := d2raster.Render(context.Background(), document, options); err == nil || !strings.Contains(err.Error(), "animation track count") {
		t.Fatalf("two-track limit error = %v, want exact three-track accounting", err)
	}
	options = shapeEffectFrameOptions()
	options.Time = 500 * time.Millisecond
	options.MaxAnimationKeyframes = 8
	if _, err := d2raster.Render(context.Background(), document, options); err == nil || !strings.Contains(err.Error(), "keyframe count") {
		t.Fatalf("eight-keyframe limit error = %v, want exact nine-keyframe accounting", err)
	}
	options = shapeEffectFrameOptions()
	options.Time = 500 * time.Millisecond
	options.MaxOffscreenBytes = 1
	if _, err := d2raster.Render(context.Background(), document, options); err == nil || !strings.Contains(err.Error(), "offscreen") {
		t.Fatalf("one-byte offscreen limit error = %v, want bounded filter-layer rejection", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Build(canceled, d2target.NewDiagram(), Options{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled Build() error = %v, want context canceled", err)
	}
	if _, err := d2raster.Render(canceled, document, shapeEffectFrameOptions()); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled Render() error = %v, want context canceled", err)
	}

	diagram := d2target.NewDiagram()
	for index := 0; index < 100; index++ {
		candidate := effectTestShape("effect-"+string(rune('a'+index%26)), d2target.ShapeRectangle, index*30, 0, 20, 20)
		candidate.Animated, candidate.Shadow, candidate.Blend = true, true, true
		diagram.Shapes = append(diagram.Shapes, candidate)
	}
	// The threshold passes target indexing, preflight, asset scans, and mask
	// construction so cancellation lands while effect-bearing scene nodes are
	// being built rather than during initial input scanning.
	checkpointContext := newCancelAfterChecksContext(620)
	if _, err := Build(checkpointContext, diagram, Options{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("checkpoint-canceled effects Build() error = %v, want context canceled", err)
	}
}

func TestBuiltAnimatedShapeRendersConcurrentlyWithoutMutation(t *testing.T) {
	shape := effectTestShape("concurrent", d2target.ShapeRectangle, 0, 0, 20, 20)
	shape.Animated, shape.Shadow, shape.Blend = true, true, true
	document := buildShapeEffectPixelDocument(t, shape)
	original := fmt.Sprintf("%#v", document)
	times := []time.Duration{0, time.Second / 30, 500 * time.Millisecond, 29 * time.Second / 30}
	want := make([][]byte, len(times))
	for index, timestamp := range times {
		frame := renderShapeEffectFrame(t, document, timestamp, shapeEffectFrameOptions())
		want[index] = append([]byte(nil), frame.Pix...)
	}
	if bytes.Equal(want[0], want[2]) {
		t.Fatal("animated shape pixels did not change between start and midpoint")
	}

	var wait sync.WaitGroup
	errorsChannel := make(chan error, len(times)*3)
	for iteration := 0; iteration < 3; iteration++ {
		for index, timestamp := range times {
			wait.Add(1)
			go func(index int, timestamp time.Duration) {
				defer wait.Done()
				options := shapeEffectFrameOptions()
				options.Time = timestamp
				frame, err := d2raster.Render(context.Background(), document, options)
				if err != nil {
					errorsChannel <- err
					return
				}
				if !bytes.Equal(frame.Pix, want[index]) {
					errorsChannel <- errors.New("concurrent animated shape frame changed")
				}
			}(index, timestamp)
		}
	}
	wait.Wait()
	close(errorsChannel)
	for err := range errorsChannel {
		t.Fatal(err)
	}
	if fmt.Sprintf("%#v", document) != original {
		t.Fatal("concurrent rendering mutated the built scene")
	}
}

func TestShapeEffectsCoverCheckedInCorpusTargets(t *testing.T) {
	for _, test := range []struct {
		fixture string
		effect  func(d2target.Shape) bool
	}{
		{fixture: "stable/all_shapes_shadow/dagre/board.exp.json", effect: func(shape d2target.Shape) bool { return shape.Shadow }},
		{fixture: "txtar/shape-animate/dagre/board.exp.json", effect: func(shape d2target.Shape) bool { return shape.Animated }},
		{fixture: "stable/sequence_diagram_groups/dagre/board.exp.json", effect: func(shape d2target.Shape) bool { return shape.Blend }},
	} {
		t.Run(test.fixture, func(t *testing.T) {
			diagram := loadCodeDiagram(t, test.fixture)
			count := 0
			for _, shape := range diagram.Shapes {
				if test.effect(shape) {
					count++
				}
			}
			if count == 0 {
				t.Fatal("corpus fixture no longer exercises its expected shape effect")
			}
			document, err := Build(context.Background(), diagram, Options{})
			if err != nil {
				t.Fatalf("scene build of effect corpus fixture: %v", err)
			}
			options := patternFrameOptions()
			options.Time = 500 * time.Millisecond
			frame, err := d2raster.Render(context.Background(), document, options)
			if err != nil {
				t.Fatalf("render of effect corpus fixture: %v", err)
			}
			if frame.Bounds().Empty() {
				t.Fatal("raster effect corpus fixture rendered an empty frame")
			}
		})
	}
}

func assertShapeEffectTrack(t *testing.T, track d2scene.Track, property d2scene.AnimationProperty, targetIndex int) {
	t.Helper()
	if track.Property != property || track.TargetIndex != targetIndex || track.Delay != 0 || track.Duration != time.Second || !track.Repeat {
		t.Fatalf("track = %+v, want property %d target %d repeating over one second", track, property, targetIndex)
	}
	if len(track.Keyframes) != 3 || track.Keyframes[0].Offset != 0 || track.Keyframes[1].Offset != .5 || track.Keyframes[2].Offset != 1 {
		t.Fatalf("keyframes = %+v, want linear 0/.5/1 endpoints", track.Keyframes)
	}
}

func assertAnimationKeyframe(t *testing.T, track d2scene.Track, index int, want d2scene.AnimationValue) {
	t.Helper()
	if index < 0 || index >= len(track.Keyframes) {
		t.Fatalf("keyframe index %d outside %d keyframes", index, len(track.Keyframes))
	}
	got := track.Keyframes[index].Value
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("keyframe %d = %+v, want %+v", index, got, want)
	}
}

func sceneNodeHasFilter(node *d2scene.Node) bool {
	if node == nil {
		return false
	}
	if len(node.Filters) != 0 {
		return true
	}
	for _, child := range node.Children {
		if sceneNodeHasFilter(child) {
			return true
		}
	}
	return false
}

func buildShapeEffectPixelDocument(t *testing.T, shape d2target.Shape) *d2scene.Document {
	t.Helper()
	diagram := d2target.NewDiagram()
	diagram.Root.Fill, diagram.Root.Stroke, diagram.Root.StrokeWidth = "#ffffff", "none", 0
	diagram.Shapes = []d2target.Shape{shape}
	pad := int64(80)
	document, err := Build(context.Background(), diagram, Options{Pad: &pad})
	if err != nil {
		t.Fatal(err)
	}
	return document
}

func renderShapeEffectFrame(t *testing.T, document *d2scene.Document, timestamp time.Duration, options d2raster.FrameOptions) *image.NRGBA {
	t.Helper()
	options.Time = timestamp
	frame, err := d2raster.Render(context.Background(), document, options)
	if err != nil {
		t.Fatal(err)
	}
	return frame
}

func shapeEffectFrameOptions() d2raster.FrameOptions {
	return d2raster.FrameOptions{
		Scale: 1, Background: color.White,
		MaxWidth: 1_000, MaxHeight: 1_000, MaxPixels: 1_000_000,
		MaxNodes: 10_000, MaxDepth: 100, MaxPathCommands: 1_000_000,
		MaxAnimationTracks: 3, MaxAnimationKeyframes: 9,
		MaxAssets: 100, MaxAssetBytes: 64 << 20, MaxDecodedAssetBytes: 64 << 20, MaxImportDepth: 100,
		MaxOffscreenBytes: 64 << 20, MaxEvenOddClipWork: 1_000_000_000,
	}
}
