//go:build js && wasm

package main

import (
	"syscall/js"

	"oss.terrastruct.com/d2/d2js/d2wasm"
)

func main() {
	api := d2wasm.NewD2API()

	// Only register functions that are used by JS/WASM builds
	api.Register("compile", d2wasm.Compile)
	api.Register("render", d2wasm.Render)
	api.Register("encode", d2wasm.Encode)
	api.Register("decode", d2wasm.Decode)
	api.Register("version", d2wasm.GetVersion)
	api.Register("jsVersion", d2wasm.GetJSVersion)

	api.ExportTo(js.Global())

	if cb := js.Global().Get("onWasmInitialized"); !cb.IsUndefined() {
		cb.Invoke()
	}
	select {}
}
