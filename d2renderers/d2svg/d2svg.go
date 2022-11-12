// d2svg implements an SVG renderer for d2 diagrams.
// The input is d2exporter's output
package d2svg

import (
	"bytes"
	_ "embed"
	"encoding/xml"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"strings"

	"math"

	"github.com/alecthomas/chroma"
	"github.com/alecthomas/chroma/formatters"
	"github.com/alecthomas/chroma/lexers"
	"github.com/alecthomas/chroma/styles"

	"oss.terrastruct.com/d2/d2renderers/d2fonts"
	"oss.terrastruct.com/d2/d2renderers/textmeasure"
	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/color"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/go2"
	"oss.terrastruct.com/d2/lib/label"
	"oss.terrastruct.com/d2/lib/shape"
)

const (
	padding                    = 100
	MIN_ARROWHEAD_STROKE_WIDTH = 2
	threeDeeOffset             = 15
)

var multipleOffset = geo.NewVector(10, -10)

//go:embed style.css
var styleCSS string

//go:embed github-markdown.css
var mdCSS string

func setViewbox(writer io.Writer, diagram *d2target.Diagram) (width int, height int) {
	tl, br := diagram.BoundingBox()
	w := br.X - tl.X + padding*2
	h := br.Y - tl.Y + padding*2
	// TODO minify

	// TODO background stuff. e.g. dotted, grid, colors
	fmt.Fprintf(writer, `<?xml version="1.0" encoding="utf-8"?>
<svg
style="background: white;"
xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink"
width="%d" height="%d" viewBox="%d %d %d %d">`, w, h, tl.X-padding, tl.Y-padding, w, h)

	return w, h
}

func arrowheadMarkerID(isTarget bool, connection d2target.Connection) string {
	var arrowhead d2target.Arrowhead
	if isTarget {
		arrowhead = connection.DstArrow
	} else {
		arrowhead = connection.SrcArrow
	}

	return fmt.Sprintf("mk-%s", hash(fmt.Sprintf("%s,%t,%d,%s",
		arrowhead, isTarget, connection.StrokeWidth, connection.Stroke,
	)))
}

func arrowheadDimensions(arrowhead d2target.Arrowhead, strokeWidth float64) (width, height float64) {
	var widthMultiplier float64
	var heightMultiplier float64
	switch arrowhead {
	case d2target.ArrowArrowhead:
		widthMultiplier = 5
		heightMultiplier = 5
	case d2target.TriangleArrowhead:
		widthMultiplier = 5
		heightMultiplier = 6
	case d2target.LineArrowhead:
		widthMultiplier = 5
		heightMultiplier = 8
	case d2target.FilledDiamondArrowhead:
		widthMultiplier = 11
		heightMultiplier = 7
	case d2target.DiamondArrowhead:
		widthMultiplier = 11
		heightMultiplier = 9
	}

	clippedStrokeWidth := go2.Max(MIN_ARROWHEAD_STROKE_WIDTH, strokeWidth)
	return clippedStrokeWidth * widthMultiplier, clippedStrokeWidth * heightMultiplier
}

func arrowheadMarker(isTarget bool, id string, connection d2target.Connection) string {
	arrowhead := connection.DstArrow
	if !isTarget {
		arrowhead = connection.SrcArrow
	}
	strokeWidth := float64(connection.StrokeWidth)
	width, height := arrowheadDimensions(arrowhead, strokeWidth)

	var path string
	switch arrowhead {
	case d2target.ArrowArrowhead:
		attrs := fmt.Sprintf(`class="connection" fill="%s" stroke-width="%d"`, connection.Stroke, connection.StrokeWidth)
		if isTarget {
			path = fmt.Sprintf(`<polygon %s points="%f,%f %f,%f %f,%f %f,%f" />`,
				attrs,
				0., 0.,
				width, height/2,
				0., height,
				width/4, height/2,
			)
		} else {
			path = fmt.Sprintf(`<polygon %s points="%f,%f %f,%f %f,%f %f,%f" />`,
				attrs,
				0., height/2,
				width, 0.,
				width*3/4, height/2,
				width, height,
			)
		}
	case d2target.TriangleArrowhead:
		attrs := fmt.Sprintf(`class="connection" fill="%s" stroke-width="%d"`, connection.Stroke, connection.StrokeWidth)
		if isTarget {
			path = fmt.Sprintf(`<polygon %s points="%f,%f %f,%f %f,%f" />`,
				attrs,
				0., 0.,
				width, height/2.0,
				0., height,
			)
		} else {
			path = fmt.Sprintf(`<polygon %s points="%f,%f %f,%f %f,%f" />`,
				attrs,
				width, 0.,
				0., height/2.0,
				width, height,
			)
		}
	case d2target.LineArrowhead:
		attrs := fmt.Sprintf(`class="connection" fill="none" stroke="%s" stroke-width="%d"`, connection.Stroke, connection.StrokeWidth)
		if isTarget {
			path = fmt.Sprintf(`<polyline %s points="%f,%f %f,%f %f,%f"/>`,
				attrs,
				strokeWidth/2, strokeWidth/2,
				width-strokeWidth/2, height/2,
				strokeWidth/2, height-strokeWidth/2,
			)
		} else {
			path = fmt.Sprintf(`<polyline %s points="%f,%f %f,%f %f,%f"/>`,
				attrs,
				width-strokeWidth/2, strokeWidth/2,
				strokeWidth/2, height/2,
				width-strokeWidth/2, height-strokeWidth/2,
			)
		}
	case d2target.FilledDiamondArrowhead:
		attrs := fmt.Sprintf(`class="connection" fill="%s" stroke-width="%d"`, connection.Stroke, connection.StrokeWidth)
		if isTarget {
			path = fmt.Sprintf(`<polygon %s points="%f,%f %f,%f %f,%f %f,%f" />`,
				attrs,
				0., height/2.0,
				width/2.0, 0.,
				width, height/2.0,
				width/2.0, height,
			)
		} else {
			path = fmt.Sprintf(`<polygon %s points="%f,%f %f,%f %f,%f %f,%f" />`,
				attrs,
				0., height/2.0,
				width/2.0, 0.,
				width, height/2.0,
				width/2.0, height,
			)
		}
	case d2target.DiamondArrowhead:
		attrs := fmt.Sprintf(`class="connection" fill="white" stroke="%s" stroke-width="%d"`, connection.Stroke, connection.StrokeWidth)
		if isTarget {
			path = fmt.Sprintf(`<polygon %s points="%f,%f %f,%f %f,%f %f,%f" />`,
				attrs,
				0., height/2.0,
				width/2, height/8,
				width, height/2.0,
				width/2.0, height*0.9,
			)
		} else {
			path = fmt.Sprintf(`<polygon %s points="%f,%f %f,%f %f,%f %f,%f" />`,
				attrs,
				width/8, height/2.0,
				width*0.6, height/8,
				width*1.1, height/2.0,
				width*0.6, height*7/8,
			)
		}
	default:
		return ""
	}

	var refX float64
	refY := height / 2
	switch arrowhead {
	case d2target.DiamondArrowhead:
		if isTarget {
			refX = width - 0.6*strokeWidth
		} else {
			refX = width/8 + 0.6*strokeWidth
		}
		width *= 1.1
	default:
		if isTarget {
			refX = width - 1.5*strokeWidth
		} else {
			refX = 1.5 * strokeWidth
		}
	}

	return strings.Join([]string{
		fmt.Sprintf(`<marker id="%s" markerWidth="%f" markerHeight="%f" refX="%f" refY="%f"`,
			id, width, height, refX, refY,
		),
		fmt.Sprintf(`viewBox="%f %f %f %f"`, 0., 0., width, height),
		`orient="auto" markerUnits="userSpaceOnUse">`,
		path,
		"</marker>",
	}, " ")
}

// compute the (dx, dy) adjustment to apply to get the arrowhead-adjusted end point
func arrowheadAdjustment(start, end *geo.Point, arrowhead d2target.Arrowhead, edgeStrokeWidth, shapeStrokeWidth int) *geo.Point {
	distance := (float64(edgeStrokeWidth) + float64(shapeStrokeWidth)) / 2.0
	if arrowhead != d2target.NoArrowhead {
		distance += float64(edgeStrokeWidth)
	}

	v := geo.NewVector(end.X-start.X, end.Y-start.Y)
	return v.Unit().Multiply(-distance).ToPoint()
}

// returns the path's d attribute for the given connection
func pathData(connection d2target.Connection, idToShape map[string]d2target.Shape) string {
	var path []string
	route := connection.Route
	srcShape := idToShape[connection.Src]
	dstShape := idToShape[connection.Dst]

	sourceAdjustment := arrowheadAdjustment(route[0], route[1], connection.SrcArrow, connection.StrokeWidth, srcShape.StrokeWidth)
	path = append(path, fmt.Sprintf("M %f %f",
		route[0].X-sourceAdjustment.X,
		route[0].Y-sourceAdjustment.Y,
	))

	if connection.IsCurve {
		i := 1
		for ; i < len(route)-3; i += 3 {
			path = append(path, fmt.Sprintf("C %f %f %f %f %f %f",
				route[i].X, route[i].Y,
				route[i+1].X, route[i+1].Y,
				route[i+2].X, route[i+2].Y,
			))
		}
		// final curve target adjustment
		targetAdjustment := arrowheadAdjustment(route[i+1], route[i+2], connection.DstArrow, connection.StrokeWidth, dstShape.StrokeWidth)
		path = append(path, fmt.Sprintf("C %f %f %f %f %f %f",
			route[i].X, route[i].Y,
			route[i+1].X, route[i+1].Y,
			route[i+2].X+targetAdjustment.X,
			route[i+2].Y+targetAdjustment.Y,
		))
	} else {
		for i := 1; i < len(route)-1; i++ {
			prevSource := route[i-1]
			prevTarget := route[i]
			currTarget := route[i+1]
			prevVector := prevSource.VectorTo(prevTarget)
			currVector := prevTarget.VectorTo(currTarget)

			dist := geo.EuclideanDistance(prevTarget.X, prevTarget.Y, currTarget.X, currTarget.Y)
			units := math.Min(10, dist/2)

			prevTranslations := prevVector.Unit().Multiply(units).ToPoint()
			currTranslations := currVector.Unit().Multiply(units).ToPoint()

			path = append(path, fmt.Sprintf("L %f %f",
				prevTarget.X-prevTranslations.X,
				prevTarget.Y-prevTranslations.Y,
			))

			// If the segment length is too small, instead of drawing 2 arcs, just skip this segment and bezier curve to the next one
			if units < 10 && i < len(route)-2 {
				nextTarget := route[i+2]
				nextVector := geo.NewVector(nextTarget.X-currTarget.X, nextTarget.Y-currTarget.Y)
				i++
				nextTranslations := nextVector.Unit().Multiply(units).ToPoint()

				// These 2 bezier control points aren't just at the corner -- they are reflected at the corner, which causes the curve to be ~tangent to the corner,
				// which matches how the two arcs look
				path = append(path, fmt.Sprintf("C %f %f %f %f %f %f",
					// Control point
					prevTarget.X+prevTranslations.X,
					prevTarget.Y+prevTranslations.Y,
					// Control point
					currTarget.X-nextTranslations.X,
					currTarget.Y-nextTranslations.Y,
					// Where curve ends
					currTarget.X+nextTranslations.X,
					currTarget.Y+nextTranslations.Y,
				))
			} else {
				path = append(path, fmt.Sprintf("S %f %f %f %f",
					prevTarget.X,
					prevTarget.Y,
					prevTarget.X+currTranslations.X,
					prevTarget.Y+currTranslations.Y,
				))
			}
		}

		lastPoint := route[len(route)-1]
		secondToLastPoint := route[len(route)-2]

		targetAdjustment := arrowheadAdjustment(secondToLastPoint, lastPoint, connection.DstArrow, connection.StrokeWidth, dstShape.StrokeWidth)
		path = append(path, fmt.Sprintf("L %f %f",
			lastPoint.X+targetAdjustment.X,
			lastPoint.Y+targetAdjustment.Y,
		))
	}

	return strings.Join(path, " ")
}

func labelMask(id string, connection d2target.Connection, labelTL, tl, br *geo.Point) string {
	width := br.X - tl.X
	height := br.Y - tl.Y
	return strings.Join([]string{
		fmt.Sprintf(`<mask id="%s" maskUnits="userSpaceOnUse" x="%f" y="%f" width="%f" height="%f">`,
			id, tl.X, tl.Y, width, height,
		),
		fmt.Sprintf(`<rect x="%f" y="%f" width="%f" height="%f" fill="white"></rect>`,
			tl.X, tl.Y, width, height,
		),
		fmt.Sprintf(`<rect x="%f" y="%f" width="%d" height="%d" fill="black"></rect>`,
			labelTL.X, labelTL.Y,
			connection.LabelWidth,
			connection.LabelHeight,
		),
		`</mask>`,
	}, "\n")
}

func drawConnection(writer io.Writer, connection d2target.Connection, markers map[string]struct{}, idToShape map[string]d2target.Shape) {
	var markerStart string
	if connection.SrcArrow != d2target.NoArrowhead {
		id := arrowheadMarkerID(false, connection)
		if _, in := markers[id]; !in {
			marker := arrowheadMarker(false, id, connection)
			if marker == "" {
				panic(fmt.Sprintf("received empty arrow head marker for: %#v", connection))
			}
			fmt.Fprint(writer, marker)
			markers[id] = struct{}{}
		}
		markerStart = fmt.Sprintf(`marker-start="url(#%s)" `, id)
	}

	var markerEnd string
	if connection.DstArrow != d2target.NoArrowhead {
		id := arrowheadMarkerID(true, connection)
		if _, in := markers[id]; !in {
			marker := arrowheadMarker(true, id, connection)
			if marker == "" {
				panic(fmt.Sprintf("received empty arrow head marker for: %#v", connection))
			}
			fmt.Fprint(writer, marker)
			markers[id] = struct{}{}
		}
		markerEnd = fmt.Sprintf(`marker-end="url(#%s)" `, id)
	}

	var labelTL *geo.Point
	var mask string
	if connection.Label != "" {
		labelTL = connection.GetLabelTopLeft()
		labelTL.X = math.Round(labelTL.X)
		labelTL.Y = math.Round(labelTL.Y)

		if label.Position(connection.LabelPosition).IsOnEdge() {
			strokeWidth := float64(connection.StrokeWidth)
			tl, br := geo.Route(connection.Route).GetBoundingBox()
			tl.X -= strokeWidth
			tl.Y -= strokeWidth
			br.X += strokeWidth
			br.Y += strokeWidth
			if connection.SrcArrow != d2target.NoArrowhead {
				width, height := arrowheadDimensions(connection.SrcArrow, strokeWidth)
				tl.X -= width
				tl.Y -= height
				br.X += width
				br.Y += height
			}
			if connection.DstArrow != d2target.NoArrowhead {
				width, height := arrowheadDimensions(connection.DstArrow, strokeWidth)
				tl.X -= width
				tl.Y -= height
				br.X += width
				br.Y += height
			}

			tl.X = math.Min(tl.X, labelTL.X)
			tl.Y = math.Min(tl.Y, labelTL.Y)
			br.X = math.Max(br.X, labelTL.X+float64(connection.LabelWidth))
			br.Y = math.Max(br.Y, labelTL.Y+float64(connection.LabelHeight))

			maskID := fmt.Sprintf("mask-%s", hash(connection.ID))
			fmt.Fprint(writer, labelMask(maskID, connection, labelTL, tl, br))
			mask = fmt.Sprintf(`mask="url(#%s)" `, maskID)
		}
	}

	fmt.Fprintf(writer, `<path d="%s" class="connection" style="fill:none;%s" %s%s%s/>`,
		pathData(connection, idToShape),
		connectionStyle(connection),
		markerStart,
		markerEnd,
		mask,
	)

	if connection.Label != "" {
		fontClass := "text"
		if connection.Bold {
			fontClass += "-bold"
		} else if connection.Italic {
			fontClass += "-italic"
		}
		textStyle := fmt.Sprintf("text-anchor:%s;font-size:%vpx;fill:%s", "middle", connection.FontSize, "black")
		x := labelTL.X + float64(connection.LabelWidth)/2
		y := labelTL.Y + float64(connection.FontSize)
		fmt.Fprintf(writer, `<text class="%s" x="%f" y="%f" style="%s">%s</text>`,
			fontClass,
			x, y,
			textStyle,
			renderText(connection.Label, x, float64(connection.LabelHeight)),
		)
	}
}

func renderOval(tl *geo.Point, width, height float64, style string) string {
	rx := width / 2
	ry := height / 2
	cx := tl.X + rx
	cy := tl.Y + ry
	return fmt.Sprintf(`<ellipse class="shape" cx="%f" cy="%f" rx="%f" ry="%f" style="%s" />`, cx, cy, rx, ry, style)
}

func defineShadowFilter(writer io.Writer) {
	fmt.Fprint(writer, `<defs>
	<filter id="shadow-filter" width="200%" height="200%" x="-50%" y="-50%">
		<feGaussianBlur stdDeviation="1.7 " in="SourceGraphic"></feGaussianBlur>
		<feFlood flood-color="#3d4574" flood-opacity="0.4" result="ShadowFeFlood" in="SourceGraphic"></feFlood>
		<feComposite in="ShadowFeFlood" in2="SourceAlpha" operator="in" result="ShadowFeComposite"></feComposite>
		<feOffset dx="3" dy="5" result="ShadowFeOffset" in="ShadowFeComposite"></feOffset>
		<feBlend in="SourceGraphic" in2="ShadowFeOffset" mode="normal" result="ShadowFeBlend"></feBlend>
	</filter>
</defs>`)
}

func drawShape(writer io.Writer, targetShape d2target.Shape) error {
	tl := geo.NewPoint(float64(targetShape.Pos.X), float64(targetShape.Pos.Y))
	width := float64(targetShape.Width)
	height := float64(targetShape.Height)
	style := shapeStyle(targetShape)
	shapeType := d2target.DSL_SHAPE_TO_SHAPE_TYPE[targetShape.Type]

	s := shape.NewShape(shapeType, geo.NewBox(tl, width, height))

	var shadowAttr string
	if targetShape.Shadow {
		switch targetShape.Type {
		case d2target.ShapeText,
			d2target.ShapeCode,
			d2target.ShapeClass,
			d2target.ShapeSQLTable:
		default:
			shadowAttr = `filter="url(#shadow-filter)" `
		}
	}

	fmt.Fprintf(writer, `<g class="shape" %s>`, shadowAttr)

	var multipleTL *geo.Point
	if targetShape.Multiple {
		multipleTL = tl.AddVector(multipleOffset)
	}

	switch targetShape.Type {
	case d2target.ShapeClass:
		drawClass(writer, targetShape)
		fmt.Fprintf(writer, `</g>`)
		return nil
	case d2target.ShapeSQLTable:
		drawTable(writer, targetShape)
		fmt.Fprintf(writer, `</g>`)
		return nil
	case d2target.ShapeOval:
		if targetShape.Multiple {
			fmt.Fprint(writer, renderOval(multipleTL, width, height, style))
		}
		fmt.Fprint(writer, renderOval(tl, width, height, style))

	case d2target.ShapeImage:
		fmt.Fprintf(writer, `<image href="%s" x="%d" y="%d" width="%d" height="%d" style="%s" />`,
			targetShape.Icon.String(),
			targetShape.Pos.X, targetShape.Pos.Y, targetShape.Width, targetShape.Height, style)

	// TODO should standardize "" to rectangle
	case d2target.ShapeRectangle, "":
		if targetShape.ThreeDee {
			darkerColor, err := color.Darken(targetShape.Fill)
			if err != nil {
				darkerColor = targetShape.Fill
			}

			var topPolygonPoints []string
			for _, v := range []d2target.Point{
				{X: 0, Y: 0},
				{X: threeDeeOffset, Y: -1 * threeDeeOffset},
				{X: targetShape.Width + threeDeeOffset, Y: -1 * threeDeeOffset},
				{X: targetShape.Width, Y: 0},
				{X: 0, Y: 0},
			} {
				topPolygonPoints = append(topPolygonPoints,
					fmt.Sprintf("%d,%d ", v.X+targetShape.Pos.X, v.Y+targetShape.Pos.Y),
				)
			}
			fmt.Fprintf(writer, `<polygon points="%s" style="fill:%s;"/>`,
				strings.Join(topPolygonPoints, ""), darkerColor)

			var rightPolygonPoints []string
			for _, v := range []d2target.Point{
				{X: targetShape.Width, Y: 0},
				{X: targetShape.Width + threeDeeOffset, Y: -1 * threeDeeOffset},
				{X: targetShape.Width + threeDeeOffset, Y: targetShape.Height - threeDeeOffset},
				{X: targetShape.Width, Y: targetShape.Height},
			} {
				rightPolygonPoints = append(rightPolygonPoints,
					fmt.Sprintf("%d,%d ", v.X+targetShape.Pos.X, v.Y+targetShape.Pos.Y),
				)
			}
			fmt.Fprintf(writer, `<polygon points="%s" style="fill:%s;"/>`,
				strings.Join(rightPolygonPoints, ""), darkerColor)
		}
		if targetShape.Multiple {
			fmt.Fprintf(writer, `<rect x="%d" y="%d" width="%d" height="%d" style="%s" />`,
				targetShape.Pos.X+10, targetShape.Pos.Y-10, targetShape.Width, targetShape.Height, style)
		}
		fmt.Fprintf(writer, `<rect x="%d" y="%d" width="%d" height="%d" style="%s" />`,
			targetShape.Pos.X, targetShape.Pos.Y, targetShape.Width, targetShape.Height, style)

	case d2target.ShapeText, d2target.ShapeCode:
	default:
		if targetShape.Multiple {
			multiplePathData := shape.NewShape(shapeType, geo.NewBox(multipleTL, width, height)).GetSVGPathData()
			for _, pathData := range multiplePathData {
				fmt.Fprintf(writer, `<path d="%s" style="%s"/>`, pathData, style)
			}
		}

		for _, pathData := range s.GetSVGPathData() {
			fmt.Fprintf(writer, `<path d="%s" style="%s"/>`, pathData, style)
		}
	}

	fmt.Fprintf(writer, `</g>`)

	if targetShape.Icon != nil && targetShape.Type != d2target.ShapeImage {
		iconPosition := label.Position(targetShape.IconPosition)
		var box *geo.Box
		if iconPosition.IsOutside() {
			box = s.GetBox()
		} else {
			box = s.GetInnerBox()
		}
		iconSize := targetShape.GetIconSize(box)

		tl := iconPosition.GetPointOnBox(box, label.PADDING, float64(iconSize), float64(iconSize))

		fmt.Fprintf(writer, `<image href="%s" x="%f" y="%f" width="%d" height="%d" />`,
			targetShape.Icon.String(),
			tl.X,
			tl.Y,
			iconSize,
			iconSize,
		)
	}

	if targetShape.Label != "" {
		labelPosition := label.Position(targetShape.LabelPosition)
		var box *geo.Box
		if labelPosition.IsOutside() {
			box = s.GetBox()
		} else {
			box = s.GetInnerBox()
		}
		labelTL := labelPosition.GetPointOnBox(box, label.PADDING, float64(targetShape.LabelWidth), float64(targetShape.LabelHeight))

		fontClass := "text"
		if targetShape.Bold {
			fontClass += "-bold"
		} else if targetShape.Italic {
			fontClass += "-italic"
		}

		switch targetShape.Type {
		case d2target.ShapeCode:
			lexer := lexers.Get(targetShape.Language)
			if lexer == nil {
				return fmt.Errorf("code snippet lexer for %s not found", targetShape.Language)
			}
			style := styles.Get("github")
			if style == nil {
				return errors.New(`code snippet style "github" not found`)
			}
			formatter := formatters.Get("svg")
			if formatter == nil {
				return errors.New(`code snippet formatter "svg" not found`)
			}
			iterator, err := lexer.Tokenise(nil, targetShape.Label)
			if err != nil {
				return err
			}

			svgStyles := styleToSVG(style)
			containerStyle := fmt.Sprintf(`stroke: %s;fill:%s`, targetShape.Stroke, style.Get(chroma.Background).Background.String())

			fmt.Fprintf(writer, `<g transform="translate(%f %f)" style="opacity:%f">`, box.TopLeft.X, box.TopLeft.Y, targetShape.Opacity)
			fmt.Fprintf(writer, `<rect class="shape" width="%d" height="%d" style="%s" />`,
				targetShape.Width, targetShape.Height, containerStyle)
			// Padding
			fmt.Fprintf(writer, `<g transform="translate(6 6)">`)

			for index, tokens := range chroma.SplitTokensIntoLines(iterator.Tokens()) {
				// TODO mono font looks better with 1.2 em (use px equivalent), but textmeasure needs to account for it. Not obvious how that should be done
				fmt.Fprintf(writer, "<text class=\"text-mono\" x=\"0\" y=\"%fem\" xml:space=\"preserve\">", 1*float64(index+1))
				for _, token := range tokens {
					text := svgEscaper.Replace(token.String())
					attr := styleAttr(svgStyles, token.Type)
					if attr != "" {
						text = fmt.Sprintf("<tspan %s>%s</tspan>", attr, text)
					}
					fmt.Fprint(writer, text)
				}
				fmt.Fprint(writer, "</text>")
			}
			fmt.Fprintf(writer, "</g></g>")
		case d2target.ShapeText:
			render, err := textmeasure.RenderMarkdown(targetShape.Label)
			if err != nil {
				return err
			}
			fmt.Fprintf(writer, `<g><foreignObject requiredFeatures="http://www.w3.org/TR/SVG11/feature#Extensibility" x="%f" y="%f" width="%d" height="%d">`,
				box.TopLeft.X, box.TopLeft.Y, targetShape.Width, targetShape.Height,
			)
			// we need the self closing form in this svg/xhtml context
			render = strings.ReplaceAll(render, "<hr>", "<hr />")
			fmt.Fprintf(writer, `<div xmlns="http://www.w3.org/1999/xhtml" class="md">%v</div>`, render)
			fmt.Fprint(writer, `</foreignObject></g>`)
		default:
			textStyle := fmt.Sprintf("text-anchor:%s;font-size:%vpx;fill:%s", "middle", targetShape.FontSize, "black")
			x := labelTL.X + float64(targetShape.LabelWidth)/2
			// text is vertically positioned at its baseline which is at labelTL+FontSize
			y := labelTL.Y + float64(targetShape.FontSize)
			fmt.Fprintf(writer, `<text class="%s" x="%f" y="%f" style="%s">%s</text>`,
				fontClass,
				x, y,
				textStyle,
				renderText(targetShape.Label, x, float64(targetShape.LabelHeight)),
			)
		}
	}
	return nil
}

func escapeText(text string) string {
	buf := new(bytes.Buffer)
	_ = xml.EscapeText(buf, []byte(text))
	return buf.String()
}

func renderText(text string, x, height float64) string {
	if !strings.Contains(text, "\n") {
		return escapeText(text)
	}
	rendered := []string{}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		dy := height / float64(len(lines))
		if i == 0 {
			dy = 0
		}
		escaped := escapeText(line)
		if escaped == "" {
			// if there are multiple newlines in a row we still need text for the tspan to render
			escaped = " "
		}
		rendered = append(rendered, fmt.Sprintf(`<tspan x="%f" dy="%f">%s</tspan>`, x, dy, escaped))
	}
	return strings.Join(rendered, "")
}

func shapeStyle(shape d2target.Shape) string {
	out := ""

	out += fmt.Sprintf(`fill:%s;`, shape.Fill)
	out += fmt.Sprintf(`stroke:%s;`, shape.Stroke)
	out += fmt.Sprintf(`opacity:%f;`, shape.Opacity)
	out += fmt.Sprintf(`stroke-width:%d;`, shape.StrokeWidth)

	return out
}

func connectionStyle(connection d2target.Connection) string {
	out := ""

	out += fmt.Sprintf(`stroke:%s;`, connection.Stroke)
	out += fmt.Sprintf(`opacity:%f;`, connection.Opacity)
	out += fmt.Sprintf(`stroke-width:%d;`, connection.StrokeWidth)
	if connection.StrokeDash != 0 {
		dashSize, gapSize := getStrokeDashAttributes(float64(connection.StrokeWidth), connection.StrokeDash)
		out += fmt.Sprintf(`stroke-dasharray:%f,%f;`, dashSize, gapSize)
	}

	return out
}

func getStrokeDashAttributes(strokeWidth, dashGapSize float64) (float64, float64) {
	// as the stroke width gets thicker, the dash gap gets smaller
	scale := math.Log10(-0.6*strokeWidth+10.6)*0.5 + 0.5
	scaledDashSize := strokeWidth * dashGapSize
	scaledGapSize := scale * scaledDashSize
	return scaledDashSize, scaledGapSize
}

func embedFonts(buf *bytes.Buffer) {
	content := buf.String()
	buf.WriteString(`<style type="text/css"><![CDATA[`)

	triggers := []string{
		`class="text"`,
		`class="md"`,
	}

	for _, t := range triggers {
		if strings.Contains(content, t) {
			fmt.Fprintf(buf, `
.text {
	font-family: "font-regular";
}
@font-face {
	font-family: font-regular;
	src: url("%s");
}`,
				d2fonts.FontEncodings[d2fonts.SourceSansPro.Font(0, d2fonts.FONT_STYLE_REGULAR)])
			break
		}
	}

	triggers = []string{
		`class="text-bold"`,
		`<b>`,
		`<strong>`,
	}

	for _, t := range triggers {
		if strings.Contains(content, t) {
			fmt.Fprintf(buf, `
.text-bold {
	font-family: "font-bold";
}
@font-face {
	font-family: font-bold;
	src: url("%s");
}`,
				d2fonts.FontEncodings[d2fonts.SourceSansPro.Font(0, d2fonts.FONT_STYLE_BOLD)])
			break
		}
	}

	triggers = []string{
		`class="text-italic"`,
		`<em>`,
		`<dfn>`,
	}

	for _, t := range triggers {
		if strings.Contains(content, t) {
			fmt.Fprintf(buf, `
.text-italic {
	font-family: "font-italic";
}
@font-face {
	font-family: font-italic;
	src: url("%s");
}`,
				d2fonts.FontEncodings[d2fonts.SourceSansPro.Font(0, d2fonts.FONT_STYLE_ITALIC)])
			break
		}
	}

	triggers = []string{
		`class="text-mono"`,
		`<pre>`,
		`<code>`,
		`<kbd>`,
		`<samp>`,
	}

	for _, t := range triggers {
		if strings.Contains(content, t) {
			fmt.Fprintf(buf, `
.text-mono {
	font-family: "font-mono";
}
@font-face {
	font-family: font-mono;
	src: url("%s");
}`,
				d2fonts.FontEncodings[d2fonts.SourceCodePro.Font(0, d2fonts.FONT_STYLE_REGULAR)])
			break
		}
	}

	buf.WriteString(`]]></style>`)
}

// TODO minify output at end
func Render(diagram *d2target.Diagram) ([]byte, error) {
	buf := &bytes.Buffer{}
	_, _ = setViewbox(buf, diagram)

	buf.WriteString(fmt.Sprintf(`<style type="text/css">
<![CDATA[
%s
]]>
</style>`, styleCSS))

	hasMarkdown := false
	for _, s := range diagram.Shapes {
		if s.Label != "" && s.Type == d2target.ShapeText {
			hasMarkdown = true
			break
		}
	}
	if hasMarkdown {
		fmt.Fprintf(buf, `<style type="text/css">%s</style>`, mdCSS)
	}

	// only define shadow filter if a shape uses it
	for _, s := range diagram.Shapes {
		if s.Shadow {
			defineShadowFilter(buf)
			break
		}
	}

	// SVG has no notion of z-index. The z-index is effectively the order it's drawn.
	// So draw from the least nested to most nested
	idToShape := make(map[string]d2target.Shape)
	highest := 1
	for _, s := range diagram.Shapes {
		highest = go2.Max(highest, s.Level)
		idToShape[s.ID] = s
	}
	for i := 1; i <= highest; i++ {
		for _, s := range diagram.Shapes {
			if s.Level == i {
				err := drawShape(buf, s)
				if err != nil {
					return nil, err
				}
			}
		}
	}

	markers := map[string]struct{}{}
	for _, c := range diagram.Connections {
		drawConnection(buf, c, markers, idToShape)
	}

	embedFonts(buf)

	buf.WriteString(`</svg>`)
	return buf.Bytes(), nil
}

func hash(s string) string {
	const secret = "lalalas"
	h := fnv.New32a()
	h.Write([]byte(fmt.Sprintf("%s%s", s, secret)))
	return fmt.Sprint(h.Sum32())
}
