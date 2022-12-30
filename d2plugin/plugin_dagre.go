//go:build !nodagre

package d2plugin

import (
	"context"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2layouts/d2dagrelayout"
	"oss.terrastruct.com/util-go/xmain"
)

var DagrePlugin = dagrePlugin{}

func init() {
	plugins = append(plugins, &DagrePlugin)
}

type dagrePlugin struct {
	opts *d2dagrelayout.Opts
}

func (p *dagrePlugin) HydrateOpts(ctx context.Context, opts interface{}) error {
	if opts != nil {
		dagreOpts, ok := opts.(d2dagrelayout.Opts)
		if !ok {
			return xmain.UsageErrorf("non-dagre layout options given for dagre")
		}

		p.opts = &dagreOpts
	}
	return nil
}

func (p dagrePlugin) Info(context.Context) (*PluginInfo, error) {
	return &PluginInfo{
		Name:      "dagre",
		ShortHelp: "The directed graph layout library Dagre",
		LongHelp: `dagre is a directed graph layout library for JavaScript.
See https://github.com/dagrejs/dagre
The implementation of this plugin is at: https://github.com/terrastruct/d2/tree/master/d2plugin/d2dagrelayout

note: dagre is the primary layout algorithm for text to diagram generator Mermaid.js.
      See https://github.com/mermaid-js/mermaid
      We have a useful comparison at https://text-to-diagram.com/?example=basic&a=d2&b=mermaid
`,
	}, nil
}

func (p dagrePlugin) Layout(ctx context.Context, g *d2graph.Graph) error {
	return d2dagrelayout.Layout(ctx, g, p.opts)
}

func (p dagrePlugin) PostProcess(ctx context.Context, in []byte) ([]byte, error) {
	return in, nil
}
