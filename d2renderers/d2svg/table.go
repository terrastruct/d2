package d2svg

import (
	"fmt"
	"io"

	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/d2themes"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/label"
	"oss.terrastruct.com/d2/lib/svg"
	"oss.terrastruct.com/util-go/go2"
)

// this func helps define a clipPath for shape class and sql_table to draw border-radius
func clipPathForBorderRadius(labelMaskID string, shape d2target.Shape) string {
	box := geo.NewBox(
		geo.NewPoint(float64(shape.Pos.X), float64(shape.Pos.Y)),
		float64(shape.Width),
		float64(shape.Height),
	)
	topX, topY := box.TopLeft.X+box.Width, box.TopLeft.Y

	out := fmt.Sprintf(`<clipPath id="%v-%v">`, labelMaskID, shape.ID)
	out += fmt.Sprintf(`<path d="M %f %f L %f %f S %f %f %f %f `, box.TopLeft.X, box.TopLeft.Y+float64(shape.BorderRadius), box.TopLeft.X, box.TopLeft.Y+float64(shape.BorderRadius), box.TopLeft.X, box.TopLeft.Y, box.TopLeft.X+float64(shape.BorderRadius), box.TopLeft.Y)
	out += fmt.Sprintf(`L %f %f L %f %f `, box.TopLeft.X+box.Width-float64(shape.BorderRadius), box.TopLeft.Y, topX-float64(shape.BorderRadius), topY)

	out += fmt.Sprintf(`S %f %f %f %f `, topX, topY, topX, topY+float64(shape.BorderRadius))
	out += fmt.Sprintf(`L %f %f `, topX, topY+box.Height-float64(shape.BorderRadius))

	if len(shape.Columns) != 0 {
		out += fmt.Sprintf(`L %f %f L %f %f`, topX, topY+box.Height, box.TopLeft.X, box.TopLeft.Y+box.Height)
	} else {
		out += fmt.Sprintf(`S %f % f %f %f `, topX, topY+box.Height, topX-float64(shape.BorderRadius), topY+box.Height)
		out += fmt.Sprintf(`L %f %f `, box.TopLeft.X+float64(shape.BorderRadius), box.TopLeft.Y+box.Height)
		out += fmt.Sprintf(`S %f %f %f %f`, box.TopLeft.X, box.TopLeft.Y+box.Height, box.TopLeft.X, box.TopLeft.Y+box.Height-float64(shape.BorderRadius))
		out += fmt.Sprintf(`L %f %f`, box.TopLeft.X, box.TopLeft.Y+float64(shape.BorderRadius))
	}
	out += fmt.Sprintf(`Z %f %f" `, box.TopLeft.X, box.TopLeft.Y)
	return out + `fill="none" /> </clipPath>`
}

func tableHeader(labelMaskID string, shape d2target.Shape, box *geo.Box, text string, textWidth, textHeight, fontSize float64) string {
	rectEl := d2themes.NewThemableElement("rect")
	rectEl.X, rectEl.Y = box.TopLeft.X, box.TopLeft.Y
	rectEl.Width, rectEl.Height = box.Width, box.Height
	rectEl.Fill = shape.Fill
	rectEl.ClassName = "class_header"
	str := rectEl.RenderWithClipPath(fmt.Sprintf("%v-%v", labelMaskID, shape.ID), shape.BorderRadius != 0)

	if text != "" {
		tl := label.InsideMiddleLeft.GetPointOnBox(
			box,
			float64(d2target.HeaderPadding),
			float64(shape.Width),
			textHeight,
		)

		textEl := d2themes.NewThemableElement("text")
		textEl.X = tl.X
		textEl.Y = tl.Y + textHeight*3/4
		textEl.Fill = shape.GetFontColor()
		textEl.ClassName = "text"
		textEl.Style = fmt.Sprintf("text-anchor:%s;font-size:%vpx",
			"start", 4+fontSize,
		)
		textEl.Content = svg.EscapeText(text)
		str += textEl.Render()
	}
	return str
}

func tableRow(shape d2target.Shape, box *geo.Box, nameText, typeText, constraintText string, fontSize, longestNameWidth float64) string {
	// Row is made up of name, type, and constraint
	// e.g. | diagram   int   FK |
	nameTL := label.InsideMiddleLeft.GetPointOnBox(
		box,
		d2target.NamePadding,
		box.Width,
		fontSize,
	)
	constraintTR := label.InsideMiddleRight.GetPointOnBox(
		box,
		d2target.TypePadding,
		0,
		fontSize,
	)

	textEl := d2themes.NewThemableElement("text")
	textEl.X = nameTL.X
	textEl.Y = nameTL.Y + fontSize*3/4
	textEl.Fill = shape.PrimaryAccentColor
	textEl.ClassName = "text"
	textEl.Style = fmt.Sprintf("text-anchor:%s;font-size:%vpx", "start", fontSize)
	textEl.Content = svg.EscapeText(nameText)
	out := textEl.Render()

	textEl.X = nameTL.X + longestNameWidth + 2*d2target.NamePadding
	textEl.Fill = shape.NeutralAccentColor
	textEl.Content = svg.EscapeText(typeText)
	out += textEl.Render()

	textEl.X = constraintTR.X
	textEl.Y = constraintTR.Y + fontSize*3/4
	textEl.Fill = shape.SecondaryAccentColor
	textEl.Style = fmt.Sprintf("text-anchor:%s;font-size:%vpx;letter-spacing:2px", "end", fontSize)
	textEl.Content = constraintText
	out += textEl.Render()

	return out
}

func drawTable(writer io.Writer, labelMaskID string, targetShape d2target.Shape) {
	rectEl := d2themes.NewThemableElement("rect")
	rectEl.X = float64(targetShape.Pos.X)
	rectEl.Y = float64(targetShape.Pos.Y)
	rectEl.Width = float64(targetShape.Width)
	rectEl.Height = float64(targetShape.Height)
	rectEl.Fill, rectEl.Stroke = d2themes.ShapeTheme(targetShape)
	rectEl.ClassName = "shape"
	rectEl.Style = targetShape.CSSStyle()
	if targetShape.BorderRadius != 0 {
		rectEl.Rx = float64(targetShape.BorderRadius)
		rectEl.Ry = float64(targetShape.BorderRadius)
	}
	fmt.Fprint(writer, rectEl.Render())

	box := geo.NewBox(
		geo.NewPoint(float64(targetShape.Pos.X), float64(targetShape.Pos.Y)),
		float64(targetShape.Width),
		float64(targetShape.Height),
	)
	rowHeight := box.Height / float64(1+len(targetShape.SQLTable.Columns))
	headerBox := geo.NewBox(box.TopLeft, box.Width, rowHeight)

	fmt.Fprint(writer,
		tableHeader(labelMaskID, targetShape, headerBox, targetShape.Label,
			float64(targetShape.LabelWidth), float64(targetShape.LabelHeight), float64(targetShape.FontSize)),
	)

	var longestNameWidth int
	for _, f := range targetShape.Columns {
		longestNameWidth = go2.Max(longestNameWidth, f.Name.LabelWidth)
	}

	rowBox := geo.NewBox(box.TopLeft.Copy(), box.Width, rowHeight)
	rowBox.TopLeft.Y += headerBox.Height
	for idx, f := range targetShape.Columns {
		fmt.Fprint(writer,
			tableRow(targetShape, rowBox, f.Name.Label, f.Type.Label, f.ConstraintAbbr(), float64(targetShape.FontSize), float64(longestNameWidth)),
		)
		rowBox.TopLeft.Y += rowHeight

		lineEl := d2themes.NewThemableElement("line")
		if idx == len(targetShape.Columns)-1 && targetShape.BorderRadius != 0 {
			lineEl.X1, lineEl.Y1 = rowBox.TopLeft.X+float64(targetShape.BorderRadius), rowBox.TopLeft.Y
			lineEl.X2, lineEl.Y2 = rowBox.TopLeft.X+rowBox.Width-float64(targetShape.BorderRadius), rowBox.TopLeft.Y
		} else {
			lineEl.X1, lineEl.Y1 = rowBox.TopLeft.X, rowBox.TopLeft.Y
			lineEl.X2, lineEl.Y2 = rowBox.TopLeft.X+rowBox.Width, rowBox.TopLeft.Y
		}
		lineEl.Stroke = targetShape.Fill
		lineEl.Style = "stroke-width:2"
		fmt.Fprint(writer, lineEl.Render())
	}
}
