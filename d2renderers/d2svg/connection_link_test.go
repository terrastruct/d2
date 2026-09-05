package d2svg_test

import (
	"context"
	"encoding/xml"
	"fmt"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/d2layouts/d2dagrelayout"
	"github.com/d2lang/d2/d2lib"
	"github.com/d2lang/d2/d2renderers/d2svg"
	"github.com/d2lang/d2/lib/log"
	"github.com/d2lang/d2/lib/textmeasure"
	"github.com/d2lang/util-go/assert"
)

func TestRichConnectionLabelsPreserveLinks(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		language string
		label    string
	}{
		{name: "latex", language: "latex", label: `x^2`},
		{name: "markdown", language: "md", label: `**linked**`},
		{name: "code", language: "go", label: `fmt.Println("linked")`},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			link := `https://example.com/?a=1&b=2`
			script := fmt.Sprintf("a -> b: |%s\n  %s\n| {\n  link: %s\n}", tc.language, tc.label, link)
			ruler, err := textmeasure.NewRuler()
			assert.Success(t, err)

			layoutResolver := func(string) (d2graph.LayoutGraph, error) {
				return d2dagrelayout.DefaultLayout, nil
			}
			diagram, _, err := d2lib.Compile(log.WithTB(context.Background(), t), script, &d2lib.CompileOptions{
				Ruler:          ruler,
				LayoutResolver: layoutResolver,
			}, nil)
			assert.Success(t, err)

			out, err := d2svg.Render(diagram, nil)
			assert.Success(t, err)
			var parsed any
			assert.Success(t, xml.Unmarshal(out, &parsed))
			svg := string(out)
			want := `<a href="https://example.com/?a=1&amp;b=2" xlink:href="https://example.com/?a=1&amp;b=2">`
			if !strings.Contains(svg, want) {
				t.Fatalf("linked %s label missing anchor %q", tc.name, want)
			}
		})
	}
}
