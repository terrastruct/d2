package d2lib

import (
	"context"
	"errors"
	"os"
	"strings"

	"oss.terrastruct.com/d2/d2compiler"
	"oss.terrastruct.com/d2/d2exporter"
	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2renderers/textmeasure"
	"oss.terrastruct.com/d2/d2target"
)

type CompileOptions struct {
	UTF16         bool
	MeasuredTexts []*d2target.MText
	Ruler         *textmeasure.Ruler
	Layout        func(context.Context, *d2graph.Graph) error

	ThemeID int64
}

func Compile(ctx context.Context, input string, opts *CompileOptions) (*d2target.Diagram, error) {
	if opts == nil {
		opts = &CompileOptions{}
	}

	g, err := d2compiler.Compile("", strings.NewReader(input), &d2compiler.CompileOptions{
		UTF16: opts.UTF16,
	})
	if err != nil {
		return nil, err
	}

	err = g.SetDimensions(opts.MeasuredTexts, opts.Ruler)
	if err != nil {
		return nil, err
	}

	if opts.Layout != nil {
		err = opts.Layout(ctx, g)
	} else if os.Getenv("D2_LAYOUT") == "dagre" && dagreLayout != nil {
		err = dagreLayout(ctx, g)
	} else {
		err = errors.New("no available layout")
	}
	if err != nil {
		return nil, err
	}

	diagram, err := d2exporter.Export(ctx, g, opts.ThemeID)
	return diagram, err
}

// See c.go
var dagreLayout func(context.Context, *d2graph.Graph) error
