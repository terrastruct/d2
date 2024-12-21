//go:build js && wasm

package d2wasm

import "oss.terrastruct.com/d2/d2ast"

type WASMResponse struct {
	Data  interface{} `json:"data,omitempty"`
	Error *WASMError  `json:"error,omitempty"`
}

type WASMError struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

func (e *WASMError) Error() string {
	return e.Message
}

type RefRangesResponse struct {
	Ranges       []d2ast.Range `json:"ranges"`
	ImportRanges []d2ast.Range `json:"importRanges"`
}

type BoardPositionResponse struct {
	BoardPath []string `json:"boardPath"`
}
