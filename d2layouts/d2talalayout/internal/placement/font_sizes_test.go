package placement

import (
	"slices"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2fonts"
)

func TestTALAFontSizesMatchD2AdapterContract(t *testing.T) {
	sizes := talaFontSizes()
	if !slices.Equal(sizes[:], d2fonts.FontSizes) {
		t.Fatalf("TALA font sizes = %v, D2 font sizes = %v", sizes, d2fonts.FontSizes)
	}
}
