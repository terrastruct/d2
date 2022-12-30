//go:build !noelk

package d2plugin

import (
	"context"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2layouts/d2elklayout"
	"oss.terrastruct.com/util-go/xmain"
)

var ELKPlugin = elkPlugin{}

func init() {
	plugins = append(plugins, &ELKPlugin)
}

type elkPlugin struct {
	opts *d2elklayout.ConfigurableOpts
}

func (p *elkPlugin) HydrateOpts(ctx context.Context, opts interface{}) error {
	if opts != nil {
		elkOpts, ok := opts.(d2elklayout.ConfigurableOpts)
		if !ok {
			return xmain.UsageErrorf("non-dagre layout options given for dagre")
		}

		p.opts = &elkOpts
	}
	return nil
}

func (p *elkPlugin) Info(context.Context) (*PluginInfo, error) {
	return &PluginInfo{
		Name:      "elk",
		ShortHelp: "Eclipse Layout Kernel (ELK) with the Layered algorithm.",
		LongHelp: `ELK is a layout engine offered by Eclipse.
Originally written in Java, it has been ported to Javascript and cross-compiled into D2.
See https://github.com/kieler/elkjs for more.`,
	}, nil
}

func (p *elkPlugin) Layout(ctx context.Context, g *d2graph.Graph) error {
	return d2elklayout.Layout(ctx, g, p.opts)
}

func (p *elkPlugin) PostProcess(ctx context.Context, in []byte) ([]byte, error) {
	return in, nil
}
