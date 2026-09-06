package testutil

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/d2lang/d2/internal/testutil/imagediff"
)

func TestValidateGIF(t *testing.T) {
	t.Parallel()

	frame := image.NewPaletted(image.Rect(0, 0, 2, 2), color.Palette{color.White})
	anim := &gif.GIF{
		Image:     []*image.Paletted{frame, frame},
		Delay:     []int{2, 2},
		LoopCount: infiniteLoop,
		Config: image.Config{
			Width:  2,
			Height: 2,
		},
	}
	var out bytes.Buffer
	assert.NoError(t, gif.EncodeAll(&out, anim))
	assert.NoError(t, ValidateGIF(out.Bytes(), 2, 20))
	assert.EqualError(t, ValidateGIF(out.Bytes(), 2, 10), "expected interval between frames to be 1, got=2 at frame=0")
}

func TestCompareGIFChecksCompositedPixelsOrderAndTiming(t *testing.T) {
	t.Parallel()

	red := color.NRGBA{R: 255, A: 255}
	blue := color.NRGBA{B: 255, A: 255}
	expected := encodeGIF(t, []color.NRGBA{red, blue}, []int{3, 4})
	reordered := encodeGIF(t, []color.NRGBA{blue, red}, []int{3, 4})
	comparison, err := CompareGIF(expected, reordered, GIFCompareOptions{RequireFrameChange: true})
	if err == nil || comparison == nil || comparison.FrameIndex != 0 || comparison.FrameResult == nil {
		t.Fatalf("reordered GIF comparison = %#v/%v", comparison, err)
	}
	var mismatch *imagediff.MismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("reordered GIF error = %v, want pixel mismatch", err)
	}
	if strings.Count(string(comparison.FrameResult.ReportHTML), "data:image/png;base64,") != 4 {
		t.Fatal("GIF pixel mismatch did not retain self-contained frame diagnostics")
	}

	wrongDelay := encodeGIF(t, []color.NRGBA{red, blue}, []int{2, 5})
	comparison, err = CompareGIF(expected, wrongDelay, GIFCompareOptions{RequireFrameChange: true})
	if err == nil || comparison == nil || !strings.Contains(err.Error(), "frame delays differ") ||
		comparison.Expected.TotalDurationCentiseconds != 7 || comparison.Actual.TotalDurationCentiseconds != 7 {
		t.Fatalf("wrong-delay GIF comparison = %#v/%v", comparison, err)
	}

	constant := encodeGIF(t, []color.NRGBA{red, red}, []int{3, 4})
	if _, err := CompareGIF(constant, constant, GIFCompareOptions{RequireFrameChange: true}); err == nil || !strings.Contains(err.Error(), "never change") {
		t.Fatalf("constant GIF change check = %v", err)
	}
}

func TestInspectGIFPreservesSubSecondAndFractionalDuration(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		delays []int
		wantCS int
	}{
		{name: "sub-second", delays: []int{3, 3, 4, 3, 3, 4, 3, 3, 4}, wantCS: 30},
		{name: "fractional-second", delays: []int{3, 3, 4, 3, 3, 4, 3, 3, 4, 3, 3, 4, 3, 3, 4, 3}, wantCS: 53},
	} {
		t.Run(test.name, func(t *testing.T) {
			colors := make([]color.NRGBA, len(test.delays))
			for index := range colors {
				colors[index] = color.NRGBA{R: uint8(index + 1), A: 255}
			}
			inspection, err := InspectGIF(encodeGIF(t, colors, test.delays))
			if err != nil {
				t.Fatal(err)
			}
			if len(inspection.FrameHashes) != len(test.delays) || inspection.TotalDurationCentiseconds != test.wantCS || inspection.ChangedFramePairs != len(test.delays)-1 {
				t.Fatalf("inspection = %+v", inspection)
			}
		})
	}
}

func encodeGIF(t *testing.T, colors []color.NRGBA, delays []int) []byte {
	t.Helper()
	if len(colors) != len(delays) {
		t.Fatal("test GIF colors/delays differ")
	}
	palette := color.Palette{color.Transparent}
	for _, value := range colors {
		palette = append(palette, value)
	}
	animation := &gif.GIF{
		Delay:     append([]int(nil), delays...),
		LoopCount: infiniteLoop,
		Config:    image.Config{Width: 2, Height: 2, ColorModel: palette},
	}
	for index := range colors {
		frame := image.NewPaletted(image.Rect(0, 0, 2, 2), palette)
		for pixel := range frame.Pix {
			frame.Pix[pixel] = uint8(index + 1)
		}
		animation.Image = append(animation.Image, frame)
	}
	var output bytes.Buffer
	if err := gif.EncodeAll(&output, animation); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}
