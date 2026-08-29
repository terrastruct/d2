package testutil

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"testing"

	"github.com/stretchr/testify/assert"
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
