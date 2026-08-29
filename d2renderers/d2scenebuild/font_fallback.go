package d2scenebuild

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/go-text/typesetting/segmenter"
	"golang.org/x/image/math/fixed"

	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/internal/fontface"
	"github.com/d2lang/d2/d2renderers/internal/fontfallback"
)

type sceneTextReference struct {
	node    *d2scene.Node
	value   d2scene.TextRun
	pointer *d2scene.TextRun
	missing map[rune]struct{}
}

func (r *sceneTextReference) setFallbacks(ids []d2scene.AssetID) {
	if r.pointer != nil {
		r.pointer.Fallbacks = append([]d2scene.AssetID(nil), ids...)
		return
	}
	r.value.Fallbacks = append([]d2scene.AssetID(nil), ids...)
	r.node.Primitive = r.value
}

func (r *sceneTextReference) setGlyphs(glyphs []d2scene.Glyph) {
	if r.pointer != nil {
		r.pointer.Glyphs = append([]d2scene.Glyph(nil), glyphs...)
		return
	}
	r.value.Glyphs = append([]d2scene.Glyph(nil), glyphs...)
	r.node.Primitive = r.value
}

type sceneFontCoverage struct {
	font *fontface.ParsedFace
}

type sceneFontFallbackKey struct {
	family string
	style  string
	weight int
}

type sceneFontFallbackBucket struct {
	request    d2fonts.FallbackRequest
	missing    map[rune]struct{}
	references []*sceneTextReference
}

const (
	defaultFontFallbackMaxRunesPerText   = 100_000
	defaultFontFallbackMaxTotalRunes     = 10_000_000
	defaultFontFallbackMaxCoverageChecks = int64(50_000_000)
	defaultFontShapingMaxFacesPerText    = 64
	defaultFontShapingMaxRuns            = 100_000
	defaultFontShapingMaxGlyphs          = 10_000_000
)

// Keep this order aligned with d2fonts' shaping placeholders. U+2610 provides
// an outline-box fallback for D2's bundled Source Sans Pro, which omits U+FFFD
// and U+25A1.
var missingGlyphPlaceholders = [...]rune{'\ufffd', '\u25a1', '\u2610', '?'}

type fontFallbackWorkLimits struct {
	maxRunesPerText   int
	maxTotalRunes     int
	maxCoverageChecks int64
	maxFacesPerText   int
	maxShapingRuns    int
	maxShapedGlyphs   int
}

func (c sceneFontCoverage) supports(ctx context.Context, value rune) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return c.font.SupportsRenderableRune(value)
}

// resolveFontFallbacks is deliberately a post-build stage: structured rows,
// highlighted code, Markdown spans, legends, and tooltips may render text that
// is not present in their source object's top-level Label field. Walking the
// completed typed scene guarantees every emitted TextRun gets the same policy.
func (b *builder) resolveFontFallbacks(root *d2scene.Node) error {
	references, err := b.sceneTextReferences(root)
	if err != nil {
		return err
	}
	workLimits, err := normalizedFontFallbackWorkLimits(b.options.Fonts)
	if err != nil {
		return err
	}
	coverage := make(map[d2scene.AssetID]sceneFontCoverage)
	missingAll := make(map[rune]struct{})
	bucketsByKey := make(map[sceneFontFallbackKey]*sceneFontFallbackBucket)
	var buckets []*sceneFontFallbackBucket
	var reusableBundledFallbacks []d2scene.AssetID
	supports := func(id d2scene.AssetID, value rune) (bool, error) {
		if b.fontCoverageChecks >= workLimits.maxCoverageChecks {
			return false, fmt.Errorf("scene: font coverage checks exceed limit %d", workLimits.maxCoverageChecks)
		}
		b.fontCoverageChecks++
		face, err := b.fontCoverage(id, coverage)
		if err != nil {
			return false, err
		}
		return face.supports(b.ctx, value)
	}
	for _, reference := range references {
		if len(reference.value.Glyphs) != 0 || reference.value.Text == "" {
			continue
		}
		if !utf8.ValidString(reference.value.Text) {
			return fmt.Errorf("scene: text node %q is not valid UTF-8", reference.node.ID)
		}
		if len(reference.value.Fallbacks) >= workLimits.maxFacesPerText {
			return fmt.Errorf("scene: text node %q fallback font reference count %d exceeds per-text face limit %d", reference.node.ID, len(reference.value.Fallbacks), workLimits.maxFacesPerText)
		}
		fontIDs := make([]d2scene.AssetID, 0, 1+len(reference.value.Fallbacks))
		fontIDs = append(fontIDs, reference.value.Font.Asset)
		fontIDs = append(fontIDs, reference.value.Fallbacks...)
		key := sceneFontFallbackKey{
			family: reference.value.Font.Family, style: reference.value.Font.Style, weight: reference.value.Font.Weight,
		}
		var bucket *sceneFontFallbackBucket
		runes := make([]rune, 0, min(len(reference.value.Text), workLimits.maxRunesPerText))
		for _, value := range reference.value.Text {
			if len(runes) >= workLimits.maxRunesPerText {
				return fmt.Errorf("scene: text node %q rune count exceeds per-text limit %d", reference.node.ID, workLimits.maxRunesPerText)
			}
			if b.fontRunes >= workLimits.maxTotalRunes {
				return fmt.Errorf("scene: text rune count exceeds total limit %d", workLimits.maxTotalRunes)
			}
			b.fontRunes++
			if b.fontRunes&1023 == 0 {
				if err := b.ctx.Err(); err != nil {
					return err
				}
			}
			runes = append(runes, value)
		}
		var graphemes segmenter.Segmenter
		graphemes.Init(runes)
		for iterator := graphemes.GraphemeIterator(); iterator.Next(); {
			if err := b.ctx.Err(); err != nil {
				return err
			}
			cluster := iterator.Grapheme()
			covered := false
			for _, id := range fontIDs {
				faceCoversCluster := true
				for _, value := range cluster.Text {
					if fontRuneIsDefaultIgnorable(value) {
						continue
					}
					supportedByFace, err := supports(id, value)
					if err != nil {
						return fmt.Errorf("scene: text node %q: %w", reference.node.ID, err)
					}
					if !supportedByFace {
						faceCoversCluster = false
						break
					}
				}
				if faceCoversCluster {
					covered = true
					break
				}
			}
			if covered {
				continue
			}
			if reference.missing == nil {
				reference.missing = make(map[rune]struct{})
			}
			for _, value := range cluster.Text {
				if !fontRuneIsDefaultIgnorable(value) {
					reference.missing[value] = struct{}{}
					missingAll[value] = struct{}{}
					if bucket == nil {
						bucket = bucketsByKey[key]
						if bucket == nil {
							bucket = &sceneFontFallbackBucket{
								request: d2fonts.FallbackRequest{Family: key.family, Style: key.style, Weight: key.weight},
								missing: make(map[rune]struct{}),
							}
							bucketsByKey[key] = bucket
							buckets = append(buckets, bucket)
						}
						bucket.references = append(bucket.references, reference)
					}
					bucket.missing[value] = struct{}{}
				}
			}
		}
	}
	retainedAssets, retainedBytes, err := b.retainedFontUsage()
	if err != nil {
		return err
	}
	if b.options.Fonts != nil {
		if b.options.Fonts.MaxAssets <= 0 || b.options.Fonts.MaxBytes <= 0 {
			return fmt.Errorf("scene: font fallback resolver requires positive asset and byte limits")
		}
		if retainedAssets > b.options.Fonts.MaxAssets {
			return fmt.Errorf("scene: retained font asset count %d exceeds limit %d", retainedAssets, b.options.Fonts.MaxAssets)
		}
		if retainedBytes > b.options.Fonts.MaxBytes {
			return fmt.Errorf("scene: retained font bytes %d exceed limit %d", retainedBytes, b.options.Fonts.MaxBytes)
		}
	}
	if len(missingAll) == 0 {
		return nil
	}
	for _, bucket := range buckets {
		resolverMissing := make(map[rune]struct{}, len(bucket.missing))
		for value := range bucket.missing {
			resolverMissing[value] = struct{}{}
		}
		fallbackIDs := make([]d2scene.AssetID, 0, len(reusableBundledFallbacks))
		for _, id := range reusableBundledFallbacks {
			used := false
			for value := range resolverMissing {
				supportedByFace, err := supports(id, value)
				if err != nil {
					return err
				}
				if supportedByFace {
					delete(resolverMissing, value)
					used = true
				}
			}
			if used {
				fallbackIDs = appendUniqueAssetID(fallbackIDs, id)
			}
		}

		var resources []fontfallback.SceneFont
		if b.options.Fonts != nil && b.options.Fonts.Resolver != nil && len(resolverMissing) != 0 {
			bucket.request.Runes = sortedRunes(resolverMissing)
			sceneResources, internal, resolveErr := fontfallback.ResolveForScene(b.ctx, b.options.Fonts.Resolver, fontfallback.Request{
				Runes: bucket.request.Runes, Family: bucket.request.Family, Style: bucket.request.Style, Weight: bucket.request.Weight,
			})
			if resolveErr != nil {
				return fmt.Errorf("scene: font fallback for family %q style %q weight %d: %w", bucket.request.Family, bucket.request.Style, bucket.request.Weight, resolveErr)
			}
			if internal {
				resources = sceneResources
			} else {
				resolved, resolveErr := b.options.Fonts.Resolver.ResolveFallbacks(b.ctx, bucket.request)
				if resolveErr != nil {
					return fmt.Errorf("scene: font fallback for family %q style %q weight %d: %w", bucket.request.Family, bucket.request.Style, bucket.request.Weight, resolveErr)
				}
				resources = make([]fontfallback.SceneFont, len(resolved))
				for index := range resolved {
					resources[index].Font = fontfallback.Font{
						Name: resolved[index].Name, MIMEType: resolved[index].MIMEType,
						Data: resolved[index].Data, FaceIndex: resolved[index].FaceIndex,
					}
				}
			}
		}
		// Bound hostile resolver output before allocating IDs or hashing whole
		// font blobs. Repeated style buckets may legitimately return an already
		// retained resource, so actual remaining capacity is enforced after its
		// content-derived identity is known.
		if b.options.Fonts != nil && len(resources) > b.options.Fonts.MaxAssets {
			return fmt.Errorf("scene: font fallback resolver returned %d resources, exceeding result limit %d", len(resources), b.options.Fonts.MaxAssets)
		}
		returnedBytes := int64(0)
		for index, resource := range resources {
			if int64(len(resource.Data)) > b.options.Fonts.MaxBytes-returnedBytes {
				return fmt.Errorf("scene: font fallback resolver bytes exceed limit %d at resource %d (%s)", b.options.Fonts.MaxBytes, index, safeFallbackName(resource.Name))
			}
			returnedBytes += int64(len(resource.Data))
		}

		for index, resource := range resources {
			if err := b.ctx.Err(); err != nil {
				return err
			}
			if len(resource.Data) == 0 {
				return fmt.Errorf("scene: font fallback %d (%s) has no data", index, safeFallbackName(resource.Name))
			}
			if strings.TrimSpace(resource.MIMEType) == "" {
				return fmt.Errorf("scene: font fallback %d (%s) has no MIME type", index, safeFallbackName(resource.Name))
			}
			id := resource.ID
			if resource.Shared {
				if id == "" || resource.Face == nil {
					return fmt.Errorf("scene: shared font fallback %d (%s) is incomplete", index, safeFallbackName(resource.Name))
				}
			} else {
				id, err = hashFallbackFont(b.ctx, resource.Font)
				if err != nil {
					return err
				}
			}
			if _, exists := b.assets[id]; !exists {
				if retainedAssets >= b.options.Fonts.MaxAssets {
					return fmt.Errorf("scene: retained font asset count exceeds limit %d", b.options.Fonts.MaxAssets)
				}
				if int64(len(resource.Data)) > b.options.Fonts.MaxBytes-retainedBytes {
					return fmt.Errorf("scene: retained font bytes exceed limit %d", b.options.Fonts.MaxBytes)
				}
				data := resource.Data
				if !resource.Shared {
					data = append([]byte(nil), resource.Data...)
				}
				b.assets[id] = d2scene.FontAsset{MIMEType: resource.MIMEType, Data: data, FaceIndex: resource.FaceIndex}
				if resource.Shared {
					b.fontFaces[id] = resource.Face
				}
				retainedAssets++
				retainedBytes += int64(len(resource.Data))
			}
			resolvedCoverage, err := b.fontCoverage(id, coverage)
			if err != nil {
				return fmt.Errorf("scene: font fallback %d (%s): %w", index, safeFallbackName(resource.Name), err)
			}
			if resolvedCoverage.font.IsBundledNotoColorEmoji() {
				reusableBundledFallbacks = appendUniqueAssetID(reusableBundledFallbacks, id)
			}
			fallbackIDs = appendUniqueAssetID(fallbackIDs, id)
		}

		for _, reference := range bucket.references {
			referenceMissing := sortedRunes(reference.missing)
			selected := append([]d2scene.AssetID(nil), reference.value.Fallbacks...)
			for _, id := range fallbackIDs {
				for _, value := range referenceMissing {
					supportedByFace, err := supports(id, value)
					if err != nil {
						return err
					}
					if supportedByFace {
						selected = appendUniqueAssetID(selected, id)
						break
					}
				}
			}
			placeholder := false
			for _, id := range append([]d2scene.AssetID{reference.value.Font.Asset}, selected...) {
				for _, value := range missingGlyphPlaceholders {
					supportedByFace, err := supports(id, value)
					if err != nil {
						return err
					}
					if supportedByFace {
						placeholder = true
						break
					}
				}
				if placeholder {
					break
				}
			}
			if !placeholder {
				id, addedBytes, err := b.ensureMissingGlyphFont(retainedAssets, retainedBytes)
				if err != nil {
					return fmt.Errorf("scene: text node %q: %w", reference.node.ID, err)
				}
				if addedBytes != 0 {
					retainedAssets++
					retainedBytes += addedBytes
				}
				selected = appendUniqueAssetID(selected, id)
			}
			reference.setFallbacks(selected)
		}
	}
	return b.ctx.Err()
}

// shapeTextRuns materializes ordinary text after fallback resolution. Raster
// frames consume only these immutable glyph IDs and placements, so animation
// does not repeat bidi segmentation or HarfBuzz work for every frame.
func (b *builder) shapeTextRuns(root *d2scene.Node) error {
	references, err := b.sceneTextReferences(root)
	if err != nil {
		return err
	}
	workLimits, err := normalizedFontFallbackWorkLimits(b.options.Fonts)
	if err != nil {
		return err
	}
	loadFace := func(id d2scene.AssetID) (*fontface.ParsedFace, error) {
		return b.fontFace(id)
	}

	for _, reference := range references {
		text := reference.value
		if len(text.Glyphs) != 0 || text.Text == "" {
			continue
		}
		if err := b.ctx.Err(); err != nil {
			return err
		}
		if len(text.Fallbacks) >= workLimits.maxFacesPerText {
			return fmt.Errorf("scene: text node %q fallback font reference count %d exceeds per-text face limit %d", reference.node.ID, len(text.Fallbacks), workLimits.maxFacesPerText)
		}
		ids := make([]d2scene.AssetID, 0, 1+len(text.Fallbacks))
		seen := make(map[d2scene.AssetID]bool, 1+len(text.Fallbacks))
		for _, id := range append([]d2scene.AssetID{text.Font.Asset}, text.Fallbacks...) {
			if seen[id] {
				continue
			}
			seen[id] = true
			ids = append(ids, id)
		}
		if len(ids) > workLimits.maxFacesPerText {
			return fmt.Errorf("scene: text node %q font face count %d exceeds per-text limit %d", reference.node.ID, len(ids), workLimits.maxFacesPerText)
		}
		shapeFaces := make([]fontface.ShapeFace, 0, len(ids))
		for _, id := range ids {
			face, err := loadFace(id)
			if err != nil {
				return fmt.Errorf("scene: text node %q: %w", reference.node.ID, err)
			}
			shapeFaces = append(shapeFaces, fontface.ShapeFace{ID: string(id), Face: face})
		}

		remainingCoverage := workLimits.maxCoverageChecks - b.fontCoverageChecks
		if remainingCoverage <= 0 {
			return fmt.Errorf("scene: font coverage checks exceed limit %d", workLimits.maxCoverageChecks)
		}
		remainingRuns := workLimits.maxShapingRuns - b.fontShapeRuns
		if remainingRuns <= 0 {
			return fmt.Errorf("scene: text shaping run count exceeds limit %d", workLimits.maxShapingRuns)
		}
		remainingGlyphs := workLimits.maxShapedGlyphs - b.fontGlyphs
		if remainingGlyphs <= 0 {
			return fmt.Errorf("scene: shaped glyph count exceeds limit %d", workLimits.maxShapedGlyphs)
		}
		if !finite(text.Font.Size) || text.Font.Size <= 0 || text.Font.Size > float64(math.MaxInt32)/64 {
			return fmt.Errorf("scene: text node %q has an invalid font size for shaping", reference.node.ID)
		}
		ppem := fixed.Int26_6(math.Round(text.Font.Size * 64))
		shaped, err := fontface.ShapeText(b.ctx, text.Text, ppem, shapeFaces, fontface.ShapeLimits{
			Runes:          workLimits.maxRunesPerText,
			Faces:          workLimits.maxFacesPerText,
			CoverageChecks: remainingCoverage,
			Runs:           remainingRuns,
			Glyphs:         remainingGlyphs,
		})
		if err != nil {
			return fmt.Errorf("scene: text node %q: %w", reference.node.ID, err)
		}
		glyphs := make([]d2scene.Glyph, 0, len(shaped.Glyphs))
		for _, glyph := range shaped.Glyphs {
			if glyph.Face < 0 || glyph.Face >= len(ids) {
				return fmt.Errorf("scene: text node %q shaper returned invalid face %d", reference.node.ID, glyph.Face)
			}
			ink := d2scene.Bounds{}
			if glyph.HasInk {
				ink = d2scene.NewBounds(
					fixedToSceneFloat(glyph.Ink.Min.X), fixedToSceneFloat(glyph.Ink.Min.Y),
					fixedToSceneFloat(glyph.Ink.Max.X), fixedToSceneFloat(glyph.Ink.Max.Y),
				)
			}
			glyphs = append(glyphs, d2scene.Glyph{
				ID: glyph.ID, Empty: glyph.Empty, Asset: ids[glyph.Face],
				Position: d2scene.Point{X: glyph.PositionX, Y: glyph.PositionY},
				Advance:  glyph.Advance, Ink: ink,
			})
		}
		if len(glyphs) == 0 {
			return fmt.Errorf("scene: text node %q shaped non-empty text to no glyphs", reference.node.ID)
		}
		b.fontCoverageChecks += shaped.CoverageChecks
		b.fontShapeRuns += shaped.Runs
		b.fontGlyphs += len(glyphs)
		reference.setGlyphs(glyphs)
	}
	return b.ctx.Err()
}

func fixedToSceneFloat(value fixed.Int26_6) float64 {
	return float64(value) / 64
}

func normalizedFontFallbackWorkLimits(options *FontFallbackOptions) (fontFallbackWorkLimits, error) {
	limits := fontFallbackWorkLimits{
		maxRunesPerText: defaultFontFallbackMaxRunesPerText, maxTotalRunes: defaultFontFallbackMaxTotalRunes,
		maxCoverageChecks: defaultFontFallbackMaxCoverageChecks,
		maxFacesPerText:   defaultFontShapingMaxFacesPerText, maxShapingRuns: defaultFontShapingMaxRuns,
		maxShapedGlyphs: defaultFontShapingMaxGlyphs,
	}
	if options == nil {
		return limits, nil
	}
	if options.MaxRunesPerText < 0 || options.MaxTotalRunes < 0 || options.MaxCoverageChecks < 0 ||
		options.MaxFontFacesPerText < 0 || options.MaxShapingRuns < 0 || options.MaxShapedGlyphs < 0 {
		return fontFallbackWorkLimits{}, fmt.Errorf("scene: font fallback work limits must not be negative")
	}
	if options.MaxRunesPerText != 0 {
		limits.maxRunesPerText = options.MaxRunesPerText
	}
	if options.MaxTotalRunes != 0 {
		limits.maxTotalRunes = options.MaxTotalRunes
	}
	if options.MaxCoverageChecks != 0 {
		limits.maxCoverageChecks = options.MaxCoverageChecks
	}
	if options.MaxFontFacesPerText != 0 {
		limits.maxFacesPerText = options.MaxFontFacesPerText
	} else if options.MaxAssets > 0 {
		limits.maxFacesPerText = min(limits.maxFacesPerText, options.MaxAssets)
	}
	if options.MaxShapingRuns != 0 {
		limits.maxShapingRuns = options.MaxShapingRuns
	}
	if options.MaxShapedGlyphs != 0 {
		limits.maxShapedGlyphs = options.MaxShapedGlyphs
	}
	return limits, nil
}

func (b *builder) retainedFontUsage() (int, int64, error) {
	count := 0
	bytes := int64(0)
	for id, rawAsset := range b.assets {
		var asset d2scene.FontAsset
		switch value := rawAsset.(type) {
		case d2scene.FontAsset:
			asset = value
		case *d2scene.FontAsset:
			if value == nil {
				return 0, 0, fmt.Errorf("scene: font asset %q is nil", id)
			}
			asset = *value
		default:
			continue
		}
		if int64(len(asset.Data)) > math.MaxInt64-bytes {
			return 0, 0, fmt.Errorf("scene: retained font bytes exceed the int64 domain")
		}
		count++
		bytes += int64(len(asset.Data))
	}
	return count, bytes, nil
}

func (b *builder) fontCoverage(id d2scene.AssetID, cache map[d2scene.AssetID]sceneFontCoverage) (sceneFontCoverage, error) {
	if face, ok := cache[id]; ok {
		return face, nil
	}
	face, err := b.fontFace(id)
	if err != nil {
		return sceneFontCoverage{}, err
	}
	result := sceneFontCoverage{font: face}
	cache[id] = result
	return result, nil
}

// fontFace owns the one parsed face for an asset throughout a scene build.
// Coverage and shaping are sequential stages on the same builder, so sharing
// this mutable go-text cache is safe and avoids parsing every font twice.
func (b *builder) fontFace(id d2scene.AssetID) (*fontface.ParsedFace, error) {
	if b.fontFaces == nil {
		b.fontFaces = make(map[d2scene.AssetID]*fontface.ParsedFace)
	}
	if face := b.fontFaces[id]; face != nil {
		return face, nil
	}
	asset, ok := b.assets[id].(d2scene.FontAsset)
	if !ok {
		return nil, fmt.Errorf("font asset %q is missing or invalid", id)
	}
	face, err := fontface.ParseFace(asset.Data, asset.FaceIndex)
	if err != nil {
		return nil, fmt.Errorf("parse face %d from font asset %q: %w", asset.FaceIndex, id, err)
	}
	b.fontFaces[id] = face
	return face, nil
}

func (b *builder) ensureMissingGlyphFont(retainedAssets int, retainedBytes int64) (d2scene.AssetID, int64, error) {
	id := d2scene.AssetID("font:" + string(d2fonts.SourceSansPro) + ":" + string(d2fonts.FONT_STYLE_REGULAR))
	if _, exists := b.assets[id]; exists {
		face, err := b.fontFace(id)
		if err != nil {
			return "", 0, err
		}
		supported, err := supportsMissingGlyphPlaceholder(face)
		if err != nil || !supported {
			if err != nil {
				return "", 0, err
			}
			return "", 0, fmt.Errorf("bundled missing-glyph font has no drawable placeholder")
		}
		return id, 0, nil
	}
	data, ok := d2fonts.FontFaces.Lookup(d2fonts.Font{Family: d2fonts.SourceSansPro, Style: d2fonts.FONT_STYLE_REGULAR})
	if !ok || len(data) == 0 {
		return "", 0, fmt.Errorf("bundled missing-glyph font is not loaded")
	}
	if b.options.Fonts != nil {
		if retainedAssets >= b.options.Fonts.MaxAssets {
			return "", 0, fmt.Errorf("retained font asset count exceeds limit %d while adding missing-glyph font", b.options.Fonts.MaxAssets)
		}
		if int64(len(data)) > b.options.Fonts.MaxBytes-retainedBytes {
			return "", 0, fmt.Errorf("retained font bytes exceed limit %d while adding missing-glyph font", b.options.Fonts.MaxBytes)
		}
	}
	b.assets[id] = d2scene.FontAsset{MIMEType: "font/ttf", Data: append([]byte(nil), data...)}
	face, err := b.fontFace(id)
	if err != nil {
		delete(b.assets, id)
		delete(b.fontFaces, id)
		return "", 0, err
	}
	supported, err := supportsMissingGlyphPlaceholder(face)
	if err != nil || !supported {
		delete(b.assets, id)
		delete(b.fontFaces, id)
		if err != nil {
			return "", 0, err
		}
		return "", 0, fmt.Errorf("bundled missing-glyph font has no drawable placeholder")
	}
	return id, int64(len(data)), nil
}

func supportsMissingGlyphPlaceholder(face *fontface.ParsedFace) (bool, error) {
	for _, value := range missingGlyphPlaceholders {
		supported, err := face.SupportsRenderableRune(value)
		if err != nil {
			return false, err
		}
		if supported {
			return true, nil
		}
	}
	return false, nil
}

func (b *builder) sceneTextReferences(root *d2scene.Node) ([]*sceneTextReference, error) {
	roots := []*d2scene.Node{root}
	assetIDs := make([]string, 0, len(b.assets))
	for id := range b.assets {
		assetIDs = append(assetIDs, string(id))
	}
	sort.Strings(assetIDs)
	for _, rawID := range assetIDs {
		if vector, ok := b.assets[d2scene.AssetID(rawID)].(d2scene.VectorAsset); ok {
			roots = append(roots, vector.Root)
		}
	}
	seen := make(map[*d2scene.Node]bool)
	stack := append([]*d2scene.Node(nil), roots...)
	var references []*sceneTextReference
	for len(stack) != 0 {
		if err := b.ctx.Err(); err != nil {
			return nil, err
		}
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if node == nil || seen[node] {
			continue
		}
		seen[node] = true
		stack = append(stack, node.Children...)
		if node.Mask != nil {
			stack = append(stack, node.Mask.Root)
		}
		switch primitive := node.Primitive.(type) {
		case d2scene.TextRun:
			references = append(references, &sceneTextReference{node: node, value: primitive})
			stack = appendPaintRoots(stack, primitive.Fill, primitive.Stroke)
		case *d2scene.TextRun:
			if primitive != nil {
				references = append(references, &sceneTextReference{node: node, value: *primitive, pointer: primitive})
				stack = appendPaintRoots(stack, primitive.Fill, primitive.Stroke)
			}
		case d2scene.Rect:
			stack = appendPaintRoots(stack, primitive.Fill, primitive.Stroke)
		case *d2scene.Rect:
			if primitive != nil {
				stack = appendPaintRoots(stack, primitive.Fill, primitive.Stroke)
			}
		case d2scene.Ellipse:
			stack = appendPaintRoots(stack, primitive.Fill, primitive.Stroke)
		case *d2scene.Ellipse:
			if primitive != nil {
				stack = appendPaintRoots(stack, primitive.Fill, primitive.Stroke)
			}
		case d2scene.Path:
			stack = appendPaintRoots(stack, primitive.Fill, primitive.Stroke)
		case *d2scene.Path:
			if primitive != nil {
				stack = appendPaintRoots(stack, primitive.Fill, primitive.Stroke)
			}
		}
	}
	return references, nil
}

func appendPaintRoots(stack []*d2scene.Node, fill d2scene.Paint, stroke *d2scene.Stroke) []*d2scene.Node {
	appendRoot := func(paint d2scene.Paint) {
		switch pattern := paint.(type) {
		case d2scene.PatternPaint:
			stack = append(stack, pattern.Root)
		case *d2scene.PatternPaint:
			if pattern != nil {
				stack = append(stack, pattern.Root)
			}
		}
	}
	appendRoot(fill)
	if stroke != nil {
		appendRoot(stroke.Paint)
	}
	return stack
}

func hashFallbackFont(ctx context.Context, resource fontfallback.Font) (d2scene.AssetID, error) {
	return fontfallback.AssetID(ctx, resource)
}

func fontRuneIsDefaultIgnorable(value rune) bool {
	return fontface.IsDefaultIgnorableRune(value)
}

func appendUniqueAssetID(values []d2scene.AssetID, value d2scene.AssetID) []d2scene.AssetID {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func sortedRunes(values map[rune]struct{}) []rune {
	result := make([]rune, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func firstMissingRune(values map[rune]struct{}) rune {
	return sortedRunes(values)[0]
}

func safeFallbackName(value string) string {
	value = strings.ToValidUTF8(value, "?")
	value = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return '?'
		}
		return r
	}, value)
	runes := []rune(value)
	if len(runes) > 80 {
		value = string(runes[:80])
	}
	if value == "" {
		return "unnamed font"
	}
	return value
}
