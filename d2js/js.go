//go:build wasm

package main

import (
	"encoding/json"
	"errors"
	"strings"
	"syscall/js"

	"oss.terrastruct.com/d2/d2compiler"
	"oss.terrastruct.com/d2/d2format"
	"oss.terrastruct.com/d2/d2parser"
	"oss.terrastruct.com/d2/lib/urlenc"
)

func main() {
	done := make(chan struct{}, 0)
	js.Global().Set("d2Compile", js.FuncOf(jsCompile))
	js.Global().Set("d2Encode", js.FuncOf(jsEncode))
	js.Global().Set("d2Decode", js.FuncOf(jsDecode))
	<-done
}

type jsObject struct {
	Result    string `json:"result"`
	UserError string `json:"userError"`
	D2Error   string `json:"d2Error"`
}

// TODO error passing
// TODO recover panics
func jsCompile(this js.Value, args []js.Value) interface{} {
	script := args[0].String()

	g, err := d2compiler.Compile("", strings.NewReader(script), &d2compiler.CompileOptions{
		UTF16: true,
	})
	var pe d2parser.ParseError
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
