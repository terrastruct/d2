package d2svg

import (
	"fmt"
	"io"
	"strings"

	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/label"
	"oss.terrastruct.com/util-go/go2"
)

func tableHeader(box *geo.Box, text string, textWidth, textHeight, fontSize float64) string {
	str := fmt.Sprintf(`<rect class="class_header" x="%f" y="%f" width="%f" height="%f" fill="%s" />`,
		box.TopLeft.X, box.TopLeft.Y, box.Width, box.Height, "#0a0f25")

	if text != "" {
		tl := label.InsideMiddleLeft.GetPointOnBox(
			box,
			20,
			textWidth,
			textHeight,
		)

		str += fmt.Sprintf(`<text class="%s" x="%f" y="%f" style="%s">%s</text>`,
			"text",
			tl.X,
			tl.Y+textHeight*3/4,
			fmt.Sprintf("text-anchor:%s;font-size:%vpx;fill:%s",
				"start",
				4+fontSize,
				"white",
			),
			escapeText(text),
		)
	}
	return str
}

func tableRow(box *geo.Box, nameText, typeText, constraintText string, fontSize, longestNameWidth float64) string {
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

	// TODO theme based
	primaryColor := "rgb(13, 50, 178)"
	accentColor := "rgb(74, 111, 243)"
	neutralColor := "rgb(103, 108, 126)"

	return strings.Join([]string{
		fmt.Sprintf(`<text class="text" x="%f" y="%f" style="%s">%s</text>`,
			nameTL.X,
			nameTL.Y+fontSize*3/4,
			fmt.Sprintf("text-anchor:%s;font-size:%vpx;fill:%s", "start", fontSize, primaryColor),
			escapeText(nameText),
		),

		// TODO light font
		fmt.Sprintf(`<text class="text" x="%f" y="%f" style="%s">%s</text>`,
			nameTL.X+longestNameWidth+2*d2target.NamePadding,
			nameTL.Y+fontSize*3/4,
			fmt.Sprintf("text-anchor:%s;font-size:%vpx;fill:%s", "start", fontSize, neutralColor),
			escapeText(typeText),
		),

		fmt.Sprintf(`<text class="text" x="%f" y="%f" style="%s">%s</text>`,
			constraintTR.X,
			constraintTR.Y+fontSize*3/4,
			fmt.Sprintf("text-anchor:%s;font-size:%vpx;fill:%s;letter-spacing:2px;", "end", fontSize, accentColor),
			constraintText,
		),
	}, "\n")
}

func constraintAbbr(constraint string) string {
	switch constraint {
	case "primary_key":
		return "PK"
	case "foreign_key":
		return "FK"
	case "unique":
		return "UNQ"
	default:
		return ""
	}
}

func drawTable(writer io.Writer, targetShape d2target.Shape) {
	fmt.Fprintf(writer, `<rect class="shape" x="%d" y="%d" width="%d" height="%d" style="%s"/>`,
		targetShape.Pos.X, targetShape.Pos.Y, targetShape.Width, targetShape.Height, shapeStyle(targetShape))

	box := geo.NewBox(
		geo.NewPoint(float64(targetShape.Pos.X), float64(targetShape.Pos.Y)),
		float64(targetShape.Width),
		float64(targetShape.Height),
	)
	rowHeight := box.Height / float64(1+len(targetShape.SQLTable.Columns))
	headerBox := geo.NewBox(box.TopLeft, box.Width, rowHeight)

	fmt.Fprint(writer,
		tableHeader(headerBox, targetShape.Label, float64(targetShape.LabelWidth), float64(targetShape.LabelHeight), float64(targetShape.FontSize)),
	)

	var longestNameWidth int
	for _, f := range targetShape.SQLTable.Columns {
		longestNameWidth = go2.Max(longestNameWidth, f.Name.LabelWidth)
	}

	rowBox := geo.NewBox(box.TopLeft.Copy(), box.Width, rowHeight)
	rowBox.TopLeft.Y += headerBox.Height
	for _, f := range targetShape.SQLTable.Columns {
		fmt.Fprint(writer,
			tableRow(rowBox, f.Name.Label, f.Type.Label, constraintAbbr(f.Constraint), float64(targetShape.FontSize), float64(longestNameWidth)),
		)
		rowBox.TopLeft.Y += rowHeight
		fmt.Fprintf(writer, `<line x1="%f" y1="%f" x2="%f" y2="%f" style="stroke-width:2;stroke:#0a0f25" />`,
			rowBox.TopLeft.X, rowBox.TopLeft.Y,
			rowBox.TopLeft.X+rowBox.Width, rowBox.TopLeft.Y,
		)
	}
}
