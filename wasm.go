//go:build wasm

package main

import (
	"strings"
	"syscall/js"

	"oss.terrastruct.com/d2/d2compiler"
	"oss.terrastruct.com/d2/d2format"
	"oss.terrastruct.com/d2/lib/urlenc"
)

func main() {
	done := make(chan struct{}, 0)
	js.Global().Set("d2Compile", js.FuncOf(jsCompile))
	js.Global().Set("d2Encode", js.FuncOf(jsEncode))
	<-done
}

// TODO error passing
func jsCompile(this js.Value, args []js.Value) interface{} {
	script := args[0].String()

	g, err := d2compiler.Compile("", strings.NewReader(script), &d2compiler.CompileOptions{
		UTF16: true,
	})
	if err != nil {
		return err
	}

	newScript := d2format.Format(g.AST)
	if script != newScript {
		return newScript
	}

	return nil
}

func jsEncode(this js.Value, args []js.Value) interface{} {
	script := args[0].String()

	encoded, err := urlenc.Encode(script)
	if err != nil {
		return err
	}

	return encoded
}
