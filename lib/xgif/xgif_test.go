package xgif

import (
	"bytes"
	_ "embed"
	"image/png"
	"testing"

	"github.com/stretchr/testify/assert"
	"oss.terrastruct.com/util-go/go2"
)

//go:embed test_input1.png
var test_input1 []byte

//go:embed test_input2.png
var test_input2 []byte

//go:embed test_output.gif
var test_output []byte

func TestPngToGif(t *testing.T) {
	board1, err := png.Decode(bytes.NewBuffer(test_input1))
	assert.NoError(t, err)
	gifWidth := board1.Bounds().Dx()
	gifHeight := board1.Bounds().Dy()

	board2, err := png.Decode(bytes.NewBuffer(test_input2))
	assert.NoError(t, err)
	gifWidth = go2.Max(board2.Bounds().Dx(), gifWidth)
	gifHeight = go2.Max(board2.Bounds().Dy(), gifHeight)

	boards := [][]byte{test_input1, test_input2}
	interval := 1_000
	gifBytes, err := AnimatePNGs(boards, gifWidth, gifHeight, interval)
	assert.NoError(t, err)

	assert.Equal(t, test_output, gifBytes)
}
