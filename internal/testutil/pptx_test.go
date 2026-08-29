package testutil

import (
	"archive/zip"
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidatePPTXRejectsNonZIP(t *testing.T) {
	t.Parallel()

	assert.EqualError(t, ValidatePPTX([]byte("not a ZIP archive"), nil, 1), "error reading pptx content: zip: not a valid zip file")
}

func TestExpectedPPTXFileCount(t *testing.T) {
	t.Parallel()

	var template bytes.Buffer
	w := zip.NewWriter(&template)
	for _, name := range []string{"one", "two", "three"} {
		_, err := w.Create(name)
		assert.NoError(t, err)
	}
	assert.NoError(t, w.Close())

	assert.Equal(t, 8, getExpectedPptxFileCount(template.Bytes(), 0))
	assert.Equal(t, 20, getExpectedPptxFileCount(template.Bytes(), 4))
	assert.Equal(t, -1, getExpectedPptxFileCount([]byte("invalid"), 1))
}
