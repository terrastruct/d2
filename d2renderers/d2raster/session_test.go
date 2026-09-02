package d2raster

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/internal/fontface"
)

func TestRenderSessionExactHitsMissesAndCharges(t *testing.T) {
	fontData := sessionFontData(t)
	rasterData := encodeRasterPNG(t, image.NewNRGBA(image.Rect(0, 0, 2, 2)))
	document := sessionAssetDocument(map[d2scene.AssetID]d2scene.Asset{
		"font": d2scene.FontAsset{MIMEType: "font/ttf", Data: fontData},
		"raster": d2scene.RasterAsset{
			MIMEType: "image/png", Data: rasterData, PixelWidth: 2, PixelHeight: 2,
		},
	})
	fontCharge := cacheEntryCharge(int64(len(fontData)), "font/ttf")
	rasterCharge := cacheEntryCharge(2*2*4, "image/png")
	fontMemoCharge := memoEntryCharge(fontData, "font", "font/ttf")
	rasterMemoCharge := memoEntryCharge(rasterData, "raster", "image/png")
	totalCharge := fontCharge + rasterCharge + fontMemoCharge + rasterMemoCharge
	session := newTestRenderSession(t, RenderSessionOptions{
		MaxCacheEntries: 2, MaxCacheBytes: totalCharge, MaxConcurrentLoads: 2,
	})

	if _, err := session.Render(context.Background(), document, testOptions()); err != nil {
		t.Fatal(err)
	}
	assertSessionStats(t, session.Stats(), RenderSessionStats{
		Misses: 2, Entries: 2, Bytes: fontCharge + rasterCharge,
		MemoMisses: 2, Hashes: 2, MemoEntries: 2, MemoBytes: fontMemoCharge + rasterMemoCharge, RetainedBytes: totalCharge,
	})
	if _, err := renderSessionTestPNG(context.Background(), session, document, testOptions()); err != nil {
		t.Fatal(err)
	}
	assertSessionStats(t, session.Stats(), RenderSessionStats{
		Hits: 2, Misses: 2, Entries: 2, Bytes: fontCharge + rasterCharge,
		MemoHits: 2, MemoMisses: 2, Hashes: 2, MemoEntries: 2, MemoBytes: fontMemoCharge + rasterMemoCharge, RetainedBytes: totalCharge,
	})
}

func TestRenderSessionSkipsHashOnlyForPrivateBundledBacking(t *testing.T) {
	canonical := sessionFontData(t)
	resolver, err := d2fonts.NewBundledFallbackResolver(nil, d2fonts.BundledFallbackLimits{
		MaxRequestedRunes: 1, MaxBundledBytes: 8 << 20, MaxResolvedBytes: 8 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	emojiFonts, err := resolver.ResolveFallbacks(context.Background(), d2fonts.FallbackRequest{Runes: []rune("😀")})
	if err != nil || len(emojiFonts) != 1 {
		t.Fatalf("bundled fallback = %#v, %v", emojiFonts, err)
	}
	emojiCanonical, err := fontface.RegisterBundledNotoColorEmoji(emojiFonts[0].Data)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name       string
		data       []byte
		wantHashes uint64
	}{
		{name: "ordinary public registry backing", data: canonical, wantHashes: 1},
		{name: "ordinary byte-identical copy", data: append([]byte(nil), canonical...), wantHashes: 1},
		{name: "emoji process-owned backing", data: emojiCanonical, wantHashes: 0},
		{name: "emoji byte-identical copy", data: append([]byte(nil), emojiCanonical...), wantHashes: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			document := sessionAssetDocument(map[d2scene.AssetID]d2scene.Asset{
				"font": d2scene.FontAsset{MIMEType: "font/ttf", Data: test.data},
			})
			asset := document.Assets["font"].(d2scene.FontAsset)
			session := newTestRenderSession(t, RenderSessionOptions{
				MaxCacheEntries: 1, MaxCacheBytes: 16 << 20, MaxConcurrentLoads: 1,
			})
			if _, err := session.font(context.Background(), document, "font", asset); err != nil {
				t.Fatal(err)
			}
			stats := session.Stats()
			if stats.MemoMisses != 1 || stats.Hashes != test.wantHashes {
				t.Fatalf("stats = %+v, want one memo miss and %d hashes", stats, test.wantHashes)
			}
		})
	}
}

func TestRenderSessionPerDocumentLimitsStillApplyOnHits(t *testing.T) {
	rasterData := encodeRasterPNG(t, image.NewNRGBA(image.Rect(0, 0, 4, 4)))
	document := sessionAssetDocument(map[d2scene.AssetID]d2scene.Asset{
		"raster": d2scene.RasterAsset{
			MIMEType: "image/png", Data: rasterData, PixelWidth: 4, PixelHeight: 4,
		},
	})
	charge := cacheEntryCharge(4*4*4, "image/png")
	session := newTestRenderSession(t, RenderSessionOptions{MaxCacheEntries: 1, MaxCacheBytes: charge, MaxConcurrentLoads: 1})
	if _, err := session.Render(context.Background(), document, testOptions()); err != nil {
		t.Fatal(err)
	}

	decodedLimited := testOptions()
	decodedLimited.MaxDecodedAssetBytes = 63
	if _, err := session.Render(context.Background(), document, decodedLimited); err == nil || !strings.Contains(err.Error(), "requires 64 bytes") {
		t.Fatalf("cached decoded-byte limit error = %v", err)
	}
	stats := session.Stats()
	if stats.Hits != 1 || stats.Misses != 1 || stats.Entries != 1 {
		t.Fatalf("stats after cached decoded limit = %+v", stats)
	}

	encodedLimited := testOptions()
	encodedLimited.MaxAssetBytes = int64(len(rasterData)) - 1
	if _, err := session.Render(context.Background(), document, encodedLimited); err == nil || !strings.Contains(err.Error(), "retained asset bytes") {
		t.Fatalf("cached encoded-byte limit error = %v", err)
	}
	if got := session.Stats(); got != stats {
		t.Fatalf("encoded limit consulted cache: before=%+v after=%+v", stats, got)
	}
}

func TestRenderSessionChangedSameIDAndMetadataAreMisses(t *testing.T) {
	redData := encodeRasterPNG(t, uniformNRGBA(1, 1, color.NRGBA{R: 255, A: 255}))
	blueData := encodeRasterPNG(t, uniformNRGBA(1, 1, color.NRGBA{B: 255, A: 255}))
	newDocument := func(id d2scene.AssetID, data []byte, decodedBytes int64) *d2scene.Document {
		document := d2scene.NewDocument(d2scene.Box{Width: 1, Height: 1}, d2scene.NewNode(d2scene.Image{
			Asset: id, Box: d2scene.Box{Width: 1, Height: 1},
		}))
		document.Assets[id] = d2scene.RasterAsset{
			MIMEType: "image/png", Data: data, PixelWidth: 1, PixelHeight: 1, DecodedBytes: decodedBytes,
		}
		return document
	}
	session := newTestRenderSession(t, RenderSessionOptions{MaxCacheEntries: 8, MaxCacheBytes: 1 << 20, MaxConcurrentLoads: 2})

	sameDocument := newDocument("same", redData, 0)
	redFrame, err := session.Render(context.Background(), sameDocument, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	assertPixel(t, redFrame.NRGBAAt(0, 0), color.NRGBA{R: 255, A: 255})
	sameDocument.Assets["same"] = d2scene.RasterAsset{
		MIMEType: "image/png", Data: blueData, PixelWidth: 1, PixelHeight: 1,
	}
	blueFrame, err := session.Render(context.Background(), sameDocument, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	assertPixel(t, blueFrame.NRGBAAt(0, 0), color.NRGBA{B: 255, A: 255})
	sameDocument.Assets["same"] = d2scene.RasterAsset{
		MIMEType: "image/png", Data: blueData, PixelWidth: 1, PixelHeight: 1, DecodedBytes: 8,
	}
	if _, err := session.Render(context.Background(), sameDocument, testOptions()); err != nil {
		t.Fatal(err)
	}
	if got := session.Stats(); got.Misses != 3 || got.Hits != 0 || got.Entries != 3 || got.Hashes != 3 {
		t.Fatalf("changed-content/metadata stats = %+v", got)
	}

	// AssetID is deliberately absent from cache identity: identical content and
	// metadata under a different document-local name reuse the validated pixels.
	if _, err := session.Render(context.Background(), newDocument("other", blueData, 8), testOptions()); err != nil {
		t.Fatal(err)
	}
	if got := session.Stats(); got.Misses != 3 || got.Hits != 1 || got.Entries != 3 || got.Hashes != 4 {
		t.Fatalf("different-ID identical-content stats = %+v", got)
	}

	wrongType := sessionAssetDocument(map[d2scene.AssetID]d2scene.Asset{
		"same": d2scene.FontAsset{MIMEType: "font/ttf", Data: blueData},
	})
	if _, err := session.Render(context.Background(), wrongType, testOptions()); err == nil || !strings.Contains(err.Error(), "parse TrueType/OpenType font") {
		t.Fatalf("changed concrete type error = %v", err)
	}
	if got := session.Stats(); got.Misses != 4 || got.Hits != 1 || got.Entries != 3 || got.Hashes != 5 {
		t.Fatalf("changed-type stats = %+v", got)
	}
}

func TestRenderSessionLRUBudgetsAndOversizeReporting(t *testing.T) {
	redData := encodeRasterPNG(t, uniformNRGBA(1, 1, color.NRGBA{R: 255, A: 255}))
	blueData := encodeRasterPNG(t, uniformNRGBA(1, 1, color.NRGBA{B: 255, A: 255}))
	assetDocument := func(data []byte) *d2scene.Document {
		return sessionAssetDocument(map[d2scene.AssetID]d2scene.Asset{
			"asset": d2scene.RasterAsset{MIMEType: "image/png", Data: data, PixelWidth: 1, PixelHeight: 1},
		})
	}
	charge := cacheEntryCharge(4, "image/png")
	session := newTestRenderSession(t, RenderSessionOptions{MaxCacheEntries: 1, MaxCacheBytes: charge, MaxConcurrentLoads: 1})
	for _, document := range []*d2scene.Document{assetDocument(redData), assetDocument(redData), assetDocument(blueData), assetDocument(redData)} {
		if _, err := session.Render(context.Background(), document, testOptions()); err != nil {
			t.Fatal(err)
		}
	}
	assertSessionStats(t, session.Stats(), RenderSessionStats{
		Hits: 1, Misses: 3, Evictions: 2, Entries: 1, Bytes: charge,
		MemoMisses: 4, Hashes: 4, MemoSkipped: 4, RetainedBytes: charge,
	})

	oversize := newTestRenderSession(t, RenderSessionOptions{MaxCacheEntries: 1, MaxCacheBytes: charge - 1, MaxConcurrentLoads: 1})
	for range 2 {
		if _, err := oversize.Render(context.Background(), assetDocument(redData), testOptions()); err != nil {
			t.Fatal(err)
		}
	}
	assertSessionStats(t, oversize.Stats(), RenderSessionStats{
		Misses: 2, SkippedOversize: 2, MemoMisses: 2, Hashes: 2, MemoSkipped: 2,
	})
}

func TestRenderSessionConcurrentUseCoalescesLoads(t *testing.T) {
	fontData := sessionFontData(t)
	document := d2scene.NewDocument(d2scene.Box{Width: 180, Height: 40}, d2scene.NewNode(d2scene.TextRun{
		Text: "office e\u0301", Origin: d2scene.Point{X: 4, Y: 28}, Font: d2scene.Font{Asset: "font", Size: 24},
		Fill: d2scene.SolidPaint{Color: color.NRGBA{A: 255}},
	}))
	document.Assets["font"] = d2scene.FontAsset{MIMEType: "font/ttf", Data: fontData}
	session := newTestRenderSession(t, RenderSessionOptions{
		MaxCacheEntries:    1,
		MaxCacheBytes:      cacheEntryCharge(int64(len(fontData)), "font/ttf") + memoEntryCharge(fontData, "font", "font/ttf"),
		MaxConcurrentLoads: 4,
	})

	const workers = 24
	start := make(chan struct{})
	errorsByWorker := make([]error, workers)
	frames := make([]*image.NRGBA, workers)
	var group sync.WaitGroup
	for worker := range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			frames[worker], errorsByWorker[worker] = session.Render(context.Background(), document, testOptions())
		}()
	}
	close(start)
	group.Wait()
	for worker, err := range errorsByWorker {
		if err != nil {
			t.Fatalf("worker %d: %v", worker, err)
		}
		if worker != 0 && !bytes.Equal(frames[0].Pix, frames[worker].Pix) {
			t.Fatalf("worker %d produced nondeterministic shaped pixels", worker)
		}
	}
	reference, err := Render(context.Background(), document, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(frames[0].Pix, reference.Pix) {
		t.Fatal("render-session pixels differ from the direct parser path")
	}
	stats := session.Stats()
	if stats.Misses != 1 || stats.Hits+stats.Waits != workers-1 || stats.Entries != 1 || stats.ActiveLoads != 0 ||
		stats.MemoMisses != 1 || stats.MemoHits+stats.MemoWaits != workers-1 || stats.Hashes != 1 {
		t.Fatalf("concurrent stats = %+v", stats)
	}
}

func TestRenderSessionBundledEmojiCoalescesAndEvicts(t *testing.T) {
	resolver, err := d2fonts.NewBundledFallbackResolver(nil, d2fonts.BundledFallbackLimits{
		MaxRequestedRunes: 1, MaxBundledBytes: 8 << 20, MaxResolvedBytes: 8 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	fonts, err := resolver.ResolveFallbacks(context.Background(), d2fonts.FallbackRequest{Runes: []rune("😀")})
	if err != nil || len(fonts) != 1 {
		t.Fatalf("bundled fallback = %#v, %v", fonts, err)
	}
	const assetID = d2scene.AssetID("emoji")
	document := d2scene.NewDocument(d2scene.Box{Width: 80, Height: 80}, d2scene.NewNode(d2scene.TextRun{
		Text: "😀", Origin: d2scene.Point{X: 8, Y: 60}, Font: d2scene.Font{Asset: assetID, Size: 48},
		Fill: d2scene.SolidPaint{Color: color.NRGBA{A: 255}},
	}))
	document.Assets[assetID] = d2scene.FontAsset{MIMEType: "font/ttf", Data: fonts[0].Data}
	charge := cacheEntryCharge(int64(len(fonts[0].Data)), "font/ttf")
	memoCharge := memoEntryCharge(fonts[0].Data, assetID, "font/ttf")
	session := newTestRenderSession(t, RenderSessionOptions{
		MaxCacheEntries: 1, MaxCacheBytes: charge + memoCharge, MaxConcurrentLoads: 4,
	})

	const workers = 12
	start := make(chan struct{})
	frames := make([]*image.NRGBA, workers)
	errorsByWorker := make([]error, workers)
	var group sync.WaitGroup
	for worker := range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			frames[worker], errorsByWorker[worker] = session.Render(context.Background(), document, testOptions())
		}()
	}
	close(start)
	group.Wait()
	for worker, err := range errorsByWorker {
		if err != nil {
			t.Fatalf("worker %d: %v", worker, err)
		}
		if worker != 0 && !bytes.Equal(frames[0].Pix, frames[worker].Pix) {
			t.Fatalf("worker %d produced different emoji pixels", worker)
		}
	}
	reference, err := Render(context.Background(), document, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(frames[0].Pix, reference.Pix) {
		t.Fatal("render-session emoji pixels differ from the direct parser path")
	}
	stats := session.Stats()
	if stats.Misses != 1 || stats.Hits+stats.Waits != workers-1 || stats.Entries != 1 ||
		stats.Bytes != charge || stats.MemoMisses != 1 || stats.MemoHits+stats.MemoWaits != workers-1 ||
		stats.MemoBytes != memoCharge || stats.Hashes != 1 || stats.RetainedBytes != charge+memoCharge {
		t.Fatalf("concurrent emoji stats = %+v", stats)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	asset := document.Assets[assetID].(d2scene.FontAsset)
	if _, err := session.font(canceled, document, assetID, asset); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled cached emoji font error = %v", err)
	}
	if after := session.Stats(); after != stats {
		t.Fatalf("canceled cached emoji font changed stats: before=%+v after=%+v", stats, after)
	}

	ordinaryData := sessionFontData(t)
	ordinaryDocument := sessionAssetDocument(map[d2scene.AssetID]d2scene.Asset{
		"ordinary": d2scene.FontAsset{MIMEType: "font/ttf", Data: ordinaryData},
	})
	if _, err := session.font(context.Background(), ordinaryDocument, "ordinary", ordinaryDocument.Assets["ordinary"].(d2scene.FontAsset)); err != nil {
		t.Fatal(err)
	}
	stats = session.Stats()
	if stats.Evictions == 0 || stats.Entries != 1 || stats.ActiveLoads != 0 {
		t.Fatalf("emoji cache eviction stats = %+v", stats)
	}
	reloaded, err := session.Render(context.Background(), document, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(reloaded.Pix, reference.Pix) {
		t.Fatal("reloaded emoji pixels differ after session eviction")
	}
	afterReload := session.Stats()
	if afterReload.Misses != stats.Misses+1 || afterReload.Evictions != stats.Evictions+1 || afterReload.Entries != 1 || afterReload.ActiveLoads != 0 {
		t.Fatalf("reloaded emoji cache stats = %+v, before reload %+v", afterReload, stats)
	}
}

func TestRenderSessionCancellationDoesNotPoisonSharedLoad(t *testing.T) {
	newSession := func() *RenderSession {
		return newTestRenderSession(t, RenderSessionOptions{MaxCacheEntries: 2, MaxCacheBytes: 1024, MaxConcurrentLoads: 1})
	}
	key := func(value byte) assetCacheKey {
		var digest [32]byte
		digest[0] = value
		return assetCacheKey{kind: cachedFontAsset, digest: digest}
	}

	t.Run("canceled waiter detaches", func(t *testing.T) {
		session := newSession()
		started, release := make(chan struct{}), make(chan struct{})
		ownerDone := make(chan error, 1)
		go func() {
			_, err := session.getOrLoad(context.Background(), key(1), 1, func(context.Context) (cachedAssetValue, error) {
				close(started)
				<-release
				return cachedAssetValue{}, nil
			})
			ownerDone <- err
		}()
		<-started
		waiterContext, cancelWaiter := context.WithCancel(context.Background())
		waiterDone := make(chan error, 1)
		go func() {
			_, err := session.getOrLoad(waiterContext, key(1), 1, func(context.Context) (cachedAssetValue, error) {
				return cachedAssetValue{}, errors.New("waiter unexpectedly became owner")
			})
			waiterDone <- err
		}()
		waitForSessionStat(t, session, func(stats RenderSessionStats) bool { return stats.Waits == 1 })
		cancelWaiter()
		if err := <-waiterDone; !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled waiter error = %v", err)
		}
		close(release)
		if err := <-ownerDone; err != nil {
			t.Fatal(err)
		}
		assertSessionStats(t, session.Stats(), RenderSessionStats{Misses: 1, Waits: 1, Entries: 1, Bytes: 1, RetainedBytes: 1})
	})

	t.Run("canceled owner does not poison waiter", func(t *testing.T) {
		session := newSession()
		ownerContext, cancelOwner := context.WithCancel(context.Background())
		started := make(chan struct{})
		ownerDone := make(chan error, 1)
		go func() {
			_, err := session.getOrLoad(ownerContext, key(2), 1, func(ctx context.Context) (cachedAssetValue, error) {
				close(started)
				<-ctx.Done()
				return cachedAssetValue{}, ctx.Err()
			})
			ownerDone <- err
		}()
		<-started
		waiterDone := make(chan error, 1)
		go func() {
			_, err := session.getOrLoad(context.Background(), key(2), 1, func(context.Context) (cachedAssetValue, error) {
				return cachedAssetValue{}, nil
			})
			waiterDone <- err
		}()
		waitForSessionStat(t, session, func(stats RenderSessionStats) bool { return stats.Waits == 1 })
		cancelOwner()
		if err := <-ownerDone; !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled owner error = %v", err)
		}
		if err := <-waiterDone; err != nil {
			t.Fatalf("retrying waiter error = %v", err)
		}
		assertSessionStats(t, session.Stats(), RenderSessionStats{Misses: 2, Waits: 1, Entries: 1, Bytes: 1, RetainedBytes: 1})
	})

	t.Run("successful loader observes late cancellation", func(t *testing.T) {
		session := newSession()
		ownerContext, cancelOwner := context.WithCancel(context.Background())
		_, err := session.getOrLoad(ownerContext, key(3), 1, func(context.Context) (cachedAssetValue, error) {
			cancelOwner()
			return cachedAssetValue{}, nil
		})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("late-canceled owner error = %v", err)
		}
		assertSessionStats(t, session.Stats(), RenderSessionStats{Misses: 1})

		if _, err := session.getOrLoad(context.Background(), key(3), 1, func(context.Context) (cachedAssetValue, error) {
			return cachedAssetValue{}, nil
		}); err != nil {
			t.Fatal(err)
		}
		assertSessionStats(t, session.Stats(), RenderSessionStats{Misses: 2, Entries: 1, Bytes: 1, RetainedBytes: 1})
	})
}

func TestFontFaceIndexPartitionsRenderSessionCache(t *testing.T) {
	data := []byte("same collection bytes")
	document := sessionAssetDocument(nil)
	session := newTestRenderSession(t, RenderSessionOptions{MaxCacheEntries: 4, MaxCacheBytes: 4096, MaxConcurrentLoads: 1})
	first, err := session.memoizedCacheKey(context.Background(), document, "face0", cachedFontAsset, "font/ttc", data, 0, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	second, err := session.memoizedCacheKey(context.Background(), document, "face1", cachedFontAsset, "font/ttc", data, 1, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("distinct collection faces shared a render-session cache key")
	}
}

func TestRenderSessionValidationAndHashCancellation(t *testing.T) {
	for name, options := range map[string]RenderSessionOptions{
		"entries":          {MaxCacheBytes: 1, MaxConcurrentLoads: 1},
		"bytes":            {MaxCacheEntries: 1, MaxConcurrentLoads: 1},
		"concurrent loads": {MaxCacheEntries: 1, MaxCacheBytes: 1},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewRenderSession(options); err == nil || !strings.Contains(err.Error(), "every render session limit") {
				t.Fatalf("NewRenderSession() error = %v", err)
			}
		})
	}
	var nilSession *RenderSession
	if _, err := nilSession.Render(context.Background(), dimensionDocument(1, 1), testOptions()); err == nil || !strings.Contains(err.Error(), "nil render session") {
		t.Fatalf("nil session Render() error = %v", err)
	}

	data := make([]byte, 2*assetDigestChunkBytes)
	owned, err := copyAssetBytes(context.Background(), data[1:assetDigestChunkBytes+1])
	if err != nil {
		t.Fatal(err)
	}
	if len(owned) != assetDigestChunkBytes || cap(owned) != len(owned) || &owned[0] == &data[1] {
		t.Fatalf("owned asset bytes len/cap/backing = %d/%d/%t", len(owned), cap(owned), &owned[0] == &data[1])
	}
	data[1] = 1
	if owned[0] != 0 {
		t.Fatal("owned asset bytes changed with their caller-owned source")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if copied, err := copyAssetBytes(canceled, data); copied != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("copyAssetBytes(canceled) = %p, %v", copied, err)
	}
	copyCancel := &cancelAfterErrChecks{remaining: 2}
	if copied, err := copyAssetBytes(copyCancel, data); copied != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("copyAssetBytes(mid-copy cancel) = %p, %v", copied, err)
	}

	ctx := &cancelAfterErrChecks{remaining: 1}
	if _, err := digestAssetBytes(ctx, data); !errors.Is(err, context.Canceled) {
		t.Fatalf("digestAssetBytes() error = %v, want context.Canceled", err)
	}

	session := newTestRenderSession(t, RenderSessionOptions{MaxCacheEntries: 1, MaxCacheBytes: int64(len(data)) + 1024, MaxConcurrentLoads: 1})
	document := dimensionDocument(1, 1)
	ctx = &cancelAfterErrChecks{remaining: 2}
	if _, err := session.memoizedCacheKey(ctx, document, "asset", cachedFontAsset, "font/ttf", data, 0, 0, 0, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("memoizedCacheKey() error = %v, want context.Canceled", err)
	}
	assertSessionStats(t, session.Stats(), RenderSessionStats{MemoMisses: 1})
	if _, err := session.memoizedCacheKey(context.Background(), document, "asset", cachedFontAsset, "font/ttf", data, 0, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	if got := session.Stats(); got.MemoMisses != 2 || got.Hashes != 1 || got.MemoEntries != 1 || got.ActiveLoads != 0 {
		t.Fatalf("memo retry stats = %+v", got)
	}
}

func BenchmarkRenderSessionFontCold(b *testing.B) {
	resolver, err := d2fonts.NewBundledFallbackResolver(nil, d2fonts.BundledFallbackLimits{
		MaxRequestedRunes: 1, MaxBundledBytes: 8 << 20, MaxResolvedBytes: 8 << 20,
	})
	if err != nil {
		b.Fatal(err)
	}
	emojiFonts, err := resolver.ResolveFallbacks(context.Background(), d2fonts.FallbackRequest{Runes: []rune("😀")})
	if err != nil || len(emojiFonts) != 1 {
		b.Fatalf("bundled fallback = %#v, %v", emojiFonts, err)
	}
	emojiData, err := fontface.RegisterBundledNotoColorEmoji(emojiFonts[0].Data)
	if err != nil {
		b.Fatal(err)
	}
	tests := []struct {
		name string
		data []byte
	}{
		{name: "ordinary", data: sessionFontData(b)},
		{name: "emoji", data: emojiData},
	}
	for _, test := range tests {
		b.Run(test.name, func(b *testing.B) {
			document := sessionAssetDocument(map[d2scene.AssetID]d2scene.Asset{
				"font": d2scene.FontAsset{MIMEType: "font/ttf", Data: test.data},
			})
			asset := document.Assets["font"].(d2scene.FontAsset)
			options := RenderSessionOptions{MaxCacheEntries: 2, MaxCacheBytes: 16 << 20, MaxConcurrentLoads: 1}
			b.ReportAllocs()
			b.SetBytes(int64(len(test.data)))
			for b.Loop() {
				session, err := NewRenderSession(options)
				if err != nil {
					b.Fatal(err)
				}
				font, err := session.font(context.Background(), document, "font", asset)
				if err != nil || font == nil {
					b.Fatalf("font() = %p, %v", font, err)
				}
			}
		})
	}
}

func BenchmarkRenderSessionFontWarm(b *testing.B) {
	resolver, err := d2fonts.NewBundledFallbackResolver(nil, d2fonts.BundledFallbackLimits{
		MaxRequestedRunes: 1, MaxBundledBytes: 8 << 20, MaxResolvedBytes: 8 << 20,
	})
	if err != nil {
		b.Fatal(err)
	}
	emojiFonts, err := resolver.ResolveFallbacks(context.Background(), d2fonts.FallbackRequest{Runes: []rune("😀")})
	if err != nil || len(emojiFonts) != 1 {
		b.Fatalf("bundled fallback = %#v, %v", emojiFonts, err)
	}
	emojiData, err := fontface.RegisterBundledNotoColorEmoji(emojiFonts[0].Data)
	if err != nil {
		b.Fatal(err)
	}
	tests := []struct {
		name string
		data []byte
	}{
		{name: "ordinary", data: sessionFontData(b)},
		{name: "emoji", data: emojiData},
	}
	for _, test := range tests {
		b.Run(test.name, func(b *testing.B) {
			document := sessionAssetDocument(map[d2scene.AssetID]d2scene.Asset{
				"font": d2scene.FontAsset{MIMEType: "font/ttf", Data: test.data},
			})
			asset := document.Assets["font"].(d2scene.FontAsset)
			charge := cacheEntryCharge(int64(len(test.data)), "font/ttf")
			memoCharge := memoEntryCharge(test.data, "font", "font/ttf")
			session := newTestRenderSession(b, RenderSessionOptions{
				MaxCacheEntries: 1, MaxCacheBytes: charge + memoCharge, MaxConcurrentLoads: 1,
			})
			if _, err := session.font(context.Background(), document, "font", asset); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			for b.Loop() {
				font, err := session.font(context.Background(), document, "font", asset)
				if err != nil || font == nil {
					b.Fatalf("font() = %p, %v", font, err)
				}
			}
		})
	}
}

func BenchmarkRendererFontPreparation(b *testing.B) {
	resolver, err := d2fonts.NewBundledFallbackResolver(nil, d2fonts.BundledFallbackLimits{
		MaxRequestedRunes: 1, MaxBundledBytes: 8 << 20, MaxResolvedBytes: 8 << 20,
	})
	if err != nil {
		b.Fatal(err)
	}
	emojiFonts, err := resolver.ResolveFallbacks(context.Background(), d2fonts.FallbackRequest{Runes: []rune("😀")})
	if err != nil || len(emojiFonts) != 1 {
		b.Fatalf("bundled fallback = %#v, %v", emojiFonts, err)
	}
	emojiData, err := fontface.RegisterBundledNotoColorEmoji(emojiFonts[0].Data)
	if err != nil {
		b.Fatal(err)
	}
	for _, test := range []struct {
		name string
		data []byte
	}{
		{name: "ordinary", data: sessionFontData(b)},
		{name: "emoji", data: emojiData},
	} {
		b.Run(test.name+"/isolated-parse", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				face, err := fontface.ParseFace(test.data, 0)
				if err != nil || face == nil {
					b.Fatalf("ParseFace() = %p, %v", face, err)
				}
			}
		})
		b.Run(test.name+"/renderer-clone", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				font, err := parsePreparedFont(test.data, 0)
				if err != nil || font == nil {
					b.Fatalf("parsePreparedFont() = %p, %v", font, err)
				}
			}
		})
	}
}

func newTestRenderSession(t testing.TB, options RenderSessionOptions) *RenderSession {
	t.Helper()
	session, err := NewRenderSession(options)
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func sessionFontData(t testing.TB) []byte {
	t.Helper()
	data, ok := d2fonts.FontFaces.Lookup(d2fonts.Font{Family: d2fonts.SourceSansPro, Style: d2fonts.FONT_STYLE_REGULAR})
	if !ok {
		t.Fatal("embedded Source Sans Pro font is not loaded")
	}
	return data
}

func sessionAssetDocument(assets map[d2scene.AssetID]d2scene.Asset) *d2scene.Document {
	document := d2scene.NewDocument(d2scene.Box{Width: 2, Height: 2}, d2scene.NewNode(nil))
	document.Assets = assets
	return document
}

func uniformNRGBA(width, height int, value color.NRGBA) *image.NRGBA {
	result := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			result.SetNRGBA(x, y, value)
		}
	}
	return result
}

func waitForSessionStat(t *testing.T, session *RenderSession, ready func(RenderSessionStats) bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if ready(session.Stats()) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for session state; stats=%+v", session.Stats())
		}
		runtime.Gosched()
	}
}

func assertSessionStats(t *testing.T, got, want RenderSessionStats) {
	t.Helper()
	if got != want {
		t.Fatalf("session stats = %+v, want %+v", got, want)
	}
}
