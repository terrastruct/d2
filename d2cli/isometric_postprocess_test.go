package d2cli

import (
	"context"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2svg"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/util-go/go2"
)

type isometricTestPostProcessor struct {
	pluginWithoutPostProcess
	calls    int
	mutateAt int
}

func (p *isometricTestPostProcessor) PostProcess(_ context.Context, data []byte) ([]byte, error) {
	p.calls++
	if p.calls == p.mutateAt {
		data[0] = '!'
	}
	return data, nil
}

func TestIsometricPostProcessorChecksEveryBoardBeforeExport(t *testing.T) {
	root := simpleRasterDiagram()
	root.Layers = []*d2target.Diagram{simpleRasterDiagram()}
	for _, mutate := range []int{0, 2} {
		plugin := &isometricTestPostProcessor{mutateAt: mutate}
		err := validateIsometricPostProcessor(context.Background(), plugin, root, d2svg.RenderOpts{Isometric: go2.Pointer(true)})
		if plugin.calls != 2 || mutate == 0 && err != nil || mutate > 0 && (err == nil || !strings.Contains(err.Error(), "postprocessor")) {
			t.Fatalf("mutateAt=%d, calls=%d, error=%v", mutate, plugin.calls, err)
		}
	}
	if err := validateIsometricPostProcessor(context.Background(), &pluginWithoutPostProcess{}, nil, d2svg.RenderOpts{}); err != nil {
		t.Fatal(err)
	}
}
