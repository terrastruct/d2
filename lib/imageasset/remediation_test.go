package imageasset

import (
	"bytes"
	"compress/flate"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/d2lang/d2/internal/rasterimage"
)

func TestAggregateLimitsExactAndPlusOne(t *testing.T) {
	base := encodePNG(t, 1, 1)
	first := insertPNGChunk(t, base, "tEXt", append([]byte("key\x00"), bytes.Repeat([]byte{'a'}, 32*1024)...))
	second := insertPNGChunk(t, base, "tEXt", append([]byte("key\x00"), bytes.Repeat([]byte{'b'}, 32*1024)...))
	third := append(append([]byte(nil), base...), 3)
	firstSource := dataURI("image/png", first)
	secondSource := dataURI("image/png", second)
	thirdSource := dataURI("image/png", third)
	encodedTotal := int64(len(first) + len(second))

	t.Run("assets", func(t *testing.T) {
		limits := generousLimits()
		limits.MaxAssets = 2
		resolver := newTestResolver(t, Options{Limits: limits})
		mustResolve(t, resolver, firstSource)
		mustResolve(t, resolver, secondSource)
		assertAggregate(t, resolver, 2, encodedTotal, 8)
		_, err := resolver.Resolve(context.Background(), thirdSource)
		assertLimitError(t, err, "assets")
		assertAggregate(t, resolver, 2, encodedTotal, 8)
	})

	t.Run("encoded exact", func(t *testing.T) {
		limits := generousLimits()
		limits.MaxCumulativeEncodedBytes = encodedTotal
		resolver := newTestResolver(t, Options{Limits: limits})
		mustResolve(t, resolver, firstSource)
		mustResolve(t, resolver, secondSource)
		assertAggregate(t, resolver, 2, encodedTotal, 8)
	})

	t.Run("encoded plus one", func(t *testing.T) {
		limits := generousLimits()
		limits.MaxCumulativeEncodedBytes = encodedTotal - 1
		resolver := newTestResolver(t, Options{Limits: limits})
		mustResolve(t, resolver, firstSource)
		_, err := resolver.Resolve(context.Background(), secondSource)
		assertLimitError(t, err, "cumulative encoded bytes")
		assertAggregate(t, resolver, 1, int64(len(first)), 4)
	})

	t.Run("decoded exact", func(t *testing.T) {
		limits := generousLimits()
		limits.MaxCumulativeDecodedBytes = 8
		resolver := newTestResolver(t, Options{Limits: limits})
		mustResolve(t, resolver, firstSource)
		mustResolve(t, resolver, secondSource)
		assertAggregate(t, resolver, 2, encodedTotal, 8)
	})

	t.Run("decoded plus one", func(t *testing.T) {
		limits := generousLimits()
		limits.MaxCumulativeDecodedBytes = 7
		resolver := newTestResolver(t, Options{Limits: limits})
		mustResolve(t, resolver, firstSource)
		_, err := resolver.Resolve(context.Background(), secondSource)
		assertLimitError(t, err, "cumulative decoded bytes")
		assertAggregate(t, resolver, 1, int64(len(first)), 4)
	})
}

func TestAggregateReservationConcurrentExactRollback(t *testing.T) {
	const (
		capacity   = 8
		goroutines = 64
	)
	limits := generousLimits()
	limits.MaxAssets = capacity
	limits.MaxCumulativeEncodedBytes = capacity * 10
	limits.MaxCumulativeDecodedBytes = capacity * 20
	resolver := newTestResolver(t, Options{Limits: limits})
	type result struct {
		rollback func()
		err      error
	}
	results := make(chan result, goroutines)
	release := make(chan struct{})
	var wait sync.WaitGroup
	for range goroutines {
		wait.Add(1)
		go func() {
			defer wait.Done()
			rollback, err := resolver.reserve(10, 20)
			results <- result{rollback: rollback, err: err}
			if err == nil {
				<-release
				rollback()
				rollback() // idempotent
			}
		}()
	}
	successes := 0
	for range goroutines {
		result := <-results
		if result.err == nil {
			successes++
			continue
		}
		var limitErr *LimitError
		if !errors.As(result.err, &limitErr) {
			t.Fatalf("reservation error = %v", result.err)
		}
	}
	if successes != capacity {
		t.Fatalf("successful concurrent reservations = %d, want %d", successes, capacity)
	}
	assertAggregate(t, resolver, capacity, capacity*10, capacity*20)
	close(release)
	wait.Wait()
	assertAggregate(t, resolver, 0, 0, 0)
}

func TestFailuresAndCancellationAreNeverMemoized(t *testing.T) {
	t.Run("malformed then valid", func(t *testing.T) {
		valid := encodePNG(t, 2, 2)
		corrupt := corruptPNGIDATCRC(t, valid)
		var requests atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if requests.Add(1) == 1 {
				_, _ = w.Write(corrupt)
				return
			}
			_, _ = w.Write(valid)
		}))
		defer server.Close()
		resolver := newTestResolver(t, Options{HTTPClient: server.Client()})
		if _, err := resolver.Resolve(context.Background(), server.URL); err == nil {
			t.Fatal("expected first malformed response to fail")
		}
		assertAggregate(t, resolver, 0, 0, 0)
		resource := mustResolve(t, resolver, server.URL)
		if again := mustResolve(t, resolver, server.URL); again != resource {
			t.Fatal("successful retry was not memoized")
		}
		if requests.Load() != 2 {
			t.Fatalf("requests = %d, want 2", requests.Load())
		}
	})

	t.Run("canceled then valid", func(t *testing.T) {
		valid := encodePNG(t, 1, 1)
		started := make(chan struct{})
		var requests atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			if requests.Add(1) == 1 {
				close(started)
				<-request.Context().Done()
				return
			}
			_, _ = w.Write(valid)
		}))
		defer server.Close()
		resolver := newTestResolver(t, Options{HTTPClient: server.Client()})
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			<-started
			cancel()
		}()
		if _, err := resolver.Resolve(ctx, server.URL); !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled Resolve error = %v", err)
		}
		assertAggregate(t, resolver, 0, 0, 0)
		mustResolve(t, resolver, server.URL)
		if requests.Load() != 2 {
			t.Fatalf("requests = %d, want 2", requests.Load())
		}
	})
}

func TestWaitingCallerRecoversFromCanceledLeader(t *testing.T) {
	raw := encodePNG(t, 1, 1)
	firstStarted := make(chan struct{})
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if requests.Add(1) == 1 {
			close(firstStarted)
			<-request.Context().Done()
			return
		}
		_, _ = w.Write(raw)
	}))
	defer server.Close()
	resolver := newTestResolver(t, Options{HTTPClient: server.Client()})
	leaderContext, cancelLeader := context.WithCancel(context.Background())
	leaderResult := make(chan error, 1)
	go func() {
		_, err := resolver.Resolve(leaderContext, server.URL)
		leaderResult <- err
	}()
	<-firstStarted
	type followerResult struct {
		resource *Resource
		err      error
	}
	followerResults := make(chan followerResult, 1)
	go func() {
		resource, err := resolver.Resolve(context.Background(), server.URL)
		followerResults <- followerResult{resource: resource, err: err}
	}()
	if len(resolver.resolveSlots) != 1 {
		t.Fatalf("duplicate in-flight caller consumed a slot: got %d active slots, want 1", len(resolver.resolveSlots))
	}
	cancelLeader()
	if err := <-leaderResult; !errors.Is(err, context.Canceled) {
		t.Fatalf("leader error = %v, want context.Canceled", err)
	}
	follower := <-followerResults
	if follower.err != nil || follower.resource == nil {
		t.Fatalf("follower result = resource %v error %v", follower.resource, follower.err)
	}
	if requests.Load() != 2 {
		t.Fatalf("requests = %d, want canceled leader plus successful retry", requests.Load())
	}
	assertAggregate(t, resolver, 1, int64(len(raw)), 4)
}

func TestSharedCacheRequiresAndIsolatesNamespace(t *testing.T) {
	cache, err := NewMemoryCache(8, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := New(Options{Cache: cache, Limits: generousLimits()}); err == nil || !strings.Contains(err.Error(), "CacheNamespace") {
		t.Fatalf("unnamespaced shared cache error = %v", err)
	}

	firstPNG := encodePNG(t, 1, 1)
	secondPNG := encodePNG(t, 2, 1)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.Header.Get("X-Tenant") == "first" {
			_, _ = w.Write(firstPNG)
			return
		}
		_, _ = w.Write(secondPNG)
	}))
	defer server.Close()
	clientFor := func(tenant string) *http.Client {
		base := server.Client()
		transport := base.Transport
		client := *base
		client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
			clone := request.Clone(request.Context())
			clone.Header = request.Header.Clone()
			clone.Header.Set("X-Tenant", tenant)
			return transport.RoundTrip(clone)
		})
		return &client
	}
	firstResolver := newTestResolver(t, Options{HTTPClient: clientFor("first"), Cache: cache, CacheNamespace: "tenant-first"})
	secondResolver := newTestResolver(t, Options{HTTPClient: clientFor("second"), Cache: cache, CacheNamespace: "tenant-second"})
	first := mustResolve(t, firstResolver, server.URL)
	second := mustResolve(t, secondResolver, server.URL)
	if first.PixelWidth() != 1 || second.PixelWidth() != 2 || first == second {
		t.Fatalf("namespace isolation failed: first=%dx%d second=%dx%d same=%v", first.PixelWidth(), first.PixelHeight(), second.PixelWidth(), second.PixelHeight(), first == second)
	}
	thirdResolver := newTestResolver(t, Options{HTTPClient: clientFor("first"), Cache: cache, CacheNamespace: "tenant-first"})
	third := mustResolve(t, thirdResolver, server.URL)
	if third != first || requests.Load() != 2 {
		t.Fatalf("same-namespace cache miss: same=%v requests=%d", third == first, requests.Load())
	}
	assertAggregate(t, thirdResolver, 1, int64(len(firstPNG)), 4)
}

func TestConcurrentSharedCacheHitsReuseSnapshotAndChargeEachResolver(t *testing.T) {
	const goroutines = 24
	raw := encodePNG(t, 2, 2)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write(raw)
	}))
	cache, err := NewMemoryCache(4, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	seed := newTestResolver(t, Options{HTTPClient: server.Client(), Cache: cache, CacheNamespace: "shared"})
	snapshot := mustResolve(t, seed, server.URL)
	server.Close()

	type result struct {
		resource *Resource
		resolver *Resolver
		err      error
	}
	start := make(chan struct{})
	results := make(chan result, goroutines)
	resolvers := make([]*Resolver, goroutines)
	for index := range resolvers {
		resolvers[index] = newTestResolver(t, Options{HTTPClient: server.Client(), Cache: cache, CacheNamespace: "shared"})
	}
	for _, resolver := range resolvers {
		go func(resolver *Resolver) {
			<-start
			resource, err := resolver.Resolve(context.Background(), server.URL)
			results <- result{resource: resource, resolver: resolver, err: err}
		}(resolver)
	}
	close(start)
	for range goroutines {
		result := <-results
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.resource != snapshot {
			t.Fatal("shared cache hit returned a different immutable snapshot")
		}
		assertAggregate(t, result.resolver, 1, int64(len(raw)), 16)
	}
	if requests.Load() != 1 {
		t.Fatalf("requests = %d, want one cache-populating fetch", requests.Load())
	}
}

func TestHTTPFragmentsShareCanonicalSnapshot(t *testing.T) {
	raw := encodePNG(t, 1, 1)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.URL.Fragment != "" {
			t.Errorf("request fragment = %q", request.URL.Fragment)
		}
		_, _ = w.Write(raw)
	}))
	defer server.Close()
	resolver := newTestResolver(t, Options{HTTPClient: server.Client()})
	first := mustResolve(t, resolver, server.URL+"/image#first")
	second := mustResolve(t, resolver, server.URL+"/image#second")
	if first != second || requests.Load() != 1 {
		t.Fatalf("fragment aliases: same=%v requests=%d", first == second, requests.Load())
	}
}

func TestURLSecretsAreAbsentFromFullErrorTree(t *testing.T) {
	transportErr := errors.New("transport failed for password token=secret fragment")
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, transportErr
	})}
	resolver := newTestResolver(t, Options{HTTPClient: client})
	_, err := resolver.Resolve(context.Background(), "https://user:password@example.invalid/image?token=secret#fragment")
	if !errors.Is(err, transportErr) {
		t.Fatalf("errors.Is lost transport sentinel: %v", err)
	}
	assertSanitizedErrorTree(t, err, "password", "token=secret", "fragment")
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		t.Fatalf("URL-bearing error remained reachable: %#v", urlErr)
	}

	for _, source := range []string{
		"http://user:password@example.invalid/%zz?token=secret#fragment",
		"ftp://user:password@example.invalid/image?token=secret#fragment",
		"ftp://user:password@example.invalid/%zz?token=secret#fragment",
		"mailto:user:password@example.invalid?token=secret#fragment",
	} {
		_, err := resolver.Resolve(context.Background(), source)
		if err == nil {
			t.Fatalf("source %q unexpectedly succeeded", source)
		}
		assertSanitizedErrorTree(t, err, "password", "token=secret", "fragment")
	}
}

func TestHTTPHeaderBoundsAndTokenParsing(t *testing.T) {
	raw := encodePNG(t, 1, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/encoding-bytes":
			w.Header().Set("Content-Encoding", strings.Repeat("g", maxContentEncodingHeaderBytes+1))
		case "/empty-encoding":
			w.Header().Set("Content-Encoding", "gzip,,br")
		case "/encoding-layers":
			w.Header().Set("Content-Encoding", "identity,identity,identity,identity,identity")
		case "/content-type-bytes":
			w.Header().Set("Content-Type", strings.Repeat("x", maxContentTypeHeaderBytes+1))
		case "/multiple-content-types":
			w.Header().Add("Content-Type", "image/png")
			w.Header().Add("Content-Type", "application/octet-stream")
		}
		_, _ = w.Write(raw)
	}))
	defer server.Close()
	tests := []struct {
		path string
		want string
	}{
		{path: "/encoding-bytes", want: "Content-Encoding header bytes"},
		{path: "/empty-encoding", want: "empty token"},
		{path: "/encoding-layers", want: "layer count"},
		{path: "/content-type-bytes", want: "Content-Type header bytes"},
		{path: "/multiple-content-types", want: "multiple Content-Type"},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			resolver := newTestResolver(t, Options{HTTPClient: server.Client()})
			_, err := resolver.Resolve(context.Background(), server.URL+test.path)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Resolve error = %v, want %q", err, test.want)
			}
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := parseContentEncodings(ctx, []string{"gzip"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled header parse error = %v", err)
	}
	resolver := newTestResolver(t, Options{})
	transport, ok := resolver.client.Transport.(*http.Transport)
	if !ok || transport.MaxResponseHeaderBytes != maxResponseHeaderBytes {
		t.Fatalf("default transport header cap = %#v", resolver.client.Transport)
	}
}

func TestCompressedDeclaredSVGCodecsAndLimits(t *testing.T) {
	raw := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><text>bounded</text></svg>`)
	for _, test := range []struct {
		name     string
		compress func(*testing.T, []byte) []byte
	}{
		{name: "gzip", compress: gzipBytes},
		{name: "zlib", compress: zlibBytes},
		{name: "raw-deflate", compress: flateBytes},
		{name: "brotli", compress: brotliBytes},
	} {
		t.Run(test.name, func(t *testing.T) {
			compressed := test.compress(t, raw)
			resolver := newTestResolver(t, Options{})
			resource := mustResolve(t, resolver, dataURI("image/svg+xml", compressed))
			if resource.Kind() != KindSVG || !bytes.Equal(resource.cloneBytes(), raw) {
				t.Fatalf("expanded resource = kind %s bytes %q", resource.Kind(), resource.cloneBytes())
			}
			if resource.EncodedBytes() != int64(len(raw)) || resource.decompressedByteCount() != int64(len(raw)) {
				t.Fatalf("expanded accounting = encoded %d decompressed %d", resource.EncodedBytes(), resource.decompressedByteCount())
			}
		})
	}

	t.Run("raw-deflate-leading-xml-byte", func(t *testing.T) {
		raw, compressed := rawDeflateStartingWithXMLByte(t)
		if len(compressed) == 0 || compressed[0] != '<' {
			t.Fatalf("raw DEFLATE prefix = %x, want 3c", compressed[:min(1, len(compressed))])
		}
		resource := mustResolve(t, newTestResolver(t, Options{}), dataURI("image/svg+xml", compressed))
		if resource.Kind() != KindSVG || !bytes.Equal(resource.cloneBytes(), raw) {
			t.Fatalf("expanded resource = kind %s bytes %q", resource.Kind(), resource.cloneBytes())
		}
	})

	t.Run("local svgz", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "asset.svgz")
		if err := os.WriteFile(path, gzipBytes(t, raw), 0o600); err != nil {
			t.Fatal(err)
		}
		resource := mustResolve(t, newTestResolver(t, Options{}), path)
		if !bytes.Equal(resource.cloneBytes(), raw) {
			t.Fatal("local svgz was not expanded")
		}
	})

	t.Run("bomb", func(t *testing.T) {
		large := append(bytes.Repeat([]byte{' '}, 32*1024), []byte(`<svg/>`)...)
		compressed := gzipBytes(t, large)
		limits := generousLimits()
		limits.MaxDecompressedBytes = int64(len(compressed) + 16)
		limits.MaxEncodedBytes = int64(len(large) + 1)
		resolver := newTestResolver(t, Options{Limits: limits})
		_, err := resolver.Resolve(context.Background(), dataURI("image/svg+xml", compressed))
		assertLimitError(t, err, "decompressed bytes")
		assertAggregate(t, resolver, 0, 0, 0)
	})

	t.Run("canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := expandDeclaredSVG(ctx, loadedSource{data: gzipBytes(t, raw)}, generousLimits())
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled expansion error = %v", err)
		}
	})
}

func rawDeflateStartingWithXMLByte(t *testing.T) ([]byte, []byte) {
	t.Helper()
	// This deterministic payload produces a sync-flushed dynamic block whose
	// first byte is both a valid raw-DEFLATE header and the ASCII '<' byte.
	raw := []byte(`<svg><!--`)
	state := uint32(1)
	for range 1_000 {
		state = state*1_664_525 + 1_013_904_223
		raw = append(raw, "abcd"[state>>30])
	}
	raw = append(raw, []byte(`--></svg>`)...)

	var output bytes.Buffer
	writer, err := flate.NewWriter(&output, flate.BestSpeed)
	if err != nil {
		t.Fatal(err)
	}
	closingRootBytes := len(`</svg>`)
	if _, err := writer.Write(raw[:len(raw)-closingRootBytes]); err != nil {
		t.Fatal(err)
	}
	if err := writer.Flush(); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(raw[len(raw)-closingRootBytes:]); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return raw, output.Bytes()
}

func TestCompressedDeclaredSVGUsesFinalEncodedSize(t *testing.T) {
	raw := []byte(`<svg/>`)
	inner := gzipBytes(t, raw)
	outer := gzipBytes(t, inner)
	if len(inner) <= len(raw) {
		t.Fatalf("compressed fixture must exceed final XML: wrapper=%d final=%d", len(inner), len(raw))
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		if request.URL.Path == "/http-encoding" {
			w.Header().Set("Content-Encoding", "gzip")
			_, _ = w.Write(outer)
			return
		}
		_, _ = w.Write(inner)
	}))
	defer server.Close()
	localPath := filepath.Join(t.TempDir(), "asset.svgz")
	if err := os.WriteFile(localPath, inner, 0o600); err != nil {
		t.Fatal(err)
	}

	limits := generousLimits()
	limits.MaxEncodedBytes = int64(len(raw))
	limits.MaxDecompressedBytes = int64(len(inner))
	for _, test := range []struct {
		name    string
		source  string
		options Options
	}{
		{name: "data", source: dataURI("image/svg+xml", inner)},
		{name: "local", source: localPath},
		{name: "http", source: server.URL + "/identity", options: Options{HTTPClient: server.Client()}},
		{name: "HTTP encoding then declared compression", source: server.URL + "/http-encoding", options: Options{HTTPClient: server.Client()}},
	} {
		t.Run(test.name, func(t *testing.T) {
			test.options.Limits = limits
			resource := mustResolve(t, newTestResolver(t, test.options), test.source)
			if !bytes.Equal(resource.cloneBytes(), raw) || resource.EncodedBytes() != int64(len(raw)) {
				t.Fatalf("resource bytes=%q encoded=%d, want final XML size %d", resource.cloneBytes(), resource.EncodedBytes(), len(raw))
			}
		})
	}

	limits.MaxEncodedBytes--
	resolver := newTestResolver(t, Options{Limits: limits})
	_, err := resolver.Resolve(context.Background(), dataURI("image/svg+xml", inner))
	assertLimitError(t, err, "encoded bytes")
	assertAggregate(t, resolver, 0, 0, 0)
}

func TestHTTPBodyURLErrorIsSealed(t *testing.T) {
	sentinel := errors.New("body sentinel")
	nested := &url.Error{
		Op:  "read",
		URL: "https://user:password@example.invalid/image?token=secret#fragment",
		Err: sentinel,
	}
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			Status:        "200 OK",
			Header:        make(http.Header),
			Body:          failingReadCloser{err: fmt.Errorf("nested body failure: %w", nested)},
			ContentLength: -1,
		}, nil
	})}
	resolver := newTestResolver(t, Options{HTTPClient: client})
	_, err := resolver.Resolve(context.Background(), "https://example.invalid/image")
	if !errors.Is(err, sentinel) {
		t.Fatalf("errors.Is lost nested sentinel: %v", err)
	}
	assertSanitizedErrorTree(t, err, "password", "token=secret", "fragment")
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		t.Fatalf("URL-bearing body error remained reachable: %#v", urlErr)
	}
}

func TestBoundedDefaultTransportHandlesNonStandardDefault(t *testing.T) {
	transport := boundedDefaultTransport(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("unused")
	}))
	if transport == nil || transport.MaxResponseHeaderBytes != maxResponseHeaderBytes {
		t.Fatalf("fallback transport = %#v", transport)
	}
}

func TestSVGErrorsDoNotEchoPayloadTokens(t *testing.T) {
	for _, data := range [][]byte{
		[]byte(`<password-secret/>`),
		[]byte(`<svg xmlns="urn:password-secret"/>`),
		[]byte(`<?password-secret?><svg/>`),
		[]byte(`<svg>&password-secret;</svg>`),
	} {
		resolver := newTestResolver(t, Options{})
		_, err := resolver.Resolve(context.Background(), dataURI("image/svg+xml", data))
		if err == nil {
			t.Fatalf("invalid SVG %q unexpectedly succeeded", data)
		}
		if strings.Contains(err.Error(), "password-secret") {
			t.Fatalf("SVG error echoed payload token: %v", err)
		}
	}
}

func TestMidstreamCancellationChecks(t *testing.T) {
	t.Run("source hash", func(t *testing.T) {
		ctx := newCancelAfterChecksContext(3)
		_, err := hashSource(ctx, strings.Repeat("x", 128<<10))
		assertCanceledAfterMultipleChecks(t, ctx, err)
	})

	t.Run("percent decoding", func(t *testing.T) {
		ctx := newCancelAfterChecksContext(3)
		_, err := readBounded(ctx, &percentDecodeReader{source: strings.Repeat("%41", 64<<10)}, 1<<20, "percent bytes")
		assertCanceledAfterMultipleChecks(t, ctx, err)
	})

	t.Run("XML validation", func(t *testing.T) {
		ctx := newCancelAfterChecksContext(4)
		data := []byte(`<svg><text>` + strings.Repeat("x", 48<<10) + `</text></svg>`)
		err := validateSVG(ctx, data, generousLimits().MaxSVGBytes)
		assertCanceledAfterMultipleChecks(t, ctx, err)
	})

	t.Run("GIF sub-block scan", func(t *testing.T) {
		ctx := newCancelAfterChecksContext(5)
		data := append([]byte("GIF89a\x01\x00\x01\x00\x00\x00\x00"), bytes.Repeat([]byte{0x21, 0xfe, 0x01, 'x', 0x00}, 64)...)
		data = append(data, 0x3b)
		_, _, err := rasterimage.Config(ctx, data, "gif")
		assertCanceledAfterMultipleChecks(t, ctx, err)
	})

	t.Run("compressed SVG expansion", func(t *testing.T) {
		ctx := newCancelAfterChecksContext(5)
		raw := []byte(`<svg>` + strings.Repeat(" ", 48<<10) + `</svg>`)
		_, err := expandDeclaredSVG(ctx, loadedSource{data: gzipBytes(t, raw)}, generousLimits())
		assertCanceledAfterMultipleChecks(t, ctx, err)
	})
}

func TestEmptyBaseDirIsFrozenAtConstruction(t *testing.T) {
	resolver := newTestResolver(t, Options{})
	workingDirectory, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	if resolver.baseDir != filepath.Clean(workingDirectory) {
		t.Fatalf("baseDir = %q, want %q", resolver.baseDir, workingDirectory)
	}
}

func mustResolve(t *testing.T, resolver *Resolver, source string) *Resource {
	t.Helper()
	resource, err := resolver.Resolve(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	return resource
}

func assertAggregate(t *testing.T, resolver *Resolver, assets int, encoded, decoded int64) {
	t.Helper()
	if resolver.assetCountSnapshot() != assets || resolver.cumulativeEncodedBytesSnapshot() != encoded || resolver.cumulativeDecodedBytesSnapshot() != decoded {
		t.Fatalf("aggregate = assets %d encoded %d decoded %d, want %d %d %d", resolver.assetCountSnapshot(), resolver.cumulativeEncodedBytesSnapshot(), resolver.cumulativeDecodedBytesSnapshot(), assets, encoded, decoded)
	}
}

func corruptPNGIDATCRC(t *testing.T, source []byte) []byte {
	t.Helper()
	corrupt := append([]byte(nil), source...)
	idat := bytes.Index(corrupt, []byte("IDAT"))
	if idat < 4 {
		t.Fatal("PNG fixture has no IDAT chunk")
	}
	length := int(binary.BigEndian.Uint32(corrupt[idat-4 : idat]))
	crcOffset := idat + 4 + length
	corrupt[crcOffset] ^= 0xff
	return corrupt
}

func assertSanitizedErrorTree(t *testing.T, err error, secrets ...string) {
	t.Helper()
	seen := make(map[error]struct{})
	queue := []error{err}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current == nil {
			continue
		}
		if _, ok := seen[current]; ok {
			continue
		}
		seen[current] = struct{}{}
		for _, secret := range secrets {
			if strings.Contains(current.Error(), secret) {
				t.Fatalf("error tree leaked %q in %T: %v", secret, current, current)
			}
		}
		switch typed := current.(type) {
		case interface{ Unwrap() []error }:
			queue = append(queue, typed.Unwrap()...)
		case interface{ Unwrap() error }:
			queue = append(queue, typed.Unwrap())
		}
	}
}

type failingReadCloser struct {
	err error
}

func (r failingReadCloser) Read([]byte) (int, error) { return 0, r.err }
func (failingReadCloser) Close() error               { return nil }

var _ io.ReadCloser = failingReadCloser{}

type cancelAfterChecksContext struct {
	context.Context
	done   chan struct{}
	after  int32
	checks atomic.Int32
	once   sync.Once
}

func newCancelAfterChecksContext(after int32) *cancelAfterChecksContext {
	return &cancelAfterChecksContext{Context: context.Background(), done: make(chan struct{}), after: after}
}

func (c *cancelAfterChecksContext) Done() <-chan struct{} {
	if c.checks.Add(1) >= c.after {
		c.once.Do(func() { close(c.done) })
	}
	return c.done
}

func (c *cancelAfterChecksContext) Err() error {
	select {
	case <-c.done:
		return context.Canceled
	default:
		return nil
	}
}

func assertCanceledAfterMultipleChecks(t *testing.T, ctx *cancelAfterChecksContext, err error) {
	t.Helper()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if ctx.checks.Load() < 2 {
		t.Fatalf("canceled before meaningful work: checks=%d", ctx.checks.Load())
	}
}
