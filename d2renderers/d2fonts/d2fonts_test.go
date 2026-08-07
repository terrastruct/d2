package d2fonts

import (
	"path/filepath"
	"testing"

	"github.com/d2lang/d2/lib/font"
	"github.com/d2lang/util-go/assert"
	"github.com/d2lang/util-go/diff"
)

func TestCutFont(t *testing.T) {
	f := Font{
		Family: SourceCodePro,
		Style:  FONT_STYLE_BOLD,
	}
	face := FontFaces.Get(f)
	fontBuf := make([]byte, len(face))
	copy(fontBuf, face)
	fontBuf = font.UTF8CutFont(fontBuf, " 1")
	err := diff.Testdata(filepath.Join("testdata", "d2fonts", "cut"), ".txt", fontBuf)
	assert.Success(t, err)
}
