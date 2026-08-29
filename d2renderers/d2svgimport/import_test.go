package d2svgimport

import (
	"context"
	"errors"
	"image/color"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/d2lang/d2/d2renderers/d2scene"
)

func generousImportLimits() Limits {
	return Limits{
		MaxBytes: 1 << 20, MaxDepth: 64, MaxElements: 1000, MaxAttributes: 2000,
		MaxAttributeBytes: 1 << 19, MaxPathCommands: 10000, MaxTransformFunctions: 1000,
		MaxUseDepth: 32, MaxResources: 1000,
	}
}

func mustImport(t *testing.T, source string) *Result {
	t.Helper()
	result, err := ImportNode(context.Background(), "fixture.svg", []byte(source), generousImportLimits())
	if err != nil {
		t.Fatalf("ImportNode: %v", err)
	}
	return result
}

func TestImportNodeGeometryAndPaintOrder(t *testing.T) {
	result := mustImport(t, `<svg xmlns="http://www.w3.org/2000/svg" width="200px" height="100" viewBox="0 0 100 50">
  <g id="shapes" fill="#102030" stroke="rgb(200, 10, 20)" stroke-width="2" transform="translate(3 4)">
    <rect id="rect" x="1" y="2" width="20" height="10" rx="9" ry="8"/>
    <circle id="circle" cx="30" cy="12" r="4"/>
    <ellipse id="ellipse" cx="45" cy="12" rx="5" ry="3"/>
    <line id="line" x1="0" y1="25" x2="20" y2="25"/>
    <polyline id="polyline" points="25,20 30,25 35,20"/>
    <polygon id="polygon" points="40,20 45,25 50,20" fill-rule="evenodd"/>
    <path id="path" d="M55 20h10v5z"/>
  </g>
</svg>`)
	if result.Width != 200 || result.Height != 100 || result.ViewBox != (d2scene.Box{Width: 100, Height: 50}) {
		t.Fatalf("unexpected viewport: %+v", result)
	}
	if len(result.Root.Children) != 1 {
		t.Fatalf("root children = %d", len(result.Root.Children))
	}
	group := result.Root.Children[0]
	if group.ID != "shapes" || group.Transform != d2scene.Translate(3, 4) || len(group.Children) != 7 {
		t.Fatalf("unexpected group: %+v", group)
	}
	rect, ok := group.Children[0].Primitive.(d2scene.Rect)
	if !ok || rect.Box != (d2scene.Box{X: 1, Y: 2, Width: 20, Height: 10}) || rect.RadiusX != 9 || rect.RadiusY != 5 {
		t.Fatalf("unexpected rect: %#v", group.Children[0].Primitive)
	}
	assertSolidColor(t, rect.Fill, color.NRGBA{R: 0x10, G: 0x20, B: 0x30, A: 0xff})
	assertSolidColor(t, rect.Stroke.Paint, color.NRGBA{R: 200, G: 10, B: 20, A: 0xff})
	if rect.Stroke.Width != 2 {
		t.Fatalf("stroke width = %v", rect.Stroke.Width)
	}
	if _, ok := group.Children[1].Primitive.(d2scene.Ellipse); !ok {
		t.Fatalf("circle primitive = %T", group.Children[1].Primitive)
	}
	if _, ok := group.Children[2].Primitive.(d2scene.Ellipse); !ok {
		t.Fatalf("ellipse primitive = %T", group.Children[2].Primitive)
	}
	line := group.Children[3].Primitive.(d2scene.Path)
	if line.Fill != nil || len(line.Commands) != 2 {
		t.Fatalf("line fill/commands = %#v/%d", line.Fill, len(line.Commands))
	}
	polyline := group.Children[4].Primitive.(d2scene.Path)
	polygon := group.Children[5].Primitive.(d2scene.Path)
	path := group.Children[6].Primitive.(d2scene.Path)
	if len(polyline.Commands) != 3 || len(polygon.Commands) != 4 || polygon.Commands[3].Kind != d2scene.CloseCommand || polygon.FillRule != d2scene.EvenOdd {
		t.Fatalf("unexpected poly geometry: %d %d %#v", len(polyline.Commands), len(polygon.Commands), polygon)
	}
	if len(path.Commands) != 4 || path.Commands[3].Kind != d2scene.CloseCommand {
		t.Fatalf("path commands = %#v", path.Commands)
	}
}

func TestImportNodeStyleCascadeCurrentColorAndVisibility(t *testing.T) {
	result := mustImport(t, `<svg viewBox="0 0 20 20" color="#112233" fill="currentColor">
  <g stroke="#01020380" stroke-width="3" stroke-linecap="round" stroke-linejoin="bevel" stroke-miterlimit="7" stroke-dasharray="2 , 3, 4" stroke-dashoffset="-2" fill-opacity="0.5" stroke-opacity="0.25">
    <rect id="cascade" width="4" height="4" fill="red" transform="translate(2 3)" style="fill: rgb(4, 5, 6); opacity: .75"/>
    <rect id="current" x="5" width="4" height="4" color="#abcdef"/>
    <g visibility="hidden"><rect id="hidden" width="2" height="2"/><rect id="shown" x="10" width="2" height="2" visibility="visible"/></g>
    <rect id="none" display="none" width="2" height="2"/>
  </g>
</svg>`)
	group := result.Root.Children[0]
	if len(group.Children) != 3 {
		t.Fatalf("group children = %d", len(group.Children))
	}
	cascadeNode := group.Children[0]
	if cascadeNode.Opacity != .75 {
		t.Fatalf("opacity = %v", cascadeNode.Opacity)
	}
	if cascadeNode.Transform != d2scene.Translate(2, 3) {
		t.Fatalf("presentation transform = %+v", cascadeNode.Transform)
	}
	cascade := cascadeNode.Primitive.(d2scene.Rect)
	assertSolidColor(t, cascade.Fill, color.NRGBA{R: 4, G: 5, B: 6, A: 128})
	assertSolidColor(t, cascade.Stroke.Paint, color.NRGBA{R: 1, G: 2, B: 3, A: 32})
	if cascade.Stroke.Cap != d2scene.CapRound || cascade.Stroke.Join != d2scene.JoinBevel ||
		cascade.Stroke.MiterLimit != 7 || cascade.Stroke.DashOffset != -2 ||
		len(cascade.Stroke.Dashes) != 6 {
		t.Fatalf("unexpected stroke: %#v", cascade.Stroke)
	}
	current := group.Children[1].Primitive.(d2scene.Rect)
	assertSolidColor(t, current.Fill, color.NRGBA{R: 0xab, G: 0xcd, B: 0xef, A: 128})
	hiddenGroup := group.Children[2]
	if len(hiddenGroup.Children) != 1 || hiddenGroup.Children[0].ID != "shown" {
		t.Fatalf("visibility descendants = %#v", hiddenGroup.Children)
	}
}

func TestImportNodeEllipseAutoRadius(t *testing.T) {
	result := mustImport(t, `<svg width="20" height="20"><ellipse id="implicit" cx="5" cy="5" rx="4"/><ellipse id="auto" cx="15" cy="5" rx="auto" ry="3"/></svg>`)
	if len(result.Root.Children) != 2 {
		t.Fatalf("ellipse children = %d", len(result.Root.Children))
	}
	implicit := result.Root.Children[0].Primitive.(d2scene.Ellipse)
	auto := result.Root.Children[1].Primitive.(d2scene.Ellipse)
	if implicit.RadiusX != 4 || implicit.RadiusY != 4 || auto.RadiusX != 3 || auto.RadiusY != 3 {
		t.Fatalf("ellipse radii = (%v,%v), (%v,%v)", implicit.RadiusX, implicit.RadiusY, auto.RadiusX, auto.RadiusY)
	}
}

func TestImportNodeConvertsAbsoluteCSSLengthsToPixels(t *testing.T) {
	result := mustImport(t, `<svg width="360pt" height="2.54cm"><rect id="absolute" x="1in" y="6pc" width="25.4mm" height="101.6q" stroke="black" stroke-width="12pt"/></svg>`)
	if !nearFloat(result.Width, 480) || !nearFloat(result.Height, 96) {
		t.Fatalf("absolute viewport = %vx%v, want 480x96", result.Width, result.Height)
	}
	rect := result.Root.Children[0].Primitive.(d2scene.Rect)
	if !nearFloat(rect.Box.X, 96) || !nearFloat(rect.Box.Y, 96) || !nearFloat(rect.Box.Width, 96) || !nearFloat(rect.Box.Height, 96) {
		t.Fatalf("absolute rectangle = %+v, want 96-unit coordinates", rect.Box)
	}
	if rect.Stroke == nil || !nearFloat(rect.Stroke.Width, 16) {
		t.Fatalf("absolute stroke = %+v, want width 16", rect.Stroke)
	}
}

func TestImportNodeUseForwardReferenceTransformAndOwnership(t *testing.T) {
	result := mustImport(t, `<svg width="100" height="100">
  <use id="first" href="#symbol" x="10" y="20" transform="scale(2)" fill="#ff0000"/>
  <use id="second" xmlns:xlink="http://www.w3.org/1999/xlink" xlink:href="#symbol" x="30"/>
  <defs><g id="symbol" transform="translate(1 2)"><path id="mark" d="M0 0L5 0" stroke="currentColor" color="#008000" stroke-dasharray="1 2" fill="none"/></g></defs>
</svg>`)
	if len(result.Root.Children) != 2 {
		t.Fatalf("root children = %d", len(result.Root.Children))
	}
	first, second := result.Root.Children[0], result.Root.Children[1]
	if first.ID != "first" || first.Transform != d2scene.Scale(2, 2).Mul(d2scene.Translate(10, 20)) {
		t.Fatalf("first use = %+v", first)
	}
	if second.ID != "second" || second.Transform != d2scene.Translate(30, 0) {
		t.Fatalf("second use = %+v", second)
	}
	if first.Children[0].ID != "first/symbol" || first.Children[0].Transform != d2scene.Translate(1, 2) || first.Children[0].Children[0].ID != "first/mark" {
		t.Fatalf("first IDs/transforms = %+v", first.Children[0])
	}
	firstPath := first.Children[0].Children[0].Primitive.(d2scene.Path)
	secondPath := second.Children[0].Children[0].Primitive.(d2scene.Path)
	assertSolidColor(t, firstPath.Stroke.Paint, color.NRGBA{G: 128, A: 255})
	firstPath.Commands[0].P1.X = 999
	firstPath.Stroke.Dashes[0] = 999
	if secondPath.Commands[0].P1.X == 999 || secondPath.Stroke.Dashes[0] == 999 {
		t.Fatal("expanded use instances alias mutable command or dash storage")
	}
}

func TestImportNodeViewBoxAspectRatio(t *testing.T) {
	tests := []struct {
		name, aspect  string
		width, height float64
		want          d2scene.Point
		fit           d2scene.AspectFit
	}{
		{"none", "none", 200, 100, d2scene.Point{}, d2scene.AspectMeet},
		{"xmin-ymin", "xMinYMin meet", 200, 100, d2scene.Point{}, d2scene.AspectMeet},
		{"xmid-ymin", "xMidYMin meet", 200, 100, d2scene.Point{X: 50}, d2scene.AspectMeet},
		{"xmax-ymin", "xMaxYMin meet", 200, 100, d2scene.Point{X: 100}, d2scene.AspectMeet},
		{"xmin-ymid", "xMinYMid meet", 100, 200, d2scene.Point{Y: 50}, d2scene.AspectMeet},
		{"xmid-ymid-tall", "xMidYMid meet", 100, 200, d2scene.Point{Y: 50}, d2scene.AspectMeet},
		{"xmax-ymid", "xMaxYMid meet", 100, 200, d2scene.Point{Y: 50}, d2scene.AspectMeet},
		{"xmax-ymid-wide", "xMaxYMid meet", 200, 100, d2scene.Point{X: 100}, d2scene.AspectMeet},
		{"xmin-ymax", "xMinYMax meet", 100, 200, d2scene.Point{Y: 100}, d2scene.AspectMeet},
		{"xmid-ymax", "xMidYMax meet", 100, 200, d2scene.Point{Y: 100}, d2scene.AspectMeet},
		{"xmid-ymax-wide", "xMidYMax meet", 200, 100, d2scene.Point{X: 50}, d2scene.AspectMeet},
		{"xmid-ymid", "xMidYMid meet", 200, 100, d2scene.Point{X: 50}, d2scene.AspectMeet},
		{"xmax-ymax", "xMaxYMax meet", 100, 200, d2scene.Point{Y: 100}, d2scene.AspectMeet},
		{"slice-ymin", "xMinYMin slice", 200, 100, d2scene.Point{}, d2scene.AspectSlice},
		{"slice-ymid", "xMidYMid slice", 200, 100, d2scene.Point{Y: -50}, d2scene.AspectSlice},
		{"slice-ymax", "xMaxYMax slice", 200, 100, d2scene.Point{Y: -100}, d2scene.AspectSlice},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := `<svg width="` + floatString(test.width) + `" height="` + floatString(test.height) + `" viewBox="10 20 100 100" preserveAspectRatio="` + test.aspect + `"/>`
			result := mustImport(t, source)
			got := result.ViewportTransform.Point(d2scene.Point{X: 10, Y: 20})
			if !nearPoint(got, test.want) || result.Aspect.Fit != test.fit {
				t.Fatalf("mapped origin = %+v, aspect = %+v; want %+v", got, result.Aspect, test.want)
			}
		})
	}
	result := mustImport(t, `<svg viewBox="5 , 6, 70 ,80"/>`)
	if result.Width != 70 || result.Height != 80 || result.ViewportTransform != d2scene.Translate(-5, -6) {
		t.Fatalf("viewBox fallback = %+v", result)
	}
	widthOnly := mustImport(t, `<svg width="200" viewBox="0 0 100 50"/>`)
	if widthOnly.Width != 200 || widthOnly.Height != 100 {
		t.Fatalf("width-only intrinsic ratio = %vx%v", widthOnly.Width, widthOnly.Height)
	}
	heightOnly := mustImport(t, `<svg height="75" viewBox="0 0 100 50"/>`)
	if heightOnly.Width != 150 || heightOnly.Height != 75 {
		t.Fatalf("height-only intrinsic ratio = %vx%v", heightOnly.Width, heightOnly.Height)
	}
	none := mustImport(t, `<svg width="200" height="100" viewBox="10 20 100 100" preserveAspectRatio="none"/>`)
	if got := none.ViewportTransform.Point(d2scene.Point{X: 110, Y: 120}); !nearPoint(got, d2scene.Point{X: 200, Y: 100}) {
		t.Fatalf("none did not apply independent-axis scaling: %+v", got)
	}
	noneSlice := mustImport(t, `<svg width="200" height="100" viewBox="10 20 100 100" preserveAspectRatio="none slice"/>`)
	if got := noneSlice.ViewportTransform.Point(d2scene.Point{X: 110, Y: 120}); !nearPoint(got, d2scene.Point{X: 200, Y: 100}) || noneSlice.Aspect.Fit != d2scene.AspectSlice {
		t.Fatalf("none slice should retain fit metadata but ignore it in mapping: %+v %+v", got, noneSlice.Aspect)
	}
}

func TestImportNodeRejectsUnsupportedAndDangerousInput(t *testing.T) {
	tests := []struct {
		name, source, want string
	}{
		{"doctype", `<!DOCTYPE svg [<!ENTITY x SYSTEM "file:///etc/passwd">]><svg width="1" height="1"/>`, "DTD"},
		{"entity", `<svg width="1" height="1">&unknown;</svg>`, "invalid XML"},
		{"canonical-doctype-entity", `<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd"><svg width="1" height="1">&bomb;</svg>`, "invalid XML"},
		{"canonical-doctype-internal-subset", `<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd" [<!ENTITY bomb "boom">]><svg width="1" height="1"/>`, "DTD"},
		{"canonical-doctype-wrong-system", `<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "https://secret.invalid/svg11.dtd"><svg width="1" height="1"/>`, "DTD"},
		{"duplicate-canonical-doctype", `<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd"><!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd"><svg width="1" height="1"/>`, "DTD"},
		{"late-canonical-doctype", `<svg width="1" height="1"/><!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd">`, "DTD"},
		{"comment-spliced-canonical-doctype", `<!DOCTYPE<!--comment--> svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd"><svg width="1" height="1"/>`, "DTD"},
		{"declaration-after-canonical-doctype", `<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd"><?xml version="1.0"?><svg width="1" height="1"/>`, "processing instructions are forbidden"},
		{"script", `<svg width="1" height="1"><script/></svg>`, "forbidden element"},
		{"foreign-object", `<svg width="1" height="1"><foreignObject/></svg>`, "forbidden element"},
		{"unknown-element", `<svg width="1" height="1"><unknown/></svg>`, "unknown element"},
		{"event", `<svg width="1" height="1"><rect onclick="go()"/></svg>`, "forbidden event"},
		{"external-use", `<svg width="1" height="1"><use href="https://secret.invalid/x"/></svg>`, "external references are forbidden"},
		{"unknown-use", `<svg width="1" height="1"><use href="#missing"/></svg>`, "unknown local id"},
		{"paint-server", `<svg width="1" height="1"><rect width="1" height="1" fill="url(https://secret.invalid/x)"/></svg>`, "paint servers"},
		{"paint-server-in-defs", `<svg width="1" height="1"><defs><rect width="1" height="1" fill="url(https://secret.invalid/x)"/></defs></svg>`, "paint servers"},
		{"invalid-hidden-style", `<svg width="1" height="1"><g display="none"><rect width="1" height="1" stroke-width="bad"/></g></svg>`, "invalid stroke-width"},
		{"percentage", `<svg width="1" height="1"><rect width="100%" height="1"/></svg>`, "percentages are unsupported"},
		{"unit", `<svg width="1" height="1"><circle r="1em"/></svg>`, "unitless number or px"},
		{"unit-whitespace", `<svg width="1" height="1"><circle r="1 px"/></svg>`, "immediately follow"},
		{"hex-number", `<svg width="1" height="1"><circle r="0x1p2"/></svg>`, "unitless number or px"},
		{"underscore-number", `<svg width="1" height="1"><circle r="1_0"/></svg>`, "unitless number or px"},
		{"points-path-command", `<svg width="10" height="10"><polyline points="0 0 C 1 1 2 2 3 3"/></svg>`, "invalid points"},
		{"points-close-command", `<svg width="10" height="10"><polygon points="0 0 1 1z"/></svg>`, "invalid points"},
		{"points-odd", `<svg width="10" height="10"><polyline points="0 0 1"/></svg>`, "complete coordinate pairs"},
		{"points-unit", `<svg width="10" height="10"><polyline points="0 0 1px 1"/></svg>`, "unitless coordinates"},
		{"mixed-zero-dash", `<svg width="10" height="10"><path d="M0 0L1 1" stroke="black" stroke-dasharray="0 2"/></svg>`, "zero-length dash"},
		{"overflowing-dash-total", `<svg width="10" height="10"><path d="M0 0L1 1" stroke="black" stroke-dasharray="1e308 1e308"/></svg>`, "dash total must be finite"},
		{"non-finite-painted-bounds", `<svg width="10" height="10"><rect x="1e308" y="1e308" width="1e308" height="1e308"/></svg>`, "non-finite visual bounds"},
		{"viewport-scale-underflow", `<svg width="5e-324" height="1" viewBox="0 0 1e308 1"/>`, "viewport scale is zero"},
		{"unknown-attribute", `<svg width="1" height="1" unknown="1"/>`, "unsupported attribute"},
		{"unknown-style", `<svg width="1" height="1" style="filter:none"/>`, "unsupported style property"},
		{"inline-css-transform", `<svg width="10" height="10"><rect width="1" height="1" style="transform:rotate(45deg)"/></svg>`, "unsupported CSS transform"},
		{"round-zero-line", `<svg width="10" height="10"><line x1="1" y1="1" x2="1" y2="1" stroke="black" stroke-linecap="round"/></svg>`, "zero-length open stroked subpath"},
		{"round-zero-path-segment", `<svg width="10" height="10"><path d="M1 1L1 1" stroke="black" stroke-linecap="round"/></svg>`, "zero-length open stroked subpath"},
		{"round-one-point-polyline", `<svg width="10" height="10"><polyline points="1 1" stroke="black" stroke-linecap="round"/></svg>`, "zero-length open stroked subpath"},
		{"round-zero-line-through-use", `<svg width="10" height="10"><defs><line id="dot" x1="1" y1="1" x2="1" y2="1"/></defs><use href="#dot" stroke="black" stroke-linecap="round"/></svg>`, "zero-length open stroked subpath"},
		{"text", `<svg width="1" height="1">secret text</svg>`, "text content is unsupported"},
		{"unbounded-nested-svg", `<svg width="1" height="1"><svg/></svg>`, "nested <svg> requires an explicit viewBox"},
		{"root-transform", `<svg width="20" height="20" viewBox="0 0 10 10" transform="translate(10)"><rect width="1" height="1"/></svg>`, "root <svg> transform is unsupported"},
		{"shape-child", `<svg width="1" height="1"><rect><path/></rect></svg>`, "cannot contain child"},
		{"namespace", `<s:svg xmlns:s="urn:other" width="1" height="1"/>`, "unsupported namespace"},
		{"attribute-namespace", `<svg xmlns:e="urn:other" width="1" height="1" e:xmlns="ignored"/>`, "unsupported namespace"},
		{"svg-attribute-namespace", `<svg xmlns:s="http://www.w3.org/2000/svg" width="1" height="1"><rect s:width="1" height="1"/></svg>`, "unsupported namespace"},
		{"namespace-reset", `<svg xmlns="http://www.w3.org/2000/svg" width="1" height="1"><rect xmlns="" width="1" height="1"/></svg>`, "mixes SVG and empty namespaces"},
		{"namespace-introduced", `<svg width="1" height="1"><s:rect xmlns:s="http://www.w3.org/2000/svg" width="1" height="1"/></svg>`, "mixes SVG and empty namespaces"},
		{"processing-instruction", `<?bad secret?><svg width="1" height="1"/>`, "processing instructions are forbidden"},
		{"iso-8859-1-doctype", `<?xml version="1.0" encoding="iso-8859-1"?><!DOCTYPE svg SYSTEM "https://secret.invalid/svg.dtd"><svg width="1" height="1"/>`, "DTD"},
		{"unsupported-xml-encoding", `<?xml version="1.0" encoding="windows-1252"?><svg width="1" height="1"/>`, "invalid XML"},
		{"duplicate-declaration", `<?xml version="1.0"?><?xml version="1.0"?><svg width="1" height="1"/>`, "processing instructions are forbidden"},
		{"malformed-declaration", `<?xml potato?><svg width="1" height="1"/>`, "invalid XML declaration"},
		{"late-declaration", ` <?xml version="1.0"?><svg width="1" height="1"/>`, "processing instructions are forbidden"},
		{"duplicate-id", `<svg width="1" height="1"><g id="x"/><g id="x"/></svg>`, "duplicate id"},
		{"href-space", `<svg width="1" height="1"><g id="x"/><use href=" #x"/></svg>`, "external references are forbidden"},
		{"duplicate-href", `<svg xmlns:xlink="http://www.w3.org/1999/xlink" width="1" height="1"><g id="x"/><use href="#x" xlink:href="#x"/></svg>`, "duplicate attribute"},
		{"duplicate-namespace", `<svg xmlns:x="urn:evil" xmlns:x="http://www.w3.org/2000/svg" width="1" height="1"><x:rect width="1" height="1"/></svg>`, "duplicate XML attribute"},
		{"xml-prefix-rebound", `<svg xmlns:xml="urn:evil" width="1" height="1"/>`, "invalid xml namespace"},
		{"xmlns-prefix-bound", `<svg xmlns:xmlns="urn:evil" width="1" height="1"/>`, "forbidden xmlns namespace"},
		{"xmlns-uri-bound", `<svg xmlns:x="http://www.w3.org/2000/xmlns/" width="1" height="1"/>`, "forbidden xmlns namespace"},
		{"xml-uri-rebound", `<svg xmlns:x="http://www.w3.org/XML/1998/namespace" width="1" height="1"/>`, "reserved xml namespace"},
		{"empty-prefixed-namespace", `<svg xmlns:x="" width="1" height="1"/>`, "empty prefixed namespace"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := ImportNode(context.Background(), "https://user:password@example.com/art.svg?token=secret#fragment", []byte(test.source), generousImportLimits())
			if err == nil || result != nil {
				t.Fatalf("result/error = %#v/%v", result, err)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error %q does not contain %q", err, test.want)
			}
			if strings.Contains(err.Error(), "password") || strings.Contains(err.Error(), "token=secret") || strings.Contains(err.Error(), "secret.invalid") {
				t.Fatalf("error leaks source data: %v", err)
			}
		})
	}
}

func TestImportNodeIgnoresMoveOnlyPath(t *testing.T) {
	result := mustImport(t, `<svg width="24" height="24"><rect id="visible" width="1" height="1"/><path stroke="#fff" stroke-width="3" stroke-linecap="round" stroke-linejoin="round" d="M4.335 19.029" fill="none"/></svg>`)
	if len(result.Root.Children) != 1 || result.Root.Children[0].ID != "visible" {
		t.Fatalf("move-only path was emitted: %#v", result.Root.Children)
	}
}

func TestImportNodeISO88591Declaration(t *testing.T) {
	source := []byte(`<?xml version="1.0" encoding="iso-8859-1"?><svg width="2" height="2"><rect id="caf`)
	source = append(source, 0xe9)
	source = append(source, []byte(`" width="1" height="1"/></svg>`)...)
	result, err := ImportNode(context.Background(), "fixture.svg", source, generousImportLimits())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Root.Children) != 1 || result.Root.Children[0].ID != "café" {
		t.Fatalf("ISO-8859-1 element = %#v", result.Root.Children)
	}
}

func TestImportNodeXMLDeclarationNamespacesAndSourceRedaction(t *testing.T) {
	result, err := ImportNode(context.Background(), "urn:bearer-token?query=secret", append([]byte{0xef, 0xbb, 0xbf}, []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg" width="1" height="1"><rect width="1" height="1"/></svg>`)...), generousImportLimits())
	if err != nil || result == nil {
		t.Fatalf("XML declaration import = %#v/%v", result, err)
	}
	_, err = ImportNode(context.Background(), "urn:bearer-token?query=secret", []byte(`<svg width="1" height="1"><bad/></svg>`), generousImportLimits())
	if err == nil || strings.Contains(err.Error(), "bearer-token") || strings.Contains(err.Error(), "secret") || !strings.Contains(err.Error(), "urn:<redacted>") {
		t.Fatalf("opaque URI redaction = %v", err)
	}
	_, err = ImportNode(context.Background(), "//user:password@example.com/art.svg?token=secret#fragment", []byte(`<svg width="1" height="1"><bad/></svg>`), generousImportLimits())
	if err == nil || strings.Contains(err.Error(), "user") || strings.Contains(err.Error(), "password") || strings.Contains(err.Error(), "token") {
		t.Fatalf("network-path URI redaction = %v", err)
	}
}

func TestImportNodeUseCyclesAndDepth(t *testing.T) {
	for _, source := range []string{
		`<svg width="1" height="1"><defs><g id="a"><use href="#a"/></g></defs><use href="#a"/></svg>`,
		`<svg width="1" height="1"><defs><g id="a"><use href="#b"/></g><g id="b"><use href="#a"/></g></defs><use href="#a"/></svg>`,
		`<svg width="1" height="1"><defs><g id="unused"><use href="#unused"/></g></defs></svg>`,
		`<svg width="1" height="1"><defs><g id="hidden" display="none"><use href="#hidden"/></g></defs><use href="#hidden"/></svg>`,
	} {
		result, err := ImportNode(context.Background(), "cycle.svg", []byte(source), generousImportLimits())
		if err == nil || result != nil || !strings.Contains(err.Error(), "cycle") {
			t.Fatalf("cycle result/error = %#v/%v", result, err)
		}
	}
	limits := generousImportLimits()
	limits.MaxUseDepth = 1
	result, err := ImportNode(context.Background(), "depth.svg", []byte(`<svg width="1" height="1"><defs><g id="a"><use href="#b"/></g><path id="b" d="M0 0L1 1"/></defs><use href="#a"/></svg>`), limits)
	if err == nil || result != nil || !strings.Contains(err.Error(), "depth exceeds") {
		t.Fatalf("use depth result/error = %#v/%v", result, err)
	}
	result, err = ImportNode(context.Background(), "unused-depth.svg", []byte(`<svg width="1" height="1"><defs><g id="a"><use href="#b"/></g><g id="b"><use href="#c"/></g><path id="c" d="M0 0L1 1"/></defs></svg>`), limits)
	if err == nil || result != nil || !strings.Contains(err.Error(), "depth exceeds") {
		t.Fatalf("unused use depth result/error = %#v/%v", result, err)
	}
}

func TestImportNodeLimits(t *testing.T) {
	base := generousImportLimits()
	tests := []struct {
		name, source, want string
		limits             Limits
	}{
		{"bytes", `<svg width="1" height="1"/>`, "bytes", withLimit(base, func(l *Limits) { l.MaxBytes = 4 })},
		{"depth", `<svg width="1" height="1"><g><g/></g></svg>`, "depth exceeds", withLimit(base, func(l *Limits) { l.MaxDepth = 2 })},
		{"elements", `<svg width="1" height="1"><g/></svg>`, "element count", withLimit(base, func(l *Limits) { l.MaxElements = 1 })},
		{"attributes", `<svg width="1" height="1"/>`, "attribute count", withLimit(base, func(l *Limits) { l.MaxAttributes = 1 })},
		{"attribute-bytes", `<svg width="1" height="1"/>`, "attribute bytes", withLimit(base, func(l *Limits) { l.MaxAttributeBytes = 2 })},
		{"path-commands", `<svg width="1" height="1"><path d="M0 0L1 1"/></svg>`, "command count", withLimit(base, func(l *Limits) { l.MaxPathCommands = 1 })},
		{"transform-functions", `<svg width="1" height="1"><g transform="translate(1) scale(2)"/></svg>`, "function count", withLimit(base, func(l *Limits) { l.MaxTransformFunctions = 1 })},
		{"id-resources", `<svg width="1" height="1"><g id="a"/><g id="b"/></svg>`, "ID resource count", withLimit(base, func(l *Limits) { l.MaxResources = 1 })},
		{"use-resources", `<svg width="1" height="1"><defs><path id="a" d="M0 0L1 1"/></defs><use href="#a"/><use href="#a"/></svg>`, "local-use resource count", withLimit(base, func(l *Limits) { l.MaxResources = 1 })},
		{"emitted-elements", `<svg width="1" height="1"><defs><g id="a"><path d="M0 0L1 1"/></g></defs><use href="#a"/><use href="#a"/></svg>`, "emitted element count", withLimit(base, func(l *Limits) { l.MaxElements = 6 })},
		{"emitted-commands", `<svg width="1" height="1"><defs><path id="a" d="M0 0L1 1"/></defs><use href="#a"/><use href="#a"/></svg>`, "emitted path command", withLimit(base, func(l *Limits) { l.MaxPathCommands = 3 })},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := ImportNode(context.Background(), "limits.svg", []byte(test.source), test.limits)
			if err == nil || result != nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("result/error = %#v/%v; want %q", result, err, test.want)
			}
		})
	}
	invalid := base
	invalid.MaxDepth = 0
	if result, err := ImportNode(context.Background(), "limits.svg", nil, invalid); err == nil || result != nil || !strings.Contains(err.Error(), "must be positive") {
		t.Fatalf("invalid limits = %#v/%v", result, err)
	}
}

func TestImportNodeLimitsAreInclusiveAndIndependent(t *testing.T) {
	data := []byte(`<svg width="1" height="1"><defs><path id="p" d="M0 0L1 1"/></defs><use href="#p" transform="translate(0)"/></svg>`)
	limits := Limits{
		MaxBytes: len(data), MaxDepth: 3, MaxElements: 4, MaxAttributes: 6,
		MaxAttributeBytes: 52, MaxPathCommands: 2, MaxTransformFunctions: 1,
		MaxUseDepth: 1, MaxResources: 1,
	}
	result, err := ImportNode(context.Background(), "inclusive.svg", data, limits)
	if err != nil || result == nil {
		t.Fatalf("exact-ceiling import = %#v/%v", result, err)
	}
	if len(result.Root.Children) != 1 || len(result.Root.Children[0].Children) != 1 {
		t.Fatalf("unexpected exact-ceiling scene: %+v", result.Root)
	}
}

func TestImportNodePathLimitCountsDegenerateSourceArcs(t *testing.T) {
	source := []byte(`<svg width="10" height="10"><path d="M0 0A1 1 0 0 0 0 0A1 1 0 0 0 0 0"/></svg>`)
	limits := generousImportLimits()
	limits.MaxPathCommands = 3
	if result, err := ImportNode(context.Background(), "arcs.svg", source, limits); err != nil || result == nil {
		t.Fatalf("exact degenerate-arc limit = %#v/%v", result, err)
	}
	limits.MaxPathCommands = 2
	if result, err := ImportNode(context.Background(), "arcs.svg", source, limits); err == nil || result != nil || !strings.Contains(err.Error(), "command count") {
		t.Fatalf("over degenerate-arc limit = %#v/%v", result, err)
	}
}

func TestImportNodeCancellationCheckpoints(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if result, err := ImportNode(canceled, "cancel.svg", []byte(`<svg width="1" height="1"/>`), generousImportLimits()); !errors.Is(err, context.Canceled) || result != nil {
		t.Fatalf("pre-canceled = %#v/%v", result, err)
	}

	ctx := &cancelAfterContext{remaining: 6}
	data := []byte(`<svg width="1" height="1" data="` + strings.Repeat("x", 128<<10) + `"/>`)
	limits := generousImportLimits()
	limits.MaxBytes = len(data) + 1
	limits.MaxAttributeBytes = len(data) + 1
	result, err := ImportNode(ctx, "cancel.svg", data, limits)
	if !errors.Is(err, context.Canceled) || result != nil {
		t.Fatalf("mid-token cancellation = %#v/%v", result, err)
	}
}

func TestImporterPostDecodeScansPreserveCancellation(t *testing.T) {
	tests := []struct {
		name string
		run  func(context.Context) error
	}{
		{
			name: "number-list",
			run: func(ctx context.Context) error {
				_, err := splitSVGNumberList(ctx, strings.Repeat(" ", 32<<10), -1)
				return err
			},
		},
		{
			name: "inline-style",
			run: func(ctx context.Context) error {
				importer := svgImporter{ctx: ctx, source: "cancel.svg"}
				_, err := importer.declarationsFor(&svgElement{name: "rect", attrs: map[string]string{"style": strings.Repeat("x", 32<<10)}})
				return err
			},
		},
		{
			name: "viewBox",
			run: func(ctx context.Context) error {
				importer := svgImporter{ctx: ctx, source: "cancel.svg"}
				_, _, err := importer.parseViewBoxFor(&svgElement{name: "svg", attrs: map[string]string{"viewBox": strings.Repeat(" ", 32<<10)}}, "root")
				return err
			},
		},
		{
			name: "dasharray",
			run: func(ctx context.Context) error {
				importer := svgImporter{ctx: ctx, source: "cancel.svg"}
				_, err := importer.computeStyle(defaultSVGStyle(), &svgElement{name: "path", declarations: map[string]string{"stroke-dasharray": strings.Repeat("1 ", 16<<10) + "1"}})
				return err
			},
		},
		{
			name: "id",
			run: func(ctx context.Context) error {
				_, err := validSVGID(ctx, strings.Repeat("x", 32<<10))
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.run(&cancelAfterContext{remaining: 3}); !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v, want context cancellation", err)
			}
		})
	}

	commands := make([]d2scene.PathCommand, 0, 2048)
	commands = append(commands, d2scene.MoveTo(0, 0))
	for index := 1; index < cap(commands); index++ {
		commands = append(commands, d2scene.LineTo(float64(index), 0))
	}
	err := validatePrimitiveBounds(&cancelAfterContext{remaining: 3}, d2scene.Path{
		Commands: commands, Stroke: &d2scene.Stroke{Paint: d2scene.SolidPaint{Color: color.NRGBA{A: 255}}, Width: 1},
	}, d2scene.Identity())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("chunked bounds error = %v, want context cancellation", err)
	}
}

func TestImportNodeConcurrentAndIndependent(t *testing.T) {
	data := []byte(`<svg viewBox="0 0 10 10"><path d="M0 0L10 10" stroke="red" fill="none"/></svg>`)
	const workers = 24
	var group sync.WaitGroup
	errorsOut := make(chan error, workers)
	for worker := 0; worker < workers; worker++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for iteration := 0; iteration < 20; iteration++ {
				_, err := ImportNode(context.Background(), "race.svg", data, generousImportLimits())
				if err != nil {
					errorsOut <- err
					return
				}
			}
		}()
	}
	group.Wait()
	close(errorsOut)
	for err := range errorsOut {
		t.Fatal(err)
	}

	first := mustImport(t, string(data))
	second := mustImport(t, string(data))
	firstPath := first.Root.Children[0].Primitive.(d2scene.Path)
	secondPath := second.Root.Children[0].Primitive.(d2scene.Path)
	firstPath.Commands[0].P1.X = 999
	if secondPath.Commands[0].P1.X == 999 {
		t.Fatal("separate imports alias path storage")
	}
}

func TestImportNodeReportsSourceAndEmittedMetrics(t *testing.T) {
	source := `<svg width="1" height="1"><defs><path id="p" d="M0 0L1 1"/></defs><use href="#p"/></svg>`
	result, err := ImportNode(context.Background(), "metrics.svg", []byte(source), generousImportLimits())
	if err != nil {
		t.Fatal(err)
	}
	metrics := result.Metrics
	if metrics.SourceBytes != len(source) || metrics.ParsedElements != 4 || metrics.ParsedAttributes != 5 ||
		metrics.ParsedPathCommands != 2 || metrics.DeclaredResources != 1 || metrics.ExpandedUseInstances != 1 ||
		metrics.EmittedElements != 3 || metrics.EmittedPathCommands != 2 {
		t.Fatalf("import metrics = %+v", metrics)
	}
	if metrics.ParsedAttributeBytes <= 0 || metrics.ParsedTransformFuncs != 0 {
		t.Fatalf("import metric details = %+v", metrics)
	}
}

func TestImportNodeDeepTreeUsesIterativeConstruction(t *testing.T) {
	const depth = 4096
	var source strings.Builder
	source.Grow(depth*7 + 64)
	source.WriteString(`<svg width="1" height="1">`)
	for index := 0; index < depth; index++ {
		source.WriteString(`<g>`)
	}
	for index := 0; index < depth; index++ {
		source.WriteString(`</g>`)
	}
	source.WriteString(`</svg>`)
	data := []byte(source.String())
	limits := generousImportLimits()
	limits.MaxBytes = len(data)
	limits.MaxDepth = depth + 1
	limits.MaxElements = depth + 1
	result, err := ImportNode(context.Background(), "deep.svg", data, limits)
	if err != nil || result == nil {
		t.Fatalf("deep iterative import = %#v/%v", result, err)
	}
	node := result.Root
	for level := 0; level < depth; level++ {
		if len(node.Children) != 1 {
			t.Fatalf("depth %d children = %d", level, len(node.Children))
		}
		node = node.Children[0]
	}
}

func assertSolidColor(t *testing.T, paint d2scene.Paint, want color.NRGBA) {
	t.Helper()
	solid, ok := paint.(d2scene.SolidPaint)
	if !ok || solid.Color != want {
		t.Fatalf("paint = %#v, want %#v", paint, want)
	}
}

func nearPoint(got, want d2scene.Point) bool {
	return math.Abs(got.X-want.X) < 1e-9 && math.Abs(got.Y-want.Y) < 1e-9
}

func nearFloat(got, want float64) bool {
	return math.Abs(got-want) < 1e-9
}

func floatString(value float64) string {
	if value == 100 {
		return "100"
	}
	return "200"
}

func withLimit(limits Limits, change func(*Limits)) Limits {
	change(&limits)
	return limits
}

type cancelAfterContext struct {
	remaining int64
}

func (c *cancelAfterContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *cancelAfterContext) Done() <-chan struct{}       { return nil }
func (c *cancelAfterContext) Value(any) any               { return nil }
func (c *cancelAfterContext) Err() error {
	if atomic.AddInt64(&c.remaining, -1) <= 0 {
		return context.Canceled
	}
	return nil
}
