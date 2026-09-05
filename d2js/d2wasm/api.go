//go:build js && wasm

package d2wasm

import (
	"encoding/json"
	"fmt"
	"runtime/debug"
	"sync"
	"syscall/js"
)

type D2API struct {
	exports map[string]js.Func
}

func NewD2API() *D2API {
	return &D2API{
		exports: make(map[string]js.Func),
	}
}

func (api *D2API) Register(name string, fn func(args []js.Value) (interface{}, error)) {
	api.exports[name] = wrapWASMCall(fn)
}

func warnDeprecatedWASMCall(once *sync.Once, message string) {
	once.Do(func() {
		// A consumer may replace console.warn with a function that throws. A
		// deprecation notice must never change the legacy call's response.
		defer func() {
			_ = recover()
		}()

		console := js.Global().Get("console")
		if console.Type() != js.TypeObject {
			return
		}
		warn := console.Get("warn")
		if warn.Type() != js.TypeFunction {
			return
		}
		warn.Invoke(message)
	})
}

func (api *D2API) ExportTo(target js.Value) {
	d2Namespace := make(map[string]interface{})
	for name, fn := range api.exports {
		d2Namespace[name] = fn
	}
	target.Set("d2", js.ValueOf(d2Namespace))
}

func wrapWASMCall(fn func(args []js.Value) (interface{}, error)) js.Func {
	return js.FuncOf(func(this js.Value, args []js.Value) (result any) {
		defer func() {
			if r := recover(); r != nil {
				resp := WASMResponse{
					Error: &WASMError{
						Message: fmt.Sprintf("panic recovered: %v\n%s", r, debug.Stack()),
						Code:    500,
					},
				}
				jsonResp, _ := json.Marshal(resp)
				result = string(jsonResp)
			}
		}()

		data, err := fn(args)
		if err != nil {
			wasmErr, ok := err.(*WASMError)
			if !ok {
				wasmErr = &WASMError{
					Message: err.Error(),
					Code:    500,
				}
			}
			resp := WASMResponse{
				Error: wasmErr,
			}
			jsonResp, _ := json.Marshal(resp)
			return string(jsonResp)
		}

		resp := WASMResponse{
			Data: data,
		}
		jsonResp, _ := json.Marshal(resp)
		return string(jsonResp)
	})
}
