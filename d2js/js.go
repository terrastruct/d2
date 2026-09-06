//go:build js && wasm

package main

import (
	"syscall/js"

	"github.com/d2lang/d2/d2js/d2wasm"
)

func main() {
	api := d2wasm.NewD2API()

	api.Register("getCompletions", d2wasm.GetCompletions)
	api.Register("getParentID", d2wasm.GetParentID)
	// Deprecated: retained for raw WASM callers for one compatibility release.
	api.Register("getObjOrder", d2wasm.GetObjOrder)
	api.Register("getRefRanges", d2wasm.GetRefRanges)
	// Deprecated: retained for raw WASM callers for one compatibility release.
	api.Register("getELKGraph", d2wasm.GetELKGraph)
	api.Register("compile", d2wasm.Compile)
	api.Register("render", d2wasm.Render)
	api.Register("getBoardAtPosition", d2wasm.GetBoardAtPosition)
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
