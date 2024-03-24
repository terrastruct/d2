package elk

import (
	"context"
	"encoding/json"
	"fmt"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2layouts/d2elklayout"
	"oss.terrastruct.com/d2/d2plugin"
	"oss.terrastruct.com/util-go/xmain"
)

type ELKPlugin struct {
	opts *d2elklayout.ConfigurableOpts
}

func (p ELKPlugin) Flags(context.Context) ([]d2plugin.PluginSpecificFlag, error) {
	return []d2plugin.PluginSpecificFlag{
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

func (p *ELKPlugin) HydrateOpts(opts []byte) error {
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

func (p ELKPlugin) Info(ctx context.Context) (*d2plugin.PluginInfo, error) {
	opts := xmain.NewOpts(nil, nil)
	flags, err := p.Flags(ctx)
	if err != nil {
		return nil, err
	}
	for _, f := range flags {
		f.AddToOpts(opts)
	}
	return &d2plugin.PluginInfo{
		Name: "elk",
		Type: "bundled",
		Features: []d2plugin.PluginFeature{
			d2plugin.CONTAINER_DIMENSIONS,
			d2plugin.DESCENDANT_EDGES,
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

func (p ELKPlugin) Layout(ctx context.Context, g *d2graph.Graph) error {
	return d2elklayout.Layout(ctx, g, p.opts)
}

func (p ELKPlugin) PostProcess(ctx context.Context, in []byte) ([]byte, error) {
	return in, nil
}
