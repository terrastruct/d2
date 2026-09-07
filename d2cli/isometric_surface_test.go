package d2cli

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestIsometricImageLocalSurfaceIconsWithoutExecutables(t *testing.T) {
	var raster bytes.Buffer
	art := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 8; y < 56; y++ {
		for x := 8; x < 56; x++ {
			art.SetNRGBA(x, y, color.NRGBA{R: 255, B: 180, A: 255})
		}
	}
	if err := png.Encode(&raster, art); err != nil {
		t.Fatal(err)
	}
	for _, asset := range []struct {
		name string
		data []byte
	}{
		{"mark.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="64" height="64" viewBox="0 0 64 64"><path fill="#ff00b4" d="M8 8H56V56H8Z"/></svg>`)},
		{"mark.png", raster.Bytes()},
	} {
		t.Run(asset.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, asset.name), asset.data, 0600); err != nil {
				t.Fatal(err)
			}
			source := "service: Service {\n  icon: ./" + asset.name + "\n}\n"
			data, err := imageCLI(t, dir, source, "--isometric", "--scale=.4", "--stdout-format=png", "-")
			if err != nil {
				t.Fatal(err)
			}
			img, err := png.Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			pink := 0
			for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
				for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
					c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
					if c.R > 220 && c.G < 50 && c.B > 130 && c.A == 255 {
						pink++
					}
				}
			}
			if pink < 40 {
				t.Fatalf("surface icon artwork did not reach native output: only %d pink pixels", pink)
			}
			vector, err := imageCLI(t, dir, source, "--isometric", "--scale=.4", "--stdout-format=svg", "-")
			if err != nil {
				t.Fatal(err)
			}
			inspectIsometricSVG(t, vector)
			hasRaster := bytes.Contains(vector, []byte("data:image/png;base64,"))
			if hasRaster != (asset.name == "mark.png") {
				t.Fatalf("SVG icon should remain vector and PNG artwork should be embedded: %s, raster=%v", asset.name, hasRaster)
			}
		})
	}
}
