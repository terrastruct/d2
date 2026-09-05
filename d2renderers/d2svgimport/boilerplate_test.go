package d2svgimport

import (
	"context"
	"errors"
	"image/color"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2scene"
)

func TestImportNodeAcceptsBoundedNonRenderingBoilerplate(t *testing.T) {
	result := mustImport(t, `<svg version="1.1" x="0px" y="-0pt" width="10" height="10" xml:space="preserve" data-name="AWS icon" data-d2-version="test" role="img" focusable="false" enable-background="new 0 0 10 10">
  <rect id="paint" data-layer="foreground" width="10" height="10" fill="#123456"><title id="asset-title">AWS &amp; D2</title></rect>
</svg>`)
	if len(result.Root.Children) != 1 || result.Root.Children[0].ID != "paint" {
		t.Fatalf("non-rendering boilerplate leaked into scene: %+v", result.Root.Children)
	}
	if result.Metrics.ParsedElements != 3 || result.Metrics.EmittedElements != 2 || result.Metrics.DeclaredResources != 2 {
		t.Fatalf("boilerplate metrics = %+v", result.Metrics)
	}
	rect := result.Root.Children[0].Primitive.(d2scene.Rect)
	assertSolidColor(t, rect.Fill, color.NRGBA{R: 0x12, G: 0x34, B: 0x56, A: 0xff})
}

func TestImportNodeAcceptsCanonicalSVGDoctype(t *testing.T) {
	result := mustImport(t, `<?xml version="1.0" encoding="utf-8"?>
<!-- Generator: Adobe Illustrator -->
<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd">
<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"><rect id="paint" width="10" height="10"/></svg>`)
	if len(result.Root.Children) != 1 || result.Root.Children[0].ID != "paint" {
		t.Fatalf("canonical SVG doctype changed imported scene: %+v", result.Root.Children)
	}

	isoSource := []byte(`<?xml version="1.0" encoding="iso-8859-1"?><!--caf`)
	isoSource = append(isoSource, 0xe9)
	isoSource = append(isoSource, []byte(`--><!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd"><svg width="1" height="1"/>`)...)
	if result, err := ImportNode(context.Background(), "fixture.svg", isoSource, generousImportLimits()); err != nil || result == nil {
		t.Fatalf("ISO-8859-1 prolog import = %#v/%v", result, err)
	}
}

func TestImportNodeAcceptsInlineEnableBackgroundAndInertRoles(t *testing.T) {
	result := mustImport(t, `<svg version="1.0" width="10" height="10" style="enable-background:new 0 0 10 10;fill:#abcdef" role="none"><g role="presentation"><rect width="10" height="10"/></g></svg>`)
	if len(result.Root.Children) != 1 || len(result.Root.Children[0].Children) != 1 {
		t.Fatalf("inline enable-background scene = %+v", result.Root)
	}
	rect := result.Root.Children[0].Children[0].Primitive.(d2scene.Rect)
	assertSolidColor(t, rect.Fill, color.NRGBA{R: 0xab, G: 0xcd, B: 0xef, A: 0xff})
}

func TestImportNodeAcceptsCorpusEditorAndRDFMetadata(t *testing.T) {
	source := `<svg xmlns="http://www.w3.org/2000/svg"
 xmlns:dc="http://purl.org/dc/elements/1.1/"
 xmlns:cc="http://creativecommons.org/ns#"
 xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
 xmlns:sodipodi="http://sodipodi.sourceforge.net/DTD/sodipodi-0.dtd"
 xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape"
 version="1.1" x="0px" y="0px" width="32" height="32" viewBox="0 0 32 32"
 style="enable-background:new 0 0 32 32" sodipodi:docname="rare.svg"
 inkscape:version="1.0.1" inkscape:export-filename="new.png" inkscape:export-xdpi="250.55" inkscape:export-ydpi="250.55">
  <metadata id="metadata845"><rdf:RDF><cc:Work rdf:about=""><dc:format> image/svg+xml </dc:format><dc:type rdf:resource="http://purl.org/dc/dcmitype/StillImage"/></cc:Work></rdf:RDF></metadata>
  <sodipodi:namedview id="base" pagecolor="#ffffff" bordercolor="#666666" borderopacity="1.0"
    objecttolerance="10" gridtolerance="10" guidetolerance="10" fit-margin-bottom="10"
    fit-margin-left="10" fit-margin-right="10" fit-margin-top="10" showgrid="false"
    inkscape:current-layer="layer1" inkscape:cx="16" inkscape:cy="16" inkscape:document-units="px"
    inkscape:pageopacity="0" inkscape:pageshadow="2" inkscape:snap-global="false"
    inkscape:window-height="822" inkscape:window-maximized="0" inkscape:window-width="1519"
    inkscape:window-x="51" inkscape:window-y="25" inkscape:zoom="16.19"/>
  <title id="title833">router-solid</title>
  <g id="layer1" inkscape:groupmode="layer" inkscape:label="Layer 1">
    <path id="paint" inkscape:connector-curvature="0" inkscape:export-filename="new.png"
      inkscape:export-xdpi="250.55" inkscape:export-ydpi="250.55" sodipodi:nodetypes="cc"
      d="M0 0L32 32" fill="none" stroke="black"/>
  </g>
</svg>`
	result := mustImport(t, source)
	if len(result.Root.Children) != 1 || result.Root.Children[0].ID != "layer1" ||
		len(result.Root.Children[0].Children) != 1 || result.Root.Children[0].Children[0].ID != "paint" {
		t.Fatalf("editor metadata leaked or paint was lost: %+v", result.Root)
	}
	if result.Metrics.ParsedElements != 10 || result.Metrics.EmittedElements != 3 || result.Metrics.DeclaredResources != 5 {
		t.Fatalf("editor metadata metrics = %+v", result.Metrics)
	}
}

func TestImportNodeRejectsRenderingOrUnrecognizedBoilerplate(t *testing.T) {
	tests := []struct {
		name, source, want string
	}{
		{"version", `<svg width="1" height="1" version="2.0"/>`, "unsupported version"},
		{"nonzero-root-x", `<svg width="1" height="1" x="1"/>`, "must be a zero"},
		{"percentage-root-y", `<svg width="1" height="1" y="0%"/>`, "must be a zero"},
		{"xml-space", `<svg width="1" height="1" xml:space="collapse"/>`, "invalid xml:space"},
		{"role", `<svg width="1" height="1" role="button"/>`, "unsupported non-rendering role"},
		{"focusable", `<svg width="1" height="1" focusable="true"/>`, "must be false"},
		{"empty-data-name", `<svg width="1" height="1" data-="x"/>`, "invalid data-*"},
		{"enable-background-none", `<svg width="1" height="1" enable-background="none"/>`, "invalid enable-background"},
		{"enable-background-short", `<svg width="1" height="1" enable-background="new 0 0 1"/>`, "invalid enable-background"},
		{"enable-background-comma", `<svg width="1" height="1" enable-background="new 0,0,1,1"/>`, "whitespace-separated"},
		{"enable-background-concatenated", `<svg width="1" height="1" enable-background="new 0 0 1+1"/>`, "invalid enable-background"},
		{"enable-background-negative-size", `<svg width="1" height="1" enable-background="new 0 0 -1 1"/>`, "must be positive"},
		{"enable-background-on-group", `<svg width="1" height="1"><g style="enable-background:new 0 0 1 1"/></svg>`, "outside the root"},
		{"enable-background-stylesheet", `<svg width="1" height="1"><style>.x{enable-background:new 0 0 1 1}</style><g class="x"/></svg>`, "unsupported stylesheet property"},
		{"title-child", `<svg width="1" height="1"><title><g/></title></svg>`, "cannot contain child"},
		{"title-paint", `<svg width="1" height="1"><title fill="red">title</title></svg>`, "unsupported attribute"},
		{"use-title", `<svg width="1" height="1"><title id="title">title</title><use href="#title"/></svg>`, "unsupported <title> resource"},
		{"metadata-rendering-child", `<svg width="1" height="1"><metadata><g/></metadata></svg>`, "unsupported child"},
		{"empty-metadata", `<svg width="1" height="1"><metadata/></svg>`, "exactly one <rdf:RDF>"},
		{"empty-rdf", `<svg xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" width="1" height="1"><metadata><rdf:RDF/></metadata></svg>`, "exactly one <cc:Work>"},
		{"incomplete-work", `<svg xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:cc="http://creativecommons.org/ns#" xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" width="1" height="1"><metadata><rdf:RDF><cc:Work><dc:format>image/svg+xml</dc:format></cc:Work></rdf:RDF></metadata></svg>`, "exactly one <dc:format>"},
		{"duplicate-format", `<svg xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:cc="http://creativecommons.org/ns#" xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" width="1" height="1"><metadata><rdf:RDF><cc:Work><dc:format>image/svg+xml</dc:format><dc:format>image/svg+xml</dc:format></cc:Work></rdf:RDF></metadata></svg>`, "exactly one <dc:format>"},
		{"rdf-outside-metadata", `<svg xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" width="1" height="1"><rdf:RDF/></svg>`, "only supported inside"},
		{"unknown-dublin-core", `<svg xmlns:dc="http://purl.org/dc/elements/1.1/" width="1" height="1"><metadata><dc:creator/></metadata></svg>`, "unsupported namespace or metadata name"},
		{"foreign-metadata", `<svg xmlns:e="urn:evil" width="1" height="1"><metadata><e:payload/></metadata></svg>`, "unsupported namespace"},
		{"metadata-text", `<svg width="1" height="1"><metadata>payload</metadata></svg>`, "text content is unsupported"},
		{"metadata-format", `<svg xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:cc="http://creativecommons.org/ns#" xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" width="1" height="1"><metadata><rdf:RDF><cc:Work><dc:format>text/html</dc:format></cc:Work></rdf:RDF></metadata></svg>`, "unsupported metadata text"},
		{"external-rdf-about", `<svg xmlns:cc="http://creativecommons.org/ns#" xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" width="1" height="1"><metadata><rdf:RDF><cc:Work rdf:about="https://example.invalid/x"/></rdf:RDF></metadata></svg>`, "unsupported RDF metadata attribute"},
		{"external-rdf-resource", `<svg xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:cc="http://creativecommons.org/ns#" xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" width="1" height="1"><metadata><rdf:RDF><cc:Work><dc:type rdf:resource="https://example.invalid/x"/></cc:Work></rdf:RDF></metadata></svg>`, "unsupported RDF metadata attribute"},
		{"namedview-position", `<svg xmlns:sodipodi="http://sodipodi.sourceforge.net/DTD/sodipodi-0.dtd" width="1" height="1"><g><sodipodi:namedview/></g></svg>`, "direct child"},
		{"namedview-child", `<svg xmlns:sodipodi="http://sodipodi.sourceforge.net/DTD/sodipodi-0.dtd" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape" width="1" height="1"><sodipodi:namedview><inkscape:grid/></sodipodi:namedview></svg>`, "unsupported namespace or metadata name"},
		{"unknown-inkscape-attribute", `<svg xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape" width="1" height="1" inkscape:onload="go()"/>`, "unsupported Inkscape metadata attribute"},
		{"misplaced-inkscape-attribute", `<svg xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape" width="1" height="1"><rect width="1" height="1" inkscape:label="paint"/></svg>`, "unsupported Inkscape metadata attribute"},
		{"metadata-script", `<svg width="1" height="1"><metadata><script/></metadata></svg>`, "forbidden element"},
		{"title-script", `<svg width="1" height="1"><title><script/></title></svg>`, "forbidden element"},
		{"xml-base", `<svg width="1" height="1" xml:base="https://example.invalid/"/>`, "unsupported xml attribute"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := ImportNode(context.Background(), "boilerplate.svg", []byte(test.source), generousImportLimits())
			if err == nil || result != nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("result/error = %#v/%v; want %q", result, err, test.want)
			}
		})
	}
}

func TestNonRenderingBoilerplatePreservesCancellation(t *testing.T) {
	if err := checkpointXMLText(&cancelAfterContext{remaining: 3}, []byte(strings.Repeat("x", 32<<10))); !errors.Is(err, context.Canceled) {
		t.Fatalf("title checkpoint error = %v", err)
	}

	importer := svgImporter{
		ctx:    &cancelAfterContext{remaining: 3},
		source: "cancel.svg",
		limits: generousImportLimits(),
	}
	if err := importer.appendMetadataText(&svgElement{}, []byte(strings.Repeat("x", 32<<10))); !errors.Is(err, context.Canceled) {
		t.Fatalf("metadata checkpoint error = %v", err)
	}

	importer.ctx = &cancelAfterContext{remaining: 3}
	if _, err := importer.validateIgnoredUnnamespacedAttribute("svg", "data-"+strings.Repeat("x", 32<<10), "value"); !errors.Is(err, context.Canceled) {
		t.Fatalf("data-name checkpoint error = %v", err)
	}

	importer.ctx = &cancelAfterContext{remaining: 3}
	if err := validateEnableBackground(importer.ctx, strings.Repeat(" ", 32<<10)+"new 0 0 1 1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("enable-background checkpoint error = %v", err)
	}
}

func TestNonRenderingBoilerplateConsumesExistingLimits(t *testing.T) {
	base := generousImportLimits()
	tests := []struct {
		name, source, want string
		limits             Limits
	}{
		{"elements", `<svg width="1" height="1"><title>title</title></svg>`, "element count", withLimit(base, func(limits *Limits) { limits.MaxElements = 1 })},
		{"depth", `<svg width="1" height="1"><metadata/></svg>`, "depth exceeds", withLimit(base, func(limits *Limits) { limits.MaxDepth = 1 })},
		{"attributes", `<svg width="1" height="1" data-name="icon"/>`, "attribute count", withLimit(base, func(limits *Limits) { limits.MaxAttributes = 2 })},
		{"attribute-bytes", `<svg width="1" height="1" data-name="icon"/>`, "attribute bytes", withLimit(base, func(limits *Limits) { limits.MaxAttributeBytes = 20 })},
		{"ids", `<svg width="1" height="1"><title id="a">a</title><title id="b">b</title></svg>`, "ID resource count", withLimit(base, func(limits *Limits) { limits.MaxResources = 1 })},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := ImportNode(context.Background(), "limits.svg", []byte(test.source), test.limits)
			if err == nil || result != nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("result/error = %#v/%v; want %q", result, err, test.want)
			}
		})
	}
}
