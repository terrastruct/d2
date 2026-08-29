package imageasset

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
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
	"time"

	"github.com/andybalholm/brotli"
)

func TestResolveLocalRelativeAbsoluteAndOwnedBytes(t *testing.T) {
	directory := t.TempDir()
	data := encodePNG(t, 2, 3)
	path := filepath.Join(directory, "icon&amp;one.bin")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	resolver := newTestResolver(t, Options{BaseDir: directory})

	relative, err := resolver.Resolve(context.Background(), "icon&amp;one.bin")
	if err != nil {
		t.Fatal(err)
	}
	assertResource(t, relative, KindRaster, "image/png", 2, 3)
	if relative.fetchedByteCount() != int64(len(data)) || relative.decompressedByteCount() != int64(len(data)) || relative.DecodedBytes() != 24 {
		t.Fatalf("unexpected byte accounting: fetched=%d decompressed=%d decoded=%d", relative.fetchedByteCount(), relative.decompressedByteCount(), relative.DecodedBytes())
	}
	copyOne := relative.cloneBytes()
	copyOne[0] ^= 0xff
	if bytes.Equal(copyOne, relative.cloneBytes()) {
		t.Fatal("Resource.Bytes exposed mutable backing data")
	}

	absolute, err := resolver.Resolve(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	assertResource(t, absolute, KindRaster, "image/png", 2, 3)
}

func TestResourceBytesContextReturnsOwnedBytesAndHonorsCancellation(t *testing.T) {
	resource := &Resource{data: bytes.Repeat([]byte{0x5a}, 64<<10)}
	copyBytes, err := resource.BytesContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	copyBytes[0] = 0
	if resource.data[0] != 0x5a {
		t.Fatal("BytesContext exposed mutable resource storage")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if canceledBytes, err := resource.BytesContext(ctx); !errors.Is(err, context.Canceled) || canceledBytes != nil {
		t.Fatalf("canceled BytesContext = %v/%v, want nil/context.Canceled", canceledBytes, err)
	}
}

func TestLocalCacheKeysIncludeResolvedBaseDirectory(t *testing.T) {
	cache, err := NewMemoryCache(8, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	firstDirectory := t.TempDir()
	secondDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(firstDirectory, "icon.png"), encodePNG(t, 2, 3), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(secondDirectory, "icon.png"), encodePNG(t, 4, 5), 0o600); err != nil {
		t.Fatal(err)
	}
	first := newTestResolver(t, Options{BaseDir: firstDirectory, Cache: cache})
	second := newTestResolver(t, Options{BaseDir: secondDirectory, Cache: cache})
	firstResource, err := first.Resolve(context.Background(), "icon.png")
	if err != nil {
		t.Fatal(err)
	}
	secondResource, err := second.Resolve(context.Background(), "icon.png")
	if err != nil {
		t.Fatal(err)
	}
	if firstResource.PixelWidth() != 2 || secondResource.PixelWidth() != 4 {
		t.Fatalf("cache collision across base directories: %dx%d, %dx%d", firstResource.PixelWidth(), firstResource.PixelHeight(), secondResource.PixelWidth(), secondResource.PixelHeight())
	}
}

func TestResolveRasterDataURIFormats(t *testing.T) {
	webpData, err := base64.StdEncoding.DecodeString(testWebPBase64)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		data      []byte
		declared  string
		mimeType  string
		width     int
		height    int
		rawBase64 bool
	}{
		{name: "png", data: encodePNG(t, 2, 3), declared: "application/octet-stream", mimeType: "image/png", width: 2, height: 3},
		{name: "jpeg", data: encodeJPEG(t, 3, 4), declared: "image/jpg;charset=binary", mimeType: "image/jpeg", width: 3, height: 4},
		{name: "gif", data: encodeGIF(t, 4, 5), declared: "", mimeType: "image/gif", width: 4, height: 5},
		{name: "webp", data: webpData, declared: "image/webp", mimeType: "image/webp", width: 75, height: 100, rawBase64: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encoding := base64.StdEncoding
			if test.rawBase64 {
				encoding = base64.RawStdEncoding
			}
			payload := encoding.EncodeToString(test.data)
			resolver := newTestResolver(t, Options{})
			resource, err := resolver.Resolve(context.Background(), "data:"+test.declared+";base64,"+payload)
			if err != nil {
				t.Fatal(err)
			}
			assertResource(t, resource, KindRaster, test.mimeType, test.width, test.height)
		})
	}
}

func TestResolveStrictRawSVGDoesNotFetchNestedResources(t *testing.T) {
	var requests atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return nil, errors.New("nested network fetch")
	})}
	raw := []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"><image href="https://example.invalid/image.png"/><text>&amp;</text></svg>`)
	source := "data:text/xml;charset=utf-8," + url.PathEscape(string(raw))
	resolver := newTestResolver(t, Options{HTTPClient: client})
	resource, err := resolver.Resolve(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	assertResource(t, resource, KindSVG, "image/svg+xml", 0, 0)
	if !bytes.Equal(resource.cloneBytes(), raw) {
		t.Fatal("SVG raw bytes changed")
	}
	if resource.DecodedBytes() != int64(len(raw)) {
		t.Fatalf("SVG decoded bytes = %d, want %d", resource.DecodedBytes(), len(raw))
	}
	if requests.Load() != 0 {
		t.Fatalf("resolver made %d nested SVG requests", requests.Load())
	}
}

func TestResolveCanonicalSVGDoctypePreservesBytesAndEntitiesStayDisabled(t *testing.T) {
	raw := []byte(`<?xml version="1.0" encoding="utf-8"?>
<!-- Generator: Adobe Illustrator -->
<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd">
<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"><rect width="10" height="10"/></svg>`)
	resolver := newTestResolver(t, Options{})
	resource, err := resolver.Resolve(context.Background(), dataURI("image/svg+xml", raw))
	if err != nil {
		t.Fatal(err)
	}
	assertResource(t, resource, KindSVG, "image/svg+xml", 0, 0)
	if !bytes.Equal(resource.cloneBytes(), raw) {
		t.Fatal("SVG source bytes changed")
	}

	withEntity := bytes.Replace(raw, []byte(`</svg>`), []byte(`&bomb;</svg>`), 1)
	if _, err := resolver.Resolve(context.Background(), dataURI("image/svg+xml", withEntity)); err == nil || !strings.Contains(err.Error(), "malformed SVG") {
		t.Fatalf("SVG with undeclared entity error = %v", err)
	}

	isoRaw := []byte(`<?xml version="1.0" encoding="iso-8859-1"?><!--caf`)
	isoRaw = append(isoRaw, 0xe9)
	isoRaw = append(isoRaw, []byte(`--><!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd"><svg/>`)...)
	resource, err = resolver.Resolve(context.Background(), dataURI("image/svg+xml", isoRaw))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(resource.cloneBytes(), isoRaw) {
		t.Fatal("ISO-8859-1 SVG source bytes changed")
	}
}

func TestResolveISO88591SVGPreservesSourceBytesAndRejectsDTD(t *testing.T) {
	raw := []byte(`<?xml version="1.0" encoding="iso-8859-1"?><svg xmlns="http://www.w3.org/2000/svg"><title>caf`)
	raw = append(raw, 0xe9)
	raw = append(raw, []byte(`</title></svg>`)...)
	resolver := newTestResolver(t, Options{})
	resource, err := resolver.Resolve(context.Background(), "data:image/svg+xml;base64,"+base64.StdEncoding.EncodeToString(raw))
	if err != nil {
		t.Fatal(err)
	}
	assertResource(t, resource, KindSVG, "image/svg+xml", 0, 0)
	if !bytes.Equal(resource.cloneBytes(), raw) {
		t.Fatal("ISO-8859-1 SVG source bytes changed")
	}

	withDTD := []byte(`<?xml version="1.0" encoding="iso-8859-1"?><!DOCTYPE svg SYSTEM "https://example.invalid/svg.dtd"><svg/>`)
	_, err = resolver.Resolve(context.Background(), "data:image/svg+xml;base64,"+base64.StdEncoding.EncodeToString(withDTD))
	if err == nil || !strings.Contains(err.Error(), "unsupported SVG") || !strings.Contains(err.Error(), "DTD") || strings.Contains(err.Error(), "malformed SVG") || strings.Contains(err.Error(), "compressed fallback") || strings.Contains(err.Error(), "brotli") {
		t.Fatalf("ISO-8859-1 SVG with DTD error = %v", err)
	}
	_, err = resolver.Resolve(context.Background(), dataURI("image/svg+xml", gzipBytes(t, withDTD)))
	if err == nil || !strings.Contains(err.Error(), "unsupported SVG") || !strings.Contains(err.Error(), "DTD") || strings.Contains(err.Error(), "malformed SVG") || strings.Contains(err.Error(), "compressed fallback") {
		t.Fatalf("compressed SVG with DTD error = %v", err)
	}

	unsupported := []byte(`<?xml version="1.0" encoding="windows-1252"?><svg/>`)
	_, err = resolver.Resolve(context.Background(), "data:image/svg+xml;base64,"+base64.StdEncoding.EncodeToString(unsupported))
	if err == nil || !strings.Contains(err.Error(), "malformed SVG") {
		t.Fatalf("unsupported XML encoding error = %v", err)
	}
}

func TestResolveHTTPRedirectAndContentSniffing(t *testing.T) {
	pngData := encodePNG(t, 6, 7)
	var sawAcceptEncoding atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/redirect":
			http.Redirect(w, request, "/image", http.StatusFound)
		case "/image":
			sawAcceptEncoding.Store(request.Header.Get("Accept-Encoding") == "gzip, deflate, br")
			w.Header().Set("Content-Type", "image/jpg; charset=binary")
			_, _ = w.Write(pngData)
		default:
			http.NotFound(w, request)
		}
	}))
	defer server.Close()
	var redirects atomic.Int32
	client := server.Client()
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		redirects.Add(1)
		return nil
	}
	resolver := newTestResolver(t, Options{HTTPClient: client})
	resource, err := resolver.Resolve(context.Background(), server.URL+"/redirect")
	if err != nil {
		t.Fatal(err)
	}
	assertResource(t, resource, KindRaster, "image/png", 6, 7)
	if redirects.Load() != 1 || !sawAcceptEncoding.Load() {
		t.Fatalf("redirects=%d accept-encoding=%v", redirects.Load(), sawAcceptEncoding.Load())
	}

	redirectErr := errors.New("redirect denied")
	denyingClient := server.Client()
	denyingClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return redirectErr }
	denyingResolver := newTestResolver(t, Options{HTTPClient: denyingClient})
	_, err = denyingResolver.Resolve(context.Background(), server.URL+"/redirect")
	if !errors.Is(err, redirectErr) {
		t.Fatalf("redirect error = %v, want %v", err, redirectErr)
	}
	if errors.Is(err, ErrUnavailable) {
		t.Fatalf("redirect policy error classified as ErrUnavailable: %v", err)
	}
}

func TestResolveHTTPContentEncodings(t *testing.T) {
	raw := encodePNG(t, 3, 2)
	tests := []struct {
		name     string
		header   string
		compress func(*testing.T, []byte) []byte
	}{
		{name: "gzip", header: "gzip", compress: gzipBytes},
		{name: "x-gzip", header: "x-gzip", compress: gzipBytes},
		{name: "deflate-zlib", header: "deflate", compress: zlibBytes},
		{name: "deflate-raw", header: "deflate", compress: flateBytes},
		{name: "brotli", header: "br", compress: brotliBytes},
		{name: "stacked", header: "gzip, br", compress: func(t *testing.T, value []byte) []byte {
			return brotliBytes(t, gzipBytes(t, value))
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			compressed := test.compress(t, raw)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Encoding", test.header)
				w.Header().Set("Content-Type", "application/octet-stream")
				_, _ = w.Write(compressed)
			}))
			defer server.Close()
			limits := generousLimits()
			limits.MaxFetchedBytes = int64(len(compressed))
			limits.MaxEncodedBytes = int64(len(raw))
			limits.MaxDecompressedBytes = int64(len(raw))
			if test.name == "stacked" {
				// Each decoded layer is bounded; the intermediate gzip stream is
				// larger than the final tiny PNG.
				limits.MaxDecompressedBytes = int64(len(gzipBytes(t, raw)))
			}
			resolver := newTestResolver(t, Options{HTTPClient: server.Client(), Limits: limits})
			resource, err := resolver.Resolve(context.Background(), server.URL)
			if err != nil {
				t.Fatal(err)
			}
			assertResource(t, resource, KindRaster, "image/png", 3, 2)
			if resource.fetchedByteCount() != int64(len(compressed)) || resource.decompressedByteCount() != int64(len(raw)) {
				t.Fatalf("byte accounting = fetched %d, decompressed %d", resource.fetchedByteCount(), resource.decompressedByteCount())
			}
		})
	}
}

func TestOptionalCacheAndCumulativeCharging(t *testing.T) {
	raw := encodePNG(t, 2, 3)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write(raw)
	}))
	defer server.Close()
	cache, err := NewMemoryCache(8, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	resolver := newTestResolver(t, Options{HTTPClient: server.Client(), Cache: cache})
	first, err := resolver.Resolve(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	second, err := resolver.Resolve(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("cache did not return the immutable resource")
	}
	if requests.Load() != 1 {
		t.Fatalf("HTTP requests = %d, want 1", requests.Load())
	}
	if resolver.cumulativeDecodedBytesSnapshot() != 24 {
		t.Fatalf("cumulative decoded bytes = %d, want 24", resolver.cumulativeDecodedBytesSnapshot())
	}

	firstUncached := newTestResolver(t, Options{HTTPClient: server.Client()})
	if _, err := firstUncached.Resolve(context.Background(), server.URL); err != nil {
		t.Fatal(err)
	}
	secondUncached := newTestResolver(t, Options{HTTPClient: server.Client()})
	if _, err := secondUncached.Resolve(context.Background(), server.URL); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 3 {
		t.Fatalf("HTTP requests after uncached resolves = %d, want 3", requests.Load())
	}
}

func TestSharedCacheSkipsInlineDataButRetainsLocalResources(t *testing.T) {
	raw := encodePNG(t, 2, 3)
	cache, err := NewMemoryCache(8, 1<<20)
	if err != nil {
		t.Fatal(err)
	}

	inline := dataURI("image/png", raw)
	firstInline := newTestResolver(t, Options{Cache: cache})
	first, err := firstInline.Resolve(context.Background(), inline)
	if err != nil {
		t.Fatal(err)
	}
	if second, err := firstInline.Resolve(context.Background(), inline); err != nil || second != first {
		t.Fatalf("same-resolver inline memoization = %p/%v, want %p/nil", second, err, first)
	}
	if len(cache.entries) != 0 {
		t.Fatalf("inline data retained %d shared cache entries, want 0", len(cache.entries))
	}
	secondInline := newTestResolver(t, Options{Cache: cache})
	second, err := secondInline.Resolve(context.Background(), inline)
	if err != nil {
		t.Fatal(err)
	}
	if second == first || len(cache.entries) != 0 {
		t.Fatalf("cross-resolver inline resource was retained: same=%v entries=%d", second == first, len(cache.entries))
	}

	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "icon.png"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	localOne := newTestResolver(t, Options{BaseDir: directory, Cache: cache})
	localFirst, err := localOne.Resolve(context.Background(), "icon.png")
	if err != nil {
		t.Fatal(err)
	}
	localTwo := newTestResolver(t, Options{BaseDir: directory, Cache: cache})
	localSecond, err := localTwo.Resolve(context.Background(), "icon.png")
	if err != nil {
		t.Fatal(err)
	}
	if localSecond != localFirst || len(cache.entries) != 1 {
		t.Fatalf("local shared cache = same %v entries %d, want true/1", localSecond == localFirst, len(cache.entries))
	}
}

func TestResolverLimits(t *testing.T) {
	raw := encodePNG(t, 4, 3)
	tests := []struct {
		name      string
		configure func(*Limits)
		wantLimit string
	}{
		{name: "fetched", configure: func(l *Limits) { l.MaxFetchedBytes = int64(len(raw) - 1) }, wantLimit: "fetched bytes"},
		{name: "encoded", configure: func(l *Limits) { l.MaxEncodedBytes = int64(len(raw) - 1) }, wantLimit: "encoded bytes"},
		{name: "decompressed", configure: func(l *Limits) { l.MaxDecompressedBytes = int64(len(raw) - 1) }, wantLimit: "decompressed bytes"},
		{name: "width", configure: func(l *Limits) { l.MaxDecodedWidth = 3 }, wantLimit: "decoded width"},
		{name: "height", configure: func(l *Limits) { l.MaxDecodedHeight = 2 }, wantLimit: "decoded height"},
		{name: "pixels", configure: func(l *Limits) { l.MaxDecodedPixels = 11 }, wantLimit: "decoded pixels"},
		{name: "single decoded footprint", configure: func(l *Limits) { l.MaxCumulativeDecodedBytes = 47 }, wantLimit: "decoded bytes"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "image.png")
			if err := os.WriteFile(path, raw, 0o600); err != nil {
				t.Fatal(err)
			}
			limits := generousLimits()
			test.configure(&limits)
			resolver := newTestResolver(t, Options{Limits: limits})
			_, err := resolver.Resolve(context.Background(), path)
			assertLimitError(t, err, test.wantLimit)
		})
	}

	t.Run("cumulative", func(t *testing.T) {
		directory := t.TempDir()
		firstPath := filepath.Join(directory, "first.png")
		secondPath := filepath.Join(directory, "second.png")
		if err := os.WriteFile(firstPath, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(secondPath, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		limits := generousLimits()
		limits.MaxCumulativeDecodedBytes = 95
		resolver := newTestResolver(t, Options{Limits: limits})
		if _, err := resolver.Resolve(context.Background(), firstPath); err != nil {
			t.Fatal(err)
		}
		_, err := resolver.Resolve(context.Background(), secondPath)
		assertLimitError(t, err, "cumulative decoded bytes")
		if resolver.cumulativeDecodedBytesSnapshot() != 48 {
			t.Fatalf("failed resolve changed cumulative bytes to %d", resolver.cumulativeDecodedBytesSnapshot())
		}
	})

	t.Run("inclusive", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "image.png")
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		limits := generousLimits()
		limits.MaxFetchedBytes = int64(len(raw))
		limits.MaxEncodedBytes = int64(len(raw))
		limits.MaxDecompressedBytes = int64(len(raw))
		limits.MaxDecodedWidth = 4
		limits.MaxDecodedHeight = 3
		limits.MaxDecodedPixels = 12
		limits.MaxCumulativeDecodedBytes = 48
		resolver := newTestResolver(t, Options{Limits: limits})
		if _, err := resolver.Resolve(context.Background(), path); err != nil {
			t.Fatal(err)
		}
	})
}

func TestHTTPDecompressionAndEncodedLimits(t *testing.T) {
	raw := encodePNG(t, 20, 20)
	compressed := gzipBytes(t, raw)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write(compressed)
	}))
	defer server.Close()

	limits := generousLimits()
	limits.MaxFetchedBytes = int64(len(compressed))
	limits.MaxDecompressedBytes = int64(len(raw) - 1)
	resolver := newTestResolver(t, Options{HTTPClient: server.Client(), Limits: limits})
	_, err := resolver.Resolve(context.Background(), server.URL)
	assertLimitError(t, err, "decompressed bytes")

	limits.MaxDecompressedBytes = int64(len(raw))
	limits.MaxEncodedBytes = int64(len(raw) - 1)
	resolver = newTestResolver(t, Options{HTTPClient: server.Client(), Limits: limits})
	_, err = resolver.Resolve(context.Background(), server.URL)
	assertLimitError(t, err, "encoded bytes")

	t.Run("streamed fetched bytes", func(t *testing.T) {
		streaming := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "image/png")
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			_, _ = w.Write(raw)
		}))
		defer streaming.Close()
		limits := generousLimits()
		limits.MaxFetchedBytes = int64(len(raw) - 1)
		resolver := newTestResolver(t, Options{HTTPClient: streaming.Client(), Limits: limits})
		_, err := resolver.Resolve(context.Background(), streaming.URL)
		assertLimitError(t, err, "fetched bytes")
	})
}

func TestMalformedAndUnsupportedSources(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{name: "invalid base64", source: "data:image/png;base64,!!!!", want: "base64"},
		{name: "truncated png", source: dataURI("image/png", []byte("\x89PNG\r\n\x1a\n")), want: "malformed PNG"},
		{name: "truncated jpeg", source: dataURI("image/jpeg", []byte("\xff\xd8\xff")), want: "malformed JPEG"},
		{name: "truncated gif", source: dataURI("image/gif", []byte("GIF89a")), want: "malformed GIF"},
		{name: "truncated webp", source: dataURI("image/webp", []byte("RIFF\x00\x00\x00\x00WEBP")), want: "malformed WEBP"},
		{name: "unsupported bytes", source: dataURI("image/bmp", []byte("not an image")), want: "unsupported or malformed"},
		{name: "malformed svg", source: dataURI("image/svg+xml", []byte(`<svg><g></svg>`)), want: "malformed SVG"},
		{name: "svg doctype", source: dataURI("image/svg+xml", []byte(`<!DOCTYPE svg><svg/>`)), want: "unsupported SVG"},
		{name: "svg doctype mismatched identifiers", source: dataURI("image/svg+xml", []byte(`<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "https://example.invalid/svg11.dtd"><svg/>`)), want: "unsupported SVG"},
		{name: "svg doctype internal subset", source: dataURI("image/svg+xml", []byte(`<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd" [<!ENTITY bomb "boom">]><svg/>`)), want: "unsupported SVG"},
		{name: "svg doctype comment-spliced", source: dataURI("image/svg+xml", []byte(`<!DOCTYPE<!--comment--> svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd"><svg/>`)), want: "unsupported SVG"},
		{name: "svg doctype duplicate", source: dataURI("image/svg+xml", []byte(`<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd"><!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd"><svg/>`)), want: "unsupported SVG"},
		{name: "svg doctype late", source: dataURI("image/svg+xml", []byte(`<svg/><!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd">`)), want: "unsupported SVG"},
		{name: "XML declaration after doctype", source: dataURI("image/svg+xml", []byte(`<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd"><?xml version="1.0"?><svg/>`)), want: "processing instruction"},
		{name: "duplicate XML declaration", source: dataURI("image/svg+xml", []byte(`<?xml version="1.0"?><?xml version="1.0"?><svg/>`)), want: "processing instruction"},
		{name: "wrong svg root", source: dataURI("image/svg+xml", []byte(`<html/>`)), want: "root element"},
		{name: "unsupported scheme", source: "ftp://example.com/image.png", want: "unsupported URI scheme"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolver := newTestResolver(t, Options{})
			_, err := resolver.Resolve(context.Background(), test.source)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Resolve error = %v, want substring %q", err, test.want)
			}
			var sourceErr *SourceError
			if !errors.As(err, &sourceErr) {
				t.Fatalf("error %T is not SourceError", err)
			}
			if strings.HasPrefix(test.source, "data:") && strings.Contains(err.Error(), base64.StdEncoding.EncodeToString([]byte("not an image"))) {
				t.Fatal("data URI payload leaked into error")
			}
		})
	}
}

func TestHTTPFailuresHaveSourceContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/encoding" {
			w.Header().Set("Content-Encoding", "compress")
			_, _ = w.Write([]byte("body"))
			return
		}
		http.Error(w, "no", http.StatusNotFound)
	}))
	defer server.Close()
	for _, path := range []string{"/missing", "/encoding"} {
		resolver := newTestResolver(t, Options{HTTPClient: server.Client()})
		_, err := resolver.Resolve(context.Background(), server.URL+path)
		if err == nil || !strings.Contains(err.Error(), server.URL+path) {
			t.Fatalf("error lacks source context: %v", err)
		}
	}

	resolver := newTestResolver(t, Options{BaseDir: t.TempDir()})
	_, err := resolver.Resolve(context.Background(), "missing.png")
	var sourceErr *SourceError
	if !errors.As(err, &sourceErr) || sourceErr.Source != "missing.png" || sourceErr.Op != "load" {
		t.Fatalf("local source error = %#v (%v)", sourceErr, err)
	}
}

func TestResolveClassifiesUnavailableSources(t *testing.T) {
	t.Run("local file is absent", func(t *testing.T) {
		resolver := newTestResolver(t, Options{BaseDir: t.TempDir()})
		_, err := resolver.Resolve(context.Background(), "missing.png")
		if !errors.Is(err, ErrUnavailable) {
			t.Fatalf("Resolve error = %v, want ErrUnavailable", err)
		}
		var sourceErr *SourceError
		if !errors.As(err, &sourceErr) || sourceErr.Op != "load" {
			t.Fatalf("Resolve error = %T %v, want load SourceError", err, err)
		}
	})

	t.Run("HTTP status is unavailable", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "missing", http.StatusNotFound)
		}))
		defer server.Close()
		resolver := newTestResolver(t, Options{HTTPClient: server.Client()})
		_, err := resolver.Resolve(context.Background(), server.URL+"/missing.png")
		if !errors.Is(err, ErrUnavailable) {
			t.Fatalf("Resolve error = %v, want ErrUnavailable", err)
		}
	})

	t.Run("transport failure is unavailable", func(t *testing.T) {
		transportErr := errors.New("transport unavailable")
		client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, transportErr
		})}
		resolver := newTestResolver(t, Options{HTTPClient: client})
		_, err := resolver.Resolve(context.Background(), "https://example.invalid/image.png")
		if !errors.Is(err, ErrUnavailable) || !errors.Is(err, transportErr) {
			t.Fatalf("Resolve error = %v, want ErrUnavailable wrapping transport error", err)
		}
	})

	t.Run("transport limit stays fatal", func(t *testing.T) {
		limitErr := &LimitError{Name: "transport bytes", Actual: 2, Limit: 1}
		client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, limitErr
		})}
		resolver := newTestResolver(t, Options{HTTPClient: client})
		_, err := resolver.Resolve(context.Background(), "https://example.invalid/image.png")
		if !errors.Is(err, limitErr) || errors.Is(err, ErrUnavailable) {
			t.Fatalf("Resolve error = %v, want transport limit without ErrUnavailable", err)
		}
	})

	t.Run("malformed fetched bytes stay fatal", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("not a PNG"))
		}))
		defer server.Close()
		resolver := newTestResolver(t, Options{HTTPClient: server.Client()})
		_, err := resolver.Resolve(context.Background(), server.URL+"/malformed.png")
		if err == nil || errors.Is(err, ErrUnavailable) {
			t.Fatalf("Resolve error = %v, want non-availability failure", err)
		}
	})
}

func TestCancellation(t *testing.T) {
	t.Run("already canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		resolver := newTestResolver(t, Options{})
		_, err := resolver.Resolve(ctx, dataURI("image/png", encodePNG(t, 1, 1)))
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Resolve error = %v, want context.Canceled", err)
		}
		if errors.Is(err, ErrUnavailable) {
			t.Fatalf("caller cancellation classified as ErrUnavailable: %v", err)
		}
	})

	t.Run("HTTP in flight", func(t *testing.T) {
		started := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
			close(started)
			<-request.Context().Done()
		}))
		defer server.Close()
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			<-started
			cancel()
		}()
		resolver := newTestResolver(t, Options{HTTPClient: server.Client()})
		_, err := resolver.Resolve(ctx, server.URL)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Resolve error = %v, want context.Canceled", err)
		}
		if errors.Is(err, ErrUnavailable) {
			t.Fatalf("caller cancellation classified as ErrUnavailable: %v", err)
		}
	})

	t.Run("HTTP body in flight", func(t *testing.T) {
		started := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			w.Header().Set("Content-Type", "image/png")
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			close(started)
			<-request.Context().Done()
		}))
		defer server.Close()
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			<-started
			cancel()
		}()
		resolver := newTestResolver(t, Options{HTTPClient: server.Client()})
		_, err := resolver.Resolve(ctx, server.URL)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Resolve error = %v, want context.Canceled", err)
		}
		if errors.Is(err, ErrUnavailable) {
			t.Fatalf("caller cancellation classified as ErrUnavailable: %v", err)
		}
	})
}

func TestMemoryCacheConcurrentUse(t *testing.T) {
	const goroutines = 24
	raw := encodePNG(t, 2, 2)
	limits := generousLimits()
	limits.MaxCumulativeDecodedBytes = goroutines * 16
	cache, err := NewMemoryCache(4, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	resolver := newTestResolver(t, Options{Cache: cache, Limits: limits})
	source := dataURI("image/png", raw)
	var wait sync.WaitGroup
	errorsChannel := make(chan error, goroutines)
	for range goroutines {
		wait.Add(1)
		go func() {
			defer wait.Done()
			resource, err := resolver.Resolve(context.Background(), source)
			if err == nil && !bytes.Equal(resource.cloneBytes(), raw) {
				err = errors.New("cached bytes changed")
			}
			errorsChannel <- err
		}()
	}
	wait.Wait()
	close(errorsChannel)
	for err := range errorsChannel {
		if err != nil {
			t.Fatal(err)
		}
	}
	if resolver.cumulativeDecodedBytesSnapshot() != 16 {
		t.Fatalf("cumulative decoded bytes = %d", resolver.cumulativeDecodedBytesSnapshot())
	}
}

func TestCacheHitsAndInflightWaitersDoNotAcquireResolutionSlots(t *testing.T) {
	raw := encodePNG(t, 2, 2)
	path := filepath.Join(t.TempDir(), "image.png")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	cache, err := NewMemoryCache(4, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	prime := newTestResolver(t, Options{Cache: cache, CacheNamespace: "slot-test"})
	if _, err := prime.Resolve(context.Background(), path); err != nil {
		t.Fatal(err)
	}

	t.Run("process cache hit", func(t *testing.T) {
		resolver := newTestResolver(t, Options{Cache: cache, CacheNamespace: "slot-test"})
		fillResolutionSlots(resolver)
		defer drainResolutionSlots(resolver)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if _, err := resolver.Resolve(ctx, path); err != nil {
			t.Fatalf("cache hit waited for a resolution slot: %v", err)
		}
	})

	t.Run("duplicate in flight", func(t *testing.T) {
		resolver := newTestResolver(t, Options{})
		spec, err := resolver.classifySource(context.Background(), path)
		if err != nil {
			t.Fatal(err)
		}
		call := &resolveCall{done: make(chan struct{})}
		resolver.mu.Lock()
		resolver.inflight[spec.key] = call
		resolver.mu.Unlock()
		fillResolutionSlots(resolver)
		defer drainResolutionSlots(resolver)

		result := make(chan error, 1)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			resource, resolveErr := resolver.Resolve(ctx, path)
			if resolveErr == nil && resource == nil {
				resolveErr = errors.New("in-flight waiter returned nil resource")
			}
			result <- resolveErr
		}()
		call.resource = &Resource{kind: KindRaster, mimeType: "image/png", data: raw, pixelWidth: 2, pixelHeight: 2, decodedBytes: 16}
		close(call.done)
		if err := <-result; err != nil {
			t.Fatalf("in-flight waiter acquired a resolution slot: %v", err)
		}
	})
}

func fillResolutionSlots(resolver *Resolver) {
	for range cap(resolver.resolveSlots) {
		resolver.resolveSlots <- struct{}{}
	}
}

func drainResolutionSlots(resolver *Resolver) {
	for len(resolver.resolveSlots) != 0 {
		<-resolver.resolveSlots
	}
}

func TestLimitsMustBeCallerSupplied(t *testing.T) {
	_, err := New(Options{})
	if err == nil || !strings.Contains(err.Error(), "every limit") {
		t.Fatalf("New error = %v", err)
	}
}

func newTestResolver(t *testing.T, options Options) *Resolver {
	t.Helper()
	if options.Limits == (Limits{}) {
		options.Limits = generousLimits()
	}
	if options.Cache != nil && options.CacheNamespace == "" {
		options.CacheNamespace = "imageasset-test"
	}
	resolver, err := New(options)
	if err != nil {
		t.Fatal(err)
	}
	return resolver
}

func generousLimits() Limits {
	return Limits{
		MaxFetchedBytes:           4 << 20,
		MaxEncodedBytes:           4 << 20,
		MaxDecompressedBytes:      4 << 20,
		MaxSVGBytes:               4 << 20,
		MaxDecodedWidth:           10_000,
		MaxDecodedHeight:          10_000,
		MaxDecodedPixels:          100_000_000,
		MaxAssets:                 10_000,
		MaxCumulativeEncodedBytes: 1 << 30,
		MaxCumulativeDecodedBytes: 1 << 30,
	}
}

func assertResource(t *testing.T, resource *Resource, kind Kind, mimeType string, width, height int) {
	t.Helper()
	if resource.Kind() != kind || resource.MIMEType() != mimeType || resource.PixelWidth() != width || resource.PixelHeight() != height {
		t.Fatalf("resource = kind %s MIME %q %dx%d, want kind %s MIME %q %dx%d", resource.Kind(), resource.MIMEType(), resource.PixelWidth(), resource.PixelHeight(), kind, mimeType, width, height)
	}
}

func assertLimitError(t *testing.T, err error, name string) {
	t.Helper()
	if errors.Is(err, ErrUnavailable) {
		t.Fatalf("limit error classified as ErrUnavailable: %v", err)
	}
	var limitErr *LimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("error = %v, want LimitError %q", err, name)
	}
	if limitErr.Name != name {
		t.Fatalf("limit name = %q, want %q (%v)", limitErr.Name, name, err)
	}
	if limitErr.Actual <= limitErr.Limit {
		t.Fatalf("non-exceeding LimitError = %+v", limitErr)
	}
}

func dataURI(mimeType string, data []byte) string {
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data)
}

func encodePNG(t *testing.T, width, height int) []byte {
	t.Helper()
	var output bytes.Buffer
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: 0x80, A: 0xff})
		}
	}
	if err := png.Encode(&output, img); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func encodeJPEG(t *testing.T, width, height int) []byte {
	t.Helper()
	var output bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	if err := jpeg.Encode(&output, img, nil); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func encodeGIF(t *testing.T, width, height int) []byte {
	t.Helper()
	var output bytes.Buffer
	img := image.NewPaletted(image.Rect(0, 0, width, height), color.Palette{color.Black, color.White})
	if err := gif.Encode(&output, img, nil); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func gzipBytes(t *testing.T, value []byte) []byte {
	t.Helper()
	var output bytes.Buffer
	writer := gzip.NewWriter(&output)
	if _, err := writer.Write(value); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func zlibBytes(t *testing.T, value []byte) []byte {
	t.Helper()
	var output bytes.Buffer
	writer := zlib.NewWriter(&output)
	if _, err := writer.Write(value); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func flateBytes(t *testing.T, value []byte) []byte {
	t.Helper()
	var output bytes.Buffer
	writer, err := flate.NewWriter(&output, flate.DefaultCompression)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(value); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func brotliBytes(t *testing.T, value []byte) []byte {
	t.Helper()
	var output bytes.Buffer
	writer := brotli.NewWriter(&output)
	if _, err := writer.Write(value); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestReadBoundedHandlesReaderErrors(t *testing.T) {
	want := errors.New("read failure")
	_, err := readBounded(context.Background(), io.MultiReader(strings.NewReader("ok"), errorReader{want}), 10, "bytes")
	if !errors.Is(err, want) {
		t.Fatalf("readBounded error = %v, want %v", err, want)
	}
}

type errorReader struct{ err error }

func (r errorReader) Read([]byte) (int, error) { return 0, r.err }

const testWebPBase64 = "UklGRrIBAABXRUJQVlA4TKUBAAAvSsAYAA8w//M///MfeJAkbXvaSG7m8Q3GfYSBJekwQztm/IcZlgwnmWImn2BK7aFmBtnVir6q//8VOkFE/xm4baTIu8c48ArEo6+B3zFKYln3pqClSCKX0begFTAXFOLXHSyF8cCNcZEG4OywuA4KVVfJCiArU7GAgJI8+lJP/OKMT/fBAjevg1cYB7YVkFuWga2lyPi5I0HFy5YTpWIHg0RZpkniRVW9odHAKOwosWuOGdxIyn2OvaCDvhg/we6TwadPBPbqBV58MsLmMJ8yZnOWk8SRz4N+QoyPL+MnamzMvcE1rHNEr91F9GKZPVUcS9w7PhhH36suB9qPeYb/oLk6cuTiJ0wOK3m5h1cKjW6EVZCYMK7dxcKCBdgP9HkKr9gkAO2P8GKZGWVdIAatQa+1IDpt6qyorVwdy01xdW8Jkfk6xjEXmVQQ+HQdFr6OKhIN34dXWq0+0qr6EJSCeeVLH9+gvGTLyqM65PQ44ihzlTXxQKjKbAvshXgir7Lil9w4L2bvMycmjQcqXaMCO6BlY28i+FOLzbfI1vEqxAhotocAAA=="
