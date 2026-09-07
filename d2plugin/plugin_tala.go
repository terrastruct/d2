package d2plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"

	"github.com/d2lang/util-go/xmain"

	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/d2layouts/d2talalayout"
)

var TALAPlugin = *newTALAPlugin()

func init() {
	plugins = append(plugins, &TALAPlugin)
}

type talaPlugin struct {
	mu   sync.RWMutex
	opts d2talalayout.Options
}

func newTALAPlugin() *talaPlugin {
	opts := d2talalayout.DefaultOptions()
	opts.Seeds = slices.Clone(opts.Seeds)
	return &talaPlugin{opts: opts}
}

func (*talaPlugin) newInstance() Plugin {
	return newTALAPlugin()
}

func (p *talaPlugin) Flags(context.Context) ([]PluginSpecificFlag, error) {
	defaults := d2talalayout.DefaultOptions()
	return []PluginSpecificFlag{
		{
			Name:    "tala-seeds",
			Type:    "[]int64",
			Default: slices.Clone(defaults.Seeds),
			Usage:   "random seeds for deterministic TALA layout attempts; the best complete result is selected.",
			Tag:     "tala-seeds",
		},
	}, nil
}

func (p *talaPlugin) HydrateOpts(raw []byte) error {
	if raw == nil {
		return nil
	}
	opts, err := decodeTALAOptions(raw)
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.opts = opts
	p.mu.Unlock()
	return nil
}

func (p *talaPlugin) Info(ctx context.Context) (*PluginInfo, error) {
	opts := xmain.NewOpts(nil, nil)
	flags, err := p.Flags(ctx)
	if err != nil {
		return nil, err
	}
	for _, flag := range flags {
		flag.AddToOpts(opts)
	}
	return &PluginInfo{
		Name: "tala",
		Type: "bundled",
		Features: []PluginFeature{
			DESCENDANT_EDGES,
			CONTAINER_DIMENSIONS,
			NEAR_OBJECT,
			TOP_LEFT,
			ROUTES_EDGES,
		},
		ShortHelp: "TALA is D2's native layout and edge-routing engine.",
		LongHelp: fmt.Sprintf(`TALA is D2's native layout and edge-routing engine for software architecture diagrams.

Diagram data under tala-seeds takes precedence over the command-line flag.

Flags:
%s
`, opts.Defaults()),
	}, nil
}

func (p *talaPlugin) Layout(ctx context.Context, graph *d2graph.Graph) error {
	opts := p.optionsSnapshot()
	return d2talalayout.Layout(ctx, graph, &opts)
}

func (p *talaPlugin) RouteEdges(ctx context.Context, graph *d2graph.Graph, edges []*d2graph.Edge) error {
	return d2talalayout.RouteEdges(ctx, graph, edges)
}

func (p *talaPlugin) optionsSnapshot() d2talalayout.Options {
	p.mu.RLock()
	opts := p.opts
	opts.Seeds = slices.Clone(p.opts.Seeds)
	p.mu.RUnlock()
	return opts
}

func decodeTALAOptions(raw []byte) (d2talalayout.Options, error) {
	opts := d2talalayout.DefaultOptions()
	if raw != nil {
		if err := json.Unmarshal(raw, &opts); err != nil {
			return d2talalayout.Options{}, xmain.UsageErrorf("non-TALA layout options given for TALA")
		}
	}
	opts.Seeds = slices.Clone(opts.Seeds)
	return opts, nil
}

var (
	_ Plugin          = (*talaPlugin)(nil)
	_ RoutingPlugin   = (*talaPlugin)(nil)
	_ pluginInstancer = (*talaPlugin)(nil)
)
