//go:build cgo && !noelk

package d2plugin

import (
	"context"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2layouts/d2elklayout"
)

var ELKPlugin = elkPlugin{}

func init() {
	plugins = append(plugins, ELKPlugin)
}

type elkPlugin struct{}

func (p elkPlugin) Info(context.Context) (*PluginInfo, error) {
	return &PluginInfo{
		Name:      "ELK",
		ShortHelp: "Eclipse Layout Kernel (ELK) with the Layered algorithm.",
		LongHelp: `ELK is a layout engine offered by Eclipse.
Originally written in Java, it has been ported to Javascript and cross-compiled into D2.
See https://github.com/kieler/elkjs for more.`,
	}, nil
}

func (p elkPlugin) Layout(ctx context.Context, g *d2graph.Graph) error {
	return d2elklayout.Layout(ctx, g)
}

func (p elkPlugin) PostProcess(ctx context.Context, in []byte) ([]byte, error) {
	return in, nil
}
