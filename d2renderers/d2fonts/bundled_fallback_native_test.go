//go:build !js || !wasm

package d2fonts

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/andybalholm/brotli"

	"github.com/d2lang/d2/d2renderers/internal/fontface"
)

type recordingFallbackResolver struct {
	fonts   []FallbackFont
	err     error
	calls   int
	request FallbackRequest
}

func (r *recordingFallbackResolver) ResolveFallbacks(ctx context.Context, request FallbackRequest) ([]FallbackFont, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.calls++
	r.request = request
	r.request.Runes = append([]rune(nil), request.Runes...)
	if r.err != nil {
		return nil, r.err
	}
	return append([]FallbackFont(nil), r.fonts...), nil
}

func TestBundledFallbackResolverIsBuiltinFirstAndImmutable(t *testing.T) {
	downstreamData := []byte("downstream")
	downstream := &recordingFallbackResolver{fonts: []FallbackFont{{
		Name: "cyrillic.ttf", MIMEType: "font/ttf", Data: downstreamData,
	}}}
	resolver, err := NewBundledFallbackResolver(downstream, BundledFallbackLimits{
		MaxRequestedRunes: 10, MaxBundledBytes: 2 * bundledNotoColorEmojiSize,
		MaxResolvedBytes: 2*bundledNotoColorEmojiSize + int64(len(downstreamData)),
	})
	if err != nil {
		t.Fatal(err)
	}
	request := FallbackRequest{
		Runes:  []rune{'\U0001F6E1', '\u0416', '\u2705', '\u2705'},
		Family: "Source Code Pro", Style: "italic", Weight: 600,
	}
	fonts, err := resolver.ResolveFallbacks(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if downstream.calls != 1 || len(downstream.request.Runes) != 1 || downstream.request.Runes[0] != '\u0416' ||
		downstream.request.Family != request.Family || downstream.request.Style != request.Style || downstream.request.Weight != request.Weight {
		t.Fatalf("downstream call/request = %d/%#v, want remaining U+0416 with original metadata", downstream.calls, downstream.request)
	}
	if len(fonts) != 2 || fonts[0].Name != "NotoColorEmoji-COLRv1-v2.051.ttf" || fonts[0].MIMEType != "font/ttf" || fonts[0].FaceIndex != 0 {
		t.Fatalf("resolved fonts = %#v", fonts)
	}
	if !bytes.Equal(fonts[1].Data, downstreamData) {
		t.Fatalf("downstream data = %q, want %q", fonts[1].Data, downstreamData)
	}
	face, err := fontface.ParseFace(fonts[0].Data, fonts[0].FaceIndex)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []rune{'\u2705', '\U0001F6E1'} {
		supported, err := face.SupportsRenderableRune(value)
		if err != nil {
			t.Fatal(err)
		}
		if !supported {
			t.Fatalf("bundled fallback does not support %U", value)
		}
	}

	fonts[0].Data[0] ^= 0xff
	fonts[1].Data[0] ^= 0xff
	if downstream.fonts[0].Data[0] != 'd' || !bytes.Equal(downstream.fonts[0].Data, downstreamData) {
		t.Fatal("composite result aliases downstream resolver bytes")
	}
	again, err := resolver.ResolveFallbacks(context.Background(), FallbackRequest{Runes: []rune{'\u2705', '\U0001F6E1'}})
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 1 || again[0].Data[0] == fonts[0].Data[0] {
		t.Fatal("composite result aliases the process-owned bundled font cache")
	}
	if downstream.calls != 1 {
		t.Fatalf("downstream calls = %d after bundled-only request, want 1", downstream.calls)
	}
}

func TestBundledFallbackResolverPrefiltersPinnedCmapBeforeLoadingFont(t *testing.T) {
	newResolver := func(t *testing.T, downstream FallbackResolver) *BundledFallbackResolver {
		t.Helper()
		resolver, err := NewBundledFallbackResolver(downstream, BundledFallbackLimits{
			MaxRequestedRunes: 8, MaxBundledBytes: bundledNotoColorEmojiSize,
			MaxResolvedBytes: bundledNotoColorEmojiSize,
		})
		if err != nil {
			t.Fatal(err)
		}
		return resolver
	}

	downstream := &recordingFallbackResolver{}
	resolver := newResolver(t, downstream)
	loads := 0
	resolver.loadBundled = func() ([]byte, error) {
		loads++
		return bundledNotoColorEmoji()
	}
	fonts, err := resolver.ResolveFallbacks(context.Background(), FallbackRequest{Runes: []rune{'\u2b24'}})
	if err != nil {
		t.Fatal(err)
	}
	if loads != 0 || len(fonts) != 0 || downstream.calls != 1 || len(downstream.request.Runes) != 1 || downstream.request.Runes[0] != '\u2b24' {
		t.Fatalf("U+2B24 loads/fonts/downstream = %d/%#v/%d/%#v, want 0/empty/1/U+2B24", loads, fonts, downstream.calls, downstream.request)
	}

	resolver = newResolver(t, nil)
	loads = 0
	resolver.loadBundled = func() ([]byte, error) {
		loads++
		return bundledNotoColorEmoji()
	}
	fonts, err = resolver.ResolveFallbacks(context.Background(), FallbackRequest{Runes: []rune{'\u2705', '\U0001f600'}})
	if err != nil {
		t.Fatal(err)
	}
	if loads != 1 || len(fonts) != 1 || fonts[0].Name != "NotoColorEmoji-COLRv1-v2.051.ttf" || len(fonts[0].Data) != bundledNotoColorEmojiSize {
		t.Fatalf("real emoji loads/fonts = %d/%#v, want one load and one bundled Noto asset", loads, fonts)
	}
}

func TestDecodeBundledBrotliFontAuthenticatesAndBounds(t *testing.T) {
	decoded := bytes.Repeat([]byte("authenticated font fixture"), 8)
	var stream bytes.Buffer
	writer := brotli.NewWriterLevel(&stream, 11)
	if _, err := writer.Write(decoded); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	compressed := stream.Bytes()
	compressedDigest := sha256.Sum256(compressed)
	decodedDigest := sha256.Sum256(decoded)

	got, err := decodeBundledBrotliFont(compressed, len(compressed), compressedDigest, len(decoded), decodedDigest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, decoded) {
		t.Fatal("decoded Brotli fixture differs from authenticated input")
	}

	for _, test := range []struct {
		name             string
		compressedSize   int
		compressedSHA256 [sha256.Size]byte
		decodedSize      int
		decodedSHA256    [sha256.Size]byte
		wantError        string
	}{
		{name: "invalid compressed limit", compressedSize: 0, compressedSHA256: compressedDigest, decodedSize: len(decoded), decodedSHA256: decodedDigest, wantError: "invalid compressed/decoded size limits"},
		{name: "invalid decoded limit", compressedSize: len(compressed), compressedSHA256: compressedDigest, decodedSize: 0, decodedSHA256: decodedDigest, wantError: "invalid compressed/decoded size limits"},
		{name: "compressed size", compressedSize: len(compressed) + 1, compressedSHA256: compressedDigest, decodedSize: len(decoded), decodedSHA256: decodedDigest, wantError: "Brotli font size"},
		{name: "compressed digest", compressedSize: len(compressed), decodedSize: len(decoded), decodedSHA256: decodedDigest, wantError: "Brotli font SHA-256"},
		{name: "decoded limit", compressedSize: len(compressed), compressedSHA256: compressedDigest, decodedSize: len(decoded) - 1, decodedSHA256: decodedDigest, wantError: "decoded font exceeds limit"},
		{name: "decoded size", compressedSize: len(compressed), compressedSHA256: compressedDigest, decodedSize: len(decoded) + 1, decodedSHA256: decodedDigest, wantError: "decoded font size"},
		{name: "decoded digest", compressedSize: len(compressed), compressedSHA256: compressedDigest, decodedSize: len(decoded), wantError: "decoded font SHA-256"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := decodeBundledBrotliFont(compressed, test.compressedSize, test.compressedSHA256, test.decodedSize, test.decodedSHA256)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("error = %v, want %q", err, test.wantError)
			}
		})
	}
}

func TestBundledFallbackResolverLimitsCancellationAndAtomicFailure(t *testing.T) {
	if _, err := NewBundledFallbackResolver(nil, BundledFallbackLimits{}); err == nil {
		t.Fatal("zero limits accepted")
	}
	if _, err := NewBundledFallbackResolver(nil, BundledFallbackLimits{MaxRequestedRunes: 1, MaxBundledBytes: 2, MaxResolvedBytes: 1}); err == nil || !strings.Contains(err.Error(), "exceeds aggregate") {
		t.Fatalf("inverted byte limits error = %v", err)
	}
	defaulted, err := NewBundledFallbackResolver(nil, BundledFallbackLimits{MaxRequestedRunes: 1, MaxResolvedBytes: 1})
	if err != nil || defaulted.limits.MaxBundledBytes != 1 {
		t.Fatalf("default bundled byte limit = %+v, %v; want aggregate limit 1", defaulted, err)
	}
	resolver, err := NewBundledFallbackResolver(nil, BundledFallbackLimits{MaxRequestedRunes: 1, MaxBundledBytes: 1, MaxResolvedBytes: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.ResolveFallbacks(context.Background(), FallbackRequest{Runes: []rune{'\u2705', '\U0001F6E1'}}); err == nil || !strings.Contains(err.Error(), "rune count") {
		t.Fatalf("request limit error = %v", err)
	}
	if _, err := resolver.ResolveFallbacks(context.Background(), FallbackRequest{
		Runes: []rune{'\u2705'}, Family: strings.Repeat("x", 1_025),
	}); err == nil || !strings.Contains(err.Error(), "metadata") {
		t.Fatalf("metadata limit error = %v", err)
	}
	if _, err := resolver.ResolveFallbacks(context.Background(), FallbackRequest{Runes: []rune{'\u2705'}}); err == nil || !strings.Contains(err.Error(), "bytes exceed limit") {
		t.Fatalf("byte limit error = %v", err)
	}
	if resolver.work.bundledBytes != 0 || resolver.work.resolvedBytes != 0 {
		t.Fatalf("failed bundled clone consumed byte budget: %+v", resolver.work)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := resolver.ResolveFallbacks(ctx, FallbackRequest{Runes: []rune{'\u2705'}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled error = %v", err)
	}

	downstream := &recordingFallbackResolver{err: errors.New("downstream failed")}
	resolver, err = NewBundledFallbackResolver(downstream, BundledFallbackLimits{
		MaxRequestedRunes: 2, MaxBundledBytes: bundledNotoColorEmojiSize,
		MaxResolvedBytes: bundledNotoColorEmojiSize + 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	fonts, err := resolver.ResolveFallbacks(context.Background(), FallbackRequest{Runes: []rune{'\u2705', '\u0416'}})
	if err == nil || !strings.Contains(err.Error(), "downstream failed") || fonts != nil {
		t.Fatalf("atomic downstream failure result/error = %#v/%v", fonts, err)
	}

	downstream = &recordingFallbackResolver{fonts: []FallbackFont{{
		Name: "small.ttf", MIMEType: "font/ttf", Data: []byte{1, 2},
	}}}
	resolver, err = NewBundledFallbackResolver(downstream, BundledFallbackLimits{
		MaxRequestedRunes: 2, MaxBundledBytes: bundledNotoColorEmojiSize,
		MaxResolvedBytes: bundledNotoColorEmojiSize + 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	fonts, err = resolver.ResolveFallbacks(context.Background(), FallbackRequest{Runes: []rune{'\u2705', '\u0416'}})
	if err == nil || !strings.Contains(err.Error(), "bytes exceed limit") || fonts != nil {
		t.Fatalf("aggregate byte limit result/error = %#v/%v", fonts, err)
	}

	resolver, err = NewBundledFallbackResolver(nil, BundledFallbackLimits{
		MaxRequestedRunes: 3, MaxBundledBytes: 2*bundledNotoColorEmojiSize - 1,
		MaxResolvedBytes: 2*bundledNotoColorEmojiSize - 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.ResolveFallbacks(context.Background(), FallbackRequest{Runes: []rune{'\u2705'}}); err != nil {
		t.Fatal(err)
	}
	fonts, err = resolver.ResolveFallbacks(context.Background(), FallbackRequest{Runes: []rune{'\u2705'}})
	if err == nil || !strings.Contains(err.Error(), "bytes exceed limit") || fonts != nil {
		t.Fatalf("cumulative byte limit result/error = %#v/%v", fonts, err)
	}
}

func TestBundledFallbackResolverSerializesCachedCoverageFace(t *testing.T) {
	const workers = 8
	resolver, err := NewBundledFallbackResolver(nil, BundledFallbackLimits{
		MaxRequestedRunes: 16, MaxBundledBytes: workers * bundledNotoColorEmojiSize,
		MaxResolvedBytes: workers * bundledNotoColorEmojiSize,
	})
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, workers)
	var wait sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			fonts, err := resolver.ResolveFallbacks(context.Background(), FallbackRequest{Runes: []rune{'\u2705', '\U0001F6E1'}})
			if err != nil {
				results <- err
				return
			}
			if len(fonts) != 1 {
				results <- errors.New("bundled fallback did not return exactly one face")
			}
		}()
	}
	wait.Wait()
	close(results)
	for err := range results {
		t.Error(err)
	}
	if resolver.bundledFace == nil {
		t.Fatal("concurrent bundled resolutions did not retain their serialized coverage face")
	}
	if want := int64(workers * bundledNotoColorEmojiSize); resolver.work.bundledBytes != want || resolver.work.resolvedBytes != want {
		t.Fatalf("concurrent bundled/total bytes = %d/%d, want %d independently owned bytes", resolver.work.bundledBytes, resolver.work.resolvedBytes, want)
	}
}

func TestBundledFallbackResolverSkipsGuaranteedMissingScalar(t *testing.T) {
	downstream := &recordingFallbackResolver{err: errors.New("must not be called")}
	resolver, err := NewBundledFallbackResolver(downstream, BundledFallbackLimits{
		MaxRequestedRunes: 1, MaxBundledBytes: bundledNotoColorEmojiSize,
		MaxResolvedBytes: bundledNotoColorEmojiSize,
	})
	if err != nil {
		t.Fatal(err)
	}
	fonts, err := resolver.ResolveFallbacks(context.Background(), FallbackRequest{Runes: []rune{'\U0010ffff'}})
	if err != nil || len(fonts) != 0 {
		t.Fatalf("noncharacter result/error = %#v/%v", fonts, err)
	}
	if downstream.calls != 0 || resolver.bundledFace != nil || resolver.work.bundledBytes != 0 || resolver.work.resolvedBytes != 0 {
		t.Fatalf("noncharacter triggered bundled/downstream work: downstream=%d face=%p work=%+v", downstream.calls, resolver.bundledFace, resolver.work)
	}
}

func TestBundledFallbackResolverDoesNotLoadEmojiFontForOtherScripts(t *testing.T) {
	downstream := &recordingFallbackResolver{}
	resolver, err := NewBundledFallbackResolver(downstream, BundledFallbackLimits{
		MaxRequestedRunes: 1, MaxBundledBytes: bundledNotoColorEmojiSize,
		MaxResolvedBytes: bundledNotoColorEmojiSize,
	})
	if err != nil {
		t.Fatal(err)
	}
	fonts, err := resolver.ResolveFallbacks(context.Background(), FallbackRequest{Runes: []rune{'\u0416'}})
	if err != nil || len(fonts) != 0 {
		t.Fatalf("non-emoji result/error = %#v/%v", fonts, err)
	}
	if downstream.calls != 1 || resolver.bundledFace != nil || resolver.work.bundledBytes != 0 {
		t.Fatalf("non-emoji triggered bundled work: downstream=%d face=%p work=%+v", downstream.calls, resolver.bundledFace, resolver.work)
	}
}
