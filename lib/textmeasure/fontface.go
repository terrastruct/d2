package textmeasure

import (
	"encoding/binary"
	"fmt"
	"sort"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// parsedFont keeps the metrics that the legacy freetype/truetype package
// exposed. opentype uses the richer OpenType line and GPOS metrics by default,
// which would otherwise change D2's text measurements and diagram geometry.
type parsedFont struct {
	font         *sfnt.Font
	unitsPerEm   fixed.Int26_6
	ascentUnits  fixed.Int26_6
	descentUnits fixed.Int26_6
	kern         []kernPair
}

type kernPair struct {
	key   uint32
	value int16
}

func parseFont(src []byte) (*parsedFont, error) {
	collection, err := opentype.ParseCollection(src)
	if err != nil {
		return nil, err
	}
	f, err := collection.Font(0)
	if err != nil {
		return nil, err
	}

	unitsPerEm := fixed.I(int(f.UnitsPerEm()))
	metrics, err := f.Metrics(nil, unitsPerEm, font.HintingNone)
	if err != nil {
		return nil, err
	}
	kern, err := parseLegacyKern(src)
	if err != nil {
		return nil, err
	}

	return &parsedFont{
		font:         f,
		unitsPerEm:   unitsPerEm,
		ascentUnits:  metrics.Ascent,
		descentUnits: metrics.Descent,
		kern:         kern,
	}, nil
}

func (f *parsedFont) newFace(size float64) font.Face {
	base, err := opentype.NewFace(f.font, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingNone,
	})
	if err != nil {
		// parseFont has already validated the font. NewFace currently cannot
		// fail for a parsed font, so an error here indicates an invariant break.
		panic(fmt.Sprintf("create OpenType face: %v", err))
	}

	return &metricPreservingFace{
		Face:         base,
		font:         f.font,
		scale:        fixed.Int26_6(0.5 + size*64),
		unitsPerEm:   f.unitsPerEm,
		ascentUnits:  f.ascentUnits,
		descentUnits: f.descentUnits,
		kern:         f.kern,
	}
}

type metricPreservingFace struct {
	font.Face
	font         *sfnt.Font
	buffer       sfnt.Buffer
	scale        fixed.Int26_6
	unitsPerEm   fixed.Int26_6
	ascentUnits  fixed.Int26_6
	descentUnits fixed.Int26_6
	kern         []kernPair
}

func (f *metricPreservingFace) Metrics() font.Metrics {
	return font.Metrics{
		Height:  f.scale,
		Ascent:  ceilScale(f.scale, f.ascentUnits, f.unitsPerEm),
		Descent: ceilScale(f.scale, f.descentUnits, f.unitsPerEm),
	}
}

func (f *metricPreservingFace) Kern(r0, r1 rune) fixed.Int26_6 {
	i0, err := f.font.GlyphIndex(&f.buffer, r0)
	if err != nil {
		return 0
	}
	i1, err := f.font.GlyphIndex(&f.buffer, r1)
	if err != nil {
		return 0
	}
	key := uint32(i0)<<16 | uint32(i1)
	i := sort.Search(len(f.kern), func(i int) bool {
		return f.kern[i].key >= key
	})
	if i == len(f.kern) || f.kern[i].key != key {
		return 0
	}
	return roundScale(f.scale, fixed.I(int(f.kern[i].value)), f.unitsPerEm)
}

func (f *metricPreservingFace) GlyphBounds(r rune) (fixed.Rectangle26_6, fixed.Int26_6, bool) {
	i, err := f.font.GlyphIndex(&f.buffer, r)
	if err != nil {
		return fixed.Rectangle26_6{}, 0, false
	}
	bounds, advance, err := f.font.GlyphBounds(&f.buffer, i, f.scale, font.HintingNone)
	return bounds, advance, err == nil
}

func (f *metricPreservingFace) GlyphAdvance(r rune) (fixed.Int26_6, bool) {
	i, err := f.font.GlyphIndex(&f.buffer, r)
	if err != nil {
		return 0, false
	}
	advance, err := f.font.GlyphAdvance(&f.buffer, i, f.scale, font.HintingNone)
	return advance, err == nil
}

func ceilScale(scale, units, unitsPerEm fixed.Int26_6) fixed.Int26_6 {
	numerator := int64(scale) * int64(units)
	return fixed.Int26_6((numerator + int64(unitsPerEm) - 1) / int64(unitsPerEm))
}

func roundScale(scale, units, unitsPerEm fixed.Int26_6) fixed.Int26_6 {
	x := int64(scale) * int64(units)
	if x >= 0 {
		x += int64(unitsPerEm) / 2
	} else {
		x -= int64(unitsPerEm) / 2
	}
	return fixed.Int26_6(x / int64(unitsPerEm))
}

func parseLegacyKern(src []byte) ([]kernPair, error) {
	table, err := sfntTable(src, "kern")
	if err != nil || table == nil {
		return nil, err
	}
	if len(table) < 18 {
		return nil, fmt.Errorf("invalid kern table: too short")
	}
	if binary.BigEndian.Uint16(table[0:2]) != 0 {
		return nil, fmt.Errorf("unsupported kern table version")
	}
	if binary.BigEndian.Uint16(table[2:4]) == 0 {
		return nil, fmt.Errorf("invalid kern table: no subtables")
	}

	length := int(binary.BigEndian.Uint16(table[6:8]))
	coverage := binary.BigEndian.Uint16(table[8:10])
	if coverage != 0x0001 {
		return nil, fmt.Errorf("unsupported kern table coverage 0x%04x", coverage)
	}
	n := int(binary.BigEndian.Uint16(table[10:12]))
	if length < 14 || 6*n != length-14 || 4+length > len(table) {
		return nil, fmt.Errorf("invalid kern table length")
	}

	pairs := make([]kernPair, n)
	for i := range pairs {
		offset := 18 + 6*i
		pairs[i] = kernPair{
			key:   binary.BigEndian.Uint32(table[offset : offset+4]),
			value: int16(binary.BigEndian.Uint16(table[offset+4 : offset+6])),
		}
	}
	return pairs, nil
}

func sfntTable(src []byte, tag string) ([]byte, error) {
	if len(src) < 12 {
		return nil, fmt.Errorf("invalid sfnt: too short")
	}

	base := 0
	if string(src[:4]) == "ttcf" {
		if len(src) < 16 || binary.BigEndian.Uint32(src[8:12]) == 0 {
			return nil, fmt.Errorf("invalid font collection")
		}
		base = int(binary.BigEndian.Uint32(src[12:16]))
		if base < 0 || base+12 > len(src) {
			return nil, fmt.Errorf("invalid font collection offset")
		}
	}

	n := int(binary.BigEndian.Uint16(src[base+4 : base+6]))
	records := base + 12
	if n > (len(src)-records)/16 {
		return nil, fmt.Errorf("invalid sfnt table directory")
	}
	for i := 0; i < n; i++ {
		record := src[records+16*i : records+16*(i+1)]
		if string(record[:4]) != tag {
			continue
		}
		offset := int(binary.BigEndian.Uint32(record[8:12]))
		length := int(binary.BigEndian.Uint32(record[12:16]))
		if offset < 0 || length < 0 || offset > len(src)-length {
			return nil, fmt.Errorf("invalid %s table bounds", tag)
		}
		return src[offset : offset+length], nil
	}
	return nil, nil
}
