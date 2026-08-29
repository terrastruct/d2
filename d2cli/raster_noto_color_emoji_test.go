//go:build !js || !wasm

package d2cli

import (
	"context"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/d2scenebuild"
	"github.com/d2lang/d2/d2renderers/d2svg"
	"github.com/d2lang/d2/d2target"
)

func TestBuildSceneUsesBundledNotoColorEmojiFallback(t *testing.T) {
	diagram := notoColorEmojiDiagram()
	document, err := buildSceneWithAssets(context.Background(), diagram, d2svg.RenderOpts{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	run := findSceneText(t, document.Root, "✅ 🛡️")
	if len(run.Fallbacks) != 1 {
		t.Fatalf("font fallbacks = %#v, want one bundled face", run.Fallbacks)
	}
	asset, ok := document.Assets[run.Fallbacks[0]].(d2scene.FontAsset)
	if !ok || asset.FaceIndex != 0 || len(asset.Data) < 1_000_000 {
		t.Fatalf("bundled fallback asset = %#v", document.Assets[run.Fallbacks[0]])
	}
}

func notoColorEmojiDiagram() *d2target.Diagram {
	diagram := d2target.NewDiagram()
	family := d2fonts.HandDrawn
	diagram.FontFamily = &family
	diagram.Shapes = []d2target.Shape{{
		ID: "emoji", Type: d2target.ShapeRectangle,
		Pos: d2target.Point{X: 0, Y: 0}, Width: 120, Height: 60,
		Fill: "#ffffff", Stroke: "#000000", StrokeWidth: 1, Opacity: 1,
		Text: d2target.Text{
			Label: "✅ 🛡️", FontSize: 18, FontFamily: "default", LabelWidth: 60, LabelHeight: 24,
		},
	}}
	return diagram
}

func TestMultiBoardFontBudgetSupportsBundledNotoColorEmoji(t *testing.T) {
	// PDF, PPTX, and GIF share one export-scoped resolver across boards.
	const boards = 27
	options, err := newFontFallbackOptions(boards)
	if err != nil {
		t.Fatal(err)
	}
	var firstID d2scene.AssetID
	var firstData []byte
	for board := 0; board < boards; board++ {
		document, err := d2scenebuild.Build(context.Background(), notoColorEmojiDiagram(), d2scenebuild.Options{Fonts: options})
		if err != nil {
			t.Fatalf("board %d scene: %v", board, err)
		}
		run := findSceneText(t, document.Root, "✅ 🛡️")
		if len(run.Fallbacks) != 1 {
			t.Fatalf("board %d fallbacks = %#v, want one bundled face", board, run.Fallbacks)
		}
		id := run.Fallbacks[0]
		asset, ok := document.Assets[id].(d2scene.FontAsset)
		if !ok || len(asset.Data) < 1_000_000 {
			t.Fatalf("board %d bundled asset = %#v", board, document.Assets[id])
		}
		if board == 0 {
			firstID = id
			firstData = asset.Data
			continue
		}
		if id != firstID || &asset.Data[0] != &firstData[0] {
			t.Fatalf("board %d did not reuse the export-scoped bundled fallback", board)
		}
	}
}

func TestBundledNotoColorEmojiExhaustsConfiguredOperationBudget(t *testing.T) {
	options, err := newFontFallbackOptions(1)
	if err != nil {
		t.Fatal(err)
	}
	request := d2fonts.FallbackRequest{Runes: []rune{'\u2705'}}
	fonts, err := options.Resolver.ResolveFallbacks(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(fonts) != 1 || len(fonts[0].Data) == 0 {
		t.Fatalf("initial bundled fallback = %#v", fonts)
	}
	fontBytes := int64(len(fonts[0].Data))
	if fontBytes > fontBundledCopyBytes {
		t.Fatalf("bundled font size %d exceeds configured copy bound %d", fontBytes, fontBundledCopyBytes)
	}
	fonts, err = options.Resolver.ResolveFallbacks(context.Background(), request)
	if err == nil || !strings.Contains(err.Error(), "bytes exceed limit") || fonts != nil {
		t.Fatalf("first over-budget bundled copy result/error = %#v/%v", fonts, err)
	}
}

func findSceneText(t *testing.T, root *d2scene.Node, text string) d2scene.TextRun {
	t.Helper()
	stack := []*d2scene.Node{root}
	for len(stack) != 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if node == nil {
			continue
		}
		stack = append(stack, node.Children...)
		switch run := node.Primitive.(type) {
		case d2scene.TextRun:
			if run.Text == text {
				return run
			}
		case *d2scene.TextRun:
			if run != nil && run.Text == text {
				return *run
			}
		}
	}
	t.Fatalf("scene has no text run %q", text)
	return d2scene.TextRun{}
}
