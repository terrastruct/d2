//go:build !nodagre

package d2plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2layouts/d2dagrelayout"
	"oss.terrastruct.com/util-go/xmain"
)

var DagrePlugin = dagrePlugin{}

func init() {
	plugins = append(plugins, &DagrePlugin)
}

type dagrePlugin struct {
	mu   sync.Mutex
	opts *d2dagrelayout.ConfigurableOpts
}

func (p *dagrePlugin) Flags(context.Context) ([]PluginSpecificFlag, error) {
	return []PluginSpecificFlag{
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

func (p *dagrePlugin) HydrateOpts(opts []byte) error {
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

func (p *dagrePlugin) Info(ctx context.Context) (*PluginInfo, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	opts := xmain.NewOpts(nil, nil, nil)
	flags, err := p.Flags(ctx)
	if err != nil {
		return nil, err
	}
	for _, f := range flags {
		f.AddToOpts(opts)
	}

	return &PluginInfo{
		Name:      "dagre",
		Type:      "bundled",
		Features:  []PluginFeature{},
		ShortHelp: "The directed graph layout library Dagre",
		LongHelp: fmt.Sprintf(`dagre is a directed graph layout library for JavaScript.
See https://d2lang.com/tour/dagre for more.

Flags correspond to ones found at https://github.com/dagrejs/dagre/wiki.

Flags:
%s
`, opts.Defaults()),
	}, nil
}

func (p *dagrePlugin) Layout(ctx context.Context, g *d2graph.Graph) error {
	p.mu.Lock()
	optsCopy := *p.opts
	p.mu.Unlock()
	return d2dagrelayout.Layout(ctx, g, &optsCopy)
}

func (p *dagrePlugin) PostProcess(ctx context.Context, in []byte) ([]byte, error) {
	return in, nil
}
