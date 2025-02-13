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
	"oss.terrastruct.com/d2/d2renderers/d2fonts"
	"oss.terrastruct.com/d2/d2renderers/d2svg"
	"oss.terrastruct.com/d2/d2renderers/d2svg/appendix"
	"oss.terrastruct.com/d2/lib/log"
	"oss.terrastruct.com/d2/lib/memfs"
	"oss.terrastruct.com/d2/lib/textmeasure"
	"oss.terrastruct.com/d2/lib/urlenc"
	"oss.terrastruct.com/d2/lib/version"
	"oss.terrastruct.com/util-go/go2"
)

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

	if _, ok := input.FS["index"]; !ok {
		return nil, &WASMError{Message: "missing 'index' file in input fs", Code: 400}
	}

	fs, err := memfs.New(input.FS)
	if err != nil {
		return nil, &WASMError{Message: fmt.Sprintf("invalid fs input: %s", err.Error()), Code: 400}
	}

	g, _, err := d2compiler.Compile("", strings.NewReader(input.FS["index"]), &d2compiler.CompileOptions{
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

	if _, ok := input.FS["index"]; !ok {
		return nil, &WASMError{Message: "missing 'index' file in input fs", Code: 400}
	}

	fs, err := memfs.New(input.FS)
	if err != nil {
		return nil, &WASMError{Message: fmt.Sprintf("invalid fs input: %s", err.Error()), Code: 400}
	}

	ruler, err := textmeasure.NewRuler()
	if err != nil {
		return nil, &WASMError{Message: fmt.Sprintf("text ruler cannot be initialized: %s", err.Error()), Code: 500}
	}
	ctx := log.WithDefault(context.Background())
	layoutFunc := d2dagrelayout.DefaultLayout
	if input.Opts != nil && input.Opts.Layout != nil {
		switch *input.Opts.Layout {
		case "dagre":
			layoutFunc = d2dagrelayout.DefaultLayout
		case "elk":
			layoutFunc = d2elklayout.DefaultLayout
		default:
			return nil, &WASMError{Message: fmt.Sprintf("layout option '%s' not recognized", *input.Opts.Layout), Code: 400}
		}
	}
	layoutResolver := func(engine string) (d2graph.LayoutGraph, error) {
		return layoutFunc, nil
	}

	renderOpts := &d2svg.RenderOpts{}
	var fontFamily *d2fonts.FontFamily
	if input.Opts != nil && input.Opts.Sketch != nil && *input.Opts.Sketch {
		fontFamily = go2.Pointer(d2fonts.HandDrawn)
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
	diagram, g, err := d2lib.Compile(ctx, input.FS["index"], &d2lib.CompileOptions{
		UTF16Pos:       true,
		FS:             fs,
		Ruler:          ruler,
		LayoutResolver: layoutResolver,
		FontFamily:     fontFamily,
	}, renderOpts)
	if err != nil {
		if pe, ok := err.(*d2parser.ParseError); ok {
			errs, _ := json.Marshal(pe.Errors)
			return nil, &WASMError{Message: string(errs), Code: 400}
		}
		return nil, &WASMError{Message: err.Error(), Code: 500}
	}

	input.FS["index"] = d2format.Format(g.AST)

	return CompileResponse{
		FS:      input.FS,
		Diagram: *diagram,
		Graph:   *g,
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

	ruler, err := textmeasure.NewRuler()
	if err != nil {
		return nil, &WASMError{Message: fmt.Sprintf("text ruler cannot be initialized: %s", err.Error()), Code: 500}
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
	out, err := d2svg.Render(input.Diagram, renderOpts)
	if err != nil {
		return nil, &WASMError{Message: fmt.Sprintf("render failed: %s", err.Error()), Code: 500}
	}
	if input.Opts != nil && *input.Opts.ForceAppendix {
		out = appendix.Append(input.Diagram, renderOpts, ruler, out)
	}

	return out, nil
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
