package urlenc

import (
	"testing"

	"oss.terrastruct.com/util-go/assert"
)

func TestBasic(t *testing.T) {
	const script = `x -> y
I just forgot my whole philosophy of life!!!: {
  s: TV is chewing gum for the eyes
}
`

	encoded, err := Encode(script)
	assert.Success(t, err)

	// Make it explicit in PRs when encoding changes
	// Something we might want to know for playground compatability
	assert.Testdata(t, ".txt", []byte(encoded))

	decoded, err := Decode(encoded)
	assert.Success(t, err)

	assert.String(t, script, decoded)
}
