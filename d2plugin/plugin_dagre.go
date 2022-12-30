//go:build !nodagre

package d2plugin

import (
	"context"
	"encoding/json"

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

func (p dagrePlugin) Flags() []PluginSpecificFlag {
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
	}
}

func (p *dagrePlugin) HydrateOpts(opts []byte) error {
	if opts != nil {
		var dagreOpts d2dagrelayout.Opts
		err := json.Unmarshal(opts, &dagreOpts)
		if err != nil {
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
