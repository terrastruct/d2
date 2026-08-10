package pptx_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/d2lang/d2/internal/testutil"
	"github.com/d2lang/d2/lib/pptx"
)

func TestValidateCompatibility(t *testing.T) {
	t.Parallel()

	content := []byte("not a pptx")
	want := testutil.ValidatePPTX(content, pptx.PPTX_TEMPLATE, 1)
	got := pptx.Validate(content, 1)
	assert.EqualError(t, got, want.Error())
}
