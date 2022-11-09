package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "embed"

	"github.com/spf13/pflag"

	"oss.terrastruct.com/d2"
	"oss.terrastruct.com/d2/d2plugin"
	"oss.terrastruct.com/d2/d2renderers/d2svg"
	"oss.terrastruct.com/d2/d2renderers/textmeasure"
	"oss.terrastruct.com/d2/d2themes"
	"oss.terrastruct.com/d2/d2themes/d2themescatalog"
	"oss.terrastruct.com/d2/lib/version"
	"oss.terrastruct.com/d2/lib/xmain"
)

func main() {
	xmain.Main(run)
}

func parseFlagsToEnv(ctx context.Context, ms *xmain.State) error {
	err := ms.FlagSet.Parse(ms.Args)

	watchFlag := ms.FlagSet.BoolP(
		"watch", "w",
		ms.Env.Getenv("D2_WATCH") == "1" || ms.Env.Getenv("D2_WATCH") == "true",
		"watch for changes to input and live reload. Use $HOST and $PORT to specify the listening address.\n$D2_HOST and $D2_PORT are also accepted and take priority (default localhost:0, which is will open on a randomly available local port).",
	)
	if *watchFlag {
		ms.Env.Setenv("D2_WATCH", "1")
	} else {
		ms.Env.Setenv("D2_WATCH", "0")
	}

	bundleFlag := ms.FlagSet.BoolP(
		"bundle", "b",
		!(ms.Env.Getenv("D2_BUNDLE") == "0" || ms.Env.Getenv("D2_BUNDLE") == "false"),
		"bundle all assets and layers into the output svg.",
	)
	if *bundleFlag {
		ms.Env.Setenv("D2_BUNDLE", "1")
	} else {
		ms.Env.Setenv("D2_BUNDLE", "0")
	}

	debugFlag := ms.FlagSet.BoolP(
		"debug", "d",
		ms.Env.Getenv("D2_DEBUG") == "1" || ms.Env.Getenv("D2_DEBUG") == "true",
		"print debug logs.",
	)
	if *debugFlag {
		ms.Env.Setenv("D2_DEBUG", "1")
	} else {
		ms.Env.Setenv("D2_DEBUG", "0")
	}

	layoutEnvVal := ms.Env.Getenv("D2_LAYOUT")
	if layoutEnvVal == "" {
		layoutEnvVal = "dagre"
	}
	layoutFlag := ms.FlagSet.StringP("layout", "l", layoutEnvVal, `the layout engine used.`)
	ms.Env.Setenv("D2_LAYOUT", *layoutFlag)

	ev := ms.Env.Getenv("D2_THEME")
	var themeEnvVal int64
	if ev != "" {
		themeEnvVal, err = strconv.ParseInt(ev, 10, 64)
	}
	themeFlag := ms.FlagSet.Int64P("theme", "t", themeEnvVal, "the diagram theme ID. For a list of available options, see https://oss.terrastruct.com/d2")
	match := d2themescatalog.Find(*themeFlag)
	if match == (d2themes.Theme{}) {
		return xmain.UsageErrorf("-t[heme] could not be found. The available options are:\n%s\nYou provided: %d", d2themescatalog.CLIString(), *themeFlag)
	}
	ms.Env.Setenv("D2_THEME", fmt.Sprintf("%d", *themeFlag))

	if !errors.Is(err, pflag.ErrHelp) && err != nil {
		return xmain.UsageErrorf("failed to parse flags: %v", err)
	}

	return nil
}

func run(ctx context.Context, ms *xmain.State) (err error) {
	// :(
	ctx = xmain.DiscardSlog(ctx)

	if err := parseFlagsToEnv(ctx, ms); err != nil {
		return err
	}
	// Flags that don't make sense to set in env
	versionFlag := ms.FlagSet.BoolP("version", "v", false, "get the version and check for updates")

	if len(ms.FlagSet.Args()) > 0 {
		switch ms.FlagSet.Arg(0) {
		case "layout":
			return layoutHelp(ctx, ms)
		}
	}

	if errors.Is(err, pflag.ErrHelp) {
		help(ms)
		return nil
	}

	var inputPath string
	var outputPath string

	if len(ms.FlagSet.Args()) == 0 {
		if versionFlag != nil && *versionFlag {
			version.CheckVersion(ctx, ms.Log)
			return nil
		}
		help(ms)
		return nil
	} else if len(ms.FlagSet.Args()) >= 3 {
		return xmain.UsageErrorf("too many arguments passed")
	}
	if len(ms.FlagSet.Args()) >= 1 {
		if ms.FlagSet.Arg(0) == "version" {
			version.CheckVersion(ctx, ms.Log)
			return nil
		}
		inputPath = ms.FlagSet.Arg(0)
	}
	if len(ms.FlagSet.Args()) >= 2 {
		outputPath = ms.FlagSet.Arg(1)
	} else {
		if inputPath == "-" {
			outputPath = "-"
		} else {
			outputPath = renameExt(inputPath, ".svg")
		}
	}

	plugin, path, err := d2plugin.FindPlugin(ctx, ms.Env.Getenv("D2_LAYOUT"))
	if errors.Is(err, exec.ErrNotFound) {
		return layoutNotFound(ctx, ms.Env.Getenv("D2_LAYOUT"))
	} else if err != nil {
		return err
	}

	pluginLocation := "bundled"
	if path != "" {
		pluginLocation = fmt.Sprintf("executable plugin at %s", humanPath(path))
	}
	ms.Log.Debug.Printf("using layout plugin %s (%s)", ms.Env.Getenv("D2_LAYOUT"), pluginLocation)

	if ms.Env.Getenv("D2_WATCH") == "1" {
		if inputPath == "-" {
			return xmain.UsageErrorf("-w[atch] cannot be combined with reading input from stdin")
		}
		ms.Env.Setenv("LOG_TIMESTAMPS", "1")
		w, err := newWatcher(ctx, ms, plugin, inputPath, outputPath)
		if err != nil {
			return err
		}
		return w.run()
	}

	ctx, cancel := context.WithTimeout(ctx, time.Minute*2)
	defer cancel()

	if ms.Env.Getenv("D2_BUNDLE") == "1" {
		_ = 343
	}

	_, err = compile(ctx, ms, plugin, inputPath, outputPath)
	if err != nil {
		return err
	}
	ms.Log.Success.Printf("successfully compiled %v to %v", inputPath, outputPath)
	return nil
}

func compile(ctx context.Context, ms *xmain.State, plugin d2plugin.Plugin, inputPath, outputPath string) ([]byte, error) {
	input, err := ms.ReadPath(inputPath)
	if err != nil {
		return nil, err
	}

	ruler, err := textmeasure.NewRuler()
	if err != nil {
		return nil, err
	}

	themeID, _ := strconv.ParseInt(ms.Env.Getenv("D2_THEME"), 10, 64)
	d, err := d2.Compile(ctx, string(input), &d2.CompileOptions{
		Layout:  plugin.Layout,
		Ruler:   ruler,
		ThemeID: themeID,
	})
	if err != nil {
		return nil, err
	}

	svg, err := d2svg.Render(d)
	if err != nil {
		return nil, err
	}
	svg, err = plugin.PostProcess(ctx, svg)
	if err != nil {
		return nil, err
	}

	err = ms.WritePath(outputPath, svg)
	if err != nil {
		return nil, err
	}
	return svg, nil
}

// newExt must include leading .
func renameExt(fp string, newExt string) string {
	ext := filepath.Ext(fp)
	if ext == "" {
		return fp + newExt
	} else {
		return strings.TrimSuffix(fp, ext) + newExt
	}
}
