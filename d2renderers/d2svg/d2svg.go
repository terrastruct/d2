// d2svg implements an SVG renderer for d2 diagrams.
// The input is d2exporter's output
package d2svg

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"hash/fnv"
	"html"
	"io"
	"sort"
	"strings"

	"math"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"

	"oss.terrastruct.com/d2/d2ast"
	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2renderers/d2fonts"
	"oss.terrastruct.com/d2/d2renderers/d2latex"
	"oss.terrastruct.com/d2/d2renderers/d2sketch"
	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/d2themes"
	"oss.terrastruct.com/d2/d2themes/d2themescatalog"
	"oss.terrastruct.com/d2/lib/color"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/jsrunner"
	"oss.terrastruct.com/d2/lib/label"
	"oss.terrastruct.com/d2/lib/shape"
	"oss.terrastruct.com/d2/lib/svg"
	"oss.terrastruct.com/d2/lib/textmeasure"
	"oss.terrastruct.com/d2/lib/version"
	"oss.terrastruct.com/util-go/go2"
)

const (
	DEFAULT_PADDING = 100

	appendixIconRadius = 16

	// Legend constants
	LEGEND_PADDING        = 20
	LEGEND_ITEM_SPACING   = 15
	LEGEND_ICON_SIZE      = 24
	LEGEND_FONT_SIZE      = 14
	LEGEND_CORNER_PADDING = 10
)

var multipleOffset = geo.NewVector(d2target.MULTIPLE_OFFSET, -d2target.MULTIPLE_OFFSET)

//go:embed tooltip.svg
var TooltipIcon string

//go:embed link.svg
var LinkIcon string

//go:embed style.css
var BaseStylesheet string

//go:embed github-markdown.css
var MarkdownCSS string

//go:embed dots.txt
var dots string

//go:embed lines.txt
var lines string

//go:embed grain.txt
var grain string

//go:embed paper.txt
var paper string

type RenderOpts struct {
	Pad                *int64
	Sketch             *bool
	Center             *bool
	ThemeID            *int64
	DarkThemeID        *int64
	ThemeOverrides     *d2target.ThemeOverrides
	DarkThemeOverrides *d2target.ThemeOverrides
	Font               string
	// the svg will be scaled by this factor, if unset the svg will fit to screen
	Scale *float64

	// MasterID is passed when the diagram should use something other than its own hash for unique targeting
	// Currently, that's when multi-boards are collapsed
	MasterID    string
	NoXMLTag    *bool
	Salt        *string
	OmitVersion *bool
}

func dimensions(diagram *d2target.Diagram, pad int) (left, top, width, height int) {
	tl, br := diagram.BoundingBox()
	left = tl.X - pad
	top = tl.Y - pad
	width = br.X - tl.X + pad*2
	height = br.Y - tl.Y + pad*2

	return left, top, width, height
}

func RenderLegend(buf *bytes.Buffer, diagram *d2target.Diagram, diagramHash string, theme *d2themes.Theme) error {
	if diagram.Legend == nil || (len(diagram.Legend.Shapes) == 0 && len(diagram.Legend.Connections) == 0) {
		return nil
	}

	_, br := diagram.BoundingBox()

	ruler, err := textmeasure.NewRuler()
	if err != nil {
		return err
	}

	totalHeight := LEGEND_PADDING + LEGEND_FONT_SIZE + LEGEND_ITEM_SPACING
	maxLabelWidth := 0

	itemCount := 0

	for _, s := range diagram.Legend.Shapes {
		if s.Label == "" {
			continue
		}

		mtext := &d2target.MText{
			Text:     s.Label,
			FontSize: LEGEND_FONT_SIZE,
		}

		dims := d2graph.GetTextDimensions(nil, ruler, mtext, nil)
		maxLabelWidth = go2.IntMax(maxLabelWidth, dims.Width)
		totalHeight += go2.IntMax(dims.Height, LEGEND_ICON_SIZE) + LEGEND_ITEM_SPACING
		itemCount++
	}

	for _, c := range diagram.Legend.Connections {
		if c.Label == "" {
			continue
		}

		mtext := &d2target.MText{
			Text:     c.Label,
			FontSize: LEGEND_FONT_SIZE,
		}

		dims := d2graph.GetTextDimensions(nil, ruler, mtext, nil)
		maxLabelWidth = go2.IntMax(maxLabelWidth, dims.Width)
		totalHeight += go2.IntMax(dims.Height, LEGEND_ICON_SIZE) + LEGEND_ITEM_SPACING
		itemCount++
	}

	if itemCount > 0 {
		totalHeight -= LEGEND_ITEM_SPACING / 2
	}

	if itemCount > 0 && len(diagram.Legend.Connections) > 0 {
		totalHeight += LEGEND_PADDING * 1.5
	} else {
		totalHeight += LEGEND_PADDING * 1.2
	}

	legendWidth := LEGEND_PADDING*2 + LEGEND_ICON_SIZE + LEGEND_PADDING + maxLabelWidth
	legendX := br.X + LEGEND_CORNER_PADDING
	tl, _ := diagram.BoundingBox()
	legendY := br.Y - totalHeight
	if legendY < tl.Y {
		legendY = tl.Y
	}

	shadowEl := d2themes.NewThemableElement("rect", theme)
	shadowEl.Fill = "#F7F7FA"
	shadowEl.Stroke = "#DEE1EB"
	shadowEl.Style = "stroke-width: 1px; filter: drop-shadow(0px 2px 3px rgba(0, 0, 0, 0.1))"
	shadowEl.X = float64(legendX)
	shadowEl.Y = float64(legendY)
	shadowEl.Width = float64(legendWidth)
	shadowEl.Height = float64(totalHeight)
	shadowEl.Rx = 4
	fmt.Fprint(buf, shadowEl.Render())

	legendEl := d2themes.NewThemableElement("rect", theme)
	legendEl.Fill = "#ffffff"
	legendEl.Stroke = "#DEE1EB"
	legendEl.Style = "stroke-width: 1px"
	legendEl.X = float64(legendX)
	legendEl.Y = float64(legendY)
	legendEl.Width = float64(legendWidth)
	legendEl.Height = float64(totalHeight)
	legendEl.Rx = 4
	fmt.Fprint(buf, legendEl.Render())

	legendLabel := "Legend"
	if diagram.Legend.Label != "" {
		legendLabel = diagram.Legend.Label
	}
	fmt.Fprintf(buf, `<text class="text-bold" x="%d" y="%d" style="font-size: %dpx;">%s</text>`,
		legendX+LEGEND_PADDING, legendY+LEGEND_PADDING+LEGEND_FONT_SIZE, LEGEND_FONT_SIZE+2, html.EscapeString(legendLabel))

	currentY := legendY + LEGEND_PADDING*2 + LEGEND_FONT_SIZE

	shapeCount := 0
	for _, s := range diagram.Legend.Shapes {
		if s.Label == "" {
			continue
		}

		iconX := legendX + LEGEND_PADDING
		iconY := currentY

		shapeIcon, err := renderLegendShapeIcon(s, iconX, iconY, diagramHash, theme)
		if err != nil {
			return err
		}
		fmt.Fprint(buf, shapeIcon)

		mtext := &d2target.MText{
			Text:     s.Label,
			FontSize: LEGEND_FONT_SIZE,
		}

		dims := d2graph.GetTextDimensions(nil, ruler, mtext, nil)

		rowHeight := go2.IntMax(dims.Height, LEGEND_ICON_SIZE)
		textY := currentY + rowHeight/2 + int(float64(dims.Height)*0.3)

		fmt.Fprintf(buf, `<text class="text" x="%d" y="%d" style="font-size: %dpx;">%s</text>`,
			iconX+LEGEND_ICON_SIZE+LEGEND_PADDING, textY, LEGEND_FONT_SIZE,
			html.EscapeString(s.Label))

		currentY += rowHeight + LEGEND_ITEM_SPACING
		shapeCount++
	}

	if shapeCount > 0 && len(diagram.Legend.Connections) > 0 {
		currentY += LEGEND_ITEM_SPACING / 2

		separatorEl := d2themes.NewThemableElement("line", theme)
		separatorEl.X1 = float64(legendX + LEGEND_PADDING)
		separatorEl.Y1 = float64(currentY)
		separatorEl.X2 = float64(legendX + legendWidth - LEGEND_PADDING)
		separatorEl.Y2 = float64(currentY)
		separatorEl.Stroke = "#DEE1EB"
		separatorEl.StrokeDashArray = "2,2"
		fmt.Fprint(buf, separatorEl.Render())

		currentY += LEGEND_ITEM_SPACING
	}

	for _, c := range diagram.Legend.Connections {
		if c.Label == "" {
			continue
		}

		iconX := legendX + LEGEND_PADDING
		iconY := currentY + LEGEND_ICON_SIZE/2

		connIcon, err := renderLegendConnectionIcon(c, iconX, iconY, theme)
		if err != nil {
			return err
		}
		fmt.Fprint(buf, connIcon)

		mtext := &d2target.MText{
			Text:     c.Label,
			FontSize: LEGEND_FONT_SIZE,
		}

		dims := d2graph.GetTextDimensions(nil, ruler, mtext, nil)

		rowHeight := go2.IntMax(dims.Height, LEGEND_ICON_SIZE)
		textY := currentY + rowHeight/2 + int(float64(dims.Height)*0.2)

		fmt.Fprintf(buf, `<text class="text" x="%d" y="%d" style="font-size: %dpx;">%s</text>`,
			iconX+LEGEND_ICON_SIZE+LEGEND_PADDING, textY, LEGEND_FONT_SIZE,
			html.EscapeString(c.Label))

		currentY += rowHeight + LEGEND_ITEM_SPACING
	}

	if shapeCount > 0 && len(diagram.Legend.Connections) > 0 {
		currentY += LEGEND_PADDING / 2
	} else {
		currentY += LEGEND_PADDING / 4
	}

	return nil
}

func renderLegendShapeIcon(s d2target.Shape, x, y int, diagramHash string, theme *d2themes.Theme) (string, error) {
	iconShape := s
	const sizeFactor = 5
	iconShape.Pos.X = 0
	iconShape.Pos.Y = 0
	iconShape.Width = LEGEND_ICON_SIZE * sizeFactor
	iconShape.Height = LEGEND_ICON_SIZE * sizeFactor
	iconShape.Label = ""
	buf := &bytes.Buffer{}
	appendixBuf := &bytes.Buffer{}
	finalBuf := &bytes.Buffer{}
	fmt.Fprintf(finalBuf, `<g transform="translate(%d, %d) scale(%f)">`,
		x, y, 1.0/sizeFactor)
	_, err := drawShape(buf, appendixBuf, diagramHash, iconShape, nil, theme)
	if err != nil {
		return "", err
	}

	fmt.Fprint(finalBuf, buf.String())

	fmt.Fprint(finalBuf, `</g>`)

	return finalBuf.String(), nil
}

func renderLegendConnectionIcon(c d2target.Connection, x, y int, theme *d2themes.Theme) (string, error) {
	finalBuf := &bytes.Buffer{}

	buf := &bytes.Buffer{}

	const sizeFactor = 2

	legendConn := *d2target.BaseConnection()

	legendConn.ID = c.ID
	legendConn.SrcArrow = c.SrcArrow
	legendConn.DstArrow = c.DstArrow
	legendConn.StrokeDash = c.StrokeDash
	legendConn.StrokeWidth = c.StrokeWidth
	legendConn.Stroke = c.Stroke
	legendConn.Fill = c.Fill
	legendConn.BorderRadius = c.BorderRadius
	legendConn.Opacity = c.Opacity
	legendConn.Animated = c.Animated

	startX := 0.0
	midY := 0.0
	width := float64(LEGEND_ICON_SIZE * sizeFactor)

	legendConn.Route = []*geo.Point{
		{X: startX, Y: midY},
		{X: startX + width, Y: midY},
	}

	legendHash := fmt.Sprintf("legend-%s", hash(fmt.Sprintf("%s-%d-%d", c.ID, x, y)))

	markers := make(map[string]struct{})
	idToShape := make(map[string]d2target.Shape)

	fmt.Fprintf(finalBuf, `<g transform="translate(%d, %d) scale(%f)">`,
		x, y, 1.0/sizeFactor)

	_, err := drawConnection(buf, legendHash, legendConn, markers, idToShape, nil, theme)
	if err != nil {
		return "", err
	}

	fmt.Fprint(finalBuf, buf.String())

	fmt.Fprint(finalBuf, `</g>`)

	return finalBuf.String(), nil
}

func arrowheadMarkerID(diagramHash string, isTarget bool, connection d2target.Connection) string {
	var arrowhead d2target.Arrowhead
	if isTarget {
		arrowhead = connection.DstArrow
	} else {
		arrowhead = connection.SrcArrow
	}

	return fmt.Sprintf("mk-%s-%s", diagramHash, hash(fmt.Sprintf("%s,%t,%d,%s",
		arrowhead, isTarget, connection.StrokeWidth, connection.Stroke,
	)))
}

func arrowheadMarker(isTarget bool, id string, connection d2target.Connection, inlineTheme *d2themes.Theme) string {
	arrowhead := connection.DstArrow
	if !isTarget {
		arrowhead = connection.SrcArrow
	}
	strokeWidth := float64(connection.StrokeWidth)
	width, height := arrowhead.Dimensions(strokeWidth)

	var path string
	switch arrowhead {
	case d2target.ArrowArrowhead:
		polygonEl := d2themes.NewThemableElement("polygon", inlineTheme)
		polygonEl.Fill = connection.Stroke
		polygonEl.ClassName = "connection"
		polygonEl.Attributes = fmt.Sprintf(`stroke-width="%d"`, connection.StrokeWidth)

		if isTarget {
			polygonEl.Points = fmt.Sprintf("%f,%f %f,%f %f,%f %f,%f",
				0., 0.,
				width, height/2,
				0., height,
				width/4, height/2,
			)
		} else {
			polygonEl.Points = fmt.Sprintf("%f,%f %f,%f %f,%f %f,%f",
				0., height/2,
				width, 0.,
				width*3/4, height/2,
				width, height,
			)
		}
		path = polygonEl.Render()
	case d2target.UnfilledTriangleArrowhead:
		polygonEl := d2themes.NewThemableElement("polygon", inlineTheme)
		polygonEl.Fill = d2target.BG_COLOR
		polygonEl.Stroke = connection.Stroke
		polygonEl.ClassName = "connection"
		polygonEl.Attributes = fmt.Sprintf(`stroke-width="%d"`, connection.StrokeWidth)

		inset := strokeWidth / 2
		if isTarget {
			polygonEl.Points = fmt.Sprintf("%f,%f %f,%f %f,%f",
				inset, inset,
				width-inset, height/2.0,
				inset, height-inset,
			)
		} else {
			polygonEl.Points = fmt.Sprintf("%f,%f %f,%f %f,%f",
				width-inset, inset,
				inset, height/2.0,
				width-inset, height-inset,
			)
		}
		path = polygonEl.Render()

	case d2target.TriangleArrowhead:
		polygonEl := d2themes.NewThemableElement("polygon", inlineTheme)
		polygonEl.Fill = connection.Stroke
		polygonEl.ClassName = "connection"
		polygonEl.Attributes = fmt.Sprintf(`stroke-width="%d"`, connection.StrokeWidth)

		if isTarget {
			polygonEl.Points = fmt.Sprintf("%f,%f %f,%f %f,%f",
				0., 0.,
				width, height/2.0,
				0., height,
			)
		} else {
			polygonEl.Points = fmt.Sprintf("%f,%f %f,%f %f,%f",
				width, 0.,
				0., height/2.0,
				width, height,
			)
		}
		path = polygonEl.Render()
	case d2target.LineArrowhead:
		polylineEl := d2themes.NewThemableElement("polyline", inlineTheme)
		polylineEl.Fill = color.None
		polylineEl.ClassName = "connection"
		polylineEl.Stroke = connection.Stroke
		polylineEl.Attributes = fmt.Sprintf(`stroke-width="%d"`, connection.StrokeWidth)

		if isTarget {
			polylineEl.Points = fmt.Sprintf("%f,%f %f,%f %f,%f",
				strokeWidth/2, strokeWidth/2,
				width-strokeWidth/2, height/2,
				strokeWidth/2, height-strokeWidth/2,
			)
		} else {
			polylineEl.Points = fmt.Sprintf("%f,%f %f,%f %f,%f",
				width-strokeWidth/2, strokeWidth/2,
				strokeWidth/2, height/2,
				width-strokeWidth/2, height-strokeWidth/2,
			)
		}
		path = polylineEl.Render()
	case d2target.FilledDiamondArrowhead:
		polygonEl := d2themes.NewThemableElement("polygon", inlineTheme)
		polygonEl.ClassName = "connection"
		polygonEl.Fill = connection.Stroke
		polygonEl.Attributes = fmt.Sprintf(`stroke-width="%d"`, connection.StrokeWidth)

		if isTarget {
			polygonEl.Points = fmt.Sprintf("%f,%f %f,%f %f,%f %f,%f",
				0., height/2.0,
				width/2.0, 0.,
				width, height/2.0,
				width/2.0, height,
			)
		} else {
			polygonEl.Points = fmt.Sprintf("%f,%f %f,%f %f,%f %f,%f",
				0., height/2.0,
				width/2.0, 0.,
				width, height/2.0,
				width/2.0, height,
			)
		}
		path = polygonEl.Render()
	case d2target.DiamondArrowhead:
		polygonEl := d2themes.NewThemableElement("polygon", inlineTheme)
		polygonEl.ClassName = "connection"
		polygonEl.Fill = d2target.BG_COLOR
		polygonEl.Stroke = connection.Stroke
		polygonEl.Attributes = fmt.Sprintf(`stroke-width="%d"`, connection.StrokeWidth)

		if isTarget {
			polygonEl.Points = fmt.Sprintf("%f,%f %f,%f %f,%f %f,%f",
				0., height/2.0,
				width/2, height/8,
				width, height/2.0,
				width/2.0, height*0.9,
			)
		} else {
			polygonEl.Points = fmt.Sprintf("%f,%f %f,%f %f,%f %f,%f",
				width/8, height/2.0,
				width*0.6, height/8,
				width*1.1, height/2.0,
				width*0.6, height*7/8,
			)
		}
		path = polygonEl.Render()
	case d2target.FilledCircleArrowhead:
		radius := width / 2

		circleEl := d2themes.NewThemableElement("circle", inlineTheme)
		circleEl.Cy = radius
		circleEl.R = radius - strokeWidth/2
		circleEl.Fill = connection.Stroke
		circleEl.ClassName = "connection"
		circleEl.Attributes = fmt.Sprintf(`stroke-width="%d"`, connection.StrokeWidth)

		if isTarget {
			circleEl.Cx = radius + strokeWidth/2
		} else {
			circleEl.Cx = radius - strokeWidth/2
		}

		path = circleEl.Render()
	case d2target.CircleArrowhead:
		radius := width / 2

		circleEl := d2themes.NewThemableElement("circle", inlineTheme)
		circleEl.Cy = radius
		circleEl.R = radius - strokeWidth
		circleEl.Fill = d2target.BG_COLOR
		circleEl.Stroke = connection.Stroke
		circleEl.Attributes = fmt.Sprintf(`stroke-width="%d"`, connection.StrokeWidth)

		if isTarget {
			circleEl.Cx = radius + strokeWidth/2
		} else {
			circleEl.Cx = radius - strokeWidth/2
		}

		path = circleEl.Render()
	case d2target.CrossArrowhead:
		inset := strokeWidth / 8
		rotationAngle := math.Pi / 4
		origin := geo.NewPoint(width/2, height/2)
		newOrigin := geo.NewPoint(math.Cos(rotationAngle)*origin.X-math.Sin(rotationAngle)*origin.Y, math.Sin(rotationAngle)*origin.X+math.Cos(rotationAngle)*origin.Y)

		crossEl := d2themes.NewThemableElement("polygon", inlineTheme)
		crossEl.Points = fmt.Sprintf("%f,%f %f,%f %f,%f %f,%f, %f,%f %f,%f %f,%f %f,%f %f,%f %f,%f %f,%f %f,%f",
			0., height/2+inset,
			width/2-inset, height/2+inset,
			width/2-inset, height,
			width/2+inset, height,
			width/2+inset, height/2+inset,
			width, height/2+inset,
			width, height/2-inset,
			width/2+inset, height/2-inset,
			width/2+inset, 0.,
			width/2-inset, 0.,
			width/2-inset, height/2-inset,
			0., height/2-inset,
		)
		crossEl.Transform = fmt.Sprintf("translate(%f, %f) rotate(45)", -newOrigin.X+width/2, -newOrigin.Y+height/2)

		childPathEl := d2themes.NewThemableElement("path", inlineTheme)
		if isTarget {
			childPathEl.D = fmt.Sprintf("M%f,%f %f,%f",
				width/2, height/2,
				width, height/2,
			)
		} else {
			childPathEl.D = fmt.Sprintf("M%f,%f %f,%f",
				width/2, height/2,
				0., height/2,
			)
		}

		gEl := d2themes.NewThemableElement("g", inlineTheme)
		gEl.Fill = d2target.BG_COLOR
		gEl.Stroke = connection.Stroke
		gEl.ClassName = "connection"
		gEl.Attributes = fmt.Sprintf(`stroke-width="%d"`, connection.StrokeWidth)
		gEl.Content = fmt.Sprintf("%s%s",
			crossEl.Render(), childPathEl.Render(),
		)
		path = gEl.Render()
	case d2target.FilledBoxArrowhead:
		polygonEl := d2themes.NewThemableElement("polygon", inlineTheme)
		polygonEl.ClassName = "connection"
		polygonEl.Fill = connection.Stroke
		polygonEl.Attributes = fmt.Sprintf(`stroke-width="%d"`, connection.StrokeWidth)

		if isTarget {
			polygonEl.Points = fmt.Sprintf("%f,%f %f,%f %f,%f %f,%f",
				0., 0.,
				0., height,
				width, height,
				width, 0.,
			)
		} else {
			polygonEl.Points = fmt.Sprintf("%f,%f %f,%f %f,%f %f,%f",
				0., 0.,
				0., height,
				width, height,
				width, 0.,
			)
		}

		path = polygonEl.Render()
	case d2target.BoxArrowhead:
		polygonEl := d2themes.NewThemableElement("polygon", inlineTheme)
		polygonEl.ClassName = "connection"
		polygonEl.Fill = d2target.BG_COLOR
		polygonEl.Stroke = connection.Stroke
		polygonEl.Attributes = fmt.Sprintf(`stroke-width="%d"`, connection.StrokeWidth)
		polygonEl.Style = fmt.Sprintf("%sstroke-linejoin:miter;", polygonEl.Style)

		inset := strokeWidth / 2
		if isTarget {
			polygonEl.Points = fmt.Sprintf("%f,%f %f,%f %f,%f %f,%f",
				inset, inset,
				inset, height-inset,
				width-inset, height-inset,
				width-inset, inset,
			)
		} else {
			polygonEl.Points = fmt.Sprintf("%f,%f %f,%f %f,%f %f,%f",
				inset, inset,
				inset, height-inset,
				width-inset, height-inset,
				width-inset, inset,
			)
		}
		path = polygonEl.Render()
	case d2target.CfOne, d2target.CfMany, d2target.CfOneRequired, d2target.CfManyRequired:
		offset := 3.0 + float64(connection.StrokeWidth)*1.8

		var modifierEl *d2themes.ThemableElement
		if arrowhead == d2target.CfOneRequired || arrowhead == d2target.CfManyRequired {
			modifierEl = d2themes.NewThemableElement("path", inlineTheme)
			modifierEl.D = fmt.Sprintf("M%f,%f %f,%f",
				offset, 0.,
				offset, height,
			)
			modifierEl.Fill = d2target.BG_COLOR
			modifierEl.Stroke = connection.Stroke
			modifierEl.ClassName = "connection"
			modifierEl.Attributes = fmt.Sprintf(`stroke-width="%d"`, connection.StrokeWidth)
		} else {
			modifierEl = d2themes.NewThemableElement("circle", inlineTheme)
			modifierEl.Cx = offset/2.0 + 2.0
			modifierEl.Cy = height / 2.0
			modifierEl.R = offset / 2.0
			modifierEl.Fill = d2target.BG_COLOR
			modifierEl.Stroke = connection.Stroke
			modifierEl.ClassName = "connection"
			modifierEl.Attributes = fmt.Sprintf(`stroke-width="%d"`, connection.StrokeWidth)
		}

		childPathEl := d2themes.NewThemableElement("path", inlineTheme)
		if arrowhead == d2target.CfMany || arrowhead == d2target.CfManyRequired {
			childPathEl.D = fmt.Sprintf("M%f,%f %f,%f M%f,%f %f,%f M%f,%f %f,%f",
				width-3.0, height/2.0,
				width+offset, height/2.0,
				offset+3.0, height/2.0,
				width+offset, 0.,
				offset+3.0, height/2.0,
				width+offset, height,
			)
		} else {
			childPathEl.D = fmt.Sprintf("M%f,%f %f,%f M%f,%f %f,%f",
				width-3.0, height/2.0,
				width+offset, height/2.0,
				offset*2.0, 0.,
				offset*2.0, height,
			)
		}

		gEl := d2themes.NewThemableElement("g", inlineTheme)
		if !isTarget {
			gEl.Transform = fmt.Sprintf("scale(-1) translate(-%f, -%f)", width, height)
		}
		gEl.Fill = d2target.BG_COLOR
		gEl.Stroke = connection.Stroke
		gEl.ClassName = "connection"
		gEl.Attributes = fmt.Sprintf(`stroke-width="%d"`, connection.StrokeWidth)
		gEl.Content = fmt.Sprintf("%s%s",
			modifierEl.Render(), childPathEl.Render(),
		)
		path = gEl.Render()
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

func getArrowheadAdjustments(connection d2target.Connection, idToShape map[string]d2target.Shape) (srcAdj, dstAdj *geo.Point) {
	route := connection.Route
	srcShape := idToShape[connection.Src]
	dstShape := idToShape[connection.Dst]

	sourceAdjustment := arrowheadAdjustment(route[1], route[0], connection.SrcArrow, connection.StrokeWidth, srcShape.StrokeWidth)

	targetAdjustment := arrowheadAdjustment(route[len(route)-2], route[len(route)-1], connection.DstArrow, connection.StrokeWidth, dstShape.StrokeWidth)
	return sourceAdjustment, targetAdjustment
}

// returns the path's d attribute for the given connection
func pathData(connection d2target.Connection, srcAdj, dstAdj *geo.Point) string {
	var path []string
	route := connection.Route

	path = append(path, fmt.Sprintf("M %f %f",
		route[0].X+srcAdj.X,
		route[0].Y+srcAdj.Y,
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
		path = append(path, fmt.Sprintf("C %f %f %f %f %f %f",
			route[i].X, route[i].Y,
			route[i+1].X, route[i+1].Y,
			route[i+2].X+dstAdj.X,
			route[i+2].Y+dstAdj.Y,
		))
	} else {
		for i := 1; i < len(route)-1; i++ {
			prevSource := route[i-1]
			prevTarget := route[i]
			currTarget := route[i+1]
			prevVector := prevSource.VectorTo(prevTarget)
			currVector := prevTarget.VectorTo(currTarget)

			dist := geo.EuclideanDistance(prevTarget.X, prevTarget.Y, currTarget.X, currTarget.Y)

			connectionBorderRadius := connection.BorderRadius
			units := math.Min(connectionBorderRadius, dist/2)

			prevTranslations := prevVector.Unit().Multiply(units).ToPoint()
			currTranslations := currVector.Unit().Multiply(units).ToPoint()

			path = append(path, fmt.Sprintf("L %f %f",
				prevTarget.X-prevTranslations.X,
				prevTarget.Y-prevTranslations.Y,
			))

			// If the segment length is too small, instead of drawing 2 arcs, just skip this segment and bezier curve to the next one
			if units < connectionBorderRadius && i < len(route)-2 {
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
		path = append(path, fmt.Sprintf("L %f %f",
			lastPoint.X+dstAdj.X,
			lastPoint.Y+dstAdj.Y,
		))
	}

	return strings.Join(path, " ")
}

func makeLabelMask(labelTL *geo.Point, width, height int, opacity float64) string {
	return makeLabelMaskWithTransform(labelTL, width, height, opacity, nil)
}

func makeLabelMaskWithTransform(labelTL *geo.Point, width, height int, opacity float64, shapePos *geo.Point) string {
	fill := "black"
	if opacity != 1 {
		fill = fmt.Sprintf("rgba(0,0,0,%.2f)", opacity)
	}

	x := labelTL.X - 2
	y := labelTL.Y

	// In sketch mode, shapes use transform="translate(shapePos.X, shapePos.Y)"
	// so we need to adjust mask coordinates to account for this transform
	if shapePos != nil {
		x -= shapePos.X
		y -= shapePos.Y
	}

	return fmt.Sprintf(`<rect x="%f" y="%f" width="%d" height="%d" fill="%s"></rect>`,
		x, y,
		width+4,
		height,
		fill,
	)
}

// border label mask is minimal, intending to only cover the portion of the border that passes through the label
// otherwise, when the inside background and outside background colors don't match, there's a bigger discrepancy
func makeBorderLabelMask(labelPosition label.Position, labelTL *geo.Point, labelWidth, labelHeight int, shapeBox *geo.Box, strokeWidth int, opacity float64, shapePos *geo.Point) string {
	fill := "black"
	if opacity != 1 {
		fill = fmt.Sprintf("rgba(0,0,0,%.2f)", opacity)
	}

	// In sketch mode, multiply stroke width by 3 for mask calculations to account for
	// the hand-drawn style that can be wider and more irregular
	effectiveStrokeWidth := float64(strokeWidth)
	if shapePos != nil {
		effectiveStrokeWidth *= 3
	}

	var maskX, maskY, maskWidth, maskHeight float64

	switch labelPosition {
	case label.BorderTopLeft, label.BorderTopCenter, label.BorderTopRight:
		maskX = labelTL.X - 2
		maskY = shapeBox.TopLeft.Y - effectiveStrokeWidth/2
		maskWidth = float64(labelWidth + 4)
		maskHeight = effectiveStrokeWidth

	case label.BorderBottomLeft, label.BorderBottomCenter, label.BorderBottomRight:
		maskX = labelTL.X - 2
		maskY = shapeBox.TopLeft.Y + shapeBox.Height - effectiveStrokeWidth/2
		maskWidth = float64(labelWidth + 4)
		maskHeight = effectiveStrokeWidth

	case label.BorderLeftTop, label.BorderLeftMiddle, label.BorderLeftBottom:
		maskX = shapeBox.TopLeft.X - effectiveStrokeWidth/2
		maskY = labelTL.Y - 2
		maskWidth = effectiveStrokeWidth
		maskHeight = float64(labelHeight + 4)

	case label.BorderRightTop, label.BorderRightMiddle, label.BorderRightBottom:
		maskX = shapeBox.TopLeft.X + shapeBox.Width - effectiveStrokeWidth/2
		maskY = labelTL.Y - 2
		maskWidth = effectiveStrokeWidth
		maskHeight = float64(labelHeight + 4)

	default:
		// never gets here
		return ""
	}

	// In sketch mode, adjust coordinates to account for shape transform
	if shapePos != nil {
		maskX -= shapePos.X
		maskY -= shapePos.Y
	}

	return fmt.Sprintf(`<rect x="%f" y="%f" width="%f" height="%f" fill="%s"></rect>`,
		maskX, maskY, maskWidth, maskHeight, fill,
	)
}

func drawConnection(writer io.Writer, diagramHash string, connection d2target.Connection, markers map[string]struct{}, idToShape map[string]d2target.Shape, jsRunner jsrunner.JSRunner, inlineTheme *d2themes.Theme) (labelMask string, _ error) {
	opacityStyle := ""
	if connection.Opacity != 1.0 {
		opacityStyle = fmt.Sprintf(" style='opacity:%f'", connection.Opacity)
	}

	classes := []string{base64.URLEncoding.EncodeToString([]byte(svg.EscapeText(connection.ID)))}
	classes = append(classes, connection.Classes...)
	classStr := fmt.Sprintf(` class="%s"`, strings.Join(classes, " "))

	fmt.Fprintf(writer, `<g%s%s>`, classStr, opacityStyle)
	var markerStart string
	if connection.SrcArrow != d2target.NoArrowhead {
		id := arrowheadMarkerID(diagramHash, false, connection)
		if _, in := markers[id]; !in {
			marker := arrowheadMarker(false, id, connection, inlineTheme)
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
		id := arrowheadMarkerID(diagramHash, true, connection)
		if _, in := markers[id]; !in {
			marker := arrowheadMarker(true, id, connection, inlineTheme)
			if marker == "" {
				panic(fmt.Sprintf("received empty arrow head marker for: %#v", connection))
			}
			fmt.Fprint(writer, marker)
			markers[id] = struct{}{}
		}
		markerEnd = fmt.Sprintf(`marker-end="url(#%s)" `, id)
	}

	if connection.Icon != nil {
		iconPos := connection.GetIconPosition()
		if iconPos != nil {
			connectionIconClipPath := ""
			if connection.IconBorderRadius != 0 {
				connectionIconClipPath = fmt.Sprintf(` clip-path="inset(0 round %fpx)"`, connection.IconBorderRadius)
			}
			fmt.Fprintf(writer, `<image href="%s" x="%f" y="%f" width="%d" height="%d"%s />`,
				html.EscapeString(connection.Icon.String()),
				iconPos.X,
				iconPos.Y,
				d2target.DEFAULT_ICON_SIZE,
				d2target.DEFAULT_ICON_SIZE,
				connectionIconClipPath,
			)
		}
	}

	var labelTL *geo.Point
	if connection.Label != "" {
		labelTL = connection.GetLabelTopLeft()
		labelTL.X = math.Round(labelTL.X)
		labelTL.Y = math.Round(labelTL.Y)

		maskTL := labelTL.Copy()
		width := connection.LabelWidth
		height := connection.LabelHeight

		if connection.Icon != nil {
			width += d2target.CONNECTION_ICON_LABEL_GAP + d2target.DEFAULT_ICON_SIZE
			maskTL.X -= float64(d2target.CONNECTION_ICON_LABEL_GAP + d2target.DEFAULT_ICON_SIZE)
		}

		if label.FromString(connection.LabelPosition).IsOnEdge() {
			labelMask = makeLabelMask(maskTL, width, height, 1)
		} else {
			labelMask = makeLabelMask(maskTL, width, height, 0.75)
		}
	} else if connection.Icon != nil {
		iconPos := connection.GetIconPosition()
		if iconPos != nil {
			maskTL := &geo.Point{
				X: iconPos.X,
				Y: iconPos.Y,
			}
			if label.FromString(connection.IconPosition).IsOnEdge() {
				labelMask = makeLabelMask(maskTL, d2target.DEFAULT_ICON_SIZE, d2target.DEFAULT_ICON_SIZE, 1)
			} else {
				labelMask = makeLabelMask(maskTL, d2target.DEFAULT_ICON_SIZE, d2target.DEFAULT_ICON_SIZE, 0.75)
			}
		}
	}

	srcAdj, dstAdj := getArrowheadAdjustments(connection, idToShape)
	path := pathData(connection, srcAdj, dstAdj)
	mask := fmt.Sprintf(`mask="url(#%s)"`, diagramHash)

	if jsRunner != nil {
		out, err := d2sketch.Connection(jsRunner, connection, path, mask)
		if err != nil {
			return "", err
		}
		fmt.Fprint(writer, out)

		// render sketch arrowheads separately
		arrowPaths, err := d2sketch.Arrowheads(jsRunner, connection, srcAdj, dstAdj)
		if err != nil {
			return "", err
		}
		fmt.Fprint(writer, arrowPaths)
	} else {
		animatedClass := ""
		if connection.Animated {
			animatedClass = " animated-connection"
		}

		// If connection is animated and bidirectional
		if connection.Animated && ((connection.DstArrow == d2target.NoArrowhead && connection.SrcArrow == d2target.NoArrowhead) || (connection.DstArrow != d2target.NoArrowhead && connection.SrcArrow != d2target.NoArrowhead)) {
			// There is no pure CSS way to animate bidirectional connections in two directions, so we split it up
			path1, path2, err := svg.SplitPath(path, 0.5)

			if err != nil {
				return "", err
			}

			pathEl1 := d2themes.NewThemableElement("path", inlineTheme)
			pathEl1.D = path1
			pathEl1.Fill = color.None
			pathEl1.Stroke = connection.Stroke
			pathEl1.ClassName = fmt.Sprintf("connection%s", animatedClass)
			pathEl1.Style = connection.CSSStyle()
			pathEl1.Style += "animation-direction: reverse;"
			pathEl1.Attributes = fmt.Sprintf("%s%s", markerStart, mask)
			fmt.Fprint(writer, pathEl1.Render())

			pathEl2 := d2themes.NewThemableElement("path", inlineTheme)
			pathEl2.D = path2
			pathEl2.Fill = color.None
			pathEl2.Stroke = connection.Stroke
			pathEl2.ClassName = fmt.Sprintf("connection%s", animatedClass)
			pathEl2.Style = connection.CSSStyle()
			pathEl2.Attributes = fmt.Sprintf("%s%s", markerEnd, mask)
			fmt.Fprint(writer, pathEl2.Render())
		} else {
			pathEl := d2themes.NewThemableElement("path", inlineTheme)
			pathEl.D = path
			pathEl.Fill = color.None
			pathEl.Stroke = connection.Stroke
			pathEl.ClassName = fmt.Sprintf("connection%s", animatedClass)
			pathEl.Style = connection.CSSStyle()
			pathEl.Attributes = fmt.Sprintf("%s%s%s", markerStart, markerEnd, mask)
			fmt.Fprint(writer, pathEl.Render())
		}
	}

	if connection.Label != "" {
		if connection.Language == "latex" {
			render, err := d2latex.Render(connection.Label)
			if err != nil {
				return labelMask, err
			}
			// Remove XML declaration and DOCTYPE from LaTeX SVG output
			xmlDecl := `<?xml version="1.0" encoding="UTF-8"?>`
			doctype := `<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.0//EN" "http://www.w3.org/TR/2001/REC-SVG-20010904/DTD/svg10.dtd">`
			render = strings.ReplaceAll(render, xmlDecl, "")
			render = strings.ReplaceAll(render, doctype, "")
			gEl := d2themes.NewThemableElement("g", inlineTheme)
			gEl.SetTranslate(labelTL.X, labelTL.Y)
			gEl.Color = connection.Color
			gEl.Content = render
			fmt.Fprint(writer, gEl.Render())
		} else if connection.Language == "markdown" {
			render, err := textmeasure.RenderMarkdown(connection.Label)
			if err != nil {
				return labelMask, err
			}

			fmt.Fprintf(writer, `<g><foreignObject requiredFeatures="http://www.w3.org/TR/SVG11/feature#Extensibility" x="%f" y="%f" width="%d" height="%d">`,
				labelTL.X, labelTL.Y, connection.LabelWidth, connection.LabelHeight,
			)

			render = strings.ReplaceAll(render, "<hr>", "<hr />")

			mdEl := d2themes.NewThemableElement("div", inlineTheme)
			mdEl.ClassName = "md"
			mdEl.Content = render

			var styles []string
			if connection.FontSize != textmeasure.MarkdownFontSize {
				styles = append(styles, fmt.Sprintf("font-size:%vpx", connection.FontSize))
			}
			if connection.Fill != "" && connection.Fill != "transparent" {
				styles = append(styles, fmt.Sprintf(`background-color:%s`, connection.Fill))
			}
			if !color.IsThemeColor(connection.Color) {
				styles = append(styles, fmt.Sprintf(`color:%s`, connection.Color))
			}

			mdEl.Style = strings.Join(styles, ";")

			fmt.Fprint(writer, mdEl.Render())
			fmt.Fprint(writer, `</foreignObject></g>`)
		} else if connection.Language != "" {
			lexer := lexers.Get(connection.Language)
			if lexer == nil {
				lexer = lexers.Fallback
			}
			for _, isLight := range []bool{true, false} {
				theme := "github"
				if !isLight {
					theme = "catppuccin-mocha"
				}
				style := styles.Get(theme)
				if style == nil {
					return labelMask, errors.New(`code snippet style "github" not found`)
				}
				iterator, err := lexer.Tokenise(nil, connection.Label)
				if err != nil {
					return labelMask, err
				}

				svgStyles := styleToSVG(style)
				class := "light-code"
				if !isLight {
					class = "dark-code"
				}
				var fontSize string
				if connection.FontSize != d2fonts.FONT_SIZE_M {
					fontSize = fmt.Sprintf(` style="font-size:%v"`, connection.FontSize)
				}
				fmt.Fprintf(writer, `<g transform="translate(%f %f)" class="%s"%s>`,
					labelTL.X, labelTL.Y, class, fontSize,
				)

				lineHeight := textmeasure.CODE_LINE_HEIGHT
				for index, tokens := range chroma.SplitTokensIntoLines(iterator.Tokens()) {
					fmt.Fprintf(writer, "<text class=\"text-mono\" x=\"0\" y=\"%fem\">", 1+float64(index)*lineHeight)
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
				fmt.Fprint(writer, "</g>")
			}
		} else {
			fontClass := "text"
			if connection.FontFamily == "mono" {
				fontClass = "text-mono"
			}
			if connection.Bold {
				fontClass += "-bold"
			} else if connection.Italic {
				fontClass += "-italic"
			}
			if connection.Underline {
				fontClass += " text-underline"
			}
			if connection.Fill != color.Empty {
				rectEl := d2themes.NewThemableElement("rect", inlineTheme)
				rectEl.Rx = 10
				rectEl.X, rectEl.Y = labelTL.X-4, labelTL.Y-3
				rectEl.Width, rectEl.Height = float64(connection.LabelWidth)+8, float64(connection.LabelHeight)+6
				rectEl.Fill = connection.Fill
				fmt.Fprint(writer, rectEl.Render())
			}

			textEl := d2themes.NewThemableElement("text", inlineTheme)
			textEl.X = labelTL.X + float64(connection.LabelWidth)/2
			textEl.Y = labelTL.Y + float64(connection.FontSize)
			textEl.ClassName = fontClass
			textEl.Style = fmt.Sprintf("text-anchor:%s;font-size:%vpx", "middle", connection.FontSize)
			textEl.Content = RenderText(connection.Label, textEl.X, float64(connection.LabelHeight))

			if connection.Link != "" {
				textEl.ClassName += " text-underline text-link"

				fmt.Fprintf(writer, `<a href="%s" xlink:href="%[1]s">`, svg.EscapeText(connection.Link))
			} else {
				textEl.Fill = connection.GetFontColor()
			}

			fmt.Fprint(writer, textEl.Render())

			if connection.Link != "" {
				fmt.Fprintf(writer, "</a>")
			}
		}
	}

	if connection.SrcLabel != nil && connection.SrcLabel.Label != "" {
		fmt.Fprint(writer, renderArrowheadLabel(connection, connection.SrcLabel.Label, false, inlineTheme))
	}
	if connection.DstLabel != nil && connection.DstLabel.Label != "" {
		fmt.Fprint(writer, renderArrowheadLabel(connection, connection.DstLabel.Label, true, inlineTheme))
	}
	fmt.Fprintf(writer, `</g>`)
	return
}

func renderArrowheadLabel(connection d2target.Connection, text string, isDst bool, inlineTheme *d2themes.Theme) string {
	var width, height float64
	if isDst {
		width = float64(connection.DstLabel.LabelWidth)
		height = float64(connection.DstLabel.LabelHeight)
	} else {
		width = float64(connection.SrcLabel.LabelWidth)
		height = float64(connection.SrcLabel.LabelHeight)
	}

	labelTL := connection.GetArrowheadLabelPosition(isDst)

	// svg text is positioned with the center of its baseline
	baselineCenter := geo.Point{
		X: labelTL.X + width/2.,
		Y: labelTL.Y + float64(connection.FontSize),
	}

	textEl := d2themes.NewThemableElement("text", inlineTheme)
	textEl.X = baselineCenter.X
	textEl.Y = baselineCenter.Y
	textEl.Fill = d2target.FG_COLOR
	if isDst {
		if connection.DstLabel.Color != "" {
			textEl.Fill = connection.DstLabel.Color
		}
	} else {
		if connection.SrcLabel.Color != "" {
			textEl.Fill = connection.SrcLabel.Color
		}
	}
	textEl.ClassName = "text-italic"
	textEl.Style = fmt.Sprintf("text-anchor:middle;font-size:%vpx", connection.FontSize)
	textEl.Content = RenderText(text, textEl.X, height)
	return textEl.Render()
}

func renderOval(tl *geo.Point, width, height float64, fill, fillPattern, stroke, style string, inlineTheme *d2themes.Theme) string {
	el := d2themes.NewThemableElement("ellipse", inlineTheme)
	el.Rx = width / 2
	el.Ry = height / 2
	el.Cx = tl.X + el.Rx
	el.Cy = tl.Y + el.Ry
	el.Fill, el.Stroke = fill, stroke
	el.FillPattern = fillPattern
	el.ClassName = "shape"
	el.Style = style
	return el.Render()
}

func renderDoubleOval(tl *geo.Point, width, height float64, fill, fillStroke, stroke, style string, inlineTheme *d2themes.Theme) string {
	var innerTL *geo.Point = tl.AddVector(geo.NewVector(d2target.INNER_BORDER_OFFSET, d2target.INNER_BORDER_OFFSET))
	return renderOval(tl, width, height, fill, fillStroke, stroke, style, inlineTheme) + renderOval(innerTL, width-10, height-10, fill, "", stroke, style, inlineTheme)
}

func defineGradients(writer io.Writer, cssGradient string) {
	gradient, _ := color.ParseGradient(cssGradient)
	fmt.Fprint(writer, fmt.Sprintf(`<defs>%s</defs>`, color.GradientToSVG(gradient)))
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

func render3DRect(diagramHash string, targetShape d2target.Shape, inlineTheme *d2themes.Theme) string {
	moveTo := func(p d2target.Point) string {
		return fmt.Sprintf("M%d,%d", p.X+targetShape.Pos.X, p.Y+targetShape.Pos.Y)
	}
	lineTo := func(p d2target.Point) string {
		return fmt.Sprintf("L%d,%d", p.X+targetShape.Pos.X, p.Y+targetShape.Pos.Y)
	}

	// draw border all in one path to prevent overlapping sections
	var borderSegments []string
	borderSegments = append(borderSegments,
		moveTo(d2target.Point{X: 0, Y: 0}),
	)
	for _, v := range []d2target.Point{
		{X: d2target.THREE_DEE_OFFSET, Y: -d2target.THREE_DEE_OFFSET},
		{X: targetShape.Width + d2target.THREE_DEE_OFFSET, Y: -d2target.THREE_DEE_OFFSET},
		{X: targetShape.Width + d2target.THREE_DEE_OFFSET, Y: targetShape.Height - d2target.THREE_DEE_OFFSET},
		{X: targetShape.Width, Y: targetShape.Height},
		{X: 0, Y: targetShape.Height},
		{X: 0, Y: 0},
		{X: targetShape.Width, Y: 0},
		{X: targetShape.Width, Y: targetShape.Height},
	} {
		borderSegments = append(borderSegments, lineTo(v))
	}
	// move to top right to draw last segment without overlapping
	borderSegments = append(borderSegments,
		moveTo(d2target.Point{X: targetShape.Width, Y: 0}),
	)
	borderSegments = append(borderSegments,
		lineTo(d2target.Point{X: targetShape.Width + d2target.THREE_DEE_OFFSET, Y: -d2target.THREE_DEE_OFFSET}),
	)
	border := d2themes.NewThemableElement("path", inlineTheme)
	border.D = strings.Join(borderSegments, " ")
	border.Fill = color.None
	_, borderStroke := d2themes.ShapeTheme(targetShape)
	border.Stroke = borderStroke
	borderStyle := targetShape.CSSStyle()
	border.Style = borderStyle
	renderedBorder := border.Render()

	// create mask from border stroke, to cut away from the shape fills
	maskID := fmt.Sprintf("border-mask-%v-%v", diagramHash, svg.EscapeText(targetShape.ID))
	borderMask := strings.Join([]string{
		fmt.Sprintf(`<defs><mask id="%s" maskUnits="userSpaceOnUse" x="%d" y="%d" width="%d" height="%d">`,
			maskID, targetShape.Pos.X, targetShape.Pos.Y-d2target.THREE_DEE_OFFSET, targetShape.Width+d2target.THREE_DEE_OFFSET, targetShape.Height+d2target.THREE_DEE_OFFSET,
		),
		fmt.Sprintf(`<rect x="%d" y="%d" width="%d" height="%d" fill="white"></rect>`,
			targetShape.Pos.X, targetShape.Pos.Y-d2target.THREE_DEE_OFFSET, targetShape.Width+d2target.THREE_DEE_OFFSET, targetShape.Height+d2target.THREE_DEE_OFFSET,
		),
		fmt.Sprintf(`<path d="%s" style="%s;stroke:#000;fill:none;opacity:1;"/></mask></defs>`,
			strings.Join(borderSegments, ""), borderStyle),
	}, "\n")

	// render the main rectangle without stroke and the border mask
	mainShape := d2themes.NewThemableElement("rect", inlineTheme)
	mainShape.X = float64(targetShape.Pos.X)
	mainShape.Y = float64(targetShape.Pos.Y)
	mainShape.Width = float64(targetShape.Width)
	mainShape.Height = float64(targetShape.Height)
	mainShape.SetMaskUrl(maskID)
	mainShapeFill, _ := d2themes.ShapeTheme(targetShape)
	mainShape.Fill = mainShapeFill
	mainShape.FillPattern = targetShape.FillPattern
	mainShape.Stroke = color.None
	mainShape.Style = targetShape.CSSStyle()
	mainShapeRendered := mainShape.Render()

	// render the side shapes in the darkened color without stroke and the border mask
	var sidePoints []string
	for _, v := range []d2target.Point{
		{X: 0, Y: 0},
		{X: d2target.THREE_DEE_OFFSET, Y: -d2target.THREE_DEE_OFFSET},
		{X: targetShape.Width + d2target.THREE_DEE_OFFSET, Y: -d2target.THREE_DEE_OFFSET},
		{X: targetShape.Width + d2target.THREE_DEE_OFFSET, Y: targetShape.Height - d2target.THREE_DEE_OFFSET},
		{X: targetShape.Width, Y: targetShape.Height},
		{X: targetShape.Width, Y: 0},
	} {
		sidePoints = append(sidePoints,
			fmt.Sprintf("%d,%d", v.X+targetShape.Pos.X, v.Y+targetShape.Pos.Y),
		)
	}
	darkerColor, err := color.Darken(targetShape.Fill)
	if err != nil {
		darkerColor = targetShape.Fill
	}
	sideShape := d2themes.NewThemableElement("polygon", inlineTheme)
	sideShape.Fill = darkerColor
	sideShape.Points = strings.Join(sidePoints, " ")
	sideShape.SetMaskUrl(maskID)
	sideShape.Style = targetShape.CSSStyle()
	renderedSides := sideShape.Render()

	return borderMask + mainShapeRendered + renderedSides + renderedBorder
}

func render3DHexagon(diagramHash string, targetShape d2target.Shape, inlineTheme *d2themes.Theme) string {
	moveTo := func(p d2target.Point) string {
		return fmt.Sprintf("M%d,%d", p.X+targetShape.Pos.X, p.Y+targetShape.Pos.Y)
	}
	lineTo := func(p d2target.Point) string {
		return fmt.Sprintf("L%d,%d", p.X+targetShape.Pos.X, p.Y+targetShape.Pos.Y)
	}
	scale := func(n int, f float64) int {
		return int(float64(n) * f)
	}
	halfYFactor := 43.6 / 87.3

	// draw border all in one path to prevent overlapping sections
	var borderSegments []string
	// start from the top-left
	borderSegments = append(borderSegments,
		moveTo(d2target.Point{X: scale(targetShape.Width, 0.25), Y: 0}),
	)
	Y_OFFSET := d2target.THREE_DEE_OFFSET / 2
	// The following iterates through the sidepoints in clockwise order from top-left, then the main points in clockwise order from bottom-right
	for _, v := range []d2target.Point{
		{X: scale(targetShape.Width, 0.25) + d2target.THREE_DEE_OFFSET, Y: -Y_OFFSET},
		{X: scale(targetShape.Width, 0.75) + d2target.THREE_DEE_OFFSET, Y: -Y_OFFSET},
		{X: targetShape.Width + d2target.THREE_DEE_OFFSET, Y: scale(targetShape.Height, halfYFactor) - Y_OFFSET},
		{X: scale(targetShape.Width, 0.75) + d2target.THREE_DEE_OFFSET, Y: targetShape.Height - Y_OFFSET},
		{X: scale(targetShape.Width, 0.75), Y: targetShape.Height},
		{X: scale(targetShape.Width, 0.25), Y: targetShape.Height},
		{X: 0, Y: scale(targetShape.Height, halfYFactor)},
		{X: scale(targetShape.Width, 0.25), Y: 0},
		{X: scale(targetShape.Width, 0.75), Y: 0},
		{X: targetShape.Width, Y: scale(targetShape.Height, halfYFactor)},
		{X: scale(targetShape.Width, 0.75), Y: targetShape.Height},
	} {
		borderSegments = append(borderSegments, lineTo(v))
	}
	for _, v := range []d2target.Point{
		{X: scale(targetShape.Width, 0.75), Y: 0},
		{X: targetShape.Width, Y: scale(targetShape.Height, halfYFactor)},
		{X: scale(targetShape.Width, 0.75), Y: targetShape.Height},
	} {
		borderSegments = append(borderSegments, moveTo(v))
		borderSegments = append(borderSegments, lineTo(
			d2target.Point{X: v.X + d2target.THREE_DEE_OFFSET, Y: v.Y - Y_OFFSET},
		))
	}
	border := d2themes.NewThemableElement("path", inlineTheme)
	border.D = strings.Join(borderSegments, " ")
	border.Fill = color.None
	_, borderStroke := d2themes.ShapeTheme(targetShape)
	border.Stroke = borderStroke
	borderStyle := targetShape.CSSStyle()
	border.Style = borderStyle
	renderedBorder := border.Render()

	var mainPoints []string
	for _, v := range []d2target.Point{
		{X: scale(targetShape.Width, 0.25), Y: 0},
		{X: scale(targetShape.Width, 0.75), Y: 0},
		{X: targetShape.Width, Y: scale(targetShape.Height, halfYFactor)},
		{X: scale(targetShape.Width, 0.75), Y: targetShape.Height},
		{X: scale(targetShape.Width, 0.25), Y: targetShape.Height},
		{X: 0, Y: scale(targetShape.Height, halfYFactor)},
	} {
		mainPoints = append(mainPoints,
			fmt.Sprintf("%d,%d", v.X+targetShape.Pos.X, v.Y+targetShape.Pos.Y),
		)
	}

	mainPointsPoly := strings.Join(mainPoints, " ")
	// create mask from border stroke, to cut away from the shape fills
	maskID := fmt.Sprintf("border-mask-%v-%v", diagramHash, svg.EscapeText(targetShape.ID))
	borderMask := strings.Join([]string{
		fmt.Sprintf(`<defs><mask id="%s" maskUnits="userSpaceOnUse" x="%d" y="%d" width="%d" height="%d">`,
			maskID, targetShape.Pos.X, targetShape.Pos.Y-d2target.THREE_DEE_OFFSET, targetShape.Width+d2target.THREE_DEE_OFFSET, targetShape.Height+d2target.THREE_DEE_OFFSET,
		),
		fmt.Sprintf(`<rect x="%d" y="%d" width="%d" height="%d" fill="white"></rect>`,
			targetShape.Pos.X, targetShape.Pos.Y-d2target.THREE_DEE_OFFSET, targetShape.Width+d2target.THREE_DEE_OFFSET, targetShape.Height+d2target.THREE_DEE_OFFSET,
		),
		fmt.Sprintf(`<path d="%s" style="%s;stroke:#000;fill:none;opacity:1;"/></mask></defs>`,
			strings.Join(borderSegments, ""), borderStyle),
	}, "\n")
	// render the main hexagon without stroke and the border mask
	mainShape := d2themes.NewThemableElement("polygon", inlineTheme)
	mainShape.X = float64(targetShape.Pos.X)
	mainShape.Y = float64(targetShape.Pos.Y)
	mainShape.Points = mainPointsPoly
	mainShape.SetMaskUrl(maskID)
	mainShapeFill, _ := d2themes.ShapeTheme(targetShape)
	mainShape.FillPattern = targetShape.FillPattern
	mainShape.Fill = mainShapeFill
	mainShape.Stroke = color.None
	mainShape.Style = targetShape.CSSStyle()
	mainShapeRendered := mainShape.Render()

	// render the side shapes in the darkened color without stroke and the border mask
	var sidePoints []string
	for _, v := range []d2target.Point{
		{X: scale(targetShape.Width, 0.25) + d2target.THREE_DEE_OFFSET, Y: -Y_OFFSET},
		{X: scale(targetShape.Width, 0.75) + d2target.THREE_DEE_OFFSET, Y: -Y_OFFSET},
		{X: targetShape.Width + d2target.THREE_DEE_OFFSET, Y: scale(targetShape.Height, halfYFactor) - Y_OFFSET},
		{X: scale(targetShape.Width, 0.75) + d2target.THREE_DEE_OFFSET, Y: targetShape.Height - Y_OFFSET},
		{X: scale(targetShape.Width, 0.75), Y: targetShape.Height},
		{X: targetShape.Width, Y: scale(targetShape.Height, halfYFactor)},
		{X: scale(targetShape.Width, 0.75), Y: 0},
		{X: scale(targetShape.Width, 0.25), Y: 0},
	} {
		sidePoints = append(sidePoints,
			fmt.Sprintf("%d,%d", v.X+targetShape.Pos.X, v.Y+targetShape.Pos.Y),
		)
	}
	// TODO make darker color part of the theme? or just keep this bypass
	darkerColor, err := color.Darken(targetShape.Fill)
	if err != nil {
		darkerColor = targetShape.Fill
	}
	sideShape := d2themes.NewThemableElement("polygon", inlineTheme)
	sideShape.Fill = darkerColor
	sideShape.Points = strings.Join(sidePoints, " ")
	sideShape.SetMaskUrl(maskID)
	sideShape.Style = targetShape.CSSStyle()
	renderedSides := sideShape.Render()

	return borderMask + mainShapeRendered + renderedSides + renderedBorder
}

func drawShape(writer, appendixWriter io.Writer, diagramHash string, targetShape d2target.Shape, jsRunner jsrunner.JSRunner, inlineTheme *d2themes.Theme) (labelMask string, err error) {
	closingTag := "</g>"
	if targetShape.Link != "" {

		fmt.Fprintf(writer, `<a href="%s" xlink:href="%[1]s">`, svg.EscapeText(targetShape.Link))
		closingTag += "</a>"
	}
	// Opacity is a unique style, it applies to everything for a shape
	opacityStyle := ""
	if targetShape.Opacity != 1.0 {
		opacityStyle = fmt.Sprintf(" style='opacity:%f'", targetShape.Opacity)
	}

	// this clipPath must be defined outside `g` element
	if targetShape.BorderRadius != 0 && (targetShape.Type == d2target.ShapeClass || targetShape.Type == d2target.ShapeSQLTable) {
		fmt.Fprint(writer, clipPathForBorderRadius(diagramHash, targetShape))
	}
	var iconClipPathID string
	if targetShape.IconBorderRadius != 0 && (targetShape.Type == d2target.ShapeImage) {
		// Set the icon's border-radius to half of it's smaller dimension in case it exceeds that
		// https://www.w3.org/Style/CSS/Tracker/issues/29?changelog
		targetShape.IconBorderRadius = min(targetShape.IconBorderRadius, min(targetShape.Width, targetShape.Height)/2)
		iconClipPathID = fmt.Sprintf("%v-%v-icon", diagramHash, svg.SVGID(targetShape.ID))
		fmt.Fprint(writer, applyIconBorderRadius(iconClipPathID, targetShape))
	}
	classes := []string{base64.URLEncoding.EncodeToString([]byte(svg.EscapeText(targetShape.ID)))}
	if targetShape.Animated {
		classes = append(classes, "animated-shape")
	}
	classes = append(classes, targetShape.Classes...)
	classStr := fmt.Sprintf(` class="%s"`, strings.Join(classes, " "))
	fmt.Fprintf(writer, `<g%s%s>`, classStr, opacityStyle)
	tl := geo.NewPoint(float64(targetShape.Pos.X), float64(targetShape.Pos.Y))
	width := float64(targetShape.Width)
	height := float64(targetShape.Height)
	fill, stroke := d2themes.ShapeTheme(targetShape)
	style := targetShape.CSSStyle()
	shapeType := d2target.DSL_SHAPE_TO_SHAPE_TYPE[targetShape.Type]

	s := shape.NewShape(shapeType, geo.NewBox(tl, width, height))
	if shapeType == shape.CLOUD_TYPE && targetShape.ContentAspectRatio != nil {
		s.SetInnerBoxAspectRatio(*targetShape.ContentAspectRatio)
	}

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

	var blendModeClass string
	if targetShape.Blend {
		blendModeClass = " blend"
	}

	fmt.Fprintf(writer, `<g class="shape%s" %s>`, blendModeClass, shadowAttr)

	var multipleTL *geo.Point
	if targetShape.Multiple {
		multipleTL = tl.AddVector(multipleOffset)
	}

	switch targetShape.Type {
	case d2target.ShapeClass:
		if jsRunner != nil {
			out, err := d2sketch.Class(jsRunner, targetShape)
			if err != nil {
				return "", err
			}
			fmt.Fprint(writer, out)
		} else {
			drawClass(writer, diagramHash, targetShape, inlineTheme)
		}
		err := addAppendixItems(appendixWriter, diagramHash, targetShape, s)
		if err != nil {
			return "", err
		}
		fmt.Fprint(writer, `</g>`)
		fmt.Fprint(writer, closingTag)
		return labelMask, nil
	case d2target.ShapeSQLTable:
		if jsRunner != nil {
			out, err := d2sketch.Table(jsRunner, targetShape)
			if err != nil {
				return "", err
			}
			fmt.Fprint(writer, out)
		} else {
			drawTable(writer, diagramHash, targetShape, inlineTheme)
		}
		err := addAppendixItems(appendixWriter, diagramHash, targetShape, s)
		if err != nil {
			return "", err
		}
		fmt.Fprint(writer, `</g>`)
		fmt.Fprint(writer, closingTag)
		return labelMask, nil
	case d2target.ShapeOval:
		if targetShape.DoubleBorder {
			if targetShape.Multiple {
				fmt.Fprint(writer, renderDoubleOval(multipleTL, width, height, fill, "", stroke, style, inlineTheme))
			}
			if jsRunner != nil {
				out, err := d2sketch.DoubleOval(jsRunner, targetShape, diagramHash)
				if err != nil {
					return "", err
				}
				fmt.Fprint(writer, out)
			} else {
				fmt.Fprint(writer, renderDoubleOval(tl, width, height, fill, targetShape.FillPattern, stroke, style, inlineTheme))
			}
		} else {
			if targetShape.Multiple {
				fmt.Fprint(writer, renderOval(multipleTL, width, height, fill, "", stroke, style, inlineTheme))
			}
			if jsRunner != nil {
				out, err := d2sketch.Oval(jsRunner, targetShape, diagramHash)
				if err != nil {
					return "", err
				}
				fmt.Fprint(writer, out)
			} else {
				fmt.Fprint(writer, renderOval(tl, width, height, fill, targetShape.FillPattern, stroke, style, inlineTheme))
			}
		}

	case d2target.ShapeImage:
		el := d2themes.NewThemableElement("image", inlineTheme)
		el.X = float64(targetShape.Pos.X)
		el.Y = float64(targetShape.Pos.Y)
		el.Width = float64(targetShape.Width)
		el.Height = float64(targetShape.Height)
		el.Href = html.EscapeString(targetShape.Icon.String())
		el.Fill = fill
		el.Stroke = stroke
		el.Style = style
		if targetShape.IconBorderRadius != 0 {
			el.ClipPath = iconClipPathID
		}
		fmt.Fprint(writer, el.Render())

	// TODO should standardize "" to rectangle
	case d2target.ShapeRectangle, d2target.ShapeSequenceDiagram, d2target.ShapeHierarchy, "":
		borderRadius := math.MaxFloat64
		if targetShape.BorderRadius != 0 {
			borderRadius = float64(targetShape.BorderRadius)
		}
		if targetShape.ThreeDee {
			fmt.Fprint(writer, render3DRect(diagramHash, targetShape, inlineTheme))
		} else {
			if !targetShape.DoubleBorder {
				if targetShape.Multiple {
					el := d2themes.NewThemableElement("rect", inlineTheme)
					el.X = float64(targetShape.Pos.X + 10)
					el.Y = float64(targetShape.Pos.Y - 10)
					el.Width = float64(targetShape.Width)
					el.Height = float64(targetShape.Height)
					el.Fill = fill
					el.Stroke = stroke
					el.Style = style
					el.Rx = borderRadius
					fmt.Fprint(writer, el.Render())
				}
				if jsRunner != nil {
					out, err := d2sketch.Rect(jsRunner, targetShape, diagramHash)
					if err != nil {
						return "", err
					}
					fmt.Fprint(writer, out)
				} else {
					el := d2themes.NewThemableElement("rect", inlineTheme)
					el.X = float64(targetShape.Pos.X)
					el.Y = float64(targetShape.Pos.Y)
					el.Width = float64(targetShape.Width)
					el.Height = float64(targetShape.Height)
					el.Fill = fill
					el.FillPattern = targetShape.FillPattern
					el.Stroke = stroke
					el.Style = style
					el.Rx = borderRadius

					if targetShape.Label != "" && label.FromString(targetShape.LabelPosition).IsBorder() {
						el.Mask = fmt.Sprintf("url(#%s)", diagramHash)
					}

					fmt.Fprint(writer, el.Render())
				}
			} else {
				if targetShape.Multiple {
					el := d2themes.NewThemableElement("rect", inlineTheme)
					el.X = float64(targetShape.Pos.X + 10)
					el.Y = float64(targetShape.Pos.Y - 10)
					el.Width = float64(targetShape.Width)
					el.Height = float64(targetShape.Height)
					el.Fill = fill
					el.FillPattern = targetShape.FillPattern
					el.Stroke = stroke
					el.Style = style
					el.Rx = borderRadius
					fmt.Fprint(writer, el.Render())

					el = d2themes.NewThemableElement("rect", inlineTheme)
					el.X = float64(targetShape.Pos.X + 10 + d2target.INNER_BORDER_OFFSET)
					el.Y = float64(targetShape.Pos.Y - 10 + d2target.INNER_BORDER_OFFSET)
					el.Width = float64(targetShape.Width - 2*d2target.INNER_BORDER_OFFSET)
					el.Height = float64(targetShape.Height - 2*d2target.INNER_BORDER_OFFSET)
					el.Fill = fill
					el.Stroke = stroke
					el.Style = style
					el.Rx = borderRadius
					fmt.Fprint(writer, el.Render())
				}
				if jsRunner != nil {
					out, err := d2sketch.DoubleRect(jsRunner, targetShape, diagramHash)
					if err != nil {
						return "", err
					}
					fmt.Fprint(writer, out)
				} else {
					el := d2themes.NewThemableElement("rect", inlineTheme)
					el.X = float64(targetShape.Pos.X)
					el.Y = float64(targetShape.Pos.Y)
					el.Width = float64(targetShape.Width)
					el.Height = float64(targetShape.Height)
					el.Fill = fill
					el.FillPattern = targetShape.FillPattern
					el.Stroke = stroke
					el.Style = style
					el.Rx = borderRadius
					fmt.Fprint(writer, el.Render())

					el = d2themes.NewThemableElement("rect", inlineTheme)
					el.X = float64(targetShape.Pos.X + d2target.INNER_BORDER_OFFSET)
					el.Y = float64(targetShape.Pos.Y + d2target.INNER_BORDER_OFFSET)
					el.Width = float64(targetShape.Width - 2*d2target.INNER_BORDER_OFFSET)
					el.Height = float64(targetShape.Height - 2*d2target.INNER_BORDER_OFFSET)
					el.Fill = "transparent"
					el.Stroke = stroke
					el.Style = style
					el.Rx = borderRadius
					fmt.Fprint(writer, el.Render())
				}
			}
		}
	case d2target.ShapeHexagon:
		if targetShape.ThreeDee {
			fmt.Fprint(writer, render3DHexagon(diagramHash, targetShape, inlineTheme))
		} else {
			if targetShape.Multiple {
				multiplePathData := shape.NewShape(shapeType, geo.NewBox(multipleTL, width, height)).GetSVGPathData()
				el := d2themes.NewThemableElement("path", inlineTheme)
				el.Fill = fill
				el.Stroke = stroke
				el.Style = style
				for _, pathData := range multiplePathData {
					el.D = pathData
					fmt.Fprint(writer, el.Render())
				}
			}

			if jsRunner != nil {
				out, err := d2sketch.Paths(jsRunner, targetShape, diagramHash, s.GetSVGPathData())
				if err != nil {
					return "", err
				}
				fmt.Fprint(writer, out)
			} else {
				el := d2themes.NewThemableElement("path", inlineTheme)
				el.Fill = fill
				el.FillPattern = targetShape.FillPattern
				el.Stroke = stroke
				el.Style = style
				for _, pathData := range s.GetSVGPathData() {
					el.D = pathData
					fmt.Fprint(writer, el.Render())
				}
			}
		}
	case d2target.ShapeText, d2target.ShapeCode:
	default:
		if targetShape.Multiple {
			multiplePathData := shape.NewShape(shapeType, geo.NewBox(multipleTL, width, height)).GetSVGPathData()
			el := d2themes.NewThemableElement("path", inlineTheme)
			el.Fill = fill
			el.Stroke = stroke
			el.Style = style
			for _, pathData := range multiplePathData {
				el.D = pathData
				fmt.Fprint(writer, el.Render())
			}
		}

		if jsRunner != nil {
			out, err := d2sketch.Paths(jsRunner, targetShape, diagramHash, s.GetSVGPathData())
			if err != nil {
				return "", err
			}
			fmt.Fprint(writer, out)
		} else {
			el := d2themes.NewThemableElement("path", inlineTheme)
			el.Fill = fill
			el.FillPattern = targetShape.FillPattern
			el.Stroke = stroke
			el.Style = style

			if targetShape.Label != "" && label.FromString(targetShape.LabelPosition).IsBorder() {
				el.Mask = fmt.Sprintf("url(#%s)", diagramHash)
			}

			for _, pathData := range s.GetSVGPathData() {
				el.D = pathData
				fmt.Fprint(writer, el.Render())
			}
		}
	}

	// // to examine shape's innerBox
	// innerBox := s.GetInnerBox()
	// el := d2themes.NewThemableElement("rect", inlineTheme)
	// el.X = float64(innerBox.TopLeft.X)
	// el.Y = float64(innerBox.TopLeft.Y)
	// el.Width = float64(innerBox.Width)
	// el.Height = float64(innerBox.Height)
	// el.Style = "fill:rgba(255,0,0,0.5);"
	// fmt.Fprint(writer, el.Render())

	// Closes the class=shape
	fmt.Fprint(writer, `</g>`)

	if targetShape.Icon != nil && targetShape.Type != d2target.ShapeImage && targetShape.Opacity != 0 {
		iconPosition := label.FromString(targetShape.IconPosition)
		var box *geo.Box
		if iconPosition.IsOutside() {
			box = s.GetBox()
		} else {
			box = s.GetInnerBox()
		}
		iconSize := d2target.GetIconSize(box, targetShape.IconPosition)

		tl := iconPosition.GetPointOnBox(box, label.PADDING, float64(iconSize), float64(iconSize))

		shapeIconClipPath := ""
		if targetShape.IconBorderRadius != 0 {
			shapeIconClipPath = fmt.Sprintf(` clip-path="inset(0 round %dpx)"`, targetShape.IconBorderRadius)
		}
		fmt.Fprintf(writer, `<image href="%s" x="%f" y="%f" width="%d" height="%d"%s />`,
			html.EscapeString(targetShape.Icon.String()),
			tl.X,
			tl.Y,
			iconSize,
			iconSize,
			shapeIconClipPath,
		)
	}

	if targetShape.Label != "" && targetShape.Opacity != 0 {
		labelPosition := label.FromString(targetShape.LabelPosition)
		var box *geo.Box
		if labelPosition.IsOutside() || labelPosition.IsBorder() {
			box = s.GetBox().Copy()
			// if it is 3d/multiple, place label using box around those
			if targetShape.ThreeDee {
				offsetY := d2target.THREE_DEE_OFFSET
				if targetShape.Type == d2target.ShapeHexagon {
					offsetY /= 2
				}
				box.TopLeft.Y -= float64(offsetY)
				box.Height += float64(offsetY)
				box.Width += d2target.THREE_DEE_OFFSET
			} else if targetShape.Multiple {
				box.TopLeft.Y -= d2target.MULTIPLE_OFFSET
				box.Height += d2target.MULTIPLE_OFFSET
				box.Width += d2target.MULTIPLE_OFFSET
			}
		} else {
			box = s.GetInnerBox()
		}
		labelTL := labelPosition.GetPointOnBox(box, label.PADDING,
			float64(targetShape.LabelWidth),
			float64(targetShape.LabelHeight),
		)

		if labelPosition.IsBorder() {
			if jsRunner != nil {
				labelMask = makeBorderLabelMask(labelPosition, labelTL, targetShape.LabelWidth, targetShape.LabelHeight, box, targetShape.StrokeWidth, 1.0, tl)
			} else {
				labelMask = makeBorderLabelMask(labelPosition, labelTL, targetShape.LabelWidth, targetShape.LabelHeight, box, targetShape.StrokeWidth, 1.0, nil)
			}
		}

		fontClass := "text"
		if targetShape.FontFamily == "mono" {
			fontClass = "text-mono"
		}
		if targetShape.Bold {
			fontClass += "-bold"
		} else if targetShape.Italic {
			fontClass += "-italic"
		}
		if targetShape.Underline {
			fontClass += " text-underline"
		}

		if targetShape.Language == "latex" {
			render, err := d2latex.Render(targetShape.Label)
			if err != nil {
				return labelMask, err
			}
			// Remove XML declaration and DOCTYPE from LaTeX SVG output
			xmlDecl := `<?xml version="1.0" encoding="UTF-8"?>`
			doctype := `<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.0//EN" "http://www.w3.org/TR/2001/REC-SVG-20010904/DTD/svg10.dtd">`
			render = strings.ReplaceAll(render, xmlDecl, "")
			render = strings.ReplaceAll(render, doctype, "")
			gEl := d2themes.NewThemableElement("g", inlineTheme)

			labelPosition := label.FromString(targetShape.LabelPosition)
			if labelPosition == label.Unset {
				labelPosition = label.InsideMiddleCenter
			}
			var box *geo.Box
			if labelPosition.IsOutside() {
				box = s.GetBox()
			} else {
				box = s.GetInnerBox()
			}
			labelTL := labelPosition.GetPointOnBox(box, label.PADDING,
				float64(targetShape.LabelWidth),
				float64(targetShape.LabelHeight),
			)
			gEl.SetTranslate(labelTL.X, labelTL.Y)

			gEl.Color = targetShape.Stroke
			gEl.Content = render
			fmt.Fprint(writer, gEl.Render())
		} else if targetShape.Language == "markdown" {
			render, err := textmeasure.RenderMarkdown(targetShape.Label)
			if err != nil {
				return labelMask, err
			}

			labelPosition := label.FromString(targetShape.LabelPosition)
			if labelPosition == label.Unset {
				labelPosition = label.InsideMiddleCenter
			}
			var box *geo.Box
			if labelPosition.IsOutside() {
				box = s.GetBox()
			} else {
				box = s.GetInnerBox()
			}
			labelTL := labelPosition.GetPointOnBox(box, label.PADDING,
				float64(targetShape.LabelWidth),
				float64(targetShape.LabelHeight),
			)

			fmt.Fprintf(writer, `<g><foreignObject requiredFeatures="http://www.w3.org/TR/SVG11/feature#Extensibility" x="%f" y="%f" width="%d" height="%d">`,
				labelTL.X, labelTL.Y, targetShape.LabelWidth, targetShape.LabelHeight,
			)

			// we need the self closing form in this svg/xhtml context
			render = strings.ReplaceAll(render, "<hr>", "<hr />")

			mdEl := d2themes.NewThemableElement("div", inlineTheme)
			mdEl.Content = render

			// We have to set with styles since within foreignObject, we're in html
			// land and not SVG attributes
			var styles []string
			var classes []string = []string{"md"}
			if targetShape.FontSize != textmeasure.MarkdownFontSize {
				styles = append(styles, fmt.Sprintf("font-size:%vpx", targetShape.FontSize))
			}

			if targetShape.Fill != "" && targetShape.Fill != "transparent" {
				if color.IsThemeColor(targetShape.Fill) {
					classes = append(classes, fmt.Sprintf("fill-%s", targetShape.Fill))
				} else {
					styles = append(styles, fmt.Sprintf(`background-color:%s`, targetShape.Fill))
				}
			}

			if !color.IsThemeColor(targetShape.Color) {
				styles = append(styles, fmt.Sprintf(`color:%s`, targetShape.Color))
			} else {
				classes = append(classes, fmt.Sprintf("color-%s", targetShape.Color))
			}

			mdEl.ClassName = strings.Join(classes, " ")
			// When using dark theme, inlineTheme is nil and we rely on CSS variables

			mdEl.Style = strings.Join(styles, ";")

			fmt.Fprint(writer, mdEl.Render())
			fmt.Fprint(writer, `</foreignObject></g>`)
		} else if targetShape.Language != "" {
			lexer := lexers.Get(targetShape.Language)
			if lexer == nil {
				lexer = lexers.Fallback
			}
			for _, isLight := range []bool{true, false} {
				theme := "github"
				if !isLight {
					theme = "catppuccin-mocha"
				}
				style := styles.Get(theme)
				if style == nil {
					return labelMask, errors.New(`code snippet style "github" not found`)
				}
				formatter := formatters.Get("svg")
				if formatter == nil {
					return labelMask, errors.New(`code snippet formatter "svg" not found`)
				}
				iterator, err := lexer.Tokenise(nil, targetShape.Label)
				if err != nil {
					return labelMask, err
				}

				svgStyles := styleToSVG(style)
				class := "light-code"
				if !isLight {
					class = "dark-code"
				}
				var fontSize string
				if targetShape.FontSize != d2fonts.FONT_SIZE_M {
					fontSize = fmt.Sprintf(` style="font-size:%v"`, targetShape.FontSize)
				}
				fmt.Fprintf(writer, `<g transform="translate(%f %f)" class="%s"%s>`,
					box.TopLeft.X, box.TopLeft.Y, class, fontSize,
				)
				rectEl := d2themes.NewThemableElement("rect", inlineTheme)
				rectEl.Width = float64(targetShape.Width)
				rectEl.Height = float64(targetShape.Height)
				rectEl.Stroke = targetShape.Stroke
				rectEl.ClassName = "shape"
				rectEl.Style = fmt.Sprintf(`fill:%s;stroke-width:%d;`,
					style.Get(chroma.Background).Background.String(),
					targetShape.StrokeWidth,
				)
				fmt.Fprint(writer, rectEl.Render())
				// Padding = 0.5em
				padding := float64(targetShape.FontSize) / 2.
				fmt.Fprintf(writer, `<g transform="translate(%f %f)">`, padding, padding)

				lineHeight := textmeasure.CODE_LINE_HEIGHT
				for index, tokens := range chroma.SplitTokensIntoLines(iterator.Tokens()) {
					fmt.Fprintf(writer, "<text class=\"text-mono\" x=\"0\" y=\"%fem\">", 1+float64(index)*lineHeight)
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
				fmt.Fprint(writer, "</g></g>")
			}
		} else {
			if targetShape.LabelFill != "" {
				rectEl := d2themes.NewThemableElement("rect", inlineTheme)
				rectEl.X = labelTL.X
				rectEl.Y = labelTL.Y
				rectEl.Width = float64(targetShape.LabelWidth)
				rectEl.Height = float64(targetShape.LabelHeight)
				rectEl.Fill = targetShape.LabelFill
				fmt.Fprint(writer, rectEl.Render())
			}
			textEl := d2themes.NewThemableElement("text", inlineTheme)
			textEl.X = labelTL.X + float64(targetShape.LabelWidth)/2
			// text is vertically positioned at its baseline which is at labelTL+FontSize
			textEl.Y = labelTL.Y + float64(targetShape.FontSize)
			textEl.Fill = targetShape.GetFontColor()
			textEl.ClassName = fontClass
			textEl.Style = fmt.Sprintf("text-anchor:%s;font-size:%vpx", "middle", targetShape.FontSize)
			textEl.Content = RenderText(targetShape.Label, textEl.X, float64(targetShape.LabelHeight))
			fmt.Fprint(writer, textEl.Render())
		}
	}
	if targetShape.Tooltip != "" && targetShape.TooltipPosition == "" {
		fmt.Fprintf(writer, `<title>%s</title>`,
			svg.EscapeText(targetShape.Tooltip),
		)
	}
	err = addAppendixItems(appendixWriter, diagramHash, targetShape, s)
	if err != nil {
		return "", err
	}

	fmt.Fprint(writer, closingTag)
	return labelMask, nil
}

func applyIconBorderRadius(clipPathID string, shape d2target.Shape) string {
	box := geo.NewBox(
		geo.NewPoint(float64(shape.Pos.X), float64(shape.Pos.Y)),
		float64(shape.Width),
		float64(shape.Height),
	)
	topX, topY := box.TopLeft.X+box.Width, box.TopLeft.Y

	out := fmt.Sprintf(`<clipPath id="%s">`, clipPathID)
	out += fmt.Sprintf(`<path d="M %f %f L %f %f S %f %f %f %f `, box.TopLeft.X, box.TopLeft.Y+float64(shape.IconBorderRadius), box.TopLeft.X, box.TopLeft.Y+float64(shape.IconBorderRadius), box.TopLeft.X, box.TopLeft.Y, box.TopLeft.X+float64(shape.IconBorderRadius), box.TopLeft.Y)
	out += fmt.Sprintf(`L %f %f L %f %f `, box.TopLeft.X+box.Width-float64(shape.IconBorderRadius), box.TopLeft.Y, topX-float64(shape.IconBorderRadius), topY)

	out += fmt.Sprintf(`S %f %f %f %f `, topX, topY, topX, topY+float64(shape.IconBorderRadius))
	out += fmt.Sprintf(`L %f %f `, topX, topY+box.Height-float64(shape.IconBorderRadius))
	out += fmt.Sprintf(`S %f % f %f %f `, topX, topY+box.Height, topX-float64(shape.IconBorderRadius), topY+box.Height)
	out += fmt.Sprintf(`L %f %f `, box.TopLeft.X+float64(shape.IconBorderRadius), box.TopLeft.Y+box.Height)
	out += fmt.Sprintf(`S %f %f %f %f`, box.TopLeft.X, box.TopLeft.Y+box.Height, box.TopLeft.X, box.TopLeft.Y+box.Height-float64(shape.IconBorderRadius))
	out += fmt.Sprintf(`L %f %f`, box.TopLeft.X, box.TopLeft.Y+float64(shape.IconBorderRadius))
	out += fmt.Sprintf(`Z %f %f" `, box.TopLeft.X, box.TopLeft.Y)
	return out + `fill="none" /> </clipPath>`
}

func addAppendixItems(writer io.Writer, diagramHash string, targetShape d2target.Shape, s shape.Shape) error {
	var p1, p2 *geo.Point
	if targetShape.Tooltip != "" || targetShape.Link != "" {
		bothIcons := targetShape.Tooltip != "" && targetShape.Link != ""
		corner := geo.NewPoint(float64(targetShape.Pos.X+targetShape.Width), float64(targetShape.Pos.Y))
		center := geo.NewPoint(
			float64(targetShape.Pos.X)+float64(targetShape.Width)/2.,
			float64(targetShape.Pos.Y)+float64(targetShape.Height)/2.,
		)
		offset := geo.Vector{-2 * appendixIconRadius, 0}
		var leftOnShape bool
		switch s.GetType() {
		case shape.STEP_TYPE, shape.HEXAGON_TYPE, shape.QUEUE_TYPE, shape.PAGE_TYPE:
			// trace straight left for these
			center.Y = float64(targetShape.Pos.Y)
		case shape.PACKAGE_TYPE:
			// trace straight down
			center.X = float64(targetShape.Pos.X + targetShape.Width)
		case shape.CIRCLE_TYPE, shape.OVAL_TYPE, shape.DIAMOND_TYPE,
			shape.PERSON_TYPE, shape.CLOUD_TYPE, shape.CYLINDER_TYPE:
			if bothIcons {
				leftOnShape = true
				corner = corner.AddVector(offset)
			}
		}
		v1 := center.VectorTo(corner)
		p1 = shape.TraceToShapeBorder(s, corner, corner.AddVector(v1))
		if bothIcons {
			if leftOnShape {
				// these shapes should have p1 on shape border
				p2 = p1.AddVector(offset.Reverse())
				p1, p2 = p2, p1
			} else {
				p2 = p1.AddVector(offset)
			}
		}
	}

	if targetShape.Tooltip != "" {
		if targetShape.TooltipPosition != "" {
			tt, err := renderPositionedTooltip(targetShape, targetShape.TooltipPosition)
			if err != nil {
				return err
			}
			fmt.Fprint(writer, tt)
		} else {
			x := int(math.Ceil(p1.X))
			y := int(math.Ceil(p1.Y))

			fmt.Fprintf(writer, `<g transform="translate(%d %d)" class="appendix-icon"><title>%s</title>%s</g>`,
				x-appendixIconRadius,
				y-appendixIconRadius,
				svg.EscapeText(targetShape.Tooltip),
				fmt.Sprintf(TooltipIcon, diagramHash, svg.SVGID(targetShape.ID)),
			)
		}
	}
	if targetShape.Link != "" {
		if p2 == nil {
			p2 = p1
		}
		x := int(math.Ceil(p2.X))
		y := int(math.Ceil(p2.Y))
		fmt.Fprintf(writer, `<g transform="translate(%d %d)" class="appendix-icon">%s</g>`,
			x-appendixIconRadius,
			y-appendixIconRadius,
			fmt.Sprintf(LinkIcon, diagramHash, svg.SVGID(targetShape.ID)),
		)
	}
	return nil
}

func calculateTooltipPosition(targetShape d2target.Shape, tooltipPosition string, tooltipWidth, tooltipHeight int) (x, y float64, tailDirection string, tailX, tailY float64) {
	shapeX := float64(targetShape.Pos.X)
	shapeY := float64(targetShape.Pos.Y)
	shapeWidth := float64(targetShape.Width)
	shapeHeight := float64(targetShape.Height)

	x, y = d2target.CalculateTooltipPosition(shapeX, shapeY, shapeWidth, shapeHeight, tooltipWidth, tooltipHeight, tooltipPosition)

	switch tooltipPosition {
	case "top-left":
		tailDirection = "bottom"
		tailX = 20
		tailY = float64(tooltipHeight)
	case "top-center":
		tailDirection = "bottom"
		tailX = float64(tooltipWidth) / 2
		tailY = float64(tooltipHeight)
	case "top-right":
		tailDirection = "bottom"
		tailX = float64(tooltipWidth) - 20
		tailY = float64(tooltipHeight)
	case "center-left":
		tailDirection = "right"
		tailX = float64(tooltipWidth)
		tailY = float64(tooltipHeight) / 2
	case "center-right":
		tailDirection = "left"
		tailX = 0
		tailY = float64(tooltipHeight) / 2
	case "bottom-left":
		tailDirection = "top"
		tailX = 20
		tailY = 0
	case "bottom-center":
		tailDirection = "top"
		tailX = float64(tooltipWidth) / 2
		tailY = 0
	case "bottom-right":
		tailDirection = "top"
		tailX = float64(tooltipWidth) - 20
		tailY = 0
	default:
		tailDirection = "bottom"
		tailX = float64(tooltipWidth) / 2
		tailY = float64(tooltipHeight)
	}

	return x, y, tailDirection, tailX, tailY
}

func renderTooltipTail(tailDirection string, tailX, tailY float64) string {
	tailSize := 8.0

	switch tailDirection {
	case "top":
		return fmt.Sprintf(`<path d="M %f %f L %f %f L %f %f Z" fill="white" stroke="#DEE1EB" stroke-width="1"/>`,
			tailX-tailSize/2, tailY,
			tailX+tailSize/2, tailY,
			tailX, tailY-tailSize)
	case "bottom":
		return fmt.Sprintf(`<path d="M %f %f L %f %f L %f %f Z" fill="white" stroke="#DEE1EB" stroke-width="1"/>`,
			tailX-tailSize/2, tailY,
			tailX+tailSize/2, tailY,
			tailX, tailY+tailSize)
	case "left":
		return fmt.Sprintf(`<path d="M %f %f L %f %f L %f %f Z" fill="white" stroke="#DEE1EB" stroke-width="1"/>`,
			tailX, tailY-tailSize/2,
			tailX, tailY+tailSize/2,
			tailX-tailSize, tailY)
	case "right":
		return fmt.Sprintf(`<path d="M %f %f L %f %f L %f %f Z" fill="white" stroke="#DEE1EB" stroke-width="1"/>`,
			tailX, tailY-tailSize/2,
			tailX, tailY+tailSize/2,
			tailX+tailSize, tailY)
	default:
		return ""
	}
}

func renderPositionedTooltip(targetShape d2target.Shape, tooltipPosition string) (string, error) {
	if targetShape.Tooltip == "" || tooltipPosition == "" {
		return "", nil
	}

	var tooltipWidth, tooltipHeight int
	var tooltipContent string

	ruler, err := textmeasure.NewRuler()
	if err != nil {
		return "", err
	}
	fontFamily := go2.Pointer(d2fonts.SourceSansPro)
	fontSize := d2fonts.FONT_SIZE_M

	width, height, err := textmeasure.MeasureMarkdown(targetShape.Tooltip, ruler, fontFamily, fontSize)
	if err != nil {
		return "", err
	}
	tooltipWidth = width + 20
	tooltipHeight = height + 20

	render, err := textmeasure.RenderMarkdown(targetShape.Tooltip)
	if err != nil {
		return "", err
	}
	x, y, tailDirection, tailX, tailY := calculateTooltipPosition(targetShape, tooltipPosition, tooltipWidth, tooltipHeight)

	tooltipContent = fmt.Sprintf(
		`<foreignObject x="%f" y="%f" width="%d" height="%d"><div xmlns="http://www.w3.org/1999/xhtml" class="md color-N1">%s</div></foreignObject>`,
		x+10, y+10, tooltipWidth-20, tooltipHeight-20, render,
	)

	tooltipBox := fmt.Sprintf(
		`<rect x="%f" y="%f" width="%d" height="%d" rx="4" ry="4" fill="white" stroke="#DEE1EB" stroke-width="1"/>`,
		x, y, tooltipWidth, tooltipHeight,
	)

	tail := renderTooltipTail(tailDirection, tailX+x, tailY+y)

	return fmt.Sprintf(`<g class="positioned-tooltip">%s%s%s</g>`, tooltipBox, tail, tooltipContent), nil
}

func RenderText(text string, x, height float64) string {
	if !strings.Contains(text, "\n") {
		return svg.EscapeText(text)
	}
	rendered := []string{}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		dy := height / float64(len(lines))
		if i == 0 {
			dy = 0
		}
		escaped := svg.EscapeText(line)
		if escaped == "" {
			// if there are multiple newlines in a row we still need text for the tspan to render
			escaped = " "
		}
		rendered = append(rendered, fmt.Sprintf(`<tspan x="%f" dy="%f">%s</tspan>`, x, dy, escaped))
	}
	return strings.Join(rendered, "")
}

func EmbedFonts(buf *bytes.Buffer, diagramHash, source string, fontFamily *d2fonts.FontFamily, corpus string) {
	fmt.Fprint(buf, `<style type="text/css"><![CDATA[`)

	appendOnTrigger(
		buf,
		source,
		[]string{
			`class="text"`,
			`class="text `,
			`class="md"`,
			`class="md `,
		},
		fmt.Sprintf(`
.%s .text {
	font-family: "%s-font-regular";
}
@font-face {
	font-family: %s-font-regular;
	src: url("%s");
}`,
			diagramHash,
			diagramHash,
			diagramHash,
			fontFamily.Font(0, d2fonts.FONT_STYLE_REGULAR).GetEncodedSubset(corpus),
		),
	)

	appendOnTrigger(
		buf,
		source,
		[]string{
			`class="md"`,
			`class="md `,
		},
		fmt.Sprintf(`
@font-face {
	font-family: %s-font-semibold;
	src: url("%s");
}`,
			diagramHash,
			fontFamily.Font(0, d2fonts.FONT_STYLE_SEMIBOLD).GetEncodedSubset(corpus),
		),
	)

	appendOnTrigger(
		buf,
		source,
		[]string{
			`text-underline`,
		},
		`
.text-underline {
	text-decoration: underline;
}`,
	)

	appendOnTrigger(
		buf,
		source,
		[]string{
			`text-link`,
		},
		`
.text-link {
	fill: blue;
}

.text-link:visited {
	fill: purple;
}`,
	)

	appendOnTrigger(
		buf,
		source,
		[]string{
			`animated-connection`,
		},
		`
@keyframes dashdraw {
	from {
		stroke-dashoffset: 0;
	}
}
`,
	)

	appendOnTrigger(
		buf,
		source,
		[]string{
			`animated-shape`,
		},
		`
@keyframes shapeappear {
    0%, 100% { transform: translateY(0); filter: drop-shadow(0px 0px 0px rgba(0,0,0,0)); }
    50% { transform: translateY(-4px); filter: drop-shadow(0px 12.6px 25.2px rgba(50,50,93,0.25)) drop-shadow(0px 7.56px 15.12px rgba(0,0,0,0.1)); }
}
.animated-shape {
	animation: shapeappear 1s linear infinite;
}
`,
	)

	appendOnTrigger(
		buf,
		source,
		[]string{
			`appendix-icon`,
		},
		`
.appendix-icon {
	filter: drop-shadow(0px 0px 32px rgba(31, 36, 58, 0.1));
}`,
	)

	appendOnTrigger(
		buf,
		source,
		[]string{
			`class="text-bold`,
			`<b>`,
			`<strong>`,
		},
		fmt.Sprintf(`
.%s .text-bold {
	font-family: "%s-font-bold";
}
@font-face {
	font-family: %s-font-bold;
	src: url("%s");
}`,
			diagramHash,
			diagramHash,
			diagramHash,
			fontFamily.Font(0, d2fonts.FONT_STYLE_BOLD).GetEncodedSubset(corpus),
		),
	)

	appendOnTrigger(
		buf,
		source,
		[]string{
			`class="text-italic`,
			`<em>`,
			`<dfn>`,
		},
		fmt.Sprintf(`
.%s .text-italic {
	font-family: "%s-font-italic";
}
@font-face {
	font-family: %s-font-italic;
	src: url("%s");
}`,
			diagramHash,
			diagramHash,
			diagramHash,
			fontFamily.Font(0, d2fonts.FONT_STYLE_ITALIC).GetEncodedSubset(corpus),
		),
	)

	appendOnTrigger(
		buf,
		source,
		[]string{
			`class="text-mono`,
			`<pre>`,
			`<code>`,
			`<kbd>`,
			`<samp>`,
		},
		fmt.Sprintf(`
.%s .text-mono {
	font-family: "%s-font-mono";
}
@font-face {
	font-family: %s-font-mono;
	src: url("%s");
}`,
			diagramHash,
			diagramHash,
			diagramHash,
			d2fonts.SourceCodePro.Font(0, d2fonts.FONT_STYLE_REGULAR).GetEncodedSubset(corpus),
		),
	)

	appendOnTrigger(
		buf,
		source,
		[]string{
			`class="text-mono-bold`,
		},
		fmt.Sprintf(`
.%s .text-mono-bold {
	font-family: "%s-font-mono-bold";
}
@font-face {
	font-family: %s-font-mono-bold;
	src: url("%s");
}`,
			diagramHash,
			diagramHash,
			diagramHash,
			d2fonts.SourceCodePro.Font(0, d2fonts.FONT_STYLE_BOLD).GetEncodedSubset(corpus),
		),
	)

	appendOnTrigger(
		buf,
		source,
		[]string{
			`class="text-mono-italic`,
		},
		fmt.Sprintf(`
.%s .text-mono-italic {
	font-family: "%s-font-mono-italic";
}
@font-face {
	font-family: %s-font-mono-italic;
	src: url("%s");
}`,
			diagramHash,
			diagramHash,
			diagramHash,
			d2fonts.SourceCodePro.Font(0, d2fonts.FONT_STYLE_ITALIC).GetEncodedSubset(corpus),
		),
	)

	appendOnTrigger(
		buf,
		source,
		[]string{
			`sketch-overlay-bright`,
		},
		fmt.Sprintf(`
.sketch-overlay-bright {
	fill: url(#streaks-bright-%s);
	mix-blend-mode: darken;
}`, diagramHash),
	)

	appendOnTrigger(
		buf,
		source,
		[]string{
			`sketch-overlay-normal`,
		},
		fmt.Sprintf(`
.sketch-overlay-normal {
	fill: url(#streaks-normal-%s);
	mix-blend-mode: color-burn;
}`, diagramHash),
	)

	appendOnTrigger(
		buf,
		source,
		[]string{
			`sketch-overlay-dark`,
		},
		fmt.Sprintf(`
.sketch-overlay-dark {
	fill: url(#streaks-dark-%s);
	mix-blend-mode: overlay;
}`, diagramHash),
	)

	appendOnTrigger(
		buf,
		source,
		[]string{
			`sketch-overlay-darker`,
		},
		fmt.Sprintf(`
.sketch-overlay-darker {
	fill: url(#streaks-darker-%s);
	mix-blend-mode: lighten;
}`, diagramHash),
	)

	fmt.Fprint(buf, `]]></style>`)
}

func appendOnTrigger(buf *bytes.Buffer, source string, triggers []string, newContent string) {
	for _, trigger := range triggers {
		if strings.Contains(source, trigger) {
			fmt.Fprint(buf, newContent)
			break
		}
	}
}

var DEFAULT_DARK_THEME *int64 = nil // no theme selected

func Render(diagram *d2target.Diagram, opts *RenderOpts) ([]byte, error) {
	var jsRunner jsrunner.JSRunner
	pad := DEFAULT_PADDING
	themeID := d2themescatalog.NeutralDefault.ID
	darkThemeID := DEFAULT_DARK_THEME
	var scale *float64
	if opts != nil {
		if opts.Pad != nil {
			pad = int(*opts.Pad)
		}
		if opts.Sketch != nil && *opts.Sketch {
			jsRunner = jsrunner.NewJSRunner()
			err := d2sketch.LoadJS(jsRunner)
			if err != nil {
				return nil, err
			}
		}
		if opts.ThemeID != nil {
			themeID = *opts.ThemeID
		}
		darkThemeID = opts.DarkThemeID
		scale = opts.Scale
	} else {
		opts = &RenderOpts{}
	}

	buf := &bytes.Buffer{}

	// only define shadow filter if a shape uses it
	for _, s := range diagram.Shapes {
		if s.Shadow {
			defineShadowFilter(buf)
			break
		}
	}

	if color.IsGradient(diagram.Root.Fill) {
		defineGradients(buf, diagram.Root.Fill)
	}
	if color.IsGradient(diagram.Root.Stroke) {
		defineGradients(buf, diagram.Root.Stroke)
	}
	for _, s := range diagram.Shapes {
		if color.IsGradient(s.Fill) {
			defineGradients(buf, s.Fill)
		}
		if color.IsGradient(s.Stroke) {
			defineGradients(buf, s.Stroke)
		}
		if color.IsGradient(s.Color) {
			defineGradients(buf, s.Color)
		}
	}
	for _, c := range diagram.Connections {
		if color.IsGradient(c.Stroke) {
			defineGradients(buf, c.Stroke)
		}
		if color.IsGradient(c.Fill) {
			defineGradients(buf, c.Fill)
		}
	}

	// Apply hash on IDs for targeting, to be specific for this diagram
	diagramHash, err := diagram.HashID(opts.Salt)
	if err != nil {
		return nil, err
	}
	// Some targeting is still per-board, like masks for connections
	isolatedDiagramHash := diagramHash
	if opts != nil && opts.MasterID != "" {
		diagramHash = opts.MasterID
	}

	// SVG has no notion of z-index. The z-index is effectively the order it's drawn.
	// So draw from the least nested to most nested
	idToShape := make(map[string]d2target.Shape)
	allObjects := make([]DiagramObject, 0, len(diagram.Shapes)+len(diagram.Connections))
	for _, s := range diagram.Shapes {
		idToShape[s.ID] = s
		allObjects = append(allObjects, s)
	}
	for _, c := range diagram.Connections {
		allObjects = append(allObjects, c)
	}

	sortObjects(allObjects)

	appendixItemBuf := &bytes.Buffer{}

	var labelMasks []string
	markers := map[string]struct{}{}
	var inlineTheme *d2themes.Theme
	// We only want to inline when no dark theme is specified, otherwise the inline style will override the CSS
	if darkThemeID == nil {
		inlineTheme = go2.Pointer(d2themescatalog.Find(themeID))
		inlineTheme.ApplyOverrides(opts.ThemeOverrides)
	}
	for _, obj := range allObjects {
		if c, is := obj.(d2target.Connection); is {
			labelMask, err := drawConnection(buf, isolatedDiagramHash, c, markers, idToShape, jsRunner, inlineTheme)
			if err != nil {
				return nil, err
			}
			if labelMask != "" {
				labelMasks = append(labelMasks, labelMask)
			}
		} else if s, is := obj.(d2target.Shape); is {
			labelMask, err := drawShape(buf, appendixItemBuf, diagramHash, s, jsRunner, inlineTheme)
			if err != nil {
				return nil, err
			} else if labelMask != "" {
				labelMasks = append(labelMasks, labelMask)
			}
		} else {
			return nil, fmt.Errorf("unknown object of type %T", obj)
		}
	}
	// add all appendix items afterwards so they are always on top
	fmt.Fprint(buf, appendixItemBuf)

	if diagram.Legend != nil && (len(diagram.Legend.Shapes) > 0 || len(diagram.Legend.Connections) > 0) {
		legendBuf := &bytes.Buffer{}
		err := RenderLegend(legendBuf, diagram, diagramHash, inlineTheme)
		if err != nil {
			return nil, err
		}
		fmt.Fprint(buf, legendBuf)
	}

	// Note: we always want this since we reference it on connections even if there end up being no masked labels
	left, top, w, h := dimensions(diagram, pad)

	if diagram.Legend != nil && (len(diagram.Legend.Shapes) > 0 || len(diagram.Legend.Connections) > 0) {
		tl, br := diagram.BoundingBox()
		totalHeight := LEGEND_PADDING + LEGEND_FONT_SIZE + LEGEND_ITEM_SPACING
		maxLabelWidth := 0
		itemCount := 0
		ruler, _ := textmeasure.NewRuler()
		if ruler != nil {
			for _, s := range diagram.Legend.Shapes {
				if s.Label == "" {
					continue
				}
				mtext := &d2target.MText{
					Text:     s.Label,
					FontSize: LEGEND_FONT_SIZE,
				}
				dims := d2graph.GetTextDimensions(nil, ruler, mtext, nil)
				maxLabelWidth = go2.IntMax(maxLabelWidth, dims.Width)
				totalHeight += go2.IntMax(dims.Height, LEGEND_ICON_SIZE) + LEGEND_ITEM_SPACING
				itemCount++
			}

			for _, c := range diagram.Legend.Connections {
				if c.Label == "" {
					continue
				}
				mtext := &d2target.MText{
					Text:     c.Label,
					FontSize: LEGEND_FONT_SIZE,
				}
				dims := d2graph.GetTextDimensions(nil, ruler, mtext, nil)
				maxLabelWidth = go2.IntMax(maxLabelWidth, dims.Width)
				totalHeight += go2.IntMax(dims.Height, LEGEND_ICON_SIZE) + LEGEND_ITEM_SPACING
				itemCount++
			}

			if itemCount > 0 {
				totalHeight -= LEGEND_ITEM_SPACING / 2
			}

			totalHeight += LEGEND_PADDING

			if totalHeight > 0 && maxLabelWidth > 0 {
				legendWidth := LEGEND_PADDING*2 + LEGEND_ICON_SIZE + LEGEND_PADDING + maxLabelWidth

				legendY := br.Y - totalHeight
				if legendY < tl.Y {
					legendY = tl.Y
				}

				legendRight := br.X + LEGEND_CORNER_PADDING + legendWidth
				if left+w < legendRight {
					w = legendRight - left + pad/2
				}

				if legendY < top {
					diffY := top - legendY
					top -= diffY
					h += diffY
				}

				legendBottom := legendY + totalHeight
				if top+h < legendBottom {
					h = legendBottom - top + pad/2
				}
			}
		}
	}
	fmt.Fprint(buf, strings.Join([]string{
		fmt.Sprintf(`<mask id="%s" maskUnits="userSpaceOnUse" x="%d" y="%d" width="%d" height="%d">`,
			isolatedDiagramHash, left, top, w, h,
		),
		fmt.Sprintf(`<rect x="%d" y="%d" width="%d" height="%d" fill="white"></rect>`,
			left, top, w, h,
		),
		strings.Join(labelMasks, "\n"),
		`</mask>`,
	}, "\n"))

	// generate style elements that will be appended to the SVG tag
	upperBuf := &bytes.Buffer{}
	if opts.MasterID == "" {
		EmbedFonts(upperBuf, diagramHash, buf.String(), diagram.FontFamily, diagram.GetCorpus()) // EmbedFonts *must* run before `d2sketch.DefineFillPatterns`, but after all elements are appended to `buf`
		themeStylesheet, err := ThemeCSS(diagramHash, &themeID, darkThemeID, opts.ThemeOverrides, opts.DarkThemeOverrides)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(upperBuf, `<style type="text/css"><![CDATA[%s%s]]></style>`, BaseStylesheet, themeStylesheet)

		hasMarkdown := false
		for _, s := range diagram.Shapes {
			if s.Language == "markdown" {
				hasMarkdown = true
				break
			}
		}
		if !hasMarkdown {
			for _, c := range diagram.Connections {
				if c.Language == "markdown" {
					hasMarkdown = true
					break
				}
			}
		}
		if hasMarkdown {
			css := MarkdownCSS
			css = strings.ReplaceAll(css, ".md", fmt.Sprintf(".%s .md", diagramHash))
			css = strings.ReplaceAll(css, "font-italic", fmt.Sprintf("%s-font-italic", diagramHash))
			css = strings.ReplaceAll(css, "font-bold", fmt.Sprintf("%s-font-bold", diagramHash))
			css = strings.ReplaceAll(css, "font-mono", fmt.Sprintf("%s-font-mono", diagramHash))
			css = strings.ReplaceAll(css, "font-regular", fmt.Sprintf("%s-font-regular", diagramHash))
			css = strings.ReplaceAll(css, "font-semibold", fmt.Sprintf("%s-font-semibold", diagramHash))
			fmt.Fprintf(upperBuf, `<style type="text/css">%s</style>`, css)
		}

		if jsRunner != nil {
			d2sketch.DefineFillPatterns(upperBuf, diagramHash)
		}
	}

	// This shift is for background el to envelop the diagram
	left -= int(math.Ceil(float64(diagram.Root.StrokeWidth) / 2.))
	top -= int(math.Ceil(float64(diagram.Root.StrokeWidth) / 2.))
	w += int(math.Ceil(float64(diagram.Root.StrokeWidth)/2.) * 2.)
	h += int(math.Ceil(float64(diagram.Root.StrokeWidth)/2.) * 2.)
	backgroundEl := d2themes.NewThemableElement("rect", inlineTheme)
	// We don't want to change the document viewbox, only the background el
	backgroundEl.X = float64(left)
	backgroundEl.Y = float64(top)
	backgroundEl.Width = float64(w)
	backgroundEl.Height = float64(h)
	backgroundEl.Fill = diagram.Root.Fill
	backgroundEl.Stroke = diagram.Root.Stroke
	backgroundEl.FillPattern = diagram.Root.FillPattern
	backgroundEl.Rx = float64(diagram.Root.BorderRadius)
	if diagram.Root.StrokeDash != 0 {
		dashSize, gapSize := svg.GetStrokeDashAttributes(float64(diagram.Root.StrokeWidth), diagram.Root.StrokeDash)
		backgroundEl.StrokeDashArray = fmt.Sprintf("%f, %f", dashSize, gapSize)
	}
	backgroundEl.Attributes = fmt.Sprintf(`stroke-width="%d"`, diagram.Root.StrokeWidth)

	// This shift is for viewbox to envelop the background el
	left -= int(math.Ceil(float64(diagram.Root.StrokeWidth) / 2.))
	top -= int(math.Ceil(float64(diagram.Root.StrokeWidth) / 2.))
	w += int(math.Ceil(float64(diagram.Root.StrokeWidth)/2.) * 2.)
	h += int(math.Ceil(float64(diagram.Root.StrokeWidth)/2.) * 2.)

	doubleBorderElStr := ""
	if diagram.Root.DoubleBorder {
		offset := d2target.INNER_BORDER_OFFSET

		left -= int(math.Ceil(float64(diagram.Root.StrokeWidth)/2.)) + offset
		top -= int(math.Ceil(float64(diagram.Root.StrokeWidth)/2.)) + offset
		w += int(math.Ceil(float64(diagram.Root.StrokeWidth)/2.)*2.) + 2*offset
		h += int(math.Ceil(float64(diagram.Root.StrokeWidth)/2.)*2.) + 2*offset

		backgroundEl2 := backgroundEl.Copy()
		// No need to double-paint
		backgroundEl.Fill = "transparent"

		backgroundEl2.X = float64(left)
		backgroundEl2.Y = float64(top)
		backgroundEl2.Width = float64(w)
		backgroundEl2.Height = float64(h)
		doubleBorderElStr = backgroundEl2.Render()

		left -= int(math.Ceil(float64(diagram.Root.StrokeWidth) / 2.))
		top -= int(math.Ceil(float64(diagram.Root.StrokeWidth) / 2.))
		w += int(math.Ceil(float64(diagram.Root.StrokeWidth)/2.) * 2.)
		h += int(math.Ceil(float64(diagram.Root.StrokeWidth)/2.) * 2.)
	}

	bufStr := buf.String()
	patternDefs := ""
	for _, pattern := range d2ast.FillPatterns {
		if strings.Contains(bufStr, fmt.Sprintf("%s-overlay", pattern)) || diagram.Root.FillPattern == pattern {
			if patternDefs == "" {
				fmt.Fprint(upperBuf, `<style type="text/css"><![CDATA[`)
			}
			switch pattern {
			case "dots":
				patternDefs += fmt.Sprintf(dots, diagramHash)
			case "lines":
				patternDefs += fmt.Sprintf(lines, diagramHash)
			case "grain":
				patternDefs += fmt.Sprintf(grain, diagramHash)
			case "paper":
				patternDefs += fmt.Sprintf(paper, diagramHash)
			}
			fmt.Fprintf(upperBuf, `
.%s-overlay {
	fill: url(#%s-%s);
	mix-blend-mode: multiply;
}`, pattern, pattern, diagramHash)
		}
	}
	if patternDefs != "" {
		fmt.Fprint(upperBuf, `]]></style>`)
		fmt.Fprint(upperBuf, "<defs>")
		fmt.Fprint(upperBuf, patternDefs)
		fmt.Fprint(upperBuf, "</defs>")
	}

	var dimensions string
	if scale != nil {
		dimensions = fmt.Sprintf(` width="%d" height="%d"`,
			int(math.Ceil((*scale)*float64(w))),
			int(math.Ceil((*scale)*float64(h))),
		)
	}

	alignment := "xMinYMin"
	if opts.Center != nil && *opts.Center {
		alignment = "xMidYMid"
	}
	fitToScreenWrapperOpening := ""
	xmlTag := ""
	fitToScreenWrapperClosing := ""
	idAttr := ""
	tag := "g"
	// Many things change when this is rendering for animation
	if opts.MasterID == "" {
		dataD2Version := ""
		if opts.OmitVersion == nil || !*opts.OmitVersion {
			dataD2Version = fmt.Sprintf(`data-d2-version="%s"`, version.Version)
		}
		fitToScreenWrapperOpening = fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" %s preserveAspectRatio="%s meet" viewBox="0 0 %d %d"%s>`,
			dataD2Version,
			alignment,
			w, h,
			dimensions,
		)
		if opts.NoXMLTag == nil || !*opts.NoXMLTag {
			xmlTag = `<?xml version="1.0" encoding="utf-8"?>`
		}
		fitToScreenWrapperClosing = "</svg>"
		idAttr = `d2-svg`
		tag = "svg"
	}

	// TODO minify
	docRendered := fmt.Sprintf(`%s%s<%s class="%s" width="%d" height="%d" viewBox="%d %d %d %d">%s%s%s%s</%s>%s`,
		xmlTag,
		fitToScreenWrapperOpening,
		tag,
		strings.Join([]string{diagramHash, idAttr}, " "),
		w, h, left, top, w, h,
		doubleBorderElStr,
		backgroundEl.Render(),
		upperBuf.String(),
		buf.String(),
		tag,
		fitToScreenWrapperClosing,
	)
	return []byte(docRendered), nil
}

// TODO include only colors that are being used to reduce size
func ThemeCSS(diagramHash string, themeID *int64, darkThemeID *int64, overrides, darkOverrides *d2target.ThemeOverrides) (stylesheet string, err error) {
	if themeID == nil {
		themeID = &d2themescatalog.NeutralDefault.ID
	}
	out, err := singleThemeRulesets(diagramHash, *themeID, overrides)
	if err != nil {
		return "", err
	}

	if darkThemeID != nil {
		darkOut, err := singleThemeRulesets(diagramHash, *darkThemeID, darkOverrides)
		if err != nil {
			return "", err
		}
		out += fmt.Sprintf("@media screen and (prefers-color-scheme:dark){%s}", darkOut)
	}

	return out, nil
}

func singleThemeRulesets(diagramHash string, themeID int64, overrides *d2target.ThemeOverrides) (rulesets string, err error) {
	out := ""
	theme := d2themescatalog.Find(themeID)
	theme.ApplyOverrides(overrides)

	// Global theme colors
	for _, property := range []string{"fill", "stroke", "background-color", "color"} {
		out += fmt.Sprintf(`
		.%s .%s-N1{%s:%s;}
		.%s .%s-N2{%s:%s;}
		.%s .%s-N3{%s:%s;}
		.%s .%s-N4{%s:%s;}
		.%s .%s-N5{%s:%s;}
		.%s .%s-N6{%s:%s;}
		.%s .%s-N7{%s:%s;}
		.%s .%s-B1{%s:%s;}
		.%s .%s-B2{%s:%s;}
		.%s .%s-B3{%s:%s;}
		.%s .%s-B4{%s:%s;}
		.%s .%s-B5{%s:%s;}
		.%s .%s-B6{%s:%s;}
		.%s .%s-AA2{%s:%s;}
		.%s .%s-AA4{%s:%s;}
		.%s .%s-AA5{%s:%s;}
		.%s .%s-AB4{%s:%s;}
		.%s .%s-AB5{%s:%s;}`,
			diagramHash,
			property, property, theme.Colors.Neutrals.N1,
			diagramHash,
			property, property, theme.Colors.Neutrals.N2,
			diagramHash,
			property, property, theme.Colors.Neutrals.N3,
			diagramHash,
			property, property, theme.Colors.Neutrals.N4,
			diagramHash,
			property, property, theme.Colors.Neutrals.N5,
			diagramHash,
			property, property, theme.Colors.Neutrals.N6,
			diagramHash,
			property, property, theme.Colors.Neutrals.N7,
			diagramHash,
			property, property, theme.Colors.B1,
			diagramHash,
			property, property, theme.Colors.B2,
			diagramHash,
			property, property, theme.Colors.B3,
			diagramHash,
			property, property, theme.Colors.B4,
			diagramHash,
			property, property, theme.Colors.B5,
			diagramHash,
			property, property, theme.Colors.B6,
			diagramHash,
			property, property, theme.Colors.AA2,
			diagramHash,
			property, property, theme.Colors.AA4,
			diagramHash,
			property, property, theme.Colors.AA5,
			diagramHash,
			property, property, theme.Colors.AB4,
			diagramHash,
			property, property, theme.Colors.AB5,
		)
	}

	// Appendix
	out += fmt.Sprintf(".appendix text.text{fill:%s}", theme.Colors.Neutrals.N1)

	// Markdown specific rulesets
	out += fmt.Sprintf(".md{--color-fg-default:%s;--color-fg-muted:%s;--color-fg-subtle:%s;--color-canvas-default:%s;--color-canvas-subtle:%s;--color-border-default:%s;--color-border-muted:%s;--color-neutral-muted:%s;--color-accent-fg:%s;--color-accent-emphasis:%s;--color-attention-subtle:%s;--color-danger-fg:%s;}",
		theme.Colors.Neutrals.N1, theme.Colors.Neutrals.N2, theme.Colors.Neutrals.N3,
		theme.Colors.Neutrals.N7, theme.Colors.Neutrals.N6,
		theme.Colors.B1, theme.Colors.B2,
		theme.Colors.Neutrals.N6,
		theme.Colors.B2, theme.Colors.B2,
		theme.Colors.Neutrals.N2, // TODO or N3 --color-attention-subtle
		"red",
	)

	// Sketch style specific rulesets
	// B
	lc, err := color.LuminanceCategory(theme.Colors.B1)
	if err != nil {
		return "", err
	}
	out += fmt.Sprintf(".sketch-overlay-%s{fill:url(#streaks-%s-%s);mix-blend-mode:%s}", color.B1, lc, diagramHash, blendMode(lc))
	lc, err = color.LuminanceCategory(theme.Colors.B2)
	if err != nil {
		return "", err
	}
	out += fmt.Sprintf(".sketch-overlay-%s{fill:url(#streaks-%s-%s);mix-blend-mode:%s}", color.B2, lc, diagramHash, blendMode(lc))
	lc, err = color.LuminanceCategory(theme.Colors.B3)
	if err != nil {
		return "", err
	}
	out += fmt.Sprintf(".sketch-overlay-%s{fill:url(#streaks-%s-%s);mix-blend-mode:%s}", color.B3, lc, diagramHash, blendMode(lc))
	lc, err = color.LuminanceCategory(theme.Colors.B4)
	if err != nil {
		return "", err
	}
	out += fmt.Sprintf(".sketch-overlay-%s{fill:url(#streaks-%s-%s);mix-blend-mode:%s}", color.B4, lc, diagramHash, blendMode(lc))
	lc, err = color.LuminanceCategory(theme.Colors.B5)
	if err != nil {
		return "", err
	}
	out += fmt.Sprintf(".sketch-overlay-%s{fill:url(#streaks-%s-%s);mix-blend-mode:%s}", color.B5, lc, diagramHash, blendMode(lc))
	lc, err = color.LuminanceCategory(theme.Colors.B6)
	if err != nil {
		return "", err
	}
	out += fmt.Sprintf(".sketch-overlay-%s{fill:url(#streaks-%s-%s);mix-blend-mode:%s}", color.B6, lc, diagramHash, blendMode(lc))

	// AA
	lc, err = color.LuminanceCategory(theme.Colors.AA2)
	if err != nil {
		return "", err
	}
	out += fmt.Sprintf(".sketch-overlay-%s{fill:url(#streaks-%s-%s);mix-blend-mode:%s}", color.AA2, lc, diagramHash, blendMode(lc))
	lc, err = color.LuminanceCategory(theme.Colors.AA4)
	if err != nil {
		return "", err
	}
	out += fmt.Sprintf(".sketch-overlay-%s{fill:url(#streaks-%s-%s);mix-blend-mode:%s}", color.AA4, lc, diagramHash, blendMode(lc))
	lc, err = color.LuminanceCategory(theme.Colors.AA5)
	if err != nil {
		return "", err
	}
	out += fmt.Sprintf(".sketch-overlay-%s{fill:url(#streaks-%s-%s);mix-blend-mode:%s}", color.AA5, lc, diagramHash, blendMode(lc))

	// AB
	lc, err = color.LuminanceCategory(theme.Colors.AB4)
	if err != nil {
		return "", err
	}
	out += fmt.Sprintf(".sketch-overlay-%s{fill:url(#streaks-%s-%s);mix-blend-mode:%s}", color.AB4, lc, diagramHash, blendMode(lc))
	lc, err = color.LuminanceCategory(theme.Colors.AB5)
	if err != nil {
		return "", err
	}
	out += fmt.Sprintf(".sketch-overlay-%s{fill:url(#streaks-%s-%s);mix-blend-mode:%s}", color.AB5, lc, diagramHash, blendMode(lc))

	// Neutrals
	lc, err = color.LuminanceCategory(theme.Colors.Neutrals.N1)
	if err != nil {
		return "", err
	}
	out += fmt.Sprintf(".sketch-overlay-%s{fill:url(#streaks-%s-%s);mix-blend-mode:%s}", color.N1, lc, diagramHash, blendMode(lc))
	lc, err = color.LuminanceCategory(theme.Colors.Neutrals.N2)
	if err != nil {
		return "", err
	}
	out += fmt.Sprintf(".sketch-overlay-%s{fill:url(#streaks-%s-%s);mix-blend-mode:%s}", color.N2, lc, diagramHash, blendMode(lc))
	lc, err = color.LuminanceCategory(theme.Colors.Neutrals.N3)
	if err != nil {
		return "", err
	}
	out += fmt.Sprintf(".sketch-overlay-%s{fill:url(#streaks-%s-%s);mix-blend-mode:%s}", color.N3, lc, diagramHash, blendMode(lc))
	lc, err = color.LuminanceCategory(theme.Colors.Neutrals.N4)
	if err != nil {
		return "", err
	}
	out += fmt.Sprintf(".sketch-overlay-%s{fill:url(#streaks-%s-%s);mix-blend-mode:%s}", color.N4, lc, diagramHash, blendMode(lc))
	lc, err = color.LuminanceCategory(theme.Colors.Neutrals.N5)
	if err != nil {
		return "", err
	}
	out += fmt.Sprintf(".sketch-overlay-%s{fill:url(#streaks-%s-%s);mix-blend-mode:%s}", color.N5, lc, diagramHash, blendMode(lc))
	lc, err = color.LuminanceCategory(theme.Colors.Neutrals.N6)
	if err != nil {
		return "", err
	}
	out += fmt.Sprintf(".sketch-overlay-%s{fill:url(#streaks-%s-%s);mix-blend-mode:%s}", color.N6, lc, diagramHash, blendMode(lc))
	lc, err = color.LuminanceCategory(theme.Colors.Neutrals.N7)
	if err != nil {
		return "", err
	}
	out += fmt.Sprintf(".sketch-overlay-%s{fill:url(#streaks-%s-%s);mix-blend-mode:%s}", color.N7, lc, diagramHash, blendMode(lc))

	if theme.IsDark() {
		out += ".light-code{display: none}"
		out += ".dark-code{display: block}"
	} else {
		out += ".light-code{display: block}"
		out += ".dark-code{display: none}"
	}

	return out, nil
}

func blendMode(lc string) string {
	switch lc {
	case "bright":
		return "darken"
	case "normal":
		return "color-burn"
	case "dark":
		return "overlay"
	case "darker":
		return "lighten"
	}
	panic("invalid luminance category")
}

type DiagramObject interface {
	GetID() string
	GetZIndex() int
}

// sortObjects sorts all diagrams objects (shapes and connections) in the desired drawing order
// the sorting criteria is:
// 1. zIndex, lower comes first
// 2. two shapes with the same zIndex are sorted by their level (container nesting), containers come first
// 3. two shapes with the same zIndex and same level, are sorted in the order they were exported
// 4. shape and edge, shapes come first
func sortObjects(allObjects []DiagramObject) {
	sort.SliceStable(allObjects, func(i, j int) bool {
		// first sort by zIndex
		iZIndex := allObjects[i].GetZIndex()
		jZIndex := allObjects[j].GetZIndex()
		if iZIndex != jZIndex {
			return iZIndex < jZIndex
		}

		// then, if both are shapes, parents come before their children
		iShape, iIsShape := allObjects[i].(d2target.Shape)
		jShape, jIsShape := allObjects[j].(d2target.Shape)
		if iIsShape && jIsShape {
			return iShape.Level < jShape.Level
		}

		// then, shapes come before connections
		_, jIsConnection := allObjects[j].(d2target.Connection)
		return iIsShape && jIsConnection
	})
}

func hash(s string) string {
	const secret = "lalalas"
	h := fnv.New32a()
	h.Write([]byte(fmt.Sprintf("%s%s", s, secret)))
	return fmt.Sprint(h.Sum32())
}

func RenderMultiboard(diagram *d2target.Diagram, opts *RenderOpts) ([][]byte, error) {
	var boards [][]byte
	for _, dl := range diagram.Layers {
		childrenBoards, err := RenderMultiboard(dl, opts)
		if err != nil {
			return nil, err
		}
		boards = append(boards, childrenBoards...)
	}
	for _, dl := range diagram.Scenarios {
		childrenBoards, err := RenderMultiboard(dl, opts)
		if err != nil {
			return nil, err
		}
		boards = append(boards, childrenBoards...)
	}
	for _, dl := range diagram.Steps {
		childrenBoards, err := RenderMultiboard(dl, opts)
		if err != nil {
			return nil, err
		}
		boards = append(boards, childrenBoards...)
	}

	if !diagram.IsFolderOnly {
		out, err := Render(diagram, opts)
		if err != nil {
			return boards, err
		}
		boards = append([][]byte{out}, boards...)
		return boards, nil
	}
	return boards, nil
}
