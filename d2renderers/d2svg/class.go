package d2svg

import (
	"fmt"
	"io"
	"strings"

	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/label"
	"oss.terrastruct.com/d2/lib/svg"
)

func classHeader(shape d2target.Shape, box *geo.Box, text string, textWidth, textHeight, fontSize float64) string {
	str := fmt.Sprintf(`<rect class="class_header" x="%f" y="%f" width="%f" height="%f" fill="%s" />`,
		box.TopLeft.X, box.TopLeft.Y, box.Width, box.Height, shape.Fill)

	if text != "" {
		tl := label.InsideMiddleCenter.GetPointOnBox(
			box,
			0,
			textWidth,
			textHeight,
		)

		str += fmt.Sprintf(`<text class="%s" x="%f" y="%f" style="%s">%s</text>`,
			"text-mono",
			tl.X+textWidth/2,
			tl.Y+textHeight*3/4,
			fmt.Sprintf(`text-anchor:%s;font-size:%vpx;fill:%s`,
				"middle",
				4+fontSize,
				shape.Stroke,
			),
			svg.EscapeText(text),
		)
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

	return strings.Join([]string{
		fmt.Sprintf(`<text class="text-mono" x="%f" y="%f" style="%s">%s</text>`,
			prefixTL.X,
			prefixTL.Y+fontSize*3/4,
			fmt.Sprintf("text-anchor:%s;font-size:%vpx;fill:%s", "start", fontSize, shape.PrimaryAccentColor),
			prefix,
		),

		fmt.Sprintf(`<text class="text-mono" x="%f" y="%f" style="%s">%s</text>`,
			prefixTL.X+d2target.PrefixWidth,
			prefixTL.Y+fontSize*3/4,
			fmt.Sprintf("text-anchor:%s;font-size:%vpx;fill:%s", "start", fontSize, shape.Fill),
			svg.EscapeText(nameText),
		),

		fmt.Sprintf(`<text class="text-mono" x="%f" y="%f" style="%s">%s</text>`,
			typeTR.X,
			typeTR.Y+fontSize*3/4,
			fmt.Sprintf("text-anchor:%s;font-size:%vpx;fill:%s", "end", fontSize, shape.SecondaryAccentColor),
			svg.EscapeText(typeText),
		),
	}, "\n")
}

func drawClass(writer io.Writer, targetShape d2target.Shape) {
	fmt.Fprintf(writer, `<rect class="shape" x="%d" y="%d" width="%d" height="%d" style="%s"/>`,
		targetShape.Pos.X, targetShape.Pos.Y, targetShape.Width, targetShape.Height, shapeStyle(targetShape))

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

	fmt.Fprintf(writer, `<line x1="%f" y1="%f" x2="%f" y2="%f" style="%s" />`,
		rowBox.TopLeft.X, rowBox.TopLeft.Y,
		rowBox.TopLeft.X+rowBox.Width, rowBox.TopLeft.Y,
		fmt.Sprintf("stroke-width:1;stroke:%v", targetShape.Fill))

	for _, m := range targetShape.Methods {
		fmt.Fprint(writer,
			classRow(targetShape, rowBox, m.VisibilityToken(), m.Name, m.Return, float64(targetShape.FontSize)),
		)
		rowBox.TopLeft.Y += rowHeight
	}
}
