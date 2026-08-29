package d2scenebuild

import (
	"bytes"
	"context"
	"encoding/json"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"

	"github.com/d2lang/d2/d2renderers/d2raster"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/d2themes/d2themescatalog"
)

func TestBuildCodeShapeMatchesCheckedInGoTarget(t *testing.T) {
	t.Parallel()

	diagram := loadCodeDiagram(t, "stable/code_snippet/dagre/board.exp.json")
	before, err := json.Marshal(diagram)
	if err != nil {
		t.Fatal(err)
	}
	document := buildCodeDocument(t, diagram, nil)
	after, err := json.Marshal(diagram)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("code shape build mutated the checked-in target")
	}

	node := mustFindCodeNode(t, document.Root, "hey")
	if len(node.Children) != 36 {
		t.Fatalf("code children = %d, want background + 35 visible Chroma tokens", len(node.Children))
	}
	background, ok := node.Children[0].Primitive.(d2scene.Rect)
	if !ok {
		t.Fatalf("code background = %T, want Rect", node.Children[0].Primitive)
	}
	if node.Children[0].ID != "hey:code-background" || background.Box != (d2scene.Box{X: 0, Y: 166, Width: 755, Height: 203}) {
		t.Fatalf("code background = %q %+v", node.Children[0].ID, background.Box)
	}
	assertSolidCodeColor(t, background.Fill, color.NRGBA{R: 0xf7, G: 0xf7, B: 0xf7, A: 0xff})

	comment := mustFindCodeNode(t, node, "hey:code-line:0:token:0").Primitive.(d2scene.TextRun)
	if comment.Text != "// RegisterHash registers a function that returns a new instance of the given" ||
		comment.Origin != (d2scene.Point{X: 8, Y: 190}) || comment.Anchor != d2scene.AnchorStart ||
		comment.Font.Family != "SourceCodePro" || comment.Font.Style != "regular" || comment.Font.Size != 16 {
		t.Fatalf("first code token = %+v", comment)
	}
	assertSolidCodeColor(t, comment.Fill, color.NRGBA{R: 0x57, G: 0x60, B: 0x6a, A: 0xff})
	if comment.Ink != d2scene.NewBounds(8, 174, 746.71875, 194.8) {
		t.Fatalf("first code token ink = %+v", comment.Ink)
	}

	function := mustFindCodeNode(t, node, "hey:code-line:3:token:2").Primitive.(d2scene.TextRun)
	if function.Text != "RegisterHash" || function.Origin != (d2scene.Point{X: 55.96875, Y: 252.4}) {
		t.Fatalf("function token geometry = %+v", function)
	}
	assertSolidCodeColor(t, function.Fill, color.NRGBA{R: 0x66, G: 0x39, B: 0xba, A: 0xff})

	// Source tabs expand to four cells.
	ifKeyword := mustFindCodeNode(t, node, "hey:code-line:4:token:1").Primitive.(d2scene.TextRun)
	if ifKeyword.Text != "if" || ifKeyword.Origin != (d2scene.Point{X: 46.375, Y: 273.2}) {
		t.Fatalf("tab-expanded keyword = %+v", ifKeyword)
	}
	if asset, ok := document.Assets[comment.Font.Asset].(d2scene.FontAsset); !ok || len(asset.Data) == 0 {
		t.Fatalf("code font asset %q is missing", comment.Font.Asset)
	}
}

func TestBuildCodeShapeSelectsDarkChromaTheme(t *testing.T) {
	t.Parallel()

	diagram := loadCodeDiagram(t, "stable/code_snippet/dagre/board.exp.json")
	themeID := d2themescatalog.DarkMauve.ID
	document := buildCodeDocument(t, diagram, &themeID)
	node := mustFindCodeNode(t, document.Root, "hey")
	if len(node.Children) != 36 {
		t.Fatalf("dark code children = %d, want one active background + 35 tokens", len(node.Children))
	}
	background := node.Children[0].Primitive.(d2scene.Rect)
	assertSolidCodeColor(t, background.Fill, color.NRGBA{R: 0x1e, G: 0x1e, B: 0x2e, A: 0xff})

	comment := mustFindCodeNode(t, node, "hey:code-line:0:token:0").Primitive.(d2scene.TextRun)
	if comment.Font.Style != "italic" || comment.Font.Family != "SourceCodePro" {
		t.Fatalf("dark comment font = %+v, want mono italic", comment.Font)
	}
	assertSolidCodeColor(t, comment.Fill, color.NRGBA{R: 0x6c, G: 0x70, B: 0x86, A: 0xff})
	keyword := mustFindCodeNode(t, node, "hey:code-line:3:token:0").Primitive.(d2scene.TextRun)
	assertSolidCodeColor(t, keyword.Fill, color.NRGBA{R: 0xf3, G: 0x8b, B: 0xa8, A: 0xff})
	function := mustFindCodeNode(t, node, "hey:code-line:3:token:2").Primitive.(d2scene.TextRun)
	assertSolidCodeColor(t, function.Fill, color.NRGBA{R: 0x89, G: 0xb4, B: 0xfa, A: 0xff})
	operator := mustFindCodeNode(t, node, "hey:code-line:4:token:5").Primitive.(d2scene.TextRun)
	if operator.Text != ">=" || operator.Font.Style != "bold" {
		t.Fatalf("dark operator font = %+v for %q, want mono bold", operator.Font, operator.Text)
	}
	assertSolidCodeColor(t, operator.Fill, color.NRGBA{R: 0x89, G: 0xdc, B: 0xeb, A: 0xff})
}

func TestBuildCodeShapeFallbackLexerAndInitialFill(t *testing.T) {
	t.Parallel()

	diagram := loadCodeDiagram(t, "regression/no-lexer/dagre/board.exp.json")
	diagram.Shapes[0].StrokeDash = 5
	document := buildCodeDocument(t, diagram, nil)
	node := mustFindCodeNode(t, document.Root, "x")
	if len(node.Children) != 2 {
		t.Fatalf("fallback code children = %d, want background + one text run", len(node.Children))
	}
	background := node.Children[0].Primitive.(d2scene.Rect)
	if background.Stroke == nil || len(background.Stroke.Dashes) != 0 {
		t.Fatalf("code background stroke = %+v, want dash-free stroke", background.Stroke)
	}
	run := node.Children[1].Primitive.(d2scene.TextRun)
	if run.Text != "x -> y" || run.Origin != (d2scene.Point{X: 8, Y: 24}) || run.Ink != d2scene.NewBounds(8, 8, 65.5625, 28.8) {
		t.Fatalf("fallback code run = %+v", run)
	}
	// GitHub's fallback token has no foreground attribute; the SVG initial
	// fill is black, not the diagram's theme font color.
	assertSolidCodeColor(t, run.Fill, color.NRGBA{A: 0xff})
}

func TestBuildCodeShapePreservesLeadingAndTrailingLineGeometry(t *testing.T) {
	t.Parallel()

	diagram := loadCodeDiagram(t, "regression/code_leading_trailing_newlines/dagre/board.exp.json")
	document := buildCodeDocument(t, diagram, nil)
	node := mustFindCodeNode(t, document.Root, "hello world")
	if findOptionalSceneNode(node, "hello world:code-line:0:token:0") != nil || findOptionalSceneNode(node, "hello world:code-line:1:token:0") != nil {
		t.Fatal("leading blank code lines unexpectedly produced paint nodes")
	}
	commentNode := findOptionalSceneNode(node, "hello world:code-line:2:token:0")
	if commentNode == nil {
		t.Fatal("first visible token did not retain line index 2")
	}
	comment := commentNode.Primitive.(d2scene.TextRun)
	if comment.Origin != (d2scene.Point{X: 8, Y: 65.6}) {
		t.Fatalf("leading-line baseline = %+v, want x=8 y=65.6", comment.Origin)
	}
	for _, child := range node.Children[1:] {
		if strings.Contains(child.ID, ":code-line:7:") || strings.Contains(child.ID, ":code-line:8:") {
			t.Fatalf("trailing newline emitted a visible node %q", child.ID)
		}
	}
}

func TestBuildCodeShapeWithoutLanguageMatchesPlainLabelBranch(t *testing.T) {
	t.Parallel()

	diagram := d2target.NewDiagram()
	diagram.Root.Fill, diagram.Root.Stroke = "#fff", "none"
	diagram.Shapes = []d2target.Shape{{
		ID: "plain-code", Type: d2target.ShapeCode,
		Pos: d2target.Point{X: 10, Y: 20}, Width: 100, Height: 40,
		Fill: "#f00", Stroke: "#00f", StrokeWidth: 2, Opacity: 1,
		Text: d2target.Text{Label: "plain", FontSize: 16, FontFamily: "DEFAULT", Bold: true, LabelWidth: 40, LabelHeight: 19},
	}}
	document := buildCodeDocument(t, diagram, nil)
	node := mustFindCodeNode(t, document.Root, "plain-code")
	if len(node.Children) != 1 {
		t.Fatalf("language-free code children = %d, want plain label only", len(node.Children))
	}
	run, ok := node.Children[0].Primitive.(d2scene.TextRun)
	if !ok || run.Text != "plain" || run.Font.Family != "SourceSansPro" || run.Font.Style != "bold" {
		t.Fatalf("language-free code primitive = %#v", node.Children[0].Primitive)
	}
}

func TestBuildCodeShapeRejectsNonPositiveTextSize(t *testing.T) {
	t.Parallel()

	diagram := loadCodeDiagram(t, "regression/no-lexer/dagre/board.exp.json")
	diagram.Shapes[0].FontSize = 0
	_, err := Build(context.Background(), diagram, Options{})
	if err == nil || !strings.Contains(err.Error(), `shape "x"`) || !strings.Contains(err.Error(), `field "fontSize"`) {
		t.Fatalf("Build() error = %v, want object-scoped code font-size error", err)
	}
}

func TestBuildCodeShapeRejectsInvalidOrOversizedSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		label string
		want  string
	}{
		{name: "invalid UTF-8", label: string([]byte{0xff}), want: "must be valid UTF-8"},
		{name: "source bytes", label: strings.Repeat("x", maxCodeSourceBytes+1), want: "code source bytes must not exceed"},
		{name: "source runes", label: strings.Repeat("x", maxCodeSourceRunes+1), want: "code source runes must not exceed"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			diagram := loadCodeDiagram(t, "regression/no-lexer/dagre/board.exp.json")
			diagram.Shapes[0].Label = test.label
			_, err := Build(context.Background(), diagram, Options{})
			if err == nil || !strings.Contains(err.Error(), `shape "x"`) || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Build() error = %v, want object-scoped %q error", err, test.want)
			}
		})
	}
}

func TestBuildCodeShapeWithEmptyLabelPaintsNothing(t *testing.T) {
	t.Parallel()

	diagram := loadCodeDiagram(t, "regression/no-lexer/dagre/board.exp.json")
	diagram.Shapes[0].Label = ""
	diagram.Shapes[0].FontSize = 0
	document := buildCodeDocument(t, diagram, nil)
	node := mustFindCodeNode(t, document.Root, "x")
	if len(node.Children) != 0 {
		t.Fatalf("empty code shape children = %d, want no background or label paint", len(node.Children))
	}
	if len(document.Assets) != 0 {
		t.Fatalf("empty code shape assets = %d, want no font assets", len(document.Assets))
	}
}

func TestBuildSyntaxHighlightedLabelOnOrdinaryShape(t *testing.T) {
	t.Parallel()

	diagram := validDiagram()
	shape := &diagram.Shapes[0]
	shape.Type = d2target.ShapeRectangle
	shape.Width, shape.Height = 180, 80
	shape.Text = d2target.Text{
		Label: "package main", Language: "go", FontSize: 16,
		LabelWidth: 120, LabelHeight: 21,
	}
	document, err := Build(context.Background(), diagram, Options{})
	if err != nil {
		t.Fatal(err)
	}
	group := mustFindCodeNode(t, document.Root, "a")
	if len(group.Children) < 3 {
		t.Fatalf("ordinary syntax shape children = %d, want outline, code background, and tokens", len(group.Children))
	}
	background := mustFindCodeNode(t, group, "a:code-background")
	if _, ok := background.Primitive.(d2scene.Rect); !ok {
		t.Fatalf("ordinary syntax background = %T, want Rect", background.Primitive)
	}
	keyword := mustFindCodeNode(t, group, "a:code-line:0:token:0").Primitive.(d2scene.TextRun)
	if keyword.Text != "package" || keyword.Origin != (d2scene.Point{X: 8, Y: 24}) || keyword.Font.Family != "SourceCodePro" {
		t.Fatalf("ordinary syntax keyword = %+v", keyword)
	}
	if _, err := d2raster.Render(context.Background(), document, markdownRasterOptions()); err != nil {
		t.Fatalf("raster ordinary syntax label: %v", err)
	}
}

func TestBuildSyntaxHighlightedConnectionLabel(t *testing.T) {
	t.Parallel()

	diagram := validDiagram()
	connection := &diagram.Connections[0]
	connection.Text = d2target.Text{
		Label: "if ready:\n    run()", Language: "python", FontSize: 16,
		LabelWidth: 150, LabelHeight: 42,
	}
	connection.LabelPosition = "INSIDE_MIDDLE_CENTER"
	connection.LabelPercentage = .5
	connection.Fill = "#ff0000"
	document, err := Build(context.Background(), diagram, Options{})
	if err != nil {
		t.Fatal(err)
	}
	group := mustFindCodeNode(t, document.Root, "a-b")
	keyword := mustFindCodeNode(t, group, "a-b:code-line:0:token:0").Primitive.(d2scene.TextRun)
	topLeft := connection.GetLabelTopLeft()
	if keyword.Text != "if" || keyword.Origin != (d2scene.Point{X: math.Round(topLeft.X), Y: math.Round(topLeft.Y) + 16}) {
		t.Fatalf("connection syntax keyword = %+v at target top-left %+v", keyword, topLeft)
	}
	if findOptionalSceneNode(group, "a-b:label-fill") != nil {
		t.Fatal("syntax-highlighted connection label incorrectly emitted ordinary label fill")
	}
}

func TestBuildCodeShapeRejectsRichLabelLanguages(t *testing.T) {
	t.Parallel()

	for _, language := range []string{"markdown", "latex"} {
		language := language
		t.Run(language, func(t *testing.T) {
			t.Parallel()
			diagram := loadCodeDiagram(t, "regression/no-lexer/dagre/board.exp.json")
			diagram.Shapes[0].Language = language
			_, err := Build(context.Background(), diagram, Options{})
			if err == nil || !strings.Contains(err.Error(), `shape "x"`) || !strings.Contains(err.Error(), "label language "+language) {
				t.Fatalf("Build() error = %v, want explicit unsupported %s error", err, language)
			}
		})
	}
}

func TestCollectCodeTokensReturnsCancellationAndLexerPanics(t *testing.T) {
	t.Parallel()

	t.Run("cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		calls := 0
		iterator := chroma.Iterator(func() chroma.Token {
			calls++
			cancel()
			return chroma.Token{Type: chroma.Text, Value: "x"}
		})
		budget := newCodeTokenBudget(0)
		tokens, err := collectCodeTokens(ctx, iterator, &budget)
		if err != context.Canceled || tokens != nil || calls != 1 {
			t.Fatalf("collectCodeTokens() = %#v, %v after %d calls, want nil, canceled after one", tokens, err, calls)
		}
	})

	t.Run("cancellation while counting one large token", func(t *testing.T) {
		ctx := &cancelAfterCodeErrChecksContext{Context: context.Background(), cancelAt: 4}
		budget := newCodeTokenBudget(0)
		iterator := chroma.Literator(chroma.Token{Type: chroma.Text, Value: strings.Repeat("x", 8192)})
		tokens, err := collectCodeTokens(ctx, iterator, &budget)
		if err != context.Canceled || tokens != nil || budget.rawTokens != 0 || budget.expandedPieces != 0 {
			t.Fatalf("collectCodeTokens() = %#v, %v with budget %+v, want cancellation before accounting", tokens, err, budget)
		}
	})

	t.Run("panic", func(t *testing.T) {
		iterator := chroma.Iterator(func() chroma.Token { panic("broken lexer") })
		budget := newCodeTokenBudget(0)
		tokens, err := collectCodeTokens(context.Background(), iterator, &budget)
		if err == nil || tokens != nil || !strings.Contains(err.Error(), "lexer panic: broken lexer") {
			t.Fatalf("collectCodeTokens() = %#v, %v, want explicit lexer panic", tokens, err)
		}
	})
}

func TestCollectCodeTokensEnforcesBuildBudget(t *testing.T) {
	t.Parallel()

	t.Run("raw tokens accumulate across labels", func(t *testing.T) {
		budget := newCodeTokenBudget(2)
		if budget.maxRawTokens != 2 || budget.maxExpandedPieces != 2 {
			t.Fatalf("newCodeTokenBudget(2) = %+v, want MaxNodes-derived ceilings", budget)
		}
		first := chroma.Literator(
			chroma.Token{Type: chroma.Text, Value: "a"},
			chroma.Token{Type: chroma.Text, Value: "b"},
		)
		if tokens, err := collectCodeTokens(context.Background(), first, &budget); err != nil || len(tokens) != 2 {
			t.Fatalf("first collectCodeTokens() = %#v, %v", tokens, err)
		}
		second := chroma.Literator(chroma.Token{Type: chroma.Text, Value: "c"})
		if tokens, err := collectCodeTokens(context.Background(), second, &budget); err == nil || tokens != nil || !strings.Contains(err.Error(), "raw lexer token count exceeds limit 2") {
			t.Fatalf("second collectCodeTokens() = %#v, %v, want cumulative raw-token limit", tokens, err)
		}
	})

	t.Run("newline expansion is rejected before allocation", func(t *testing.T) {
		budget := codeTokenBudget{maxRawTokens: 10, maxExpandedPieces: 2}
		iterator := chroma.Literator(chroma.Token{Type: chroma.Text, Value: "a\nb\nc"})
		tokens, err := collectCodeTokens(context.Background(), iterator, &budget)
		if err == nil || tokens != nil || !strings.Contains(err.Error(), "newline-expanded token piece count exceeds limit 2") {
			t.Fatalf("collectCodeTokens() = %#v, %v, want expanded-token limit", tokens, err)
		}
		if budget.rawTokens != 0 || budget.expandedPieces != 0 {
			t.Fatalf("failed token changed budget to %+v", budget)
		}
	})
}

func TestTokeniseCodeRecoversImmediateLexerPanic(t *testing.T) {
	t.Parallel()

	iterator, err := tokeniseCode(panickingCodeLexer{}, "x")
	if err == nil || iterator != nil || !strings.Contains(err.Error(), "lexer panic: broken tokenise") {
		t.Fatalf("tokeniseCode() = %v, %v, want explicit lexer panic", iterator, err)
	}
}

func TestCodeTokenPaintsUnderlinedWhitespace(t *testing.T) {
	t.Parallel()

	if codeTokenPaintsInk(" \t", chroma.StyleEntry{}) {
		t.Fatal("plain whitespace unexpectedly paints ink")
	}
	if !codeTokenPaintsInk(" \t", chroma.StyleEntry{Underline: chroma.Yes}) {
		t.Fatal("underlined whitespace must retain its decoration run")
	}
	if !codeTokenPaintsInk("x", chroma.StyleEntry{}) {
		t.Fatal("non-whitespace token unexpectedly omitted")
	}
}

type panickingCodeLexer struct{}

type cancelAfterCodeErrChecksContext struct {
	context.Context
	cancelAt int
	checks   int
}

func (c *cancelAfterCodeErrChecksContext) Err() error {
	c.checks++
	if c.checks >= c.cancelAt {
		return context.Canceled
	}
	return nil
}

func (panickingCodeLexer) Config() *chroma.Config { return &chroma.Config{Name: "panicking"} }
func (panickingCodeLexer) Tokenise(*chroma.TokeniseOptions, string) (chroma.Iterator, error) {
	panic("broken tokenise")
}
func (lexer panickingCodeLexer) SetRegistry(*chroma.LexerRegistry) chroma.Lexer { return lexer }
func (lexer panickingCodeLexer) SetAnalyser(func(string) float32) chroma.Lexer  { return lexer }
func (panickingCodeLexer) AnalyseText(string) float32                           { return 0 }

func loadCodeDiagram(t *testing.T, fixture string) *d2target.Diagram {
	t.Helper()
	path := filepath.Join("..", "..", "e2etests", "testdata", filepath.FromSlash(fixture))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var diagram d2target.Diagram
	if err := json.Unmarshal(data, &diagram); err != nil {
		t.Fatal(err)
	}
	return &diagram
}

func buildCodeDocument(t *testing.T, diagram *d2target.Diagram, themeID *int64) *d2scene.Document {
	t.Helper()
	pad := int64(0)
	document, err := Build(context.Background(), diagram, Options{Pad: &pad, ThemeID: themeID})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	return document
}

func findOptionalSceneNode(root *d2scene.Node, id string) *d2scene.Node {
	if root == nil {
		return nil
	}
	if root.ID == id {
		return root
	}
	for _, child := range root.Children {
		if found := findOptionalSceneNode(child, id); found != nil {
			return found
		}
	}
	return nil
}

func mustFindCodeNode(t *testing.T, root *d2scene.Node, id string) *d2scene.Node {
	t.Helper()
	if node := findOptionalSceneNode(root, id); node != nil {
		return node
	}
	t.Fatalf("scene node %q not found", id)
	return nil
}

func assertSolidCodeColor(t *testing.T, paint d2scene.Paint, want color.NRGBA) {
	t.Helper()
	solid, ok := paint.(d2scene.SolidPaint)
	if !ok || solid.Color != want {
		t.Fatalf("paint = %#v, want solid %v", paint, want)
	}
}
