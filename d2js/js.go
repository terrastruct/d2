//go:build wasm

package main

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"strings"
	"syscall/js"

	"oss.terrastruct.com/d2/d2ast"
	"oss.terrastruct.com/d2/d2compiler"
	"oss.terrastruct.com/d2/d2format"
	"oss.terrastruct.com/d2/d2lsp"
	"oss.terrastruct.com/d2/d2oracle"
	"oss.terrastruct.com/d2/d2parser"
	"oss.terrastruct.com/d2/lib/urlenc"
	"oss.terrastruct.com/d2/lib/version"
)

func main() {
	js.Global().Set("d2GetParentID", js.FuncOf(jsGetParentID))
	js.Global().Set("d2GetObjOrder", js.FuncOf(jsGetObjOrder))
	js.Global().Set("d2GetRefRanges", js.FuncOf(jsGetRefRanges))
	js.Global().Set("d2GetImportRanges", js.FuncOf(jsGetImportRanges))
	js.Global().Set("d2Compile", js.FuncOf(jsCompile))
	js.Global().Set("d2Parse", js.FuncOf(jsParse))
	js.Global().Set("d2Encode", js.FuncOf(jsEncode))
	js.Global().Set("d2Decode", js.FuncOf(jsDecode))
	js.Global().Set("d2Version", js.FuncOf(jsVersion))
	initCallback := js.Global().Get("onWasmInitialized")
	if !initCallback.IsUndefined() {
		initCallback.Invoke()
	}
	select {}
}

type jsObjOrder struct {
	Order []string `json:"order"`
	Error string   `json:"error"`
}

func jsGetObjOrder(this js.Value, args []js.Value) interface{} {
	dsl := args[0].String()

	g, _, err := d2compiler.Compile("", strings.NewReader(dsl), &d2compiler.CompileOptions{
		UTF16Pos: true,
	})
	if err != nil {
		ret := jsObjOrder{Error: err.Error()}
		str, _ := json.Marshal(ret)
		return string(str)
	}

	objOrder, err := d2oracle.GetObjOrder(g, nil)
	if err != nil {
		ret := jsObjOrder{Error: err.Error()}
		str, _ := json.Marshal(ret)
		return string(str)
	}
	resp := jsObjOrder{
		Order: objOrder,
	}

	str, _ := json.Marshal(resp)
	return string(str)
}

func jsGetParentID(this js.Value, args []js.Value) interface{} {
	id := args[0].String()

	mk, _ := d2parser.ParseMapKey(id)

	if len(mk.Edges) > 0 {
		return ""
	}

	if mk.Key != nil {
		if len(mk.Key.Path) == 1 {
			return "root"
		}
		mk.Key.Path = mk.Key.Path[:len(mk.Key.Path)-1]
		return strings.Join(mk.Key.IDA(), ".")
	}

	return ""
}

type jsRefRanges struct {
	Ranges     []d2ast.Range `json:"ranges"`
	ParseError string        `json:"parseError"`
	UserError  string        `json:"userError"`
	D2Error    string        `json:"d2Error"`
}

func jsGetRefRanges(this js.Value, args []js.Value) interface{} {
	fsRaw := args[0].String()
	file := args[1].String()
	key := args[2].String()
	boardPathRaw := args[3].String()

	var fs map[string]string
	err := json.Unmarshal([]byte(fsRaw), &fs)
	if err != nil {
		ret := jsRefRanges{D2Error: err.Error()}
		str, _ := json.Marshal(ret)
		return string(str)
	}

	_, err = d2parser.ParseMapKey(key)
	if err != nil {
		ret := jsRefRanges{D2Error: err.Error()}
		str, _ := json.Marshal(ret)
		return string(str)
	}

	var boardPath []string
	err = json.Unmarshal([]byte(boardPathRaw), &boardPath)
	if err != nil {
		ret := jsRefRanges{D2Error: err.Error()}
		str, _ := json.Marshal(ret)
		return string(str)
	}

	refs, err := d2lsp.GetRefs(file, fs, boardPath, key)
	if err != nil {
		ret := jsRefRanges{D2Error: err.Error()}
		str, _ := json.Marshal(ret)
		return string(str)
	}

	var ranges []d2ast.Range

	for _, ref := range refs {
		ranges = append(ranges, ref.AST().GetRange())
	}

	resp := jsRefRanges{
		Ranges: ranges,
	}

	str, _ := json.Marshal(resp)
	return string(str)
}

func jsGetImportRanges(this js.Value, args []js.Value) interface{} {
	fsRaw := args[0].String()
	path := args[1].String()
	importPath := args[2].String()

	var fs map[string]string
	err := json.Unmarshal([]byte(fsRaw), &fs)
	if err != nil {
		ret := jsRefRanges{D2Error: err.Error()}
		str, _ := json.Marshal(ret)
		return string(str)
	}

	ranges, err := d2lsp.GetImportRanges(path, fs, importPath)
	if err != nil {
		ret := jsRefRanges{D2Error: err.Error()}
		str, _ := json.Marshal(ret)
		return string(str)
	}

	resp := jsRefRanges{
		Ranges: ranges,
	}

	str, _ := json.Marshal(resp)
	return string(str)
}

type jsObject struct {
	Result    string `json:"result"`
	UserError string `json:"userError"`
	D2Error   string `json:"d2Error"`
}

type jsParseResponse struct {
	DSL        string `json:"dsl"`
	ParseError string `json:"parseError"`
	UserError  string `json:"userError"`
	D2Error    string `json:"d2Error"`
}

type emptyFile struct{}

func (f *emptyFile) Stat() (os.FileInfo, error) {
	return nil, nil
}

func (f *emptyFile) Read(p []byte) (int, error) {
	return 0, io.EOF
}

func (f *emptyFile) Close() error {
	return nil
}

type detectFS struct {
	importUsed bool
}

func (detectFS *detectFS) Open(name string) (fs.File, error) {
	detectFS.importUsed = true
	return &emptyFile{}, nil
}

func jsParse(this js.Value, args []js.Value) interface{} {
	dsl := args[0].String()
	themeID := args[1].Int()

	detectFS := detectFS{}

	g, _, err := d2compiler.Compile("", strings.NewReader(dsl), &d2compiler.CompileOptions{
		UTF16Pos: true,
		FS:       &detectFS,
	})
	// If an import was used, client side D2 cannot reliably compile
	// Defer to backend compilation
	if !detectFS.importUsed {
		var pe *d2parser.ParseError
		if err != nil {
			if errors.As(err, &pe) {
				serialized, _ := json.Marshal(err)
				ret := jsParseResponse{ParseError: string(serialized)}
				str, _ := json.Marshal(ret)
				return string(str)
			}
			ret := jsParseResponse{D2Error: err.Error()}
			str, _ := json.Marshal(ret)
			return string(str)
		}

		for _, o := range g.Objects {
			if (o.Attributes.Top == nil) != (o.Attributes.Left == nil) {
				ret := jsParseResponse{UserError: `keywords "top" and "left" currently must be used together`}
				str, _ := json.Marshal(ret)
				return string(str)
			}
		}

		err = g.ApplyTheme(int64(themeID))
		if err != nil {
			ret := jsParseResponse{D2Error: err.Error()}
			str, _ := json.Marshal(ret)
			return string(str)
		}
	}

	m, err := d2parser.Parse("", strings.NewReader(dsl), &d2parser.ParseOptions{
		UTF16Pos: true,
	})
	if err != nil {
		return err
	}

	resp := jsParseResponse{}

	newDSL := d2format.Format(m)
	if dsl != newDSL {
		resp.DSL = newDSL
	}

	str, _ := json.Marshal(resp)
	return string(str)
}

// TODO error passing
// TODO recover panics
func jsCompile(this js.Value, args []js.Value) interface{} {
	script := args[0].String()

	g, _, err := d2compiler.Compile("", strings.NewReader(script), &d2compiler.CompileOptions{
		UTF16Pos: true,
	})
	var pe *d2parser.ParseError
	if err != nil {
		if errors.As(err, &pe) {
			serialized, _ := json.Marshal(err)
			ret := jsObject{UserError: string(serialized)}
			str, _ := json.Marshal(ret)
			return string(str)
		}
		ret := jsObject{D2Error: err.Error()}
		str, _ := json.Marshal(ret)
		return string(str)
	}

	newScript := d2format.Format(g.AST)
	if script != newScript {
		ret := jsObject{Result: newScript}
		str, _ := json.Marshal(ret)
		return string(str)
	}

	return nil
}

func jsEncode(this js.Value, args []js.Value) interface{} {
	script := args[0].String()

	encoded, err := urlenc.Encode(script)
	// should never happen
	if err != nil {
		ret := jsObject{D2Error: err.Error()}
		str, _ := json.Marshal(ret)
		return string(str)
	}

	ret := jsObject{Result: encoded}
	str, _ := json.Marshal(ret)
	return string(str)
}

func jsDecode(this js.Value, args []js.Value) interface{} {
	script := args[0].String()

	script, err := urlenc.Decode(script)
	if err != nil {
		ret := jsObject{UserError: err.Error()}
		str, _ := json.Marshal(ret)
		return string(str)
	}

	ret := jsObject{Result: script}
	str, _ := json.Marshal(ret)
	return string(str)
}

func jsVersion(this js.Value, args []js.Value) interface{} {
	return version.Version
}