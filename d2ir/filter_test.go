package d2ir_test

import (
	"testing"

	"oss.terrastruct.com/util-go/assert"
)

func testCompileFilters(t *testing.T) {
	t.Parallel()

	tca := []testCase{
		{
			name: "escaped",
			run: func(t testing.TB) {
				m, err := compile(t, `jacob: {
	shape: circle
}
jeremy: {
	shape: rectangle
}
*: {
	&shape: rectangle
	label: I'm a rectangle
}`)
				assert.Success(t, err)
				t.Log(m.String())
				assertQuery(t, m, 1, 0, nil, "jacob")
				assertQuery(t, m, 2, 0, "", "jeremy")
				assertQuery(t, m, 0, 0, "I'm a rectangle", "jeremy.label")
			},
		},
	}

	runa(t, tca)
}
