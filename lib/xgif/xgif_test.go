package xgif

import (
	_ "embed"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

//go:embed test_input1.png
var test_input1 []byte

//go:embed test_input2.png
var test_input2 []byte

//go:embed test_output.gif
var test_output []byte

func TestPngToGif(t *testing.T) {
	boards := [][]byte{test_input1, test_input2}
	interval := 1_000
	gifBytes, err := AnimatePNGs(nil, boards, interval)
	assert.NoError(t, err)

	// use this to update the test output
	if false {
		f, err := os.Create("test_output_2.gif")
		assert.NoError(t, err)
		defer f.Close()
		f.Write(gifBytes)
	}

	assert.Equal(t, test_output, gifBytes)
}
