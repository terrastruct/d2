package d2lib_test

import (
	"context"
	"testing"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2layouts/d2dagrelayout"
	"oss.terrastruct.com/d2/d2lib"
	"oss.terrastruct.com/d2/lib/textmeasure"
	"oss.terrastruct.com/util-go/assert"
)

func TestValidateTopLeft(t *testing.T) {
	assertCompile(t, `
container: {
	a: {
		top: 100
		left: 100
	}
	b: {
		top: 100
		left: 100
	}
}
`,
		`4:3: invalid top/left overlapping with "b"
8:3: invalid top/left overlapping with "a"`,
	)

	assertCompile(t, `
container: {
	a: {
		top: 100
		left: 100
	}
	b: {
		top: 100
		left: 200
	}
}
`,
		``,
	)

	assertCompile(t, `
a: {
	top: 100
	left: 100
}
b: {
	top: 101
	left: 101
}
`,
		`3:2: invalid top/left overlapping with "b"
7:2: invalid top/left overlapping with "a"`,
	)

	assertCompile(t, `
a: {
	top: 100
	left: 100
}
b: {
	top: 99
	left: 99
}
`,
		`3:2: invalid top/left overlapping with "b"
7:2: invalid top/left overlapping with "a"`,
	)
}

func assertCompile(t *testing.T, text string, expErr string) {
	ruler, _ := textmeasure.NewRuler()
	defaultLayout := func(ctx context.Context, g *d2graph.Graph) error {
		return d2dagrelayout.Layout(ctx, g, nil)
	}
	_, _, err := d2lib.Compile(context.Background(), text, &d2lib.CompileOptions{
		Layout: defaultLayout,
		Ruler:  ruler,
	})
	if expErr != "" {
		assert.ErrorString(t, err, expErr)
	} else {
		assert.Success(t, err)
	}
}
