package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/playwright-community/playwright-go"
	"github.com/spf13/pflag"

	"oss.terrastruct.com/d2"
	"oss.terrastruct.com/d2/d2plugin"
	"oss.terrastruct.com/d2/d2renderers/d2svg"
	"oss.terrastruct.com/d2/d2renderers/textmeasure"
	"oss.terrastruct.com/d2/d2themes"
	"oss.terrastruct.com/d2/d2themes/d2themescatalog"
	"oss.terrastruct.com/d2/lib/imgbundler"
	"oss.terrastruct.com/d2/lib/png"
	"oss.terrastruct.com/d2/lib/version"
	"oss.terrastruct.com/d2/lib/xmain"
)

func main() {
	xmain.Main(run)
}

func run(ctx context.Context, ms *xmain.State) (err error) {
	// :(
	ctx = xmain.DiscardSlog(ctx)

	// These should be kept up-to-date with the d2 man page
	watchFlag, err := ms.Opts.Bool("D2_WATCH", "watch", "w", false, "watch for changes to input and live reload. Use $HOST and $PORT to specify the listening address.\n(default localhost:0, which is will open on a randomly available local port).")
	if err != nil {
		return xmain.UsageErrorf(err.Error())
	}
	hostFlag := ms.Opts.String("HOST", "host", "h", "localhost", "host listening address when used with watch")
	portFlag := ms.Opts.String("PORT", "port", "p", "0", "port listening address when used with watch")
	bundleFlag, err := ms.Opts.Bool("D2_BUNDLE", "bundle", "b", true, "when outputting SVG, bundle all assets and layers into the output file.")
	if err != nil {
		return xmain.UsageErrorf(err.Error())
	}
	debugFlag, err := ms.Opts.Bool("DEBUG", "debug", "d", false, "print debug logs.")
	if err != nil {
		return xmain.UsageErrorf(err.Error())
	}
	layoutFlag := ms.Opts.String("D2_LAYOUT", "layout", "l", "dagre", `the layout engine used.`)
	themeFlag, err := ms.Opts.Int64("D2_THEME", "theme", "t", 0, "the diagram theme ID. For a list of available options, see https://oss.terrastruct.com/d2")
	if err != nil {
		return xmain.UsageErrorf(err.Error())
	}
	versionFlag, err := ms.Opts.Bool("", "version", "v", false, "get the version")
	if err != nil {
		return xmain.UsageErrorf(err.Error())
	}

	err = ms.Opts.Flags.Parse(ms.Opts.Args)
	if !errors.Is(err, pflag.ErrHelp) && err != nil {
		return xmain.UsageErrorf("failed to parse flags: %v", err)
	}

	if len(ms.Opts.Flags.Args()) > 0 {
		switch ms.Opts.Flags.Arg(0) {
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

	if len(ms.Opts.Flags.Args()) == 0 {
		if versionFlag != nil && *versionFlag {
			fmt.Println(version.Version)
			return nil
		}
		help(ms)
		return nil
	} else if len(ms.Opts.Flags.Args()) >= 3 {
		return xmain.UsageErrorf("too many arguments passed")
	}

	if len(ms.Opts.Flags.Args()) >= 1 {
		if ms.Opts.Flags.Arg(0) == "version" {
			fmt.Println(version.Version)
			return nil
		}
		inputPath = ms.Opts.Flags.Arg(0)
	}
	if len(ms.Opts.Flags.Args()) >= 2 {
		outputPath = ms.Opts.Flags.Arg(1)
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

	var pw png.Playwright
	if filepath.Ext(outputPath) == ".png" {
		pw, err = png.InitPlaywright()
		if err != nil {
			return err
		}
		defer func() {
			cleanupErr := pw.Cleanup()
			if err == nil {
				err = cleanupErr
			}
		}()
	}

	if *watchFlag {
		if inputPath == "-" {
			return xmain.UsageErrorf("-w[atch] cannot be combined with reading input from stdin")
		}
		ms.Env.Setenv("LOG_TIMESTAMPS", "1")
		w, err := newWatcher(ctx, ms, watcherOpts{
			layoutPlugin: plugin,
			themeID:      *themeFlag,
			host:         *hostFlag,
			port:         *portFlag,
			inputPath:    inputPath,
			outputPath:   outputPath,
			pw:           pw,
		})
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

	_, err = compile(ctx, ms, plugin, *themeFlag, inputPath, outputPath, pw.Page)
	if err != nil {
		return err
	}
	ms.Log.Success.Printf("successfully compiled %v to %v", inputPath, outputPath)
	return nil
}

func compile(ctx context.Context, ms *xmain.State, plugin d2plugin.Plugin, themeID int64, inputPath, outputPath string, page playwright.Page) ([]byte, error) {
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

	// TODO this may be desirable even for SVGs. Should make it a flag
	svg, err = imgbundler.Inline(ms, svg)
	if err != nil {
		return nil, err
	}

	out := svg
	if filepath.Ext(outputPath) == ".png" {
		out, err = png.ConvertSVG(ms, page, svg)
		if err != nil {
			return nil, err
		}
	}

	err = ms.WritePath(outputPath, out)
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
