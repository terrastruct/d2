package d2lib

import (
	"context"
	"errors"
	"os"
	"strings"

	"oss.terrastruct.com/d2/d2compiler"
	"oss.terrastruct.com/d2/d2exporter"
	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/textmeasure"
)

type CompileOptions struct {
	UTF16         bool
	MeasuredTexts []*d2target.MText
	Ruler         *textmeasure.Ruler
	Layout        func(context.Context, *d2graph.Graph) error

	ThemeID int64
}

func Compile(ctx context.Context, input string, opts *CompileOptions) (*d2target.Diagram, *d2graph.Graph, error) {
	if opts == nil {
		opts = &CompileOptions{}
	}

	g, err := d2compiler.Compile("", strings.NewReader(input), &d2compiler.CompileOptions{
		UTF16: opts.UTF16,
	})
	if err != nil {
		return nil, nil, err
	}

	err = g.SetDimensions(opts.MeasuredTexts, opts.Ruler)
	if err != nil {
		return nil, nil, err
	}

	if layout, err := getLayout(opts); err != nil {
		return nil, err
	} else if err := d2sequence.Layout(ctx, g, layout); err != nil {
		return nil, err
	}

	diagram, err := d2exporter.Export(ctx, g, opts.ThemeID)
	return diagram, g, err
}

func getLayout(opts *CompileOptions) (func(context.Context, *d2graph.Graph) error, error) {
	if opts.Layout != nil {
		return opts.Layout, nil
	} else if os.Getenv("D2_LAYOUT") == "dagre" && dagreLayout != nil {
		return dagreLayout, nil
	} else {
		return nil, errors.New("no available layout")
	}
}

// See c.go
var dagreLayout func(context.Context, *d2graph.Graph) error
