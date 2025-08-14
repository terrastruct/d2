//go:build !js || !wasm

package d2wasm

const DEFAULT_INPUT_PATH = "index"

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"syscall/js"

	"oss.terrastruct.com/d2/d2ast"
	"oss.terrastruct.com/d2/d2compiler"
	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2layouts/d2elklayout"
	"oss.terrastruct.com/d2/d2lsp"
	"oss.terrastruct.com/d2/d2oracle"
	"oss.terrastruct.com/d2/d2parser"
	"oss.terrastruct.com/d2/lib/memfs"
	"oss.terrastruct.com/d2/lib/textmeasure"
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
	err = g.SetDimensions(nil, ruler, nil, nil)
	if err != nil {
		return nil, err
	}

	elk, err := d2elklayout.ConvertGraph(context.Background(), g, nil)
	if err != nil {
		return nil, &WASMError{Message: err.Error(), Code: 400}
	}
	return elk, nil
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
		return BoardPositionResponse{BoardPath: boardPath, Err: err.Error()}, nil
	}

	return BoardPositionResponse{BoardPath: boardPath}, nil
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