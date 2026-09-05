//go:build js && wasm

package d2wasm

import (
	"encoding/json"
	"testing"
)

func TestSequenceV2WASMCompile(t *testing.T) {
	call := wrapWASMCall(Compile)
	defer call.Release()
	request, err := json.Marshal(CompileRequest{Opts: &CompileOptions{}, FS: map[string]string{"index": `shape: sequence-diagram
vars: {mirror: true; numbered: true}
request: {shape: edge-group; a -> b.work: Start; b.work -> a: Done}
b.work: Processing
`}})
	if err != nil {
		t.Fatal(err)
	}
	got := call.Invoke(string(request)).String()
	var response struct {
		Data  CompileResponse `json:"data"`
		Error *WASMError      `json:"error"`
	}
	if err := json.Unmarshal([]byte(got), &response); err != nil {
		t.Fatal(err)
	}
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	shapes := make(map[string]bool)
	for _, s := range response.Data.Diagram.Shapes {
		shapes[s.ID] = true
	}
	for _, id := range []string{"a", "b", "b.work", "a.mirror", "b.mirror", "request"} {
		if !shapes[id] {
			t.Errorf("missing shape %q", id)
		}
	}
	messages := 0
	for _, c := range response.Data.Diagram.Connections {
		if c.Label == "1. Start" || c.Label == "2. Done" {
			messages++
		}
	}
	if messages != 2 {
		t.Errorf("numbered messages = %d, want 2", messages)
	}
}
