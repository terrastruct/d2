package d2lib

import (
	"context"
	"errors"
	"os"
	"strings"

	"oss.terrastruct.com/d2/d2compiler"
	"oss.terrastruct.com/d2/d2exporter"
	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2layouts/d2dagrelayout"
	"oss.terrastruct.com/d2/d2layouts/d2grid"
	"oss.terrastruct.com/d2/d2layouts/d2near"
	"oss.terrastruct.com/d2/d2layouts/d2sequence"
	"oss.terrastruct.com/d2/d2renderers/d2fonts"
	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/textmeasure"
)

type CompileOptions struct {
	UTF16         bool
	MeasuredTexts []*d2target.MText
	Ruler         *textmeasure.Ruler
	Layout        func(context.Context, *d2graph.Graph) error
	ThemeID       int64

	// FontFamily controls the font family used for all texts that are not the following:
	// - code
	// - latex
	// - pre-measured (web setting)
	// TODO maybe some will want to configure code font too, but that's much lower priority
	FontFamily *d2fonts.FontFamily
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

	d, err := compile(ctx, g, opts)
	if err != nil {
		return nil, nil, err
	}
	return d, g, nil
}

func compile(ctx context.Context, g *d2graph.Graph, opts *CompileOptions) (*d2target.Diagram, error) {
	err := g.ApplyTheme(opts.ThemeID)
	if err != nil {
		return nil, err
	}

	if len(g.Objects) > 0 {
		err := g.SetDimensions(opts.MeasuredTexts, opts.Ruler, opts.FontFamily)
		if err != nil {
			return nil, err
		}

		coreLayout, err := getLayout(opts)
		if err != nil {
			return nil, err
		}

		constantNearGraphs := d2near.WithoutConstantNears(ctx, g)

		layoutWithGrids := d2grid.Layout(ctx, g, coreLayout)

		// run core layout for constantNears
		for _, tempGraph := range constantNearGraphs {
			if err = layoutWithGrids(ctx, tempGraph); err != nil {
				return nil, err
			}
		}

		err = d2sequence.Layout(ctx, g, layoutWithGrids)
		if err != nil {
			return nil, err
		}

		err = d2near.Layout(ctx, g, constantNearGraphs)
		if err != nil {
			return nil, err
		}
	}

	d, err := d2exporter.Export(ctx, g, opts.FontFamily)
	if err != nil {
		return nil, err
	}

	for _, l := range g.Layers {
		ld, err := compile(ctx, l, opts)
		if err != nil {
			return nil, err
		}
		d.Layers = append(d.Layers, ld)
	}
	for _, l := range g.Scenarios {
		ld, err := compile(ctx, l, opts)
		if err != nil {
			return nil, err
		}
		d.Scenarios = append(d.Scenarios, ld)
	}
	for _, l := range g.Steps {
		ld, err := compile(ctx, l, opts)
		if err != nil {
			return nil, err
		}
		d.Steps = append(d.Steps, ld)
	}
	return d, nil
}

func getLayout(opts *CompileOptions) (d2graph.LayoutGraph, error) {
	if opts.Layout != nil {
		return opts.Layout, nil
	} else if os.Getenv("D2_LAYOUT") == "dagre" {
		defaultLayout := func(ctx context.Context, g *d2graph.Graph) error {
			return d2dagrelayout.Layout(ctx, g, nil)
		}
		return defaultLayout, nil
	} else {
		return nil, errors.New("no available layout")
	}
}
