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

	decoded, err := Decode(encoded)
	assert.Success(t, err)

	assert.String(t, script, decoded)
}
