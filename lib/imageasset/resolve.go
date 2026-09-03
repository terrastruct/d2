package imageasset

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/andybalholm/brotli"
)

type sourceKind uint8

const (
	sourceLocal sourceKind = iota + 1
	sourceHTTP
	sourceData
)

type sourceSpec struct {
	kind  sourceKind
	raw   string
	label string
	key   string
	path  string
	url   string
}

type loadedSource struct {
	data              []byte
	mimeHint          string
	fetchedBytes      int64
	decompressedBytes int64
}

type resolveCall struct {
	done     chan struct{}
	resource *Resource
	err      error
}

// Data-URI media-type parameters are metadata, not fetched image bytes. Bound
// them separately before hashing or splitting attacker-controlled input.
const maxDataURIMetadataBytes = 4 << 10

const maxLocatorBytes = 64 << 10

const (
	maxContentEncodingHeaderBytes = 256
	maxContentEncodingTokens      = 4
	maxContentTypeHeaderBytes     = 1024
)

// Resolve resolves one image source and charges its decoded footprint before
// returning. Supported inputs are relative/absolute local paths, http(s) URLs,
// and data URIs. Animated GIF, APNG, and WebP resources retain their original
// bytes; native raster consumers deterministically paint the first animation
// frame on the format's logical canvas.
func (r *Resolver) Resolve(ctx context.Context, source string) (*Resource, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := checkContext(ctx); err != nil {
		return nil, sourceError(displaySource(source), "resolve", err)
	}
	spec, err := r.classifySource(ctx, source)
	if err != nil {
		return nil, sourceError(displaySource(source), "classify source", err)
	}
	if err := checkContext(ctx); err != nil {
		return nil, sourceError(spec.label, "resolve", err)
	}
	for {
		r.mu.Lock()
		if resource := r.resolvedSources[spec.key]; resource != nil {
			r.mu.Unlock()
			return resource, nil
		}
		if call := r.inflight[spec.key]; call != nil {
			r.mu.Unlock()
			select {
			case <-ctx.Done():
				return nil, sourceError(spec.label, "resolve", ctx.Err())
			case <-call.done:
				if call.err == nil {
					if err := checkContext(ctx); err != nil {
						return nil, sourceError(spec.label, "resolve", err)
					}
					return call.resource, nil
				}
				// A failed leader (including one canceled by its own caller) does
				// not poison another caller's independent context.
				continue
			}
		}
		call := &resolveCall{done: make(chan struct{})}
		r.inflight[spec.key] = call
		r.mu.Unlock()

		resource, err := r.resolveOne(ctx, spec)
		// Inline resources are already bundled. Keep same-resolver memoization,
		// but do not retain changing data URIs in the process-wide IMG_CACHE;
		// this preserves imgbundler behavior in watch mode.
		if err == nil && r.cache != nil && spec.kind != sourceData {
			r.cache.Put(r.cachePrefix+spec.key, resource)
		}

		r.mu.Lock()
		if err == nil {
			r.resolvedSources[spec.key] = resource
		}
		call.resource = resource
		call.err = err
		delete(r.inflight, spec.key)
		close(call.done)
		r.mu.Unlock()
		return resource, err
	}
}

func (r *Resolver) resolveOne(ctx context.Context, spec sourceSpec) (*Resource, error) {
	if r.cache != nil && spec.kind != sourceData {
		if resource, ok := r.cache.Get(r.cachePrefix + spec.key); ok {
			if resource == nil {
				return nil, sourceError(spec.label, "read cache", errors.New("cache returned a nil resource"))
			}
			if err := r.validateCached(resource); err != nil {
				return nil, sourceError(spec.label, "validate cached resource", err)
			}
			if err := checkContext(ctx); err != nil {
				return nil, sourceError(spec.label, "resolve", err)
			}
			rollback, err := r.reserve(int64(len(resource.data)), resource.decodedBytes)
			if err != nil {
				return nil, sourceError(spec.label, "reserve resource", err)
			}
			if err := checkContext(ctx); err != nil {
				rollback()
				return nil, sourceError(spec.label, "resolve", err)
			}
			return resource, nil
		}
	}
	// Only the elected loader consumes a resolution slot. Same-resolver hits,
	// process-cache hits, and callers waiting on an in-flight canonical source
	// do not reduce capacity available to unrelated resources.
	select {
	case r.resolveSlots <- struct{}{}:
		defer func() { <-r.resolveSlots }()
	case <-ctx.Done():
		return nil, sourceError(spec.label, "wait for resolution slot", ctx.Err())
	}

	loaded, err := r.load(ctx, spec)
	if err != nil {
		return nil, sourceError(spec.label, "load", err)
	}
	if err := checkContext(ctx); err != nil {
		return nil, sourceError(spec.label, "resolve", err)
	}
	resource, err := probeResource(ctx, loaded, r.limits, func(encodedBytes, decodedBytes int64) (func(), error) {
		return r.reserve(encodedBytes, decodedBytes)
	})
	if err != nil {
		return nil, sourceError(spec.label, "probe", err)
	}
	return resource, nil
}

func (r *Resolver) classifySource(ctx context.Context, source string) (sourceSpec, error) {
	raw := source
	if raw == "" {
		return sourceSpec{}, errors.New("empty source")
	}
	if hasPrefixFold(raw, "data:") {
		remainder := raw[len("data:"):]
		metadataScanBytes := len(remainder)
		if metadataScanBytes > maxDataURIMetadataBytes+1 {
			metadataScanBytes = maxDataURIMetadataBytes + 1
		}
		comma := strings.IndexByte(remainder[:metadataScanBytes], ',')
		if comma < 0 {
			if len(remainder) > maxDataURIMetadataBytes {
				return sourceSpec{}, &LimitError{Name: "data URI metadata bytes", Actual: maxDataURIMetadataBytes + 1, Limit: maxDataURIMetadataBytes}
			}
			return sourceSpec{}, errors.New("data URI has no comma separator")
		}
		payloadBytes := len(remainder) - comma - 1
		if int64(payloadBytes) > r.limits.MaxFetchedBytes {
			return sourceSpec{}, &LimitError{Name: "fetched bytes", Actual: int64(payloadBytes), Limit: r.limits.MaxFetchedBytes}
		}
		digest, err := hashSource(ctx, raw)
		if err != nil {
			return sourceSpec{}, err
		}
		return sourceSpec{kind: sourceData, raw: raw, label: displaySource(raw), key: "data:" + digest}, nil
	}
	if len(raw) > maxLocatorBytes {
		return sourceSpec{}, &LimitError{Name: "source locator bytes", Actual: int64(len(raw)), Limit: maxLocatorBytes}
	}
	if strings.HasPrefix(raw, "//") {
		return sourceSpec{}, errors.New("network-path references beginning with // are unsupported; use an explicit http:// or https:// URL")
	}
	if hasPrefixFold(raw, "http://") || hasPrefixFold(raw, "https://") {
		parsed, err := url.Parse(raw)
		if err != nil {
			return sourceSpec{}, sanitizeError("invalid HTTP URL", err)
		}
		if parsed.Host == "" {
			return sourceSpec{}, errors.New("HTTP URL has no host")
		}
		parsed.Fragment = ""
		parsed.RawFragment = ""
		canonicalURL := parsed.String()
		digest, err := hashSource(ctx, canonicalURL)
		if err != nil {
			return sourceSpec{}, err
		}
		return sourceSpec{kind: sourceHTTP, raw: raw, label: displaySource(canonicalURL), key: "http:" + digest, url: canonicalURL}, nil
	}
	if scheme, ok := uriScheme(raw); ok && !filepath.IsAbs(raw) {
		if _, err := url.Parse(raw); err != nil {
			return sourceSpec{}, sanitizeError("invalid "+scheme+" URI", err)
		}
		return sourceSpec{}, fmt.Errorf("unsupported URI scheme %q", scheme)
	}
	localPath := raw
	if !filepath.IsAbs(localPath) && r.baseDir != "" {
		localPath = filepath.Join(r.baseDir, localPath)
	}
	absolute, err := filepath.Abs(localPath)
	if err != nil {
		return sourceSpec{}, err
	}
	absolute = filepath.Clean(absolute)
	return sourceSpec{kind: sourceLocal, raw: raw, label: raw, key: "file:" + absolute, path: absolute}, nil
}

func displaySource(source string) string {
	if hasPrefixFold(source, "data:") {
		metadata := source[len("data:"):]
		if len(metadata) > 129 {
			metadata = metadata[:129]
		}
		metadata, _, _ = strings.Cut(metadata, ",")
		mediaType, _, _ := strings.Cut(metadata, ";")
		mediaType = strings.TrimSpace(mediaType)
		if !safeMediaTypeLabel(mediaType) {
			return "data URI"
		}
		return "data:" + mediaType
	}
	if strings.HasPrefix(source, "//") {
		return "<network-path reference>"
	}
	if len(source) > maxLocatorBytes {
		return "<oversized source locator>"
	}
	if _, ok := uriScheme(source); ok && !filepath.IsAbs(source) {
		if parsed, err := url.Parse(source); err == nil {
			if parsed.Opaque != "" {
				return parsed.Scheme + ":<redacted>"
			}
			parsed.User = nil
			parsed.RawQuery = ""
			parsed.ForceQuery = false
			parsed.Fragment = ""
			parsed.RawFragment = ""
			return parsed.String()
		}
		return "<malformed URI>"
	}
	return source
}

func safeMediaTypeLabel(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	slashes := 0
	for index := 0; index < len(value); index++ {
		character := value[index]
		switch {
		case character >= 'a' && character <= 'z',
			character >= 'A' && character <= 'Z',
			character >= '0' && character <= '9',
			character == '.', character == '+', character == '-', character == '_':
		case character == '/':
			slashes++
			if index == 0 || index == len(value)-1 {
				return false
			}
		default:
			return false
		}
	}
	return slashes == 1
}

func uriScheme(value string) (string, bool) {
	if value == "" || value[0] < 'A' || value[0] > 'Z' && (value[0] < 'a' || value[0] > 'z') {
		return "", false
	}
	for index := 1; index < len(value); index++ {
		character := value[index]
		if character == ':' {
			return value[:index], true
		}
		if character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z' ||
			character >= '0' && character <= '9' || character == '+' || character == '-' || character == '.' {
			continue
		}
		return "", false
	}
	return "", false
}

func hashSource(ctx context.Context, source string) (string, error) {
	hash := sha256.New()
	for offset := 0; offset < len(source); {
		if err := checkContext(ctx); err != nil {
			return "", err
		}
		end := offset + 32*1024
		if end > len(source) {
			end = len(source)
		}
		_, _ = io.WriteString(hash, source[offset:end])
		offset = end
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func hasPrefixFold(value, prefix string) bool {
	return len(value) >= len(prefix) && strings.EqualFold(value[:len(prefix)], prefix)
}

func (r *Resolver) load(ctx context.Context, spec sourceSpec) (loadedSource, error) {
	switch spec.kind {
	case sourceLocal:
		return r.loadLocal(ctx, spec)
	case sourceHTTP:
		return r.loadHTTP(ctx, spec)
	case sourceData:
		return r.loadData(ctx, spec)
	default:
		return loadedSource{}, errors.New("unknown source kind")
	}
}

func (r *Resolver) loadLocal(ctx context.Context, spec sourceSpec) (loadedSource, error) {
	if err := checkContext(ctx); err != nil {
		return loadedSource{}, err
	}
	info, err := os.Stat(spec.path)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			err = ctxErr
		} else if errors.Is(err, os.ErrNotExist) {
			err = unavailable(err)
		}
		return loadedSource{}, fmt.Errorf("stat local file %q: %w", spec.path, err)
	}
	if !info.Mode().IsRegular() {
		return loadedSource{}, fmt.Errorf("local path %q is not a regular file", spec.path)
	}
	if info.Size() > r.limits.MaxFetchedBytes {
		return loadedSource{}, &LimitError{Name: "fetched bytes", Actual: info.Size(), Limit: r.limits.MaxFetchedBytes}
	}
	file, err := os.Open(spec.path)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			err = ctxErr
		} else if errors.Is(err, os.ErrNotExist) {
			err = unavailable(err)
		}
		return loadedSource{}, fmt.Errorf("open local file %q: %w", spec.path, err)
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil {
		return loadedSource{}, fmt.Errorf("stat opened local file %q: %w", spec.path, err)
	}
	if !openedInfo.Mode().IsRegular() {
		return loadedSource{}, fmt.Errorf("opened local path %q is not a regular file", spec.path)
	}
	if openedInfo.Size() > r.limits.MaxFetchedBytes {
		return loadedSource{}, &LimitError{Name: "fetched bytes", Actual: openedInfo.Size(), Limit: r.limits.MaxFetchedBytes}
	}
	data, err := readBounded(ctx, file, r.limits.MaxFetchedBytes, "fetched bytes")
	if err != nil {
		return loadedSource{}, err
	}
	if int64(len(data)) > r.limits.MaxDecompressedBytes {
		return loadedSource{}, &LimitError{Name: "decompressed bytes", Actual: int64(len(data)), Limit: r.limits.MaxDecompressedBytes}
	}
	mimeHint := mime.TypeByExtension(strings.ToLower(filepath.Ext(spec.path)))
	if strings.EqualFold(filepath.Ext(spec.path), ".svgz") {
		mimeHint = "image/svg+xml"
	}
	return loadedSource{
		data:              data,
		mimeHint:          mimeHint,
		fetchedBytes:      int64(len(data)),
		decompressedBytes: int64(len(data)),
	}, nil
}

func (r *Resolver) loadHTTP(ctx context.Context, spec sourceSpec) (loadedSource, error) {
	requestContext, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	request, err := http.NewRequestWithContext(requestContext, http.MethodGet, spec.url, nil)
	if err != nil {
		return loadedSource{}, sanitizeError("create HTTP request failed", err)
	}
	request.Header.Set("User-Agent", "Mozilla/5.0 (compatible; D2 imageasset)")
	request.Header.Set("Accept", "image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
	// Setting this explicitly prevents net/http from transparently handling only
	// gzip, allowing the same bounded path to handle gzip, deflate, and Brotli.
	request.Header.Set("Accept-Encoding", "gzip, deflate, br")

	response, err := r.client.Do(request)
	if err != nil {
		var limitErr *LimitError
		if ctxErr := ctx.Err(); ctxErr != nil {
			err = ctxErr
		} else if response == nil && !errors.As(err, &limitErr) {
			// net/http returns both a response and an error only when the
			// caller's redirect policy rejects a request. Preserve that policy
			// error and transport-enforced limits instead of treating either as
			// transport unavailability.
			err = unavailable(err)
		}
		return loadedSource{}, sanitizeError("HTTP request failed", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		if err := ctx.Err(); err != nil {
			return loadedSource{}, err
		}
		// A custom transport can supply an arbitrary Status string. Report only
		// the numeric code so an untrusted reason phrase cannot reach diagnostics.
		return loadedSource{}, unavailable(fmt.Errorf("expected status 200 but got status code %d", response.StatusCode))
	}
	if response.ContentLength > r.limits.MaxFetchedBytes {
		return loadedSource{}, &LimitError{Name: "fetched bytes", Actual: response.ContentLength, Limit: r.limits.MaxFetchedBytes}
	}
	contentType, err := selectContentType(ctx, response.Header.Values("Content-Type"))
	if err != nil {
		return loadedSource{}, err
	}
	contentEncodings, err := parseContentEncodings(ctx, response.Header.Values("Content-Encoding"))
	if err != nil {
		return loadedSource{}, err
	}
	fetched, err := readBounded(ctx, response.Body, r.limits.MaxFetchedBytes, "fetched bytes")
	if err != nil {
		var limitErr *LimitError
		if ctxErr := ctx.Err(); ctxErr != nil {
			err = ctxErr
		} else if !errors.As(err, &limitErr) {
			err = unavailable(err)
		}
		return loadedSource{}, sanitizeURLError("read HTTP response body failed", err)
	}
	decoded, err := decodeContentEncodings(ctx, fetched, contentEncodings, r.limits.MaxDecompressedBytes)
	if err != nil {
		return loadedSource{}, err
	}
	return loadedSource{
		data:              decoded,
		mimeHint:          contentType,
		fetchedBytes:      int64(len(fetched)),
		decompressedBytes: int64(len(decoded)),
	}, nil
}

type sanitizedError struct {
	message  string
	original error
}

func (e *sanitizedError) Error() string { return e.message }

// Is preserves sentinel matching without exposing a URL-bearing original
// error through Unwrap or errors.As.
func (e *sanitizedError) Is(target error) bool { return errors.Is(e.original, target) }

func sanitizeError(message string, original error) error {
	return &sanitizedError{message: message, original: original}
}

func sanitizeURLError(message string, original error) error {
	var urlErr *url.Error
	if errors.As(original, &urlErr) {
		return sanitizeError(message, original)
	}
	return original
}

func (r *Resolver) loadData(ctx context.Context, spec sourceSpec) (loadedSource, error) {
	metadata, payload, ok := strings.Cut(spec.raw[len("data:"):], ",")
	if !ok {
		return loadedSource{}, errors.New("data URI has no comma separator")
	}
	if int64(len(payload)) > r.limits.MaxFetchedBytes {
		return loadedSource{}, &LimitError{Name: "fetched bytes", Actual: int64(len(payload)), Limit: r.limits.MaxFetchedBytes}
	}
	parts := strings.Split(metadata, ";")
	mimeHint := strings.TrimSpace(parts[0])
	base64Encoded := false
	for index, parameter := range parts[1:] {
		if strings.EqualFold(strings.TrimSpace(parameter), "base64") {
			if base64Encoded || index != len(parts[1:])-1 {
				return loadedSource{}, errors.New("data URI base64 marker must occur once and last")
			}
			base64Encoded = true
		}
	}
	decodedPayloadLimit := r.limits.MaxFetchedBytes
	if !base64Encoded && r.limits.MaxDecompressedBytes < decodedPayloadLimit {
		decodedPayloadLimit = r.limits.MaxDecompressedBytes
	}
	decodedPayload, err := readBounded(ctx, &percentDecodeReader{source: payload}, decodedPayloadLimit, "data URI decoded payload bytes")
	if err != nil {
		return loadedSource{}, fmt.Errorf("decode data URI escapes: %w", err)
	}
	var data []byte
	if base64Encoded {
		encoding := base64.StdEncoding
		base64Bytes := 0
		for offset, value := range decodedPayload {
			if offset%(32*1024) == 0 {
				if err := checkContext(ctx); err != nil {
					return loadedSource{}, err
				}
			}
			if value != '\r' && value != '\n' {
				base64Bytes++
			}
		}
		if base64Bytes%4 != 0 {
			encoding = base64.RawStdEncoding
		}
		decodedLimit := r.limits.MaxFetchedBytes
		if r.limits.MaxDecompressedBytes < decodedLimit {
			decodedLimit = r.limits.MaxDecompressedBytes
		}
		data, err = readBounded(ctx, base64.NewDecoder(encoding, bytes.NewReader(decodedPayload)), decodedLimit, "decompressed bytes")
		if err != nil {
			return loadedSource{}, fmt.Errorf("decode data URI base64: %w", err)
		}
	} else {
		data = decodedPayload
	}
	if int64(len(data)) > r.limits.MaxDecompressedBytes {
		return loadedSource{}, &LimitError{Name: "decompressed bytes", Actual: int64(len(data)), Limit: r.limits.MaxDecompressedBytes}
	}
	return loadedSource{
		data:              data,
		mimeHint:          mimeHint,
		fetchedBytes:      int64(len(payload)),
		decompressedBytes: int64(len(data)),
	}, nil
}

type percentDecodeReader struct {
	source string
	offset int
}

func (r *percentDecodeReader) Read(destination []byte) (int, error) {
	if len(destination) == 0 {
		return 0, nil
	}
	written := 0
	for written < len(destination) && r.offset < len(r.source) {
		value := r.source[r.offset]
		if value != '%' {
			destination[written] = value
			r.offset++
			written++
			continue
		}
		if len(r.source)-r.offset < 3 {
			return written, errors.New("incomplete percent escape")
		}
		high, highOK := hexNibble(r.source[r.offset+1])
		low, lowOK := hexNibble(r.source[r.offset+2])
		if !highOK || !lowOK {
			return written, errors.New("invalid percent escape")
		}
		destination[written] = high<<4 | low
		r.offset += 3
		written++
	}
	if written == 0 && r.offset == len(r.source) {
		return 0, io.EOF
	}
	return written, nil
}

func hexNibble(value byte) (byte, bool) {
	switch {
	case value >= '0' && value <= '9':
		return value - '0', true
	case value >= 'a' && value <= 'f':
		return value - 'a' + 10, true
	case value >= 'A' && value <= 'F':
		return value - 'A' + 10, true
	default:
		return 0, false
	}
}

func checkByteLimits(data []byte, limits Limits) error {
	length := int64(len(data))
	if length > limits.MaxEncodedBytes {
		return &LimitError{Name: "encoded bytes", Actual: length, Limit: limits.MaxEncodedBytes}
	}
	if length > limits.MaxDecompressedBytes {
		return &LimitError{Name: "decompressed bytes", Actual: length, Limit: limits.MaxDecompressedBytes}
	}
	return nil
}

func (r *Resolver) validateCached(resource *Resource) error {
	if len(resource.data) == 0 {
		return errors.New("cached resource has no data")
	}
	switch resource.kind {
	case KindRaster:
		if resource.mimeType != "image/png" && resource.mimeType != "image/jpeg" &&
			resource.mimeType != "image/gif" && resource.mimeType != "image/webp" {
			return fmt.Errorf("cached raster has invalid MIME type %q", resource.mimeType)
		}
	case KindSVG:
		if resource.mimeType != "image/svg+xml" || resource.pixelWidth != 0 || resource.pixelHeight != 0 {
			return errors.New("cached SVG metadata is invalid")
		}
	default:
		return fmt.Errorf("cached resource has invalid kind %d", resource.kind)
	}
	if resource.fetchedBytes <= 0 || resource.fetchedBytes > r.limits.MaxFetchedBytes {
		return &LimitError{Name: "fetched bytes", Actual: resource.fetchedBytes, Limit: r.limits.MaxFetchedBytes}
	}
	if resource.decompressedBytes != int64(len(resource.data)) {
		return errors.New("cached resource decompressed-byte metadata is invalid")
	}
	if int64(len(resource.data)) > r.limits.MaxEncodedBytes {
		return &LimitError{Name: "encoded bytes", Actual: int64(len(resource.data)), Limit: r.limits.MaxEncodedBytes}
	}
	if resource.decompressedBytes > r.limits.MaxDecompressedBytes {
		return &LimitError{Name: "decompressed bytes", Actual: resource.decompressedBytes, Limit: r.limits.MaxDecompressedBytes}
	}
	if resource.kind == KindSVG {
		svgLimit, limitName := svgByteLimit(r.limits)
		if int64(len(resource.data)) > svgLimit {
			return &LimitError{Name: limitName, Actual: int64(len(resource.data)), Limit: svgLimit}
		}
	}
	if resource.kind == KindRaster {
		if resource.pixelWidth <= 0 || resource.pixelHeight <= 0 ||
			int64(resource.pixelWidth) > int64(^uint64(0)>>1)/int64(resource.pixelHeight) {
			return errors.New("cached raster dimension metadata is invalid")
		}
		pixels := int64(resource.pixelWidth) * int64(resource.pixelHeight)
		if pixels > int64(^uint64(0)>>1)/8 ||
			resource.decodedBytes != pixels*4 && resource.decodedBytes != pixels*8 {
			return errors.New("cached raster decoded-byte metadata is invalid")
		}
	} else if resource.decodedBytes != int64(len(resource.data)) {
		return errors.New("cached SVG decoded-byte metadata is invalid")
	}
	return validateDecodedLimits(resource.pixelWidth, resource.pixelHeight, resource.decodedBytes, resource.kind, r.limits)
}

// reserve atomically claims aggregate count, retained encoded bytes, and
// decoded memory before validation allocates a full raster or immutable copy.
// The returned rollback is idempotent and exact.
func (r *Resolver) reserve(encodedBytes, decodedBytes int64) (func(), error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.assetCount >= r.limits.MaxAssets {
		actual := int64(r.assetCount)
		if actual < int64(^uint64(0)>>1) {
			actual++
		}
		return nil, &LimitError{Name: "assets", Actual: actual, Limit: int64(r.limits.MaxAssets)}
	}
	if encodedBytes > r.limits.MaxCumulativeEncodedBytes-r.cumulativeEncodedBytes {
		actual := saturatedAdd(r.cumulativeEncodedBytes, encodedBytes)
		return nil, &LimitError{Name: "cumulative encoded bytes", Actual: actual, Limit: r.limits.MaxCumulativeEncodedBytes}
	}
	if decodedBytes > r.limits.MaxCumulativeDecodedBytes-r.cumulativeDecodedBytes {
		actual := saturatedAdd(r.cumulativeDecodedBytes, decodedBytes)
		return nil, &LimitError{Name: "cumulative decoded bytes", Actual: actual, Limit: r.limits.MaxCumulativeDecodedBytes}
	}
	r.assetCount++
	r.cumulativeEncodedBytes += encodedBytes
	r.cumulativeDecodedBytes += decodedBytes
	rolledBack := false
	return func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if rolledBack {
			return
		}
		rolledBack = true
		r.assetCount--
		r.cumulativeEncodedBytes -= encodedBytes
		r.cumulativeDecodedBytes -= decodedBytes
	}, nil
}

func saturatedAdd(left, right int64) int64 {
	maximum := int64(^uint64(0) >> 1)
	if right > maximum-left {
		return maximum
	}
	return left + right
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := checkContext(r.ctx); err != nil {
		return 0, err
	}
	n, err := r.r.Read(p)
	if err == nil {
		if contextErr := checkContext(r.ctx); contextErr != nil {
			return n, contextErr
		}
	}
	return n, err
}

func readBounded(ctx context.Context, reader io.Reader, limit int64, name string) ([]byte, error) {
	var output bytes.Buffer
	buffer := make([]byte, 32*1024)
	reader = contextReader{ctx: ctx, r: reader}
	var total int64
	emptyReads := 0
	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			emptyReads = 0
			if int64(n) > limit-total {
				return nil, &LimitError{Name: name, Actual: saturatedAdd(total, int64(n)), Limit: limit}
			}
			_, _ = output.Write(buffer[:n])
			total += int64(n)
		}
		if err != nil {
			if err == io.EOF {
				return output.Bytes(), nil
			}
			return nil, err
		}
		if n == 0 {
			emptyReads++
			if emptyReads >= 100 {
				return nil, io.ErrNoProgress
			}
		}
	}
}

func selectContentType(ctx context.Context, values []string) (string, error) {
	totalBytes := 0
	selected := ""
	for _, value := range values {
		if err := checkContext(ctx); err != nil {
			return "", err
		}
		if len(value) > maxContentTypeHeaderBytes-totalBytes {
			return "", &LimitError{Name: "Content-Type header bytes", Actual: int64(totalBytes + len(value)), Limit: maxContentTypeHeaderBytes}
		}
		totalBytes += len(value)
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if selected != "" {
			return "", errors.New("multiple Content-Type header values")
		}
		selected = value
	}
	return selected, nil
}

func parseContentEncodings(ctx context.Context, values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	totalBytes := 0
	encodings := make([]string, 0, maxContentEncodingTokens)
	for _, value := range values {
		if len(value) > maxContentEncodingHeaderBytes-totalBytes {
			return nil, &LimitError{Name: "Content-Encoding header bytes", Actual: int64(totalBytes + len(value)), Limit: maxContentEncodingHeaderBytes}
		}
		totalBytes += len(value)
		start := 0
		for {
			if err := checkContext(ctx); err != nil {
				return nil, err
			}
			end := start
			for end < len(value) && value[end] != ',' {
				if end%(32*1024) == 0 {
					if err := checkContext(ctx); err != nil {
						return nil, err
					}
				}
				end++
			}
			token := strings.TrimSpace(value[start:end])
			if token == "" {
				return nil, errors.New("Content-Encoding contains an empty token")
			}
			canonical, err := canonicalContentEncoding(token)
			if err != nil {
				return nil, err
			}
			if len(encodings) == maxContentEncodingTokens {
				return nil, fmt.Errorf("content encoding layer count %d exceeds limit %d", len(encodings)+1, maxContentEncodingTokens)
			}
			encodings = append(encodings, canonical)
			if end == len(value) {
				break
			}
			start = end + 1
		}
	}
	return encodings, nil
}

func canonicalContentEncoding(value string) (string, error) {
	for _, supported := range []string{"identity", "gzip", "x-gzip", "deflate", "br"} {
		if strings.EqualFold(value, supported) {
			return supported, nil
		}
	}
	return "", fmt.Errorf("unsupported content encoding %q", value)
}

func decodeContentEncodings(ctx context.Context, data []byte, encodings []string, limit int64) ([]byte, error) {
	for i := len(encodings) - 1; i >= 0; i-- {
		encoding := encodings[i]
		if encoding == "identity" {
			continue
		}
		var reader io.ReadCloser
		encodedReader := contextReader{ctx: ctx, r: bytes.NewReader(data)}
		switch encoding {
		case "gzip", "x-gzip":
			gzipReader, err := gzip.NewReader(encodedReader)
			if err != nil {
				return nil, fmt.Errorf("decode %s content encoding: %w", encoding, err)
			}
			reader = gzipReader
		case "deflate":
			zlibReader, err := zlib.NewReader(encodedReader)
			if err == nil {
				reader = zlibReader
			} else {
				if contextErr := checkContext(ctx); contextErr != nil {
					return nil, fmt.Errorf("decode %s content encoding: %w", encoding, contextErr)
				}
				reader = flate.NewReader(contextReader{ctx: ctx, r: bytes.NewReader(data)})
			}
		case "br":
			reader = io.NopCloser(brotli.NewReader(encodedReader))
		default:
			return nil, fmt.Errorf("unsupported canonical content encoding %q", encoding)
		}
		decoded, err := readBounded(ctx, reader, limit, "decompressed bytes")
		closeErr := reader.Close()
		if err != nil {
			return nil, fmt.Errorf("decode %s content encoding: %w", encoding, err)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("decode %s content encoding: %w", encoding, closeErr)
		}
		data = decoded
	}
	if int64(len(data)) > limit {
		return nil, &LimitError{Name: "decompressed bytes", Actual: int64(len(data)), Limit: limit}
	}
	return data, nil
}
