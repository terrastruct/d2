//go:build js && wasm

package d2wasm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"syscall/js"

	"oss.terrastruct.com/d2/d2ast"
	"oss.terrastruct.com/d2/d2compiler"
	"oss.terrastruct.com/d2/d2format"
	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2layouts/d2dagrelayout"
	"oss.terrastruct.com/d2/d2layouts/d2elklayout"
	"oss.terrastruct.com/d2/d2lib"
	"oss.terrastruct.com/d2/d2lsp"
	"oss.terrastruct.com/d2/d2oracle"
	"oss.terrastruct.com/d2/d2parser"
	"oss.terrastruct.com/d2/d2renderers/d2animate"
	"oss.terrastruct.com/d2/d2renderers/d2fonts"
	"oss.terrastruct.com/d2/d2renderers/d2svg"
	"oss.terrastruct.com/d2/d2renderers/d2svg/appendix"
	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/log"
	"oss.terrastruct.com/d2/lib/memfs"
	"oss.terrastruct.com/d2/lib/textmeasure"
	"oss.terrastruct.com/d2/lib/urlenc"
	"oss.terrastruct.com/d2/lib/version"
)

const DEFAULT_INPUT_PATH = "index"

func GetParentID(args []js.Value) (interface{}, error) {
	if len(args) < 1 {
		return nil, &WASMError{Message: "missing id argument", Code: 400}
	}

	id := args[0].String()
	mk, err := d2parser.ParseMapKey(id)
	if err != nil {
		return nil, &WASMError{Message: err.Error(), Code: 400}
	}

	if len(mk.Edges) > 0 {
		return "", nil
	}

	if mk.Key != nil {
		if len(mk.Key.Path) == 1 {
			return "root", nil
		}
		mk.Key.Path = mk.Key.Path[:len(mk.Key.Path)-1]
		return strings.Join(mk.Key.StringIDA(), "."), nil
	}

	return "", nil
}

func GetObjOrder(args []js.Value) (interface{}, error) {
	if len(args) < 1 {
		return nil, &WASMError{Message: "missing dsl argument", Code: 400}
	}

	dsl := args[0].String()
	g, _, err := d2compiler.Compile("", strings.NewReader(dsl), &d2compiler.CompileOptions{
		UTF16Pos: true,
	})
	if err != nil {
		return nil, &WASMError{Message: err.Error(), Code: 400}
	}

	objOrder, err := d2oracle.GetObjOrder(g, nil)
	if err != nil {
		return nil, &WASMError{Message: err.Error(), Code: 500}
	}

	return map[string]interface{}{
		"order": objOrder,
	}, nil
}

func GetRefRanges(args []js.Value) (interface{}, error) {
	if len(args) < 4 {
		return nil, &WASMError{Message: "missing required arguments", Code: 400}
	}

	var fs map[string]string
	if err := json.Unmarshal([]byte(args[0].String()), &fs); err != nil {
		return nil, &WASMError{Message: "invalid fs argument", Code: 400}
	}

	file := args[1].String()
	key := args[2].String()

	var boardPath []string
	if err := json.Unmarshal([]byte(args[3].String()), &boardPath); err != nil {
		return nil, &WASMError{Message: "invalid boardPath argument", Code: 400}
	}

	ranges, importRanges, err := d2lsp.GetRefRanges(file, fs, boardPath, key)
	if err != nil {
		return nil, &WASMError{Message: err.Error(), Code: 500}
	}

	return RefRangesResponse{
		Ranges:       ranges,
		ImportRanges: importRanges,
	}, nil
}

func GetELKGraph(args []js.Value) (interface{}, error) {
	if len(args) < 1 {
		return nil, &WASMError{Message: "missing JSON argument", Code: 400}
	}
	var input CompileRequest
	if err := json.Unmarshal([]byte(args[0].String()), &input); err != nil {
		return nil, &WASMError{Message: "invalid JSON input", Code: 400}
	}

	if input.FS == nil {
		return nil, &WASMError{Message: "missing 'fs' field in input JSON", Code: 400}
	}

	inputPath := DEFAULT_INPUT_PATH

	if input.InputPath != nil {
		inputPath = *input.InputPath
	}

	if _, ok := input.FS[inputPath]; !ok {
		return nil, &WASMError{Message: fmt.Sprintf("missing '%s' file in input fs", inputPath), Code: 400}
	}

	fs, err := memfs.New(input.FS)
	if err != nil {
		return nil, &WASMError{Message: fmt.Sprintf("invalid fs input: %s", err.Error()), Code: 400}
	}

	g, _, err := d2compiler.Compile(inputPath, strings.NewReader(input.FS[inputPath]), &d2compiler.CompileOptions{
		UTF16Pos: true,
		FS:       fs,
	})
	if err != nil {
		return nil, &WASMError{Message: err.Error(), Code: 400}
	}

	ruler, err := textmeasure.NewRuler()
	if err != nil {
		return nil, &WASMError{Message: fmt.Sprintf("text ruler cannot be initialized: %s", err.Error()), Code: 500}
	}
	err = g.SetDimensions(nil, ruler, nil)
	if err != nil {
		return nil, err
	}

	elk, err := d2elklayout.ConvertGraph(context.Background(), g, nil)
	if err != nil {
		return nil, &WASMError{Message: err.Error(), Code: 400}
	}
	return elk, nil
}

func Compile(args []js.Value) (interface{}, error) {
	if len(args) < 1 {
		return nil, &WASMError{Message: "missing JSON argument", Code: 400}
	}
	var input CompileRequest
	if err := json.Unmarshal([]byte(args[0].String()), &input); err != nil {
		return nil, &WASMError{Message: "invalid JSON input", Code: 400}
	}

	if input.FS == nil {
		return nil, &WASMError{Message: "missing 'fs' field in input JSON", Code: 400}
	}

	compileOpts := &d2lib.CompileOptions{
		UTF16Pos: true,
	}

	inputPath := DEFAULT_INPUT_PATH

	if input.InputPath != nil {
		inputPath = *input.InputPath
	}

	if _, ok := input.FS[inputPath]; !ok {
		return nil, &WASMError{Message: fmt.Sprintf("missing '%s' file in input fs", inputPath), Code: 400}
	}

	compileOpts.InputPath = inputPath

	compileOpts.LayoutResolver = func(engine string) (d2graph.LayoutGraph, error) {
		switch engine {
		case "dagre":
			return d2dagrelayout.DefaultLayout, nil
		case "elk":
			return d2elklayout.DefaultLayout, nil
		default:
			return nil, &WASMError{Message: fmt.Sprintf("layout option '%s' not recognized", engine), Code: 400}
		}
	}

	var err error
	compileOpts.FS, err = memfs.New(input.FS)
	if err != nil {
		return nil, &WASMError{Message: fmt.Sprintf("invalid fs input: %s", err.Error()), Code: 400}
	}

	var fontRegular []byte
	var fontItalic []byte
	var fontBold []byte
	var fontSemibold []byte
	if input.Opts != nil && (input.Opts.FontRegular != nil) {
		fontRegular = *input.Opts.FontRegular
	}
	if input.Opts != nil && (input.Opts.FontItalic != nil) {
		fontItalic = *input.Opts.FontItalic
	}
	if input.Opts != nil && (input.Opts.FontBold != nil) {
		fontBold = *input.Opts.FontBold
	}
	if input.Opts != nil && (input.Opts.FontSemibold != nil) {
		fontSemibold = *input.Opts.FontSemibold
	}
	if fontRegular != nil || fontItalic != nil || fontBold != nil || fontSemibold != nil {
		fontFamily, err := d2fonts.AddFontFamily("custom", fontRegular, fontItalic, fontBold, fontSemibold)
		if err != nil {
			return nil, &WASMError{Message: fmt.Sprintf("custom fonts could not be initialized: %s", err.Error()), Code: 400}
		}
		compileOpts.FontFamily = fontFamily
	}

	compileOpts.Ruler, err = textmeasure.NewRuler()
	if err != nil {
		return nil, &WASMError{Message: fmt.Sprintf("text ruler cannot be initialized: %s", err.Error()), Code: 500}
	}

	if input.Opts != nil && input.Opts.Layout != nil {
		compileOpts.Layout = input.Opts.Layout
	}

	renderOpts := &d2svg.RenderOpts{}
	if input.Opts != nil && input.Opts.Sketch != nil {
		renderOpts.Sketch = input.Opts.Sketch
	}
	if input.Opts != nil && input.Opts.Pad != nil {
		renderOpts.Pad = input.Opts.Pad
	}
	if input.Opts != nil && input.Opts.Center != nil {
		renderOpts.Center = input.Opts.Center
	}
	if input.Opts != nil && input.Opts.ThemeID != nil {
		renderOpts.ThemeID = input.Opts.ThemeID
	}
	if input.Opts != nil && input.Opts.DarkThemeID != nil {
		renderOpts.DarkThemeID = input.Opts.DarkThemeID
	}
	if input.Opts != nil && input.Opts.Scale != nil {
		renderOpts.Scale = input.Opts.Scale
	}

	ctx := log.WithDefault(context.Background())
	diagram, g, err := d2lib.Compile(ctx, input.FS[inputPath], compileOpts, renderOpts)
	if err != nil {
		if pe, ok := err.(*d2parser.ParseError); ok {
			errs, _ := json.Marshal(pe.Errors)
			return nil, &WASMError{Message: string(errs), Code: 400}
		}
		return nil, &WASMError{Message: err.Error(), Code: 500}
	}

	input.FS[inputPath] = d2format.Format(g.AST)

	return CompileResponse{
		FS:        input.FS,
		InputPath: inputPath,
		Diagram:   *diagram,
		Graph:     *g,
		RenderOptions: RenderOptions{
			ThemeID:         renderOpts.ThemeID,
			DarkThemeID:     renderOpts.DarkThemeID,
			Sketch:          renderOpts.Sketch,
			Pad:             renderOpts.Pad,
			Center:          renderOpts.Center,
			Scale:           renderOpts.Scale,
			ForceAppendix:   input.Opts.ForceAppendix,
			Target:          input.Opts.Target,
			AnimateInterval: input.Opts.AnimateInterval,
			Salt:            input.Opts.Salt,
			NoXMLTag:        input.Opts.NoXMLTag,
		},
	}, nil
}

func Render(args []js.Value) (interface{}, error) {
	if len(args) < 1 {
		return nil, &WASMError{Message: "missing JSON argument", Code: 400}
	}
	var input RenderRequest
	if err := json.Unmarshal([]byte(args[0].String()), &input); err != nil {
		return nil, &WASMError{Message: "invalid JSON input", Code: 400}
	}

	if input.Diagram == nil {
		return nil, &WASMError{Message: "missing 'diagram' field in input JSON", Code: 400}
	}

	animateInterval := 0
	if input.Opts != nil && input.Opts.AnimateInterval != nil && *input.Opts.AnimateInterval > 0 {
		animateInterval = int(*input.Opts.AnimateInterval)
	}

	var boardPath []string
	noChildren := true

	if input.Opts.Target != nil {
		switch *input.Opts.Target {
		case "*":
			noChildren = false
		case "":
		default:
			target := *input.Opts.Target
			if strings.HasSuffix(target, ".*") {
				target = target[:len(target)-2]
				noChildren = false
			}
			key, err := d2parser.ParseKey(target)
			if err != nil {
				return nil, &WASMError{Message: fmt.Sprintf("target '%s' not recognized", target), Code: 400}
			}
			boardPath = key.StringIDA()
		}
		if !noChildren && animateInterval <= 0 {
			return nil, &WASMError{Message: fmt.Sprintf("target '%s' only supported for animated SVGs", *input.Opts.Target), Code: 500}
		}
	}

	diagram := input.Diagram.GetBoard(boardPath)
	if diagram == nil {
		return nil, &WASMError{Message: fmt.Sprintf("render target '%s' not found", strings.Join(boardPath, ".")), Code: 400}
	}
	if noChildren {
		diagram.Layers = nil
		diagram.Scenarios = nil
		diagram.Steps = nil
	}

	renderOpts := &d2svg.RenderOpts{}

	if input.Opts != nil && input.Opts.Salt != nil {
		renderOpts.Salt = input.Opts.Salt
	}

	if animateInterval > 0 {
		masterID, err := diagram.HashID(renderOpts.Salt)
		if err != nil {
			return nil, &WASMError{Message: fmt.Sprintf("cannot process animate interval: %s", err.Error()), Code: 500}
		}
		renderOpts.MasterID = masterID
	}

	ruler, err := textmeasure.NewRuler()
	if err != nil {
		return nil, &WASMError{Message: fmt.Sprintf("text ruler cannot be initialized: %s", err.Error()), Code: 500}
	}

	if input.Opts != nil && input.Opts.Sketch != nil {
		renderOpts.Sketch = input.Opts.Sketch
	}
	if input.Opts != nil && input.Opts.Pad != nil {
		renderOpts.Pad = input.Opts.Pad
	}
	if input.Opts != nil && input.Opts.Center != nil {
		renderOpts.Center = input.Opts.Center
	}
	if input.Opts != nil && input.Opts.ThemeID != nil {
		renderOpts.ThemeID = input.Opts.ThemeID
	}
	if input.Opts != nil && input.Opts.DarkThemeID != nil {
		renderOpts.DarkThemeID = input.Opts.DarkThemeID
	}
	if input.Opts != nil && input.Opts.Scale != nil {
		renderOpts.Scale = input.Opts.Scale
	}
	if input.Opts != nil && input.Opts.NoXMLTag != nil {
		renderOpts.NoXMLTag = input.Opts.NoXMLTag
	}

	forceAppendix := input.Opts != nil && input.Opts.ForceAppendix != nil && *input.Opts.ForceAppendix

	var boards [][]byte
	if noChildren {
		var board []byte
		board, err = renderSingleBoard(renderOpts, forceAppendix, ruler, diagram)
		boards = [][]byte{board}
	} else {
		boards, err = renderBoards(renderOpts, forceAppendix, ruler, diagram)
	}
	if err != nil {
		return nil, &WASMError{Message: fmt.Sprintf("render failed: %s", err.Error()), Code: 500}
	}

	var out []byte
	if len(boards) > 0 {
		out = boards[0]
		if animateInterval > 0 {
			out, err = d2animate.Wrap(diagram, boards, *renderOpts, animateInterval)
			if err != nil {
				return nil, &WASMError{Message: fmt.Sprintf("animation failed: %s", err.Error()), Code: 500}
			}
		}
	}
	return out, nil
}

func renderSingleBoard(opts *d2svg.RenderOpts, forceAppendix bool, ruler *textmeasure.Ruler, diagram *d2target.Diagram) ([]byte, error) {
	out, err := d2svg.Render(diagram, opts)
	if err != nil {
		return nil, &WASMError{Message: fmt.Sprintf("render failed: %s", err.Error()), Code: 500}
	}
	if forceAppendix {
		out = appendix.Append(diagram, opts, ruler, out)
	}
	return out, nil
}

func renderBoards(opts *d2svg.RenderOpts, forceAppendix bool, ruler *textmeasure.Ruler, diagram *d2target.Diagram) ([][]byte, error) {
	var boards [][]byte
	for _, dl := range diagram.Layers {
		childrenBoards, err := renderBoards(opts, forceAppendix, ruler, dl)
		if err != nil {
			return nil, err
		}
		boards = append(boards, childrenBoards...)
	}
	for _, dl := range diagram.Scenarios {
		childrenBoards, err := renderBoards(opts, forceAppendix, ruler, dl)
		if err != nil {
			return nil, err
		}
		boards = append(boards, childrenBoards...)
	}
	for _, dl := range diagram.Steps {
		childrenBoards, err := renderBoards(opts, forceAppendix, ruler, dl)
		if err != nil {
			return nil, err
		}
		boards = append(boards, childrenBoards...)
	}

	if !diagram.IsFolderOnly {
		out, err := renderSingleBoard(opts, forceAppendix, ruler, diagram)
		if err != nil {
			return boards, err
		}
		boards = append([][]byte{out}, boards...)
	}
	return boards, nil
}

func GetBoardAtPosition(args []js.Value) (interface{}, error) {
	if len(args) < 3 {
		return nil, &WASMError{Message: "missing required arguments", Code: 400}
	}

	dsl := args[0].String()
	line := args[1].Int()
	column := args[2].Int()

	boardPath, err := d2lsp.GetBoardAtPosition(dsl, d2ast.Position{
		Line:   line,
		Column: column,
	})
	if err != nil {
		return nil, &WASMError{Message: err.Error(), Code: 500}
	}

	return BoardPositionResponse{BoardPath: boardPath}, nil
}

func Encode(args []js.Value) (interface{}, error) {
	if len(args) < 1 {
		return nil, &WASMError{Message: "missing script argument", Code: 400}
	}

	script := args[0].String()
	encoded, err := urlenc.Encode(script)
	// should never happen
	if err != nil {
		return nil, &WASMError{Message: err.Error(), Code: 500}
	}

	return map[string]string{"result": encoded}, nil
}

func Decode(args []js.Value) (interface{}, error) {
	if len(args) < 1 {
		return nil, &WASMError{Message: "missing script argument", Code: 400}
	}

	script := args[0].String()
	script, err := urlenc.Decode(script)
	if err != nil {
		return nil, &WASMError{Message: err.Error(), Code: 500}
	}
	return map[string]string{"result": script}, nil
}

func GetVersion(args []js.Value) (interface{}, error) {
	return version.Version, nil
}

func GetCompletions(args []js.Value) (interface{}, error) {
	if len(args) < 3 {
		return nil, &WASMError{Message: "missing required arguments", Code: 400}
	}

	text := args[0].String()
	line := args[1].Int()
	column := args[2].Int()

	completions, err := d2lsp.GetCompletionItems(text, line, column)
	if err != nil {
		return nil, &WASMError{Message: err.Error(), Code: 500}
	}

	// Convert to map for JSON serialization
	items := make([]map[string]interface{}, len(completions))
	for i, completion := range completions {
		items[i] = map[string]interface{}{
			"label":      completion.Label,
			"kind":       int(completion.Kind),
			"detail":     completion.Detail,
			"insertText": completion.InsertText,
		}
	}

	return CompletionResponse{
		Items: items,
	}, nil
}
