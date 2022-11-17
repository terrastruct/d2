package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
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

func run(ctx context.Context, ms *xmain.State) (err error) {
	// :(
	ctx = xmain.DiscardSlog(ctx)

	watchFlag := ms.Opts.Bool("D2_WATCH", "watch", "w", false, "watch for changes to input and live reload. Use $HOST and $PORT to specify the listening address.\n$D2_HOST and $D2_PORT are also accepted and take priority (default localhost:0, which is will open on a randomly available local port).")
	bundleFlag := ms.Opts.Bool("D2_BUNDLE", "bundle", "b", true, "bundle all assets and layers into the output svg.")
	debugFlag := ms.Opts.Bool("DEBUG", "debug", "d", false, "print debug logs.")
	layoutFlag := ms.Opts.String("D2_LAYOUT", "layout", "l", "dagre", `the layout engine used.`)
	themeFlag := ms.Opts.Int64("D2_THEME", "theme", "t", 0, "the diagram theme ID. For a list of available options, see https://oss.terrastruct.com/d2")
	versionFlag := ms.Opts.Bool("", "version", "v", false, "get the version")

	err = ms.Opts.Parse()
	if !errors.Is(err, pflag.ErrHelp) && err != nil {
		return xmain.UsageErrorf("failed to parse flags: %v", err)
	}

	if len(ms.Opts.Args()) > 0 {
		switch ms.Opts.Arg(0) {
		case "layout":
			return layoutHelp(ctx, ms)
		}
	}

	if errors.Is(err, pflag.ErrHelp) {
		help(ms)
		return nil
	}

	if *debugFlag {
		ms.Env.Setenv("DEBUG", "1")
	}

	var inputPath string
	var outputPath string

	if len(ms.Opts.Args()) == 0 {
		if versionFlag != nil && *versionFlag {
			fmt.Println(version.Version)
			return nil
		}
		help(ms)
		return nil
	} else if len(ms.Opts.Args()) >= 3 {
		return xmain.UsageErrorf("too many arguments passed")
	}

	if len(ms.Opts.Args()) >= 1 {
		if ms.Opts.Arg(0) == "version" {
			fmt.Println(version.Version)
			return nil
		}
		inputPath = ms.Opts.Arg(0)
	}
	if len(ms.Opts.Args()) >= 2 {
		outputPath = ms.Opts.Arg(1)
	} else {
		if inputPath == "-" {
			outputPath = "-"
		} else {
			outputPath = renameExt(inputPath, ".svg")
		}
	}

	match := d2themescatalog.Find(*themeFlag)
	if match == (d2themes.Theme{}) {
		return xmain.UsageErrorf("-t[heme] could not be found. The available options are:\n%s\nYou provided: %d", d2themescatalog.CLIString(), *themeFlag)
	}
	ms.Log.Debug.Printf("using theme %s (ID: %d)", match.Name, *themeFlag)

	plugin, path, err := d2plugin.FindPlugin(ctx, *layoutFlag)
	if errors.Is(err, exec.ErrNotFound) {
		return layoutNotFound(ctx, *layoutFlag)
	} else if err != nil {
		return err
	}

	pluginLocation := "bundled"
	if path != "" {
		pluginLocation = fmt.Sprintf("executable plugin at %s", humanPath(path))
	}
	ms.Log.Debug.Printf("using layout plugin %s (%s)", *layoutFlag, pluginLocation)

	if *watchFlag {
		if inputPath == "-" {
			return xmain.UsageErrorf("-w[atch] cannot be combined with reading input from stdin")
		}
		ms.Env.Setenv("LOG_TIMESTAMPS", "1")
		w, err := newWatcher(ctx, ms, plugin, *themeFlag, inputPath, outputPath)
		if err != nil {
			return err
		}
		return w.run()
	}

	ctx, cancel := context.WithTimeout(ctx, time.Minute*2)
	defer cancel()

	if *bundleFlag {
		_ = 343
	}

	_, err = compile(ctx, ms, plugin, *themeFlag, inputPath, outputPath)
	if err != nil {
		return err
	}
	ms.Log.Success.Printf("successfully compiled %v to %v", inputPath, outputPath)
	return nil
}

func compile(ctx context.Context, ms *xmain.State, plugin d2plugin.Plugin, themeID int64, inputPath, outputPath string) ([]byte, error) {
	input, err := ms.ReadPath(inputPath)
	if err != nil {
		return nil, err
	}

	ruler, err := textmeasure.NewRuler()
	if err != nil {
		return nil, err
	}

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
