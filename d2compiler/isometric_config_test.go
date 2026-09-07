package d2compiler_test

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/d2lang/d2/d2compiler"
)

func TestIsometricConfig(t *testing.T) {
	for _, test := range []struct {
		name, source string
		want         bool
		wantErr      string
	}{
		{"true", "vars: {d2-config: {isometric: true}}\na", true, ""},
		{"false", "vars: {d2-config: {isometric: false}}\na", false, ""},
		{"substitution", "vars: {mode: true; d2-config: {isometric: ${mode}}}\na", true, ""},
		{"invalid", "vars: {d2-config: {isometric: sideways}}\na", false, `expected a boolean for "isometric", got "sideways"`},
		{"missing", "vars: {d2-config: {isometric}}\na", false, `"isometric" needs a value`},
		{"nested", "a: {vars: {d2-config: {isometric: true}}}", false, `"d2-config" can only appear at root vars`},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, config, err := d2compiler.Compile("input.d2", strings.NewReader(test.source), nil)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("error = %v, want %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if config == nil || config.Isometric == nil || *config.Isometric != test.want {
				t.Fatalf("config = %+v, want isometric %t", config, test.want)
			}
		})
	}
}

func TestIsometricConfigFromImport(t *testing.T) {
	fs := fstest.MapFS{"appearance.d2": &fstest.MapFile{Data: []byte("vars: {d2-config: {isometric: true}}")}}
	_, config, err := d2compiler.Compile("input.d2", strings.NewReader("...@appearance\na -> b"), &d2compiler.CompileOptions{FS: fs})
	if err != nil {
		t.Fatal(err)
	}
	if config == nil || config.Isometric == nil || !*config.Isometric {
		t.Fatal("imported source config did not select isometric rendering")
	}
}
