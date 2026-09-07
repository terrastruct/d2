package d2cli

import (
	"context"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2plugin"
)

func TestTALARouterResolverUsesBundledRouter(t *testing.T) {
	ctx := context.Background()
	resolve := RouterResolver(ctx, nil, []d2plugin.Plugin{&d2plugin.TALAPlugin})
	router, err := resolve("tala")
	if err != nil {
		t.Fatal(err)
	}
	if router == nil {
		t.Fatal("TALA router resolver returned nil")
	}
	err = router(ctx, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "tala requires a D2 graph with a root object") {
		t.Fatalf("TALA router error = %v, want native adapter validation", err)
	}
}
