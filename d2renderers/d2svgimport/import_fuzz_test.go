package d2svgimport

import (
	"bytes"
	"context"
	"image/color"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2scene"
)

func FuzzImportNode(f *testing.F) {
	embeddedPNG := embeddedRasterPNG(f, 2, 2, color.NRGBA{R: 0xff, A: 0xff})
	for _, seed := range [][]byte{
		[]byte(`<svg viewBox="0 0 10 10"><path d="M0 0L10 10"/></svg>`),
		[]byte(`<svg width="20" height="10" preserveAspectRatio="xMaxYMid slice"><defs><rect id="r" width="2" height="3"/></defs><use href="#r"/></svg>`),
		[]byte(`<svg width="10" height="10"><style>.paint{fill:red;stroke:black;stroke-width:2}</style><rect class="paint" width="10" height="10"/></svg>`),
		[]byte(`<svg width="10" height="10"><defs><linearGradient id="g" x1="0" y1="0" x2="10" y2="10" gradientUnits="userSpaceOnUse" gradientTransform="translate(1 2)"><stop offset="0" stop-color="#f00"/><stop offset="1" style="stop-color:#00f;stop-opacity:.5"/></linearGradient></defs><rect width="10" height="10" fill="url(#g)"/></svg>`),
		[]byte(`<svg width="10" height="10"><defs><path id="shape" d="M0 0H5V10H0Z"/></defs><clipPath id="clip"><use href="#shape" overflow="visible"/></clipPath><rect width="10" height="10" clip-path="url(#clip)"/></svg>`),
		[]byte(`<svg xmlns="http://www.w3.org/2000/svg" version="1.1" x="0px" y="0px" viewBox="0 0 10 10" data-name="fuzz" role="img" focusable="false" style="enable-background:new 0 0 10 10"><title>bounded title</title><rect width="10" height="10"/></svg>`),
		[]byte(`<svg xmlns="http://www.w3.org/2000/svg" style="vertical-align:-0.25ex" width="2.5ex" height="1.25ex" role="img" focusable="false" viewBox="0 0 10 5"><g transform="scale(1,-1)"><path data-c="41" d="M0 0H10V5H0Z"/></g></svg>`),
		[]byte(minimalFrozenMathJaxTextSVG),
		[]byte(`<svg width="2" height="2"><image width="2" height="2" href="` + embeddedRasterDataURI("image/png", embeddedPNG) + `"/></svg>`),
		[]byte(`<svg width="2" height="2"><image width="2" height="2" href="https://example.invalid/icon.png"/></svg>`),
		[]byte(`<svg width="20" height="10" viewBox="0 0 20 10"><svg x="2" y="1" width="8" height="6" viewBox="-1 -2 4 3" preserveAspectRatio="xMaxYMid slice"><path d="M-1-2H3V1H-1Z"/></svg></svg>`),
		[]byte(`<svg xmlns="http://www.w3.org/2000/svg" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:cc="http://creativecommons.org/ns#" xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" viewBox="0 0 1 1"><metadata><rdf:RDF><cc:Work rdf:about=""><dc:format>image/svg+xml</dc:format><dc:type rdf:resource="http://purl.org/dc/dcmitype/StillImage"/></cc:Work></rdf:RDF></metadata><path d="M0 0L1 1"/></svg>`),
		[]byte(`<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape" xmlns:sodipodi="http://sodipodi.sourceforge.net/DTD/sodipodi-0.dtd" viewBox="0 0 1 1"><sodipodi:namedview inkscape:zoom="1"/><path inkscape:connector-curvature="0" sodipodi:nodetypes="cc" d="M0 0L1 1"/></svg>`),
		[]byte(`<svg xmlns="http://www.w3.org/2000/svg" xmlns:evil="urn:evil" viewBox="0 0 1 1"><metadata><evil:script href="https://example.invalid/x"/></metadata></svg>`),
		[]byte(`<svg><script>alert(1)</script></svg>`),
		[]byte(`<?xml version="1.0"?><!--editor--><!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd"><svg width="1" height="1"/>`),
		[]byte(`<!DOCTYPE svg [<!ENTITY bomb "boom">]><svg>&bomb;</svg>`),
		[]byte(`<svg width="1" height="1"><g id="x"><use href="#x"/></g></svg>`),
		{0, 1, 2, '<', '&', 0xff},
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input []byte) {
		limits := Limits{
			MaxBytes: 32 << 10, MaxDepth: 64, MaxElements: 512, MaxAttributes: 1024,
			MaxAttributeBytes: 24 << 10, MaxPathCommands: 4096, MaxTransformFunctions: 512,
			MaxUseDepth: 32, MaxResources: 512,
		}
		original := append([]byte(nil), input...)
		result, err := ImportNode(context.Background(), "https://user:secret@example.invalid/input.svg?token=hidden", input, limits)
		if !bytes.Equal(input, original) {
			t.Fatal("ImportNode mutated caller input")
		}
		if err != nil {
			if result != nil {
				t.Fatal("ImportNode returned a partial result with an error")
			}
			return
		}
		if result == nil || result.Root == nil {
			t.Fatal("successful import returned nil result/root")
		}
		if !result.ViewportTransform.IsFinite() || result.Width <= 0 || result.Height <= 0 {
			t.Fatalf("successful import returned invalid viewport: %+v", result)
		}
		assetBytes := 0
		decodedBytes := int64(0)
		for id, raw := range result.Assets {
			asset, ok := raw.(d2scene.RasterAsset)
			if !ok || id == "" || asset.MIMEType == "" || len(asset.Data) == 0 || asset.PixelWidth <= 0 || asset.PixelHeight <= 0 || asset.DecodedBytes <= 0 {
				t.Fatalf("successful import produced invalid raster asset %q: %#v", id, raw)
			}
			assetBytes += len(asset.Data)
			decodedBytes += asset.DecodedBytes
		}
		if assetBytes > limits.MaxBytes || decodedBytes > int64(limits.MaxBytes) ||
			result.Metrics.EmbeddedRasterAssets != len(result.Assets) || result.Metrics.EmbeddedRasterBytes != assetBytes ||
			result.Metrics.DecodedRasterBytes != decodedBytes {
			t.Fatalf("successful import exceeded or misreported raster budgets: assets=%d decoded=%d metrics=%+v", assetBytes, decodedBytes, result.Metrics)
		}
	})
}
