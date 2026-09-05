package d2scenebuild

import (
	"fmt"
	"math"

	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
)

func (b *builder) buildStructuredShape(targetShape d2target.Shape, outerFill d2scene.Paint, outerStroke *d2scene.Stroke) ([]*d2scene.Node, error) {
	if err := validateStructuredShape(targetShape); err != nil {
		return nil, err
	}
	box := d2scene.Box{
		X: float64(targetShape.Pos.X), Y: float64(targetShape.Pos.Y),
		Width: float64(targetShape.Width), Height: float64(targetShape.Height),
	}
	radius := clampBorderRadius(float64(targetShape.BorderRadius), box.Width, box.Height)
	outer := d2scene.NewNode(d2scene.Rect{Box: box, RadiusX: radius, RadiusY: radius, Fill: outerFill, Stroke: outerStroke})
	outer.ID = targetShape.ID + ":outline"
	nodes := []*d2scene.Node{outer}

	switch targetShape.Type {
	case d2target.ShapeClass:
		children, err := b.buildClassContents(targetShape, box, radius)
		if err != nil {
			return nil, err
		}
		return append(nodes, children...), nil
	case d2target.ShapeSQLTable:
		children, err := b.buildSQLTableContents(targetShape, box, radius)
		if err != nil {
			return nil, err
		}
		return append(nodes, children...), nil
	default:
		return nil, unsupported(fmt.Sprintf("shape %q", targetShape.ID), "structured shape "+targetShape.Type)
	}
}

func validateStructuredShape(targetShape d2target.Shape) error {
	object := fmt.Sprintf("shape %q", targetShape.ID)
	if targetShape.FontSize <= 0 && (targetShape.Label != "" || len(targetShape.Fields) != 0 || len(targetShape.Methods) != 0 || len(targetShape.Columns) != 0) {
		return invalidField(object, "fontSize", targetShape.FontSize, "must be positive for a structured shape with text")
	}
	if targetShape.Width <= 0 || targetShape.Height <= 0 {
		return invalidField(object, "dimensions", fmt.Sprintf("%dx%d", targetShape.Width, targetShape.Height), "must be positive for a structured shape")
	}
	switch targetShape.Type {
	case d2target.ShapeClass:
		rowCount := 2 + len(targetShape.Fields) + len(targetShape.Methods)
		rowHeight := float64(targetShape.Height) / float64(rowCount)
		headerHeight := math.Max(2*rowHeight, float64(targetShape.LabelHeight)+2*label.PADDING)
		if headerHeight > float64(targetShape.Height) {
			return invalidField(object, "labelHeight", targetShape.LabelHeight, "does not fit the class height")
		}
	case d2target.ShapeSQLTable:
		// Width, height, and text constraints above are the complete static
		// table layout preconditions.
	default:
		return unsupported(object, "structured shape "+targetShape.Type)
	}
	return nil
}

func (b *builder) buildClassContents(targetShape d2target.Shape, box d2scene.Box, radius float64) ([]*d2scene.Node, error) {
	rowCount := 2 + len(targetShape.Fields) + len(targetShape.Methods)
	rowHeight := box.Height / float64(rowCount)
	headerHeight := math.Max(2*rowHeight, float64(targetShape.LabelHeight)+2*label.PADDING)
	headerBox := d2scene.Box{X: box.X, Y: box.Y, Width: box.Width, Height: headerHeight}
	headerFill, err := b.paint(targetShape.Fill, fmt.Sprintf("shape %q class header", targetShape.ID))
	if err != nil {
		return nil, err
	}
	header := d2scene.NewNode(structuredHeaderPrimitive(headerBox, box, radius, headerFill))
	header.ID = targetShape.ID + ":class-header"
	nodes := []*d2scene.Node{header}

	if targetShape.Label != "" {
		fontText := targetShape.Text
		fontText.FontFamily = "mono"
		fontText.FontSize = targetShape.FontSize + d2target.HeaderFontAdd
		// Structured-shape text uses the SVG renderer's dedicated regular
		// class faces rather than the bold default carried by BaseShape.
		fontText.Bold = false
		fontText.Italic = false
		font, err := b.font(fontText)
		if err != nil {
			return nil, fmt.Errorf("scene: shape %q class header: %w", targetShape.ID, err)
		}
		fill, err := b.paint(targetShape.GetFontColor(), fmt.Sprintf("shape %q class header text", targetShape.ID))
		if err != nil {
			return nil, err
		}
		headerGeo := sceneBoxToGeo(headerBox)
		topLeft := label.InsideMiddleCenter.GetPointOnBox(headerGeo, 0, float64(targetShape.LabelWidth), float64(targetShape.LabelHeight))
		headerID := targetShape.ID + ":class-header-label"
		headerRuns := centeredTextRuns(
			headerID, targetShape.Label, topLeft,
			targetShape.LabelWidth, targetShape.LabelHeight, targetShape.FontSize,
			font, fill, false,
		)
		// Preserve the historical single-line scene ID while assigning stable,
		// unique suffixes to additional multiline baselines.
		for index, run := range headerRuns {
			if index == 0 {
				run.ID = headerID
			} else {
				run.ID = fmt.Sprintf("%s:%d", headerID, index)
			}
		}
		nodes = append(nodes, headerRuns...)
	}

	primary, err := b.paint(targetShape.PrimaryAccentColor, fmt.Sprintf("shape %q class primary accent", targetShape.ID))
	if err != nil {
		return nil, err
	}
	namePaint, err := b.paint(targetShape.Fill, fmt.Sprintf("shape %q class row name", targetShape.ID))
	if err != nil {
		return nil, err
	}
	secondary, err := b.paint(targetShape.SecondaryAccentColor, fmt.Sprintf("shape %q class secondary accent", targetShape.ID))
	if err != nil {
		return nil, err
	}
	rowFontText := targetShape.Text
	rowFontText.FontFamily = "mono"
	rowFontText.Bold = false
	rowFontText.Italic = false
	rowFont, err := b.font(rowFontText)
	if err != nil {
		return nil, fmt.Errorf("scene: shape %q class row: %w", targetShape.ID, err)
	}
	rowBox := d2scene.Box{X: box.X, Y: box.Y + headerHeight, Width: box.Width, Height: rowHeight}
	for index, field := range targetShape.Fields {
		nodes = append(nodes, buildClassRowNodes(targetShape.ID, "field", index, rowBox, float64(targetShape.FontSize), rowFont,
			field.VisibilityToken(), field.Name, field.Type, field.Underline, primary, namePaint, secondary)...)
		rowBox.Y += rowHeight
	}

	separatorStart, separatorEnd := rowBox.X, rowBox.X+rowBox.Width
	if radius != 0 && len(targetShape.Methods) == 0 {
		separatorStart += radius
		separatorEnd -= radius
	}
	separatorPaint, err := b.paint(targetShape.Fill, fmt.Sprintf("shape %q class separator", targetShape.ID))
	if err != nil {
		return nil, err
	}
	separator := d2scene.NewNode(d2scene.Path{
		Commands: []d2scene.PathCommand{d2scene.MoveTo(separatorStart, rowBox.Y), d2scene.LineTo(separatorEnd, rowBox.Y)},
		Stroke:   &d2scene.Stroke{Paint: separatorPaint, Width: 1, Cap: d2scene.CapButt, Join: d2scene.JoinMiter, MiterLimit: 4},
	})
	separator.ID = targetShape.ID + ":class-separator"
	nodes = append(nodes, separator)

	for index, method := range targetShape.Methods {
		nodes = append(nodes, buildClassRowNodes(targetShape.ID, "method", index, rowBox, float64(targetShape.FontSize), rowFont,
			method.VisibilityToken(), method.Name, method.Return, method.Underline, primary, namePaint, secondary)...)
		rowBox.Y += rowHeight
	}
	return nodes, nil
}

func buildClassRowNodes(shapeID, kind string, index int, box d2scene.Box, fontSize float64, font d2scene.Font, prefix, name, typeName string, underline bool, prefixPaint, namePaint, typePaint d2scene.Paint) []*d2scene.Node {
	geoBox := sceneBoxToGeo(box)
	prefixTopLeft := label.InsideMiddleLeft.GetPointOnBox(geoBox, d2target.PrefixPadding, box.Width, fontSize)
	typeTopRight := label.InsideMiddleRight.GetPointOnBox(geoBox, d2target.TypePadding, 0, fontSize)
	baseline := prefixTopLeft.Y + fontSize*3/4
	prefixID := fmt.Sprintf("%s:class-%s:%d:prefix", shapeID, kind, index)
	nameID := fmt.Sprintf("%s:class-%s:%d:name", shapeID, kind, index)
	typeID := fmt.Sprintf("%s:class-%s:%d:type", shapeID, kind, index)
	return []*d2scene.Node{
		structuredTextNode(prefixID, prefix, d2scene.Point{X: prefixTopLeft.X, Y: baseline}, d2scene.AnchorStart, font, prefixPaint, false, box),
		structuredTextNode(nameID, name, d2scene.Point{X: prefixTopLeft.X + d2target.PrefixWidth, Y: baseline}, d2scene.AnchorStart, font, namePaint, underline, box),
		structuredTextNode(typeID, typeName, d2scene.Point{X: typeTopRight.X, Y: baseline}, d2scene.AnchorEnd, font, typePaint, false, box),
	}
}

func (b *builder) buildSQLTableContents(targetShape d2target.Shape, box d2scene.Box, radius float64) ([]*d2scene.Node, error) {
	rowHeight := box.Height / float64(1+len(targetShape.Columns))
	headerBox := d2scene.Box{X: box.X, Y: box.Y, Width: box.Width, Height: rowHeight}
	headerFill, err := b.paint(targetShape.Fill, fmt.Sprintf("shape %q table header", targetShape.ID))
	if err != nil {
		return nil, err
	}
	header := d2scene.NewNode(structuredHeaderPrimitive(headerBox, box, radius, headerFill))
	header.ID = targetShape.ID + ":table-header"
	nodes := []*d2scene.Node{header}

	if targetShape.Label != "" {
		fontText := targetShape.Text
		fontText.FontSize = targetShape.FontSize + d2target.HeaderFontAdd
		fontText.Bold = false
		fontText.Italic = false
		font, err := b.font(fontText)
		if err != nil {
			return nil, fmt.Errorf("scene: shape %q table header: %w", targetShape.ID, err)
		}
		fill, err := b.paint(targetShape.GetFontColor(), fmt.Sprintf("shape %q table header text", targetShape.ID))
		if err != nil {
			return nil, err
		}
		headerGeo := sceneBoxToGeo(headerBox)
		topLeft := label.InsideMiddleLeft.GetPointOnBox(headerGeo, float64(d2target.HeaderPadding), box.Width, float64(targetShape.LabelHeight))
		nodes = append(nodes, structuredTextNode(
			targetShape.ID+":table-header-label", targetShape.Label,
			d2scene.Point{X: topLeft.X, Y: topLeft.Y + float64(targetShape.LabelHeight)*3/4},
			d2scene.AnchorStart, font, fill, false, headerBox,
		))
	}

	longestNameWidth, longestTypeWidth := 0, 0
	for _, column := range targetShape.Columns {
		if column.Name.LabelWidth > longestNameWidth {
			longestNameWidth = column.Name.LabelWidth
		}
		if column.Type.LabelWidth > longestTypeWidth {
			longestTypeWidth = column.Type.LabelWidth
		}
	}
	_ = longestTypeWidth // retained to mirror the measured table layout contract.
	rowFontText := targetShape.Text
	rowFontText.Bold = false
	rowFontText.Italic = false
	rowFont, err := b.font(rowFontText)
	if err != nil {
		return nil, fmt.Errorf("scene: shape %q table row: %w", targetShape.ID, err)
	}
	namePaint, err := b.paint(targetShape.PrimaryAccentColor, fmt.Sprintf("shape %q table name accent", targetShape.ID))
	if err != nil {
		return nil, err
	}
	typePaint, err := b.paint(targetShape.NeutralAccentColor, fmt.Sprintf("shape %q table type accent", targetShape.ID))
	if err != nil {
		return nil, err
	}
	constraintPaint, err := b.paint(targetShape.SecondaryAccentColor, fmt.Sprintf("shape %q table constraint accent", targetShape.ID))
	if err != nil {
		return nil, err
	}
	separatorPaint, err := b.paint(targetShape.Fill, fmt.Sprintf("shape %q table separator", targetShape.ID))
	if err != nil {
		return nil, err
	}
	rowBox := d2scene.Box{X: box.X, Y: box.Y + rowHeight, Width: box.Width, Height: rowHeight}
	for index, column := range targetShape.Columns {
		geoBox := sceneBoxToGeo(rowBox)
		nameTopLeft := label.InsideMiddleLeft.GetPointOnBox(geoBox, d2target.NamePadding, 0, float64(targetShape.FontSize))
		baseline := nameTopLeft.Y + float64(targetShape.FontSize)*3/4
		prefix := fmt.Sprintf("%s:table-row:%d", targetShape.ID, index)
		nodes = append(nodes,
			structuredTextNode(prefix+":name", column.Name.Label, d2scene.Point{X: nameTopLeft.X, Y: baseline}, d2scene.AnchorStart, rowFont, namePaint, false, rowBox),
			structuredTextNode(prefix+":type", column.Type.Label, d2scene.Point{X: nameTopLeft.X + float64(longestNameWidth+d2target.TypePadding), Y: baseline}, d2scene.AnchorStart, rowFont, typePaint, false, rowBox),
			structuredTextNode(prefix+":constraint", column.ConstraintAbbr(), d2scene.Point{X: rowBox.X + rowBox.Width - d2target.NamePadding, Y: baseline}, d2scene.AnchorEnd, rowFont, constraintPaint, false, rowBox),
		)
		rowBox.Y += rowHeight
		lineStart, lineEnd := rowBox.X, rowBox.X+rowBox.Width
		if index == len(targetShape.Columns)-1 && radius != 0 {
			lineStart += radius
			lineEnd -= radius
		}
		separator := d2scene.NewNode(d2scene.Path{
			Commands: []d2scene.PathCommand{d2scene.MoveTo(lineStart, rowBox.Y), d2scene.LineTo(lineEnd, rowBox.Y)},
			Stroke:   &d2scene.Stroke{Paint: separatorPaint, Width: 2, Cap: d2scene.CapButt, Join: d2scene.JoinMiter, MiterLimit: 4},
		})
		separator.ID = prefix + ":separator"
		nodes = append(nodes, separator)
	}
	return nodes, nil
}

func structuredHeaderPrimitive(header, outer d2scene.Box, radius float64, fill d2scene.Paint) d2scene.Primitive {
	if radius == 0 {
		return d2scene.Rect{Box: header, Fill: fill}
	}
	radius = math.Min(radius, math.Min(header.Width/2, header.Height))
	x, y, w, h := header.X, header.Y, header.Width, header.Height
	commands := []d2scene.PathCommand{
		d2scene.MoveTo(x, y+radius),
		// Smooth cubics after a line use the current point as the reflected first
		// control.
		d2scene.CubicTo(x, y+radius, x, y, x+radius, y),
		d2scene.LineTo(x+w-radius, y),
		d2scene.CubicTo(x+w-radius, y, x+w, y, x+w, y+radius),
	}
	if header.Height >= outer.Height {
		commands = append(commands,
			d2scene.LineTo(x+w, y+h-radius),
			d2scene.CubicTo(x+w, y+h-radius, x+w, y+h, x+w-radius, y+h),
			d2scene.LineTo(x+radius, y+h),
			d2scene.CubicTo(x+radius, y+h, x, y+h, x, y+h-radius),
			d2scene.LineTo(x, y+radius),
			d2scene.ClosePath(),
		)
	} else {
		commands = append(commands,
			d2scene.LineTo(x+w, y+h),
			d2scene.LineTo(x, y+h),
			d2scene.ClosePath(),
		)
	}
	return d2scene.Path{Fill: fill, Commands: commands}
}

func structuredTextNode(id, text string, origin d2scene.Point, anchor d2scene.TextAnchor, font d2scene.Font, fill d2scene.Paint, underline bool, ink d2scene.Box) *d2scene.Node {
	node := d2scene.NewNode(d2scene.TextRun{
		Text: text, Origin: origin, Anchor: anchor, Font: font, Fill: fill, Underline: underline, Ink: ink.Bounds(),
	})
	node.ID = id
	return node
}

func sceneBoxToGeo(box d2scene.Box) *geo.Box {
	return geo.NewBox(geo.NewPoint(box.X, box.Y), box.Width, box.Height)
}
