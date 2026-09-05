package d2fonts

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"strings"
	"sync"
	"testing"

	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	"github.com/d2lang/d2/d2renderers/internal/fontface"
)

func TestBundledFontFacesAreRegisteredCloneSources(t *testing.T) {
	for _, spec := range bundledFaceSpecs {
		spec := spec
		t.Run(string(spec.font.Family)+"/"+string(spec.font.Style), func(t *testing.T) {
			data, ok := FontFaces.Lookup(spec.font)
			if !ok || len(data) == 0 {
				t.Fatal("bundled face is unavailable")
			}
			if _, privateBacking := fontface.RegisteredBundledFaceBackingDigest(data); privateBacking {
				t.Fatal("public FontFaces entry aliases the private parser-registry backing")
			}
			source, matched, err := fontface.RegisteredBundledFace(data, 0)
			if err != nil || !matched || source == nil {
				t.Fatalf("registered face = %p, %v, %v", source, matched, err)
			}
			first, err := source.CloneReadOnly()
			if err != nil {
				t.Fatal(err)
			}
			second, err := source.CloneReadOnly()
			if err != nil {
				t.Fatal(err)
			}
			if first == second || first.Shaping == second.Shaping || first.Shaping.Font != second.Shaping.Font || first.Outline != second.Outline {
				t.Fatal("registered clones do not have independent mutable faces over shared immutable tables")
			}
			outline, err := source.Outline()
			if err != nil || outline != first.Outline {
				t.Fatalf("source outline = %p, %v; want %p", outline, err, first.Outline)
			}
			var buffer sfnt.Buffer
			glyph, err := outline.GlyphIndex(&buffer, 'A')
			if err != nil || glyph == 0 {
				t.Fatalf("source glyph A = %d, %v", glyph, err)
			}
			bounds, hasInk, err := source.GlyphRenderBounds(uint32(glyph), fixed.I(16))
			if err != nil || !hasInk || bounds.Empty() {
				t.Fatalf("source glyph bounds = %v/%v, %v", bounds, hasInk, err)
			}
			if kind, err := source.GlyphDataKind(uint32(glyph)); err != nil || kind != "outline" {
				t.Fatalf("source glyph kind = %q, %v", kind, err)
			}

			// A renderer-local clone owns the mutable Face wrapper. Reassigning
			// its exported fields must not change the private source wrapper.
			first.Outline = nil
			first.Shaping = nil
			if got, err := source.Outline(); err != nil || got != outline {
				t.Fatalf("source was poisoned through clone fields: %p, %v", got, err)
			}
			if boundsAfter, hasInk, err := source.GlyphRenderBounds(uint32(glyph), fixed.I(16)); err != nil || !hasInk || boundsAfter != bounds {
				t.Fatalf("source bounds after clone mutation = %v/%v, %v; want %v", boundsAfter, hasInk, err, bounds)
			}

			copied := append([]byte(nil), data...)
			if copiedSource, copiedMatch, copiedErr := fontface.RegisteredBundledFace(copied, 0); copiedErr != nil || !copiedMatch || copiedSource != source {
				t.Fatalf("copied registered face = %p, %v, %v; want %p, true, nil", copiedSource, copiedMatch, copiedErr, source)
			}
			if _, matched, err := fontface.RegisteredBundledFace(data, 1); !matched || err == nil || !strings.Contains(err.Error(), "collection has 1 faces") {
				t.Fatalf("out-of-range face = matched %v, error %v", matched, err)
			}
		})
	}
}

func TestGenericParseFaceIsIsolatedFromBundledSource(t *testing.T) {
	data, ok := FontFaces.Lookup(Font{Family: SourceSansPro, Style: FONT_STYLE_REGULAR})
	if !ok {
		t.Fatal("bundled face is unavailable")
	}
	source, matched, err := fontface.RegisteredBundledFace(data, 0)
	if err != nil || !matched {
		t.Fatalf("registered face = %p, %v, %v", source, matched, err)
	}
	shared, err := source.CloneReadOnly()
	if err != nil {
		t.Fatal(err)
	}
	isolated, err := fontface.ParseFace(data, 0)
	if err != nil {
		t.Fatal(err)
	}
	if isolated.Outline == shared.Outline || isolated.Shaping.Font == shared.Shaping.Font {
		t.Fatal("generic parsed face shared parser state with the bundled renderer source")
	}
	isolated.Shaping.Font.Cmap = nil
	third, err := source.CloneReadOnly()
	if err != nil {
		t.Fatal(err)
	}
	supported, err := third.SupportsRenderableRune('A')
	if err != nil || !supported {
		t.Fatalf("registered source was poisoned through generic ParseFace: support=%v, error=%v", supported, err)
	}
}

func TestBundledFontSourceCoverageMatchesParsedFace(t *testing.T) {
	data, ok := FontFaces.Lookup(Font{Family: SourceSansPro, Style: FONT_STYLE_REGULAR})
	if !ok {
		t.Fatal("bundled face is unavailable")
	}
	source, matched, err := fontface.RegisteredBundledFace(data, 0)
	if err != nil || !matched {
		t.Fatalf("registered face = %p, %v, %v", source, matched, err)
	}
	parsed, err := source.CloneReadOnly()
	if err != nil {
		t.Fatal(err)
	}
	for value := rune(0); value <= '\uffff'; value++ {
		got, err := source.SupportsRenderableRune(value)
		if err != nil {
			t.Fatalf("source coverage U+%04X: %v", value, err)
		}
		want, err := parsed.SupportsRenderableRune(value)
		if err != nil {
			t.Fatalf("parsed coverage U+%04X: %v", value, err)
		}
		if got != want {
			t.Fatalf("source coverage U+%04X = %v, want %v", value, got, want)
		}
	}
}

func TestBundledFontSourceIsIsolatedFromPublicRegistryMutation(t *testing.T) {
	data, ok := FontFaces.Lookup(Font{Family: SourceSansPro, Style: FONT_STYLE_REGULAR})
	if !ok || len(data) == 0 {
		t.Fatal("bundled face is unavailable")
	}
	source, matched, err := fontface.RegisteredBundledFace(data, 0)
	if err != nil || !matched || source == nil {
		t.Fatalf("registered face = %p, %v, %v", source, matched, err)
	}

	offset := len(data) / 2
	original := data[offset]
	data[offset] ^= 1
	t.Cleanup(func() { data[offset] = original })
	if candidate, matched, err := fontface.RegisteredBundledFace(data, 0); candidate != nil || matched || err != nil {
		t.Fatalf("mutated public face = %p, %v, %v; want nil, false, nil", candidate, matched, err)
	}
	clone, err := source.CloneReadOnly()
	if err != nil {
		t.Fatal(err)
	}
	supported, err := clone.SupportsRenderableRune('A')
	if err != nil || !supported {
		t.Fatalf("private registered source after public mutation: support=%v, error=%v", supported, err)
	}

	data[offset] = original
	if candidate, matched, err := fontface.RegisteredBundledFace(data, 0); candidate != source || !matched || err != nil {
		t.Fatalf("restored public face = %p, %v, %v; want %p, true, nil", candidate, matched, err, source)
	}
}

func TestRegisteredBundledFaceAuthenticatesByteIdenticalCopies(t *testing.T) {
	data, ok := FontFaces.Lookup(Font{Family: SourceSansPro, Style: FONT_STYLE_REGULAR})
	if !ok || len(data) == 0 {
		t.Fatal("bundled face is unavailable")
	}
	source, matched, err := fontface.RegisteredBundledFace(data, 0)
	if err != nil || !matched || source == nil {
		t.Fatalf("registered face = %p, %v, %v", source, matched, err)
	}
	copied := bytes.Clone(data)
	if candidate, matched, err := fontface.RegisteredBundledFace(copied, 0); candidate != source || !matched || err != nil {
		t.Fatalf("byte-identical copy = %p, %v, %v; want %p, true, nil", candidate, matched, err, source)
	}
	copied[len(copied)/2] ^= 1
	if candidate, matched, err := fontface.RegisteredBundledFace(copied, 0); candidate != nil || matched || err != nil {
		t.Fatalf("modified copy = %p, %v, %v; want nil, false, nil", candidate, matched, err)
	}
}

func TestBundledFontFaceClonesAreConcurrent(t *testing.T) {
	data, ok := FontFaces.Lookup(Font{Family: SourceSansPro, Style: FONT_STYLE_REGULAR})
	if !ok {
		t.Fatal("bundled face is unavailable")
	}
	source, matched, err := fontface.RegisteredBundledFace(data, 0)
	if err != nil || !matched {
		t.Fatalf("registered face = %p, %v, %v", source, matched, err)
	}
	const workers = 16
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			outline, err := source.Outline()
			if err == nil {
				var buffer sfnt.Buffer
				glyph, glyphErr := outline.GlyphIndex(&buffer, 'A')
				if glyphErr != nil {
					err = glyphErr
				} else {
					_, _, err = source.GlyphRenderBounds(uint32(glyph), fixed.I(16))
				}
			}
			if err == nil {
				supported, supportErr := source.SupportsRenderableRune('A')
				if supportErr != nil {
					err = supportErr
				} else if !supported {
					err = errors.New("bundled source does not support A")
				}
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	mutated := append([]byte(nil), data...)
	mutated[len(mutated)/2] ^= 1
	if bytes.Equal(mutated, data) {
		t.Fatal("test mutation did not change data")
	}
	if source, matched, err := fontface.RegisteredBundledFace(mutated, 0); source != nil || matched || err != nil {
		t.Fatalf("mutated face = %p, %v, %v; want nil, false, nil", source, matched, err)
	}
}

func BenchmarkRegisteredBundledFontFaceLookup(b *testing.B) {
	data, ok := FontFaces.Lookup(Font{Family: SourceSansPro, Style: FONT_STYLE_REGULAR})
	if !ok {
		b.Fatal("bundled face is unavailable")
	}
	if _, matched, err := fontface.RegisteredBundledFace(data, 0); err != nil || !matched {
		b.Fatalf("prime registered face = %v, %v", matched, err)
	}
	digest := sha256.Sum256(data)
	for _, benchmark := range []struct {
		name        string
		data        []byte
		knownDigest bool
	}{
		{name: "canonical", data: data},
		{name: "copied", data: append([]byte(nil), data...)},
		{name: "canonical-known-digest", data: data, knownDigest: true},
		{name: "copied-known-digest", data: append([]byte(nil), data...), knownDigest: true},
	} {
		b.Run(benchmark.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				var source *fontface.BundledFaceSource
				var matched bool
				var err error
				if benchmark.knownDigest {
					source, matched, err = fontface.RegisteredBundledFaceDigest(benchmark.data, 0, digest)
				} else {
					source, matched, err = fontface.RegisteredBundledFace(benchmark.data, 0)
				}
				if err != nil || !matched || source == nil {
					b.Fatalf("registered face = %p, %v, %v", source, matched, err)
				}
			}
		})
	}
}
