//go:build !noelk

package d2plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/d2layouts/d2elklayout"
	"github.com/d2lang/util-go/xmain"
)

var ELKPlugin = elkPlugin{}

func init() {
	plugins = append(plugins, &ELKPlugin)
}

type elkPlugin struct {
	mu   sync.Mutex
	opts *d2elklayout.ConfigurableOpts
}

func (p *elkPlugin) Flags(context.Context) ([]PluginSpecificFlag, error) {
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
			Name:    "elk-edgeEdgeBetweenLayers",
			Type:    "int64",
			Default: int64(d2elklayout.DefaultOpts.EdgeEdgeSpacing),
			Usage:   "the spacing to be preserved between pairs of edges routed between the same pair of layers",
			Tag:     "spacing.edgeEdgeBetweenLayers",
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

		p.mu.Lock()
		p.opts = &elkOpts
		p.mu.Unlock()
	}
	return nil
}

func (p *elkPlugin) Info(ctx context.Context) (*PluginInfo, error) {
	opts := xmain.NewOpts(nil, nil)
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
			DESCENDANT_EDGES,
		},
		ShortHelp: "Eclipse Layout Kernel (ELK) with the Layered algorithm.",
		LongHelp: fmt.Sprintf(`ELK is a layout engine offered by Eclipse.
Originally written in Java, D2's ELK.js 0.12.0 layout profile is bundled through the native Go elk-go port. Layered remains the default.
See https://d2lang.com/tour/elk for more.

Flags correspond to ones found at https://www.eclipse.org/elk/reference.html.

Flags:
%s
`, opts.Defaults()),
	}, nil
}

func (p *elkPlugin) Layout(ctx context.Context, g *d2graph.Graph) error {
	p.mu.Lock()
	opts := p.opts
	p.mu.Unlock()
	return d2elklayout.Layout(ctx, g, opts)
}
