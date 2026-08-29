package png

import (
	"bytes"
	"image"
	"image/color"
	stdpng "image/png"
	"testing"

	pngstruct "github.com/dsoprea/go-png-image-structure/v2"
)

func TestStaticRasterScale(t *testing.T) {
	if SCALE != 2 {
		t.Fatalf("SCALE = %v, want 2", SCALE)
	}
}

func TestAddExifPreservesPNGAndAddsMetadata(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	source.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	source.SetNRGBA(1, 0, color.NRGBA{B: 255, A: 255})
	var encoded bytes.Buffer
	if err := stdpng.Encode(&encoded, source); err != nil {
		t.Fatal(err)
	}

	withExif, err := AddExif(encoded.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := stdpng.Decode(bytes.NewReader(withExif))
	if err != nil {
		t.Fatal(err)
	}
	if !decoded.Bounds().Eq(source.Bounds()) {
		t.Fatalf("bounds = %v, want %v", decoded.Bounds(), source.Bounds())
	}
	for x := 0; x < source.Bounds().Dx(); x++ {
		got := color.NRGBAModel.Convert(decoded.At(x, 0)).(color.NRGBA)
		want := source.NRGBAAt(x, 0)
		if got != want {
			t.Fatalf("pixel %d = %#v, want %#v", x, got, want)
		}
	}

	parsed, err := pngstruct.NewPngMediaParser().ParseBytes(withExif)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parsed.(*pngstruct.ChunkSlice).FindExif(); err != nil {
		t.Fatalf("output has no EXIF chunk: %v", err)
	}
}
