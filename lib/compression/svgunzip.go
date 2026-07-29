package compression

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"encoding/base64"
	"io"
	"regexp"
	"strings"

	"github.com/andybalholm/brotli"
)

var reDataSVG = regexp.MustCompile(`(?i)(?:(?:xlink:)?href)\s*=\s*"(data:image/svg\+xml;base64,([^"]+))"`)

// imgbundler gets remote images which may be compressed by either gzip or brotli
// When we load the SVG into headless chromium for PNG screenshots, these compressed images are not detected as proper images that need to be waited on before we screenshot.
// So prior to loading the SVG into chromium page, we replace these images with their decompressed versions
func UnzipEmbeddedSVGImages(svg []byte) []byte {
	matches := reDataSVG.FindAllSubmatchIndex(svg, -1)
	out := make([]byte, 0, len(svg))
	last := 0

	for _, m := range matches {
		urlStart, urlEnd := m[2], m[3]
		b64Start, b64End := m[4], m[5]

		out = append(out, svg[last:urlStart]...)

		raw, err := base64.StdEncoding.DecodeString(string(svg[b64Start:b64End]))
		if err != nil || looksSVG(raw) {
			out = append(out, svg[urlStart:urlEnd]...)
			last = urlEnd
			continue
		}

		var dec []byte
		if d, e := gunzipBytes(raw); e == nil && looksSVG(d) {
			dec = d
		} else if d, e := inflateBytes(raw); e == nil && looksSVG(d) {
			dec = d
		} else if d, e := brotliBytes(raw); e == nil && looksSVG(d) {
			dec = d
		}

		if dec == nil {
			out = append(out, svg[urlStart:urlEnd]...)
			last = urlEnd
			continue
		}

		newURL := "data:image/svg+xml;charset=utf-8;base64," + base64.StdEncoding.EncodeToString(dec)
		out = append(out, []byte(newURL)...)
		last = urlEnd
	}

	out = append(out, svg[last:]...)
	return out
}

func looksSVG(b []byte) bool {
	s := string(bytes.TrimSpace(b))
	if strings.HasPrefix(s, "<svg") || strings.HasPrefix(s, "<?xml") {
		return true
	}
	n := 512
	if len(s) < n {
		n = len(s)
	}
	return strings.Contains(s[:n], "<svg")
}

func gunzipBytes(b []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

func inflateBytes(b []byte) ([]byte, error) {
	if zr, err := zlib.NewReader(bytes.NewReader(b)); err == nil {
		defer zr.Close()
		return io.ReadAll(zr)
	}
	fr := flate.NewReader(bytes.NewReader(b))
	defer fr.Close()
	return io.ReadAll(fr)
}

func brotliBytes(b []byte) ([]byte, error) {
	return io.ReadAll(brotli.NewReader(bytes.NewReader(b)))
}
