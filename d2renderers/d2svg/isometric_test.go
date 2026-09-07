package d2svg

import (
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"math"
	"net/url"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/d2renderers/d2isometric/d2isometricimg"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/util-go/go2"
)

func isometricAPIDiagram() *d2target.Diagram {
	d := d2target.NewDiagram()
	d.FontFamily = go2.Pointer(d2fonts.SourceSansPro)
	d.MonoFontFamily = go2.Pointer(d2fonts.SourceCodePro)
	n := *d2target.BaseShape()
	n.ID, n.Type, n.Width, n.Height = "database", d2target.ShapeCylinder, 160, 100
	n.Fill, n.Stroke = "#fffdf6", "#304552"
	n.Label, n.LabelWidth, n.LabelHeight = "Database", 68, 20
	n.Link = "https://example.com/docs#database"
	d.Shapes = []d2target.Shape{n}
	return d
}

func TestRenderIsometricModeAndDocumentOptions(t *testing.T) {
	d := isometricAPIDiagram()
	options := &RenderOpts{Isometric: go2.Pointer(true), Pad: go2.Pointer(int64(17)), Scale: go2.Pointer(.5), Center: go2.Pointer(true), Salt: go2.Pointer("one"), NoXMLTag: go2.Pointer(true), OmitVersion: go2.Pointer(true)}
	out, err := Render(d, options)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`d2-isometric`, `<title>D2 isometric diagram</title>`, `preserveAspectRatio="xMidYMid meet"`, `viewBox="-17 -17 `, `https://example.com/docs#database`} {
		if !bytes.Contains(out, []byte(want)) {
			t.Errorf("missing %s", want)
		}
	}
	if bytes.Contains(out, []byte("<?xml")) || bytes.Contains(out, []byte("data-d2-version")) {
		t.Fatal("omitted document metadata was emitted")
	}
	ids := map[string]bool{}
	refs := []string{}
	var outerWidth, innerWidth string
	decoder := xml.NewDecoder(bytes.NewReader(out))
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		for _, attr := range start.Attr {
			if start.Name.Local == "svg" && attr.Name.Local == "width" {
				if outerWidth == "" {
					outerWidth = attr.Value
				} else {
					innerWidth = attr.Value
				}
			}
			if attr.Name.Local == "id" {
				if ids[attr.Value] {
					t.Fatalf("duplicate resource ID %s", attr.Value)
				}
				ids[attr.Value] = true
			}
			if strings.HasPrefix(attr.Value, "url(#") {
				refs = append(refs, strings.TrimSuffix(strings.TrimPrefix(attr.Value, "url(#"), ")"))
			}
			if attr.Name.Local == "href" && strings.HasPrefix(attr.Value, "#") {
				refs = append(refs, strings.TrimPrefix(attr.Value, "#"))
			}
		}
	}
	if outerWidth == "" || innerWidth == "" || outerWidth == innerWidth {
		t.Fatalf("scale did not affect only outer dimensions: %s, %s", outerWidth, innerWidth)
	}
	if len(ids) == 0 || len(refs) == 0 {
		t.Fatal("fixture did not exercise SVG resource references")
	}
	for _, ref := range refs {
		if !ids[ref] {
			t.Errorf("unresolved resource %s", ref)
		}
	}
	options.Salt = go2.Pointer("two")
	second, err := Render(d, options)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range isometricSVGID.FindAllSubmatch(second, -1) {
		if ids[string(id[1])] {
			t.Errorf("salt did not isolate resource %s", id[1])
		}
	}

	flat, err := Render(d, &RenderOpts{Isometric: go2.Pointer(false)})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(flat, []byte("d2-isometric")) {
		t.Fatal("explicit false rendered isometric output")
	}
}

func TestRenderIsometricRejectsIncompatibleModes(t *testing.T) {
	for _, test := range []struct {
		name string
		opts RenderOpts
		want string
	}{
		{"sketch", RenderOpts{Sketch: go2.Pointer(true)}, "sketch cannot"},
		{"dark theme", RenderOpts{DarkThemeID: go2.Pointer(int64(200))}, "responsive dark themes"},
		{"dark overrides", RenderOpts{DarkThemeOverrides: &d2target.ThemeOverrides{}}, "responsive dark themes"},
		{"animation", RenderOpts{MasterID: "master"}, "multi-board SVG animation"},
		{"infinite scale", RenderOpts{Scale: go2.Pointer(math.Inf(1))}, "scale"},
		{"negative scale", RenderOpts{Scale: go2.Pointer(-1.)}, "scale"},
		{"overflow padding", RenderOpts{Pad: go2.Pointer(int64(math.MaxInt64))}, "padding"},
		{"empty viewport", RenderOpts{Pad: go2.Pointer(int64(-10000))}, "padding"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := RenderIsometric(context.Background(), isometricAPIDiagram(), &test.opts, &d2isometricimg.Options{Width: 160, Height: 100})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %s", err, test.want)
			}
		})
	}
}

func TestRenderIsometricMultiboardIsStaticSVG(t *testing.T) {
	d := isometricAPIDiagram()
	child := isometricAPIDiagram()
	child.Name = "detail"
	d.Layers = []*d2target.Diagram{child}
	boards, err := RenderMultiboard(d, &RenderOpts{Isometric: go2.Pointer(true)})
	if err != nil {
		t.Fatal(err)
	}
	if len(boards) != 2 {
		t.Fatalf("got %d boards, want 2", len(boards))
	}
	for _, board := range boards {
		if !bytes.Contains(board, []byte("d2-isometric")) {
			t.Fatal("board lost mode")
		}
	}
}

func TestRenderIsometricRequiresResolverForExternalImages(t *testing.T) {
	diagram := isometricAPIDiagram()
	diagram.Shapes[0].Icon, _ = url.Parse("https://example.com/icon.svg")
	_, err := Render(diagram, &RenderOpts{Isometric: go2.Pointer(true)})
	if err == nil || !strings.Contains(err.Error(), "asset resolver") {
		t.Fatalf("error = %v, want explicit external asset resolver requirement", err)
	}
}
