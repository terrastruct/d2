package fontfallback

import (
	"context"
	"errors"
	"testing"
	"time"
)

type cancelAfterChecksContext struct {
	remaining int
}

func (c *cancelAfterChecksContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *cancelAfterChecksContext) Done() <-chan struct{}       { return nil }
func (c *cancelAfterChecksContext) Value(any) any               { return nil }

func (c *cancelAfterChecksContext) Err() error {
	if c.remaining == 0 {
		return context.Canceled
	}
	c.remaining--
	return nil
}

func TestPrepareTrustedCancellationDoesNotPublish(t *testing.T) {
	cache := NewSceneCache(nil)
	source := Font{MIMEType: "font/ttf", Data: make([]byte, 3*32*1024)}
	ctx := &cancelAfterChecksContext{remaining: 2}

	if _, _, err := cache.PrepareTrusted(ctx, source); !errors.Is(err, context.Canceled) {
		t.Fatalf("PrepareTrusted error = %v, want context.Canceled", err)
	}
	if cache.HasTrusted() {
		t.Fatal("canceled preparation published a trusted resource")
	}
	if stats := cache.stats; stats != (CacheStats{}) {
		t.Fatalf("canceled preparation accounting = %+v, want zero", stats)
	}
}

func TestPrepareTrustedCopiesAndReusesStableAsset(t *testing.T) {
	cache := NewSceneCache(nil)
	source := Font{Name: "fallback.ttf", MIMEType: "font/ttf", Data: []byte{1, 2, 3, 4}, FaceIndex: 2}
	wantID, err := AssetID(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}

	first, added, err := cache.PrepareTrusted(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if !added || !first.Shared || first.ID != wantID {
		t.Fatalf("first preparation = %+v, added=%v; want shared resource %q", first, added, wantID)
	}
	source.Data[0] = 99
	if first.Data[0] != 1 {
		t.Fatal("trusted resource aliases mutable source data")
	}

	second, added, err := cache.PrepareTrusted(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if added || second.ID != first.ID || &second.Data[0] != &first.Data[0] {
		t.Fatalf("second preparation = %+v, added=%v; want reused stable resource", second, added)
	}
	if stats := cache.stats; stats.Assets != 1 || stats.Hashes != 1 || stats.Copies != 1 || stats.CopiedBytes != 4 {
		t.Fatalf("trusted cache accounting = %+v, want one asset/hash/copy", stats)
	}
}
