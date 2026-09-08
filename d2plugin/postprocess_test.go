package d2plugin

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/util-go/xmain"
	"github.com/d2lang/util-go/xos"
)

const postProcessHelperEnv = "D2_POSTPROCESS_HELPER_PROCESS"

const (
	pluginInfoHelperEnv = "D2_PLUGIN_INFO_HELPER_PROCESS"
	pluginInfoMarkerEnv = "D2_PLUGIN_INFO_HELPER_MARKER"
)

func TestMain(m *testing.M) {
	if os.Getenv(pluginInfoHelperEnv) == "1" {
		if marker := os.Getenv(pluginInfoMarkerEnv); marker != "" {
			if err := os.WriteFile(marker, []byte("executed"), 0o600); err != nil {
				fmt.Fprint(os.Stderr, err)
				os.Exit(2)
			}
		}
		fmt.Fprint(os.Stdout, `{"name":"tala","features":[]}`)
		os.Exit(0)
	}
	if os.Getenv(postProcessHelperEnv) == "1" {
		if len(os.Args) != 2 || os.Args[1] != "postprocess" {
			fmt.Fprintf(os.Stderr, "unexpected helper arguments: %q", os.Args[1:])
			os.Exit(2)
		}
		in, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprint(os.Stderr, err)
			os.Exit(2)
		}
		if _, err := os.Stdout.Write(append([]byte("wire:"), in...)); err != nil {
			fmt.Fprint(os.Stderr, err)
			os.Exit(2)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type pluginWithoutWirePostProcess struct{}

func (*pluginWithoutWirePostProcess) Info(context.Context) (*PluginInfo, error) {
	return &PluginInfo{}, nil
}

func (*pluginWithoutWirePostProcess) Flags(context.Context) ([]PluginSpecificFlag, error) {
	return nil, nil
}

func (*pluginWithoutWirePostProcess) HydrateOpts([]byte) error {
	return nil
}

func (*pluginWithoutWirePostProcess) Layout(context.Context, *d2graph.Graph) error {
	return nil
}

type pluginWithWirePostProcess struct {
	pluginWithoutWirePostProcess
}

func (*pluginWithWirePostProcess) PostProcess(_ context.Context, in []byte) ([]byte, error) {
	return append([]byte("served:"), in...), nil
}

type bufferWriteCloser struct {
	bytes.Buffer
}

func (*bufferWriteCloser) Close() error {
	return nil
}

func TestServeRetainsPostProcessWireCommand(t *testing.T) {
	tests := []struct {
		name   string
		plugin Plugin
		want   string
	}{
		{
			name:   "plugin without postprocessor",
			plugin: &pluginWithoutWirePostProcess{},
			want:   "render",
		},
		{
			name:   "legacy postprocessor",
			plugin: &pluginWithWirePostProcess{},
			want:   "served:render",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := xmain.NewOpts(xos.NewEnv(nil), []string{"postprocess"})
			if err := opts.Flags.Parse(opts.Args); err != nil {
				t.Fatal(err)
			}

			out := &bufferWriteCloser{}
			ms := &xmain.State{
				Stdin:  bytes.NewBufferString("render"),
				Stdout: out,
				Opts:   opts,
			}
			if err := Serve(tt.plugin)(context.Background(), ms); err != nil {
				t.Fatal(err)
			}
			if got := out.String(); got != tt.want {
				t.Fatalf("postprocess output = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExecPluginRetainsPostProcessWireCommand(t *testing.T) {
	t.Setenv(postProcessHelperEnv, "1")

	p := &execPlugin{path: os.Args[0]}
	got, err := p.PostProcess(context.Background(), []byte("render"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "wire:render"; string(got) != want {
		t.Fatalf("PostProcess() = %q, want %q", got, want)
	}
}

var _ Plugin = (*pluginWithoutWirePostProcess)(nil)
var _ PostProcessor = (*pluginWithWirePostProcess)(nil)
