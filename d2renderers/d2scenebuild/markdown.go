package d2scenebuild

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2target"
	libcolor "github.com/d2lang/d2/lib/color"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
	"github.com/d2lang/d2/lib/textmeasure"
)

const (
	// Markdown is already bounded by the compiler's source input in ordinary
	// CLI use, but scene construction is also a public API. These ceilings keep
	// a single rich label from turning into unbounded Goldmark/layout work or an
	// arbitrarily large retained scene.
	maxMarkdownSourceBytes = 1 << 20
	maxMarkdownPrimitives  = 100_000
)

func validateMarkdownLabel(object, source string, fontSize, width, height int) error {
	if !utf8.ValidString(source) {
		return invalidField(object, "label", nil, "must be valid UTF-8")
	}
	if len(source) > maxMarkdownSourceBytes {
		return invalidField(object, "label", len(source), fmt.Sprintf("Markdown source bytes must not exceed %d", maxMarkdownSourceBytes))
	}
	if width > 0 && height > 0 && fontSize <= 0 {
		return invalidField(object, "fontSize", fontSize, "must be positive for a Markdown label")
	}
	return nil
}

func (b *builder) buildShapeMarkdown(targetShape d2target.Shape) ([]*d2scene.Node, error) {
	topLeft := markdownShapeLabelTopLeft(targetShape)
	if topLeft == nil {
		return nil, invalidField(fmt.Sprintf("shape %q", targetShape.ID), "labelPosition", targetShape.LabelPosition, "does not resolve to a finite Markdown viewport")
	}
	node, err := b.buildMarkdownLabel(
		fmt.Sprintf("shape %q", targetShape.ID), targetShape.ID+":markdown",
		targetShape.Label, targetShape.FontFamily, targetShape.FontSize,
		targetShape.LabelWidth, targetShape.LabelHeight,
		targetShape.GetFontColor(), targetShape.Fill,
		targetShape.Link != "", targetShape.Underline, topLeft,
	)
	if err != nil {
		return nil, err
	}
	return []*d2scene.Node{node}, nil
}

func (b *builder) buildConnectionMarkdown(connection d2target.Connection, topLeft *geo.Point) (*d2scene.Node, error) {
	return b.buildMarkdownLabel(
		fmt.Sprintf("connection %q", connection.ID), connection.ID+":markdown",
		connection.Label, connection.FontFamily, connection.FontSize,
		connection.LabelWidth, connection.LabelHeight,
		connection.GetFontColor(), connection.Fill,
		connection.Link != "", connection.Underline, topLeft,
	)
}

func markdownShapeLabelTopLeft(targetShape d2target.Shape) *geo.Point {
	position := label.FromString(targetShape.LabelPosition)
	if position == label.Unset {
		position = label.InsideMiddleCenter
	}
	geometry := targetGeometry(targetShape)
	var box *geo.Box
	if position.IsOutside() || position.IsBorder() {
		box = geometry.GetBox().Copy()
		if targetShape.ThreeDee {
			offsetY := d2target.THREE_DEE_OFFSET
			if targetShape.Type == d2target.ShapeHexagon {
				offsetY /= 2
			}
			box.TopLeft.Y -= float64(offsetY)
			box.Height += float64(offsetY)
			box.Width += float64(d2target.THREE_DEE_OFFSET)
		} else if targetShape.Multiple {
			box.TopLeft.Y -= float64(d2target.MULTIPLE_OFFSET)
			box.Height += float64(d2target.MULTIPLE_OFFSET)
			box.Width += float64(d2target.MULTIPLE_OFFSET)
		}
	} else {
		box = geometry.GetInnerBox()
	}
	return position.GetPointOnBox(box, label.PADDING, float64(targetShape.LabelWidth), float64(targetShape.LabelHeight))
}

func (b *builder) buildMarkdownLabel(
	object, id, source, fontName string,
	fontSize, width, height int,
	foreground, background string,
	disableLinks, underline bool,
	topLeft *geo.Point,
) (*d2scene.Node, error) {
	if err := b.ctx.Err(); err != nil {
		return nil, err
	}
	root := d2scene.NewNode(nil)
	root.ID = id
	root.Transform = d2scene.Translate(topLeft.X, topLeft.Y)
	primary, mono := b.markdownFontFamilies(fontName)
	ruler, err := b.markdownLayoutRuler()
	if err != nil {
		return nil, fmt.Errorf("scene: %s Markdown ruler: %w", object, err)
	}
	layoutFontSize := fontSize
	if layoutFontSize <= 0 {
		// Zero-sized empty Markdown labels may carry a zero font size in frozen
		// targets. Still parse and validate their source with D2's
		// normal body size before discarding their non-painting viewport.
		layoutFontSize = d2fonts.FONT_SIZE_M
	}
	layout, err := textmeasure.LayoutMarkdown(source, ruler, &primary, &mono, layoutFontSize)
	if err != nil {
		return nil, fmt.Errorf("scene: %s Markdown layout: %w", object, err)
	}
	if len(layout.Primitives) > maxMarkdownPrimitives {
		return nil, fmt.Errorf("scene: %s Markdown primitive count %d exceeds limit %d", object, len(layout.Primitives), maxMarkdownPrimitives)
	}
	if err := b.ctx.Err(); err != nil {
		return nil, err
	}
	// A zero-sized Markdown viewport cannot paint or expose interactive hit
	// regions. Represent it as an empty group without a degenerate clip.
	if width == 0 || height == 0 {
		return root, nil
	}

	paints, err := b.markdownRolePaints(foreground)
	if err != nil {
		return nil, fmt.Errorf("scene: %s Markdown paints: %w", object, err)
	}
	viewport := d2scene.Box{Width: float64(width), Height: float64(height)}
	root.Clip = boxClip(viewport)

	if background != "" && !strings.EqualFold(background, libcolor.None) && !strings.EqualFold(background, "transparent") {
		fill, err := b.paint(background, object+" Markdown background")
		if err != nil {
			return nil, err
		}
		if fill != nil {
			backgroundNode := d2scene.NewNode(d2scene.Rect{Box: viewport, Fill: fill})
			backgroundNode.ID = id + ":background"
			root.Children = append(root.Children, backgroundNode)
		}
	}

	for index, primitive := range layout.Primitives {
		if index&255 == 0 {
			if err := b.ctx.Err(); err != nil {
				return nil, err
			}
		}
		node, linkBox, err := b.buildMarkdownPrimitive(id, index, primitive, primary, mono, paints, underline)
		if err != nil {
			return nil, fmt.Errorf("scene: %s Markdown primitive %d: %w", object, index, err)
		}
		if node != nil {
			root.Children = append(root.Children, node)
		}
		link := textmeasure.SafeMarkdownLink(primitive.Link)
		if disableLinks || link == "" || !linkBox.Valid {
			continue
		}
		linkBox = intersectMarkdownBounds(linkBox, viewport.Bounds())
		if !linkBox.Valid {
			continue
		}
		if err := b.addMarkdownLinkRegion(object, index, link, primitive.LinkTitle, linkBox.Translate(d2scene.Point{X: topLeft.X, Y: topLeft.Y}).Box()); err != nil {
			return nil, err
		}
	}
	return root, nil
}

func (b *builder) markdownLayoutRuler() (*textmeasure.Ruler, error) {
	if b.markdownRuler == nil && b.markdownRulerErr == nil {
		b.markdownRuler, b.markdownRulerErr = textmeasure.NewRuler()
	}
	return b.markdownRuler, b.markdownRulerErr
}

func (b *builder) markdownFontFamilies(fontName string) (d2fonts.FontFamily, d2fonts.FontFamily) {
	primary := d2fonts.SourceSansPro
	if b.diagram.FontFamily != nil {
		primary = *b.diagram.FontFamily
	}
	mono := d2fonts.SourceCodePro
	if b.diagram.MonoFontFamily != nil {
		mono = *b.diagram.MonoFontFamily
	}
	if strings.EqualFold(fontName, "mono") {
		primary = mono
	} else if fontName != "" && !strings.EqualFold(fontName, "default") {
		if family, ok := d2fonts.D2_FONT_TO_FAMILY[strings.ToLower(fontName)]; ok {
			primary = family
		}
	}
	return primary, mono
}

func (b *builder) markdownRolePaints(foreground string) (map[textmeasure.MarkdownColorRole]d2scene.Paint, error) {
	values := map[textmeasure.MarkdownColorRole]string{
		textmeasure.MarkdownColorForeground:       foreground,
		textmeasure.MarkdownColorForegroundStroke: foreground,
		textmeasure.MarkdownColorMuted:            libcolor.N2,
		textmeasure.MarkdownColorMutedStroke:      libcolor.N2,
		textmeasure.MarkdownColorAccent:           libcolor.B2,
		textmeasure.MarkdownColorBorder:           libcolor.B1,
		textmeasure.MarkdownColorBorderMuted:      libcolor.B2,
		textmeasure.MarkdownColorCanvas:           libcolor.N7,
		textmeasure.MarkdownColorCanvasSubtle:     libcolor.N6,
		textmeasure.MarkdownColorNeutralMuted:     libcolor.N6,
	}
	paints := make(map[textmeasure.MarkdownColorRole]d2scene.Paint, len(values))
	for role, value := range values {
		paint, err := b.paint(value, "Markdown "+string(role))
		if err != nil {
			return nil, err
		}
		paints[role] = paint
	}
	return paints, nil
}

func (b *builder) buildMarkdownPrimitive(
	id string,
	index int,
	primitive textmeasure.MarkdownPrimitive,
	primary, mono d2fonts.FontFamily,
	paints map[textmeasure.MarkdownColorRole]d2scene.Paint,
	underline bool,
) (*d2scene.Node, d2scene.Bounds, error) {
	fill := paints[primitive.FillRole]
	strokePaint := paints[primitive.StrokeRole]
	nodeID := fmt.Sprintf("%s:primitive:%d", id, index)
	switch primitive.Kind {
	case textmeasure.MarkdownRectPrimitive:
		box := d2scene.Box{X: primitive.X, Y: primitive.Y, Width: primitive.Width, Height: primitive.Height}
		var stroke *d2scene.Stroke
		if strokePaint != nil && primitive.StrokeWidth > 0 {
			stroke = &d2scene.Stroke{Paint: strokePaint, Width: primitive.StrokeWidth, Cap: d2scene.CapButt, Join: d2scene.JoinMiter, MiterLimit: 4}
		}
		node := d2scene.NewNode(d2scene.Rect{
			Box: box, RadiusX: primitive.Radius, RadiusY: primitive.Radius,
			Fill: fill, Stroke: stroke,
		})
		node.ID = nodeID
		return node, box.Bounds(), nil
	case textmeasure.MarkdownLinePrimitive:
		if strokePaint == nil || primitive.StrokeWidth <= 0 {
			return nil, d2scene.Bounds{}, nil
		}
		stroke := &d2scene.Stroke{Paint: strokePaint, Width: primitive.StrokeWidth, Cap: d2scene.CapButt, Join: d2scene.JoinMiter, MiterLimit: 4}
		path := d2scene.Path{Commands: []d2scene.PathCommand{
			d2scene.MoveTo(primitive.X, primitive.Y),
			d2scene.LineTo(primitive.X2, primitive.Y2),
		}, Stroke: stroke}
		node := d2scene.NewNode(path)
		node.ID = nodeID
		bounds := d2scene.BoundsFromPoints(
			d2scene.Point{X: primitive.X, Y: primitive.Y},
			d2scene.Point{X: primitive.X2, Y: primitive.Y2},
		).Expand(primitive.StrokeWidth/2, primitive.StrokeWidth/2)
		return node, bounds, nil
	case textmeasure.MarkdownTextPrimitive:
		fontValue, err := b.markdownFont(primitive.Font, primary, mono, primitive.FontSize)
		if err != nil {
			return nil, d2scene.Bounds{}, err
		}
		width := primitive.Width
		if width <= 0 {
			width = markdownEstimatedAdvance(primitive.Text, primitive.FontSize)
		}
		height := primitive.Height
		if height <= 0 {
			height = primitive.FontSize
		}
		ink := d2scene.NewBounds(
			primitive.X, primitive.Y-height,
			primitive.X+width, primitive.Y+primitive.FontSize*.25,
		)
		var textStroke *d2scene.Stroke
		if primitive.SyntheticBold && fill != nil {
			textStroke = &d2scene.Stroke{
				Paint: fill, Width: math.Max(.35, primitive.FontSize/32),
				Cap: d2scene.CapRound, Join: d2scene.JoinRound, MiterLimit: 4,
			}
		}
		run := d2scene.TextRun{
			Text: primitive.Text, Origin: d2scene.Point{X: primitive.X, Y: primitive.Y},
			Anchor: d2scene.AnchorStart, Font: fontValue, Fill: fill, Stroke: textStroke,
			Underline: underline, Strike: primitive.Decoration == textmeasure.MarkdownTextDecorationLineThrough,
			Ink: ink,
		}
		node := d2scene.NewNode(run)
		node.ID = nodeID
		transform := d2scene.Identity()
		if primitive.TextLength && primitive.Width > 0 {
			face, err := b.fontFace(fontValue.Asset)
			if err != nil {
				return nil, d2scene.Bounds{}, fmt.Errorf("measure textLength: %w", err)
			}
			advance, err := markdownFontAdvance(b.ctx, face.Outline, primitive.Text, primitive.FontSize)
			if err != nil {
				return nil, d2scene.Bounds{}, fmt.Errorf("measure textLength: %w", err)
			}
			if advance <= 0 {
				return nil, d2scene.Bounds{}, fmt.Errorf("textLength source has zero advance")
			}
			scaleX := primitive.Width / advance
			if !finite(scaleX) || scaleX <= 0 {
				return nil, d2scene.Bounds{}, fmt.Errorf("textLength produces invalid horizontal scale %v", scaleX)
			}
			transform = d2scene.Translate(primitive.X, primitive.Y).
				Mul(d2scene.Scale(scaleX, 1)).
				Mul(d2scene.Translate(-primitive.X, -primitive.Y))
		}
		if primitive.SyntheticItalic {
			italic := d2scene.Translate(primitive.X, primitive.Y).
				Mul(d2scene.SkewX(-12 * math.Pi / 180)).
				Mul(d2scene.Translate(-primitive.X, -primitive.Y))
			transform = italic.Mul(transform)
		}
		node.Transform = transform
		return node, ink.Transform(transform), nil
	default:
		return nil, d2scene.Bounds{}, fmt.Errorf("unknown primitive kind %q", primitive.Kind)
	}
}

func (b *builder) markdownFont(role textmeasure.MarkdownFontRole, primary, mono d2fonts.FontFamily, size float64) (d2scene.Font, error) {
	family := primary
	style := d2fonts.FONT_STYLE_REGULAR
	switch role {
	case textmeasure.MarkdownFontRegular:
	case textmeasure.MarkdownFontSemibold:
		style = d2fonts.FONT_STYLE_SEMIBOLD
	case textmeasure.MarkdownFontBold:
		style = d2fonts.FONT_STYLE_BOLD
	case textmeasure.MarkdownFontItalic:
		style = d2fonts.FONT_STYLE_ITALIC
	case textmeasure.MarkdownFontMono:
		family = mono
	case textmeasure.MarkdownFontMonoSemibold:
		family, style = mono, d2fonts.FONT_STYLE_SEMIBOLD
	case textmeasure.MarkdownFontMonoBold:
		family, style = mono, d2fonts.FONT_STYLE_BOLD
	case textmeasure.MarkdownFontMonoItalic:
		family, style = mono, d2fonts.FONT_STYLE_ITALIC
	default:
		return d2scene.Font{}, fmt.Errorf("unknown font role %q", role)
	}
	fontSpec := d2fonts.Font{Family: family, Style: style}
	fontBytes, ok := d2fonts.FontFaces.Lookup(fontSpec)
	if !ok || len(fontBytes) == 0 {
		return d2scene.Font{}, fmt.Errorf("font %s/%s is not loaded", family, style)
	}
	assetID := d2scene.AssetID("font:" + string(family) + ":" + string(style))
	if _, exists := b.assets[assetID]; !exists {
		b.assets[assetID] = d2scene.FontAsset{MIMEType: "font/ttf", Data: retainedFontBytes(fontBytes)}
	}
	weight := 400
	if style == d2fonts.FONT_STYLE_BOLD {
		weight = 700
	} else if style == d2fonts.FONT_STYLE_SEMIBOLD {
		weight = 600
	}
	return d2scene.Font{
		Family: string(family), Style: string(style), Weight: weight,
		Size: size, Asset: assetID,
	}, nil
}

func markdownFontAdvance(ctx interface{ Err() error }, parsed *sfnt.Font, text string, size float64) (float64, error) {
	if parsed == nil {
		return 0, fmt.Errorf("nil font")
	}
	ppem := fixed.Int26_6(math.Round(size * 64))
	var buffer sfnt.Buffer
	advance := fixed.Int26_6(0)
	var previous sfnt.GlyphIndex
	havePrevious := false
	for _, value := range text {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		glyph, err := parsed.GlyphIndex(&buffer, value)
		if err != nil {
			return 0, fmt.Errorf("look up U+%04X: %w", value, err)
		}
		if glyph == 0 {
			return 0, fmt.Errorf("missing glyph U+%04X", value)
		}
		if havePrevious {
			kern, err := parsed.Kern(&buffer, previous, glyph, ppem, font.HintingNone)
			if err != nil && !errors.Is(err, sfnt.ErrNotFound) {
				return 0, fmt.Errorf("kern U+%04X: %w", value, err)
			}
			if err == nil {
				advance += kern
			}
		}
		glyphAdvance, err := parsed.GlyphAdvance(&buffer, glyph, ppem, font.HintingNone)
		if err != nil {
			return 0, fmt.Errorf("advance U+%04X: %w", value, err)
		}
		advance += glyphAdvance
		previous, havePrevious = glyph, true
	}
	return float64(advance) / 64, nil
}

func markdownEstimatedAdvance(text string, fontSize float64) float64 {
	count := utf8.RuneCountInString(text)
	if count == 0 {
		return math.Max(1, fontSize*.25)
	}
	return math.Max(1, float64(count)*fontSize*.6)
}

func (b *builder) addMarkdownLinkRegion(object string, primitiveIndex int, link, title string, box d2scene.Box) error {
	if b.options.LinkBudget.MaxRegions <= 0 || b.options.LinkBudget.MaxStringBytes <= 0 {
		return invalidField("options", "linkBudget", b.options.LinkBudget, "must provide positive MaxRegions and MaxStringBytes for Markdown links")
	}
	metadataObject := fmt.Sprintf("%s Markdown primitive %d", object, primitiveIndex)
	if err := b.validateLinkFields(metadataObject, link, title); err != nil {
		return err
	}
	startLinks, startBytes := len(b.links), b.linkBytes
	// Markdown destinations originate as URLs, including relative URLs such as
	// root.html. Only typed D2 object and connection links carry board-target
	// provenance and may be classified by linkDestination.
	if err := b.addLinkRegion(metadataObject, d2scene.LinkRegion{Box: box, URL: link, Tooltip: title}); err != nil {
		return err
	}
	if err := b.addAppendixItem(metadataObject, "linkTitle", title); err != nil {
		b.links = b.links[:startLinks]
		b.linkBytes = startBytes
		return err
	}
	return nil
}

func intersectMarkdownBounds(left, right d2scene.Bounds) d2scene.Bounds {
	if !left.Valid || !right.Valid {
		return d2scene.Bounds{}
	}
	minX := math.Max(left.Min.X, right.Min.X)
	minY := math.Max(left.Min.Y, right.Min.Y)
	maxX := math.Min(left.Max.X, right.Max.X)
	maxY := math.Min(left.Max.Y, right.Max.Y)
	if maxX <= minX || maxY <= minY {
		return d2scene.Bounds{}
	}
	return d2scene.NewBounds(minX, minY, maxX, maxY)
}
