package d2svgimport

import (
	"context"
	"errors"
	"image/color"
	"reflect"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2scene"
)

func TestImportNodeStylesheetCascadeGlobalOrderAndClasses(t *testing.T) {
	result := mustImport(t, `<svg width="20" height="10" fill="#aa0000">
  <style type=" Text/CSS ">
    .base { fill: #010203; stroke: #040506; stroke-width: 2; opacity: .5; }
    .first { fill: #111213; }
  </style>
  <g id="group" class="base">
    <rect id="sheet" class=" first&#x9;base&#xA;first " fill="#ff0000" width="4" height="4"/>
    <rect id="inline" class="first base" x="5" width="4" height="4" fill="#ff0000" style="fill:#aabbcc;stroke-width:4"/>
    <rect id="inherited" x="10" width="4" height="4"/>
  </g>
  <style><![CDATA[
    .first { fill: #070809; }
    .base { fill: #0a0b0c; stroke-width: 3; }
  ]]></style>
</svg>`)

	// Both style elements are non-rendering. The second block occurs after the
	// shapes, but still participates in the global author cascade.
	if len(result.Root.Children) != 1 || result.Root.Children[0].ID != "group" {
		t.Fatalf("root children = %#v", result.Root.Children)
	}
	group := result.Root.Children[0]
	if !reflect.DeepEqual(group.Classes, []string{"base"}) || len(group.Children) != 3 {
		t.Fatalf("group classes/children = %#v/%d", group.Classes, len(group.Children))
	}

	sheetNode := group.Children[0]
	if !reflect.DeepEqual(sheetNode.Classes, []string{"first", "base"}) || sheetNode.Opacity != .5 {
		t.Fatalf("sheet classes/opacity = %#v/%v", sheetNode.Classes, sheetNode.Opacity)
	}
	sheet := sheetNode.Primitive.(d2scene.Rect)
	// Presentation fill loses to class rules. Class attribute token order does
	// not affect the result; the later .base rule wins by stylesheet order.
	assertSolidColor(t, sheet.Fill, color.NRGBA{R: 0x0a, G: 0x0b, B: 0x0c, A: 0xff})
	assertSolidColor(t, sheet.Stroke.Paint, color.NRGBA{R: 4, G: 5, B: 6, A: 0xff})
	if sheet.Stroke.Width != 3 {
		t.Fatalf("stylesheet stroke width = %v", sheet.Stroke.Width)
	}

	inlineNode := group.Children[1]
	inline := inlineNode.Primitive.(d2scene.Rect)
	assertSolidColor(t, inline.Fill, color.NRGBA{R: 0xaa, G: 0xbb, B: 0xcc, A: 0xff})
	if inline.Stroke == nil || inline.Stroke.Width != 4 || inlineNode.Opacity != .5 {
		t.Fatalf("inline cascade = %#v, opacity %v", inline.Stroke, inlineNode.Opacity)
	}

	// The class rule on the group inherits to an unclassified descendant;
	// opacity remains non-inherited.
	inheritedNode := group.Children[2]
	inherited := inheritedNode.Primitive.(d2scene.Rect)
	assertSolidColor(t, inherited.Fill, color.NRGBA{R: 0x0a, G: 0x0b, B: 0x0c, A: 0xff})
	if inherited.Stroke == nil || inherited.Stroke.Width != 3 || inheritedNode.Opacity != 1 {
		t.Fatalf("inherited stylesheet = %#v, opacity %v", inherited.Stroke, inheritedNode.Opacity)
	}
	if result.Metrics.ParsedElements != 7 || result.Metrics.EmittedElements != 5 {
		t.Fatalf("stylesheet element metrics = %+v", result.Metrics)
	}
}

func TestImportNodeStylesheetSelectorMatchingIsCaseSensitive(t *testing.T) {
	result := mustImport(t, `<svg width="2" height="1"><style>.Paint{fill:red}</style><rect class="paint" width="1" height="1"/><rect class="Paint" x="1" width="1" height="1"/></svg>`)
	first := result.Root.Children[0].Primitive.(d2scene.Rect)
	second := result.Root.Children[1].Primitive.(d2scene.Rect)
	assertSolidColor(t, first.Fill, color.NRGBA{A: 0xff})
	assertSolidColor(t, second.Fill, color.NRGBA{R: 0xff, A: 0xff})
}

func TestClassTokensUseOnlySVGWhitespaceAndPreserveOrder(t *testing.T) {
	importer := svgImporter{ctx: context.Background(), source: "classes.svg", limits: generousImportLimits()}
	classes, classSet, err := importer.classTokens(&svgElement{name: "rect", attrs: map[string]string{
		"class": " alpha\tbeta\ngamma\ralpha\fpunctuated:name ",
	}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"alpha", "beta", "gamma", "punctuated:name"}
	if !reflect.DeepEqual(classes, want) || len(classSet) != len(want) {
		t.Fatalf("classes/set = %#v/%#v, want %#v", classes, classSet, want)
	}
	for _, class := range want {
		if _, ok := classSet[class]; !ok {
			t.Fatalf("class set is missing %q", class)
		}
	}
}

func TestImportNodeRejectsUnsupportedStylesheets(t *testing.T) {
	tests := []struct {
		name, source, want string
	}{
		{"type", `<svg width="1" height="1"><style type="text/plain">.a{fill:red}</style></svg>`, "unsupported type"},
		{"style-attribute", `<svg width="1" height="1"><style class="a">.a{fill:red}</style></svg>`, `unsupported attribute "class"`},
		{"style-child", `<svg width="1" height="1"><style>.a{fill:red}<g/></style></svg>`, "cannot contain child"},
		{"at-rule", `<svg width="1" height="1"><style>@media screen {.a{fill:red}}</style></svg>`, "at-rules"},
		{"id-selector", `<svg width="1" height="1"><style>#a{fill:red}</style></svg>`, "unsupported stylesheet selector"},
		{"compound-selector", `<svg width="1" height="1"><style>.a.b{fill:red}</style></svg>`, "unsupported stylesheet selector"},
		{"descendant-selector", `<svg width="1" height="1"><style>.a .b{fill:red}</style></svg>`, "unsupported stylesheet selector"},
		{"comma-selector", `<svg width="1" height="1"><style>.a,.b{fill:red}</style></svg>`, "unsupported stylesheet selector"},
		{"pseudo-selector", `<svg width="1" height="1"><style>.a:hover{fill:red}</style></svg>`, "unsupported stylesheet selector"},
		{"attribute-selector", `<svg width="1" height="1"><style>.a[x]{fill:red}</style></svg>`, "unsupported stylesheet selector"},
		{"css-comment", `<svg width="1" height="1"><style>/* no */.a{fill:red}</style></svg>`, "CSS comments"},
		{"xml-comment", `<svg width="1" height="1"><style>.a{fill:red}<!-- no --></style></svg>`, "CSS comments"},
		{"important", `<svg width="1" height="1"><style>.a{fill:red ! important}</style></svg>`, "!important"},
		{"unknown-property-unused", `<svg width="1" height="1"><style>.unused{filter:none}</style></svg>`, `unsupported stylesheet property "filter"`},
		{"invalid-value-unused", `<svg width="1" height="1"><style>.unused{stroke-width:bad}</style></svg>`, "invalid stroke-width"},
		{"css-transform", `<svg width="1" height="1"><style>.a{transform:scale(2)}</style></svg>`, "unsupported stylesheet transform"},
		{"missing-colon", `<svg width="1" height="1"><style>.a{fill}</style></svg>`, "malformed stylesheet declaration"},
		{"empty-value", `<svg width="1" height="1"><style>.a{fill:}</style></svg>`, "malformed stylesheet declaration"},
		{"nested-brace", `<svg width="1" height="1"><style>.a{{fill:red}}</style></svg>`, "malformed stylesheet declaration block"},
		{"unterminated", `<svg width="1" height="1"><style>.a{fill:red</style></svg>`, "unterminated declaration block"},
		{"unexpected-close", `<svg width="1" height="1"><style>}</style></svg>`, "unexpected closing brace"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := ImportNode(context.Background(), "style.svg", []byte(test.source), generousImportLimits())
			if err == nil || result != nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("result/error = %#v/%v; want %q", result, err, test.want)
			}
		})
	}
}

func TestImportNodeStylesheetLimitsAndMetrics(t *testing.T) {
	source := []byte(`<svg width="2" height="1"><style type="text/css">.paint{fill:red}</style><rect class="paint" width="2" height="1"/></svg>`)
	limits := generousImportLimits()
	limits.MaxBytes = len(source)
	result, err := ImportNode(context.Background(), "style.svg", source, limits)
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.SourceBytes != len(source) || result.Metrics.ParsedElements != 3 || result.Metrics.ParsedAttributes != 6 || result.Metrics.EmittedElements != 2 {
		t.Fatalf("stylesheet metrics = %+v", result.Metrics)
	}

	for _, test := range []struct {
		name, want string
		change     func(*Limits)
	}{
		{"bytes", "bytes", func(l *Limits) { l.MaxBytes = len(source) - 1 }},
		{"elements", "element count", func(l *Limits) { l.MaxElements = result.Metrics.ParsedElements - 1 }},
		{"attributes", "attribute count", func(l *Limits) { l.MaxAttributes = result.Metrics.ParsedAttributes - 1 }},
		{"attribute-bytes", "attribute bytes", func(l *Limits) { l.MaxAttributeBytes = result.Metrics.ParsedAttributeBytes - 1 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			limited := limits
			test.change(&limited)
			got, err := ImportNode(context.Background(), "style.svg", source, limited)
			if err == nil || got != nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("result/error = %#v/%v; want %q", got, err, test.want)
			}
		})
	}
}

func TestStylesheetAndClassScansPreserveCancellation(t *testing.T) {
	limits := generousImportLimits()
	limits.MaxBytes = 1 << 20
	limits.MaxAttributeBytes = 1 << 20

	importer := svgImporter{ctx: &cancelAfterContext{remaining: 3}, source: "cancel.svg", limits: limits}
	if _, err := importer.parseStylesheet(strings.Repeat(" ", 32<<10)); !errors.Is(err, context.Canceled) {
		t.Fatalf("stylesheet cancellation = %v", err)
	}

	importer = svgImporter{ctx: &cancelAfterContext{remaining: 3}, source: "cancel.svg", limits: limits}
	_, _, err := importer.classTokens(&svgElement{name: "rect", attrs: map[string]string{"class": strings.Repeat("x ", 16<<10)}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("class token cancellation = %v", err)
	}

	importer = svgImporter{ctx: &cancelAfterContext{remaining: 3}, source: "cancel.svg", limits: limits}
	if err := importer.appendStylesheetText(&svgElement{name: "style"}, []byte(strings.Repeat("x", 32<<10))); !errors.Is(err, context.Canceled) {
		t.Fatalf("stylesheet text cancellation = %v", err)
	}
}
