package d2cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/pflag"
	"go.uber.org/multierr"

	"github.com/d2lang/util-go/go2"
	"github.com/d2lang/util-go/xmain"

	"github.com/d2lang/d2/d2ast"
	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/d2lib"
	"github.com/d2lang/d2/d2parser"
	"github.com/d2lang/d2/d2plugin"
	"github.com/d2lang/d2/d2renderers/d2animate"
	"github.com/d2lang/d2/d2renderers/d2ascii"
	"github.com/d2lang/d2/d2renderers/d2ascii/charset"
	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/d2renderers/d2svg"
	"github.com/d2lang/d2/d2renderers/d2svg/appendix"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/d2themes"
	"github.com/d2lang/d2/d2themes/d2themescatalog"
	"github.com/d2lang/d2/lib/background"
	"github.com/d2lang/d2/lib/imgbundler"
	"github.com/d2lang/d2/lib/log"
	"github.com/d2lang/d2/lib/pdf"
	"github.com/d2lang/d2/lib/pptx"
	"github.com/d2lang/d2/lib/simplelog"
	"github.com/d2lang/d2/lib/textmeasure"
	timelib "github.com/d2lang/d2/lib/time"
	"github.com/d2lang/d2/lib/version"
)

func Run(ctx context.Context, ms *xmain.State) (err error) {
	ctx = log.WithDefault(ctx)
	// These should be kept up-to-date with the d2 man page
	watchFlag, err := ms.Opts.Bool("D2_WATCH", "watch", "w", false, "watch for changes to input and live reload. Use $HOST and $PORT to specify the listening address.\n(default localhost:0, which will open on a randomly available local port).")
	if err != nil {
		return err
	}
	hostFlag := ms.Opts.String("HOST", "host", "h", "localhost", "host listening address when used with watch")
	portFlag := ms.Opts.String("PORT", "port", "p", "0", "port listening address when used with watch")
	bundleFlag, err := ms.Opts.Bool("D2_BUNDLE", "bundle", "b", true, "when outputting SVG, bundle all assets and layers into the output file")
	if err != nil {
		return err
	}
	forceAppendixFlag, err := ms.Opts.Bool("D2_FORCE_APPENDIX", "force-appendix", "", false, "an appendix for tooltips and links is added to PNG exports since they are not interactive. --force-appendix adds an appendix to SVG exports as well")
	if err != nil {
		return err
	}
	debugFlag, err := ms.Opts.Bool("DEBUG", "debug", "d", false, "print debug logs.")
	if err != nil {
		ms.Log.Warn.Printf("Invalid DEBUG flag value ignored")
		debugFlag = go2.Pointer(false)
	}
	imgCacheFlag, err := ms.Opts.Bool("IMG_CACHE", "img-cache", "", true, "in watch mode, images used in icons are cached for subsequent compilations. This should be disabled if images might change.")
	if err != nil {
		return err
	}
	layoutFlag := ms.Opts.String("D2_LAYOUT", "layout", "l", "dagre", `the layout engine used`)
	themeFlag, err := ms.Opts.Int64("D2_THEME", "theme", "t", 0, "the diagram theme ID")
	if err != nil {
		return err
	}
	darkThemeFlag, err := ms.Opts.Int64("D2_DARK_THEME", "dark-theme", "", -1, "the theme to use when the viewer's browser is in dark mode. When left unset -theme is used for both light and dark mode. Be aware that explicit styles set in D2 code will still be applied and this may produce unexpected results. We plan on resolving this by making style maps in D2 light/dark mode specific. See https://github.com/d2lang/d2/issues/831.")
	if err != nil {
		return err
	}
	padFlag, err := ms.Opts.Int64("D2_PAD", "pad", "", d2svg.DEFAULT_PADDING, "pixels padded around the rendered diagram")
	if err != nil {
		return err
	}
	animateIntervalFlag, err := ms.Opts.Int64("D2_ANIMATE_INTERVAL", "animate-interval", "", 0, "if given, multiple boards are packaged as 1 SVG which transitions through each board at the interval (in milliseconds). Can only be used with SVG or GIF exports. For GIF exports, defaults to 1000ms if not specified.")
	if err != nil {
		return err
	}
	timeoutFlag, err := ms.Opts.Int64("D2_TIMEOUT", "timeout", "", 120, "the maximum number of seconds that D2 runs for before timing out and exiting. When rendering a large diagram, it is recommended to increase this value")
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
	stdoutFormatFlag := ms.Opts.String("D2_STDOUT_FORMAT", "stdout-format", "", "", "output format when writing to stdout (svg, png, ascii, txt, pdf, pptx, gif). Usage: d2 input.d2 --stdout-format png - > output.png")
	if err != nil {
		return err
	}

	browserFlag := ms.Opts.String("BROWSER", "browser", "", "", "browser executable that watch opens. Setting to 0 opens no browser.")
	centerFlag, err := ms.Opts.Bool("D2_CENTER", "center", "c", false, "center the SVG in the containing viewbox, such as your browser screen")
	if err != nil {
		return err
	}
	scaleFlag, err := ms.Opts.Float64("SCALE", "scale", "", -1, "scale the output. E.g., 0.5 to halve the default size. Default -1 means that SVG's will fit to screen and all others will use their default render size. Setting to 1 turns off SVG fitting to screen.")
	if err != nil {
		return err
	}
	targetFlag := ms.Opts.String("", "target", "", "*", "target board to render. Pass an empty string to target root board. If target ends with '*', it will be rendered with all of its scenarios, steps, and layers. Otherwise, only the target board will be rendered. E.g. --target='' to render root board only or --target='layers.x.*' to render layer 'x' with all of its children.")

	fontRegularFlag := ms.Opts.String("D2_FONT_REGULAR", "font-regular", "", "", "path to .ttf file to use for the regular font. If none provided, Source Sans Pro Regular is used.")
	fontItalicFlag := ms.Opts.String("D2_FONT_ITALIC", "font-italic", "", "", "path to .ttf file to use for the italic font. If none provided, Source Sans Pro Regular-Italic is used.")
	fontBoldFlag := ms.Opts.String("D2_FONT_BOLD", "font-bold", "", "", "path to .ttf file to use for the bold font. If none provided, Source Sans Pro Bold is used.")
	fontSemiboldFlag := ms.Opts.String("D2_FONT_SEMIBOLD", "font-semibold", "", "", "path to .ttf file to use for the semibold font. If none provided, Source Sans Pro Semibold is used.")
	fontMonoFlag := ms.Opts.String("D2_FONT_MONO", "font-mono", "", "", "path to .ttf file to use for the monospace font. If none provided, Source Code Pro Regular is used.")
	fontMonoBoldFlag := ms.Opts.String("D2_FONT_MONO_BOLD", "font-mono-bold", "", "", "path to .ttf file to use for the monospace bold font. If none provided, Source Code Pro Bold is used.")
	fontMonoItalicFlag := ms.Opts.String("D2_FONT_MONO_ITALIC", "font-mono-italic", "", "", "path to .ttf file to use for the monospace italic font. If none provided, Source Code Pro Italic is used.")
	fontMonoSemiboldFlag := ms.Opts.String("D2_FONT_MONO_SEMIBOLD", "font-mono-semibold", "", "", "path to .ttf file to use for the monospace semibold font. If none provided, Source Code Pro Semibold is used.")

	checkFlag, err := ms.Opts.Bool("D2_CHECK", "check", "", false, "check that the specified files are formatted correctly.")
	if err != nil {
		return err
	}

	noXMLTagFlag, err := ms.Opts.Bool("D2_NO_XML_TAG", "no-xml-tag", "", false, "omit XML tag (<?xml ...?>) from output SVG files. Useful when generating SVGs for direct HTML embedding")
	if err != nil {
		return err
	}

	saltFlag := ms.Opts.String("", "salt", "", "", "Add a salt value to ensure the output uses unique IDs. This is useful when generating multiple identical diagrams to be included in the same HTML doc, so that duplicate IDs do not cause invalid HTML. The salt value is a string that will be appended to IDs in the output.")

	omitVersionFlag, err := ms.Opts.Bool("OMIT_VERSION", "omit-version", "", false, "omit D2 version from generated image")
	if err != nil {
		return err
	}

	asciiModeFlag := ms.Opts.String("D2_ASCII_MODE", "ascii-mode", "", "extended", "ASCII rendering mode for text outputs. Options: 'standard' (basic ASCII chars) or 'extended' (Unicode chars)")
	if err != nil {
		return err
	}

	plugins, err := d2plugin.ListPlugins(ctx)
	if err != nil {
		return err
	}
	err = populateLayoutOpts(ctx, ms, plugins)
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

	fontFamily, monoFontFamily, err := loadFonts(ms, *fontRegularFlag, *fontItalicFlag, *fontBoldFlag, *fontSemiboldFlag, *fontMonoFlag, *fontMonoBoldFlag, *fontMonoItalicFlag, *fontMonoSemiboldFlag)
	if err != nil {
		return xmain.UsageErrorf("failed to load specified fonts: %v", err)
	}

	if len(ms.Opts.Flags.Args()) > 0 {
		switch ms.Opts.Flags.Arg(0) {
		case "layout":
			return layoutCmd(ctx, ms, plugins)
		case "themes":
			themesCmd(ctx, ms)
			return nil
		case "fmt":
			return fmtCmd(ctx, ms, *checkFlag)
		case "play":
			return playCmd(ctx, ms)
		case "validate":
			return validateCmd(ctx, ms)
		case "version":
			if len(ms.Opts.Flags.Args()) > 1 {
				return xmain.UsageErrorf("version subcommand accepts no arguments")
			}
			fmt.Println(version.Version)
			return nil
		}
	}

	if *debugFlag {
		ctx = log.Leveled(ctx, slog.LevelDebug)
		ms.Env.Setenv("DEBUG", "1")
	}
	if *imgCacheFlag {
		ms.Env.Setenv("IMG_CACHE", "1")
	}
	if *browserFlag != "" {
		ms.Env.Setenv("BROWSER", *browserFlag)
	}
	if timeoutFlag != nil {
		os.Setenv("D2_TIMEOUT", fmt.Sprintf("%d", *timeoutFlag))
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
	if inputPath != "-" {
		inputPath = ms.AbsPath(inputPath)
		d, err := os.Stat(inputPath)
		if err == nil && d.IsDir() {
			inputPath = filepath.Join(inputPath, "index.d2")
		}
	}
	if filepath.Ext(outputPath) == ".ppt" {
		return xmain.UsageErrorf("D2 does not support ppt exports, did you mean \"pptx\"?")
	}

	outputFormat, err := getOutputFormat(stdoutFormatFlag, outputPath)
	if err != nil {
		return xmain.UsageErrorf("%v", err)
	}
	if outputPath != "-" {
		outputPath = ms.AbsPath(outputPath)
		if *animateIntervalFlag > 0 && !outputFormat.supportsAnimation() {
			return xmain.UsageErrorf("--animate-interval can only be used when exporting to SVG or GIF.\nYou provided: %s", filepath.Ext(outputPath))
		}
	}

	match := d2themescatalog.Find(*themeFlag)
	if match == (d2themes.Theme{}) {
		return xmain.UsageErrorf("-t[heme] could not be found. The available options are:\n%s\nYou provided: %d", d2themescatalog.CLIString(), *themeFlag)
	}
	ms.Log.Debug.Printf("using theme %s (ID: %d)", match.Name, *themeFlag)

	// If flag is not explicitly set by user, set to nil.
	// Later, configs from D2 code will only overwrite if they weren't explicitly set by user
	flagSet := make(map[string]struct{})
	ms.Opts.Flags.Visit(func(f *pflag.Flag) {
		flagSet[f.Name] = struct{}{}
	})
	if ms.Env.Getenv("D2_LAYOUT") == "" {
		if _, ok := flagSet["layout"]; !ok {
			layoutFlag = nil
		}
	}
	if ms.Env.Getenv("D2_THEME") == "" {
		if _, ok := flagSet["theme"]; !ok {
			themeFlag = nil
		}
	}
	if ms.Env.Getenv("D2_SKETCH") == "" {
		if _, ok := flagSet["sketch"]; !ok {
			sketchFlag = nil
		}
	}
	if ms.Env.Getenv("D2_PAD") == "" {
		if _, ok := flagSet["pad"]; !ok {
			padFlag = nil
		}
	}
	if ms.Env.Getenv("D2_CENTER") == "" {
		if _, ok := flagSet["center"]; !ok {
			centerFlag = nil
		}
	}

	if *darkThemeFlag == -1 {
		darkThemeFlag = nil // TODO this is a temporary solution: https://github.com/d2lang/util-go/issues/7
	}
	if darkThemeFlag != nil {
		match = d2themescatalog.Find(*darkThemeFlag)
		if match == (d2themes.Theme{}) {
			return xmain.UsageErrorf("--dark-theme could not be found. The available options are:\n%s\nYou provided: %d", d2themescatalog.CLIString(), *darkThemeFlag)
		}
		ms.Log.Debug.Printf("using dark theme %s (ID: %d)", match.Name, *darkThemeFlag)
	}
	var scale *float64
	if scaleFlag != nil && *scaleFlag > 0. {
		scale = scaleFlag
	}

	if !outputFormat.supportsDarkTheme() {
		if darkThemeFlag != nil {
			ms.Log.Warn.Printf("--dark-theme cannot be used while exporting to another format other than .svg")
			darkThemeFlag = nil
		}
	}
	renderOpts := d2svg.RenderOpts{
		Pad:         padFlag,
		Sketch:      sketchFlag,
		Center:      centerFlag,
		ThemeID:     themeFlag,
		DarkThemeID: darkThemeFlag,
		Scale:       scale,
		NoXMLTag:    noXMLTagFlag,
		Salt:        saltFlag,
		OmitVersion: omitVersionFlag,
	}

	if *watchFlag {
		if inputPath == "-" {
			return xmain.UsageErrorf("-w[atch] cannot be combined with reading input from stdin")
		}
		if *targetFlag != "*" {
			return xmain.UsageErrorf("-w[atch] cannot be combined with --target")
		}
		animateInterval := *animateIntervalFlag
		if outputFormat == GIF && animateInterval == 0 {
			animateInterval = 1000
			ms.Log.Debug.Printf("GIF export: animate-interval not specified, defaulting to 1000ms")
		}
		w, err := newWatcher(ctx, ms, watcherOpts{
			plugins:         plugins,
			layout:          layoutFlag,
			renderOpts:      renderOpts,
			animateInterval: animateInterval,
			host:            *hostFlag,
			port:            *portFlag,
			inputPath:       inputPath,
			outputPath:      outputPath,
			bundle:          *bundleFlag,
			forceAppendix:   *forceAppendixFlag,
			fontFamily:      fontFamily,
			monoFontFamily:  monoFontFamily,
			outputFormat:    outputFormat,
			asciiMode:       *asciiModeFlag,
		})
		if err != nil {
			return err
		}
		return w.run()
	}

	var boardPath []string
	var noChildren bool
	switch *targetFlag {
	case "*":
	case "":
		noChildren = true
	default:
		target := *targetFlag
		if strings.HasSuffix(target, ".*") {
			target = target[:len(target)-2]
		} else {
			noChildren = true
		}
		key, err := d2parser.ParseKey(target)
		if err != nil {
			return xmain.UsageErrorf("invalid target: %s", *targetFlag)
		}
		boardPath = key.StringIDA()
	}

	ctx, cancel := timelib.WithTimeout(ctx, time.Minute*2)
	defer cancel()

	animateInterval := *animateIntervalFlag
	if outputFormat == GIF && animateInterval == 0 {
		animateInterval = 1000
		ms.Log.Debug.Printf("GIF export: animate-interval not specified, defaulting to 1000ms")
	}

	_, written, err := compile(ctx, ms, plugins, nil, layoutFlag, renderOpts, fontFamily, monoFontFamily, animateInterval, inputPath, outputPath, boardPath, noChildren, *bundleFlag, *forceAppendixFlag, outputFormat, *asciiModeFlag, false)
	if err != nil {
		if written {
			return fmt.Errorf("failed to fully compile (partial render written) %s: %w", ms.HumanPath(inputPath), err)
		}
		return fmt.Errorf("failed to compile %s: %w", ms.HumanPath(inputPath), err)
	}
	return nil
}

func LayoutResolver(ctx context.Context, ms *xmain.State, plugins []d2plugin.Plugin) func(engine string) (d2graph.LayoutGraph, error) {
	cached := make(map[string]d2graph.LayoutGraph)
	return func(engine string) (d2graph.LayoutGraph, error) {
		if c, ok := cached[engine]; ok {
			return c, nil
		}

		plugin, err := d2plugin.FindPlugin(ctx, plugins, engine)
		if err != nil {
			if errors.Is(err, exec.ErrNotFound) {
				return nil, layoutNotFound(ctx, plugins, engine)
			}
			return nil, err
		}

		err = d2plugin.HydratePluginOpts(ctx, ms, plugin)
		if err != nil {
			return nil, err
		}

		cached[engine] = plugin.Layout
		return plugin.Layout, nil
	}
}

func RouterResolver(ctx context.Context, ms *xmain.State, plugins []d2plugin.Plugin) func(engine string) (d2graph.RouteEdges, error) {
	cached := make(map[string]d2graph.RouteEdges)
	return func(engine string) (d2graph.RouteEdges, error) {
		if c, ok := cached[engine]; ok {
			return c, nil
		}

		plugin, err := d2plugin.FindPlugin(ctx, plugins, engine)
		if err != nil {
			if errors.Is(err, exec.ErrNotFound) {
				return nil, layoutNotFound(ctx, plugins, engine)
			}
			return nil, err
		}

		pluginInfo, err := plugin.Info(ctx)
		if err != nil {
			return nil, err
		}
		hasRouter := false
		for _, feat := range pluginInfo.Features {
			if feat == d2plugin.ROUTES_EDGES {
				hasRouter = true
				break
			}
		}
		if !hasRouter {
			return nil, nil
		}
		routingPlugin, ok := plugin.(d2plugin.RoutingPlugin)
		if !ok {
			return nil, fmt.Errorf("plugin has routing feature but does not implement RoutingPlugin")
		}

		routeEdges := d2graph.RouteEdges(routingPlugin.RouteEdges)
		cached[engine] = routeEdges
		return routeEdges, nil
	}
}

func compile(ctx context.Context, ms *xmain.State, plugins []d2plugin.Plugin, fs fs.FS, layout *string, renderOpts d2svg.RenderOpts, fontFamily *d2fonts.FontFamily, monoFontFamily *d2fonts.FontFamily, animateInterval int64, inputPath, outputPath string, boardPath []string, noChildren, bundle, forceAppendix bool, ext exportExtension, asciiMode string, wantPreview bool) (_ []byte, written bool, _ error) {
	// Use ELK layout for ascii outputs when layout is dagre or unspecified
	if ext == TXT {
		if layout == nil || *layout == "dagre" {
			if ms.Log.Debug != nil {
				prevLayout := "unspecified"
				if layout != nil {
					prevLayout = *layout
				}
				ms.Log.Debug.Printf("switching layout engine to ELK for ASCII format (was %s)", prevLayout)
			}
			layout = go2.Pointer("elk")
		}
	}

	start := time.Now()
	input, err := ms.ReadPath(inputPath)
	if err != nil {
		return nil, false, err
	}

	ruler, err := textmeasure.NewRuler()
	if err != nil {
		return nil, false, err
	}

	opts := &d2lib.CompileOptions{
		Ruler:          ruler,
		FontFamily:     fontFamily,
		MonoFontFamily: monoFontFamily,
		InputPath:      inputPath,
		LayoutResolver: LayoutResolver(ctx, ms, plugins),
		Layout:         layout,
		RouterResolver: RouterResolver(ctx, ms, plugins),
		FS:             fs,
		LayoutReuse:    true,
	}

	if os.Getenv("D2_LSP_MODE") == "1" {
		// only the parse result is needed if running d2 for lsp,
		// if this, "fails", the AST is still valid and can be sent
		// to vscode extension
		ast, err := d2lib.Parse(ctx, string(input), opts)

		type LspOutputData struct {
			Ast *d2ast.Map
			Err error
		}
		jsonOutput, err := json.Marshal(LspOutputData{Ast: ast, Err: err})
		if err != nil {
			return nil, false, err
		}
		fmt.Print(string(jsonOutput))
		os.Exit(42)
		return nil, false, nil
	}

	cancel := background.Repeat(func() {
		ms.Log.Info.Printf("compiling & running layout algorithms...")
	}, time.Second*5)
	defer cancel()

	rootDiagram, g, err := d2lib.Compile(ctx, string(input), opts, &renderOpts)
	if err != nil {
		return nil, false, err
	}
	cancel()

	diagram := rootDiagram.GetBoard(boardPath)
	if diagram == nil {
		return nil, false, fmt.Errorf(`render target "%s" not found`, strings.Join(boardPath, "."))
	}
	if noChildren {
		diagram.Layers = nil
		diagram.Scenarios = nil
		diagram.Steps = nil
	}

	plugin, _ := d2plugin.FindPlugin(ctx, plugins, *opts.Layout)

	if animateInterval > 0 {
		masterID, err := diagram.HashID(renderOpts.Salt)
		if err != nil {
			return nil, false, err
		}
		renderOpts.MasterID = masterID
	}

	pinfo, err := plugin.Info(ctx)
	if err != nil {
		return nil, false, err
	}
	plocation := pinfo.Type
	if pinfo.Type == "binary" {
		plocation = fmt.Sprintf("executable plugin at %s", humanPath(pinfo.Path))
	}
	ms.Log.Debug.Printf("using layout plugin %s (%s)", *opts.Layout, plocation)

	pluginInfo, err := plugin.Info(ctx)
	if err != nil {
		return nil, false, err
	}

	err = d2plugin.FeatureSupportCheck(pluginInfo, g)
	if err != nil {
		return nil, false, err
	}

	switch ext {
	case GIF:
		cacheImages := ms.Env.Getenv("IMG_CACHE") == "1"
		out, previewSVG, err := renderGIF(ctx, plugin, inputPath, cacheImages, diagram, renderOpts, int(animateInterval), wantPreview)
		if err != nil {
			return nil, false, err
		}
		if wantPreview {
			previewSVG, err = appendRasterPreview(diagram, renderOpts, ruler, previewSVG)
			if err != nil {
				return nil, false, err
			}
		}
		if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
			return nil, false, err
		}
		outputWritten, err := runStatusFinalizer(ctx, func() (bool, error) {
			return writeWithStatus(ms, outputPath, out)
		})
		if err != nil {
			return nil, outputWritten, err
		}
		dur := time.Since(start)
		ms.Log.Success.Printf("successfully compiled %s to %s in %s", ms.HumanPath(inputPath), ms.HumanPath(outputPath), dur)
		return previewSVG, true, nil
	case PDF:
		path := []pdf.BoardTitle{
			{Name: diagram.Root.Label, BoardID: "root"},
		}
		if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
			return nil, false, err
		}
		cacheImages := ms.Env.Getenv("IMG_CACHE") == "1"
		var preview []byte
		var outputWritten bool
		if outputPath == "-" {
			var output bytes.Buffer
			preview, err = renderPDFTo(ctx, plugin, renderOpts, inputPath, &output, cacheImages, ruler, diagram, path, diagram.Root.Label != "", wantPreview)
			if err == nil {
				outputWritten, err = runStatusFinalizer(ctx, func() (bool, error) {
					return writeStdout(ms.Stdout, output.Bytes())
				})
			}
		} else {
			preview, outputWritten, err = renderPDFWithStatus(ctx, plugin, renderOpts, inputPath, outputPath, cacheImages, ruler, diagram, path, diagram.Root.Label != "", wantPreview)
		}
		if err != nil {
			return preview, outputWritten, err
		}
		dur := time.Since(start)
		ms.Log.Success.Printf("successfully compiled %s to %s in %s", ms.HumanPath(inputPath), ms.HumanPath(outputPath), dur)
		return preview, true, nil
	case PPTX:
		var username string
		if user, err := user.Current(); err == nil {
			username = user.Username
		}
		description := "Presentation generated with D2 - https://d2lang.com"
		rootName := getFileName(outputPath)
		if outputPath == "-" {
			rootName = getFileName(inputPath)
			if inputPath == "-" {
				rootName = "stdin"
			}
		}
		// version must be only numbers to avoid issues with PowerPoint
		p := pptx.NewPresentation(rootName, description, rootName, username, version.OnlyNumbers(), diagram.Root.Label != "")

		path := []pptx.BoardTitle{
			{Name: "root", BoardID: "root"},
		}
		if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
			return nil, false, err
		}
		cacheImages := ms.Env.Getenv("IMG_CACHE") == "1"
		preview, err := renderPPTX(ctx, p, plugin, renderOpts, inputPath, cacheImages, ruler, diagram, path, wantPreview)
		if err != nil {
			return preview, false, err
		}
		if outputPath == "-" {
			var output bytes.Buffer
			if err := runFinalizer(ctx, func() error { return p.ExportTo(&output) }); err != nil {
				return preview, false, err
			}
			outputWritten, err := runStatusFinalizer(ctx, func() (bool, error) {
				return writeStdout(ms.Stdout, output.Bytes())
			})
			if err != nil {
				return preview, outputWritten, err
			}
		} else {
			outputWritten, err := runStatusFinalizer(ctx, func() (bool, error) {
				return p.SaveToWithStatus(outputPath)
			})
			if err != nil {
				return preview, outputWritten, err
			}
		}
		dur := time.Since(start)
		ms.Log.Success.Printf("successfully compiled %s to %s in %s", ms.HumanPath(inputPath), ms.HumanPath(outputPath), dur)
		return preview, true, nil
	default:
		compileDur := time.Since(start)
		if animateInterval <= 0 {
			// Rename all the "root.layers.x" to the paths that the boards get output to
			linkToOutput, err := resolveLinks("root", outputPath, rootDiagram)
			if err != nil {
				return nil, false, err
			}
			err = relink("root", rootDiagram, linkToOutput)
			if err != nil {
				return nil, false, err
			}
		}

		var boards [][]byte
		var outputWritten bool
		var err error
		if noChildren {
			boards, outputWritten, err = renderSingle(ctx, ms, compileDur, plugin, renderOpts, inputPath, outputPath, bundle, forceAppendix, ruler, diagram, ext, asciiMode, wantPreview)
		} else {
			boards, outputWritten, err = render(ctx, ms, compileDur, plugin, renderOpts, inputPath, outputPath, bundle, forceAppendix, ruler, diagram, ext, asciiMode, wantPreview)
		}
		if err != nil {
			return nil, outputWritten, err
		}
		var out []byte
		if len(boards) > 0 {
			out = boards[0]
			if animateInterval > 0 {
				out, err = d2animate.Wrap(diagram, boards, renderOpts, int(animateInterval))
				if err != nil {
					return nil, false, err
				}
				out, err = postProcess(ctx, plugin, out)
				if err != nil {
					return nil, false, err
				}
				err = os.MkdirAll(filepath.Dir(outputPath), 0755)
				if err != nil {
					return nil, false, err
				}
				var finalWritten bool
				finalWritten, err = runStatusFinalizer(ctx, func() (bool, error) {
					return writeWithStatus(ms, outputPath, out)
				})
				outputWritten = outputWritten || finalWritten
				if err != nil {
					return nil, outputWritten, err
				}
				ms.Log.Success.Printf("successfully compiled %s to %s in %s", ms.HumanPath(inputPath), ms.HumanPath(outputPath), time.Since(start))
			}
		}
		return out, true, nil
	}
}

func resolveLinks(currDiagramPath, outputPath string, diagram *d2target.Diagram) (linkToOutput map[string]string, err error) {
	if diagram.Name != "" {
		ext := filepath.Ext(outputPath)
		outputPath = strings.TrimSuffix(outputPath, ext)
		outputPath = filepath.Join(outputPath, diagram.Name)
		outputPath += ext
	}

	boardOutputPath := outputPath
	if len(diagram.Layers) > 0 || len(diagram.Scenarios) > 0 || len(diagram.Steps) > 0 {
		ext := filepath.Ext(boardOutputPath)
		boardOutputPath = strings.TrimSuffix(boardOutputPath, ext)
		boardOutputPath = filepath.Join(boardOutputPath, "index")
		boardOutputPath += ext
	}

	layersOutputPath := outputPath
	if len(diagram.Scenarios) > 0 || len(diagram.Steps) > 0 {
		ext := filepath.Ext(layersOutputPath)
		layersOutputPath = strings.TrimSuffix(layersOutputPath, ext)
		layersOutputPath = filepath.Join(layersOutputPath, "layers")
		layersOutputPath += ext
	}
	scenariosOutputPath := outputPath
	if len(diagram.Layers) > 0 || len(diagram.Steps) > 0 {
		ext := filepath.Ext(scenariosOutputPath)
		scenariosOutputPath = strings.TrimSuffix(scenariosOutputPath, ext)
		scenariosOutputPath = filepath.Join(scenariosOutputPath, "scenarios")
		scenariosOutputPath += ext
	}
	stepsOutputPath := outputPath
	if len(diagram.Layers) > 0 || len(diagram.Scenarios) > 0 {
		ext := filepath.Ext(stepsOutputPath)
		stepsOutputPath = strings.TrimSuffix(stepsOutputPath, ext)
		stepsOutputPath = filepath.Join(stepsOutputPath, "steps")
		stepsOutputPath += ext
	}

	linkToOutput = map[string]string{currDiagramPath: boardOutputPath}

	for _, dl := range diagram.Layers {
		m, err := resolveLinks(strings.Join([]string{currDiagramPath, "layers", dl.Name}, "."), layersOutputPath, dl)
		if err != nil {
			return nil, err
		}
		for k, v := range m {
			linkToOutput[k] = v
		}
	}
	for _, dl := range diagram.Scenarios {
		m, err := resolveLinks(strings.Join([]string{currDiagramPath, "scenarios", dl.Name}, "."), scenariosOutputPath, dl)
		if err != nil {
			return nil, err
		}
		for k, v := range m {
			linkToOutput[k] = v
		}
	}
	for _, dl := range diagram.Steps {
		m, err := resolveLinks(strings.Join([]string{currDiagramPath, "steps", dl.Name}, "."), stepsOutputPath, dl)
		if err != nil {
			return nil, err
		}
		for k, v := range m {
			linkToOutput[k] = v
		}
	}

	return linkToOutput, nil
}

func relink(currDiagramPath string, d *d2target.Diagram, linkToOutput map[string]string) error {
	for i, shape := range d.Shapes {
		if shape.Link != "" {
			for k, v := range linkToOutput {
				if shape.Link == k {
					rel, err := filepath.Rel(filepath.Dir(linkToOutput[currDiagramPath]), v)
					if err != nil {
						return err
					}
					d.Shapes[i].Link = rel
					break
				}
			}
		}
	}
	for _, board := range d.Layers {
		err := relink(strings.Join([]string{currDiagramPath, "layers", board.Name}, "."), board, linkToOutput)
		if err != nil {
			return err
		}
	}
	for _, board := range d.Scenarios {
		err := relink(strings.Join([]string{currDiagramPath, "scenarios", board.Name}, "."), board, linkToOutput)
		if err != nil {
			return err
		}
	}
	for _, board := range d.Steps {
		err := relink(strings.Join([]string{currDiagramPath, "steps", board.Name}, "."), board, linkToOutput)
		if err != nil {
			return err
		}
	}
	return nil
}

func postProcess(ctx context.Context, plugin d2plugin.Plugin, in []byte) ([]byte, error) {
	postProcessor, ok := plugin.(d2plugin.PostProcessor)
	if !ok {
		return in, nil
	}
	return postProcessor.PostProcess(ctx, in)
}

func render(ctx context.Context, ms *xmain.State, compileDur time.Duration, plugin d2plugin.Plugin, opts d2svg.RenderOpts, inputPath, outputPath string, bundle, forceAppendix bool, ruler *textmeasure.Ruler, diagram *d2target.Diagram, ext exportExtension, asciiMode string, wantPreview bool) (_ [][]byte, written bool, _ error) {
	if ext == PNG {
		var encoder rasterPNGEncoder
		defer encoder.close()
		return renderWithPNGEncoder(ctx, ms, compileDur, plugin, opts, inputPath, outputPath, bundle, forceAppendix, ruler, diagram, ext, asciiMode, wantPreview, &encoder)
	}
	return renderWithPNGEncoder(ctx, ms, compileDur, plugin, opts, inputPath, outputPath, bundle, forceAppendix, ruler, diagram, ext, asciiMode, wantPreview, nil)
}

func renderWithPNGEncoder(ctx context.Context, ms *xmain.State, compileDur time.Duration, plugin d2plugin.Plugin, opts d2svg.RenderOpts, inputPath, outputPath string, bundle, forceAppendix bool, ruler *textmeasure.Ruler, diagram *d2target.Diagram, ext exportExtension, asciiMode string, wantPreview bool, pngEncoder *rasterPNGEncoder) (_ [][]byte, written bool, _ error) {
	if diagram.Name != "" {
		ext := filepath.Ext(outputPath)
		outputPath = strings.TrimSuffix(outputPath, ext)
		outputPath = filepath.Join(outputPath, diagram.Name)
		outputPath += ext
	}

	boardOutputPath := outputPath
	if len(diagram.Layers) > 0 || len(diagram.Scenarios) > 0 || len(diagram.Steps) > 0 {
		if outputPath == "-" {
			// TODO it can if composed into one
			return nil, false, fmt.Errorf("multiboard output cannot be written to stdout")
		}
		// Boards with subboards must be self-contained folders.
		ext := filepath.Ext(boardOutputPath)
		boardOutputPath = strings.TrimSuffix(boardOutputPath, ext)
		os.RemoveAll(boardOutputPath)
		boardOutputPath = filepath.Join(boardOutputPath, "index")
		boardOutputPath += ext
	}

	layersOutputPath := outputPath
	if len(diagram.Scenarios) > 0 || len(diagram.Steps) > 0 {
		ext := filepath.Ext(layersOutputPath)
		layersOutputPath = strings.TrimSuffix(layersOutputPath, ext)
		layersOutputPath = filepath.Join(layersOutputPath, "layers")
		layersOutputPath += ext
	}
	scenariosOutputPath := outputPath
	if len(diagram.Layers) > 0 || len(diagram.Steps) > 0 {
		ext := filepath.Ext(scenariosOutputPath)
		scenariosOutputPath = strings.TrimSuffix(scenariosOutputPath, ext)
		scenariosOutputPath = filepath.Join(scenariosOutputPath, "scenarios")
		scenariosOutputPath += ext
	}
	stepsOutputPath := outputPath
	if len(diagram.Layers) > 0 || len(diagram.Scenarios) > 0 {
		ext := filepath.Ext(stepsOutputPath)
		stepsOutputPath = strings.TrimSuffix(stepsOutputPath, ext)
		stepsOutputPath = filepath.Join(stepsOutputPath, "steps")
		stepsOutputPath += ext
	}

	var boards [][]byte
	for _, dl := range diagram.Layers {
		childPreview := wantPreview && diagram.IsFolderOnly && len(boards) == 0
		childrenBoards, childWritten, err := renderWithPNGEncoder(ctx, ms, compileDur, plugin, opts, inputPath, layersOutputPath, bundle, forceAppendix, ruler, dl, ext, asciiMode, childPreview, pngEncoder)
		written = written || childWritten
		if err != nil {
			return boards, written, err
		}
		boards = append(boards, childrenBoards...)
	}
	for _, dl := range diagram.Scenarios {
		childPreview := wantPreview && diagram.IsFolderOnly && len(boards) == 0
		childrenBoards, childWritten, err := renderWithPNGEncoder(ctx, ms, compileDur, plugin, opts, inputPath, scenariosOutputPath, bundle, forceAppendix, ruler, dl, ext, asciiMode, childPreview, pngEncoder)
		written = written || childWritten
		if err != nil {
			return boards, written, err
		}
		boards = append(boards, childrenBoards...)
	}
	for _, dl := range diagram.Steps {
		childPreview := wantPreview && diagram.IsFolderOnly && len(boards) == 0
		childrenBoards, childWritten, err := renderWithPNGEncoder(ctx, ms, compileDur, plugin, opts, inputPath, stepsOutputPath, bundle, forceAppendix, ruler, dl, ext, asciiMode, childPreview, pngEncoder)
		written = written || childWritten
		if err != nil {
			return boards, written, err
		}
		boards = append(boards, childrenBoards...)
	}

	if !diagram.IsFolderOnly {
		start := time.Now()
		out, boardWritten, err := _renderWithPNGEncoder(ctx, ms, plugin, opts, inputPath, boardOutputPath, bundle, forceAppendix, ruler, diagram, ext, asciiMode, wantPreview, pngEncoder)
		written = written || boardWritten
		if err != nil {
			return boards, written, err
		}
		dur := compileDur + time.Since(start)
		if opts.MasterID == "" {
			ms.Log.Success.Printf("successfully compiled %s to %s in %s", ms.HumanPath(inputPath), ms.HumanPath(boardOutputPath), dur)
		}
		boards = append([][]byte{out}, boards...)
	}

	return boards, written, nil
}

func renderSingle(ctx context.Context, ms *xmain.State, compileDur time.Duration, plugin d2plugin.Plugin, opts d2svg.RenderOpts, inputPath, outputPath string, bundle, forceAppendix bool, ruler *textmeasure.Ruler, diagram *d2target.Diagram, outputFormat exportExtension, asciiMode string, wantPreview bool) ([][]byte, bool, error) {
	start := time.Now()
	out, written, err := _renderWithPNGEncoder(ctx, ms, plugin, opts, inputPath, outputPath, bundle, forceAppendix, ruler, diagram, outputFormat, asciiMode, wantPreview, nil)
	if err != nil {
		return [][]byte{}, written, err
	}
	dur := compileDur + time.Since(start)
	if opts.MasterID == "" {
		ms.Log.Success.Printf("successfully compiled %s to %s in %s", ms.HumanPath(inputPath), ms.HumanPath(outputPath), dur)
	}
	return [][]byte{out}, written, nil
}

func _renderWithPNGEncoder(ctx context.Context, ms *xmain.State, plugin d2plugin.Plugin, opts d2svg.RenderOpts, inputPath, outputPath string, bundle, forceAppendix bool, ruler *textmeasure.Ruler, diagram *d2target.Diagram, outputFormat exportExtension, asciiMode string, wantPreview bool, pngEncoder *rasterPNGEncoder) ([]byte, bool, error) {
	if outputFormat == TXT {
		var charsetType charset.Type
		switch asciiMode {
		case "standard":
			charsetType = charset.ASCII
		default: // "extended" or any other value defaults to Unicode
			charsetType = charset.Unicode
		}

		renderOpts := &d2ascii.RenderOpts{
			Scale:   opts.Scale,
			Charset: charsetType,
		}
		asciiArtist := d2ascii.NewASCIIartist()
		ascii, err := asciiArtist.Render(ctx, diagram, renderOpts)
		if err != nil {
			return ascii, false, err
		}
		written, err := writeWithStatus(ms, outputPath, ascii)
		if err != nil {
			return ascii, written, err
		}
		return ascii, written, nil
	}
	toPNG := outputFormat == PNG

	var scale *float64
	if opts.Scale != nil {
		scale = opts.Scale
	} else if toPNG {
		scale = go2.Pointer(1.)
	}
	renderOpts := &d2svg.RenderOpts{
		Pad:                opts.Pad,
		Sketch:             opts.Sketch,
		Center:             opts.Center,
		MasterID:           opts.MasterID,
		ThemeID:            opts.ThemeID,
		DarkThemeID:        opts.DarkThemeID,
		ThemeOverrides:     opts.ThemeOverrides,
		DarkThemeOverrides: opts.DarkThemeOverrides,
		NoXMLTag:           opts.NoXMLTag,
		Salt:               opts.Salt,
		Scale:              scale,
		OmitVersion:        opts.OmitVersion,
	}
	if toPNG {
		returnSVG := wantPreview || opts.MasterID != ""
		svg, err := renderRasterSVG(ctx, plugin, diagram, *renderOpts, returnSVG, opts.MasterID == "")
		if err != nil {
			return svg, false, err
		}
		cacheImages := ms.Env.Getenv("IMG_CACHE") == "1"
		if opts.MasterID == "" {
			written, err := writePNGWithStatus(ctx, ms, outputPath, func(output io.Writer) error {
				return renderPNGToWriter(ctx, inputPath, cacheImages, diagram, *renderOpts, pngEncoder, output)
			})
			if err != nil {
				return svg, written, err
			}
			return svg, written, nil
		}
		// Animated SVG composition only needs validation, not retained PNG bytes.
		err = renderPNGToWriter(ctx, inputPath, cacheImages, diagram, *renderOpts, pngEncoder, io.Discard)
		return svg, false, err
	}

	svg, err := d2svg.Render(diagram, renderOpts)
	if err != nil {
		return nil, false, err
	}
	if opts.MasterID == "" {
		svg, err = postProcess(ctx, plugin, svg)
		if err != nil {
			return svg, false, err
		}
	}

	cacheImages := ms.Env.Getenv("IMG_CACHE") == "1"
	l := simplelog.FromCmdLog(ms.Log)
	svg, bundleErr := imgbundler.BundleLocal(ctx, l, inputPath, svg, cacheImages)
	if bundle {
		var bundleErr2 error
		svg, bundleErr2 = imgbundler.BundleRemote(ctx, l, svg, cacheImages)
		bundleErr = multierr.Combine(bundleErr, bundleErr2)
	}
	if forceAppendix {
		svg = appendix.Append(diagram, renderOpts, ruler, svg)
	}

	out := svg
	if len(out) > 0 && out[len(out)-1] != '\n' {
		out = append(out, '\n')
	}

	var written bool
	if opts.MasterID == "" {
		err = os.MkdirAll(filepath.Dir(outputPath), 0755)
		if err != nil {
			return svg, false, err
		}
		written, err = writeWithStatus(ms, outputPath, out)
		if err != nil {
			return svg, written, err
		}
	}
	if bundleErr != nil {
		return svg, written, bundleErr
	}
	return svg, written, nil
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

func getFileName(path string) string {
	ext := filepath.Ext(path)
	return strings.TrimSuffix(filepath.Base(path), ext)
}

func populateLayoutOpts(ctx context.Context, ms *xmain.State, ps []d2plugin.Plugin) error {
	pluginFlags, err := d2plugin.ListPluginFlags(ctx, ps)
	if err != nil {
		return err
	}

	for _, f := range pluginFlags {
		f.AddToOpts(ms.Opts)
		// Don't pollute the main d2 flagset with these. It'll be a lot
		ms.Opts.Flags.MarkHidden(f.Name)
	}

	return nil
}

func loadFont(ms *xmain.State, path string) ([]byte, error) {
	if filepath.Ext(path) != ".ttf" {
		return nil, fmt.Errorf("expected .ttf file but %s has extension %s", path, filepath.Ext(path))
	}
	ttf, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read font at %s: %v", path, err)
	}
	ms.Log.Info.Printf("font %s loaded", filepath.Base(path))
	return ttf, nil
}

func loadFonts(ms *xmain.State, pathToRegular, pathToItalic, pathToBold, pathToSemibold, pathToMono, pathToMonoBold, pathToMonoItalic, pathToMonoSemibold string) (*d2fonts.FontFamily, *d2fonts.FontFamily, error) {
	if pathToRegular == "" && pathToItalic == "" && pathToBold == "" && pathToSemibold == "" &&
		pathToMono == "" && pathToMonoBold == "" && pathToMonoItalic == "" && pathToMonoSemibold == "" {
		return nil, nil, nil
	}

	var regularTTF []byte
	var italicTTF []byte
	var boldTTF []byte
	var semiboldTTF []byte
	var monoTTF []byte
	var monoBoldTTF []byte
	var monoItalicTTF []byte
	var monoSemiboldTTF []byte

	var err error
	if pathToRegular != "" {
		regularTTF, err = loadFont(ms, pathToRegular)
		if err != nil {
			return nil, nil, err
		}
	}
	if pathToItalic != "" {
		italicTTF, err = loadFont(ms, pathToItalic)
		if err != nil {
			return nil, nil, err
		}
	}
	if pathToBold != "" {
		boldTTF, err = loadFont(ms, pathToBold)
		if err != nil {
			return nil, nil, err
		}
	}
	if pathToSemibold != "" {
		semiboldTTF, err = loadFont(ms, pathToSemibold)
		if err != nil {
			return nil, nil, err
		}
	}

	if pathToMono != "" {
		monoTTF, err = loadFont(ms, pathToMono)
		if err != nil {
			return nil, nil, err
		}
	}
	if pathToMonoBold != "" {
		monoBoldTTF, err = loadFont(ms, pathToMonoBold)
		if err != nil {
			return nil, nil, err
		}
	}
	if pathToMonoItalic != "" {
		monoItalicTTF, err = loadFont(ms, pathToMonoItalic)
		if err != nil {
			return nil, nil, err
		}
	}
	if pathToMonoSemibold != "" {
		monoSemiboldTTF, err = loadFont(ms, pathToMonoSemibold)
		if err != nil {
			return nil, nil, err
		}
	}

	var fontFamily *d2fonts.FontFamily
	var monoFontFamily *d2fonts.FontFamily

	if pathToRegular != "" || pathToItalic != "" || pathToBold != "" || pathToSemibold != "" {
		fontFamily, err = d2fonts.AddFontFamily("custom", regularTTF, italicTTF, boldTTF, semiboldTTF)
		if err != nil {
			return nil, nil, err
		}
	}

	if pathToMono != "" || pathToMonoBold != "" || pathToMonoItalic != "" || pathToMonoSemibold != "" {
		monoFontFamily, err = d2fonts.AddFontFamily("customMono", monoTTF, monoItalicTTF, monoBoldTTF, monoSemiboldTTF)
		if err != nil {
			return nil, nil, err
		}
	}

	return fontFamily, monoFontFamily, nil
}

const LAYERS = "layers"
const STEPS = "steps"
const SCENARIOS = "scenarios"

func appendRasterPreview(diagram *d2target.Diagram, opts d2svg.RenderOpts, ruler *textmeasure.Ruler, sourceSVG []byte) ([]byte, error) {
	if len(sourceSVG) == 0 {
		return nil, nil
	}
	renderOpts := rasterRenderOptions(opts)
	preview := appendix.Append(diagram, &renderOpts, ruler, sourceSVG)
	if int64(len(preview)) > rasterPreviewMaxOutputBytes {
		return nil, fmt.Errorf("raster watch preview output bytes %d exceed limit %d", len(preview), rasterPreviewMaxOutputBytes)
	}
	return preview, nil
}

func Write(ms *xmain.State, path string, out []byte) error {
	_, err := writeWithStatus(ms, path, out)
	return err
}

func writeWithStatus(ms *xmain.State, path string, out []byte) (touched bool, err error) {
	if path == "-" {
		return writeStdout(ms.Stdout, out)
	}
	err = ms.AtomicWritePath(path, out)
	if err == nil {
		return true, nil
	}
	ms.Log.Debug.Printf("atomic write failed: %s, trying non-atomic write", err.Error())
	return writeFileWithStatus(path, out)
}

func writeFileWithStatus(path string, out []byte) (touched bool, err error) {
	output, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return false, err
	}
	n, writeErr := output.Write(out)
	if writeErr == nil && n != len(out) {
		writeErr = io.ErrShortWrite
	}
	return true, errors.Join(writeErr, output.Close())
}

// runFinalizer makes context-unaware encoding and output operations honor the
// command deadline before they start and before they report success.
func runFinalizer(ctx context.Context, finalize func() error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := finalize(); err != nil {
		return err
	}
	return ctx.Err()
}

func runStatusFinalizer(ctx context.Context, finalize func() (bool, error)) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	touched, err := finalize()
	if err != nil {
		return touched, err
	}
	return touched, ctx.Err()
}

func writeStdout(output io.WriteCloser, out []byte) (written bool, err error) {
	n, err := output.Write(out)
	written = n > 0
	if err != nil {
		return written, err
	}
	if n != len(out) {
		return written, io.ErrShortWrite
	}
	return written, output.Close()
}

func init() {
	log.Init()
}
