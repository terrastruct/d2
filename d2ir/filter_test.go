package d2ir_test

import (
	"testing"

	"oss.terrastruct.com/util-go/assert"
)

func testCompileFilters(t *testing.T) {
	t.Parallel()

	tca := []testCase{
		{
			name: "base",
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
				assertQuery(t, m, 1, 0, nil, "jacob")
				assertQuery(t, m, 2, 0, nil, "jeremy")
				assertQuery(t, m, 0, 0, "I'm a rectangle", "jeremy.label")
			},
		},
		{
			name: "order",
			run: func(t testing.TB) {
				m, err := compile(t, `jacob: {
	shape: circle
}
jeremy: {
	shape: rectangle
}
*: {
	label: I'm a rectangle
	&shape: rectangle
}`)
				assert.Success(t, err)
				assertQuery(t, m, 1, 0, nil, "jacob")
				assertQuery(t, m, 2, 0, nil, "jeremy")
				assertQuery(t, m, 0, 0, "I'm a rectangle", "jeremy.label")
			},
		},
	}

	runa(t, tca)

	t.Run("errors", func(t *testing.T) {
		tca := []testCase{
			{
				name: "bad-syntax",
				run: func(t testing.TB) {
					_, err := compile(t, `jacob.style: {
	fill: red
	multiple: true
}

*.&style: {
		fill: red
		multiple: true
}
`)
					assert.ErrorString(t, err, `TestCompile/filters/errors/bad-syntax.d2:6:3: unexpected text after map key
TestCompile/filters/errors/bad-syntax.d2:9:1: unexpected map termination character } in file map`)
				},
			},
			{
				name: "composite",
				run: func(t testing.TB) {
					_, err := compile(t, `jacob.style: {
	fill: red
	multiple: true
}
*: {
	&style: {
		fill: red
		multiple: true
	}
}
`)
					assert.ErrorString(t, err, `TestCompile/filters/errors/composite.d2:6:2: ampersand filters cannot be composites`)
				},
			},
		}
		runa(t, tca)
	})
}
