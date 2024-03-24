package dagre

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2layouts/d2dagrelayout"
	"oss.terrastruct.com/d2/d2plugin"
	"oss.terrastruct.com/util-go/xmain"
)

type DagrePlugin struct {
	mu   sync.Mutex
	opts *d2dagrelayout.ConfigurableOpts
}

func (p *DagrePlugin) Flags(context.Context) ([]d2plugin.PluginSpecificFlag, error) {
	return []d2plugin.PluginSpecificFlag{
		{
			Name:    "dagre-nodesep",
			Type:    "int64",
			Default: int64(d2dagrelayout.DefaultOpts.NodeSep),
			Usage:   "number of pixels that separate nodes horizontally.",
			Tag:     "nodesep",
		},
		{
			Name:    "dagre-edgesep",
			Type:    "int64",
			Default: int64(d2dagrelayout.DefaultOpts.EdgeSep),
			Usage:   "number of pixels that separate edges horizontally.",
			Tag:     "edgesep",
		},
	}, nil
}

func (p *DagrePlugin) HydrateOpts(opts []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if opts != nil {
		var dagreOpts d2dagrelayout.ConfigurableOpts
		err := json.Unmarshal(opts, &dagreOpts)
		if err != nil {
			return xmain.UsageErrorf("non-dagre layout options given for dagre")
		}

		p.opts = &dagreOpts
	}
	return nil
}

func (p *DagrePlugin) Info(ctx context.Context) (*d2plugin.PluginInfo, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	opts := xmain.NewOpts(nil, nil)
	flags, err := p.Flags(ctx)
	if err != nil {
		return nil, err
	}
	for _, f := range flags {
		f.AddToOpts(opts)
	}

	return &d2plugin.PluginInfo{
		Name:      "dagre",
		Type:      "bundled",
		Features:  []d2plugin.PluginFeature{},
		ShortHelp: "The directed graph layout library Dagre",
		LongHelp: fmt.Sprintf(`dagre is a directed graph layout library for JavaScript.
See https://d2lang.com/tour/dagre for more.

Flags correspond to ones found at https://github.com/dagrejs/dagre/wiki.

Flags:
%s
`, opts.Defaults()),
	}, nil
}

func (p *DagrePlugin) Layout(ctx context.Context, g *d2graph.Graph) error {
	p.mu.Lock()
	optsCopy := *p.opts
	p.mu.Unlock()
	return d2dagrelayout.Layout(ctx, g, &optsCopy)
}

func (p *DagrePlugin) PostProcess(ctx context.Context, in []byte) ([]byte, error) {
	return in, nil
}
