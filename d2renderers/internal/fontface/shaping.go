package fontface

import (
	"context"
	"fmt"
	"math"
	"sort"
	"unicode/utf8"

	"github.com/go-text/typesetting/di"
	gotextfont "github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/segmenter"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// ShapeFace couples a caller-owned identifier to one parsed OpenType face.
// ParsedFace.Shaping contains mutable lookup caches, so callers must not share
// one ShapeFace concurrently between ShapeText calls.
type ShapeFace struct {
	ID   string
	Face *ParsedFace
}

// ShapeLimits bounds all work which ShapeText cannot interrupt internally.
// Every field must be positive. Callers enforcing document-wide budgets pass
// the remaining aggregate allowance for CoverageChecks, Runs, and Glyphs.
type ShapeLimits struct {
	Runes          int
	Faces          int
	CoverageChecks int64
	Runs           int
	Glyphs         int
}

// ShapedGlyph is renderer-neutral placement for one glyph. Ink stays in the
// source font's fixed-point, Y-down coordinate system and is relative to the
// glyph position. Empty glyphs intentionally carry ID zero and no ink.
type ShapedGlyph struct {
	ID          uint32
	Face        int
	PositionX   float64
	PositionY   float64
	Advance     float64
	Ink         fixed.Rectangle26_6
	HasInk      bool
	Empty       bool
	Source      rune
	SourceIndex int
}

// ShapedText is one visually ordered, left-to-right placed line. Work reports
// the charged units so a caller can update aggregate document/frame budgets.
type ShapedText struct {
	Glyphs         []ShapedGlyph
	Advance        float64
	Runes          int
	CoverageChecks int64
	Runs           int
}

// missingGlyphPlaceholderRunes are ordered from the conventional replacement
// character through outline-box alternatives, with an ASCII last resort.
// Keeping a box before '?' makes an absent scalar visibly distinct from user
// text even when the selected font omits U+FFFD and U+25A1.
var missingGlyphPlaceholderRunes = [...]rune{'\ufffd', '\u25a1', '\u2610', '?'}

// ShapeText applies pure-Go HarfBuzz-compatible shaping with bidi, script,
// ligature, mark, and ordered font-fallback support. Font selection has
// grapheme-cluster affinity: one face must cover every visible rune in a UAX
// #29 extended grapheme cluster. This prevents a combining mark that happens
// to exist in the primary font from being detached from a fallback base.
func ShapeText(ctx context.Context, text string, size fixed.Int26_6, faces []ShapeFace, limits ShapeLimits) (ShapedText, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return ShapedText{}, err
	}
	if size <= 0 {
		return ShapedText{}, fmt.Errorf("d2fonts: shaping size must be positive")
	}
	if limits.Runes <= 0 || limits.Faces <= 0 || limits.CoverageChecks <= 0 || limits.Runs <= 0 || limits.Glyphs <= 0 {
		return ShapedText{}, fmt.Errorf("d2fonts: every shaping limit must be positive")
	}
	if len(faces) == 0 {
		return ShapedText{}, fmt.Errorf("d2fonts: shaping requires at least one font face")
	}
	if len(faces) > limits.Faces {
		return ShapedText{}, fmt.Errorf("d2fonts: font face count %d exceeds limit %d", len(faces), limits.Faces)
	}
	faceIndexes := make(map[*gotextfont.Face]int, len(faces))
	for index, face := range faces {
		if face.Face == nil || face.Face.Outline == nil || face.Face.Shaping == nil {
			return ShapedText{}, fmt.Errorf("d2fonts: font face %d (%q) is nil or incomplete", index, face.ID)
		}
		if _, exists := faceIndexes[face.Face.Shaping]; exists {
			return ShapedText{}, fmt.Errorf("d2fonts: font face %d (%q) duplicates an earlier shaping face", index, face.ID)
		}
		faceIndexes[face.Face.Shaping] = index
	}

	runeCount, err := boundedRuneCount(ctx, text, limits.Runes)
	if err != nil {
		return ShapedText{}, err
	}
	result := ShapedText{Runes: runeCount}
	if runeCount == 0 {
		return result, nil
	}
	runes := make([]rune, 0, runeCount)
	for _, value := range text {
		if len(runes)&1023 == 0 {
			if err := ctx.Err(); err != nil {
				return ShapedText{}, err
			}
		}
		runes = append(runes, value)
	}

	fontMap := clusterFaceResolver{
		faces:    faces,
		coverage: make(map[faceRune]bool, min(runeCount, 256)),
		limits:   limits,
	}
	faceAssignments := make([]int, runeCount)
	replacementGlyphs := make([]uint32, runeCount)
	var graphemes segmenter.Segmenter
	graphemes.Init(runes)
	for iterator := graphemes.GraphemeIterator(); iterator.Next(); {
		if err := ctx.Err(); err != nil {
			return ShapedText{}, err
		}
		cluster := iterator.Grapheme()
		face, replacement, err := fontMap.resolveCluster(cluster.Text, cluster.Offset)
		if err != nil {
			return ShapedText{}, err
		}
		for index := range cluster.Text {
			faceAssignments[cluster.Offset+index] = face
			replacementGlyphs[cluster.Offset+index] = replacement
		}
	}
	result.CoverageChecks = fontMap.checks

	input := shaping.Input{
		Text: runes, RunEnd: len(runes), Direction: di.DirectionLTR, Size: size,
	}
	var runSegmenter shaping.Segmenter
	semanticInputs := runSegmenter.Split(input, oneFaceMap{face: faces[0].Face.Shaping})
	if err := ctx.Err(); err != nil {
		return ShapedText{}, err
	}
	inputs, err := splitInputsByAssignedFace(semanticInputs, faceAssignments, faces, limits.Runs)
	if err != nil {
		return ShapedText{}, err
	}
	if len(inputs) > limits.Runs {
		return ShapedText{}, fmt.Errorf("d2fonts: text shaping run count %d exceeds limit %d", len(inputs), limits.Runs)
	}
	result.Runs = len(inputs)

	shaper := shaping.HarfbuzzShaper{}
	runs := make([]shapedFontRun, 0, len(inputs))
	totalGlyphs := 0
	for index, input := range inputs {
		if err := ctx.Err(); err != nil {
			return ShapedText{}, err
		}
		output := shaper.Shape(input)
		if err := ctx.Err(); err != nil {
			return ShapedText{}, err
		}
		if len(output.Glyphs) > limits.Glyphs-totalGlyphs {
			return ShapedText{}, fmt.Errorf("d2fonts: shaped glyph count exceeds limit %d", limits.Glyphs)
		}
		totalGlyphs += len(output.Glyphs)
		faceIndex, ok := faceIndexes[output.Face]
		if !ok {
			return ShapedText{}, fmt.Errorf("d2fonts: shaper run %d returned an unknown font face", index)
		}
		runs = append(runs, shapedFontRun{output: output, face: faceIndex, visual: index})
	}
	orderShapedRunsLTR(runs)
	sort.SliceStable(runs, func(i, j int) bool { return runs[i].visual < runs[j].visual })

	result.Glyphs = make([]ShapedGlyph, 0, totalGlyphs)
	pen := 0.0
	type glyphKey struct {
		face int
		id   sfnt.GlyphIndex
	}
	type glyphOutline struct {
		bounds fixed.Rectangle26_6
		hasInk bool
	}
	outlines := make(map[glyphKey]glyphOutline, min(totalGlyphs, 256))
	for _, run := range runs {
		parsed := faces[run.face].Face
		runPen := pen
		var metricsBuffer sfnt.Buffer
		for glyphIndex, glyph := range run.output.Glyphs {
			if err := ctx.Err(); err != nil {
				return ShapedText{}, err
			}
			source, sourceAt := shapedGlyphSource(runes, glyph.ClusterIndex)
			positionX := runPen + fixedToFloat(glyph.XOffset)
			positionY := -fixedToFloat(glyph.YOffset)
			advance := fixedToFloat(glyph.XAdvance)
			if glyph.GlyphID == gotextfont.EmptyGlyph {
				result.Glyphs = append(result.Glyphs, ShapedGlyph{
					Face: run.face, PositionX: positionX, PositionY: positionY, Advance: advance,
					Empty: true, Source: source, SourceIndex: sourceAt,
				})
				runPen += advance
				continue
			}
			glyphID := uint32(glyph.GlyphID)
			if glyph.GlyphID == 0 {
				if sourceAt < 0 || sourceAt >= len(replacementGlyphs) || replacementGlyphs[sourceAt] == 0 {
					return ShapedText{}, missingShapedGlyphError(runes, glyph.ClusterIndex, faces[run.face].ID)
				}
				glyphID = replacementGlyphs[sourceAt]
				positionX = runPen
				positionY = 0
				replacementAdvance, err := parsed.Outline.GlyphAdvance(
					&metricsBuffer, sfnt.GlyphIndex(glyphID), size, font.HintingNone,
				)
				if err != nil {
					return ShapedText{}, fmt.Errorf("d2fonts: measure missing-glyph placeholder %d in font %q: %w", glyphID, faces[run.face].ID, err)
				}
				advance = fixedToFloat(replacementAdvance)
			}
			if glyphID > math.MaxUint16 || int(glyphID) >= parsed.Outline.NumGlyphs() {
				return ShapedText{}, fmt.Errorf("d2fonts: shaped glyph ID %d at index %d is out of range for font %q", glyphID, glyphIndex, faces[run.face].ID)
			}
			key := glyphKey{face: run.face, id: sfnt.GlyphIndex(glyphID)}
			outline, ok := outlines[key]
			if !ok {
				bounds, hasInk, err := parsed.GlyphRenderBounds(uint32(key.id), size)
				if err != nil {
					return ShapedText{}, unsupportedShapedGlyphError(parsed.Shaping, gotextfont.GID(glyphID), source, sourceAt, faces[run.face].ID, err)
				}
				outline.bounds = bounds
				outline.hasInk = hasInk
				outlines[key] = outline
			}
			result.Glyphs = append(result.Glyphs, ShapedGlyph{
				ID: glyphID, Face: run.face, PositionX: positionX, PositionY: positionY, Advance: advance,
				Ink: outline.bounds, HasInk: outline.hasInk, Source: source, SourceIndex: sourceAt,
			})
			runPen += advance
		}
		pen = runPen
	}
	result.Advance = pen
	return result, nil
}

func fixedToFloat(value fixed.Int26_6) float64 {
	return float64(value) / 64
}

func boundedRuneCount(ctx context.Context, value string, limit int) (int, error) {
	count := 0
	for offset := 0; offset < len(value); {
		if count&1023 == 0 {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
		}
		decoded, size := utf8.DecodeRuneInString(value[offset:])
		if decoded == utf8.RuneError && size == 1 {
			return 0, fmt.Errorf("d2fonts: text is not valid UTF-8 at byte %d", offset)
		}
		count++
		if count > limit {
			return 0, fmt.Errorf("d2fonts: text rune count exceeds per-run limit %d", limit)
		}
		offset += size
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return count, nil
}

type faceRune struct {
	face  int
	value rune
}

type clusterFaceResolver struct {
	faces    []ShapeFace
	coverage map[faceRune]bool
	known    map[faceRune]bool
	checks   int64
	limits   ShapeLimits
}

func (m *clusterFaceResolver) supports(face int, value rune) (bool, error) {
	key := faceRune{face: face, value: value}
	if m.known != nil && m.known[key] {
		return m.coverage[key], nil
	}
	if m.checks >= m.limits.CoverageChecks {
		return false, fmt.Errorf("d2fonts: text font coverage checks exceed limit %d", m.limits.CoverageChecks)
	}
	m.checks++
	supported, err := m.faces[face].Face.SupportsRenderableRune(value)
	if err != nil {
		return false, err
	}
	if m.known == nil {
		m.known = make(map[faceRune]bool, min(m.limits.Runes, 256))
	}
	m.known[key] = true
	m.coverage[key] = supported
	return supported, nil
}

func (m *clusterFaceResolver) resolveCluster(values []rune, offset int) (int, uint32, error) {
	for face := range m.faces {
		covers := true
		for _, value := range values {
			if IsDefaultIgnorableRune(value) {
				continue
			}
			supported, err := m.supports(face, value)
			if err != nil {
				return 0, 0, err
			}
			if !supported {
				covers = false
				break
			}
		}
		if covers {
			return face, 0, nil
		}
	}

	// Keep the grapheme on one face and ask the shaping loop to replace any
	// resulting glyph zero with that face's deterministic placeholder. This
	// covers both a genuinely absent scalar and a cluster whose visible runes
	// exist only across different faces.
	for _, replacement := range missingGlyphPlaceholderRunes {
		for face := range m.faces {
			supported, err := m.supports(face, replacement)
			if err != nil {
				return 0, 0, err
			}
			if !supported {
				continue
			}
			glyph, ok := m.faces[face].Face.Shaping.NominalGlyph(replacement)
			if !ok || glyph == 0 || glyph > math.MaxUint16 {
				continue
			}
			return face, uint32(glyph), nil
		}
	}
	return 0, 0, fmt.Errorf("d2fonts: no font face has a drawable missing-glyph placeholder for grapheme cluster beginning at rune %d", offset)
}

type oneFaceMap struct {
	face *gotextfont.Face
}

func (m oneFaceMap) ResolveFace(rune) *gotextfont.Face { return m.face }

// splitInputsByAssignedFace preserves the bidi/script/language runs computed
// by go-text while imposing occurrence-specific grapheme face choices. A
// rune-keyed Fontmap cannot express that the same combining mark uses primary
// in one cluster and fallback in another, so this split must happen by index.
func splitInputsByAssignedFace(inputs []shaping.Input, assignments []int, faces []ShapeFace, maxRuns int) ([]shaping.Input, error) {
	result := make([]shaping.Input, 0, min(len(inputs), maxRuns))
	for _, input := range inputs {
		if input.RunStart < 0 || input.RunStart >= input.RunEnd || input.RunEnd > len(assignments) {
			return nil, fmt.Errorf("d2fonts: shaper segment has invalid rune range [%d:%d]", input.RunStart, input.RunEnd)
		}
		start := input.RunStart
		face := assignments[start]
		for index := start + 1; index <= input.RunEnd; index++ {
			if index < input.RunEnd && assignments[index] == face {
				continue
			}
			if face < 0 || face >= len(faces) {
				return nil, fmt.Errorf("d2fonts: invalid assigned font face %d", face)
			}
			if len(result) >= maxRuns {
				return nil, fmt.Errorf("d2fonts: text shaping run count exceeds limit %d", maxRuns)
			}
			run := input
			run.RunStart = start
			run.RunEnd = index
			run.Face = faces[face].Face.Shaping
			result = append(result, run)
			if index < input.RunEnd {
				start = index
				face = assignments[index]
			}
		}
	}
	return result, nil
}

type shapedFontRun struct {
	output shaping.Output
	face   int
	visual int
}

// orderShapedRunsLTR mirrors go-text's UAX #9 run-order post-processing for
// D2/SVG's default left-to-right paragraph direction. HarfBuzz already emits
// each individual run in visual glyph order.
func orderShapedRunsLTR(runs []shapedFontRun) {
	bidiStart := -1
	swap := func(from, to int) {
		for left, right := from, to-1; left < right; left, right = left+1, right-1 {
			runs[left].visual, runs[right].visual = runs[right].visual, runs[left].visual
		}
	}
	for index := range runs {
		runs[index].visual = index
		if runs[index].output.Direction == di.DirectionLTR {
			if bidiStart != -1 {
				swap(bidiStart, index)
				bidiStart = -1
			}
		} else if bidiStart == -1 {
			bidiStart = index
		}
	}
	if bidiStart != -1 {
		swap(bidiStart, len(runs))
	}
}

func shapedGlyphSource(runes []rune, cluster int) (rune, int) {
	if cluster >= 0 && cluster < len(runes) {
		return runes[cluster], cluster
	}
	return 0, cluster
}

func missingShapedGlyphError(runes []rune, cluster int, face string) error {
	if cluster >= 0 && cluster < len(runes) {
		return fmt.Errorf("d2fonts: missing shaped glyph for U+%04X at rune %d in font %q", runes[cluster], cluster, face)
	}
	return fmt.Errorf("d2fonts: missing shaped glyph at cluster %d in font %q", cluster, face)
}

func unsupportedShapedGlyphError(face *gotextfont.Face, glyph gotextfont.GID, source rune, sourceAt int, id string, cause error) error {
	kind := "non-outline"
	switch face.GlyphData(glyph).(type) {
	case gotextfont.GlyphOutline:
		kind = "outline"
	case gotextfont.GlyphColor:
		kind = "color"
	case gotextfont.GlyphBitmap:
		kind = "bitmap"
	case gotextfont.GlyphSVG:
		kind = "SVG"
	case nil:
		kind = "missing"
	}
	if sourceAt >= 0 {
		return fmt.Errorf("d2fonts: %s glyph U+%04X at rune %d in font %q cannot be rasterized: %w", kind, source, sourceAt, id, cause)
	}
	return fmt.Errorf("d2fonts: %s glyph at cluster %d in font %q cannot be rasterized: %w", kind, sourceAt, id, cause)
}
