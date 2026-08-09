package d2lib

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"strings"

	"github.com/d2lang/d2/d2ast"
	"github.com/d2lang/d2/d2compiler"
	"github.com/d2lang/d2/d2exporter"
	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/d2layouts"
	"github.com/d2lang/d2/d2layouts/d2dagrelayout"
	"github.com/d2lang/d2/d2parser"
	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/d2renderers/d2svg"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/d2themes/d2themescatalog"
	"github.com/d2lang/d2/lib/textmeasure"
	"github.com/d2lang/util-go/go2"
)

type CompileOptions struct {
	UTF16Pos       bool
	FS             fs.FS
	MeasuredTexts  []*d2target.MText
	Ruler          *textmeasure.Ruler
	RouterResolver func(engine string) (d2graph.RouteEdges, error)
	LayoutResolver func(engine string) (d2graph.LayoutGraph, error)

	// LayoutReuse allows scenarios and steps whose layout inputs are unchanged
	// to reuse the geometry of their base board. Callers should enable this only
	// when LayoutResolver and RouterResolver return a stable built-in Dagre or
	// ELK configuration for the duration of Compile. It is disabled by default
	// so custom resolvers retain their existing per-board behavior.
	LayoutReuse bool

	Layout *string

	// FontFamily controls the font family used for all texts that are not the following:
	// - code
	// - latex
	// - pre-measured (web setting)
	// TODO maybe some will want to configure code font too, but that's much lower priority
	FontFamily *d2fonts.FontFamily

	// MonoFontFamily controls the font family used for code/mono texts
	MonoFontFamily *d2fonts.FontFamily

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
	return compileInput(ctx, input, compileOpts, renderOpts)
}

func compileInput(ctx context.Context, input string, compileOpts *CompileOptions, renderOpts *d2svg.RenderOpts) (*d2target.Diagram, *d2graph.Graph, error) {
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
	d, _, err := compileBoard(ctx, g, compileOpts, renderOpts, nil, false)
	return d, err
}

func compileBoard(ctx context.Context, g *d2graph.Graph, compileOpts *CompileOptions, renderOpts *d2svg.RenderOpts, reuse *layoutGeometry, captureForCaller bool) (*d2target.Diagram, *layoutGeometry, error) {
	err := g.ApplyTheme(*renderOpts.ThemeID)
	if err != nil {
		return nil, nil, err
	}

	var geometry *layoutGeometry
	if len(g.Objects) > 0 {
		err := g.SetDimensions(compileOpts.MeasuredTexts, compileOpts.Ruler, compileOpts.FontFamily, compileOpts.MonoFontFamily)
		if err != nil {
			return nil, nil, err
		}

		needsGeometry := compileOpts.LayoutReuse && (reuse != nil || captureForCaller || len(g.Scenarios) > 0 || len(g.Steps) > 0)
		var signature layoutInputSignature
		reusable := false
		if needsGeometry {
			engine := ""
			if compileOpts.Layout != nil {
				engine = *compileOpts.Layout
			}
			signature, reusable, err = newLayoutInputSignature(g, engine)
			if err != nil {
				return nil, nil, err
			}
		}
		// A built-in layout may surface context cancellation at the start of
		// every board. Do not let geometry replay bypass that error path.
		reused := ctx.Err() == nil && reusable && compileOpts.LayoutReuse && reuse.apply(g, signature)
		if !reused {
			coreLayout, err := getLayout(compileOpts)
			if err != nil {
				return nil, nil, err
			}
			edgeRouter, err := getEdgeRouter(compileOpts)
			if err != nil {
				return nil, nil, err
			}
			graphInfo := d2layouts.NestedGraphInfo(g.Root)
			err = d2layouts.LayoutNested(ctx, g, graphInfo, coreLayout, edgeRouter)
			if err != nil {
				return nil, nil, err
			}
		}
		if reusable && (captureForCaller || len(g.Scenarios) > 0 || len(g.Steps) > 0) {
			geometry = captureLayoutGeometry(g, signature)
		}
	}

	d, err := d2exporter.Export(ctx, g, compileOpts.FontFamily, compileOpts.MonoFontFamily)
	if err != nil {
		return nil, nil, err
	}

	for _, l := range g.Layers {
		// Layers are independent boards rather than overlays of their parent, so
		// retain their existing always-layout behavior.
		ld, _, err := compileBoard(ctx, l, compileOpts, renderOpts, nil, false)
		if err != nil {
			return nil, nil, err
		}
		d.Layers = append(d.Layers, ld)
	}
	for _, l := range g.Scenarios {
		ld, _, err := compileBoard(ctx, l, compileOpts, renderOpts, geometry, false)
		if err != nil {
			return nil, nil, err
		}
		d.Scenarios = append(d.Scenarios, ld)
	}
	previous := geometry
	for i, l := range g.Steps {
		ld, stepGeometry, err := compileBoard(ctx, l, compileOpts, renderOpts, previous, i+1 < len(g.Steps))
		if err != nil {
			return nil, nil, err
		}
		d.Steps = append(d.Steps, ld)
		previous = stepGeometry
	}
	return d, geometry, nil
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
}
