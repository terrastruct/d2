//go:build js && wasm

package d2wasm

import (
	"oss.terrastruct.com/d2/d2ast"
	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2target"
)

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

type CompileRequest struct {
	FS        map[string]string `json:"fs"`
	InputPath *string           `json:"inputPath"`
	Opts      *CompileOptions   `json:"options"`
}

type RenderOptions struct {
	Pad             *int64   `json:"pad"`
	Sketch          *bool    `json:"sketch"`
	Center          *bool    `json:"center"`
	ThemeID         *int64   `json:"themeID"`
	DarkThemeID     *int64   `json:"darkThemeID"`
	Scale           *float64 `json:"scale"`
	ForceAppendix   *bool    `json:"forceAppendix"`
	Target          *string  `json:"target"`
	AnimateInterval *int64   `json:"animateInterval"`
	Salt            *string  `json:"salt"`
	NoXMLTag        *bool    `json:"noXMLTag"`
}

type CompileOptions struct {
	RenderOptions
	Layout       *string `json:"layout"`
	FontRegular  *[]byte `json:"FontRegular"`
	FontItalic   *[]byte `json:"FontItalic"`
	FontBold     *[]byte `json:"FontBold"`
	FontSemibold *[]byte `json:"FontSemibold"`
}

type CompileResponse struct {
	FS            map[string]string `json:"fs"`
	InputPath     string            `json:"inputPath"`
	Diagram       d2target.Diagram  `json:"diagram"`
	Graph         d2graph.Graph     `json:"graph"`
	RenderOptions RenderOptions     `json:"renderOptions"`
}

type CompletionResponse struct {
	Items []map[string]interface{} `json:"items"`
}

type RenderRequest struct {
	Diagram *d2target.Diagram `json:"diagram"`
	Opts    *RenderOptions    `json:"options"`
}
