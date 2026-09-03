package imageasset

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/d2lang/d2/internal/rasterimage"
)

func TestAnimatedGIFUsesFirstFramePolicy(t *testing.T) {
	palette := color.Palette{color.Black, color.White}
	first := image.NewPaletted(image.Rect(0, 0, 2, 2), palette)
	second := image.NewPaletted(image.Rect(0, 0, 2, 2), palette)
	for index := range first.Pix {
		first.Pix[index] = 1
	}
	var animatedGIF bytes.Buffer
	if err := gif.EncodeAll(&animatedGIF, &gif.GIF{
		Image: []*image.Paletted{first, second},
		Delay: []int{0, 0},
	}); err != nil {
		t.Fatal(err)
	}

	resolver := newTestResolver(t, Options{})
	resource, err := resolver.Resolve(context.Background(), dataURI("image/gif", animatedGIF.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if resource.MIMEType() != "image/gif" || resource.PixelWidth() != 2 || resource.PixelHeight() != 2 || resource.DecodedBytes() != 16 {
		t.Fatalf("animated GIF resource = MIME %q %dx%d decoded=%d", resource.MIMEType(), resource.PixelWidth(), resource.PixelHeight(), resource.DecodedBytes())
	}
	if resolver.cumulativeDecodedBytesSnapshot() != 16 {
		t.Fatalf("animation charged %d decoded bytes, want one logical canvas (16)", resolver.cumulativeDecodedBytesSnapshot())
	}
}

func TestIncompleteAnimationContainersAreMalformed(t *testing.T) {
	staticPNG := encodePNG(t, 2, 2)
	incompleteAPNG := insertPNGChunk(t, staticPNG, "acTL", []byte{0, 0, 0, 2, 0, 0, 0, 0})
	incompleteWebP := make([]byte, 30)
	copy(incompleteWebP, "RIFF")
	binary.LittleEndian.PutUint32(incompleteWebP[4:8], 22)
	copy(incompleteWebP[8:12], "WEBP")
	copy(incompleteWebP[12:16], "VP8X")
	binary.LittleEndian.PutUint32(incompleteWebP[16:20], 10)
	incompleteWebP[20] = 0x02
	for _, test := range []struct {
		name string
		mime string
		data []byte
	}{
		{name: "apng", mime: "image/png", data: incompleteAPNG},
		{name: "webp", mime: "image/webp", data: incompleteWebP},
	} {
		t.Run(test.name, func(t *testing.T) {
			resolver := newTestResolver(t, Options{})
			if _, err := resolver.Resolve(context.Background(), dataURI(test.mime, test.data)); err == nil || !strings.Contains(err.Error(), "malformed") {
				t.Fatalf("Resolve error = %v, want malformed animation", err)
			}
			if resolver.cumulativeDecodedBytesSnapshot() != 0 {
				t.Fatalf("malformed animation charged %d bytes", resolver.cumulativeDecodedBytesSnapshot())
			}
		})
	}
}

func TestSixteenBitPNGChargesEightBytesPerPixel(t *testing.T) {
	img := image.NewNRGBA64(image.Rect(0, 0, 3, 2))
	img.SetNRGBA64(0, 0, color.NRGBA64{R: 0x1234, G: 0x5678, B: 0x9abc, A: 0xffff})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	resolver := newTestResolver(t, Options{})
	resource, err := resolver.Resolve(context.Background(), dataURI("image/png", encoded.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if resource.DecodedBytes() != 3*2*8 {
		t.Fatalf("decoded bytes = %d, want 48", resource.DecodedBytes())
	}
}

func TestDataURIMetadataIsBoundedBeforeParsing(t *testing.T) {
	resolver := newTestResolver(t, Options{})
	source := "data:" + strings.Repeat("x", maxDataURIMetadataBytes+1) + ",payload"
	_, err := resolver.Resolve(context.Background(), source)
	assertLimitError(t, err, "data URI metadata bytes")
}

func TestSVGValidationWorkIsBounded(t *testing.T) {
	t.Run("bytes before XML parsing", func(t *testing.T) {
		const svgBytes = 64 << 10
		data := append([]byte(`<svg>`), bytes.Repeat([]byte{'x'}, svgBytes)...)
		limits := generousLimits()
		limits.MaxSVGBytes = svgBytes
		resolver := newTestResolver(t, Options{Limits: limits})
		_, err := resolver.Resolve(context.Background(), dataURI("image/svg+xml", data))
		assertLimitError(t, err, "SVG bytes")
	})

	t.Run("depth", func(t *testing.T) {
		limits := validationLimitsForSVG(1 << 20)
		limits.depth = 32
		exact := []byte(`<svg>` + strings.Repeat(`<g>`, limits.depth-1) + strings.Repeat(`</g>`, limits.depth-1) + `</svg>`)
		if err := validateSVGWithLimits(context.Background(), exact, limits); err != nil {
			t.Fatalf("exact depth failed: %v", err)
		}
		tooDeep := []byte(`<svg>` + strings.Repeat(`<g>`, limits.depth) + strings.Repeat(`</g>`, limits.depth) + `</svg>`)
		assertLimitError(t, validateSVGWithLimits(context.Background(), tooDeep, limits), "SVG XML depth")
	})

	t.Run("elements", func(t *testing.T) {
		limits := validationLimitsForSVG(1 << 20)
		limits.elements = 256
		exact := []byte(`<svg>` + strings.Repeat(`<g/>`, int(limits.elements)-1) + `</svg>`)
		if err := validateSVGWithLimits(context.Background(), exact, limits); err != nil {
			t.Fatalf("exact elements failed: %v", err)
		}
		tooMany := []byte(`<svg>` + strings.Repeat(`<g/>`, int(limits.elements)) + `</svg>`)
		assertLimitError(t, validateSVGWithLimits(context.Background(), tooMany, limits), "SVG elements")
	})

	t.Run("attributes", func(t *testing.T) {
		limits := validationLimitsForSVG(1 << 20)
		limits.attributes = 512
		var exact strings.Builder
		exact.WriteString(`<svg`)
		for index := range int(limits.attributes) {
			fmt.Fprintf(&exact, ` a%d=""`, index)
		}
		exact.WriteString(`/>`)
		if err := validateSVGWithLimits(context.Background(), []byte(exact.String()), limits); err != nil {
			t.Fatalf("exact attributes failed: %v", err)
		}
		tooMany := strings.Replace(exact.String(), `/>`, fmt.Sprintf(` a%d=""/>`, limits.attributes), 1)
		assertLimitError(t, validateSVGWithLimits(context.Background(), []byte(tooMany), limits), "SVG attributes")
	})

	t.Run("tokens", func(t *testing.T) {
		limits := validationLimitsForSVG(1 << 20)
		limits.tokens = 4_096
		exact := []byte(`<svg>` + strings.Repeat(`<!---->`, int(limits.tokens)-2) + `</svg>`)
		if err := validateSVGWithLimits(context.Background(), exact, limits); err != nil {
			t.Fatalf("exact tokens failed: %v", err)
		}
		tooMany := []byte(`<svg>` + strings.Repeat(`<!---->`, int(limits.tokens)-1) + `</svg>`)
		assertLimitError(t, validateSVGWithLimits(context.Background(), tooMany, limits), "SVG XML tokens")
	})
}

func TestDecodeReservationRollsBackOnMalformedBody(t *testing.T) {
	corrupt := append([]byte(nil), encodePNG(t, 2, 2)...)
	idat := bytes.Index(corrupt, []byte("IDAT"))
	if idat < 0 {
		t.Fatal("encoded PNG has no IDAT chunk")
	}
	length := int(binary.BigEndian.Uint32(corrupt[idat-4 : idat]))
	crcOffset := idat + 4 + length
	corrupt[crcOffset] ^= 0xff
	resolver := newTestResolver(t, Options{})
	_, err := resolver.Resolve(context.Background(), dataURI("image/png", corrupt))
	if err == nil || !strings.Contains(err.Error(), "malformed PNG") {
		t.Fatalf("Resolve error = %v, want malformed PNG", err)
	}
	if resolver.assetCountSnapshot() != 0 || resolver.cumulativeEncodedBytesSnapshot() != 0 || resolver.cumulativeDecodedBytesSnapshot() != 0 {
		t.Fatalf("failed decode retained reservation: assets=%d encoded=%d decoded=%d", resolver.assetCountSnapshot(), resolver.cumulativeEncodedBytesSnapshot(), resolver.cumulativeDecodedBytesSnapshot())
	}
}

func TestMalformedPixelDataIsDeferredPastResolution(t *testing.T) {
	corrupt := corruptPNGPixelData(t, encodePNG(t, 2, 2))
	resolver := newTestResolver(t, Options{})
	resource, err := resolver.Resolve(context.Background(), dataURI("image/png", corrupt))
	if err != nil {
		t.Fatalf("Resolve rejected structurally valid PNG before pixel decode: %v", err)
	}
	if resource.PixelWidth() != 2 || resource.PixelHeight() != 2 || resource.DecodedBytes() != 16 {
		t.Fatalf("resolved malformed-pixel resource = %dx%d decoded=%d", resource.PixelWidth(), resource.PixelHeight(), resource.DecodedBytes())
	}
	if _, err := rasterimage.DecodeFirst(context.Background(), corrupt, "png"); err == nil {
		t.Fatal("corrupt PNG pixel entropy unexpectedly decoded")
	}
}

func corruptPNGPixelData(t *testing.T, data []byte) []byte {
	t.Helper()
	result := append([]byte(nil), data...)
	marker := bytes.Index(result, []byte("IDAT"))
	if marker < 4 {
		t.Fatal("PNG fixture has no IDAT chunk")
	}
	length := int(binary.BigEndian.Uint32(result[marker-4 : marker]))
	if length < 4 || marker+4+length+4 > len(result) {
		t.Fatalf("PNG fixture has invalid IDAT length %d", length)
	}
	result[marker+4+length-1] ^= 0xff
	checksum := crc32.ChecksumIEEE(result[marker : marker+4+length])
	binary.BigEndian.PutUint32(result[marker+4+length:marker+4+length+4], checksum)
	return result
}

func TestDefaultHTTPTimeoutAndInjectedClient(t *testing.T) {
	defaultResolver := newTestResolver(t, Options{})
	if defaultResolver.client.Timeout != time.Minute {
		t.Fatalf("default HTTP timeout = %v, want 1m", defaultResolver.client.Timeout)
	}
	injected := &http.Client{}
	injectedResolver := newTestResolver(t, Options{HTTPClient: injected})
	if injectedResolver.client.Timeout != 0 {
		t.Fatalf("injected HTTP timeout changed to %v", injectedResolver.client.Timeout)
	}
}

func TestMalformedHTTPURLRedactsSecrets(t *testing.T) {
	resolver := newTestResolver(t, Options{})
	_, err := resolver.Resolve(context.Background(), "http://user:password@example.com/%zz?token=secret#fragment")
	if err == nil {
		t.Fatal("expected malformed URL error")
	}
	for _, secret := range []string{"password", "token=secret", "fragment"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("malformed URL error leaked %q: %v", secret, err)
		}
	}
}

func TestNetworkPathReferenceRejectsWithoutLeakingFullErrorTree(t *testing.T) {
	resolver := newTestResolver(t, Options{})
	_, err := resolver.Resolve(context.Background(), "//user:password@example.com/icon.png?token=secret#fragment")
	if err == nil || !strings.Contains(err.Error(), "network-path references") {
		t.Fatalf("Resolve error = %v, want actionable network-path rejection", err)
	}
	assertSanitizedErrorTree(t, err,
		"user:password", "password", "example.com", "token=secret", "fragment",
	)
}

func insertPNGChunk(t *testing.T, source []byte, chunkType string, payload []byte) []byte {
	t.Helper()
	idat := bytes.Index(source, []byte("IDAT"))
	if idat < 4 || len(chunkType) != 4 {
		t.Fatal("invalid PNG fixture or chunk type")
	}
	insertion := idat - 4
	chunk := make([]byte, 12+len(payload))
	binary.BigEndian.PutUint32(chunk[:4], uint32(len(payload)))
	copy(chunk[4:8], chunkType)
	copy(chunk[8:], payload)
	binary.BigEndian.PutUint32(chunk[len(chunk)-4:], crc32.ChecksumIEEE(chunk[4:len(chunk)-4]))
	output := make([]byte, 0, len(source)+len(chunk))
	output = append(output, source[:insertion]...)
	output = append(output, chunk...)
	output = append(output, source[insertion:]...)
	return output
}

func TestCumulativeConcurrentReservation(t *testing.T) {
	first := encodePNG(t, 2, 2)
	second := append([]byte(nil), first...)
	// A harmless trailing byte makes the canonical data-URI identities distinct;
	// Go's PNG decoder intentionally tolerates trailing bytes.
	second = append(second, 0)
	limits := generousLimits()
	limits.MaxCumulativeDecodedBytes = 16
	resolver := newTestResolver(t, Options{Limits: limits})
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, source := range []string{dataURI("image/png", first), dataURI("image/png", second)} {
		source := source
		go func() {
			<-start
			_, err := resolver.Resolve(context.Background(), source)
			results <- err
		}()
	}
	close(start)
	var successes, limited int
	for range 2 {
		err := <-results
		if err == nil {
			successes++
			continue
		}
		var limitErr *LimitError
		if errors.As(err, &limitErr) && limitErr.Name == "cumulative decoded bytes" {
			limited++
			continue
		}
		t.Fatalf("unexpected Resolve error: %v", err)
	}
	if successes != 1 || limited != 1 || resolver.cumulativeDecodedBytesSnapshot() != 16 {
		t.Fatalf("successes=%d limited=%d cumulative=%d", successes, limited, resolver.cumulativeDecodedBytesSnapshot())
	}
	if bytes.Equal(first, second) {
		t.Fatal("test sources unexpectedly have the same identity")
	}
}

func TestConcurrentSameSourceLoadsOnce(t *testing.T) {
	const goroutines = 16
	raw := encodePNG(t, 2, 2)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write(raw)
	}))
	defer server.Close()
	resolver := newTestResolver(t, Options{HTTPClient: server.Client()})
	start := make(chan struct{})
	type result struct {
		resource *Resource
		err      error
	}
	results := make(chan result, goroutines)
	for range goroutines {
		go func() {
			<-start
			resource, err := resolver.Resolve(context.Background(), server.URL)
			results <- result{resource: resource, err: err}
		}()
	}
	close(start)
	var snapshot *Resource
	for range goroutines {
		result := <-results
		if result.err != nil {
			t.Fatal(result.err)
		}
		if snapshot == nil {
			snapshot = result.resource
		} else if result.resource != snapshot {
			t.Fatal("concurrent callers received different resource snapshots")
		}
	}
	if requests.Load() != 1 {
		t.Fatalf("requests = %d, want 1", requests.Load())
	}
	if resolver.cumulativeDecodedBytesSnapshot() != 16 {
		t.Fatalf("cumulative decoded bytes = %d, want 16", resolver.cumulativeDecodedBytesSnapshot())
	}
	if resolver.assetCountSnapshot() != 1 || resolver.cumulativeEncodedBytesSnapshot() != int64(len(raw)) {
		t.Fatalf("aggregate charge = assets %d encoded %d, want 1 and %d", resolver.assetCountSnapshot(), resolver.cumulativeEncodedBytesSnapshot(), len(raw))
	}
}

func TestResolverMemoizesSuccessfulResourceSnapshot(t *testing.T) {
	raw := encodePNG(t, 2, 2)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write(raw)
	}))
	defer server.Close()
	resolver := newTestResolver(t, Options{HTTPClient: server.Client()})
	first, err := resolver.Resolve(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	second, err := resolver.Resolve(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("same resolver did not return its immutable resource snapshot")
	}
	if requests.Load() != 1 || resolver.assetCountSnapshot() != 1 || resolver.cumulativeEncodedBytesSnapshot() != int64(len(raw)) || resolver.cumulativeDecodedBytesSnapshot() != 16 {
		t.Fatalf("requests=%d assets=%d encoded=%d decoded=%d", requests.Load(), resolver.assetCountSnapshot(), resolver.cumulativeEncodedBytesSnapshot(), resolver.cumulativeDecodedBytesSnapshot())
	}
	newResolver := newTestResolver(t, Options{HTTPClient: server.Client()})
	third, err := newResolver.Resolve(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if third == first || requests.Load() != 2 {
		t.Fatalf("new resolver reused an unshared resource: same=%v requests=%d", third == first, requests.Load())
	}
}

func TestMemoryCacheBoundedEviction(t *testing.T) {
	raw := encodePNG(t, 2, 2)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write(raw)
	}))
	defer server.Close()

	cache, err := NewMemoryCache(2, int64(2*len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/one", "/two", "/one", "/three", "/two"} {
		resolver := newTestResolver(t, Options{HTTPClient: server.Client(), Cache: cache})
		if _, err := resolver.Resolve(context.Background(), server.URL+path); err != nil {
			t.Fatal(err)
		}
	}
	if requests.Load() != 4 {
		t.Fatalf("requests = %d, want 4 after deterministic LRU eviction", requests.Load())
	}

	tinyCache, err := NewMemoryCache(2, int64(len(raw)-1))
	if err != nil {
		t.Fatal(err)
	}
	tinyResolver := newTestResolver(t, Options{HTTPClient: server.Client(), Cache: tinyCache})
	if _, err := tinyResolver.Resolve(context.Background(), server.URL+"/large"); err != nil {
		t.Fatal(err)
	}
	tinyResolver = newTestResolver(t, Options{HTTPClient: server.Client(), Cache: tinyCache})
	if _, err := tinyResolver.Resolve(context.Background(), server.URL+"/large"); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 6 {
		t.Fatalf("oversize cache entry was retained; requests = %d", requests.Load())
	}
}

func TestSharedCacheRevalidatesResolverLimits(t *testing.T) {
	raw := encodePNG(t, 3, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(raw)
	}))
	defer server.Close()
	cache, err := NewMemoryCache(2, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	permissive := newTestResolver(t, Options{HTTPClient: server.Client(), Cache: cache})
	resource, err := permissive.Resolve(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	limits := generousLimits()
	limits.MaxFetchedBytes = resource.fetchedByteCount() - 1
	strict := newTestResolver(t, Options{HTTPClient: server.Client(), Cache: cache, Limits: limits})
	_, err = strict.Resolve(context.Background(), server.URL)
	assertLimitError(t, err, "fetched bytes")
}

func TestSharedCacheRevalidatesSVGByteLimit(t *testing.T) {
	raw := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><!--cached--></svg>`)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "image/svg+xml")
		_, _ = w.Write(raw)
	}))
	defer server.Close()
	cache, err := NewMemoryCache(2, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	permissive := newTestResolver(t, Options{HTTPClient: server.Client(), Cache: cache})
	if _, err := permissive.Resolve(context.Background(), server.URL); err != nil {
		t.Fatal(err)
	}
	limits := generousLimits()
	limits.MaxSVGBytes = int64(len(raw) - 1)
	strict := newTestResolver(t, Options{HTTPClient: server.Client(), Cache: cache, Limits: limits})
	_, err = strict.Resolve(context.Background(), server.URL)
	assertLimitError(t, err, "SVG bytes")
	if requests.Load() != 1 {
		t.Fatalf("strict resolver refetched cached SVG: requests=%d, want 1", requests.Load())
	}
}

func TestLocalRejectsNonRegularFiles(t *testing.T) {
	resolver := newTestResolver(t, Options{})
	_, err := resolver.Resolve(context.Background(), t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("directory error = %v", err)
	}
}

func TestHTTPSourceErrorsRedactCredentialsAndQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no", http.StatusInternalServerError)
	}))
	defer server.Close()
	secretURL := strings.Replace(server.URL, "://", "://user:password@", 1) + "/icon?token=secret#fragment"
	resolver := newTestResolver(t, Options{HTTPClient: server.Client()})
	_, err := resolver.Resolve(context.Background(), secretURL)
	if err == nil {
		t.Fatal("expected HTTP error")
	}
	for _, secret := range []string{"password", "token=secret", "fragment"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("HTTP error leaked %q: %v", secret, err)
		}
	}
	if !strings.Contains(err.Error(), "/icon") {
		t.Fatalf("HTTP error lost useful path context: %v", err)
	}
}

func TestHTTPStatusReasonPhraseIsNotExposed(t *testing.T) {
	t.Parallel()
	const secret = "reason-phrase-secret"
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Status:     "502 " + secret,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("ignored")),
		}, nil
	})}
	resolver := newTestResolver(t, Options{HTTPClient: client})
	_, err := resolver.Resolve(context.Background(), "https://example.invalid/image")
	if err == nil {
		t.Fatal("expected HTTP status error")
	}
	if strings.Contains(err.Error(), secret) || !strings.Contains(err.Error(), "status code 502") {
		t.Fatalf("status error = %q", err)
	}
}

func TestDataURIStrictParsingAndRawPreservation(t *testing.T) {
	raw := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><text>&amp;</text></svg>`)
	resolver := newTestResolver(t, Options{})
	resource, err := resolver.Resolve(context.Background(), "data:image/svg+xml,"+percentEncode(raw))
	if err != nil {
		t.Fatal(err)
	}
	if string(resource.cloneBytes()) != string(raw) {
		t.Fatalf("raw SVG changed: %q", resource.cloneBytes())
	}
	for _, source := range []string{
		"data:image/png;base64;base64,AAAA",
		"data:image/png;base64;charset=utf-8,AAAA",
	} {
		_, err := resolver.Resolve(context.Background(), source)
		if err == nil || !strings.Contains(err.Error(), "base64 marker") {
			t.Fatalf("data URI error = %v", err)
		}
	}
}

func TestContentEncodingLayerBound(t *testing.T) {
	raw := encodePNG(t, 1, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Add("Content-Encoding", "identity, identity")
		w.Header().Add("Content-Encoding", "identity, identity, identity")
		_, _ = w.Write(raw)
	}))
	defer server.Close()
	resolver := newTestResolver(t, Options{HTTPClient: server.Client()})
	_, err := resolver.Resolve(context.Background(), server.URL)
	if err == nil || !strings.Contains(err.Error(), "layer count") {
		t.Fatalf("content-encoding error = %v", err)
	}
}

func TestFailedAndCanceledResolvesDoNotConsumeBudgetOrCache(t *testing.T) {
	cache, err := NewMemoryCache(2, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	resolver := newTestResolver(t, Options{Cache: cache})
	_, err = resolver.Resolve(context.Background(), dataURI("image/png", []byte("not png")))
	if err == nil {
		t.Fatal("expected malformed image error")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = resolver.Resolve(ctx, dataURI("image/png", encodePNG(t, 1, 1)))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled error = %v", err)
	}
	if resolver.cumulativeDecodedBytesSnapshot() != 0 {
		t.Fatalf("failed resolve charged %d decoded bytes", resolver.cumulativeDecodedBytesSnapshot())
	}
	if len(cache.entries) != 0 {
		t.Fatalf("failed resolve cached %d entries", len(cache.entries))
	}
}

func TestCanceledContextAvoidsLocalFilesystemAccess(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "image.png")
	if err := os.WriteFile(path, encodePNG(t, 1, 1), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	resolver := newTestResolver(t, Options{})
	_, err := resolver.Resolve(ctx, path)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Resolve error = %v, want context.Canceled", err)
	}
}

func percentEncode(data []byte) string {
	const hexadecimal = "0123456789ABCDEF"
	var output strings.Builder
	for _, value := range data {
		if value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9' || strings.ContainsRune("-._~", rune(value)) {
			output.WriteByte(value)
		} else {
			output.WriteByte('%')
			output.WriteByte(hexadecimal[value>>4])
			output.WriteByte(hexadecimal[value&0x0f])
		}
	}
	return output.String()
}
