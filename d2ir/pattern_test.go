package d2ir_test

import (
	"testing"

	"oss.terrastruct.com/util-go/assert"
)

func testCompilePatterns(t *testing.T) {
	t.Parallel()

	tca := []testCase{
		{
			name: "escaped",
			run: func(t testing.TB) {
				m, err := compile(t, `animal: meow
action: yes
a\*: globbed`)
				assert.Success(t, err)
				assertQuery(t, m, 3, 0, nil, "")
				assertQuery(t, m, 0, 0, "meow", "animal")
				assertQuery(t, m, 0, 0, "yes", "action")
				assertQuery(t, m, 0, 0, "globbed", `a\*`)
			},
		},
		{
			name: "prefix",
			run: func(t testing.TB) {
				m, err := compile(t, `animal: meow
action: yes
a*: globbed`)
				assert.Success(t, err)
				assertQuery(t, m, 2, 0, nil, "")
				assertQuery(t, m, 0, 0, "globbed", "animal")
				assertQuery(t, m, 0, 0, "globbed", "action")
			},
		},
		{
			name: "case/1",
			run: func(t testing.TB) {
				m, err := compile(t, `animal: meow
action: yes
A*: globbed`)
				assert.Success(t, err)
				assertQuery(t, m, 2, 0, nil, "")
				assertQuery(t, m, 0, 0, "globbed", "animal")
				assertQuery(t, m, 0, 0, "globbed", "action")
			},
		},
		{
			name: "case/2",
			run: func(t testing.TB) {
				m, err := compile(t, `diddy kong
Donkey Kong
*kong: yes`)
				assert.Success(t, err)
				assertQuery(t, m, 2, 0, nil, "")
				assertQuery(t, m, 0, 0, "yes", "diddy kong")
				assertQuery(t, m, 0, 0, "yes", "Donkey Kong")
			},
		},
		{
			name: "suffix",
			run: func(t testing.TB) {
				m, err := compile(t, `animal: meow
jingle: loud
*l: globbed`)
				assert.Success(t, err)
				assertQuery(t, m, 2, 0, nil, "")
				assertQuery(t, m, 0, 0, "globbed", "animal")
				assertQuery(t, m, 0, 0, "globbed", "jingle")
			},
		},
		{
			name: "prefix-suffix",
			run: func(t testing.TB) {
				m, err := compile(t, `tinker: meow
thinker: yes
t*r: globbed`)
				assert.Success(t, err)
				assertQuery(t, m, 2, 0, nil, "")
				assertQuery(t, m, 0, 0, "globbed", "tinker")
				assertQuery(t, m, 0, 0, "globbed", "thinker")
			},
		},
		{
			name: "prefix-suffix/2",
			run: func(t testing.TB) {
				m, err := compile(t, `tinker: meow
thinker: yes
t*ink*r: globbed`)
				assert.Success(t, err)
				assertQuery(t, m, 2, 0, nil, "")
				assertQuery(t, m, 0, 0, "globbed", "tinker")
				assertQuery(t, m, 0, 0, "globbed", "thinker")
			},
		},
		{
			name: "prefix-suffix/3",
			run: func(t testing.TB) {
				m, err := compile(t, `tinkertinker: meow
thinkerthinker: yes
t*ink*r*t*inke*: globbed`)
				assert.Success(t, err)
				assertQuery(t, m, 2, 0, nil, "")
				assertQuery(t, m, 0, 0, "globbed", "tinkertinker")
				assertQuery(t, m, 0, 0, "globbed", "thinkerthinker")
			},
		},
		{
			name: "nested/prefix-suffix/3",
			run: func(t testing.TB) {
				m, err := compile(t, `animate.constant.tinkertinker: meow
astronaut.constant.thinkerthinker: yes
a*n*t*.constant.t*ink*r*t*inke*: globbed`)
				assert.Success(t, err)
				assertQuery(t, m, 6, 0, nil, "")
				assertQuery(t, m, 0, 0, "globbed", "animate.constant.tinkertinker")
				assertQuery(t, m, 0, 0, "globbed", "astronaut.constant.thinkerthinker")
			},
		},
		{
			name: "edge/1",
			run: func(t testing.TB) {
				m, err := compile(t, `animate
animal
an* -> an*`)
				assert.Success(t, err)
				assertQuery(t, m, 2, 4, nil, "")
				assertQuery(t, m, 0, 0, nil, "(animate -> animal)[0]")
				assertQuery(t, m, 0, 0, nil, "(animal -> animal)[0]")
			},
		},
		{
			name: "edge/2",
			run: func(t testing.TB) {
				m, err := compile(t, `shared.animate
shared.animal
sh*.(an* -> an*)`)
				assert.Success(t, err)
				assertQuery(t, m, 3, 4, nil, "")
				assertQuery(t, m, 2, 4, nil, "shared")
				assertQuery(t, m, 0, 0, nil, "shared.(animate -> animal)[0]")
				assertQuery(t, m, 0, 0, nil, "shared.(animal -> animal)[0]")
			},
		},
		{
			name: "edge/3",
			run: func(t testing.TB) {
				m, err := compile(t, `shared.animate
shared.animal
sh*.an* -> sh*.an*`)
				assert.Success(t, err)
				assertQuery(t, m, 3, 4, nil, "")
				assertQuery(t, m, 2, 4, nil, "shared")
				assertQuery(t, m, 0, 0, nil, "shared.(animate -> animal)[0]")
				assertQuery(t, m, 0, 0, nil, "shared.(animal -> animal)[0]")
			},
		},
		{
			name: "edge-glob-index",
			run: func(t testing.TB) {
				m, err := compile(t, `a -> b
a -> b
a -> b
(a -> b)[*].style.fill: red
`)
				assert.Success(t, err)
				assertQuery(t, m, 8, 3, nil, "")
				assertQuery(t, m, 0, 0, "red", "(a -> b)[0].style.fill")
				assertQuery(t, m, 0, 0, "red", "(a -> b)[1].style.fill")
				assertQuery(t, m, 0, 0, "red", "(a -> b)[2].style.fill")
			},
		},
		{
			name: "glob-edge-glob-index",
			run: func(t testing.TB) {
				m, err := compile(t, `a -> b
a -> b
a -> b
c -> b
(* -> b)[*].style.fill: red
`)
				assert.Success(t, err)
				assertQuery(t, m, 11, 4, nil, "")
				assertQuery(t, m, 0, 0, "red", "(a -> b)[0].style.fill")
				assertQuery(t, m, 0, 0, "red", "(a -> b)[1].style.fill")
				assertQuery(t, m, 0, 0, "red", "(a -> b)[2].style.fill")
				assertQuery(t, m, 0, 0, "red", "(c -> b)[0].style.fill")
			},
		},
		{
			name: "double-glob/1",
			run: func(t testing.TB) {
				m, err := compile(t, `shared.animate
shared.animal
**.style.fill: red`)
				assert.Success(t, err)
				assertQuery(t, m, 9, 0, nil, "")
				assertQuery(t, m, 8, 0, nil, "shared")
				assertQuery(t, m, 1, 0, nil, "shared.style")
				assertQuery(t, m, 2, 0, nil, "shared.animate")
				assertQuery(t, m, 1, 0, nil, "shared.animate.style")
				assertQuery(t, m, 2, 0, nil, "shared.animal")
				assertQuery(t, m, 1, 0, nil, "shared.animal.style")
			},
		},
		{
			name: "reserved",
			run: func(t testing.TB) {
				m, err := compile(t, `vars: {
  d2-config: {
    layout-engine: elk
  }
}

Spiderman 1
Spiderman 2
Spiderman 3

* -> *: arrow`)
				assert.Success(t, err)
				assertQuery(t, m, 6, 9, nil, "")
				assertQuery(t, m, 0, 0, "arrow", "(* -> *)[*]")
			},
		},
	}

	runa(t, tca)

	t.Run("errors", func(t *testing.T) {
		tca := []testCase{
			{
				name: "glob-edge-glob-index",
				run: func(t testing.TB) {
					m, err := compile(t, `(* -> b)[*].style.fill: red
`)
					assert.ErrorString(t, err, `TestCompile/patterns/errors/glob-edge-glob-index.d2:1:2: indexed edge does not exist`)
					assertQuery(t, m, 0, 0, nil, "")
				},
			},
		}
		runa(t, tca)
	})
}
