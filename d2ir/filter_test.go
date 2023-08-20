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
				assertQuery(t, m, 5, 0, nil, "")
				assertQuery(t, m, 1, 0, nil, "jacob")
				assertQuery(t, m, 2, 0, nil, "jeremy")
				assertQuery(t, m, 0, 0, "I'm a rectangle", "jeremy.label")
			},
		},
		{
			name: "array",
			run: func(t testing.TB) {
				m, err := compile(t, `the-little-cannon: {
	class: [server; deployed]
}
dino: {
	class: [internal; deployed]
}
catapult: {
	class: [jacob; server]
}

*: {
	&class: server
	style.multiple: true
}
`)
				assert.Success(t, err)
				assertQuery(t, m, 10, 0, nil, "")
				assertQuery(t, m, 3, 0, nil, "the-little-cannon")
				assertQuery(t, m, 1, 0, nil, "dino")
				assertQuery(t, m, 3, 0, nil, "catapult")
			},
		},
		{
			name: "edge",
			run: func(t testing.TB) {
				m, err := compile(t, `x -> y: {
	source-arrowhead.shape: diamond
	target-arrowhead.shape: diamond
}
x -> y

(x -> *)[*]: {
	&source-arrowhead.shape: diamond
	&target-arrowhead.shape: diamond
	label: diamond shape arrowheads
}
`)
				assert.Success(t, err)
				assertQuery(t, m, 7, 2, nil, "")
				assertQuery(t, m, 5, 0, nil, "(x -> y)[0]")
				assertQuery(t, m, 0, 0, "diamond shape arrowheads", "(x -> y)[0].label")
				assertQuery(t, m, 0, 0, nil, "(x -> y)[1]")
			},
		},
		{
			name: "label-filter/1",
			run: func(t testing.TB) {
				m, err := compile(t, `
x
y
p: p
a -> z: delta

*.style.opacity: 0.1
*: {
  &label: x
  style.opacity: 1
}
*: {
  &label: p
  style.opacity: 0.5
}
(* -> *)[*]: {
	&label: delta
	target-arrowhead.shape: diamond
}
`)
				assert.Success(t, err)
				assertQuery(t, m, 17, 1, nil, "")
				assertQuery(t, m, 0, 0, 1, "x.style.opacity")
				assertQuery(t, m, 0, 0, 0.1, "y.style.opacity")
				assertQuery(t, m, 0, 0, 0.5, "p.style.opacity")
				assertQuery(t, m, 0, 0, 0.1, "a.style.opacity")
				assertQuery(t, m, 0, 0, 0.1, "z.style.opacity")
				assertQuery(t, m, 0, 0, "diamond", "(a -> z).target-arrowhead.shape")
			},
		},
		{
			name: "label-filter/2",
			run: func(t testing.TB) {
				m, err := compile(t, `
(* -> *)[*].style.opacity: 0.1

(* -> *)[*]: {
  &label: hi
  style.opacity: 1
}

x -> y: hi
x -> y
`)
				assert.Success(t, err)
				assertQuery(t, m, 6, 2, nil, "")
				assertQuery(t, m, 2, 0, "hi", "(x -> y)[0]")
				assertQuery(t, m, 0, 0, 1, "(x -> y)[0].style.opacity")
				assertQuery(t, m, 0, 0, 0.1, "(x -> y)[1].style.opacity")
			},
		},
		{
			name: "lazy-filter",
			run: func(t testing.TB) {
				m, err := compile(t, `
*: {
  &label: a
  style.fill: yellow
}

a
# if i remove this line, the glob applies as expected
b
b.label: a
`)
				assert.Success(t, err)
				assertQuery(t, m, 7, 0, nil, "")
				assertQuery(t, m, 0, 0, "yellow", "a.style.fill")
				assertQuery(t, m, 0, 0, "yellow", "b.style.fill")
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
				name: "no-glob",
				run: func(t testing.TB) {
					_, err := compile(t, `jacob.style: {
	fill: red
	multiple: true
}

jasmine.style: {
		&fill: red
		multiple: false
}
`)
					assert.ErrorString(t, err, `TestCompile/filters/errors/no-glob.d2:7:3: glob filters cannot be used outside globs`)
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
					assert.ErrorString(t, err, `TestCompile/filters/errors/composite.d2:6:2: glob filters cannot be composites`)
				},
			},
		}
		runa(t, tca)
	})
}
