package textmeasure

import (
	"encoding/binary"
	"image"
	"sort"
	"testing"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"

	"github.com/d2lang/d2/d2renderers/d2fonts"
)

func TestMetricPreservingFace(t *testing.T) {
	data := d2fonts.FontFaces.Get(d2fonts.SourceSansPro.Font(0, d2fonts.FONT_STYLE_REGULAR))
	parsed, err := parseFont(data)
	if err != nil {
		t.Fatal(err)
	}
	face := parsed.newFace(13)

	wantMetrics := font.Metrics{
		Height:  fixed.I(13),
		Ascent:  fixed.I(12) + 51,
		Descent: fixed.I(3) + 36,
	}
	if got := face.Metrics(); got != wantMetrics {
		t.Fatalf("metrics changed: got %+v, want %+v", got, wantMetrics)
	}
	if got := face.Kern('A', 'V'); got != 0 {
		t.Fatalf("legacy kerning changed: got %v, want 0:00", got)
	}
	if _, _, ok := face.GlyphBounds('\x00'); !ok {
		t.Fatal("missing glyph should use the font's .notdef glyph")
	}
	if got := face.Metrics().CaretSlope; got != (image.Point{}) {
		t.Fatalf("caret slope changed: got %v", got)
	}
}

func TestMetricPreservingFaceLegacyKern(t *testing.T) {
	data := d2fonts.FontFaces.Get(d2fonts.SourceSansPro.Font(0, d2fonts.FONT_STYLE_REGULAR))
	base, err := parseFont(data)
	if err != nil {
		t.Fatal(err)
	}
	left, err := base.font.GlyphIndex(nil, 'A')
	if err != nil {
		t.Fatal(err)
	}
	right, err := base.font.GlyphIndex(nil, 'V')
	if err != nil {
		t.Fatal(err)
	}

	const kernUnits = -80
	data = addLegacyKernTable(t, data, uint32(left)<<16|uint32(right), kernUnits)
	parsed, err := parseFont(data)
	if err != nil {
		t.Fatal(err)
	}
	face := parsed.newFace(13)
	want := roundScale(fixed.I(13), fixed.I(kernUnits), fixed.I(int(parsed.font.UnitsPerEm())))
	if got := face.Kern('A', 'V'); got != want {
		t.Fatalf("legacy kern changed: got %v, want %v", got, want)
	}
}

func addLegacyKernTable(t *testing.T, src []byte, key uint32, value int) []byte {
	t.Helper()
	if len(src) < 12 || string(src[:4]) == "ttcf" {
		t.Fatal("test helper requires a single sfnt font")
	}

	n := int(binary.BigEndian.Uint16(src[4:6]))
	directoryEnd := 12 + 16*n
	if directoryEnd > len(src) {
		t.Fatal("invalid source table directory")
	}

	type tableRecord struct {
		tag      uint32
		checksum uint32
		offset   uint32
		length   uint32
	}
	records := make([]tableRecord, 0, n+1)
	for i := 0; i < n; i++ {
		record := src[12+16*i : 12+16*(i+1)]
		records = append(records, tableRecord{
			tag:      binary.BigEndian.Uint32(record[0:4]),
			checksum: binary.BigEndian.Uint32(record[4:8]),
			offset:   binary.BigEndian.Uint32(record[8:12]) + 16,
			length:   binary.BigEndian.Uint32(record[12:16]),
		})
	}

	kern := make([]byte, 24)
	binary.BigEndian.PutUint16(kern[2:4], 1)  // one subtable
	binary.BigEndian.PutUint16(kern[6:8], 20) // subtable length
	binary.BigEndian.PutUint16(kern[8:10], 1) // horizontal format 0
	binary.BigEndian.PutUint16(kern[10:12], 1)
	binary.BigEndian.PutUint32(kern[18:22], key)
	binary.BigEndian.PutUint16(kern[22:24], uint16(int16(value)))

	kernOffset := len(src) + 16
	padding := (4 - kernOffset%4) % 4
	kernOffset += padding
	records = append(records, tableRecord{
		tag:    binary.BigEndian.Uint32([]byte("kern")),
		offset: uint32(kernOffset),
		length: uint32(len(kern)),
	})
	sort.Slice(records, func(i, j int) bool { return records[i].tag < records[j].tag })

	out := make([]byte, kernOffset+len(kern))
	copy(out[:12], src[:12])
	binary.BigEndian.PutUint16(out[4:6], uint16(len(records)))
	power := 1
	selector := 0
	for power*2 <= len(records) {
		power *= 2
		selector++
	}
	binary.BigEndian.PutUint16(out[6:8], uint16(power*16))
	binary.BigEndian.PutUint16(out[8:10], uint16(selector))
	binary.BigEndian.PutUint16(out[10:12], uint16(len(records)*16-power*16))
	for i, record := range records {
		dst := out[12+16*i : 12+16*(i+1)]
		binary.BigEndian.PutUint32(dst[0:4], record.tag)
		binary.BigEndian.PutUint32(dst[4:8], record.checksum)
		binary.BigEndian.PutUint32(dst[8:12], record.offset)
		binary.BigEndian.PutUint32(dst[12:16], record.length)
	}
	copy(out[directoryEnd+16:], src[directoryEnd:])
	copy(out[kernOffset:], kern)
	return out
}
