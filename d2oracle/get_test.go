package d2oracle_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"oss.terrastruct.com/d2/d2oracle"
)

func TestIsLabelKeyID(t *testing.T) {
	t.Parallel()

	assert.Equal(t, true, d2oracle.IsLabelKeyID("x", "x"))
	assert.Equal(t, true, d2oracle.IsLabelKeyID("y.x", "x"))
	assert.Equal(t, true, d2oracle.IsLabelKeyID(`x."y.x"`, "y.x"))

	assert.Equal(t, false, d2oracle.IsLabelKeyID("x", "y"))
	assert.Equal(t, false, d2oracle.IsLabelKeyID("x->y", "y"))
}
