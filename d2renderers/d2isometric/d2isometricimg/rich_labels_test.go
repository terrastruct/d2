package d2isometricimg

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image/color"
	"math"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2target"
)

func richTestStyle() labelTextStyle {
	return labelTextStyle{Width: 4, Depth: 3, FontSize: 16, PixelScale: .01, Color: color.NRGBA{R: 25, G: 45, B: 65, A: 255}, Opacity: 1}
}
func richRuns(doc *d2scene.Document) []d2scene.TextRun {
	var runs []d2scene.TextRun
	var walk func(*d2scene.Node)
	walk = func(n *d2scene.Node) {
		if run, ok := n.Primitive.(d2scene.TextRun); ok {
			runs = append(runs, run)
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(doc.Root)
	return runs
}

func TestRichMarkdownPreservesFormattingAndLongContent(t *testing.T) {
	source := "# Release plan\n\n**Important** and *careful*.\n\n- First step\n- Second step\n\n`Build()` is ready.\n\n" + strings.Repeat("This paragraph retains all of its words. ", 8) + "THE END"
	s := d2target.Shape{Text: d2target.Text{Label: source, Language: "md", FontSize: 16, LabelWidth: 700, LabelHeight: 600}}
	before, _ := json.Marshal(s)
	doc, err := richLabelDocument(context.Background(), s, richTestStyle())
	if err != nil {
		t.Fatal(err)
	}
	var text strings.Builder
	heading, bold, italic, mono := false, false, false, false
	for _, run := range richRuns(doc) {
		text.WriteString(run.Text)
		if run.Text == "Release plan" && run.Font.Size > 16 {
			heading = true
		}
		if run.Text == "Important" && run.Font.Weight >= 600 {
			bold = true
		}
		if run.Text == "careful" && strings.Contains(run.Font.Style, "italic") {
			italic = true
		}
		if run.Text == "Build()" && run.Font.Family == "SourceCodePro" {
			mono = true
		}
	}
	if !heading || !bold || !italic || !mono || !strings.Contains(text.String(), "THE END") || strings.Contains(text.String(), "**") {
		t.Fatalf("rich formatting missing: heading=%v bold=%v italic=%v mono=%v text=%q", heading, bold, italic, mono, text.String())
	}
	after, _ := json.Marshal(s)
	if !bytes.Equal(before, after) {
		t.Fatal("rich renderer mutated source")
	}
	p, _ := newRichLabelPainter(context.Background(), 1)
	tex, err := p.texture(s, richTestStyle())
	if err != nil {
		t.Fatal(err)
	}
	if tex.Bounds().Dx() < 500 || tex.Bounds().Dy() < 500 {
		t.Fatal("rich label resolution was reduced to a plain-text patch")
	}
	visible := 0
	for i := 3; i < len(tex.Pix); i += 4 {
		if tex.Pix[i] > 0 {
			visible++
		}
	}
	if visible < 100 {
		t.Fatal("rich texture blank")
	}
}

func TestRichCodeUsesSyntaxAndDarkPalette(t *testing.T) {
	s := d2target.Shape{Text: d2target.Text{Language: "go", Label: "package main\n\nfunc main() {\n  println(\"ready\")\n}", FontSize: 16, LabelWidth: 260, LabelHeight: 120}}
	light, err := richLabelDocument(context.Background(), s, richTestStyle())
	if err != nil {
		t.Fatal(err)
	}
	style := richTestStyle()
	style.Color = color.NRGBA{245, 248, 252, 255}
	dark, err := richLabelDocument(context.Background(), s, style)
	if err != nil {
		t.Fatal(err)
	}
	colors := map[color.NRGBA]bool{}
	darkColors := map[color.NRGBA]bool{}
	for _, run := range richRuns(light) {
		if paint, ok := run.Fill.(d2scene.SolidPaint); ok {
			colors[paint.Color] = true
		}
		if run.Font.Family != "SourceCodePro" {
			t.Fatal("code lost mono font")
		}
	}
	for _, run := range richRuns(dark) {
		if paint, ok := run.Fill.(d2scene.SolidPaint); ok {
			darkColors[paint.Color] = true
		}
	}
	if len(colors) < 3 || len(darkColors) < 3 {
		t.Fatalf("syntax colors absent: %v / %v", colors, darkColors)
	}
	a, _ := json.Marshal(colorsAsStrings(colors))
	b, _ := json.Marshal(colorsAsStrings(darkColors))
	if bytes.Equal(a, b) {
		t.Fatal("dark face retained light syntax palette")
	}
}
func colorsAsStrings(m map[color.NRGBA]bool) map[string]bool {
	out := map[string]bool{}
	for c := range m {
		out[richColor(c)] = true
	}
	return out
}

func TestRichStructuredRowsAndOwnership(t *testing.T) {
	for _, typ := range []string{d2target.ShapeSQLTable, d2target.ShapeClass} {
		s := d2target.Shape{Type: typ, Width: 340, Height: 160, Text: d2target.Text{Label: "Account", FontSize: 16, LabelWidth: 80, LabelHeight: 20},
			Class:    d2target.Class{Fields: []d2target.ClassField{{Name: "owner", Type: "string", Visibility: "private"}}, Methods: []d2target.ClassMethod{{Name: "save()", Return: "error"}}},
			SQLTable: d2target.SQLTable{Columns: []d2target.SQLColumn{{Name: d2target.Text{Label: "owner_id", LabelWidth: 80}, Type: d2target.Text{Label: "uuid", LabelWidth: 40}, Constraint: []string{"primary_key", "foreign_key"}}}}}
		before, _ := json.Marshal(s)
		doc, err := richLabelDocument(context.Background(), s, richTestStyle())
		if err != nil {
			t.Fatal(err)
		}
		var text strings.Builder
		for _, run := range richRuns(doc) {
			text.WriteString(run.Text)
			text.WriteByte(' ')
		}
		want := []string{"Account", "owner_id", "uuid", "PK, FK"}
		if typ == d2target.ShapeClass {
			want = []string{"Account", "owner", "string", "save()", "error", "-"}
		}
		for _, value := range want {
			if !strings.Contains(text.String(), value) {
				t.Fatalf("%s lost %q: %q", typ, value, text.String())
			}
		}
		if doc.ViewBox.Width != 340 || doc.ViewBox.Height != 160 {
			t.Fatal("structured source dimensions changed")
		}
		after, _ := json.Marshal(s)
		if !bytes.Equal(before, after) {
			t.Fatal("structured source mutated")
		}
	}
}

func TestRichOpacityBudgetsAndCancellation(t *testing.T) {
	s := d2target.Shape{Text: d2target.Text{Label: "**Readable**", Language: "markdown", LabelWidth: 140, LabelHeight: 40, FontSize: 16}}
	style := richTestStyle()
	style.Opacity = .3
	p, _ := newRichLabelPainter(context.Background(), 1)
	tex, err := p.texture(s, style)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < len(tex.Pix); i += 4 {
		if tex.Pix[i+3] > 77 || tex.Pix[i] > tex.Pix[i+3] || tex.Pix[i+1] > tex.Pix[i+3] || tex.Pix[i+2] > tex.Pix[i+3] {
			t.Fatal("opacity or premultiplied texture invariant violated")
		}
	}
	if _, err = p.texture(s, style); err == nil {
		t.Fatal("declared label count exceeded")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = newRichLabelPainter(ctx, 1); !errors.Is(err, context.Canceled) {
		t.Fatal("cancel not observed")
	}
	p, _ = newRichLabelPainter(context.Background(), 1)
	s.Label = strings.Repeat("x", maxRichLabelBytes+1)
	if _, err = p.texture(s, style); err == nil {
		t.Fatal("unbounded source accepted")
	}
	s.Label = "x"
	style.Width = math.Inf(1)
	if _, err = p.texture(s, style); err == nil {
		t.Fatal("non-finite surface accepted")
	}
	s.Type = d2target.ShapeSQLTable
	s.Columns = make([]d2target.SQLColumn, maxRichRows+1)
	if _, _, err = richSourceSize(s); err == nil {
		t.Fatal("unbounded structured rows accepted")
	}
	if !isRichLabel(d2target.Shape{Type: d2target.ShapeCode, Text: d2target.Text{Language: "latex"}}) {
		t.Fatal("LaTeX must use the native rich pipeline")
	}
}

func TestRichSurfaceFittingDoesNotEnlargeSourceText(t *testing.T) {
	doc := d2scene.NewDocument(d2scene.Box{Width: 100, Height: 20}, d2scene.NewNode(nil))
	style := richTestStyle()
	if err := fitRichViewport(doc, style); err != nil {
		t.Fatal(err)
	}
	if doc.ViewBox != (d2scene.Box{X: -150, Y: -140, Width: 400, Height: 300}) {
		t.Fatalf("authored font scale lost in extra component space: %+v", doc.ViewBox)
	}
	doc.ViewBox = d2scene.Box{Width: 800, Height: 200}
	if err := fitRichViewport(doc, style); err != nil {
		t.Fatal(err)
	}
	if doc.ViewBox.Width < 800 || doc.ViewBox.Height < 200 || math.Abs(doc.ViewBox.Width/doc.ViewBox.Height-style.Width/style.Depth) > 1e-9 {
		t.Fatalf("rich content clipped or stretched: %+v", doc.ViewBox)
	}
}
