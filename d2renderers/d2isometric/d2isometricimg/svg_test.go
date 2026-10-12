package d2isometricimg

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"image/color"
	"io"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2isometric"
)

func TestNativeSVGVectorGeometryAndText(t *testing.T) {
	diagram := captureDiagram()
	diagram.Shapes[0].Link = "https://example.com/details?a=1&b=2"
	diagram.Shapes[0].Tooltip = `Worker <details> & "queue"`
	before, _ := json.Marshal(diagram)
	options := &Options{Format: SVG, Width: 640, Height: 400, FitContent: true, Render: d2isometric.RenderOpts{}}
	first, err := Render(context.Background(), diagram, options)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Render(context.Background(), diagram, options)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("SVG output is not deterministic")
	}
	after, _ := json.Marshal(diagram)
	if !bytes.Equal(before, after) {
		t.Fatal("SVG export changed source diagram")
	}
	decoder := xml.NewDecoder(bytes.NewReader(first))
	paths, images, anchors := 0, 0, 0
	ids := make(map[string]bool)
	var references []string
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		element, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		switch element.Name.Local {
		case "path":
			paths++
		case "image":
			images++
		case "a":
			anchors++
		}
		for _, a := range element.Attr {
			if a.Name.Local == "id" {
				if ids[a.Value] {
					t.Fatalf("duplicate SVG resource ID %q", a.Value)
				}
				ids[a.Value] = true
			}
			if strings.HasPrefix(a.Value, "url(#") && strings.HasSuffix(a.Value, ")") {
				references = append(references, strings.TrimSuffix(strings.TrimPrefix(a.Value, "url(#"), ")"))
			}
			if a.Name.Local == "href" && strings.HasPrefix(a.Value, "#") {
				references = append(references, a.Value[1:])
			}
		}
	}
	if paths < 20 || images != 0 || anchors != 1 {
		t.Fatalf("expected vector solids and glyphs with one link; paths=%d images=%d anchors=%d", paths, images, anchors)
	}
	for _, ref := range references {
		if !ids[ref] {
			t.Fatalf("unresolved SVG resource %q", ref)
		}
	}
	if bytes.Contains(first, []byte("data:image")) {
		t.Fatal("plain SVG diagram embeds raster pixels")
	}
}

func TestNativeSVGVisibilityRemovesHiddenFaces(t *testing.T) {
	red := &Material{Color: color.NRGBA{R: 255, A: 255}, Unlit: true}
	green := &Material{Color: color.NRGBA{G: 255, A: 255}, Unlit: true}
	camera := nativeCameraAxes()
	camera.width, camera.height, camera.scale = 240, 200, 60
	for _, reverse := range []bool{false, true} {
		triangles := append(rasterTestQuad(-1, red), rasterTestQuad(1, green)...)
		if reverse {
			triangles = append(rasterTestQuad(1, green), rasterTestQuad(-1, red)...)
		}
		s := &nativeScene{triangles: triangles, camera: camera, width: 240, height: 200, background: color.NRGBA{255, 255, 255, 255}}
		data, err := nativeSceneSVG(context.Background(), s)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(data, []byte("#ff0000")) || !bytes.Contains(data, []byte("#00ff00")) {
			t.Fatalf("hidden geometry survives SVG visibility, reverse=%v", reverse)
		}
	}
}

func TestNativeSVGAdmissionAndCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Render(ctx, captureDiagram(), &Options{Format: SVG}); !errors.Is(err, context.Canceled) {
		t.Fatalf("SVG ignored cancellation: %v", err)
	}
	if _, err := Render(context.Background(), nil, &Options{Format: SVG}); err == nil {
		t.Fatal("SVG admitted nil diagram")
	}
	if _, err := Render(context.Background(), captureDiagram(), &Options{Format: SVG, Width: 1, Height: 1}); err == nil {
		t.Fatal("SVG admitted invalid viewport")
	}
	d := captureDiagram()
	d.Shapes[0].Tooltip = "invalid\x00XML"
	if _, err := Render(context.Background(), d, &Options{Format: SVG}); err == nil {
		t.Fatal("SVG admitted invalid XML metadata")
	}
}

func TestNativeSVGCoveragePreservesPhysicalGeometry(t *testing.T) {
	diagram := sourcePanelFixture(t, "stable/all_shapes/elk/board.exp.json")
	scene, err := d2isometric.BuildScene(diagram, &d2isometric.RenderOpts{})
	if err != nil {
		t.Fatal(err)
	}
	build := func(vector bool) []Triangle {
		ctx := nativeVectorContext(context.Background())
		native, err := newNativeSceneWithOptions(ctx, scene, 1200, 1200, nil, nil, nativeSceneOptions{deferRaster: true, vector: vector, fitContent: true, outputDensity: sceneOutputDensity(scene, 1200, 1200, nil)})
		if err != nil {
			t.Fatal(err)
		}
		return native.triangles
	}
	raster, vector := build(false), build(true)
	physical := make([]Triangle, 0, len(vector))
	coverage := 0
	for _, triangle := range vector {
		if triangle.svgCoverageOnly {
			coverage++
			if triangle.CastShadow || triangle.Material.Texture != nil {
				t.Fatal("SVG coverage proxy changes the physical shadow or paint")
			}
			continue
		}
		physical = append(physical, triangle)
	}
	if coverage == 0 || len(physical) != len(raster) {
		t.Fatalf("SVG changed physical geometry: raster=%d vector=%d proxies=%d", len(raster), len(physical), coverage)
	}
	for i, triangle := range physical {
		want := raster[i]
		if triangle.V != want.V || triangle.CastShadow != want.CastShadow || triangle.DepthBias != want.DepthBias || triangle.NoDepthWrite != want.NoDepthWrite {
			t.Fatalf("SVG changed the physical surface or ink silhouette at triangle %d", i)
		}
	}
}
