package e2etests_cli

import (
	"bytes"
	"context"
	"encoding/xml"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/d2lang/util-go/xos"
)

func TestTALACLISelection(t *testing.T) {
	nonFiniteNumber := regexp.MustCompile(`(?i)(?:^|[^[:alnum:]])(?:nan|[+-]?inf(?:inity)?)(?:[^[:alnum:]]|$)`)
	embeddedBase64 := regexp.MustCompile(`(?i)(base64,)[a-z0-9+/=\r\n]+`)
	tests := []struct {
		name       string
		source     string
		imports    map[string]string
		args       []string
		envLayout  string
		wantPlugin string
	}{
		{
			name:       "flag selects tala",
			source:     "client -> service\n",
			args:       []string{"--layout=tala", "--tala-seeds=1"},
			wantPlugin: "tala",
		},
		{
			name:       "environment selects tala",
			source:     "client -> service\n",
			args:       []string{"--tala-seeds=1"},
			envLayout:  "tala",
			wantPlugin: "tala",
		},
		{
			name: "source selects tala",
			source: `vars: {
  d2-config: {
    layout-engine: tala
    data: {
      tala-seeds: [1]
    }
  }
}
client -> service
`,
			wantPlugin: "tala",
		},
		{
			name: "flag overrides source",
			source: `vars: {
  d2-config: {
    layout-engine: elk
  }
}
client -> service
`,
			args:       []string{"--layout=tala", "--tala-seeds=1"},
			wantPlugin: "tala",
		},
		{
			name: "non-tala flag overrides source",
			source: `vars: {
  d2-config: {
    layout-engine: tala
    data: {
      tala-seeds: [1]
    }
  }
}
client -> service
`,
			args:       []string{"--layout=elk"},
			wantPlugin: "elk",
		},
		{
			name: "source import and nested routing",
			source: `vars: {
  d2-config: {
    layout-engine: tala
    data: {
      tala-seeds: [1]
    }
  }
}
...@part.d2
`,
			imports: map[string]string{
				"part.d2": `container: {
  child
}
outside
container.child -> outside
`,
			},
			wantPlugin: "tala",
		},
		{
			name: "animation boards",
			source: `vars: {
  d2-config: {
    layout-engine: tala
    data: {
      tala-seeds: [1]
    }
  }
}
start -> middle
steps: {
  1: { middle -> review }
  2: { review -> done }
}
`,
			args:       []string{"--animate-interval=10"},
			wantPlugin: "tala",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			writeFile(t, directory, "index.d2", test.source)
			for name, source := range test.imports {
				writeFile(t, directory, name, source)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			environment := xos.NewEnv(nil)
			if test.envLayout != "" {
				environment.Setenv("D2_LAYOUT", test.envLayout)
			}
			args := append([]string{"--debug", "--omit-version"}, test.args...)
			args = append(args, "index.d2", "output.svg")
			state := testMain(directory, environment, args...)
			var stderr bytes.Buffer
			state.Stderr = &stderr
			state.Start(t, ctx)
			defer state.Cleanup(t)
			if err := state.Wait(ctx); err != nil {
				t.Fatalf("CLI compile failed: %v\nstderr:\n%s", err, stderr.String())
			}
			if !strings.Contains(stderr.String(), "using layout plugin "+test.wantPlugin+" (bundled)") {
				t.Fatalf("CLI plugin trace does not select %q:\n%s", test.wantPlugin, stderr.String())
			}

			svg := readFile(t, directory, "output.svg")
			var parsed any
			if err := xml.Unmarshal(svg, &parsed); err != nil {
				t.Fatalf("CLI output is not valid SVG XML: %v", err)
			}
			if nonFiniteNumber.Match(embeddedBase64.ReplaceAll(svg, []byte("$1"))) {
				t.Fatal("CLI output contains non-finite geometry")
			}
		})
	}
}

func TestTALACLIHelp(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "list", args: []string{"layout"}, want: "tala (bundled)"},
		{name: "details", args: []string{"layout", "tala"}, want: "TALA is D2's native layout and edge-routing engine"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			state := testMain(t.TempDir(), xos.NewEnv(nil), test.args...)
			var stdout bytes.Buffer
			state.Stdout = &stdout
			state.Start(t, ctx)
			defer state.Cleanup(t)
			if err := state.Wait(ctx); err != nil {
				t.Fatalf("CLI help failed: %v", err)
			}
			if !strings.Contains(stdout.String(), test.want) {
				t.Fatalf("CLI help does not contain %q:\n%s", test.want, stdout.String())
			}
		})
	}
}
