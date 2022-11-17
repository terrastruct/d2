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

	"github.com/playwright-community/playwright-go"
	"github.com/spf13/pflag"

	"oss.terrastruct.com/d2"
	"oss.terrastruct.com/d2/d2plugin"
	"oss.terrastruct.com/d2/d2renderers/d2svg"
	"oss.terrastruct.com/d2/d2renderers/textmeasure"
	"oss.terrastruct.com/d2/d2themes"
	"oss.terrastruct.com/d2/d2themes/d2themescatalog"
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

	watchFlag := ms.FlagSet.BoolP("watch", "w", false, "watch for changes to input and live reload. Use $PORT and $HOST to specify the listening address.\n$D2_PORT and $D2_HOST are also accepted and take priority. Default is localhost:0")
	themeFlag := ms.FlagSet.Int64P("theme", "t", 0, "set the diagram theme. For a list of available options, see https://oss.terrastruct.com/d2")
	bundleFlag := ms.FlagSet.BoolP("bundle", "b", true, "when outputting SVG, bundle all assets and layers into the output file")
	versionFlag := ms.FlagSet.BoolP("version", "v", false, "get the version")
	debugFlag := ms.FlagSet.BoolP("debug", "d", false, "print debug logs")
	err = ms.FlagSet.Parse(ms.Args)

	if !errors.Is(err, pflag.ErrHelp) && err != nil {
		return xmain.UsageErrorf("failed to parse flags: %v", err)
	}

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

	if *debugFlag {
		ms.Env.Setenv("DEBUG", "1")
	}

	var inputPath string
	var outputPath string

	if len(ms.FlagSet.Args()) == 0 {
		if versionFlag != nil && *versionFlag {
			fmt.Println(version.Version)
			return nil
		}
		help(ms)
		return nil
	} else if len(ms.FlagSet.Args()) >= 3 {
		return xmain.UsageErrorf("too many arguments passed")
	}
	if len(ms.FlagSet.Args()) >= 1 {
		if ms.FlagSet.Arg(0) == "version" {
			fmt.Println(version.Version)
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

	match := d2themescatalog.Find(*themeFlag)
	if match == (d2themes.Theme{}) {
		return xmain.UsageErrorf("-t[heme] could not be found. The available options are:\n%s\nYou provided: %d", d2themescatalog.CLIString(), *themeFlag)
	}
	ms.Env.Setenv("D2_THEME", fmt.Sprintf("%d", *themeFlag))

	envD2Layout := ms.Env.Getenv("D2_LAYOUT")
	if envD2Layout == "" {
		envD2Layout = "dagre"
	}

	plugin, path, err := d2plugin.FindPlugin(ctx, envD2Layout)
	if errors.Is(err, exec.ErrNotFound) {
		return layoutNotFound(ctx, envD2Layout)
	} else if err != nil {
		return err
	}

	pluginLocation := "bundled"
	if path != "" {
		pluginLocation = fmt.Sprintf("executable plugin at %s", humanPath(path))
	}
	ms.Log.Debug.Printf("using layout plugin %s (%s)", envD2Layout, pluginLocation)

	var pw png.Playwright
	if filepath.Ext(outputPath) == ".png" {
		pw, err = png.InitPlaywright()
		if err != nil {
			return err
		}
		defer func() {
			cleanupErr := pw.Cleanup()
			if cleanupErr != nil {
				ms.Log.Error.Printf("error cleaning up playwright: %v", cleanupErr.Error())
			}
		}()
	}

	if *watchFlag {
		if inputPath == "-" {
			return xmain.UsageErrorf("-w[atch] cannot be combined with reading input from stdin")
		}
		ms.Env.Setenv("LOG_TIMESTAMPS", "1")
		w, err := newWatcher(ctx, ms, plugin, inputPath, outputPath, pw)
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

	_, err = compile(ctx, ms, plugin, inputPath, outputPath, pw.Page)
	if err != nil {
		return err
	}

	ms.Log.Success.Printf("successfully compiled %v to %v", inputPath, outputPath)
	return nil
}

func compile(ctx context.Context, ms *xmain.State, plugin d2plugin.Plugin, inputPath, outputPath string, page playwright.Page) ([]byte, error) {
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
	outputImage, err := plugin.PostProcess(ctx, svg)
	if err != nil {
		return nil, err
	}

	if filepath.Ext(outputPath) == ".png" {
		outputImage, err = png.ExportPNG(ms, page, outputImage)
		if err != nil {
			return nil, err
		}
	}

	err = ms.WritePath(outputPath, outputImage)
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
