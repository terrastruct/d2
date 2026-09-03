package fontface

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"strings"
	"testing"

	gotextfont "github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/font/opentype/tables"
	"golang.org/x/image/font/sfnt"
)

func TestParsedFaceAcceptsEmptyOutlineSpacingGlyph(t *testing.T) {
	data := testFontData(t, "SourceSansPro-Regular.ttf")
	face, err := ParseFace(data, 0)
	if err != nil {
		t.Fatal(err)
	}
	supported, err := face.SupportsRenderableRune(' ')
	if err != nil || !supported {
		t.Fatalf("space coverage = %v, %v; empty outline spacing glyph must be valid", supported, err)
	}
}

func TestOrdinaryFaceParsingDoesNotHashSource(t *testing.T) {
	var zeroDigest [sha256.Size]byte
	data := testFontData(t, "SourceSansPro-Regular.ttf")
	face, err := ParseFace(data, 0)
	if err != nil {
		t.Fatal(err)
	}
	if face.source.shapingFont == nil || face.source.digest != zeroDigest {
		t.Fatalf("ordinary parser provenance = %#v, want unhashed parser-issued source", face.source)
	}
	if face.hasParsedSource(zeroDigest) {
		t.Fatal("unhashed ordinary face authenticated a zero digest")
	}
	clone, err := face.Clone()
	if err != nil {
		t.Fatal(err)
	}
	if clone.source.shapingFont == nil || clone.source.digest != zeroDigest {
		t.Fatalf("ordinary clone provenance = %#v, want unhashed parser-issued source", clone.source)
	}
	if clone.hasParsedSource(zeroDigest) {
		t.Fatal("unhashed ordinary clone authenticated a zero digest")
	}
	if err := face.CloneInto(nil); err == nil || !strings.Contains(err.Error(), "clone destination") {
		t.Fatalf("nil CloneInto destination error = %v", err)
	}
	previousShaping := face.Shaping
	if err := face.CloneInto(face); err != nil {
		t.Fatal(err)
	}
	if face.Shaping == previousShaping || !face.hasParsedSourceComponents() {
		t.Fatal("in-place CloneInto did not replace mutable caches and preserve provenance")
	}
}

func BenchmarkParseFaceOrdinary(b *testing.B) {
	data := testFontData(b, "SourceSansPro-Regular.ttf")
	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	for b.Loop() {
		face, err := ParseFace(data, 0)
		if err != nil || face == nil {
			b.Fatalf("ParseFace() = %p, %v", face, err)
		}
	}
}

func TestUnsupportedColorPaintRequiresOutlineFallback(t *testing.T) {
	data := testFontData(t, "SourceSansPro-Regular.ttf")
	face, err := ParseFace(data, 0)
	if err != nil {
		t.Fatal(err)
	}
	var buffer sfnt.Buffer
	paint := tables.PaintSolid{}

	visible, err := face.Outline.GlyphIndex(&buffer, 'A')
	if err != nil || visible == 0 {
		t.Fatalf("visible glyph = %d, %v", visible, err)
	}
	if layers, colorGlyph, err := face.resolveColorPaint(uint32(visible), paint); err != nil || colorGlyph || layers != nil {
		t.Fatalf("visible outline fallback = %#v/%v/%v", layers, colorGlyph, err)
	}

	empty, err := face.Outline.GlyphIndex(&buffer, ' ')
	if err != nil || empty == 0 {
		t.Fatalf("spacing glyph = %d, %v", empty, err)
	}
	if _, _, err := face.resolveColorPaint(uint32(empty), paint); err == nil || !strings.Contains(err.Error(), "unsupported COLRv1 paint without an outline fallback") {
		t.Fatalf("empty outline fallback error = %v", err)
	}
}

func TestParseFaceCollectionRejectsCountBeforeEagerFaceConstruction(t *testing.T) {
	data := testFontData(t, "SourceSansPro-Regular.ttf")
	collectionData := duplicateTTFCollection(t, data, 2)
	if _, err := ParseFaceCollectionWithLimit(collectionData, 1); err == nil {
		t.Fatal("two-face collection passed one-face limit")
	} else {
		var limitError *FaceCountLimitError
		if !errors.As(err, &limitError) || limitError.Count != 2 || limitError.Limit != 1 {
			t.Fatalf("face-count error = %#v / %v", limitError, err)
		}
	}
	collection, err := ParseFaceCollectionWithLimit(collectionData, 2)
	if err != nil {
		t.Fatal(err)
	}
	if collection.NumFaces() != 2 {
		t.Fatalf("face count = %d, want 2", collection.NumFaces())
	}
	face, err := collection.Face(1)
	if err != nil {
		t.Fatal(err)
	}
	supported, err := face.SupportsRenderableRune('A')
	if err != nil || !supported {
		t.Fatalf("second face coverage = %v, %v", supported, err)
	}
}

func TestParseFaceCollectionRecoversShapingParserPanic(t *testing.T) {
	data := testFontData(t, "SourceSansPro-Regular.ttf")
	collection, err := parseFaceCollectionWithLimitUsingParser(data, maxParsedFontFaces, func(gotextfont.Resource) ([]*gotextfont.Face, error) {
		panic("malformed shaping table")
	})
	if collection != nil {
		t.Fatalf("collection after parser panic = %#v, want nil", collection)
	}
	if !errors.Is(err, errShapingParserPanic) {
		t.Fatalf("parser panic error = %v, want %v", err, errShapingParserPanic)
	}
}

func TestParseFaceCollectionPreservesShapingParserError(t *testing.T) {
	data := testFontData(t, "SourceSansPro-Regular.ttf")
	wantErr := errors.New("ordinary shaping parse error")
	collection, err := parseFaceCollectionWithLimitUsingParser(data, maxParsedFontFaces, func(gotextfont.Resource) ([]*gotextfont.Face, error) {
		return nil, wantErr
	})
	if collection != nil {
		t.Fatalf("collection after parser error = %#v, want nil", collection)
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("parser error = %v, want %v", err, wantErr)
	}
}

func duplicateTTFCollection(t *testing.T, font []byte, count int) []byte {
	t.Helper()
	if len(font) < 12 || count <= 0 {
		t.Fatal("invalid source font or collection count")
	}
	fonts := make([][]byte, count)
	for index := range fonts {
		fonts[index] = font
	}
	return combineTTFCollection(t, fonts...)
}

func combineTTFCollection(t *testing.T, fonts ...[]byte) []byte {
	t.Helper()
	if len(fonts) == 0 {
		t.Fatal("font collection requires at least one source font")
	}
	for _, font := range fonts {
		if len(font) < 12 {
			t.Fatal("invalid source font")
		}
	}
	count := len(fonts)
	headerBytes := 12 + 4*count
	totalBytes := headerBytes
	for _, font := range fonts {
		totalBytes += (len(font) + 3) &^ 3
	}
	result := make([]byte, totalBytes)
	copy(result[:4], "ttcf")
	binary.BigEndian.PutUint32(result[4:8], 0x00010000)
	binary.BigEndian.PutUint32(result[8:12], uint32(count))
	base := headerBytes
	for index, font := range fonts {
		binary.BigEndian.PutUint32(result[12+4*index:16+4*index], uint32(base))
		copy(result[base:base+len(font)], font)
		tableCount := int(binary.BigEndian.Uint16(result[base+4 : base+6]))
		if 12+16*tableCount > len(font) {
			t.Fatal("invalid source sfnt table directory")
		}
		for table := 0; table < tableCount; table++ {
			offsetAt := base + 12 + table*16 + 8
			offset := binary.BigEndian.Uint32(result[offsetAt : offsetAt+4])
			binary.BigEndian.PutUint32(result[offsetAt:offsetAt+4], offset+uint32(base))
		}
		base += (len(font) + 3) &^ 3
	}
	return result
}
