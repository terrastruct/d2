package d2svgimport

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2raster"
	"github.com/d2lang/d2/d2renderers/d2scene"
)

func TestImportNodeGoIconClipTopologyAndOwnership(t *testing.T) {
	var source strings.Builder
	source.WriteString(`<svg xmlns:xlink="http://www.w3.org/1999/xlink" width="128" height="128"><g>`)
	source.WriteString(`<defs><path id="a" d="M18.8 1h90.5v126H18.8z"/></defs>`)
	source.WriteString(`<clipPath id="b"><use xlink:href="#a" overflow="visible"/></clipPath>`)
	for index := 0; index < 34; index++ {
		source.WriteString(`<path d="M0 0H128V128H0Z" fill="#6ad7e5" fill-rule="evenodd" clip-rule="evenodd" clip-path="url(#b)"/>`)
	}
	source.WriteString(`</g></svg>`)

	result := mustImport(t, source.String())
	if len(result.Root.Children) != 1 || len(result.Root.Children[0].Children) != 34 {
		t.Fatalf("Go icon scene topology = root children %d, group children %d", len(result.Root.Children), len(result.Root.Children[0].Children))
	}
	for index, node := range result.Root.Children[0].Children {
		path, ok := node.Primitive.(d2scene.Path)
		if !ok || path.FillRule != d2scene.EvenOdd {
			t.Fatalf("painted path %d = %#v", index, node.Primitive)
		}
		if node.Clip == nil || node.Clip.Transform != d2scene.Identity() || node.Clip.Path.FillRule != d2scene.NonZero || len(node.Clip.Path.Commands) != 5 {
			t.Fatalf("clip %d = %+v", index, node.Clip)
		}
	}
	first := result.Root.Children[0].Children[0].Clip
	second := result.Root.Children[0].Children[1].Clip
	first.Path.Commands[0].P1.X = 999
	if second.Path.Commands[0].P1.X == 999 {
		t.Fatal("independent clip applications alias mutable command storage")
	}

	metrics := result.Metrics
	if metrics.ParsedElements != 40 || metrics.ParsedPathCommands != 175 || metrics.DeclaredResources != 2 ||
		metrics.ExpandedUseInstances != 34 || metrics.EmittedElements != 36 || metrics.EmittedPathCommands != 340 {
		t.Fatalf("Go icon clip metrics = %+v", metrics)
	}
}

func TestImportNodeClipPathTransformsAndPixels(t *testing.T) {
	result := mustImport(t, `<svg width="8" height="5">
	  <defs><rect id="window" width="2" height="2" transform="translate(1 0)"/></defs>
  <clipPath id="clip" clipPathUnits="userSpaceOnUse" transform="translate(1 0)">
    <use href="#window" x="1" transform="scale(2)" clip-rule="evenodd" overflow="visible"/>
  </clipPath>
  <g transform="translate(1 1)" clip-path="url(#clip)"><rect width="8" height="5" fill="#ff0000"/></g>
</svg>`)
	if len(result.Root.Children) != 1 {
		t.Fatalf("root children = %d", len(result.Root.Children))
	}
	group := result.Root.Children[0]
	wantTransform := d2scene.Matrix{A: 2, D: 2, E: 5}
	if group.Transform != d2scene.Translate(1, 1) || group.Clip == nil || group.Clip.Transform != wantTransform || group.Clip.Path.FillRule != d2scene.EvenOdd {
		t.Fatalf("transformed group/clip = group %v, clip %+v", group.Transform, group.Clip)
	}
	if result.Metrics.ParsedTransformFuncs != 4 || result.Metrics.ExpandedUseInstances != 1 || result.Metrics.EmittedPathCommands != 5 {
		t.Fatalf("transformed clip metrics = %+v", result.Metrics)
	}

	document := d2scene.NewDocument(d2scene.Box{Width: 8, Height: 5}, result.Root)
	frame, err := d2raster.Render(context.Background(), document, d2raster.FrameOptions{
		Scale: 1, MaxWidth: 8, MaxHeight: 5, MaxPixels: 40,
		MaxNodes: 100, MaxDepth: 100, MaxPathCommands: 100,
		MaxAnimationTracks: 1, MaxAnimationKeyframes: 1,
		MaxAssets: 1, MaxAssetBytes: 1, MaxDecodedAssetBytes: 1, MaxImportDepth: 10,
		MaxOffscreenBytes: 1 << 20, MaxEvenOddClipWork: 1 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	red := color.NRGBA{R: 255, A: 255}
	transparent := color.NRGBA{}
	for point, want := range map[[2]int]color.NRGBA{
		{6, 1}: red,
		{7, 4}: red,
		{5, 1}: transparent,
		{6, 0}: transparent,
	} {
		if got := frame.NRGBAAt(point[0], point[1]); got != want {
			t.Fatalf("pixel %v = %+v, want %+v", point, got, want)
		}
	}
}

func TestImportNodeClipPathForwardReferences(t *testing.T) {
	result := mustImport(t, `<svg width="2" height="2">
  <rect width="2" height="2" clip-path="url(#clip)"/>
  <clipPath id="clip"><use href="#shape"/></clipPath>
  <defs><path id="shape" d="M0 0H1V1H0Z"/></defs>
</svg>`)
	if len(result.Root.Children) != 1 || result.Root.Children[0].Clip == nil || len(result.Root.Children[0].Clip.Path.Commands) != 5 {
		t.Fatalf("forward clip scene = %+v", result.Root)
	}
	if result.Metrics.DeclaredResources != 2 || result.Metrics.ExpandedUseInstances != 1 || result.Metrics.EmittedPathCommands != 5 {
		t.Fatalf("forward clip metrics = %+v", result.Metrics)
	}
}

func TestImportNodeClipPathGeometryConversion(t *testing.T) {
	tests := []struct {
		name, geometry string
		commands       int
	}{
		{"path", `<path d="M0 0H2V2Z" clip-rule="evenodd"/>`, 4},
		{"rect", `<rect width="2" height="2"/>`, 5},
		{"rounded-rect", `<rect width="2" height="2" rx=".5" ry=".25"/>`, 10},
		{"circle", `<circle cx="1" cy="1" r="1"/>`, 4},
		{"ellipse", `<ellipse cx="1" cy="1" rx="1" ry=".5"/>`, 4},
		{"line", `<line x1="0" y1="0" x2="2" y2="2"/>`, 2},
		{"polyline", `<polyline points="0 0 2 0 2 2"/>`, 3},
		{"polygon", `<polygon points="0 0 2 0 2 2"/>`, 4},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := mustImport(t, `<svg width="2" height="2"><clipPath id="c">`+test.geometry+`</clipPath><rect width="2" height="2" clip-path="url(#c)"/></svg>`)
			clip := result.Root.Children[0].Clip
			if clip == nil || len(clip.Path.Commands) != test.commands {
				t.Fatalf("clip = %+v, want %d commands", clip, test.commands)
			}
			if test.name == "path" && clip.Path.FillRule != d2scene.EvenOdd {
				t.Fatalf("clip rule = %v, want evenodd", clip.Path.FillRule)
			}
		})
	}
}

func TestImportNodeRejectsUnsupportedClipPaths(t *testing.T) {
	resource := `<defs><path id="p" d="M0 0H1V1Z"/></defs><clipPath id="c"><use href="#p"/></clipPath>`
	tests := []struct {
		name, source, want string
	}{
		{"external", resource + `<rect width="1" height="1" clip-path="url(https://secret.invalid/c)"/>`, "external, quoted, and fallback"},
		{"quoted", resource + `<rect width="1" height="1" clip-path="url('#c')"/>`, "external, quoted, and fallback"},
		{"fallback", resource + `<rect width="1" height="1" clip-path="url(#c) none"/>`, "external, quoted, and fallback"},
		{"unknown", resource + `<rect width="1" height="1" clip-path="url(#missing)"/>`, "unknown local id"},
		{"wrong-kind", `<defs><path id="p" d="M0 0H1V1Z"/></defs><rect width="1" height="1" clip-path="url(#p)"/>`, "does not name a <clipPath>"},
		{"use-clip-path", `<clipPath id="c"><path d="M0 0H1V1Z"/></clipPath><use href="#c"/>`, "references unsupported <clipPath>"},
		{"object-bounds", `<clipPath id="c" clipPathUnits="objectBoundingBox"><path d="M0 0H1V1Z"/></clipPath>`, "userSpaceOnUse"},
		{"missing-id", `<clipPath><path d="M0 0H1V1Z"/></clipPath>`, "must declare an id"},
		{"empty", `<clipPath id="c"/>`, "exactly one"},
		{"multiple-children", `<clipPath id="c"><path d="M0 0H1V1Z"/><rect width="1" height="1"/></clipPath>`, "exactly one"},
		{"unsupported-child", `<clipPath id="c"><g/></clipPath>`, "unsupported child <g>"},
		{"wrong-use-target", `<defs><g id="p"/></defs><clipPath id="c"><use href="#p"/></clipPath>`, "resolves to unsupported <g>"},
		{"overflow-outside", `<defs><path id="p" d="M0 0H1V1Z"/></defs><use href="#p" overflow="visible"/>`, "only on a direct <use>"},
		{"invalid-overflow", `<defs><path id="p" d="M0 0H1V1Z"/></defs><clipPath id="c"><use href="#p" overflow="hidden"/></clipPath>`, "only overflow=\"visible\""},
		{"resource-paint", `<clipPath id="c" fill="red"><path d="M0 0H1V1Z"/></clipPath>`, "unsupported clipping-resource property"},
		{"resource-attribute", `<clipPath id="c" href="#p"><path d="M0 0H1V1Z"/></clipPath>`, `unsupported attribute "href"`},
		{"invalid-clip-rule", resource + `<rect width="1" height="1" clip-path="url(#c)" clip-rule="xor"/>`, "invalid clip-rule"},
		{"clip-on-defs", resource + `<defs clip-path="url(#c)"/>`, "not an emitted element"},
		{"bad-parent", `<path d="M0 0H1V1Z"><clipPath id="c"><path d="M0 0H1V1Z"/></clipPath></path>`, "only supported under"},
		{"use-cycle", `<defs><use id="a" href="#b"/><use id="b" href="#a"/></defs><clipPath id="c"><use href="#a"/></clipPath>`, "cycle"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := `<svg width="2" height="2">` + test.source + `</svg>`
			result, err := ImportNode(context.Background(), "clip.svg", []byte(source), generousImportLimits())
			if err == nil || result != nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("result/error = %#v/%v; want %q", result, err, test.want)
			}
			if strings.Contains(err.Error(), "secret.invalid") {
				t.Fatalf("error leaks external clip URL: %v", err)
			}
		})
	}
}

func TestImportNodeClipPathSharedLimits(t *testing.T) {
	base := generousImportLimits()
	sources := map[string]string{
		"commands":  `<svg width="2" height="2"><defs><rect id="p" width="1" height="1"/></defs><clipPath id="c"><use href="#p"/></clipPath><rect width="1" height="1" clip-path="url(#c)"/><rect width="1" height="1" clip-path="url(#c)"/></svg>`,
		"resources": `<svg width="2" height="2"><defs><rect id="p" width="1" height="1"/></defs><clipPath id="c"><use href="#p"/></clipPath><rect width="1" height="1" clip-path="url(#c)"/><rect width="1" height="1" clip-path="url(#c)"/><rect width="1" height="1" clip-path="url(#c)"/></svg>`,
		"depth":     `<svg width="2" height="2"><defs><rect id="p" width="1" height="1"/><use id="u" href="#p"/></defs><clipPath id="c"><use href="#u"/></clipPath></svg>`,
	}
	tests := []struct {
		name, want string
		limits     Limits
	}{
		{"commands", "emitted path command", withLimit(base, func(l *Limits) { l.MaxPathCommands = 9 })},
		{"resources", "local-use resource count", withLimit(base, func(l *Limits) { l.MaxResources = 2 })},
		{"depth", "depth exceeds", withLimit(base, func(l *Limits) { l.MaxUseDepth = 1 })},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := ImportNode(context.Background(), "clip-limit.svg", []byte(sources[test.name]), test.limits)
			if err == nil || result != nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("result/error = %#v/%v; want %q", result, err, test.want)
			}
		})
	}
}

func TestClipPathExpansionCancellationDoesNotCommitMetrics(t *testing.T) {
	commands := make([]d2scene.PathCommand, 1024)
	for index := range commands {
		commands[index] = d2scene.MoveTo(float64(index), 0)
	}
	resource := &svgElement{name: "clipPath", clipPath: &compiledClipPath{
		path: d2scene.Path{Commands: commands}, transform: d2scene.Identity(), useInstances: 1,
	}}
	importer := svgImporter{
		ctx: &cancelAfterContext{remaining: 3}, source: "cancel.svg",
		limits: Limits{MaxResources: 10, MaxPathCommands: 2048}, ids: map[string]*svgElement{"c": resource},
	}
	clip, err := importer.instantiateClipPath("c")
	if clip != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("clip/error = %+v/%v", clip, err)
	}
	if importer.useInstances != 0 || importer.emittedCommands != 0 {
		t.Fatalf("canceled expansion committed metrics: uses=%d commands=%d", importer.useInstances, importer.emittedCommands)
	}
}

func ExampleImportNode_localClipPath() {
	result, err := ImportNode(context.Background(), "clip.svg", []byte(`<svg width="2" height="1"><clipPath id="c"><rect width="1" height="1"/></clipPath><rect width="2" height="1" clip-path="url(#c)"/></svg>`), generousImportLimits())
	fmt.Println(err, result.Root.Children[0].Clip != nil)
	// Output: <nil> true
}
