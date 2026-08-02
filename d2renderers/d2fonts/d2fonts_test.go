package d2fonts

import (
	"path/filepath"
	"strings"
	"testing"

	"oss.terrastruct.com/d2/internal/testdiff"
	"oss.terrastruct.com/d2/lib/font"
	"oss.terrastruct.com/util-go/assert"
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
	err := testdiff.Testdata(filepath.Join("testdata", "d2fonts", "cut"), ".txt", fontBuf)
	assert.Success(t, err)
}

func TestFontEncodingsHaveNoNewlines(t *testing.T) {
	FontEncodings.Range(func(f Font, encoding string) bool {
		if strings.ContainsAny(encoding, "\r\n") {
			t.Fatalf("font encoding for %s/%s contains a newline", f.Family, f.Style)
		}
		return true
	})
}

func TestTrimFontEncoding(t *testing.T) {
	tcs := []struct {
		in  string
		out string
	}{
		{in: "abc\r\n", out: "abc"},
		{in: "abc\n", out: "abc"},
		{in: "abc\r", out: "abc"},
		{in: "ab\nc", out: "ab\nc"},
	}

	for _, tc := range tcs {
		if got := trimFontEncoding(tc.in); got != tc.out {
			t.Fatalf("trimFontEncoding(%q) = %q, want %q", tc.in, got, tc.out)
		}
	}
}
