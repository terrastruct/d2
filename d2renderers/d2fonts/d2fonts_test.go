package d2fonts

import (
	"path/filepath"
	"testing"

	"github.com/jung-kurt/gofpdf"
	"oss.terrastruct.com/util-go/assert"
	"oss.terrastruct.com/util-go/diff"
)

func TestCutFont(t *testing.T) {
	f := Font{
		Family: SourceCodePro,
		Style:  FONT_STYLE_REGULAR,
	}
	fontBuf := FontFaces[f]
	fontBuf = gofpdf.UTF8CutFont(fontBuf, "a")
	err := diff.Testdata(filepath.Join("testdata", "d2fonts", "cut"), ".txt", fontBuf)
	assert.Success(t, err)
}
