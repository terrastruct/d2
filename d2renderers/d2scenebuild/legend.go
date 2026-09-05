package d2scenebuild

import (
	"fmt"
	"image/color"
	"math"
	"strings"
	"time"

	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/textmeasure"
)

const (
	legendPadding       = 20
	legendItemSpacing   = 15
	legendIconSize      = 24
	legendFontSize      = 14
	legendCornerPadding = 10

	legendShapeScaleFactor      = 5
	legendConnectionScaleFactor = 2
)

type legendItemDimensions struct {
	width  int
	height int
}

type legendLayout struct {
	x      int
	y      int
	width  int
	height int

	maxLabelWidth        int
	itemCount            int
	shapeDimensions      []legendItemDimensions
	connectionDimensions []legendItemDimensions
	titleDimensions      legendItemDimensions
}

func (b *builder) preflightLegend() error {
	legend := b.diagram.Legend
	if legend == nil || len(legend.Shapes) == 0 && len(legend.Connections) == 0 {
		return nil
	}
	for index, targetShape := range legend.Shapes {
		if err := b.ctx.Err(); err != nil {
			return err
		}
		if targetShape.Label == "" {
			continue
		}
		object := legendShapeObject(index, targetShape.ID)
		if targetShape.Link != "" || targetShape.PrettyLink != "" || targetShape.Tooltip != "" || targetShape.TooltipPosition != "" {
			return unsupported(object, "link/tooltip metadata on a legend icon")
		}
		if err := validateOpacity(object, targetShape.Opacity); err != nil {
			return err
		}
		icon := legendShapeIconTarget(index, targetShape)
		// Validate the geometry that buildLegendShapeIcon actually emits. The
		// outer node carries the source opacity, while its geometry is always
		// built at opacity one so structured and embedded-image work cannot hide
		// behind an opacity-zero preflight shortcut. A generic icon is omitted at
		// source opacity zero, matching drawShape. Label language is irrelevant
		// because the icon clone has no label.
		icon.Opacity = 1
		icon.Language = ""
		icon.Shadow = false
		icon.Blend = false
		icon.Animated = false
		embeddedIcon := icon.Type == d2target.ShapeImage || icon.Type == d2target.ShapeClass || icon.Type == d2target.ShapeSQLTable
		if !embeddedIcon && targetShape.Opacity == 0 {
			icon.Icon = nil
		}
		if err := b.preflightShape(icon); err != nil {
			return fmt.Errorf("scene: %s icon: %w", object, err)
		}
	}
	for index, connection := range legend.Connections {
		if err := b.ctx.Err(); err != nil {
			return err
		}
		if connection.Label == "" {
			continue
		}
		object := legendConnectionObject(index, connection.ID)
		if connection.Icon != nil {
			return unsupported(object, "connection icon asset (svg legend connections render only route and arrow styles)")
		}
		if (connection.SrcLabel != nil && connection.SrcLabel.Label != "") ||
			(connection.DstLabel != nil && connection.DstLabel.Label != "") {
			return unsupported(object, "endpoint label (svg legend connections render only route and arrow styles)")
		}
		if connection.Link != "" || connection.PrettyLink != "" || connection.Tooltip != "" {
			return unsupported(object, "link/tooltip metadata on a legend connection")
		}
		icon := legendConnectionIconTarget(index, connection)
		if err := b.preflightConnection(icon); err != nil {
			return fmt.Errorf("scene: %s icon: %w", object, err)
		}
	}
	return b.ctx.Err()
}

func legendShapeObject(index int, id string) string {
	return fmt.Sprintf("legend shape[%d] %q", index, id)
}

func legendConnectionObject(index int, id string) string {
	return fmt.Sprintf("legend connection[%d] %q", index, id)
}

func legendShapeIconTarget(index int, targetShape d2target.Shape) d2target.Shape {
	icon := targetShape
	icon.ID = fmt.Sprintf("legend:shape:%d:icon", index)
	icon.Pos = d2target.Point{}
	icon.Width = legendIconSize * legendShapeScaleFactor
	icon.Height = legendIconSize * legendShapeScaleFactor
	icon.Label = ""
	return icon
}

func legendConnectionIconTarget(index int, connection d2target.Connection) d2target.Connection {
	icon := *d2target.BaseConnection()
	icon.ID = fmt.Sprintf("legend:connection:%d:icon", index)
	icon.SrcArrow = connection.SrcArrow
	icon.DstArrow = connection.DstArrow
	icon.StrokeDash = connection.StrokeDash
	icon.StrokeWidth = connection.StrokeWidth
	icon.Stroke = connection.Stroke
	icon.Fill = connection.Fill
	icon.BorderRadius = connection.BorderRadius
	icon.Opacity = connection.Opacity
	icon.Animated = connection.Animated
	width := float64(legendIconSize * legendConnectionScaleFactor)
	icon.Route = []*geo.Point{{}, {X: width}}
	return icon
}

func (b *builder) measureLegend(tl, br d2target.Point) (*legendLayout, error) {
	legend := b.diagram.Legend
	if legend == nil || len(legend.Shapes) == 0 && len(legend.Connections) == 0 {
		return nil, nil
	}
	if err := b.ctx.Err(); err != nil {
		return nil, err
	}
	ruler, err := textmeasure.NewRuler()
	if err != nil {
		return nil, fmt.Errorf("scene: legend text ruler: %w", err)
	}
	if ruler == nil {
		return nil, fmt.Errorf("scene: legend text ruler is nil")
	}

	layout := &legendLayout{
		shapeDimensions:      make([]legendItemDimensions, len(legend.Shapes)),
		connectionDimensions: make([]legendItemDimensions, len(legend.Connections)),
	}
	totalHeight := int64(legendPadding + legendFontSize + legendItemSpacing)
	itemCount := 0
	measure := func(object, value, family string) (legendItemDimensions, error) {
		if err := b.ctx.Err(); err != nil {
			return legendItemDimensions{}, err
		}
		mtext := &d2target.MText{Text: value, FontSize: legendFontSize}
		dimensions := d2graph.GetTextDimensions(nil, ruler, mtext, legendFontToFamily(family))
		if dimensions == nil || dimensions.Width < 0 || dimensions.Height < 0 {
			return legendItemDimensions{}, fmt.Errorf("scene: %s has invalid measured text dimensions", object)
		}
		return legendItemDimensions{width: dimensions.Width, height: dimensions.Height}, nil
	}
	addItem := func(object string, dimensions legendItemDimensions) error {
		rowHeight := maxInt(dimensions.height, legendIconSize)
		increment := int64(rowHeight + legendItemSpacing)
		maxPlatform := int64(^uint(0) >> 1)
		if totalHeight > maxPlatform-increment {
			return fmt.Errorf("scene: %s causes legend height to exceed the platform integer domain", object)
		}
		totalHeight += increment
		layout.maxLabelWidth = maxInt(layout.maxLabelWidth, dimensions.width)
		itemCount++
		return nil
	}
	for index, targetShape := range legend.Shapes {
		if err := b.ctx.Err(); err != nil {
			return nil, err
		}
		if targetShape.Label == "" {
			continue
		}
		dimensions, err := measure(legendShapeObject(index, targetShape.ID), targetShape.Label, targetShape.FontFamily)
		if err != nil {
			return nil, err
		}
		layout.shapeDimensions[index] = dimensions
		if err := addItem(legendShapeObject(index, targetShape.ID), dimensions); err != nil {
			return nil, err
		}
	}
	for index, connection := range legend.Connections {
		if err := b.ctx.Err(); err != nil {
			return nil, err
		}
		if connection.Label == "" {
			continue
		}
		dimensions, err := measure(legendConnectionObject(index, connection.ID), connection.Label, connection.FontFamily)
		if err != nil {
			return nil, err
		}
		layout.connectionDimensions[index] = dimensions
		if err := addItem(legendConnectionObject(index, connection.ID), dimensions); err != nil {
			return nil, err
		}
	}
	if itemCount > 0 {
		totalHeight -= legendItemSpacing / 2
	}
	if itemCount > 0 && len(legend.Connections) > 0 {
		totalHeight += legendPadding * 3 / 2
	} else {
		totalHeight += legendPadding * 6 / 5
	}
	maxPlatform := int64(^uint(0) >> 1)
	if totalHeight <= 0 || totalHeight > maxPlatform {
		return nil, fmt.Errorf("scene: legend height exceeds the platform integer domain")
	}
	legendWidth := int64(legendPadding*2 + legendIconSize + legendPadding)
	if int64(layout.maxLabelWidth) > maxPlatform-legendWidth {
		return nil, fmt.Errorf("scene: legend width exceeds the platform integer domain")
	}
	legendWidth += int64(layout.maxLabelWidth)
	legendX, ok := checkedAdd(int64(br.X), legendCornerPadding, -maxPlatform-1, maxPlatform)
	if !ok {
		return nil, fmt.Errorf("scene: legend x position exceeds the platform integer domain")
	}
	legendY, ok := checkedSub(int64(br.Y), totalHeight, -maxPlatform-1, maxPlatform)
	if !ok {
		return nil, fmt.Errorf("scene: legend y position exceeds the platform integer domain")
	}
	if legendY < int64(tl.Y) {
		legendY = int64(tl.Y)
	}
	layout.x, layout.y = int(legendX), int(legendY)
	layout.width, layout.height = int(legendWidth), int(totalHeight)
	for _, operation := range []struct {
		name  string
		value int64
		delta int64
	}{
		{name: "right edge", value: legendX, delta: legendWidth},
		{name: "bottom edge", value: legendY, delta: totalHeight},
		{name: "title x position", value: legendX, delta: legendPadding},
		{name: "title y position", value: legendY, delta: legendPadding + legendFontSize},
		{name: "item y position", value: legendY, delta: legendPadding*2 + legendFontSize},
	} {
		if _, ok := checkedAdd(operation.value, operation.delta, -maxPlatform-1, maxPlatform); !ok {
			return nil, fmt.Errorf("scene: legend %s exceeds the platform integer domain", operation.name)
		}
	}

	title := legend.Label
	if title == "" {
		title = "Legend"
	}
	titleFamily := b.diagram.FontFamily
	titleText := &d2target.MText{Text: title, FontSize: legendFontSize + 2, IsBold: true}
	titleDimensions := d2graph.GetTextDimensions(nil, ruler, titleText, titleFamily)
	if titleDimensions == nil || titleDimensions.Width < 0 || titleDimensions.Height < 0 {
		return nil, fmt.Errorf("scene: legend title has invalid measured text dimensions")
	}
	layout.titleDimensions = legendItemDimensions{width: titleDimensions.Width, height: titleDimensions.Height}
	layout.itemCount = itemCount
	return layout, b.ctx.Err()
}

func legendFontToFamily(fontFamily string) *d2fonts.FontFamily {
	family, ok := d2fonts.D2_FONT_TO_FAMILY[fontFamily]
	if !ok {
		return nil
	}
	return &family
}

func (layout *legendLayout) expandViewBox(left, top, width, height int, pad int64) (int, int, int, int, error) {
	if layout == nil || layout.maxLabelWidth <= 0 {
		return left, top, width, height, nil
	}
	maxPlatform := int64(^uint(0) >> 1)
	minPlatform := -maxPlatform - 1
	legendRight, ok := checkedAdd(int64(layout.x), int64(layout.width), minPlatform, maxPlatform)
	if !ok {
		return 0, 0, 0, 0, fmt.Errorf("scene: legend right edge exceeds the platform integer domain")
	}
	right, ok := checkedAdd(int64(left), int64(width), minPlatform, maxPlatform)
	if !ok {
		return 0, 0, 0, 0, fmt.Errorf("scene: legend viewbox right edge exceeds the platform integer domain")
	}
	left64, top64, width64, height64 := int64(left), int64(top), int64(width), int64(height)
	if right < legendRight {
		width64, ok = checkedSub(legendRight, left64, minPlatform, maxPlatform)
		if ok {
			width64, ok = checkedAdd(width64, pad/2, minPlatform, maxPlatform)
		}
		if !ok {
			return 0, 0, 0, 0, fmt.Errorf("scene: legend width expansion exceeds the platform integer domain")
		}
	}
	if int64(layout.y) < top64 {
		difference, ok := checkedSub(top64, int64(layout.y), minPlatform, maxPlatform)
		if !ok {
			return 0, 0, 0, 0, fmt.Errorf("scene: legend top expansion exceeds the platform integer domain")
		}
		top64 = int64(layout.y)
		height64, ok = checkedAdd(height64, difference, minPlatform, maxPlatform)
		if !ok {
			return 0, 0, 0, 0, fmt.Errorf("scene: legend height expansion exceeds the platform integer domain")
		}
	}
	legendBottom, ok := checkedAdd(int64(layout.y), int64(layout.height), minPlatform, maxPlatform)
	if !ok {
		return 0, 0, 0, 0, fmt.Errorf("scene: legend bottom edge exceeds the platform integer domain")
	}
	bottom, ok := checkedAdd(top64, height64, minPlatform, maxPlatform)
	if !ok {
		return 0, 0, 0, 0, fmt.Errorf("scene: legend viewbox bottom edge exceeds the platform integer domain")
	}
	if bottom < legendBottom {
		height64, ok = checkedSub(legendBottom, top64, minPlatform, maxPlatform)
		if ok {
			height64, ok = checkedAdd(height64, pad/2, minPlatform, maxPlatform)
		}
		if !ok {
			return 0, 0, 0, 0, fmt.Errorf("scene: legend bottom expansion exceeds the platform integer domain")
		}
	}
	if width64 < 0 || height64 < 0 {
		return 0, 0, 0, 0, invalidPaddingError(pad)
	}
	return int(left64), int(top64), int(width64), int(height64), nil
}

func (b *builder) buildLegend(layout *legendLayout) (*d2scene.Node, error) {
	if layout == nil {
		return nil, nil
	}
	if err := b.ctx.Err(); err != nil {
		return nil, err
	}
	shadowFill, err := b.paint("#F7F7FA", "legend shadow fill")
	if err != nil {
		return nil, err
	}
	panelFill, err := b.paint("#ffffff", "legend panel fill")
	if err != nil {
		return nil, err
	}
	borderPaint, err := b.paint("#DEE1EB", "legend border")
	if err != nil {
		return nil, err
	}
	textPaint, err := b.paint("#000000", "legend text")
	if err != nil {
		return nil, err
	}
	border := func() *d2scene.Stroke {
		return &d2scene.Stroke{Paint: borderPaint, Width: 1, Cap: d2scene.CapButt, Join: d2scene.JoinMiter, MiterLimit: 4}
	}
	box := d2scene.Box{X: float64(layout.x), Y: float64(layout.y), Width: float64(layout.width), Height: float64(layout.height)}
	legend := d2scene.NewNode(nil)
	legend.ID = "legend"
	shadow := d2scene.NewNode(d2scene.Rect{Box: box, RadiusX: 4, RadiusY: 4, Fill: shadowFill, Stroke: border()})
	shadow.ID = "legend:shadow"
	shadow.Filters = []d2scene.Filter{d2scene.DropShadow{
		OffsetY: 2, SigmaX: 3, SigmaY: 3, Color: color.NRGBA{A: 26},
	}}
	panel := d2scene.NewNode(d2scene.Rect{Box: box, RadiusX: 4, RadiusY: 4, Fill: panelFill, Stroke: border()})
	panel.ID = "legend:panel"
	legend.Children = append(legend.Children, shadow, panel)

	title := b.diagram.Legend.Label
	if title == "" {
		title = "Legend"
	}
	titleFont, err := b.font(d2target.Text{FontSize: legendFontSize + 2, Bold: true})
	if err != nil {
		return nil, fmt.Errorf("scene: legend title: %w", err)
	}
	titleX := float64(layout.x + legendPadding)
	titleY := float64(layout.y + legendPadding + legendFontSize)
	titleNode := d2scene.NewNode(d2scene.TextRun{
		Text: legendRenderedText(title), Origin: d2scene.Point{X: titleX, Y: titleY}, Anchor: d2scene.AnchorStart,
		Font: titleFont, Fill: textPaint,
		Ink: legendTextInk(titleX, titleY, layout.titleDimensions),
	})
	titleNode.ID = "legend:title"
	legend.Children = append(legend.Children, titleNode)

	var itemFont d2scene.Font
	if layout.itemCount > 0 {
		itemFont, err = b.font(d2target.Text{FontSize: legendFontSize})
		if err != nil {
			return nil, fmt.Errorf("scene: legend item: %w", err)
		}
	}
	currentY := layout.y + legendPadding*2 + legendFontSize
	shapeCount := 0
	for index, targetShape := range b.diagram.Legend.Shapes {
		if err := b.ctx.Err(); err != nil {
			return nil, err
		}
		if targetShape.Label == "" {
			continue
		}
		iconX, iconY := layout.x+legendPadding, currentY
		icon, err := b.buildLegendShapeIcon(index, targetShape, iconX, iconY)
		if err != nil {
			return nil, err
		}
		legend.Children = append(legend.Children, icon)
		dimensions := layout.shapeDimensions[index]
		rowHeight := maxInt(dimensions.height, legendIconSize)
		textX := float64(iconX + legendIconSize + legendPadding)
		textY := float64(currentY + rowHeight/2 + int(float64(dimensions.height)*.3))
		text := d2scene.NewNode(d2scene.TextRun{
			Text: legendRenderedText(targetShape.Label), Origin: d2scene.Point{X: textX, Y: textY}, Anchor: d2scene.AnchorStart,
			Font: itemFont, Fill: textPaint, Ink: legendTextInk(textX, textY, dimensions),
		})
		text.ID = fmt.Sprintf("legend:shape:%d:label", index)
		legend.Children = append(legend.Children, text)
		currentY += rowHeight + legendItemSpacing
		shapeCount++
	}

	if shapeCount > 0 && len(b.diagram.Legend.Connections) > 0 {
		currentY += legendItemSpacing / 2
		separatorStroke := border()
		separatorStroke.Dashes = []float64{2, 2}
		separator := d2scene.NewNode(d2scene.Path{
			Commands: []d2scene.PathCommand{
				d2scene.MoveTo(float64(layout.x+legendPadding), float64(currentY)),
				d2scene.LineTo(float64(layout.x+layout.width-legendPadding), float64(currentY)),
			},
			Stroke: separatorStroke,
		})
		separator.ID = "legend:separator"
		legend.Children = append(legend.Children, separator)
		currentY += legendItemSpacing
	}

	for index, connection := range b.diagram.Legend.Connections {
		if err := b.ctx.Err(); err != nil {
			return nil, err
		}
		if connection.Label == "" {
			continue
		}
		iconX := layout.x + legendPadding
		iconY := currentY + legendIconSize/2
		icon, err := b.buildLegendConnectionIcon(index, connection, iconX, iconY)
		if err != nil {
			return nil, err
		}
		legend.Children = append(legend.Children, icon)
		dimensions := layout.connectionDimensions[index]
		rowHeight := maxInt(dimensions.height, legendIconSize)
		textX := float64(iconX + legendIconSize + legendPadding)
		textY := float64(currentY + rowHeight/2 + int(float64(dimensions.height)*.2))
		text := d2scene.NewNode(d2scene.TextRun{
			Text: legendRenderedText(connection.Label), Origin: d2scene.Point{X: textX, Y: textY}, Anchor: d2scene.AnchorStart,
			Font: itemFont, Fill: textPaint, Ink: legendTextInk(textX, textY, dimensions),
		})
		text.ID = fmt.Sprintf("legend:connection:%d:label", index)
		legend.Children = append(legend.Children, text)
		currentY += rowHeight + legendItemSpacing
	}
	return legend, b.ctx.Err()
}

func legendTextInk(x, baseline float64, dimensions legendItemDimensions) d2scene.Bounds {
	width := math.Max(1, float64(dimensions.width))
	height := math.Max(1, float64(dimensions.height))
	return d2scene.NewBounds(x, baseline-height, x+width, baseline)
}

// SVG's default white-space handling collapses ASCII spaces, tabs, and line
// breaks in a text node. Keep measuring the raw target string, as RenderLegend
// does, but give raster renderers the same whitespace-collapsed text.
func legendRenderedText(value string) string {
	var result strings.Builder
	pendingSpace := false
	for _, runeValue := range value {
		switch runeValue {
		case ' ', '\t', '\n', '\r':
			if result.Len() != 0 {
				pendingSpace = true
			}
		default:
			if pendingSpace {
				result.WriteByte(' ')
				pendingSpace = false
			}
			result.WriteRune(runeValue)
		}
	}
	return result.String()
}

func (b *builder) buildLegendShapeIcon(index int, targetShape d2target.Shape, x, y int) (*d2scene.Node, error) {
	target := legendShapeIconTarget(index, targetShape)
	geometryTarget := target
	geometryTarget.Opacity = 1
	geometryTarget.Animated = false
	geometryTarget.Shadow = false
	geometryTarget.Blend = false
	embeddedIcon := target.Type == d2target.ShapeImage || target.Type == d2target.ShapeClass || target.Type == d2target.ShapeSQLTable
	if !embeddedIcon {
		geometryTarget.Icon = nil
	}
	geometry, err := b.buildShape(geometryTarget)
	if err != nil {
		return nil, fmt.Errorf("scene: %s: %w", legendShapeObject(index, targetShape.ID), err)
	}
	geometry.ID = target.ID + ":shape"
	geometry.Classes = nil
	if targetShape.Shadow && legendShapePaintsShadow(targetShape.Type) {
		geometry.Filters = append(geometry.Filters, d2scene.DropShadow{
			OffsetX: 3, OffsetY: 5, SigmaX: 1.7, SigmaY: 1.7,
			Color: color.NRGBA{R: 0x3d, G: 0x45, B: 0x74, A: 102},
		})
	}
	if targetShape.Blend {
		geometry.Blend = d2scene.BlendMultiply
		geometry.Opacity *= .5
	}

	targetNode := d2scene.NewNode(nil)
	targetNode.ID = target.ID
	targetNode.Classes = append([]string(nil), targetShape.Classes...)
	targetNode.Opacity = targetShape.Opacity
	targetNode.Children = append(targetNode.Children, geometry)
	if !embeddedIcon && targetShape.Icon != nil && targetShape.Opacity != 0 {
		iconTarget := target
		iconTarget.Opacity = 1
		icon, err := b.buildShapeIcon(iconTarget, false)
		if err != nil {
			return nil, fmt.Errorf("scene: %s: %w", legendShapeObject(index, targetShape.ID), err)
		}
		if icon != nil {
			targetNode.Children = append(targetNode.Children, icon)
		}
	}
	if targetShape.Animated {
		addLegendShapeAnimation(targetNode)
	}

	wrapper := d2scene.NewNode(nil)
	wrapper.ID = fmt.Sprintf("legend:shape:%d", index)
	wrapper.Transform = d2scene.Translate(float64(x), float64(y)).Mul(d2scene.Scale(1.0/legendShapeScaleFactor, 1.0/legendShapeScaleFactor))
	wrapper.Children = []*d2scene.Node{targetNode}
	return wrapper, nil
}

func legendShapePaintsShadow(shapeType string) bool {
	switch shapeType {
	case d2target.ShapeText, d2target.ShapeCode, d2target.ShapeClass, d2target.ShapeSQLTable:
		return false
	default:
		return true
	}
}

func addLegendShapeAnimation(node *d2scene.Node) {
	transparent := d2scene.DropShadow{Color: color.NRGBA{}}
	node.Filters = append(node.Filters, transparent, transparent)
	linear := d2scene.Easing{Kind: d2scene.EaseLinear}
	node.Animations = append(node.Animations,
		d2scene.Track{
			Property: d2scene.AnimateTransform, Duration: time.Second, Repeat: true,
			Keyframes: []d2scene.Keyframe{
				{Offset: 0, Value: d2scene.TransformValue(d2scene.Identity()), Easing: linear},
				{Offset: .5, Value: d2scene.TransformValue(d2scene.Translate(0, -4)), Easing: linear},
				{Offset: 1, Value: d2scene.TransformValue(d2scene.Identity())},
			},
		},
		legendShapeShadowTrack(0, d2scene.DropShadow{
			OffsetY: 12.6, SigmaX: 25.2, SigmaY: 25.2,
			Color: color.NRGBA{R: 50, G: 50, B: 93, A: 64},
		}),
		legendShapeShadowTrack(1, d2scene.DropShadow{
			OffsetY: 7.56, SigmaX: 15.12, SigmaY: 15.12,
			Color: color.NRGBA{A: 26},
		}),
	)
}

func legendShapeShadowTrack(index int, middle d2scene.DropShadow) d2scene.Track {
	transparent := d2scene.DropShadow{Color: color.NRGBA{}}
	return d2scene.Track{
		Property: d2scene.AnimateDropShadow, TargetIndex: index, Duration: time.Second, Repeat: true,
		Keyframes: []d2scene.Keyframe{
			{Offset: 0, Value: d2scene.ShadowValue(transparent), Easing: d2scene.Easing{Kind: d2scene.EaseLinear}},
			{Offset: .5, Value: d2scene.ShadowValue(middle), Easing: d2scene.Easing{Kind: d2scene.EaseLinear}},
			{Offset: 1, Value: d2scene.ShadowValue(transparent)},
		},
	}
}

func (b *builder) buildLegendConnectionIcon(index int, connection d2target.Connection, x, y int) (*d2scene.Node, error) {
	target := legendConnectionIconTarget(index, connection)
	connectionMask := b.connectionMask
	idToShape := b.idToShape
	b.connectionMask = nil
	b.idToShape = nil
	node, err := func() (*d2scene.Node, error) {
		defer func() {
			b.connectionMask = connectionMask
			b.idToShape = idToShape
		}()
		return b.buildConnection(target)
	}()
	if err != nil {
		return nil, fmt.Errorf("scene: %s: %w", legendConnectionObject(index, connection.ID), err)
	}
	wrapper := d2scene.NewNode(nil)
	wrapper.ID = fmt.Sprintf("legend:connection:%d", index)
	wrapper.Transform = d2scene.Translate(float64(x), float64(y)).Mul(d2scene.Scale(1.0/legendConnectionScaleFactor, 1.0/legendConnectionScaleFactor))
	wrapper.Children = []*d2scene.Node{node}
	return wrapper, nil
}
