//go:build !noelk

package d2plugin

import (
	"context"
	"encoding/json"
	"fmt"

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
	return []PluginSpecificFlag{
		{
			Name:    "elk-algorithm",
			Type:    "string",
			Default: d2elklayout.DefaultOpts.Algorithm,
			Usage:   "layout algorithm",
			Tag:     "elk.algorithm",
		},
		{
			Name:    "elk-nodeNodeBetweenLayers",
			Type:    "int64",
			Default: int64(d2elklayout.DefaultOpts.NodeSpacing),
			Usage:   "the spacing to be preserved between any pair of nodes of two adjacent layers",
			Tag:     "spacing.nodeNodeBetweenLayers",
		},
		{
			Name:    "elk-padding",
			Type:    "string",
			Default: d2elklayout.DefaultOpts.Padding,
			Usage:   "the padding to be left to a parent element’s border when placing child elements",
			Tag:     "elk.padding",
		},
		{
			Name:    "elk-edgeNodeBetweenLayers",
			Type:    "int64",
			Default: int64(d2elklayout.DefaultOpts.EdgeNodeSpacing),
			Usage:   "the spacing to be preserved between nodes and edges that are routed next to the node’s layer",
			Tag:     "spacing.edgeNodeBetweenLayers",
		},
		{
			Name:    "elk-nodeSelfLoop",
			Type:    "int64",
			Default: int64(d2elklayout.DefaultOpts.SelfLoopSpacing),
			Usage:   "spacing to be preserved between a node and its self loops",
			Tag:     "elk.spacing.nodeSelfLoop",
		},
	}, nil
}

func (p *elkPlugin) HydrateOpts(opts []byte) error {
	if opts != nil {
		var elkOpts d2elklayout.ConfigurableOpts
		err := json.Unmarshal(opts, &elkOpts)
		if err != nil {
			return xmain.UsageErrorf("non-ELK layout options given for ELK")
		}

		p.opts = &elkOpts
	}
	return nil
}

func (p elkPlugin) Info(ctx context.Context) (*PluginInfo, error) {
	opts := xmain.NewOpts(nil, nil, nil)
	flags, err := p.Flags(ctx)
	if err != nil {
		return nil, err
	}
	for _, f := range flags {
		f.AddToOpts(opts)
	}
	return &PluginInfo{
		Name: "elk",
		Type: "bundled",
		Features: []PluginFeature{
			CONTAINER_DIMENSIONS,
		},
		ShortHelp: "Eclipse Layout Kernel (ELK) with the Layered algorithm.",
		LongHelp: fmt.Sprintf(`ELK is a layout engine offered by Eclipse.
Originally written in Java, it has been ported to Javascript and cross-compiled into D2.
See https://d2lang.com/tour/elk for more.

Flags correspond to ones found at https://www.eclipse.org/elk/reference.html.

Flags:
%s
`, opts.Defaults()),
	}, nil
}

func (p elkPlugin) Layout(ctx context.Context, g *d2graph.Graph) error {
	return d2elklayout.Layout(ctx, g, p.opts)
}

func (p elkPlugin) PostProcess(ctx context.Context, in []byte) ([]byte, error) {
	return in, nil
}
