package fontface

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"image/color"
	"math"

	gotextfont "github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/font/opentype/tables"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

const (
	maxParsedFontFaces  = 256
	maxCOLR0GlyphLayers = 1_024
)

var errShapingParserPanic = errors.New("shaping font parser panicked")

var errColorGlyphLookupPanic = errors.New("color glyph table lookup panicked")

type shapingCollectionParser func(gotextfont.Resource) ([]*gotextfont.Face, error)

// FaceCountLimitError reports a collection rejected before the shaping parser
// constructs one Face per entry.
type FaceCountLimitError struct {
	Count int
	Limit int
}

func (e *FaceCountLimitError) Error() string {
	return fmt.Sprintf("font collection face count %d exceeds limit %d", e.Count, e.Limit)
}

// Collection parses one font source with both libraries used by the
// raster text pipeline. A source is accepted only when outline lookup and
// HarfBuzz-compatible shaping agree on its face topology.
type Collection struct {
	outline *opentype.Collection
	shaping []*gotextfont.Face
	digest  [sha256.Size]byte
}

// parsedFaceSource is parser-issued provenance bound to the exact outline,
// shaping face, and immutable shaping font selected from one source. Keeping
// this unexported prevents sibling renderer packages from authorizing a
// synthesized ParsedFace by copying a digest into an exported field.
type parsedFaceSource struct {
	digest         [sha256.Size]byte
	outline        *sfnt.Font
	shaping        *gotextfont.Face
	shapingFont    *gotextfont.Font
	bundledOutline bool
}

// ParsedFace is one matching outline/shaping face. The Shaping face owns
// mutable lookup caches and must not be shared across concurrent renders.
type ParsedFace struct {
	Outline *sfnt.Font
	Shaping *gotextfont.Face
	source  parsedFaceSource
}

// BundledFaceSource is an authenticated, package-owned clone source. Its
// parser state is not exposed.
type BundledFaceSource struct {
	face *ParsedFace
}

// BundledNotoColorEmojiSource preserves the specific name used by the bundled
// emoji resolver and its authenticated color-paint and coverage APIs.
type BundledNotoColorEmojiSource = BundledFaceSource

func (s *BundledFaceSource) parsed() (*ParsedFace, error) {
	if s == nil || s.face == nil || !s.face.hasParsedSourceComponents() {
		return nil, fmt.Errorf("bundled font clone source is unavailable")
	}
	return s.face, nil
}

// Outline returns the immutable sfnt reader for this authenticated source.
// sfnt.Font has no mutable exported state; callers supply their own Buffer for
// every operation, so the reader may safely be shared by concurrent renders.
func (s *BundledFaceSource) Outline() (*sfnt.Font, error) {
	face, err := s.parsed()
	if err != nil {
		return nil, err
	}
	return face.Outline, nil
}

// IsBundledNotoColorEmoji reports whether this source is D2's authenticated
// color-emoji font without exposing its parser face.
func (s *BundledFaceSource) IsBundledNotoColorEmoji() bool {
	face, err := s.parsed()
	return err == nil && face.IsBundledNotoColorEmoji()
}

// COLR0GlyphLayers resolves immutable COLRv0 table data without allocating a
// mutable go-text Face clone. Returned layers are detached value records.
func (s *BundledFaceSource) COLR0GlyphLayers(glyphID uint32) ([]ColorGlyphLayer, bool, error) {
	face, err := s.parsed()
	if err != nil {
		return nil, false, err
	}
	return face.COLR0GlyphLayers(glyphID)
}

// CompileBundledNotoColorEmojiCOLRv1Plan compiles an immutable renderer plan
// through the authenticated package-private color tables.
func (s *BundledFaceSource) CompileBundledNotoColorEmojiCOLRv1Plan(glyphID uint32) (*COLRv1Plan, bool, error) {
	face, err := s.parsed()
	if err != nil {
		return nil, false, err
	}
	return face.CompileBundledNotoColorEmojiCOLRv1Plan(glyphID)
}

// GlyphRenderBounds reads glyph paint bounds without constructing per-Face
// shaping caches.
func (s *BundledFaceSource) GlyphRenderBounds(glyphID uint32, size fixed.Int26_6) (fixed.Rectangle26_6, bool, error) {
	face, err := s.parsed()
	if err != nil {
		return fixed.Rectangle26_6{}, false, err
	}
	return face.GlyphRenderBounds(glyphID, size)
}

// GlyphDataKind reports only the diagnostic category of a glyph. Keeping the
// table-backed GlyphData value private prevents callers from retaining or
// modifying parser-owned slices while still preserving renderer errors.
func (s *BundledFaceSource) GlyphDataKind(glyphID uint32) (string, error) {
	face, err := s.parsed()
	if err != nil {
		return "", err
	}
	return glyphDataKind(face.Shaping.GlyphData(gotextfont.GID(glyphID))), nil
}

func glyphDataKind(data gotextfont.GlyphData) string {
	switch data.(type) {
	case gotextfont.GlyphOutline:
		return "outline"
	case gotextfont.GlyphColor:
		return "color"
	case gotextfont.GlyphBitmap:
		return "bitmap"
	case gotextfont.GlyphSVG:
		return "SVG"
	case nil:
		return "missing"
	default:
		return "non-outline"
	}
}

// CloneReadOnly returns independent shaping caches while sharing go-text's
// documented read-only Font. It is for renderer-owned faces whose parsed table
// fields are never reassigned by the caller.
func (s *BundledFaceSource) CloneReadOnly() (*ParsedFace, error) {
	face, err := s.parsed()
	if err != nil {
		return nil, err
	}
	clone := new(ParsedFace)
	if err := face.CloneInto(clone); err != nil {
		return nil, err
	}
	return clone, nil
}

// CloneReadOnlyInto is CloneReadOnly with caller-owned result storage.
func (s *BundledFaceSource) CloneReadOnlyInto(clone *ParsedFace) error {
	face, err := s.parsed()
	if err != nil {
		return err
	}
	return face.CloneInto(clone)
}

// SupportsRenderableRune checks exact bundled-font coverage without mutating a
// shaping face cache, so one registered source can serve concurrent resolvers.
// The fixed ordinary D2 fonts contain only authenticated outline glyphs, so a
// matching non-zero cmap entry is sufficient. The bundled color-emoji source
// additionally validates its supported COLR paint path.
func (s *BundledFaceSource) SupportsRenderableRune(value rune) (bool, error) {
	face, err := s.parsed()
	if err != nil {
		return false, err
	}
	shapingGlyph, ok := face.Shaping.Font.NominalGlyph(value)
	if !ok || shapingGlyph == 0 {
		return false, nil
	}
	var buffer sfnt.Buffer
	outlineGlyph, err := face.Outline.GlyphIndex(&buffer, value)
	if err != nil || outlineGlyph == 0 || uint32(outlineGlyph) != uint32(shapingGlyph) {
		return false, nil
	}
	if !face.hasParsedSource(bundledNotoColorEmojiCOLRv1SHA256) {
		return true, nil
	}
	plan, found, err := face.CompileBundledNotoColorEmojiCOLRv1Plan(uint32(outlineGlyph))
	if err != nil {
		return false, err
	}
	if found {
		return plan.Clip != nil && plan.Clip.XMin < plan.Clip.XMax && plan.Clip.YMin < plan.Clip.YMax, nil
	}
	layers, colorGlyph, err := face.COLR0GlyphLayers(uint32(outlineGlyph))
	if err != nil {
		return false, err
	}
	if colorGlyph {
		for _, layer := range layers {
			if _, err := face.Outline.LoadGlyph(&buffer, sfnt.GlyphIndex(layer.GlyphID), fixed.I(1_000), nil); err != nil {
				return false, fmt.Errorf("load COLRv0 layer glyph %d: %w", layer.GlyphID, err)
			}
		}
		return true, nil
	}
	if _, err := face.Outline.LoadGlyph(&buffer, outlineGlyph, fixed.I(1_000), nil); err != nil {
		return false, nil
	}
	return true, nil
}

// ColorGlyphLayer is one foreground or palette-colored outline in a COLRv0
// glyph. Layers are returned in paint order.
type ColorGlyphLayer struct {
	GlyphID    uint32
	Color      color.NRGBA
	Foreground bool
}

func parseFaceCollection(data []byte) (*Collection, error) {
	return ParseFaceCollectionWithLimit(data, maxParsedFontFaces)
}

// ParseFaceCollectionWithLimit inspects the cheap sfnt collection directory
// first and rejects excessive face counts before go-text eagerly constructs
// its per-face parser state.
func ParseFaceCollectionWithLimit(data []byte, maxFaces int) (*Collection, error) {
	maxFaces = min(maxFaces, maxParsedFontFaces)
	return parseFaceCollectionWithLimitUsingParser(data, maxFaces, gotextfont.ParseTTC)
}

func parseFaceCollectionWithLimitUsingParser(data []byte, maxFaces int, parseShaping shapingCollectionParser) (*Collection, error) {
	collection, err := parseFaceCollectionWithLimitUsingParserAndDigest(data, maxFaces, parseShaping, [sha256.Size]byte{})
	if err != nil {
		return nil, err
	}
	if len(data) == bundledNotoColorEmojiCOLRv1Size {
		collection.digest = sha256.Sum256(data)
	}
	return collection, nil
}

func parseFaceCollectionWithLimitUsingParserAndDigest(data []byte, maxFaces int, parseShaping shapingCollectionParser, digest [sha256.Size]byte) (*Collection, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty font data")
	}
	if maxFaces <= 0 {
		return nil, fmt.Errorf("font face limit must be positive")
	}
	outline, err := opentype.ParseCollection(data)
	if err != nil {
		return nil, fmt.Errorf("parse TrueType/OpenType font: %w", err)
	}
	if outline.NumFonts() > maxFaces {
		return nil, &FaceCountLimitError{Count: outline.NumFonts(), Limit: maxFaces}
	}
	shaping, err := parseShapingCollection(bytes.NewReader(data), parseShaping)
	if err != nil {
		return nil, fmt.Errorf("parse shaping font: %w", err)
	}
	if outline.NumFonts() != len(shaping) {
		return nil, fmt.Errorf("font parser face-count mismatch: outline=%d shaping=%d", outline.NumFonts(), len(shaping))
	}
	return &Collection{outline: outline, shaping: shaping, digest: digest}, nil
}

// Contain go-text parser panics caused by malformed or unsupported external
// font bytes so they become ordinary parse failures instead of terminating an
// export.
func parseShapingCollection(resource gotextfont.Resource, parse shapingCollectionParser) (faces []*gotextfont.Face, err error) {
	if parse == nil {
		return nil, fmt.Errorf("nil shaping font parser")
	}
	defer func() {
		if recover() != nil {
			faces = nil
			err = errShapingParserPanic
		}
	}()
	return parse(resource)
}

// NumFaces reports the number of matching outline and shaping faces.
func (c *Collection) NumFaces() int {
	if c == nil {
		return 0
	}
	return len(c.shaping)
}

// Face returns one parser-authenticated face by zero-based index.
func (c *Collection) Face(index int) (*ParsedFace, error) {
	if c == nil || index < 0 || index >= c.NumFaces() {
		return nil, fmt.Errorf("load font face %d: collection has %d faces", index, c.NumFaces())
	}
	outline, err := c.outline.Font(index)
	if err != nil {
		return nil, fmt.Errorf("load outline font face %d: %w", index, err)
	}
	return newParsedFace(outline, c.shaping[index], c.digest), nil
}

func newParsedFace(outline *sfnt.Font, shaping *gotextfont.Face, digest [sha256.Size]byte) *ParsedFace {
	face := &ParsedFace{Outline: outline, Shaping: shaping}
	if outline != nil && shaping != nil && shaping.Font != nil {
		face.source = parsedFaceSource{digest: digest, outline: outline, shaping: shaping, shapingFont: shaping.Font}
	}
	return face
}

// Clone returns a face with fresh shaping caches while preserving provenance
// only when this value still contains the exact parser-selected components.
// It is safe to use the resulting face independently in another render.
func (f *ParsedFace) Clone() (*ParsedFace, error) {
	if f == nil || f.Outline == nil || f.Shaping == nil || f.Shaping.Font == nil {
		return nil, fmt.Errorf("nil parsed font face")
	}
	clone := new(ParsedFace)
	if err := f.CloneInto(clone); err != nil {
		return nil, err
	}
	return clone, nil
}

// CloneInto is Clone with caller-owned result storage.
func (f *ParsedFace) CloneInto(clone *ParsedFace) error {
	if f == nil || f.Outline == nil || f.Shaping == nil || f.Shaping.Font == nil {
		return fmt.Errorf("nil parsed font face")
	}
	if clone == nil {
		return fmt.Errorf("nil parsed font face clone destination")
	}
	outline, shapingFont := f.Outline, f.Shaping.Font
	digest, bundledOutline, sourced := f.source.digest, f.source.bundledOutline, f.hasParsedSourceComponents()
	shaping := gotextfont.NewFace(shapingFont)
	result := ParsedFace{Outline: outline, Shaping: shaping}
	if sourced {
		result.source = parsedFaceSource{
			digest: digest, outline: outline, shaping: shaping, shapingFont: shapingFont, bundledOutline: bundledOutline,
		}
	}
	*clone = result
	return nil
}

func (f *ParsedFace) hasParsedSourceComponents() bool {
	return f != nil && f.source.outline != nil && f.Outline == f.source.outline && f.Shaping == f.source.shaping && f.Shaping != nil && f.Shaping.Font == f.source.shapingFont
}

func (f *ParsedFace) hasParsedSource(digest [sha256.Size]byte) bool {
	return digest != ([sha256.Size]byte{}) && f.hasParsedSourceComponents() && f.source.digest == digest
}

// IsBundledNotoColorEmoji reports whether this face retains parser-issued
// provenance for D2's exact bundled color-emoji resource.
func (f *ParsedFace) IsBundledNotoColorEmoji() bool {
	return f.hasParsedSource(bundledNotoColorEmojiCOLRv1SHA256)
}

// ParseFace parses one face from a TrueType, OpenType, or collection resource.
func ParseFace(data []byte, faceIndex uint16) (*ParsedFace, error) {
	collection, err := parseFaceCollection(data)
	if err != nil {
		return nil, err
	}
	face, err := collection.Face(int(faceIndex))
	if err != nil {
		return nil, err
	}
	if face.IsBundledNotoColorEmoji() {
		if _, err := registerAuthenticatedBundledNotoColorEmoji(data, collection.digest); err != nil {
			return nil, err
		}
	}
	return face, nil
}

// SupportsRenderableRune reports whether both parsers select the same non-zero
// glyph and the raster pipeline can paint its base outline, COLRv0/CPAL layers,
// or an authenticated bundled COLRv1 plan. Empty outline glyphs are valid:
// spaces and spacing format controls can intentionally paint no contour while
// still contributing advance.
func (f *ParsedFace) SupportsRenderableRune(value rune) (bool, error) {
	if f == nil || f.Outline == nil || f.Shaping == nil {
		return false, fmt.Errorf("nil parsed font face")
	}
	if f.hasParsedSourceComponents() && f.source.bundledOutline {
		shapingGlyph, ok := f.Shaping.Font.NominalGlyph(value)
		if !ok || shapingGlyph == 0 {
			return false, nil
		}
		var buffer sfnt.Buffer
		outlineGlyph, err := f.Outline.GlyphIndex(&buffer, value)
		return err == nil && outlineGlyph != 0 && uint32(outlineGlyph) == uint32(shapingGlyph), nil
	}
	shapingGlyph, ok := f.Shaping.NominalGlyph(value)
	if !ok || shapingGlyph == 0 {
		return false, nil
	}
	var buffer sfnt.Buffer
	outlineGlyph, err := f.Outline.GlyphIndex(&buffer, value)
	if err != nil || outlineGlyph == 0 || uint32(outlineGlyph) != uint32(shapingGlyph) {
		return false, nil
	}
	plan, trusted, err := f.trustedCOLRv1Plan(uint32(outlineGlyph))
	if err != nil {
		return false, err
	}
	if trusted {
		return plan.Clip != nil && plan.Clip.XMin < plan.Clip.XMax && plan.Clip.YMin < plan.Clip.YMax, nil
	}
	return supportsRenderableRune(f.Outline, f.Shaping, value)
}

// trustedCOLRv1Plan authenticates the one bundled COLRv1 resource before
// examining its paint graph. ParsedFace values synthesized by sibling packages
// lack parser-issued provenance and therefore follow the conservative
// outline/COLRv0 path.
func (f *ParsedFace) trustedCOLRv1Plan(glyphID uint32) (*COLRv1Plan, bool, error) {
	if !f.hasParsedSource(bundledNotoColorEmojiCOLRv1SHA256) {
		return nil, false, nil
	}
	return f.CompileBundledNotoColorEmojiCOLRv1Plan(glyphID)
}

// supportsRenderableRune applies the digest-neutral coverage predicate used
// when no ParsedFace source provenance is available. It intentionally admits
// only outlines and COLRv0/CPAL layers.
func supportsRenderableRune(outline *sfnt.Font, shaping *gotextfont.Face, value rune) (bool, error) {
	if outline == nil || shaping == nil {
		return false, fmt.Errorf("nil outline or shaping face")
	}
	shapingGlyph, ok := shaping.NominalGlyph(value)
	if !ok || shapingGlyph == 0 {
		return false, nil
	}
	var buffer sfnt.Buffer
	outlineGlyph, err := outline.GlyphIndex(&buffer, value)
	if err != nil || outlineGlyph == 0 || uint32(outlineGlyph) != uint32(shapingGlyph) {
		return false, nil
	}
	face := &ParsedFace{Outline: outline, Shaping: shaping}
	layers, colorGlyph, err := face.COLR0GlyphLayers(uint32(outlineGlyph))
	if err != nil {
		return false, err
	}
	if colorGlyph {
		for _, layer := range layers {
			if _, err := outline.LoadGlyph(&buffer, sfnt.GlyphIndex(layer.GlyphID), fixed.I(1_000), nil); err != nil {
				return false, fmt.Errorf("load COLRv0 layer glyph %d: %w", layer.GlyphID, err)
			}
		}
		return true, nil
	}
	if _, err := outline.LoadGlyph(&buffer, outlineGlyph, fixed.I(1_000), nil); err != nil {
		return false, nil
	}
	return true, nil
}

// COLR0GlyphLayers returns the default-palette layers for one supported
// COLRv0 glyph. A false second result means the glyph should use its ordinary
// outline; this preserves outline fallback for other color-font formats.
func (f *ParsedFace) COLR0GlyphLayers(glyphID uint32) ([]ColorGlyphLayer, bool, error) {
	if f == nil || f.Outline == nil || f.Shaping == nil {
		return nil, false, fmt.Errorf("nil parsed font face")
	}
	if glyphID == 0 || glyphID > math.MaxUint16 || int(glyphID) >= f.Outline.NumGlyphs() {
		return nil, false, fmt.Errorf("glyph ID %d is out of range", glyphID)
	}
	if f.Shaping.COLR == nil {
		return nil, false, nil
	}
	paint, ok, err := searchCOLRPaint(f.Shaping.COLR, tables.GlyphID(glyphID))
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, nil
	}
	return f.resolveColorPaint(glyphID, paint)
}

func (f *ParsedFace) resolveColorPaint(glyphID uint32, paint tables.PaintTable) ([]ColorGlyphLayer, bool, error) {
	layers, ok := paint.(tables.PaintColrLayersResolved)
	if !ok {
		var buffer sfnt.Buffer
		segments, err := f.Outline.LoadGlyph(&buffer, sfnt.GlyphIndex(glyphID), fixed.I(1_000), nil)
		if err != nil {
			return nil, false, fmt.Errorf("load fallback outline for color glyph %d: %w", glyphID, err)
		}
		if len(segments) == 0 {
			return nil, false, fmt.Errorf("color glyph %d uses unsupported COLRv1 paint without an outline fallback", glyphID)
		}
		return nil, false, nil
	}
	if len(layers) == 0 {
		return nil, true, fmt.Errorf("COLRv0 glyph %d has no layers", glyphID)
	}
	if len(f.Shaping.CPAL) == 0 {
		return nil, true, fmt.Errorf("COLRv0 glyph %d has no CPAL palette", glyphID)
	}
	validated, err := validateCOLR0Layers(layers, f.Shaping.CPAL[0], f.Outline.NumGlyphs())
	if err != nil {
		return nil, true, fmt.Errorf("COLRv0 glyph %d: %w", glyphID, err)
	}
	return validated, true, nil
}

// searchCOLRPaint contains malformed-table panics from go-text's COLRv0 layer
// slice lookup. Font bytes are untrusted, so a corrupt FirstLayerIndex or
// NumLayers must fail one export rather than terminate the process.
func searchCOLRPaint(colr *tables.COLR1, glyphID tables.GlyphID) (paint tables.PaintTable, found bool, err error) {
	if colr == nil {
		return nil, false, nil
	}
	defer func() {
		if recover() != nil {
			paint = nil
			found = false
			err = errColorGlyphLookupPanic
		}
	}()
	paint, found = colr.Search(glyphID)
	return paint, found, nil
}

// GlyphRenderBounds returns paint bounds in the same fixed-point coordinate
// system as sfnt.LoadGlyph. Authenticated COLRv1 glyphs use their static COLR
// clip box; all other faces use outlines or COLRv0 layers.
func (f *ParsedFace) GlyphRenderBounds(glyphID uint32, size fixed.Int26_6) (fixed.Rectangle26_6, bool, error) {
	if f == nil || f.Outline == nil || f.Shaping == nil {
		return fixed.Rectangle26_6{}, false, fmt.Errorf("nil parsed font face")
	}
	if size <= 0 {
		return fixed.Rectangle26_6{}, false, fmt.Errorf("glyph size must be positive")
	}
	plan, trusted, err := f.trustedCOLRv1Plan(glyphID)
	if err != nil {
		return fixed.Rectangle26_6{}, false, err
	}
	if trusted {
		if plan.Clip == nil {
			return fixed.Rectangle26_6{}, false, fmt.Errorf("trusted COLRv1 glyph %d has no static clip box", glyphID)
		}
		return colrv1ClipBounds(plan.Clip, f.Outline.UnitsPerEm(), size)
	}
	layers, colorGlyph, err := f.COLR0GlyphLayers(glyphID)
	if err != nil {
		return fixed.Rectangle26_6{}, false, err
	}
	if !colorGlyph {
		var buffer sfnt.Buffer
		segments, err := f.Outline.LoadGlyph(&buffer, sfnt.GlyphIndex(glyphID), size, nil)
		if err != nil {
			return fixed.Rectangle26_6{}, false, fmt.Errorf("load outline glyph %d: %w", glyphID, err)
		}
		return segments.Bounds(), len(segments) != 0, nil
	}

	var buffer sfnt.Buffer
	var bounds fixed.Rectangle26_6
	hasInk := false
	for _, layer := range layers {
		segments, err := f.Outline.LoadGlyph(&buffer, sfnt.GlyphIndex(layer.GlyphID), size, nil)
		if err != nil {
			return fixed.Rectangle26_6{}, false, fmt.Errorf("load COLRv0 layer glyph %d: %w", layer.GlyphID, err)
		}
		if len(segments) == 0 {
			continue
		}
		if hasInk {
			bounds = bounds.Union(segments.Bounds())
		} else {
			bounds = segments.Bounds()
			hasInk = true
		}
	}
	return bounds, hasInk, nil
}

func colrv1ClipBounds(clip *COLRv1ClipBox, unitsPerEm sfnt.Units, size fixed.Int26_6) (fixed.Rectangle26_6, bool, error) {
	if clip == nil || unitsPerEm == 0 {
		return fixed.Rectangle26_6{}, false, fmt.Errorf("invalid COLRv1 clip bounds")
	}
	if !finiteCOLRv1(clip.XMin) || !finiteCOLRv1(clip.YMin) || !finiteCOLRv1(clip.XMax) || !finiteCOLRv1(clip.YMax) || clip.XMin >= clip.XMax || clip.YMin >= clip.YMax {
		return fixed.Rectangle26_6{}, false, fmt.Errorf("invalid COLRv1 clip bounds")
	}
	scale := float64(size) / float64(unitsPerEm)
	convert := func(value float64) (fixed.Int26_6, error) {
		value *= scale
		if !finiteCOLRv1(value) || value > float64(math.MaxInt32) || value < float64(math.MinInt32) {
			return 0, fmt.Errorf("COLRv1 clip bounds exceed fixed-point range")
		}
		return fixed.Int26_6(math.Round(value)), nil
	}
	xMin, err := convert(clip.XMin)
	if err != nil {
		return fixed.Rectangle26_6{}, false, err
	}
	// COLR clip coordinates are Y-up, while sfnt.LoadGlyph and ShapeText ink
	// bounds are Y-down. Reflect the extrema around the baseline.
	yMin, err := convert(-clip.YMax)
	if err != nil {
		return fixed.Rectangle26_6{}, false, err
	}
	xMax, err := convert(clip.XMax)
	if err != nil {
		return fixed.Rectangle26_6{}, false, err
	}
	yMax, err := convert(-clip.YMin)
	if err != nil {
		return fixed.Rectangle26_6{}, false, err
	}
	if xMin >= xMax || yMin >= yMax {
		return fixed.Rectangle26_6{}, false, fmt.Errorf("COLRv1 clip bounds collapse at requested size")
	}
	return fixed.Rectangle26_6{Min: fixed.Point26_6{X: xMin, Y: yMin}, Max: fixed.Point26_6{X: xMax, Y: yMax}}, true, nil
}

func finiteCOLRv1(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func validateCOLR0Layers(layers []tables.Layer, palette []tables.ColorRecord, numGlyphs int) ([]ColorGlyphLayer, error) {
	if numGlyphs <= 0 {
		return nil, fmt.Errorf("font has no glyphs")
	}
	if len(layers) == 0 {
		return nil, fmt.Errorf("glyph has no layers")
	}
	if len(layers) > maxCOLR0GlyphLayers {
		return nil, fmt.Errorf("layer count %d exceeds limit %d", len(layers), maxCOLR0GlyphLayers)
	}
	validated := make([]ColorGlyphLayer, 0, len(layers))
	for index, layer := range layers {
		if int(layer.GlyphID) >= numGlyphs {
			return nil, fmt.Errorf("layer %d glyph ID %d is out of range", index, layer.GlyphID)
		}
		resolved := ColorGlyphLayer{GlyphID: uint32(layer.GlyphID)}
		if layer.PaletteIndex == math.MaxUint16 {
			resolved.Foreground = true
		} else {
			if int(layer.PaletteIndex) >= len(palette) {
				return nil, fmt.Errorf("layer %d palette index %d is out of range", index, layer.PaletteIndex)
			}
			entry := palette[layer.PaletteIndex]
			resolved.Color = color.NRGBA{R: entry.Red, G: entry.Green, B: entry.Blue, A: entry.Alpha}
		}
		validated = append(validated, resolved)
	}
	return validated, nil
}
