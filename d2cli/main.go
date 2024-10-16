package d2cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/playwright-community/playwright-go"
	"github.com/spf13/pflag"
	"go.uber.org/multierr"

	"oss.terrastruct.com/util-go/go2"
	"oss.terrastruct.com/util-go/xmain"

	"oss.terrastruct.com/d2/d2ast"
	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2lib"
	"oss.terrastruct.com/d2/d2parser"
	"oss.terrastruct.com/d2/d2plugin"
	"oss.terrastruct.com/d2/d2renderers/d2animate"
	"oss.terrastruct.com/d2/d2renderers/d2fonts"
	"oss.terrastruct.com/d2/d2renderers/d2svg"
	"oss.terrastruct.com/d2/d2renderers/d2svg/appendix"
	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/d2themes"
	"oss.terrastruct.com/d2/d2themes/d2themescatalog"
	"oss.terrastruct.com/d2/lib/background"
	"oss.terrastruct.com/d2/lib/imgbundler"
	"oss.terrastruct.com/d2/lib/log"
	"oss.terrastruct.com/d2/lib/pdf"
	"oss.terrastruct.com/d2/lib/png"
	"oss.terrastruct.com/d2/lib/pptx"
	"oss.terrastruct.com/d2/lib/simplelog"
	"oss.terrastruct.com/d2/lib/textmeasure"
	timelib "oss.terrastruct.com/d2/lib/time"
	"oss.terrastruct.com/d2/lib/version"
	"oss.terrastruct.com/d2/lib/xgif"
)

func Run(ctx context.Context, ms *xmain.State) (err error) {
	ctx = log.WithDefault(ctx)
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
	darkThemeFlag, err := ms.Opts.Int64("D2_DARK_THEME", "dark-theme", "", -1, "the theme to use when the viewer's browser is in dark mode. When left unset -theme is used for both light and dark mode. Be aware that explicit styles set in D2 code will still be applied and this may produce unexpected results. We plan on resolving this by making style maps in D2 light/dark mode specific. See https://github.com/terrastruct/d2/issues/831.")
	if err != nil {
		return err
	}
	padFlag, err := ms.Opts.Int64("D2_PAD", "pad", "", d2svg.DEFAULT_PADDING, "pixels padded around the rendered diagram")
	if err != nil {
		return err
	}
	animateIntervalFlag, err := ms.Opts.Int64("D2_ANIMATE_INTERVAL", "animate-interval", "", 0, "if given, multiple boards are packaged as 1 SVG which transitions through each board at the interval (in milliseconds). Can only be used with SVG exports.")
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

	fontFamily, err := loadFonts(ms, *fontRegularFlag, *fontItalicFlag, *fontBoldFlag, *fontSemiboldFlag)
	if err != nil {
		return xmain.UsageErrorf("failed to load specified fonts: %v", err)
	}

	if len(ms.Opts.Flags.Args()) > 0 {
		switch ms.Opts.Flags.Arg(0) {
		case "init-playwright":
			return initPlaywright()
		case "layout":
			return layoutCmd(ctx, ms, plugins)
		case "themes":
			themesCmd(ctx, ms)
			return nil
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
	outputFormat := getExportExtension(outputPath)
	if outputPath != "-" {
		outputPath = ms.AbsPath(outputPath)
		if *animateIntervalFlag > 0 && !outputFormat.supportsAnimation() {
			return xmain.UsageErrorf("--animate-interval can only be used when exporting to SVG or GIF.\nYou provided: %s", filepath.Ext(outputPath))
		} else if *animateIntervalFlag <= 0 && outputFormat.requiresAnimationInterval() {
			return xmain.UsageErrorf("--animate-interval must be greater than 0 for %s outputs.\nYou provided: %d", outputFormat, *animateIntervalFlag)
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
		darkThemeFlag = nil // TODO this is a temporary solution: https://github.com/terrastruct/util-go/issues/7
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
	var pw png.Playwright
	if outputFormat.requiresPNGRenderer() {
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

	renderOpts := d2svg.RenderOpts{
		Pad:         padFlag,
		Sketch:      sketchFlag,
		Center:      centerFlag,
		ThemeID:     themeFlag,
		DarkThemeID: darkThemeFlag,
		Scale:       scale,
	}

	if *watchFlag {
		if inputPath == "-" {
			return xmain.UsageErrorf("-w[atch] cannot be combined with reading input from stdin")
		}
		if *targetFlag != "*" {
			return xmain.UsageErrorf("-w[atch] cannot be combined with --target")
		}
		w, err := newWatcher(ctx, ms, watcherOpts{
			plugins:         plugins,
			layout:          layoutFlag,
			renderOpts:      renderOpts,
			animateInterval: *animateIntervalFlag,
			host:            *hostFlag,
			port:            *portFlag,
			inputPath:       inputPath,
			outputPath:      outputPath,
			bundle:          *bundleFlag,
			forceAppendix:   *forceAppendixFlag,
			pw:              pw,
			fontFamily:      fontFamily,
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
		boardPath = key.IDA()
	}

	ctx, cancel := timelib.WithTimeout(ctx, time.Minute*2)
	defer cancel()

	_, written, err := compile(ctx, ms, plugins, nil, layoutFlag, renderOpts, fontFamily, *animateIntervalFlag, inputPath, outputPath, boardPath, noChildren, *bundleFlag, *forceAppendixFlag, pw.Page)
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

func compile(ctx context.Context, ms *xmain.State, plugins []d2plugin.Plugin, fs fs.FS, layout *string, renderOpts d2svg.RenderOpts, fontFamily *d2fonts.FontFamily, animateInterval int64, inputPath, outputPath string, boardPath []string, noChildren, bundle, forceAppendix bool, page playwright.Page) (_ []byte, written bool, _ error) {
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
		InputPath:      inputPath,
		LayoutResolver: LayoutResolver(ctx, ms, plugins),
		Layout:         layout,
		RouterResolver: RouterResolver(ctx, ms, plugins),
		FS:             fs,
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
		masterID, err := diagram.HashID()
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

	ext := getExportExtension(outputPath)
	switch ext {
	case GIF:
		svg, pngs, err := renderPNGsForGIF(ctx, ms, plugin, renderOpts, ruler, page, inputPath, diagram)
		if err != nil {
			return nil, false, err
		}
		out, err := AnimatePNGs(ms, pngs, int(animateInterval))
		if err != nil {
			return nil, false, err
		}
		err = os.MkdirAll(filepath.Dir(outputPath), 0755)
		if err != nil {
			return nil, false, err
		}
		err = Write(ms, outputPath, out)
		if err != nil {
			return nil, false, err
		}
		dur := time.Since(start)
		ms.Log.Success.Printf("successfully compiled %s to %s in %s", ms.HumanPath(inputPath), ms.HumanPath(outputPath), dur)
		return svg, true, nil
	case PDF:
		pageMap := buildBoardIDToIndex(diagram, nil, nil)
		path := []pdf.BoardTitle{
			{Name: diagram.Root.Label, BoardID: "root"},
		}
		pdf, err := renderPDF(ctx, ms, plugin, renderOpts, inputPath, outputPath, page, ruler, diagram, nil, path, pageMap, diagram.Root.Label != "")
		if err != nil {
			return pdf, false, err
		}
		dur := time.Since(start)
		ms.Log.Success.Printf("successfully compiled %s to %s in %s", ms.HumanPath(inputPath), ms.HumanPath(outputPath), dur)
		return pdf, true, nil
	case PPTX:
		var username string
		if user, err := user.Current(); err == nil {
			username = user.Username
		}
		description := "Presentation generated with D2 - https://d2lang.com"
		rootName := getFileName(outputPath)
		// version must be only numbers to avoid issues with PowerPoint
		p := pptx.NewPresentation(rootName, description, rootName, username, version.OnlyNumbers(), diagram.Root.Label != "")

		boardIdToIndex := buildBoardIDToIndex(diagram, nil, nil)
		path := []pptx.BoardTitle{
			{Name: "root", BoardID: "root", LinkToSlide: boardIdToIndex["root"] + 1},
		}
		svg, err := renderPPTX(ctx, ms, p, plugin, renderOpts, ruler, inputPath, outputPath, page, diagram, path, boardIdToIndex)
		if err != nil {
			return nil, false, err
		}
		err = p.SaveTo(outputPath)
		if err != nil {
			return nil, false, err
		}
		dur := time.Since(start)
		ms.Log.Success.Printf("successfully compiled %s to %s in %s", ms.HumanPath(inputPath), ms.HumanPath(outputPath), dur)
		return svg, true, nil
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
		var err error
		if noChildren {
			boards, err = renderSingle(ctx, ms, compileDur, plugin, renderOpts, inputPath, outputPath, bundle, forceAppendix, page, ruler, diagram)
		} else {
			boards, err = render(ctx, ms, compileDur, plugin, renderOpts, inputPath, outputPath, bundle, forceAppendix, page, ruler, diagram)
		}
		if err != nil {
			return nil, false, err
		}
		var out []byte
		if len(boards) > 0 {
			out = boards[0]
			if animateInterval > 0 {
				out, err = d2animate.Wrap(diagram, boards, renderOpts, int(animateInterval))
				if err != nil {
					return nil, false, err
				}
				out, err = plugin.PostProcess(ctx, out)
				if err != nil {
					return nil, false, err
				}
				err = os.MkdirAll(filepath.Dir(outputPath), 0755)
				if err != nil {
					return nil, false, err
				}
				err = Write(ms, outputPath, out)
				if err != nil {
					return nil, false, err
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

func render(ctx context.Context, ms *xmain.State, compileDur time.Duration, plugin d2plugin.Plugin, opts d2svg.RenderOpts, inputPath, outputPath string, bundle, forceAppendix bool, page playwright.Page, ruler *textmeasure.Ruler, diagram *d2target.Diagram) ([][]byte, error) {
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
			return nil, fmt.Errorf("multiboard output cannot be written to stdout")
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
		childrenBoards, err := render(ctx, ms, compileDur, plugin, opts, inputPath, layersOutputPath, bundle, forceAppendix, page, ruler, dl)
		if err != nil {
			return nil, err
		}
		boards = append(boards, childrenBoards...)
	}
	for _, dl := range diagram.Scenarios {
		childrenBoards, err := render(ctx, ms, compileDur, plugin, opts, inputPath, scenariosOutputPath, bundle, forceAppendix, page, ruler, dl)
		if err != nil {
			return nil, err
		}
		boards = append(boards, childrenBoards...)
	}
	for _, dl := range diagram.Steps {
		childrenBoards, err := render(ctx, ms, compileDur, plugin, opts, inputPath, stepsOutputPath, bundle, forceAppendix, page, ruler, dl)
		if err != nil {
			return nil, err
		}
		boards = append(boards, childrenBoards...)
	}

	if !diagram.IsFolderOnly {
		start := time.Now()
		out, err := _render(ctx, ms, plugin, opts, inputPath, boardOutputPath, bundle, forceAppendix, page, ruler, diagram)
		if err != nil {
			return boards, err
		}
		dur := compileDur + time.Since(start)
		if opts.MasterID == "" {
			ms.Log.Success.Printf("successfully compiled %s to %s in %s", ms.HumanPath(inputPath), ms.HumanPath(boardOutputPath), dur)
		}
		boards = append([][]byte{out}, boards...)
	}

	return boards, nil
}

func renderSingle(ctx context.Context, ms *xmain.State, compileDur time.Duration, plugin d2plugin.Plugin, opts d2svg.RenderOpts, inputPath, outputPath string, bundle, forceAppendix bool, page playwright.Page, ruler *textmeasure.Ruler, diagram *d2target.Diagram) ([][]byte, error) {
	start := time.Now()
	out, err := _render(ctx, ms, plugin, opts, inputPath, outputPath, bundle, forceAppendix, page, ruler, diagram)
	if err != nil {
		return [][]byte{}, err
	}
	dur := compileDur + time.Since(start)
	if opts.MasterID == "" {
		ms.Log.Success.Printf("successfully compiled %s to %s in %s", ms.HumanPath(inputPath), ms.HumanPath(outputPath), dur)
	}
	return [][]byte{out}, nil
}

func _render(ctx context.Context, ms *xmain.State, plugin d2plugin.Plugin, opts d2svg.RenderOpts, inputPath, outputPath string, bundle, forceAppendix bool, page playwright.Page, ruler *textmeasure.Ruler, diagram *d2target.Diagram) ([]byte, error) {
	toPNG := getExportExtension(outputPath) == PNG
	var scale *float64
	if opts.Scale != nil {
		scale = opts.Scale
	} else if toPNG {
		scale = go2.Pointer(1.)
	}
	svg, err := d2svg.Render(diagram, &d2svg.RenderOpts{
		Pad:                opts.Pad,
		Sketch:             opts.Sketch,
		Center:             opts.Center,
		MasterID:           opts.MasterID,
		ThemeID:            opts.ThemeID,
		DarkThemeID:        opts.DarkThemeID,
		ThemeOverrides:     opts.ThemeOverrides,
		DarkThemeOverrides: opts.DarkThemeOverrides,
		Scale:              scale,
	})
	if err != nil {
		return nil, err
	}

	if opts.MasterID == "" {
		svg, err = plugin.PostProcess(ctx, svg)
		if err != nil {
			return svg, err
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
	if forceAppendix && !toPNG {
		svg = appendix.Append(diagram, ruler, svg)
	}

	out := svg
	if toPNG {
		svg := appendix.Append(diagram, ruler, svg)

		if !bundle {
			var bundleErr2 error
			svg, bundleErr2 = imgbundler.BundleRemote(ctx, l, svg, cacheImages)
			bundleErr = multierr.Combine(bundleErr, bundleErr2)
		}

		out, err = ConvertSVG(ms, page, svg)
		if err != nil {
			return svg, err
		}
		out, err = png.AddExif(out)
		if err != nil {
			return svg, err
		}
	} else {
		if len(out) > 0 && out[len(out)-1] != '\n' {
			out = append(out, '\n')
		}
	}

	if opts.MasterID == "" {
		err = os.MkdirAll(filepath.Dir(outputPath), 0755)
		if err != nil {
			return svg, err
		}
		err = Write(ms, outputPath, out)
		if err != nil {
			return svg, err
		}
	}
	if bundleErr != nil {
		return svg, bundleErr
	}
	return svg, nil
}

func renderPDF(ctx context.Context, ms *xmain.State, plugin d2plugin.Plugin, opts d2svg.RenderOpts, inputPath, outputPath string, page playwright.Page, ruler *textmeasure.Ruler, diagram *d2target.Diagram, doc *pdf.GoFPDF, boardPath []pdf.BoardTitle, pageMap map[string]int, includeNav bool) (svg []byte, err error) {
	var isRoot bool
	if doc == nil {
		doc = pdf.Init()
		isRoot = true
	}

	if !diagram.IsFolderOnly {
		rootFill := diagram.Root.Fill
		// gofpdf will print the png img with a slight filter
		// make the bg fill within the png transparent so that the pdf bg fill is the only bg color present
		diagram.Root.Fill = "transparent"

		var scale *float64
		if opts.Scale != nil {
			scale = opts.Scale
		} else {
			scale = go2.Pointer(1.)
		}

		svg, err = d2svg.Render(diagram, &d2svg.RenderOpts{
			Pad:                opts.Pad,
			Sketch:             opts.Sketch,
			Center:             opts.Center,
			Scale:              scale,
			ThemeID:            opts.ThemeID,
			DarkThemeID:        opts.DarkThemeID,
			ThemeOverrides:     opts.ThemeOverrides,
			DarkThemeOverrides: opts.DarkThemeOverrides,
		})
		if err != nil {
			return nil, err
		}

		svg, err = plugin.PostProcess(ctx, svg)
		if err != nil {
			return svg, err
		}

		cacheImages := ms.Env.Getenv("IMG_CACHE") == "1"
		l := simplelog.FromCmdLog(ms.Log)
		svg, bundleErr := imgbundler.BundleLocal(ctx, l, inputPath, svg, cacheImages)
		svg, bundleErr2 := imgbundler.BundleRemote(ctx, l, svg, cacheImages)
		bundleErr = multierr.Combine(bundleErr, bundleErr2)
		if bundleErr != nil {
			return svg, bundleErr
		}
		svg = appendix.Append(diagram, ruler, svg)

		pngImg, err := ConvertSVG(ms, page, svg)
		if err != nil {
			return svg, err
		}

		viewboxSlice := appendix.FindViewboxSlice(svg)
		viewboxX, err := strconv.ParseFloat(viewboxSlice[0], 64)
		if err != nil {
			return svg, err
		}
		viewboxY, err := strconv.ParseFloat(viewboxSlice[1], 64)
		if err != nil {
			return svg, err
		}
		err = doc.AddPDFPage(pngImg, boardPath, *opts.ThemeID, rootFill, diagram.Shapes, *opts.Pad, viewboxX, viewboxY, pageMap, includeNav)
		if err != nil {
			return svg, err
		}
	}

	for _, dl := range diagram.Layers {
		path := append(boardPath, pdf.BoardTitle{
			Name:    dl.Root.Label,
			BoardID: strings.Join([]string{boardPath[len(boardPath)-1].BoardID, LAYERS, dl.Name}, "."),
		})
		_, err := renderPDF(ctx, ms, plugin, opts, inputPath, "", page, ruler, dl, doc, path, pageMap, includeNav)
		if err != nil {
			return nil, err
		}
	}
	for _, dl := range diagram.Scenarios {
		path := append(boardPath, pdf.BoardTitle{
			Name:    dl.Root.Label,
			BoardID: strings.Join([]string{boardPath[len(boardPath)-1].BoardID, SCENARIOS, dl.Name}, "."),
		})
		_, err := renderPDF(ctx, ms, plugin, opts, inputPath, "", page, ruler, dl, doc, path, pageMap, includeNav)
		if err != nil {
			return nil, err
		}
	}
	for _, dl := range diagram.Steps {
		path := append(boardPath, pdf.BoardTitle{
			Name:    dl.Root.Label,
			BoardID: strings.Join([]string{boardPath[len(boardPath)-1].BoardID, STEPS, dl.Name}, "."),
		})
		_, err := renderPDF(ctx, ms, plugin, opts, inputPath, "", page, ruler, dl, doc, path, pageMap, includeNav)
		if err != nil {
			return nil, err
		}
	}

	if isRoot {
		err := doc.Export(outputPath)
		if err != nil {
			return nil, err
		}
	}

	return svg, nil
}

func renderPPTX(ctx context.Context, ms *xmain.State, presentation *pptx.Presentation, plugin d2plugin.Plugin, opts d2svg.RenderOpts, ruler *textmeasure.Ruler, inputPath, outputPath string, page playwright.Page, diagram *d2target.Diagram, boardPath []pptx.BoardTitle, boardIDToIndex map[string]int) ([]byte, error) {
	var svg []byte
	if !diagram.IsFolderOnly {
		// gofpdf will print the png img with a slight filter
		// make the bg fill within the png transparent so that the pdf bg fill is the only bg color present
		diagram.Root.Fill = "transparent"

		var scale *float64
		if opts.Scale != nil {
			scale = opts.Scale
		} else {
			scale = go2.Pointer(1.)
		}

		var err error

		svg, err = d2svg.Render(diagram, &d2svg.RenderOpts{
			Pad:                opts.Pad,
			Sketch:             opts.Sketch,
			Center:             opts.Center,
			Scale:              scale,
			ThemeID:            opts.ThemeID,
			DarkThemeID:        opts.DarkThemeID,
			ThemeOverrides:     opts.ThemeOverrides,
			DarkThemeOverrides: opts.DarkThemeOverrides,
		})
		if err != nil {
			return nil, err
		}

		svg, err = plugin.PostProcess(ctx, svg)
		if err != nil {
			return nil, err
		}

		cacheImages := ms.Env.Getenv("IMG_CACHE") == "1"
		l := simplelog.FromCmdLog(ms.Log)
		svg, bundleErr := imgbundler.BundleLocal(ctx, l, inputPath, svg, cacheImages)
		svg, bundleErr2 := imgbundler.BundleRemote(ctx, l, svg, cacheImages)
		bundleErr = multierr.Combine(bundleErr, bundleErr2)
		if bundleErr != nil {
			return nil, bundleErr
		}

		svg = appendix.Append(diagram, ruler, svg)

		pngImg, err := ConvertSVG(ms, page, svg)
		if err != nil {
			return nil, err
		}

		slide, err := presentation.AddSlide(pngImg, boardPath)
		if err != nil {
			return nil, err
		}

		viewboxSlice := appendix.FindViewboxSlice(svg)
		viewboxX, err := strconv.ParseFloat(viewboxSlice[0], 64)
		if err != nil {
			return nil, err
		}
		viewboxY, err := strconv.ParseFloat(viewboxSlice[1], 64)
		if err != nil {
			return nil, err
		}

		// Draw links
		for _, shape := range diagram.Shapes {
			if shape.Link == "" {
				continue
			}

			linkX := png.SCALE * (float64(shape.Pos.X) - viewboxX - float64(shape.StrokeWidth))
			linkY := png.SCALE * (float64(shape.Pos.Y) - viewboxY - float64(shape.StrokeWidth))
			linkWidth := png.SCALE * (float64(shape.Width) + float64(shape.StrokeWidth*2))
			linkHeight := png.SCALE * (float64(shape.Height) + float64(shape.StrokeWidth*2))
			link := &pptx.Link{
				Left:    int(linkX),
				Top:     int(linkY),
				Width:   int(linkWidth),
				Height:  int(linkHeight),
				Tooltip: shape.Link,
			}
			slide.AddLink(link)
			key, err := d2parser.ParseKey(shape.Link)
			if err != nil || key.Path[0].Unbox().ScalarString() != "root" {
				// External link
				link.ExternalUrl = shape.Link
			} else if pageNum, ok := boardIDToIndex[shape.Link]; ok {
				// Internal link
				link.SlideIndex = pageNum + 1
			}
		}
	}

	for _, dl := range diagram.Layers {
		boardID := strings.Join([]string{boardPath[len(boardPath)-1].BoardID, LAYERS, dl.Name}, ".")
		path := append(boardPath, pptx.BoardTitle{
			Name:        dl.Name,
			BoardID:     boardID,
			LinkToSlide: boardIDToIndex[boardID] + 1,
		})
		_, err := renderPPTX(ctx, ms, presentation, plugin, opts, ruler, inputPath, "", page, dl, path, boardIDToIndex)
		if err != nil {
			return nil, err
		}
	}
	for _, dl := range diagram.Scenarios {
		boardID := strings.Join([]string{boardPath[len(boardPath)-1].BoardID, SCENARIOS, dl.Name}, ".")
		path := append(boardPath, pptx.BoardTitle{
			Name:        dl.Name,
			BoardID:     boardID,
			LinkToSlide: boardIDToIndex[boardID] + 1,
		})
		_, err := renderPPTX(ctx, ms, presentation, plugin, opts, ruler, inputPath, "", page, dl, path, boardIDToIndex)
		if err != nil {
			return nil, err
		}
	}
	for _, dl := range diagram.Steps {
		boardID := strings.Join([]string{boardPath[len(boardPath)-1].BoardID, STEPS, dl.Name}, ".")
		path := append(boardPath, pptx.BoardTitle{
			Name:        dl.Name,
			BoardID:     boardID,
			LinkToSlide: boardIDToIndex[boardID] + 1,
		})
		_, err := renderPPTX(ctx, ms, presentation, plugin, opts, ruler, inputPath, "", page, dl, path, boardIDToIndex)
		if err != nil {
			return nil, err
		}
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

func initPlaywright() error {
	pw, err := png.InitPlaywright()
	if err != nil {
		return err
	}
	return pw.Cleanup()
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

func loadFonts(ms *xmain.State, pathToRegular, pathToItalic, pathToBold, pathToSemibold string) (*d2fonts.FontFamily, error) {
	if pathToRegular == "" && pathToItalic == "" && pathToBold == "" && pathToSemibold == "" {
		return nil, nil
	}

	var regularTTF []byte
	var italicTTF []byte
	var boldTTF []byte
	var semiboldTTF []byte

	var err error
	if pathToRegular != "" {
		regularTTF, err = loadFont(ms, pathToRegular)
		if err != nil {
			return nil, err
		}
	}
	if pathToItalic != "" {
		italicTTF, err = loadFont(ms, pathToItalic)
		if err != nil {
			return nil, err
		}
	}
	if pathToBold != "" {
		boldTTF, err = loadFont(ms, pathToBold)
		if err != nil {
			return nil, err
		}
	}
	if pathToSemibold != "" {
		semiboldTTF, err = loadFont(ms, pathToSemibold)
		if err != nil {
			return nil, err
		}
	}

	return d2fonts.AddFontFamily("custom", regularTTF, italicTTF, boldTTF, semiboldTTF)
}

const LAYERS = "layers"
const STEPS = "steps"
const SCENARIOS = "scenarios"

// buildBoardIDToIndex returns a map from board path to page int
// To map correctly, it must follow the same traversal of pdf/pptx building
func buildBoardIDToIndex(diagram *d2target.Diagram, dictionary map[string]int, path []string) map[string]int {
	newPath := append(path, diagram.Name)
	if dictionary == nil {
		dictionary = map[string]int{}
		newPath[0] = "root"
	}

	key := strings.Join(newPath, ".")
	dictionary[key] = len(dictionary)

	for _, dl := range diagram.Layers {
		buildBoardIDToIndex(dl, dictionary, append(newPath, LAYERS))
	}
	for _, dl := range diagram.Scenarios {
		buildBoardIDToIndex(dl, dictionary, append(newPath, SCENARIOS))
	}
	for _, dl := range diagram.Steps {
		buildBoardIDToIndex(dl, dictionary, append(newPath, STEPS))
	}

	return dictionary
}

func renderPNGsForGIF(ctx context.Context, ms *xmain.State, plugin d2plugin.Plugin, opts d2svg.RenderOpts, ruler *textmeasure.Ruler, page playwright.Page, inputPath string, diagram *d2target.Diagram) (svg []byte, pngs [][]byte, err error) {
	if !diagram.IsFolderOnly {

		var scale *float64
		if opts.Scale != nil {
			scale = opts.Scale
		} else {
			scale = go2.Pointer(1.)
		}
		svg, err = d2svg.Render(diagram, &d2svg.RenderOpts{
			Pad:                opts.Pad,
			Sketch:             opts.Sketch,
			Center:             opts.Center,
			Scale:              scale,
			ThemeID:            opts.ThemeID,
			DarkThemeID:        opts.DarkThemeID,
			ThemeOverrides:     opts.ThemeOverrides,
			DarkThemeOverrides: opts.DarkThemeOverrides,
		})
		if err != nil {
			return nil, nil, err
		}

		svg, err = plugin.PostProcess(ctx, svg)
		if err != nil {
			return nil, nil, err
		}

		cacheImages := ms.Env.Getenv("IMG_CACHE") == "1"
		l := simplelog.FromCmdLog(ms.Log)
		svg, bundleErr := imgbundler.BundleLocal(ctx, l, inputPath, svg, cacheImages)
		svg, bundleErr2 := imgbundler.BundleRemote(ctx, l, svg, cacheImages)
		bundleErr = multierr.Combine(bundleErr, bundleErr2)
		if bundleErr != nil {
			return nil, nil, bundleErr
		}

		svg = appendix.Append(diagram, ruler, svg)

		pngImg, err := ConvertSVG(ms, page, svg)
		if err != nil {
			return nil, nil, err
		}
		pngs = append(pngs, pngImg)
	}

	for _, dl := range diagram.Layers {
		_, layerPNGs, err := renderPNGsForGIF(ctx, ms, plugin, opts, ruler, page, inputPath, dl)
		if err != nil {
			return nil, nil, err
		}
		pngs = append(pngs, layerPNGs...)
	}
	for _, dl := range diagram.Scenarios {
		_, scenarioPNGs, err := renderPNGsForGIF(ctx, ms, plugin, opts, ruler, page, inputPath, dl)
		if err != nil {
			return nil, nil, err
		}
		pngs = append(pngs, scenarioPNGs...)
	}
	for _, dl := range diagram.Steps {
		_, stepsPNGs, err := renderPNGsForGIF(ctx, ms, plugin, opts, ruler, page, inputPath, dl)
		if err != nil {
			return nil, nil, err
		}
		pngs = append(pngs, stepsPNGs...)
	}

	return svg, pngs, nil
}

func ConvertSVG(ms *xmain.State, page playwright.Page, svg []byte) ([]byte, error) {
	cancel := background.Repeat(func() {
		ms.Log.Info.Printf("converting to PNG...")
	}, time.Second*5)
	defer cancel()

	return png.ConvertSVG(page, svg)
}

func AnimatePNGs(ms *xmain.State, pngs [][]byte, animIntervalMs int) ([]byte, error) {
	cancel := background.Repeat(func() {
		ms.Log.Info.Printf("generating GIF...")
	}, time.Second*5)
	defer cancel()

	return xgif.AnimatePNGs(pngs, animIntervalMs)
}

func Write(ms *xmain.State, path string, out []byte) error {
	err := ms.AtomicWritePath(path, out)
	if err == nil {
		return nil
	}
	ms.Log.Debug.Printf("atomic write failed: %s, trying non-atomic write", err.Error())
	return ms.WritePath(path, out)
}

func init() {
	log.Init()
}
