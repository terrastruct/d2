package d2lib

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"strings"

	"oss.terrastruct.com/d2/d2ast"
	"oss.terrastruct.com/d2/d2compiler"
	"oss.terrastruct.com/d2/d2exporter"
	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2layouts"
	"oss.terrastruct.com/d2/d2layouts/d2dagrelayout"
	"oss.terrastruct.com/d2/d2parser"
	"oss.terrastruct.com/d2/d2renderers/d2fonts"
	"oss.terrastruct.com/d2/d2renderers/d2svg"
	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/d2themes/d2themescatalog"
	"oss.terrastruct.com/d2/lib/textmeasure"
	"oss.terrastruct.com/util-go/go2"
)

type CompileOptions struct {
	UTF16Pos       bool
	FS             fs.FS
	MeasuredTexts  []*d2target.MText
	Ruler          *textmeasure.Ruler
	RouterResolver func(engine string) (d2graph.RouteEdges, error)
	LayoutResolver func(engine string) (d2graph.LayoutGraph, error)

	Layout *string

	// FontFamily controls the font family used for all texts that are not the following:
	// - code
	// - latex
	// - pre-measured (web setting)
	// TODO maybe some will want to configure code font too, but that's much lower priority
	FontFamily *d2fonts.FontFamily

	// MonoFontFamily controls the font family used for code/mono texts
	MonoFontFamily *d2fonts.FontFamily

	// ASCII indicates that the output will be rendered as ASCII text, which affects font choices
	// for layout calculations to ensure alignment with character grid
	ASCII bool

	InputPath string
}

func Parse(ctx context.Context, input string, compileOpts *CompileOptions) (*d2ast.Map, error) {
	if compileOpts == nil {
		compileOpts = &CompileOptions{}
	}

	ast, err := d2parser.Parse(compileOpts.InputPath, strings.NewReader(input), &d2parser.ParseOptions{
		UTF16Pos: compileOpts.UTF16Pos,
	})
	return ast, err
}

func Compile(ctx context.Context, input string, compileOpts *CompileOptions, renderOpts *d2svg.RenderOpts) (*d2target.Diagram, *d2graph.Graph, error) {
	if compileOpts == nil {
		compileOpts = &CompileOptions{}
	}
	if renderOpts == nil {
		renderOpts = &d2svg.RenderOpts{}
	}

	g, config, err := d2compiler.Compile(compileOpts.InputPath, strings.NewReader(input), &d2compiler.CompileOptions{
		UTF16Pos: compileOpts.UTF16Pos,
		FS:       compileOpts.FS,
	})
	if err != nil {
		return nil, nil, err
	}

	applyConfigs(config, compileOpts, renderOpts)
	applyDefaults(compileOpts, renderOpts)
	if config != nil {
		g.Data = config.Data
	}

	d, err := compile(ctx, g, compileOpts, renderOpts)
	if d != nil {
		if config == nil {
			config = &d2target.Config{}
		}
		// These are fields that affect a diagram's appearance, so feed them back
		// into diagram.Config to ensure the hash computed for CSS styling purposes
		// is unique to its appearance
		config.ThemeID = renderOpts.ThemeID
		config.DarkThemeID = renderOpts.DarkThemeID
		config.Sketch = renderOpts.Sketch
		d.Config = config
	}
	return d, g, err
}

func compile(ctx context.Context, g *d2graph.Graph, compileOpts *CompileOptions, renderOpts *d2svg.RenderOpts) (*d2target.Diagram, error) {
	err := g.ApplyTheme(*renderOpts.ThemeID)
	if err != nil {
		return nil, err
	}

	if len(g.Objects) > 0 {
		g.ASCII = compileOpts.ASCII
		err := g.SetDimensions(compileOpts.MeasuredTexts, compileOpts.Ruler, compileOpts.FontFamily, compileOpts.MonoFontFamily)
		if err != nil {
			return nil, err
		}

		coreLayout, err := getLayout(compileOpts)
		if err != nil {
			return nil, err
		}
		edgeRouter, err := getEdgeRouter(compileOpts)
		if err != nil {
			return nil, err
		}

		graphInfo := d2layouts.NestedGraphInfo(g.Root)
		err = d2layouts.LayoutNested(ctx, g, graphInfo, coreLayout, edgeRouter)
		if err != nil {
			return nil, err
		}
	}

	d, err := d2exporter.Export(ctx, g, compileOpts.FontFamily, compileOpts.MonoFontFamily)
	if err != nil {
		return nil, err
	}

	for _, l := range g.Layers {
		ld, err := compile(ctx, l, compileOpts, renderOpts)
		if err != nil {
			return nil, err
		}
		d.Layers = append(d.Layers, ld)
	}
	for _, l := range g.Scenarios {
		ld, err := compile(ctx, l, compileOpts, renderOpts)
		if err != nil {
			return nil, err
		}
		d.Scenarios = append(d.Scenarios, ld)
	}
	for _, l := range g.Steps {
		ld, err := compile(ctx, l, compileOpts, renderOpts)
		if err != nil {
			return nil, err
		}
		d.Steps = append(d.Steps, ld)
	}
	return d, nil
}

func getLayout(opts *CompileOptions) (d2graph.LayoutGraph, error) {
	if opts.Layout != nil {
		return opts.LayoutResolver(*opts.Layout)
	} else if os.Getenv("D2_LAYOUT") == "dagre" {
		defaultLayout := func(ctx context.Context, g *d2graph.Graph) error {
			return d2dagrelayout.Layout(ctx, g, nil)
		}
		return defaultLayout, nil
	} else {
		return nil, errors.New("no available layout")
	}
}

func getEdgeRouter(opts *CompileOptions) (d2graph.RouteEdges, error) {
	if opts.Layout != nil && opts.RouterResolver != nil {
		router, err := opts.RouterResolver(*opts.Layout)
		if err != nil {
			return nil, err
		}
		if router != nil {
			return router, nil
		}
	}
	return d2layouts.DefaultRouter, nil
}

// applyConfigs applies the configs read from D2 and applies it to passed in opts
// It will only write to opt fields that are nil, as passed-in opts have precedence
func applyConfigs(config *d2target.Config, compileOpts *CompileOptions, renderOpts *d2svg.RenderOpts) {
	if config == nil {
		return
	}

	if compileOpts.Layout == nil {
		compileOpts.Layout = config.LayoutEngine
	}

	if renderOpts.ThemeID == nil {
		renderOpts.ThemeID = config.ThemeID
	}
	if renderOpts.DarkThemeID == nil {
		renderOpts.DarkThemeID = config.DarkThemeID
	}
	if renderOpts.Sketch == nil {
		renderOpts.Sketch = config.Sketch
	}
	if renderOpts.Pad == nil {
		renderOpts.Pad = config.Pad
	}
	if renderOpts.Center == nil {
		renderOpts.Center = config.Center
	}
	renderOpts.ThemeOverrides = config.ThemeOverrides
	renderOpts.DarkThemeOverrides = config.DarkThemeOverrides
}

func applyDefaults(compileOpts *CompileOptions, renderOpts *d2svg.RenderOpts) {
	if compileOpts.Layout == nil {
		compileOpts.Layout = go2.Pointer("dagre")
	}

	if renderOpts.ThemeID == nil {
		renderOpts.ThemeID = &d2themescatalog.NeutralDefault.ID
	}
	if renderOpts.Sketch == nil {
		renderOpts.Sketch = go2.Pointer(false)
	}
	if *renderOpts.Sketch && compileOpts.FontFamily == nil {
		compileOpts.FontFamily = go2.Pointer(d2fonts.HandDrawn)
	}
	if renderOpts.Pad == nil {
		renderOpts.Pad = go2.Pointer(int64(d2svg.DEFAULT_PADDING))
	}
	if renderOpts.Center == nil {
		renderOpts.Center = go2.Pointer(false)
	}

	if compileOpts.ASCII {
		if compileOpts.Layout == nil || *compileOpts.Layout == "dagre" {
			compileOpts.Layout = go2.Pointer("elk")
		}
	}
}
