//go:build cgo

package d2plugin

import (
	"context"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2renderers/d2sketch"
)

var SketchPlugin = sketchPlugin{}

func init() {
	plugins = append(plugins, SketchPlugin)
}

type sketchPlugin struct{}

func (p sketchPlugin) Info(context.Context) (*PluginInfo, error) {
	return &PluginInfo{
		Name:      "sketch",
		ShortHelp: "Transform the render to look sketched by hand. Warning: experimental.",
		LongHelp: `sketch is a plugin for D2 that post-processes SVG renders to make them look sketched by hand.
Internally, it uses rough.js (https://github.com/rough-stuff/rough), a Javascript graphics library.

Currently this plugin is experimental. It handles basic rectangles and connections only.
`,
	}, nil
}

func (p sketchPlugin) Layout(ctx context.Context, g *d2graph.Graph) error {
	return nil
}

func (p sketchPlugin) PostProcess(ctx context.Context, in []byte) ([]byte, error) {
	return d2sketch.Sketch(in)
}

func (p sketchPlugin) Options(ctx context.Context) ([]string, error) {
	return []string{"postProcess"}, nil
}
