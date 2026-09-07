package png_test

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// Compare decoded pixels, so lossless PNG encoder or metadata changes alone
// do not churn the goldens. A sparse one-quantum RGB allowance covers floating
// point text antialiasing differences between arm64 and amd64. Like diff.Testdata,
// only mismatches outside that bound keep .got files.
func pngSnapshot(path string, data []byte, accept bool) error {
	got, err := decodePNG(data)
	if err != nil {
		return fmt.Errorf("invalid isometric render %s: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	gotPath, expPath := path+".got.png", path+".exp.png"
	if err := os.WriteFile(gotPath, data, 0600); err != nil {
		return err
	}
	expected, err := os.ReadFile(expPath)
	if err == nil {
		exp, decodeErr := decodePNG(expected)
		if decodeErr == nil && pngPixelsMatch(exp, got) {
			return os.Remove(gotPath)
		}
		if decodeErr != nil && !accept {
			return fmt.Errorf("invalid expected isometric PNG %s: %w", expPath, decodeErr)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if accept {
		return os.Rename(gotPath, expPath)
	}
	return fmt.Errorf("isometric pixels differ: %s vs %s (review the expected and got PNG files; rerun with TESTDATA_ACCEPT=1 or TA=1 to accept)", expPath, gotPath)
}

func pngPixelsMatch(expected, got *image.RGBA) bool {
	if expected.Bounds() != got.Bounds() {
		return false
	}
	if bytes.Equal(expected.Pix, got.Pix) {
		return true
	}
	// At most 0.01% of pixels may differ, and only by one RGB quantum.
	// Dimensions and alpha are exact; images below 10,000 pixels stay exact.
	limit := expected.Rect.Dx() * expected.Rect.Dy() / 10000
	changed := 0
	for y := expected.Rect.Min.Y; y < expected.Rect.Max.Y; y++ {
		for x := expected.Rect.Min.X; x < expected.Rect.Max.X; x++ {
			a, b := expected.PixOffset(x, y), got.PixOffset(x, y)
			if expected.Pix[a+3] != got.Pix[b+3] {
				return false
			}
			different := false
			for channel := 0; channel < 3; channel++ {
				delta := int(expected.Pix[a+channel]) - int(got.Pix[b+channel])
				if delta < -1 || delta > 1 {
					return false
				}
				different = different || delta != 0
			}
			if different {
				changed++
				if changed > limit {
					return false
				}
			}
		}
	}
	return true
}

func decodePNG(data []byte) (*image.RGBA, error) {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	rgba := image.NewRGBA(img.Bounds())
	draw.Draw(rgba, rgba.Bounds(), img, img.Bounds().Min, draw.Src)
	return rgba, nil
}

func TestSnapshotPixels(t *testing.T) {
	path := filepath.Join(t.TempDir(), "isometric")
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	encode := func(level png.CompressionLevel) []byte {
		var b bytes.Buffer
		encoder := png.Encoder{CompressionLevel: level}
		if err := encoder.Encode(&b, img); err != nil {
			t.Fatal(err)
		}
		return b.Bytes()
	}
	data := encode(png.DefaultCompression)
	if err := pngSnapshot(path, data, false); err == nil {
		t.Fatal("missing expected PNG passed")
	}
	if err := pngSnapshot(path, data, true); err != nil {
		t.Fatal(err)
	}
	if err := pngSnapshot(path, encode(png.NoCompression), false); err != nil {
		t.Fatalf("lossless encoding change failed: %v", err)
	}
	if _, err := os.Stat(path + ".got.png"); !os.IsNotExist(err) {
		t.Fatal("unchanged render retained got file")
	}
	img.Pix[0], img.Pix[3] = 255, 255
	if err := pngSnapshot(path, encode(png.DefaultCompression), false); err == nil {
		t.Fatal("changed pixel passed")
	}
	expected, err := os.ReadFile(path + ".exp.png")
	if err != nil || !bytes.Equal(expected, data) {
		t.Fatal("unaccepted pixel change modified expected snapshot")
	}
}

func TestPixelRoundingBound(t *testing.T) {
	newImage := func(width, height int) *image.RGBA {
		img := image.NewRGBA(image.Rect(0, 0, width, height))
		for i := 0; i < len(img.Pix); i += 4 {
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = 100, 120, 140, 255
		}
		return img
	}
	for _, tc := range []struct {
		name          string
		width, height int
		change        func(*image.RGBA)
		want          bool
	}{
		{"exact", 100, 100, func(*image.RGBA) {}, true},
		{"one pixel with both RGB directions", 100, 100, func(img *image.RGBA) {
			img.Pix[0]++
			img.Pix[1]--
			img.Pix[2]++
		}, true},
		{"at density limit", 143, 140, func(img *image.RGBA) {
			img.Pix[0]++
			img.Pix[len(img.Pix)-2]--
		}, true},
		{"above density limit", 143, 140, func(img *image.RGBA) {
			img.Pix[0]++
			img.Pix[4]++
			img.Pix[len(img.Pix)-2]--
		}, false},
		{"two RGB quanta", 100, 100, func(img *image.RGBA) { img.Pix[0] += 2 }, false},
		{"alpha remains exact", 100, 100, func(img *image.RGBA) { img.Pix[3]-- }, false},
		{"tiny image remains exact", 99, 101, func(img *image.RGBA) { img.Pix[0]++ }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expected, got := newImage(tc.width, tc.height), newImage(tc.width, tc.height)
			tc.change(got)
			if match := pngPixelsMatch(expected, got); match != tc.want {
				t.Fatalf("match = %v, want %v", match, tc.want)
			}
		})
	}
	if pngPixelsMatch(newImage(100, 100), newImage(100, 101)) {
		t.Fatal("different dimensions passed")
	}
}
