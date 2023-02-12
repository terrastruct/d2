package d2svg

import (
	"fmt"
	"io"
	"strings"

	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/label"
	"oss.terrastruct.com/d2/lib/svg"
	"oss.terrastruct.com/util-go/go2"
)

func tableHeader(shape d2target.Shape, box *geo.Box, text string, textWidth, textHeight, fontSize float64) string {
	str := fmt.Sprintf(`<rect class="class_header" x="%f" y="%f" width="%f" height="%f" fill="%s" />`,
		box.TopLeft.X, box.TopLeft.Y, box.Width, box.Height, shape.Fill)

	if text != "" {
		tl := label.InsideMiddleLeft.GetPointOnBox(
			box,
			float64(d2target.HeaderPadding),
			float64(shape.Width),
			textHeight,
		)

		str += fmt.Sprintf(`<text class="%s" x="%f" y="%f" style="%s">%s</text>`,
			"text",
			tl.X,
			tl.Y+textHeight*3/4,
			fmt.Sprintf("text-anchor:%s;font-size:%vpx;fill:%s",
				"start",
				4+fontSize,
				shape.Stroke,
			),
			svg.EscapeText(text),
		)
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

	return strings.Join([]string{
		fmt.Sprintf(`<text class="text" x="%f" y="%f" style="%s">%s</text>`,
			nameTL.X,
			nameTL.Y+fontSize*3/4,
			fmt.Sprintf("text-anchor:%s;font-size:%vpx;fill:%s", "start", fontSize, shape.PrimaryAccentColor),
			svg.EscapeText(nameText),
		),

		fmt.Sprintf(`<text class="text" x="%f" y="%f" style="%s">%s</text>`,
			nameTL.X+longestNameWidth+2*d2target.NamePadding,
			nameTL.Y+fontSize*3/4,
			fmt.Sprintf("text-anchor:%s;font-size:%vpx;fill:%s", "start", fontSize, shape.NeutralAccentColor),
			svg.EscapeText(typeText),
		),

		fmt.Sprintf(`<text class="text" x="%f" y="%f" style="%s">%s</text>`,
			constraintTR.X,
			constraintTR.Y+fontSize*3/4,
			fmt.Sprintf("text-anchor:%s;font-size:%vpx;fill:%s;letter-spacing:2px;", "end", fontSize, shape.SecondaryAccentColor),
			constraintText,
		),
	}, "\n")
}

func drawTable(writer io.Writer, targetShape d2target.Shape) {
	fmt.Fprintf(writer, `<rect class="shape" x="%d" y="%d" width="%d" height="%d" style="%s"/>`,
		targetShape.Pos.X, targetShape.Pos.Y, targetShape.Width, targetShape.Height, targetShape.CSSStyle())

	box := geo.NewBox(
		geo.NewPoint(float64(targetShape.Pos.X), float64(targetShape.Pos.Y)),
		float64(targetShape.Width),
		float64(targetShape.Height),
	)
	rowHeight := box.Height / float64(1+len(targetShape.SQLTable.Columns))
	headerBox := geo.NewBox(box.TopLeft, box.Width, rowHeight)

	fmt.Fprint(writer,
		tableHeader(targetShape, headerBox, targetShape.Label,
			float64(targetShape.LabelWidth), float64(targetShape.LabelHeight), float64(targetShape.FontSize)),
	)

	var longestNameWidth int
	for _, f := range targetShape.Columns {
		longestNameWidth = go2.Max(longestNameWidth, f.Label.LabelWidth)
	}

	rowBox := geo.NewBox(box.TopLeft.Copy(), box.Width, rowHeight)
	rowBox.TopLeft.Y += headerBox.Height
	for _, f := range targetShape.Columns {
		fmt.Fprint(writer,
			tableRow(targetShape, rowBox, f.Label.Label, f.Type.Label, f.ConstraintAbbr(), float64(targetShape.FontSize), float64(longestNameWidth)),
		)
		rowBox.TopLeft.Y += rowHeight
		fmt.Fprintf(writer, `<line x1="%f" y1="%f" x2="%f" y2="%f" style="stroke-width:2;stroke:%s" />`,
			rowBox.TopLeft.X, rowBox.TopLeft.Y,
			rowBox.TopLeft.X+rowBox.Width, rowBox.TopLeft.Y,
			targetShape.Fill,
		)
	}
}
