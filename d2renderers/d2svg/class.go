package d2svg

import (
	"fmt"
	"io"

	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/d2themes"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/label"
	"oss.terrastruct.com/d2/lib/svg"
)

func classHeader(shape d2target.Shape, box *geo.Box, text string, textWidth, textHeight, fontSize float64) string {
	rectEl := d2themes.NewThemableElement("rect")
	rectEl.X, rectEl.Y = box.TopLeft.X, box.TopLeft.Y
	rectEl.Width, rectEl.Height = box.Width, box.Height
	rectEl.Fill = shape.Fill
	rectEl.ClassName = "class_header"
	str := rectEl.Render()

	if text != "" {
		tl := label.InsideMiddleCenter.GetPointOnBox(
			box,
			0,
			textWidth,
			textHeight,
		)

		textEl := d2themes.NewThemableElement("text")
		textEl.X = tl.X + textWidth/2
		textEl.Y = tl.Y + textHeight*3/4
		textEl.Fill = shape.GetFontColor()
		textEl.ClassName = "text-mono"
		textEl.Style = fmt.Sprintf(`text-anchor:%s;font-size:%vpx;`,
			"middle", 4+fontSize,
		)
		textEl.Content = svg.EscapeText(text)
		str += textEl.Render()
	}
	return str
}

func classRow(shape d2target.Shape, box *geo.Box, prefix, nameText, typeText string, fontSize float64) string {
	// Row is made up of prefix, name, and type
	// e.g. | + firstName   string  |
	prefixTL := label.InsideMiddleLeft.GetPointOnBox(
		box,
		d2target.PrefixPadding,
		box.Width,
		fontSize,
	)
	typeTR := label.InsideMiddleRight.GetPointOnBox(
		box,
		d2target.TypePadding,
		0,
		fontSize,
	)

	textEl := d2themes.NewThemableElement("text")
	textEl.X = prefixTL.X
	textEl.Y = prefixTL.Y + fontSize*3/4
	textEl.Fill = shape.PrimaryAccentColor
	textEl.ClassName = "text-mono"
	textEl.Style = fmt.Sprintf("text-anchor:%s;font-size:%vpx", "start", fontSize)
	textEl.Content = prefix
	out := textEl.Render()

	textEl.X = prefixTL.X + d2target.PrefixWidth
	textEl.Fill = shape.Fill
	textEl.Content = svg.EscapeText(nameText)
	out += textEl.Render()

	textEl.X = typeTR.X
	textEl.Y = typeTR.Y + fontSize*3/4
	textEl.Fill = shape.SecondaryAccentColor
	textEl.Style = fmt.Sprintf("text-anchor:%s;font-size:%vpx", "end", fontSize)
	textEl.Content = svg.EscapeText(typeText)
	out += textEl.Render()

	return out
}

func drawClass(writer io.Writer, targetShape d2target.Shape) {
	el := d2themes.NewThemableElement("rect")
	el.X = float64(targetShape.Pos.X)
	el.Y = float64(targetShape.Pos.Y)
	el.Width = float64(targetShape.Width)
	el.Height = float64(targetShape.Height)
	el.Fill, el.Stroke = d2themes.ShapeTheme(targetShape)
	el.Style = targetShape.CSSStyle()
	fmt.Fprint(writer, el.Render())

	box := geo.NewBox(
		geo.NewPoint(float64(targetShape.Pos.X), float64(targetShape.Pos.Y)),
		float64(targetShape.Width),
		float64(targetShape.Height),
	)
	rowHeight := box.Height / float64(2+len(targetShape.Class.Fields)+len(targetShape.Class.Methods))
	headerBox := geo.NewBox(box.TopLeft, box.Width, 2*rowHeight)

	fmt.Fprint(writer,
		classHeader(targetShape, headerBox, targetShape.Label, float64(targetShape.LabelWidth), float64(targetShape.LabelHeight), float64(targetShape.FontSize)),
	)

	rowBox := geo.NewBox(box.TopLeft.Copy(), box.Width, rowHeight)
	rowBox.TopLeft.Y += headerBox.Height
	for _, f := range targetShape.Fields {
		fmt.Fprint(writer,
			classRow(targetShape, rowBox, f.VisibilityToken(), f.Name, f.Type, float64(targetShape.FontSize)),
		)
		rowBox.TopLeft.Y += rowHeight
	}

	lineEl := d2themes.NewThemableElement("line")
	lineEl.X1, lineEl.Y1 = rowBox.TopLeft.X, rowBox.TopLeft.Y
	lineEl.X2, lineEl.Y2 = rowBox.TopLeft.X+rowBox.Width, rowBox.TopLeft.Y
	lineEl.Stroke = targetShape.Fill
	lineEl.Style = "stroke-width:1"
	fmt.Fprint(writer, lineEl.Render())

	for _, m := range targetShape.Methods {
		fmt.Fprint(writer,
			classRow(targetShape, rowBox, m.VisibilityToken(), m.Name, m.Return, float64(targetShape.FontSize)),
		)
		rowBox.TopLeft.Y += rowHeight
	}
}
