// Package d2plugin enables the d2 CLI to run functions bundled
// with the d2 binary or via external plugin binaries.
//
// Binary plugins are stored in $PATH with the prefix d2plugin-*. i.e the binary for
// dagre might be d2plugin-dagre. See ListPlugins() below.
package d2plugin

import (
	"context"
	"os/exec"

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

// PluginInfo is the current info information of a plugin.
// note: The two fields Type and Path are not set by the plugin
// itself but only in ListPlugins.
type PluginInfo struct {
	Name      string `json:"name"`
	ShortHelp string `json:"shortHelp"`
	LongHelp  string `json:"longHelp"`

	// These two are set by ListPlugins and not the plugin itself.
	// bundled | binary
	Type string `json:"type"`
	// If Type == binary then this contains the absolute path to the binary.
	Path string `json:"path"`
}

const binaryPrefix = "d2plugin-"

func ListPlugins(ctx context.Context) ([]*PluginInfo, error) {
	// 1. Run Info on all bundled plugins in the global plugins array.
	//    - set Type for each bundled plugin to "bundled".
	// 2. Iterate through directories in $PATH and look for executables within these
	//    directories with the prefix d2plugin-*
	// 3. Run each plugin binary with the argument info. e.g. d2plugin-dagre info

	var infoSlice []*PluginInfo

	for _, p := range plugins {
		info, err := p.Info(ctx)
		if err != nil {
			return nil, err
		}
		info.Type = "bundled"
		infoSlice = append(infoSlice, info)
	}

	matches, err := xexec.SearchPath(binaryPrefix)
	if err != nil {
		return nil, err
	}
	for _, path := range matches {
		p := &execPlugin{path: path}
		info, err := p.Info(ctx)
		if err != nil {
			return nil, err
		}
		info.Type = "binary"
		info.Path = path
		infoSlice = append(infoSlice, info)
	}

	return infoSlice, nil
}

// FindPlugin finds the plugin with the given name.
// 1. It first searches the bundled plugins in the global plugins slice.
// 2. If not found, it then searches each directory in $PATH for a binary with the name
//    d2plugin-<name>.
//    **NOTE** When D2 upgrades to go 1.19, remember to ignore exec.ErrDot
// 3. If such a binary is found, it builds an execPlugin in exec.go
//    to get a plugin implementation around the binary and returns it.
func FindPlugin(ctx context.Context, name string) (Plugin, string, error) {
	for _, p := range plugins {
		info, err := p.Info(ctx)
		if err != nil {
			return nil, "", err
		}
		if info.Name == name {
			return p, "", nil
		}
	}

	path, err := exec.LookPath(binaryPrefix + name)
	if err != nil {
		return nil, "", err
	}

	return &execPlugin{path: path}, path, nil
}

func ListPluginFlags(ctx context.Context) ([]PluginSpecificFlag, error) {
	var out []PluginSpecificFlag
	for _, p := range plugins {
		flags, err := p.Flags(ctx)
		if err != nil {
			return nil, err
		}
		out = append(out, flags...)
	}

	matches, err := xexec.SearchPath(binaryPrefix)
	if err != nil {
		return nil, err
	}
	for _, path := range matches {
		p := &execPlugin{path: path}
		flags, err := p.Flags(ctx)
		if err != nil {
			return nil, err
		}
		out = append(out, flags...)
	}

	return out, nil
}
