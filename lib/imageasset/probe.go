package imageasset

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"mime"
	"net/http"
	"strings"

	"github.com/andybalholm/brotli"
	_ "golang.org/x/image/webp"
	"golang.org/x/text/encoding/charmap"

	"github.com/d2lang/d2/internal/rasterimage"
	libsvg "github.com/d2lang/d2/lib/svg"
)

var errUnsupportedSVG = errors.New("unsupported SVG")

// XML nesting receives an independent safety ceiling in addition to the
// caller's byte-derived element, attribute, and token budgets. encoding/xml
// itself is iterative, but downstream SVG consumers commonly retain ancestry.
const maxSVGXMLDepth = 256

type reserveResource func(encodedBytes, decodedBytes int64) (rollback func(), err error)

func probeResource(ctx context.Context, loaded loadedSource, limits Limits, reserve reserveResource) (*Resource, error) {
	if len(loaded.data) == 0 {
		return nil, errors.New("empty image resource")
	}
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	hint := normalizeMIME(loaded.mimeHint)
	signatureFormat := rasterSignature(loaded.data)
	if signatureFormat != "" {
		if err := checkByteLimits(loaded.data, limits); err != nil {
			return nil, err
		}
		return probeRaster(ctx, loaded, signatureFormat, limits, reserve)
	}
	looksXML, err := looksLikeXML(ctx, loaded.data)
	if err != nil {
		return nil, err
	}
	if hint == "image/svg+xml" || looksXML {
		var rawErr error
		if looksXML {
			svgLimit, svgLimitName := svgByteLimit(limits)
			if int64(len(loaded.data)) > svgLimit {
				rawErr = &LimitError{Name: svgLimitName, Actual: int64(len(loaded.data)), Limit: svgLimit}
			} else {
				rawErr = validateSVG(ctx, loaded.data, svgLimit)
			}
		} else {
			rawErr = errors.New("not raw SVG XML")
		}
		if rawErr == nil {
			if err := checkByteLimits(loaded.data, limits); err != nil {
				return nil, err
			}
			decodedBytes := int64(len(loaded.data))
			if err := validateDecodedLimits(0, 0, decodedBytes, KindSVG, limits); err != nil {
				return nil, err
			}
			return svgResource(ctx, loaded, decodedBytes, reserve)
		}
		if errors.Is(rawErr, context.Canceled) || errors.Is(rawErr, context.DeadlineExceeded) {
			return nil, rawErr
		}
		if errors.Is(rawErr, errUnsupportedSVG) {
			return nil, rawErr
		}
		var rawLimit *LimitError
		if errors.As(rawErr, &rawLimit) && hint != "image/svg+xml" {
			return nil, rawErr
		}
		if hint == "image/svg+xml" {
			expanded, compressionErr := expandDeclaredSVG(ctx, loaded, limits)
			if compressionErr != nil {
				if errors.Is(compressionErr, errUnsupportedSVG) {
					return nil, compressionErr
				}
				if rawLimit != nil {
					return nil, rawErr
				}
				if looksXML {
					return nil, fmt.Errorf("malformed SVG: %w", rawErr)
				}
				return nil, fmt.Errorf("malformed SVG (raw validation failed: %v; compressed fallback failed): %w", rawErr, compressionErr)
			}
			decodedBytes := int64(len(expanded.data))
			if err := validateDecodedLimits(0, 0, decodedBytes, KindSVG, limits); err != nil {
				return nil, err
			}
			return svgResource(ctx, expanded, decodedBytes, reserve)
		}
	}
	if err := checkByteLimits(loaded.data, limits); err != nil {
		return nil, err
	}
	if hint != "" {
		return nil, fmt.Errorf("unsupported or malformed image with MIME type %q", hint)
	}
	return nil, fmt.Errorf("unsupported or malformed image (sniffed %q)", normalizeMIME(http.DetectContentType(loaded.data)))
}

func svgResource(ctx context.Context, loaded loadedSource, decodedBytes int64, reserve reserveResource) (_ *Resource, err error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	rollback, err := reserve(int64(len(loaded.data)), decodedBytes)
	if err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if !committed {
			rollback()
		}
	}()
	resource := newResource(KindSVG, "image/svg+xml", loaded, 0, 0, decodedBytes)
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	committed = true
	return resource, nil
}

type compressedSVGCodec struct {
	name string
	open func(context.Context, []byte) (io.ReadCloser, error)
}

func expandDeclaredSVG(ctx context.Context, loaded loadedSource, limits Limits) (loadedSource, error) {
	codecs := []compressedSVGCodec{
		{name: "gzip", open: func(ctx context.Context, data []byte) (io.ReadCloser, error) {
			return gzip.NewReader(contextReader{ctx: ctx, r: bytes.NewReader(data)})
		}},
		{name: "zlib", open: func(ctx context.Context, data []byte) (io.ReadCloser, error) {
			return zlib.NewReader(contextReader{ctx: ctx, r: bytes.NewReader(data)})
		}},
		{name: "raw deflate", open: func(ctx context.Context, data []byte) (io.ReadCloser, error) {
			return flate.NewReader(contextReader{ctx: ctx, r: bytes.NewReader(data)}), nil
		}},
		{name: "brotli", open: func(ctx context.Context, data []byte) (io.ReadCloser, error) {
			return io.NopCloser(brotli.NewReader(contextReader{ctx: ctx, r: bytes.NewReader(data)})), nil
		}},
	}
	var firstLimit error
	var lastErr error
	lastCodec := "none"
	for _, codec := range codecs {
		if err := checkContext(ctx); err != nil {
			return loadedSource{}, err
		}
		lastCodec = codec.name
		reader, err := codec.open(ctx, loaded.data)
		if err != nil {
			if contextErr := checkContext(ctx); contextErr != nil {
				return loadedSource{}, contextErr
			}
			lastErr = err
			continue
		}
		decodedLimit, limitName := svgByteLimit(limits)
		decoded, readErr := readBounded(ctx, reader, decodedLimit, limitName)
		closeErr := reader.Close()
		if readErr != nil {
			if errors.Is(readErr, context.Canceled) || errors.Is(readErr, context.DeadlineExceeded) {
				return loadedSource{}, readErr
			}
			var limitErr *LimitError
			if firstLimit == nil && errors.As(readErr, &limitErr) {
				firstLimit = readErr
			}
			lastErr = readErr
			continue
		}
		if closeErr != nil {
			lastErr = closeErr
			continue
		}
		if int64(len(decoded)) > limits.MaxEncodedBytes {
			limitErr := &LimitError{Name: "encoded bytes", Actual: int64(len(decoded)), Limit: limits.MaxEncodedBytes}
			if firstLimit == nil {
				firstLimit = limitErr
			}
			lastErr = limitErr
			continue
		}
		svgLimit, _ := svgByteLimit(limits)
		if err := validateSVG(ctx, decoded, svgLimit); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return loadedSource{}, err
			}
			if errors.Is(err, errUnsupportedSVG) {
				return loadedSource{}, err
			}
			var validationLimit *LimitError
			if errors.As(err, &validationLimit) {
				return loadedSource{}, err
			}
			lastErr = err
			continue
		}
		loaded.data = decoded
		loaded.decompressedBytes = int64(len(decoded))
		return loaded, nil
	}
	if firstLimit != nil {
		return loadedSource{}, firstLimit
	}
	if lastErr == nil {
		lastErr = errors.New("no compressed data")
	}
	return loadedSource{}, fmt.Errorf("no supported codec produced SVG XML (last codec %s): %w", lastCodec, lastErr)
}

func probeRaster(ctx context.Context, loaded loadedSource, expectedFormat string, limits Limits, reserve reserveResource) (*Resource, error) {
	config, _, err := rasterimage.Config(ctx, loaded.data, expectedFormat)
	if err != nil {
		return nil, fmt.Errorf("malformed %s image: %w", strings.ToUpper(expectedFormat), err)
	}
	return rasterResource(ctx, loaded, config, expectedFormat, limits, reserve)
}

func rasterResource(ctx context.Context, loaded loadedSource, config image.Config, format string, limits Limits, reserve reserveResource) (_ *Resource, err error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	if config.Width <= 0 || config.Height <= 0 {
		return nil, fmt.Errorf("invalid %s dimensions %dx%d", format, config.Width, config.Height)
	}
	if int64(config.Width) > math.MaxInt64/int64(config.Height) {
		return nil, errors.New("decoded pixel count overflows int64")
	}
	pixels := int64(config.Width) * int64(config.Height)
	bytesPerPixel := decodedBytesPerPixel(config)
	if pixels > math.MaxInt64/bytesPerPixel {
		return nil, errors.New("decoded byte count overflows int64")
	}
	decodedBytes := pixels * bytesPerPixel
	if err := validateDecodedLimits(config.Width, config.Height, decodedBytes, KindRaster, limits); err != nil {
		return nil, err
	}
	rollback, err := reserve(int64(len(loaded.data)), decodedBytes)
	if err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if !committed {
			rollback()
		}
	}()
	// Config has already performed bounded format-specific structure checks and
	// decoded the logical dimensions. Pixel entropy is decoded once, by the
	// renderer that consumes the retained resource.
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	mimeType, ok := mimeForFormat(format)
	if !ok {
		return nil, fmt.Errorf("unsupported raster format %q", format)
	}
	resource := newResource(KindRaster, mimeType, loaded, config.Width, config.Height, decodedBytes)
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	committed = true
	return resource, nil
}

func mimeForFormat(format string) (string, bool) {
	switch format {
	case "png":
		return "image/png", true
	case "jpeg":
		return "image/jpeg", true
	case "gif":
		return "image/gif", true
	case "webp":
		return "image/webp", true
	default:
		return "", false
	}
}

func decodedBytesPerPixel(config image.Config) int64 {
	if config.ColorModel == nil {
		return 8
	}
	// Go's PNG decoder can allocate 16-bit-per-channel images. Conservatively
	// charge eight bytes per pixel for any 16-bit model; static 8-bit formats
	// are charged at four bytes even when their concrete representation is
	// smaller (for example GIF palettes or JPEG YCbCr planes).
	sample := config.ColorModel.Convert(color.RGBA64{R: 0x1234, G: 0x5678, B: 0x9abc, A: 0xffff})
	switch sample.(type) {
	case color.RGBA64, color.NRGBA64, color.Gray16, color.Alpha16:
		return 8
	default:
		return 4
	}
}

func newResource(kind Kind, mimeType string, loaded loadedSource, width, height int, decodedBytes int64) *Resource {
	return &Resource{
		kind:              kind,
		mimeType:          mimeType,
		data:              append([]byte(nil), loaded.data...),
		pixelWidth:        width,
		pixelHeight:       height,
		fetchedBytes:      loaded.fetchedBytes,
		decompressedBytes: loaded.decompressedBytes,
		decodedBytes:      decodedBytes,
	}
}

func validateDecodedLimits(width, height int, decodedBytes int64, kind Kind, limits Limits) error {
	if kind == KindRaster {
		if width > limits.MaxDecodedWidth {
			return &LimitError{Name: "decoded width", Actual: int64(width), Limit: int64(limits.MaxDecodedWidth)}
		}
		if height > limits.MaxDecodedHeight {
			return &LimitError{Name: "decoded height", Actual: int64(height), Limit: int64(limits.MaxDecodedHeight)}
		}
		if width <= 0 || height <= 0 || int64(width) > math.MaxInt64/int64(height) {
			return errors.New("decoded pixel count overflows int64")
		}
		pixels := int64(width) * int64(height)
		if pixels > limits.MaxDecodedPixels {
			return &LimitError{Name: "decoded pixels", Actual: pixels, Limit: limits.MaxDecodedPixels}
		}
	}
	if decodedBytes > limits.MaxCumulativeDecodedBytes {
		return &LimitError{Name: "decoded bytes", Actual: decodedBytes, Limit: limits.MaxCumulativeDecodedBytes}
	}
	return nil
}

func normalizeMIME(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	mediaType, _, err := mime.ParseMediaType(value)
	if err == nil {
		value = mediaType
	} else if index := strings.IndexByte(value, ';'); index >= 0 {
		value = value[:index]
	}
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "image/jpg", "image/pjpeg":
		return "image/jpeg"
	case "image/x-png":
		return "image/png"
	case "text/xml", "application/xml", "application/svg+xml":
		return "image/svg+xml"
	default:
		return value
	}
}

func rasterSignature(data []byte) string {
	switch {
	case len(data) >= 8 && bytes.Equal(data[:8], []byte("\x89PNG\r\n\x1a\n")):
		return "png"
	case len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff:
		return "jpeg"
	case len(data) >= 6 && (bytes.Equal(data[:6], []byte("GIF87a")) || bytes.Equal(data[:6], []byte("GIF89a"))):
		return "gif"
	case len(data) >= 12 && bytes.Equal(data[:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return "webp"
	default:
		return ""
	}
}

func looksLikeXML(ctx context.Context, data []byte) (bool, error) {
	offset := 0
	if bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}) {
		offset = 3
	}
	for offset < len(data) {
		if offset%(32*1024) == 0 {
			if err := checkContext(ctx); err != nil {
				return false, err
			}
		}
		switch data[offset] {
		case ' ', '\t', '\r', '\n':
			offset++
			continue
		default:
			return data[offset] == '<', nil
		}
	}
	return false, checkContext(ctx)
}

// validateSVG only parses the supplied bytes. Exact SVG 1.0 and 1.1 external
// doctypes are recognized and ignored. Their external subsets are not fetched,
// so DTD entities and default attributes are not applied.
type svgValidationLimits struct {
	bytes          int64
	depth          int
	elements       int64
	attributes     int64
	attributeBytes int64
	tokens         int64
}

func svgByteLimit(limits Limits) (int64, string) {
	result := limits.MaxSVGBytes
	name := "SVG bytes"
	if limits.MaxEncodedBytes < result {
		result = limits.MaxEncodedBytes
		name = "encoded bytes"
	}
	if limits.MaxDecompressedBytes < result {
		result = limits.MaxDecompressedBytes
		name = "decompressed bytes"
	}
	return result, name
}

func validationLimitsForSVG(maxBytes int64) svgValidationLimits {
	// The shortest element, attribute, and distinct XML token encodings consume
	// multiple source bytes. These deliberately generous derived ceilings make
	// every parser work dimension finite without imposing importer semantics.
	return svgValidationLimits{
		bytes: maxBytes, depth: maxSVGXMLDepth,
		elements: maxBytes/3 + 1, attributes: maxBytes/3 + 1,
		attributeBytes: maxBytes, tokens: maxBytes/2 + 2,
	}
}

func validateSVG(ctx context.Context, data []byte, maxBytes int64) error {
	return validateSVGWithLimits(ctx, data, validationLimitsForSVG(maxBytes))
}

func validateSVGWithLimits(ctx context.Context, data []byte, limits svgValidationLimits) error {
	if int64(len(data)) > limits.bytes {
		return &LimitError{Name: "SVG bytes", Actual: int64(len(data)), Limit: limits.bytes}
	}
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	decoder := xml.NewDecoder(contextReader{ctx: ctx, r: bytes.NewReader(data)})
	decoder.Strict = true
	decoder.CharsetReader = iso88591XMLReader
	depth := 0
	seenRoot := false
	closedRoot := false
	seenXMLDeclaration := false
	seenDoctype := false
	declarationAllowed := true
	var elements int64
	var attributes int64
	var attributeBytes int64
	var tokens int64
	for {
		if err := checkContext(ctx); err != nil {
			return err
		}
		token, err := decoder.Token()
		if err == io.EOF {
			if !seenRoot || !closedRoot || depth != 0 {
				return errors.New("missing or unclosed svg root element")
			}
			return nil
		}
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			return errors.New("malformed SVG XML")
		}
		tokens++
		if tokens > limits.tokens {
			return &LimitError{Name: "SVG XML tokens", Actual: tokens, Limit: limits.tokens}
		}
		switch token := token.(type) {
		case xml.StartElement:
			declarationAllowed = false
			elements++
			if elements > limits.elements {
				return &LimitError{Name: "SVG elements", Actual: elements, Limit: limits.elements}
			}
			if int64(len(token.Attr)) > limits.attributes-attributes {
				return &LimitError{Name: "SVG attributes", Actual: attributes + int64(len(token.Attr)), Limit: limits.attributes}
			}
			attributes += int64(len(token.Attr))
			for _, attribute := range token.Attr {
				amount := int64(len(attribute.Name.Space) + len(attribute.Name.Local) + len(attribute.Value))
				if amount > limits.attributeBytes-attributeBytes {
					return &LimitError{Name: "SVG attribute bytes", Actual: attributeBytes + amount, Limit: limits.attributeBytes}
				}
				attributeBytes += amount
			}
			if depth == 0 {
				if seenRoot {
					return errors.New("multiple root elements")
				}
				if token.Name.Local != "svg" {
					return errors.New("root element is not svg")
				}
				if token.Name.Space != "" && token.Name.Space != "http://www.w3.org/2000/svg" {
					return errors.New("svg root has an unexpected namespace")
				}
				seenRoot = true
			}
			if closedRoot {
				return errors.New("element after svg root")
			}
			depth++
			if depth > limits.depth {
				return &LimitError{Name: "SVG XML depth", Actual: int64(depth), Limit: int64(limits.depth)}
			}
		case xml.EndElement:
			declarationAllowed = false
			depth--
			if depth < 0 {
				return errors.New("unexpected closing element")
			}
			if depth == 0 {
				closedRoot = true
			}
		case xml.Directive:
			if seenRoot || seenDoctype || !libsvg.IsSupportedExternalDoctype(data, token) {
				return fmt.Errorf("%w: XML directive or DTD", errUnsupportedSVG)
			}
			seenDoctype = true
			declarationAllowed = false
		case xml.ProcInst:
			if token.Target != "xml" || seenXMLDeclaration || !declarationAllowed || seenRoot {
				return errors.New("processing instruction is not allowed")
			}
			seenXMLDeclaration = true
			declarationAllowed = false
		case xml.CharData:
			if depth == 0 && len(bytes.TrimSpace(token)) != 0 {
				return errors.New("text outside svg root element")
			}
			if len(token) != 0 {
				declarationAllowed = false
			}
		case xml.Comment:
			declarationAllowed = false
		}
	}
}

func iso88591XMLReader(charset string, input io.Reader) (io.Reader, error) {
	if !strings.EqualFold(charset, "iso-8859-1") {
		return nil, errors.New("unsupported XML character encoding")
	}
	return charmap.ISO8859_1.NewDecoder().Reader(input), nil
}
