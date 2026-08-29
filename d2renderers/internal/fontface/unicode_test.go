package fontface

import (
	"testing"

	"github.com/go-text/typesetting/harfbuzz"
)

func TestIsDefaultIgnorableRune(t *testing.T) {
	tests := map[rune]bool{
		0x00ad:  true,
		0x034f:  true,
		0x061c:  true,
		0x115f:  false,
		0x1160:  false,
		0x180e:  true,
		0x180f:  false,
		0x200d:  true,
		0x2065:  true,
		0x3164:  false,
		0xfe0f:  true,
		0xffa0:  false,
		0x1bca0: false,
		0xe0100: true,
		0x0600:  false,
		0x0601:  false,
		0x06dd:  false,
		0x070f:  false,
		'A':     false,
	}
	for value, want := range tests {
		if got := IsDefaultIgnorableRune(value); got != want {
			t.Errorf("IsDefaultIgnorableRune(U+%04X) = %v, want %v", value, got, want)
		}
	}
}

func TestDefaultIgnorablePredicateMatchesPinnedShaper(t *testing.T) {
	for value := rune(0); value <= 0x10ffff; value++ {
		if got, want := IsDefaultIgnorableRune(value), harfbuzz.IsDefaultIgnorable(value); got != want {
			t.Fatalf("default-ignorable mismatch at U+%04X: scene=%v shaper=%v", value, got, want)
		}
	}
}
