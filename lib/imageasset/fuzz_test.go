package imageasset

import (
	"context"
	"testing"

	"github.com/d2lang/d2/internal/rasterimage"
)

func FuzzDataURI(f *testing.F) {
	f.Add("image/svg+xml", "%3Csvg%20xmlns='http://www.w3.org/2000/svg'/%3E")
	f.Add("image/png;base64", "iVBORw0KGgo=")
	f.Add("text/plain", "%")
	f.Fuzz(func(t *testing.T, metadata, payload string) {
		limits := fuzzLimits()
		resolver, err := New(Options{Limits: limits})
		if err != nil {
			t.Fatal(err)
		}
		_, err = resolver.Resolve(context.Background(), "data:"+metadata+","+payload)
		if err == nil {
			return
		}
		if sourceErr, ok := err.(*SourceError); ok {
			if len(sourceErr.Source) > 133 {
				t.Fatalf("unbounded data URI source label: %q", sourceErr.Source)
			}
			for _, character := range []byte(sourceErr.Source) {
				if character < 0x20 || character > 0x7e {
					t.Fatalf("non-printable byte in data URI source label: %q", sourceErr.Source)
				}
			}
		}
	})
}

func FuzzSVGValidation(f *testing.F) {
	f.Add([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><path d="M0 0"/></svg>`))
	f.Add([]byte(`<?xml version="1.0"?><!--editor--><!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd"><svg/>`))
	f.Add([]byte(`<!DOCTYPE svg><svg/>`))
	f.Add([]byte{0xef, 0xbb, 0xbf, '<', 's', 'v', 'g', '/', '>'})
	f.Fuzz(func(t *testing.T, data []byte) {
		_ = validateSVG(context.Background(), data, fuzzLimits().MaxSVGBytes)
	})
}

func FuzzContentEncodingStacks(f *testing.F) {
	f.Add("identity", []byte("plain"))
	f.Add("gzip, br", []byte{0x1f, 0x8b})
	f.Add("gzip,,br", []byte("invalid"))
	f.Fuzz(func(t *testing.T, header string, data []byte) {
		encodings, err := parseContentEncodings(context.Background(), []string{header})
		if err != nil {
			return
		}
		_, _ = decodeContentEncodings(context.Background(), data, encodings, 4<<10)
	})
}

func FuzzGIFScanner(f *testing.F) {
	f.Add([]byte("GIF89a\x01\x00\x01\x00\x00\x00\x00\x3b"))
	f.Add([]byte("GIF89a"))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _, _ = rasterimage.Config(context.Background(), data, "gif")
	})
}

func FuzzPNGScanner(f *testing.F) {
	f.Add([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\x00IEND\xaeB`\x82"))
	f.Add([]byte("\x89PNG\r\n\x1a\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _, _ = rasterimage.Config(context.Background(), data, "png")
	})
}

func FuzzWebPScanner(f *testing.F) {
	f.Add([]byte("RIFF\x04\x00\x00\x00WEBP"))
	f.Add([]byte("RIFF"))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _, _ = rasterimage.Config(context.Background(), data, "webp")
	})
}

func fuzzLimits() Limits {
	return Limits{
		MaxFetchedBytes:           4 << 10,
		MaxEncodedBytes:           4 << 10,
		MaxDecompressedBytes:      4 << 10,
		MaxSVGBytes:               4 << 10,
		MaxDecodedWidth:           128,
		MaxDecodedHeight:          128,
		MaxDecodedPixels:          128 * 128,
		MaxAssets:                 8,
		MaxCumulativeEncodedBytes: 16 << 10,
		MaxCumulativeDecodedBytes: 64 << 10,
	}
}
