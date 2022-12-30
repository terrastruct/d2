//go:build !noelk

package d2plugin

import (
	"context"
	"encoding/json"

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

func (p elkPlugin) Flags(context.Context) ([]PluginSpecificFlag, error) {
	// ms.Opts.String("", "elk-algorithm", "", d2elklayout.DefaultOpts.Algorithm, "number of pixels that separate nodes horizontally.")
	//   _, err = ms.Opts.Int64("", "elk-nodeNodeBetweenLayers", "", int64(d2elklayout.DefaultOpts.NodeSpacing), "number of pixels that separate edges horizontally.")
	//   if err != nil {
	//     return err
	//   }
	//   ms.Opts.String("", "elk-padding", "", d2elklayout.DefaultOpts.Padding, "number of pixels that separate nodes horizontally.")
	//   _, err = ms.Opts.Int64("", "elk-edgeNodeBetweenLayers", "", int64(d2elklayout.DefaultOpts.EdgeNodeSpacing), "number of pixels that separate edges horizontally.")
	//   if err != nil {
	//     return err
	//   }
	//   _, err = ms.Opts.Int64("", "elk-nodeSelfLoop", "", int64(d2elklayout.DefaultOpts.SelfLoopSpacing), "number of pixels that separate edges horizontally.")
	//   if err != nil {
	//     return err
	//   }
	return []PluginSpecificFlag{
		{
			Name:    "elk-algorithm",
			Type:    "string",
			Default: d2elklayout.DefaultOpts.Algorithm,
			Usage:   "number of pixels that separate nodes horizontally.",
			Tag:     "elk.algorithm",
		},
	}, nil
}

func (p *elkPlugin) HydrateOpts(opts []byte) error {
	if opts != nil {
		var elkOpts d2elklayout.ConfigurableOpts
		err := json.Unmarshal(opts, &elkOpts)
		if err != nil {
			// TODO not right
			return xmain.UsageErrorf("non-dagre layout options given for dagre")
		}

		p.opts = &elkOpts
	}
	return nil
}

func (p elkPlugin) Info(context.Context) (*PluginInfo, error) {
	return &PluginInfo{
		Name:      "elk",
		ShortHelp: "Eclipse Layout Kernel (ELK) with the Layered algorithm.",
		LongHelp: `ELK is a layout engine offered by Eclipse.
Originally written in Java, it has been ported to Javascript and cross-compiled into D2.
See https://github.com/kieler/elkjs for more.`,
	}, nil
}

func (p elkPlugin) Layout(ctx context.Context, g *d2graph.Graph) error {
	return d2elklayout.Layout(ctx, g, p.opts)
}

func (p elkPlugin) PostProcess(ctx context.Context, in []byte) ([]byte, error) {
	return in, nil
}
