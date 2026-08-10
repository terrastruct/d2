//go:build !noelk

package d2plugin

import (
	"encoding/json"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2elklayout"
)

func TestELKPluginHydrateOpts(t *testing.T) {
	want := d2elklayout.ConfigurableOpts{
		Algorithm:       "layered",
		NodeSpacing:     123,
		Padding:         "[top=11,left=22,bottom=33,right=44]",
		EdgeNodeSpacing: 67,
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
