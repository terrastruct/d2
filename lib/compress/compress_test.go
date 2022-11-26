package compress

import (
	"testing"

	"oss.terrastruct.com/diff"
)

func TestCompression(t *testing.T) {
	script := `x -> y
I just forgot my whole philosophy of life!!!: {
  s: TV is chewing gum for the eyes
}
`

	encoded, err := Compress(script)
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := Decompress(encoded)
	if err != nil {
		t.Fatal(err)
	}

	diff.AssertStringEq(t, script, decoded)
}
