package d2scenebuild

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"math"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/textmeasure"
)

const (
	lightCodeStyle             = "github"
	darkCodeStyle              = "catppuccin-mocha"
	maxCodeSourceBytes         = 1 << 20
	maxCodeSourceRunes         = 100_000
	maxCodeRawTokens           = 100_000
	maxCodeExpandedTokenPieces = 100_000
)

type codeTokenBudget struct {
	maxRawTokens      int
	maxExpandedPieces int
	rawTokens         int
	expandedPieces    int
}

func newCodeTokenBudget(maxNodes int) codeTokenBudget {
	rawLimit := maxCodeRawTokens
	expandedLimit := maxCodeExpandedTokenPieces
	if maxNodes > 0 {
		rawLimit = min(rawLimit, maxNodes)
		expandedLimit = min(expandedLimit, maxNodes)
	}
	return codeTokenBudget{maxRawTokens: rawLimit, maxExpandedPieces: expandedLimit}
}

// buildCodeShape translates a syntax-highlighted code shape into one
// active-theme scene without hidden duplicate theme content.
func (b *builder) buildCodeShape(targetShape d2target.Shape, stroke *d2scene.Stroke) ([]*d2scene.Node, error) {
	if err := validateCodeShape(targetShape); err != nil {
		return nil, err
	}
	// drawShape guards every label branch with Label != "". ShapeCode's
	// ordinary outline branch is empty, so a label-free code target paints
	// nothing even when it carries a language.
	if targetShape.Label == "" {
		return nil, nil
	}
	// A code target without a language has no outline or syntax highlighting.
	if targetShape.Language == "" {
		return b.buildShapeText(targetShape)
	}

	object := fmt.Sprintf("shape %q", targetShape.ID)

	codeStyle, err := b.activeCodeStyle(object)
	if err != nil {
		return nil, err
	}
	backgroundColor := codeStyle.Get(chroma.Background).Background
	if !backgroundColor.IsSet() {
		return nil, fmt.Errorf("scene: %s code snippet style has no background", object)
	}

	box := d2scene.Box{
		X: float64(targetShape.Pos.X), Y: float64(targetShape.Pos.Y),
		Width: float64(targetShape.Width), Height: float64(targetShape.Height),
	}
	// Code-shape backgrounds intentionally omit stroke dashes.
	codeStroke := cloneCodeStrokeWithoutDashes(stroke)
	background := d2scene.NewNode(d2scene.Rect{
		Box: box, Fill: codeColourPaint(backgroundColor), Stroke: codeStroke,
	})
	background.ID = targetShape.ID + ":code-background"
	nodes := []*d2scene.Node{background}

	padding := float64(targetShape.FontSize) / 2
	runs, err := b.buildCodeTextRuns(
		object, targetShape.ID+":code", targetShape.Text, codeStyle,
		d2scene.Point{X: box.X + padding, Y: box.Y + padding},
	)
	if err != nil {
		return nil, err
	}
	nodes = append(nodes, runs...)
	return nodes, nil
}

func (b *builder) activeCodeStyle(object string) (*chroma.Style, error) {
	styleName := lightCodeStyle
	if b.theme.IsDark() {
		styleName = darkCodeStyle
	}
	codeStyle := styles.Get(styleName)
	if codeStyle == nil {
		return nil, fmt.Errorf("scene: %s code snippet style %q not found", object, styleName)
	}
	return codeStyle, nil
}

func (b *builder) buildCodeTextRuns(object, idPrefix string, targetText d2target.Text, codeStyle *chroma.Style, origin d2scene.Point) ([]*d2scene.Node, error) {
	// Build initializes this eagerly. Keep direct package-internal builders used
	// by focused shape tests subject to the same default limits.
	if b.codeTokens.maxRawTokens == 0 && b.codeTokens.maxExpandedPieces == 0 {
		b.codeTokens = newCodeTokenBudget(b.options.MaxNodes)
	}
	lexer := lexers.Get(targetText.Language)
	if lexer == nil {
		lexer = lexers.Fallback
	}
	iterator, err := tokeniseCode(lexer, targetText.Label)
	if err != nil {
		return nil, fmt.Errorf("scene: %s tokenize %s code: %w", object, targetText.Language, err)
	}
	tokens, err := collectCodeTokens(b.ctx, iterator, &b.codeTokens)
	if err != nil {
		return nil, fmt.Errorf("scene: %s tokenize %s code: %w", object, targetText.Language, err)
	}
	lines := chroma.SplitTokensIntoLines(tokens)
	metrics := make(map[d2scene.AssetID]*codeFontMetrics, 3)
	lineAdvance := float64(targetText.FontSize) * textmeasure.CODE_LINE_HEIGHT
	var nodes []*d2scene.Node
	for lineIndex, tokens := range lines {
		if err := b.ctx.Err(); err != nil {
			return nil, err
		}
		lineTop := origin.Y + float64(lineIndex)*lineAdvance
		baseline := lineTop + float64(targetText.FontSize)
		cursor := origin.X
		for tokenIndex, token := range tokens {
			if err := b.ctx.Err(); err != nil {
				return nil, err
			}
			tokenText := codeTokenText(token.Value)
			if tokenText == "" {
				continue
			}
			entry := codeStyle.Get(token.Type)
			codeFont, err := b.codeTokenFont(targetText, entry)
			if err != nil {
				return nil, fmt.Errorf("scene: %s code token %d:%d: %w", object, lineIndex, tokenIndex, err)
			}
			fontMetrics, ok := metrics[codeFont.Asset]
			if !ok {
				fontMetrics, err = b.newCodeFontMetrics(codeFont.Asset)
				if err != nil {
					return nil, fmt.Errorf("scene: %s code token %d:%d: %w", object, lineIndex, tokenIndex, err)
				}
				metrics[codeFont.Asset] = fontMetrics
			}
			advance, err := fontMetrics.advance(b.ctx, tokenText, codeFont.Size)
			if err != nil {
				return nil, fmt.Errorf("scene: %s code token %d:%d: %w", object, lineIndex, tokenIndex, err)
			}

			// Whitespace advances the line cursor without painting ink. Avoid empty
			// raster work while retaining exact positions for following runs.
			if codeTokenPaintsInk(tokenText, entry) {
				run := d2scene.TextRun{
					Text:   tokenText,
					Origin: d2scene.Point{X: cursor, Y: baseline},
					Anchor: d2scene.AnchorStart,
					Font:   codeFont,
					Fill:   codeForegroundPaint(entry.Colour),
					// Chroma's SVG formatter represents underlining on the token;
					// background and border token styles are intentionally ignored.
					Underline: entry.Underline == chroma.Yes,
					Ink:       d2scene.NewBounds(cursor, lineTop, cursor+advance, lineTop+lineAdvance),
				}
				node := d2scene.NewNode(run)
				node.ID = fmt.Sprintf("%s-line:%d:token:%d", idPrefix, lineIndex, tokenIndex)
				nodes = append(nodes, node)
			}
			cursor += advance
		}
	}
	return nodes, nil
}

func validateCodeShape(targetShape d2target.Shape) error {
	return validateCodeText(fmt.Sprintf("shape %q", targetShape.ID), targetShape.Text)
}

func validateCodeText(object string, text d2target.Text) error {
	if text.Label == "" || text.Language == "" {
		return nil
	}
	if !utf8.ValidString(text.Label) {
		return invalidField(object, "label", nil, "must be valid UTF-8")
	}
	if len(text.Label) > maxCodeSourceBytes {
		return invalidField(object, "label", len(text.Label), fmt.Sprintf("code source bytes must not exceed %d", maxCodeSourceBytes))
	}
	runes := utf8.RuneCountInString(text.Label)
	if runes > maxCodeSourceRunes {
		return invalidField(object, "label", runes, fmt.Sprintf("code source runes must not exceed %d", maxCodeSourceRunes))
	}
	if text.FontSize <= 0 {
		return invalidField(object, "fontSize", text.FontSize, "must be positive for a syntax-highlighted label")
	}
	if text.FontSize > math.MaxInt32/64 {
		return invalidField(object, "fontSize", text.FontSize, "must fit raster font scaling")
	}
	return nil
}

func cloneCodeStrokeWithoutDashes(stroke *d2scene.Stroke) *d2scene.Stroke {
	if stroke == nil {
		return nil
	}
	clone := *stroke
	clone.Dashes = nil
	clone.DashOffset = 0
	return &clone
}

func codeTokenText(value string) string {
	value = strings.TrimSuffix(value, "\n")
	value = strings.TrimSuffix(value, "\r")
	return strings.ReplaceAll(value, "\t", "    ")
}

func tokeniseCode(lexer chroma.Lexer, value string) (iterator chroma.Iterator, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			iterator = nil
			err = fmt.Errorf("lexer panic: %v", recovered)
		}
	}()
	return lexer.Tokenise(nil, value)
}

func collectCodeTokens(ctx context.Context, iterator chroma.Iterator, budget *codeTokenBudget) (tokens []chroma.Token, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			tokens = nil
			err = fmt.Errorf("lexer panic: %v", recovered)
		}
	}()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		token := iterator()
		if token == chroma.EOF {
			break
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if budget.rawTokens >= budget.maxRawTokens {
			return nil, fmt.Errorf("raw lexer token count exceeds limit %d", budget.maxRawTokens)
		}
		pieces, err := countCodeTokenPieces(
			ctx, token.Value,
			budget.maxExpandedPieces-budget.expandedPieces,
			budget.maxExpandedPieces,
		)
		if err != nil {
			return nil, err
		}
		budget.rawTokens++
		budget.expandedPieces += pieces
		tokens = append(tokens, token)
	}
	return tokens, nil
}

// countCodeTokenPieces accounts for every token that SplitTokensIntoLines will
// materialize before that helper is allowed to allocate them. It checks the
// context while scanning because a lexer may return one large token.
func countCodeTokenPieces(ctx context.Context, value string, remaining, limit int) (int, error) {
	pieces := 1
	if remaining < pieces {
		return 0, fmt.Errorf("newline-expanded token piece count exceeds limit %d", limit)
	}
	for i := 0; i < len(value); i++ {
		if i&4095 == 0 {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
		}
		if value[i] != '\n' {
			continue
		}
		pieces++
		if pieces > remaining {
			return 0, fmt.Errorf("newline-expanded token piece count exceeds limit %d", limit)
		}
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return pieces, nil
}

func codeWhitespaceOnly(value string) bool {
	for _, r := range value {
		if !unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

func codeTokenPaintsInk(value string, entry chroma.StyleEntry) bool {
	// Whitespace contributes no visible text pixels, but an underlined whitespace
	// token still paints its decoration across the token advance.
	return !codeWhitespaceOnly(value) || entry.Underline == chroma.Yes
}

func codeColourPaint(value chroma.Colour) d2scene.Paint {
	return d2scene.SolidPaint{Color: color.NRGBA{R: value.Red(), G: value.Green(), B: value.Blue(), A: 0xff}}
}

func codeForegroundPaint(value chroma.Colour) d2scene.Paint {
	// An absent SVG fill inherits the SVG initial fill, black. This matters for
	// fallback lexers under the light GitHub style.
	if !value.IsSet() {
		return d2scene.SolidPaint{Color: color.NRGBA{A: 0xff}}
	}
	return codeColourPaint(value)
}

func (b *builder) codeTokenFont(text d2target.Text, entry chroma.StyleEntry) (d2scene.Font, error) {
	text.FontFamily = "mono"
	// The SVG stylesheet declares the italic mono class after the bold mono
	// class. When Chroma requests both, italic therefore wins its font-family.
	text.Italic = entry.Italic == chroma.Yes
	text.Bold = entry.Bold == chroma.Yes && !text.Italic
	return b.font(text)
}

type codeFontMetrics struct {
	font *sfnt.Font
}

func (b *builder) newCodeFontMetrics(assetID d2scene.AssetID) (*codeFontMetrics, error) {
	asset, ok := b.assets[assetID].(d2scene.FontAsset)
	if !ok {
		return nil, fmt.Errorf("font asset %q is missing or not a font", assetID)
	}
	collection, err := opentype.ParseCollection(asset.Data)
	if err != nil {
		return nil, fmt.Errorf("parse font asset %q: %w", assetID, err)
	}
	parsed, err := collection.Font(0)
	if err != nil {
		return nil, fmt.Errorf("load first font in asset %q: %w", assetID, err)
	}
	return &codeFontMetrics{font: parsed}, nil
}

func (m *codeFontMetrics) advance(ctx context.Context, value string, size float64) (float64, error) {
	if !utf8.ValidString(value) {
		return 0, fmt.Errorf("text is not valid UTF-8")
	}
	ppem := fixed.Int26_6(math.Round(size * 64))
	var buffer sfnt.Buffer
	var previous sfnt.GlyphIndex
	havePrevious := false
	advance := 0.0
	for byteOffset, r := range value {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		glyph, err := m.font.GlyphIndex(&buffer, r)
		if err != nil {
			return 0, fmt.Errorf("look up glyph for U+%04X at byte %d: %w", r, byteOffset, err)
		}
		if glyph == 0 {
			return 0, fmt.Errorf("missing glyph U+%04X at byte %d", r, byteOffset)
		}
		if havePrevious {
			kern, err := m.font.Kern(&buffer, previous, glyph, ppem, font.HintingNone)
			if err != nil && !errors.Is(err, sfnt.ErrNotFound) {
				return 0, fmt.Errorf("kern glyph for U+%04X at byte %d: %w", r, byteOffset, err)
			}
			if err == nil {
				advance += float64(kern) / 64
			}
		}
		glyphAdvance, err := m.font.GlyphAdvance(&buffer, glyph, ppem, font.HintingNone)
		if err != nil {
			return 0, fmt.Errorf("measure glyph for U+%04X at byte %d: %w", r, byteOffset, err)
		}
		advance += float64(glyphAdvance) / 64
		previous, havePrevious = glyph, true
	}
	return advance, nil
}
