// Package d2plugin enables the d2 CLI to run functions bundled
// with the d2 binary or via external plugin binaries.
//
// Binary plugins are stored in $PATH with the prefix d2plugin-*. i.e the binary for
// dagre might be d2plugin-dagre. See ListPlugins() below.
package d2plugin

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"

	"oss.terrastruct.com/util-go/xexec"
	"oss.terrastruct.com/util-go/xmain"

	"oss.terrastruct.com/d2/d2graph"
)

// plugins contains the bundled d2 plugins.
//
// See plugin_* files for the plugins available for bundling.
var plugins []Plugin

type PluginSpecificFlag struct {
	Name    string
	Type    string
	Default interface{}
	Usage   string
	// Must match the tag in the opt
	Tag string
}

func (f *PluginSpecificFlag) AddToOpts(opts *xmain.Opts) {
	switch f.Type {
	case "string":
		opts.String("", f.Name, "", f.Default.(string), f.Usage)
	case "int64":
		var val int64
		switch defaultType := f.Default.(type) {
		case int64:
			val = defaultType
		case float64:
			// json unmarshals numbers to float64
			val = int64(defaultType)
		}
		opts.Int64("", f.Name, "", val, f.Usage)
	case "[]int64":
		var slice []int64
		switch defaultType := f.Default.(type) {
		case []int64:
			slice = defaultType
		case []interface{}:
			for _, v := range defaultType {
				switch defaultType := v.(type) {
				case int64:
					slice = append(slice, defaultType)
				case float64:
					// json unmarshals numbers to float64
					slice = append(slice, int64(defaultType))
				}
			}
		}
		opts.Int64Slice("", f.Name, "", slice, f.Usage)
	}
}

type Plugin interface {
	// Info returns the current info information of the plugin.
	Info(context.Context) (*PluginInfo, error)

	Flags(context.Context) ([]PluginSpecificFlag, error)

	HydrateOpts([]byte) error

	// Layout runs the plugin's autolayout algorithm on the input graph
	// and returns a new graph with the computed placements.
	Layout(context.Context, *d2graph.Graph) error

	// PostProcess makes changes to the default render
	PostProcess(context.Context, []byte) ([]byte, error)
}

type RoutingPlugin interface {
	// RouteEdges runs the plugin's edge routing algorithm for the given edges in the input graph
	RouteEdges(context.Context, *d2graph.Graph, []*d2graph.Edge) error
}

type routeEdgesInput struct {
	G      []byte `json:"g"`
	GEdges []byte `json:"gEdges"`
}

// PluginInfo is the current info information of a plugin.
// note: The two fields Type and Path are not set by the plugin
// itself but only in ListPlugins.
type PluginInfo struct {
	Name      string `json:"name"`
	ShortHelp string `json:"shortHelp"`
	LongHelp  string `json:"longHelp"`

	// Set to bundled when returning from the plugin.
	// execPlugin will set to binary when used.
	// bundled | binary
	Type string `json:"type"`
	// If Type == binary then this contains the absolute path to the binary.
	Path string `json:"path"`

	Features []PluginFeature `json:"features"`
}

const binaryPrefix = "d2plugin-"

func ListPlugins(ctx context.Context) ([]Plugin, error) {
	// 1. Run Info on all bundled plugins in the global plugins array.
	//    - set Type for each bundled plugin to "bundled".
	// 2. Iterate through directories in $PATH and look for executables within these
	//    directories with the prefix d2plugin-*
	// 3. Run each plugin binary with the argument info. e.g. d2plugin-dagre info

	var ps []Plugin
	ps = append(ps, plugins...)

	matches, err := xexec.SearchPath(binaryPrefix)
	if err != nil {
		return nil, err
	}
BINARY_PLUGINS_LOOP:
	for _, path := range matches {
		p := &execPlugin{path: path}
		info, err := p.Info(ctx)
		if err != nil {
			return nil, err
		}
		for _, p2 := range ps {
			info2, err := p2.Info(ctx)
			if err != nil {
				return nil, err
			}
			if info.Name == info2.Name {
				continue BINARY_PLUGINS_LOOP
			}
		}
		ps = append(ps, p)
	}
	return ps, nil
}

func ListPluginInfos(ctx context.Context, ps []Plugin) ([]*PluginInfo, error) {
	var infoSlice []*PluginInfo
	for _, p := range ps {
		info, err := p.Info(ctx)
		if err != nil {
			return nil, err
		}
		infoSlice = append(infoSlice, info)
	}

	return infoSlice, nil
}

// FindPlugin finds the plugin with the given name.
//  1. It first searches the bundled plugins in the global plugins slice.
//  2. If not found, it then searches each directory in $PATH for a binary with the name
//     d2plugin-<name>.
//  3. If such a binary is found, it builds an execPlugin in exec.go
//     to get a plugin implementation around the binary and returns it.
func FindPlugin(ctx context.Context, ps []Plugin, name string) (Plugin, error) {
	for _, p := range ps {
		info, err := p.Info(ctx)
		if err != nil {
			return nil, err
		}
		if strings.EqualFold(info.Name, name) {
			return p, nil
		}
	}
	return nil, exec.ErrNotFound
}

func ListPluginFlags(ctx context.Context, ps []Plugin) ([]PluginSpecificFlag, error) {
	var out []PluginSpecificFlag
	for _, p := range ps {
		flags, err := p.Flags(ctx)
		if err != nil {
			return nil, err
		}
		out = append(out, flags...)
	}

	return out, nil
}

func HydratePluginOpts(ctx context.Context, ms *xmain.State, plugin Plugin) error {
	opts := make(map[string]interface{})
	flags, err := plugin.Flags(ctx)
	if err != nil {
		return err
	}
	for _, f := range flags {
		switch f.Type {
		case "string":
			val, _ := ms.Opts.Flags.GetString(f.Name)
			opts[f.Tag] = val
		case "int64":
			val, _ := ms.Opts.Flags.GetInt64(f.Name)
			opts[f.Tag] = val
		case "[]int64":
			val, _ := ms.Opts.Flags.GetInt64Slice(f.Name)
			opts[f.Tag] = val
		}
	}

	b, err := json.Marshal(opts)
	if err != nil {
		return err
	}

	return plugin.HydrateOpts(b)
}
