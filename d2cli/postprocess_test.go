package d2cli

import (
	"context"
	"testing"

	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/d2plugin"
)

type pluginWithoutPostProcess struct{}

func (*pluginWithoutPostProcess) Info(context.Context) (*d2plugin.PluginInfo, error) {
	return &d2plugin.PluginInfo{}, nil
}

func (*pluginWithoutPostProcess) Flags(context.Context) ([]d2plugin.PluginSpecificFlag, error) {
	return nil, nil
}

func (*pluginWithoutPostProcess) HydrateOpts([]byte) error {
	return nil
}

func (*pluginWithoutPostProcess) Layout(context.Context, *d2graph.Graph) error {
	return nil
}

type pluginWithPostProcess struct {
	pluginWithoutPostProcess
	called bool
}

func (p *pluginWithPostProcess) PostProcess(_ context.Context, in []byte) ([]byte, error) {
	p.called = true
	return append([]byte("processed:"), in...), nil
}

func TestPostProcessIsOptional(t *testing.T) {
	t.Run("plugin without postprocessor", func(t *testing.T) {
		in := []byte("render")
		got, err := postProcess(context.Background(), &pluginWithoutPostProcess{}, in)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(in) {
			t.Fatalf("postProcess() = %q, want %q", got, in)
		}
	})

	t.Run("optional postprocessor", func(t *testing.T) {
		plugin := &pluginWithPostProcess{}
		got, err := postProcess(context.Background(), plugin, []byte("render"))
		if err != nil {
			t.Fatal(err)
		}
		if !plugin.called {
			t.Fatal("PostProcess was not called")
		}
		if want := "processed:render"; string(got) != want {
			t.Fatalf("postProcess() = %q, want %q", got, want)
		}
	})
}

var _ d2plugin.Plugin = (*pluginWithoutPostProcess)(nil)
var _ d2plugin.PostProcessor = (*pluginWithPostProcess)(nil)
