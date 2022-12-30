package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/playwright-community/playwright-go"
	"github.com/spf13/pflag"
	"go.uber.org/multierr"

	"oss.terrastruct.com/util-go/go2"
	"oss.terrastruct.com/util-go/xmain"

	"oss.terrastruct.com/d2/d2lib"
	"oss.terrastruct.com/d2/d2plugin"
	"oss.terrastruct.com/d2/d2renderers/d2fonts"
	"oss.terrastruct.com/d2/d2renderers/d2svg"
	"oss.terrastruct.com/d2/d2renderers/d2svg/appendix"
	"oss.terrastruct.com/d2/d2themes"
	"oss.terrastruct.com/d2/d2themes/d2themescatalog"
	"oss.terrastruct.com/d2/lib/imgbundler"
	ctxlog "oss.terrastruct.com/d2/lib/log"
	"oss.terrastruct.com/d2/lib/png"
	"oss.terrastruct.com/d2/lib/textmeasure"
	"oss.terrastruct.com/d2/lib/version"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
)

func main() {
	xmain.Main(run)
}

func run(ctx context.Context, ms *xmain.State) (err error) {
	// :(
	ctx = DiscardSlog(ctx)

	// These should be kept up-to-date with the d2 man page
	watchFlag, err := ms.Opts.Bool("D2_WATCH", "watch", "w", false, "watch for changes to input and live reload. Use $HOST and $PORT to specify the listening address.\n(default localhost:0, which is will open on a randomly available local port).")
	if err != nil {
		return err
	}
	hostFlag := ms.Opts.String("HOST", "host", "h", "localhost", "host listening address when used with watch")
	portFlag := ms.Opts.String("PORT", "port", "p", "0", "port listening address when used with watch")
	bundleFlag, err := ms.Opts.Bool("D2_BUNDLE", "bundle", "b", true, "when outputting SVG, bundle all assets and layers into the output file")
	if err != nil {
		return err
	}
	debugFlag, err := ms.Opts.Bool("DEBUG", "debug", "d", false, "print debug logs.")
	if err != nil {
		return err
	}
	layoutFlag := ms.Opts.String("D2_LAYOUT", "layout", "l", "dagre", `the layout engine used`)
	themeFlag, err := ms.Opts.Int64("D2_THEME", "theme", "t", 0, "the diagram theme ID. For a list of available options, see https://oss.terrastruct.com/d2")
	if err != nil {
		return err
	}
	padFlag, err := ms.Opts.Int64("D2_PAD", "pad", "", d2svg.DEFAULT_PADDING, "pixels padded around the rendered diagram")
	if err != nil {
		return err
	}
	versionFlag, err := ms.Opts.Bool("", "version", "v", false, "get the version")
	if err != nil {
		return err
	}
	sketchFlag, err := ms.Opts.Bool("D2_SKETCH", "sketch", "s", false, "render the diagram to look like it was sketched by hand")
	if err != nil {
		return err
	}

	err = populateLayoutOpts(ms)
	if err != nil {
		return err
	}

	err = ms.Opts.Flags.Parse(ms.Opts.Args)
	if !errors.Is(err, pflag.ErrHelp) && err != nil {
		return xmain.UsageErrorf("failed to parse flags: %v", err)
	}

	if errors.Is(err, pflag.ErrHelp) {
		help(ms)
		return nil
	}

	if len(ms.Opts.Flags.Args()) > 0 {
		switch ms.Opts.Flags.Arg(0) {
		case "layout":
			return layoutCmd(ctx, ms)
		case "fmt":
			return fmtCmd(ctx, ms)
		case "version":
			if len(ms.Opts.Flags.Args()) > 1 {
				return xmain.UsageErrorf("version subcommand accepts no arguments")
			}
			fmt.Println(version.Version)
			return nil
		}
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

	err = parseLayoutOpts(ms, plugin)
	if err != nil {
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
		ms.Log.SetTS(true)
		w, err := newWatcher(ctx, ms, watcherOpts{
			layoutPlugin: plugin,
			sketch:       *sketchFlag,
			themeID:      *themeFlag,
			pad:          *padFlag,
			host:         *hostFlag,
			port:         *portFlag,
			inputPath:    inputPath,
			outputPath:   outputPath,
			bundle:       *bundleFlag,
			pw:           pw,
		})
		if err != nil {
			return err
		}
		return w.run()
	}

	ctx, cancel := context.WithTimeout(ctx, time.Minute*2)
	defer cancel()

	_, written, err := compile(ctx, ms, plugin, *sketchFlag, *padFlag, *themeFlag, inputPath, outputPath, *bundleFlag, pw.Page)
	if err != nil {
		if written {
			return fmt.Errorf("failed to fully compile (partial render written): %w", err)
		}
		return fmt.Errorf("failed to compile: %w", err)
	}
	ms.Log.Success.Printf("successfully compiled %v to %v", inputPath, outputPath)
	return nil
}

func compile(ctx context.Context, ms *xmain.State, plugin d2plugin.Plugin, sketch bool, pad, themeID int64, inputPath, outputPath string, bundle bool, page playwright.Page) (_ []byte, written bool, _ error) {
	input, err := ms.ReadPath(inputPath)
	if err != nil {
		return nil, false, err
	}

	ruler, err := textmeasure.NewRuler()
	if err != nil {
		return nil, false, err
	}

	layout := plugin.Layout
	opts := &d2lib.CompileOptions{
		Layout:  layout,
		Ruler:   ruler,
		ThemeID: themeID,
	}
	if sketch {
		opts.FontFamily = go2.Pointer(d2fonts.HandDrawn)
	}
	diagram, _, err := d2lib.Compile(ctx, string(input), opts)
	if err != nil {
		return nil, false, err
	}

	svg, err := d2svg.Render(diagram, &d2svg.RenderOpts{
		Pad:    int(pad),
		Sketch: sketch,
	})
	if err != nil {
		return nil, false, err
	}

	svg, err = plugin.PostProcess(ctx, svg)
	if err != nil {
		return svg, false, err
	}

	svg, bundleErr := imgbundler.BundleLocal(ctx, ms, svg)
	if bundle {
		var bundleErr2 error
		svg, bundleErr2 = imgbundler.BundleRemote(ctx, ms, svg)
		bundleErr = multierr.Combine(bundleErr, bundleErr2)
	}

	out := svg
	if filepath.Ext(outputPath) == ".png" {
		svg := appendix.Append(diagram, ruler, svg)

		if !bundle {
			var bundleErr2 error
			svg, bundleErr2 = imgbundler.BundleRemote(ctx, ms, svg)
			bundleErr = multierr.Combine(bundleErr, bundleErr2)
		}

		out, err = png.ConvertSVG(ms, page, svg)
		if err != nil {
			return svg, false, err
		}
	} else {
		if len(out) > 0 && out[len(out)-1] != '\n' {
			out = append(out, '\n')
		}
	}

	err = ms.WritePath(outputPath, out)
	if err != nil {
		return svg, false, err
	}

	return svg, true, bundleErr
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

// TODO: remove after removing slog
func DiscardSlog(ctx context.Context) context.Context {
	return ctxlog.With(ctx, slog.Make(sloghuman.Sink(io.Discard)))
}

func populateLayoutOpts(ms *xmain.State) error {
	pluginFlags := d2plugin.ListPluginFlags()

	for _, f := range pluginFlags {
		switch f.Type {
		case "string":
			ms.Opts.String("", f.Name, "", f.Default.(string), f.Usage)
		case "int64":
			ms.Opts.Int64("", f.Name, "", f.Default.(int64), f.Usage)
		}
	}

	return nil
}

func parseLayoutOpts(ms *xmain.State, plugin d2plugin.Plugin) error {
	opts := make(map[string]interface{})
	for _, f := range plugin.Flags() {
		switch f.Type {
		case "string":
			val, _ := ms.Opts.Flags.GetString(f.Name)
			opts[f.Tag] = val
		case "int64":
			val, _ := ms.Opts.Flags.GetInt64(f.Name)
			opts[f.Tag] = val
		}
	}

	b, err := json.Marshal(opts)
	if err != nil {
		return err
	}

	err = plugin.HydrateOpts(b)
	return err

	// switch layout {
	// case "dagre":
	//   nodesep, _ := ms.Opts.Flags.GetInt64("dagre-nodesep")
	//   edgesep, _ := ms.Opts.Flags.GetInt64("dagre-edgesep")
	//   return d2dagrelayout.Opts{
	//     NodeSep: int(nodesep),
	//     EdgeSep: int(edgesep),
	//   }, nil
	// case "elk":
	//   algorithm, _ := ms.Opts.Flags.GetString("elk-algorithm")
	//   nodeSpacing, _ := ms.Opts.Flags.GetInt64("elk-nodeNodeBetweenLayers")
	//   padding, _ := ms.Opts.Flags.GetString("elk-padding")
	//   edgeNodeSpacing, _ := ms.Opts.Flags.GetInt64("elk-edgeNodeSpacing")
	//   selfLoopSpacing, _ := ms.Opts.Flags.GetInt64("elk-nodeSelfLoop")
	//   return d2elklayout.ConfigurableOpts{
	//     Algorithm:       algorithm,
	//     NodeSpacing:     int(nodeSpacing),
	//     Padding:         padding,
	//     EdgeNodeSpacing: int(edgeNodeSpacing),
	//     SelfLoopSpacing: int(selfLoopSpacing),
	//   }, nil
	// default:
	//
	// }
	//
	// return nil, fmt.Errorf("unexpected error, layout not found for parsing opts")
}
