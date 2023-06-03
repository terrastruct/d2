package d2fonts

import (
	"path/filepath"
	"testing"

	"oss.terrastruct.com/d2/lib/font"
	"oss.terrastruct.com/util-go/assert"
	"oss.terrastruct.com/util-go/diff"
)

func TestCutFont(t *testing.T) {
	f := Font{
		Family: SourceCodePro,
		Style:  FONT_STYLE_BOLD,
	}
	fontBuf := make([]byte, len(FontFaces[f]))
	copy(fontBuf, FontFaces[f])
	fontBuf = font.UTF8CutFont(fontBuf, " 1")
	err := diff.Testdata(filepath.Join("testdata", "d2fonts", "cut"), ".txt", fontBuf)
	assert.Success(t, err)
}
