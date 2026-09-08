//go:build !noelk

package d2plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/d2layouts/d2elklayout"
)

func TestELKPluginHydrateOpts(t *testing.T) {
	want := d2elklayout.ConfigurableOpts{
		Algorithm:       "layered",
		NodeSpacing:     123,
		Padding:         "[top=11,left=22,bottom=33,right=44]",
		EdgeNodeSpacing: 67,
		EdgeEdgeSpacing: 78,
		SelfLoopSpacing: 89,
	}
	raw, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}

	p := &elkPlugin{}
	if err := p.HydrateOpts(raw); err != nil {
		t.Fatalf("hydrate ELK options: %v", err)
	}
	if p.opts == nil {
		t.Fatal("hydrated options are nil")
	}
	if *p.opts != want {
		t.Fatalf("hydrated options = %#v, want %#v", *p.opts, want)
	}
}

func TestELKPluginHydrateOptsRejectsNonELKJSON(t *testing.T) {
	p := &elkPlugin{}
	if err := p.HydrateOpts([]byte(`{"elk.padding":42}`)); err == nil {
		t.Fatal("expected invalid ELK options to fail")
	}
}

func TestELKPluginConcurrentMetadataAndLayout(t *testing.T) {
	var p Plugin = &elkPlugin{}
	ctx := context.Background()
	options := make([][]byte, 2)
	for i := range options {
		opts := d2elklayout.DefaultOpts
		opts.NodeSpacing += i
		var err error
		options[i], err = json.Marshal(opts)
		if err != nil {
			t.Fatal(err)
		}
	}

	checks := []func(int) error{
		func(i int) error { return p.HydrateOpts(options[i%len(options)]) },
		func(int) error {
			flags, err := p.Flags(ctx)
			if err != nil {
				return err
			}
			info, err := p.Info(ctx)
			if err != nil {
				return err
			}
			if len(flags) == 0 || info.Name != "elk" {
				return fmt.Errorf("unexpected plugin metadata: %v, %v", flags, info)
			}
			return nil
		},
		func(int) error { return p.Layout(ctx, d2graph.NewGraph()) },
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	for _, check := range checks {
		wg.Add(1)
		go func(check func(int) error) {
			defer wg.Done()
			<-start
			for i := 0; i < 100; i++ {
				if err := check(i); err != nil {
					t.Error(err)
					return
				}
			}
		}(check)
	}
	close(start)
	wg.Wait()
}
