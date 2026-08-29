package d2sketch

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	_ "embed"

	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/d2themes"
	"github.com/d2lang/d2/lib/color"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
	"github.com/d2lang/d2/lib/svg"
	rough "github.com/d2lang/rough-go"
	"github.com/d2lang/util-go/go2"
)

//go:embed streaks.txt
var streaks string

var floatRE = regexp.MustCompile(`(\d+)\.(\d+)`)

const (
	BG_COLOR = color.N7
	FG_COLOR = color.N1
)

func newGenerator() *rough.Generator {
	return rough.NewGenerator(&rough.Config{Options: &rough.Options{Seed: rough.Float64(1)}})
}

func baseOptions(strokeWidth float64) *rough.Options {
	return &rough.Options{
		Fill:        rough.String("#000"),
		Stroke:      rough.String("#000"),
		StrokeWidth: rough.Float64(strokeWidth),
		FillWeight:  rough.Float64(2),
		HachureGap:  rough.Float64(16),
		FillStyle:   rough.String("solid"),
		Bowing:      rough.Float64(2),
		Seed:        rough.Float64(1),
	}
}

// DefineFillPatterns adds reusable patterns that are overlayed on shapes with
// fill. This gives it a subtle streaky effect that subtly looks hand-drawn but
// not distractingly so.
func DefineFillPatterns(buf *bytes.Buffer, diagramHash string) {
	source := buf.String()
	fmt.Fprint(buf, "<defs>")

	defineFillPattern(buf, source, diagramHash, "bright", "rgba(0, 0, 0, 0.1)")
	defineFillPattern(buf, source, diagramHash, "normal", "rgba(0, 0, 0, 0.16)")
	defineFillPattern(buf, source, diagramHash, "dark", "rgba(0, 0, 0, 0.32)")
	defineFillPattern(buf, source, diagramHash, "darker", "rgba(255, 255, 255, 0.24)")

	fmt.Fprint(buf, "</defs>")
}

func defineFillPattern(buf *bytes.Buffer, source, diagramHash string, luminanceCategory, fill string) {
	trigger := fmt.Sprintf(`url(#streaks-%s-%s)`, luminanceCategory, diagramHash)
	if strings.Contains(source, trigger) {
		fmt.Fprintf(buf, streaks, luminanceCategory, diagramHash, fill)
	}
}

func Rect(shape d2target.Shape, diagramHash string) (string, error) {
	g := newGenerator()
	paths, err := computeRoughPathData(g, g.Rectangle(0, 0, float64(shape.Width), float64(shape.Height), baseOptions(float64(shape.StrokeWidth))))
	if err != nil {
		return "", err
	}
	output := ""
	pathEl := d2themes.NewThemableElement("path", nil)
	pathEl.SetTranslate(float64(shape.Pos.X), float64(shape.Pos.Y))
	pathEl.Fill, pathEl.Stroke = d2themes.ShapeTheme(shape)
	pathEl.FillPattern = shape.FillPattern
	pathEl.ClassName = "shape"
	pathEl.Style = shape.CSSStyle()

	if shape.Label != "" && label.FromString(shape.LabelPosition).IsBorder() {
		pathEl.Mask = fmt.Sprintf("url(#%s)", diagramHash)
	}

	for _, p := range paths {
		pathEl.D = p
		output += pathEl.Render()
	}

	sketchOEl := d2themes.NewThemableElement("rect", nil)
	sketchOEl.SetTranslate(float64(shape.Pos.X), float64(shape.Pos.Y))
	sketchOEl.Width = float64(shape.Width)
	sketchOEl.Height = float64(shape.Height)

	if shape.Label != "" && label.FromString(shape.LabelPosition).IsBorder() {
		sketchOEl.Mask = fmt.Sprintf("url(#%s)", diagramHash)
	}

	renderedSO, err := d2themes.NewThemableSketchOverlay(sketchOEl, pathEl.Fill).Render()
	if err != nil {
		return "", err
	}
	output += renderedSO

	return output, nil
}

func DoubleRect(shape d2target.Shape, diagramHash string) (string, error) {
	g := newGenerator()
	pathsBigRect, err := computeRoughPathData(g, g.Rectangle(0, 0, float64(shape.Width), float64(shape.Height), baseOptions(float64(shape.StrokeWidth))))
	if err != nil {
		return "", err
	}
	pathsSmallRect, err := computeRoughPathData(g, g.Rectangle(0, 0,
		float64(shape.Width-d2target.INNER_BORDER_OFFSET*2),
		float64(shape.Height-d2target.INNER_BORDER_OFFSET*2),
		baseOptions(float64(shape.StrokeWidth))))
	if err != nil {
		return "", err
	}

	output := ""

	pathEl := d2themes.NewThemableElement("path", nil)
	pathEl.SetTranslate(float64(shape.Pos.X), float64(shape.Pos.Y))
	pathEl.Fill, pathEl.Stroke = d2themes.ShapeTheme(shape)
	pathEl.FillPattern = shape.FillPattern
	pathEl.ClassName = "shape"
	pathEl.Style = shape.CSSStyle()

	if shape.Label != "" && label.FromString(shape.LabelPosition).IsBorder() {
		pathEl.Mask = fmt.Sprintf("url(#%s)", diagramHash)
	}

	for _, p := range pathsBigRect {
		pathEl.D = p
		output += pathEl.Render()
	}

	pathEl = d2themes.NewThemableElement("path", nil)
	pathEl.SetTranslate(float64(shape.Pos.X+d2target.INNER_BORDER_OFFSET), float64(shape.Pos.Y+d2target.INNER_BORDER_OFFSET))
	pathEl.Fill, pathEl.Stroke = d2themes.ShapeTheme(shape)
	// No need for inner to double paint
	pathEl.Fill = "transparent"
	pathEl.ClassName = "shape"
	pathEl.Style = shape.CSSStyle()

	if shape.Label != "" && label.FromString(shape.LabelPosition).IsBorder() {
		pathEl.Mask = fmt.Sprintf("url(#%s)", diagramHash)
	}

	for _, p := range pathsSmallRect {
		pathEl.D = p
		output += pathEl.Render()
	}

	sketchOEl := d2themes.NewThemableElement("rect", nil)
	sketchOEl.SetTranslate(float64(shape.Pos.X), float64(shape.Pos.Y))
	sketchOEl.Width = float64(shape.Width)
	sketchOEl.Height = float64(shape.Height)

	if shape.Label != "" && label.FromString(shape.LabelPosition).IsBorder() {
		sketchOEl.Mask = fmt.Sprintf("url(#%s)", diagramHash)
	}

	renderedSO, err := d2themes.NewThemableSketchOverlay(sketchOEl, shape.Fill).Render()
	if err != nil {
		return "", err
	}
	output += renderedSO

	return output, nil
}

func Oval(shape d2target.Shape, diagramHash string) (string, error) {
	g := newGenerator()
	paths, err := computeRoughPathData(g, g.Ellipse(
		float64(shape.Width/2), float64(shape.Height/2),
		float64(shape.Width), float64(shape.Height), baseOptions(float64(shape.StrokeWidth))))
	if err != nil {
		return "", err
	}
	output := ""
	pathEl := d2themes.NewThemableElement("path", nil)
	pathEl.SetTranslate(float64(shape.Pos.X), float64(shape.Pos.Y))
	pathEl.Fill, pathEl.Stroke = d2themes.ShapeTheme(shape)
	pathEl.FillPattern = shape.FillPattern
	pathEl.ClassName = "shape"
	pathEl.Style = shape.CSSStyle()

	if shape.Label != "" && label.FromString(shape.LabelPosition).IsBorder() {
		pathEl.Mask = fmt.Sprintf("url(#%s)", diagramHash)
	}

	for _, p := range paths {
		pathEl.D = p
		output += pathEl.Render()
	}

	soElement := d2themes.NewThemableElement("ellipse", nil)
	soElement.SetTranslate(float64(shape.Pos.X+shape.Width/2), float64(shape.Pos.Y+shape.Height/2))
	soElement.Rx = float64(shape.Width / 2)
	soElement.Ry = float64(shape.Height / 2)

	if shape.Label != "" && label.FromString(shape.LabelPosition).IsBorder() {
		soElement.Mask = fmt.Sprintf("url(#%s)", diagramHash)
	}

	renderedSO, err := d2themes.NewThemableSketchOverlay(
		soElement,
		pathEl.Fill,
	).Render()
	if err != nil {
		return "", err
	}
	output += renderedSO

	return output, nil
}

func DoubleOval(shape d2target.Shape, diagramHash string) (string, error) {
	g := newGenerator()
	pathsBigCircle, err := computeRoughPathData(g, g.Ellipse(
		float64(shape.Width/2), float64(shape.Height/2),
		float64(shape.Width), float64(shape.Height), baseOptions(float64(shape.StrokeWidth))))
	if err != nil {
		return "", err
	}
	pathsSmallCircle, err := computeRoughPathData(g, g.Ellipse(
		float64(shape.Width/2), float64(shape.Height/2),
		float64(shape.Width-d2target.INNER_BORDER_OFFSET*2),
		float64(shape.Height-d2target.INNER_BORDER_OFFSET*2),
		baseOptions(float64(shape.StrokeWidth))))
	if err != nil {
		return "", err
	}

	output := ""

	pathEl := d2themes.NewThemableElement("path", nil)
	pathEl.SetTranslate(float64(shape.Pos.X), float64(shape.Pos.Y))
	pathEl.Fill, pathEl.Stroke = d2themes.ShapeTheme(shape)
	pathEl.FillPattern = shape.FillPattern
	pathEl.ClassName = "shape"
	pathEl.Style = shape.CSSStyle()

	if shape.Label != "" && label.FromString(shape.LabelPosition).IsBorder() {
		pathEl.Mask = fmt.Sprintf("url(#%s)", diagramHash)
	}

	for _, p := range pathsBigCircle {
		pathEl.D = p
		output += pathEl.Render()
	}

	pathEl = d2themes.NewThemableElement("path", nil)
	pathEl.SetTranslate(float64(shape.Pos.X), float64(shape.Pos.Y))
	pathEl.Fill, pathEl.Stroke = d2themes.ShapeTheme(shape)
	// No need for inner to double paint
	pathEl.Fill = "transparent"
	pathEl.ClassName = "shape"
	pathEl.Style = shape.CSSStyle()

	if shape.Label != "" && label.FromString(shape.LabelPosition).IsBorder() {
		pathEl.Mask = fmt.Sprintf("url(#%s)", diagramHash)
	}

	for _, p := range pathsSmallCircle {
		pathEl.D = p
		output += pathEl.Render()
	}
	soElement := d2themes.NewThemableElement("ellipse", nil)
	soElement.SetTranslate(float64(shape.Pos.X+shape.Width/2), float64(shape.Pos.Y+shape.Height/2))
	soElement.Rx = float64(shape.Width / 2)
	soElement.Ry = float64(shape.Height / 2)

	if shape.Label != "" && label.FromString(shape.LabelPosition).IsBorder() {
		soElement.Mask = fmt.Sprintf("url(#%s)", diagramHash)
	}

	renderedSO, err := d2themes.NewThemableSketchOverlay(
		soElement,
		shape.Fill,
	).Render()
	if err != nil {
		return "", err
	}
	output += renderedSO

	return output, nil
}

// TODO need to personalize this per shape like we do in Terrastruct app
func Paths(shape d2target.Shape, diagramHash string, paths []string) (string, error) {
	output := ""
	for _, path := range paths {
		g := newGenerator()
		sketchPaths, err := computeRoughPathData(g, g.Path(path, baseOptions(float64(shape.StrokeWidth))))
		if err != nil {
			return "", err
		}
		pathEl := d2themes.NewThemableElement("path", nil)
		pathEl.Fill, pathEl.Stroke = d2themes.ShapeTheme(shape)
		pathEl.FillPattern = shape.FillPattern
		pathEl.ClassName = "shape"
		pathEl.Style = shape.CSSStyle()

		if shape.Label != "" && label.FromString(shape.LabelPosition).IsBorder() {
			pathEl.Mask = fmt.Sprintf("url(#%s)", diagramHash)
		}

		for _, p := range sketchPaths {
			pathEl.D = p
			output += pathEl.Render()
		}

		soElement := d2themes.NewThemableElement("path", nil)

		if shape.Label != "" && label.FromString(shape.LabelPosition).IsBorder() {
			soElement.Mask = fmt.Sprintf("url(#%s)", diagramHash)
		}

		for _, p := range sketchPaths {
			soElement.D = p
			renderedSO, err := d2themes.NewThemableSketchOverlay(
				soElement,
				pathEl.Fill,
			).Render()
			if err != nil {
				return "", err
			}
			output += renderedSO
		}
	}
	return output, nil
}

func Connection(connection d2target.Connection, path, attrs string) (string, error) {
	animatedClass := ""
	if connection.Animated {
		animatedClass = " animated-connection"
	}

	if connection.Animated {
		// If connection is animated and bidirectional
		if (connection.DstArrow == d2target.NoArrowhead && connection.SrcArrow == d2target.NoArrowhead) || (connection.DstArrow != d2target.NoArrowhead && connection.SrcArrow != d2target.NoArrowhead) {
			// There is no pure CSS way to animate bidirectional connections in two directions, so we split it up
			path1, path2, err := svg.SplitPath(path, 0.5)

			if err != nil {
				return "", err
			}

			pathEl1 := d2themes.NewThemableElement("path", nil)
			pathEl1.D = path1
			pathEl1.Fill = color.None
			pathEl1.Stroke = connection.Stroke
			pathEl1.ClassName = fmt.Sprintf("connection%s", animatedClass)
			pathEl1.Style = connection.CSSStyle()
			pathEl1.Style += "animation-direction: reverse;"
			pathEl1.Attributes = attrs

			pathEl2 := d2themes.NewThemableElement("path", nil)
			pathEl2.D = path2
			pathEl2.Fill = color.None
			pathEl2.Stroke = connection.Stroke
			pathEl2.ClassName = fmt.Sprintf("connection%s", animatedClass)
			pathEl2.Style = connection.CSSStyle()
			pathEl2.Attributes = attrs
			return pathEl1.Render() + " " + pathEl2.Render(), nil
		} else {
			pathEl := d2themes.NewThemableElement("path", nil)
			pathEl.D = path
			pathEl.Fill = color.None
			pathEl.Stroke = connection.Stroke
			pathEl.ClassName = fmt.Sprintf("connection%s", animatedClass)
			pathEl.Style = connection.CSSStyle()
			pathEl.Attributes = attrs
			return pathEl.Render(), nil
		}
	} else {
		roughness := 0.5
		g := newGenerator()
		paths, err := computeRoughPathData(g, g.Path(path, &rough.Options{
			Roughness: rough.Float64(roughness),
			Seed:      rough.Float64(1),
		}))
		if err != nil {
			return "", err
		}

		output := ""

		pathEl := d2themes.NewThemableElement("path", nil)
		pathEl.Fill = color.None
		pathEl.Stroke = connection.Stroke
		pathEl.ClassName = fmt.Sprintf("connection%s", animatedClass)
		pathEl.Style = connection.CSSStyle()
		pathEl.Attributes = attrs
		for _, p := range paths {
			pathEl.D = p
			output += pathEl.Render()
		}
		return output, nil
	}
}

// TODO cleanup
func Table(shape d2target.Shape) (string, error) {
	output := ""
	g := newGenerator()
	paths, err := computeRoughPathData(g, g.Rectangle(0, 0, float64(shape.Width), float64(shape.Height), baseOptions(float64(shape.StrokeWidth))))
	if err != nil {
		return "", err
	}
	pathEl := d2themes.NewThemableElement("path", nil)
	pathEl.SetTranslate(float64(shape.Pos.X), float64(shape.Pos.Y))
	pathEl.Fill, pathEl.Stroke = d2themes.ShapeTheme(shape)
	pathEl.FillPattern = shape.FillPattern
	pathEl.ClassName = "shape"
	pathEl.Style = shape.CSSStyle()
	for _, p := range paths {
		pathEl.D = p
		output += pathEl.Render()
	}

	box := geo.NewBox(
		geo.NewPoint(float64(shape.Pos.X), float64(shape.Pos.Y)),
		float64(shape.Width),
		float64(shape.Height),
	)
	rowHeight := box.Height / float64(1+len(shape.SQLTable.Columns))
	headerBox := geo.NewBox(box.TopLeft, box.Width, rowHeight)

	paths, err = computeRoughPathData(g, g.Rectangle(0, 0, float64(shape.Width), rowHeight, baseOptions(1)))
	if err != nil {
		return "", err
	}
	pathEl = d2themes.NewThemableElement("path", nil)
	pathEl.SetTranslate(float64(shape.Pos.X), float64(shape.Pos.Y))
	pathEl.Fill = shape.Fill
	pathEl.FillPattern = shape.FillPattern
	pathEl.ClassName = "class_header"
	for _, p := range paths {
		pathEl.D = p
		output += pathEl.Render()
	}

	if shape.Label != "" {
		tl := label.InsideMiddleLeft.GetPointOnBox(
			headerBox,
			20,
			float64(shape.LabelWidth),
			float64(shape.LabelHeight),
		)

		textEl := d2themes.NewThemableElement("text", nil)
		textEl.X = tl.X
		textEl.Y = tl.Y + float64(shape.LabelHeight)*3/4
		textEl.Fill = shape.GetFontColor()
		textEl.ClassName = "text"
		textEl.Style = fmt.Sprintf("text-anchor:%s;font-size:%vpx",
			"start", 4+shape.FontSize,
		)
		textEl.Content = svg.EscapeText(shape.Label)
		output += textEl.Render()
	}

	var longestNameWidth int
	for _, f := range shape.Columns {
		longestNameWidth = go2.Max(longestNameWidth, f.Name.LabelWidth)
	}

	rowBox := geo.NewBox(box.TopLeft.Copy(), box.Width, rowHeight)
	rowBox.TopLeft.Y += headerBox.Height
	for _, f := range shape.Columns {
		nameTL := label.InsideMiddleLeft.GetPointOnBox(
			rowBox,
			d2target.NamePadding,
			rowBox.Width,
			float64(shape.FontSize),
		)
		constraintTR := label.InsideMiddleRight.GetPointOnBox(
			rowBox,
			d2target.TypePadding,
			0,
			float64(shape.FontSize),
		)

		textEl := d2themes.NewThemableElement("text", nil)
		textEl.X = nameTL.X
		textEl.Y = nameTL.Y + float64(shape.FontSize)*3/4
		textEl.Fill = shape.PrimaryAccentColor
		textEl.ClassName = "text"
		textEl.Style = fmt.Sprintf("text-anchor:%s;font-size:%vpx", "start", float64(shape.FontSize))
		textEl.Content = svg.EscapeText(f.Name.Label)
		output += textEl.Render()

		textEl.X = nameTL.X + float64(longestNameWidth) + 2*d2target.NamePadding
		textEl.Fill = shape.NeutralAccentColor
		textEl.Content = svg.EscapeText(f.Type.Label)
		output += textEl.Render()

		textEl.X = constraintTR.X
		textEl.Y = constraintTR.Y + float64(shape.FontSize)*3/4
		textEl.Fill = shape.SecondaryAccentColor
		textEl.Style = fmt.Sprintf("text-anchor:%s;font-size:%vpx;letter-spacing:2px", "end", float64(shape.FontSize))
		textEl.Content = f.ConstraintAbbr()
		output += textEl.Render()

		rowBox.TopLeft.Y += rowHeight

		paths, err = computeRoughPathData(g, g.Line(
			rowBox.TopLeft.X, rowBox.TopLeft.Y,
			rowBox.TopLeft.X+rowBox.Width, rowBox.TopLeft.Y,
			baseOptions(1)))
		if err != nil {
			return "", err
		}
		pathEl := d2themes.NewThemableElement("path", nil)
		pathEl.Fill = shape.Fill
		pathEl.FillPattern = shape.FillPattern
		for _, p := range paths {
			pathEl.D = p
			output += pathEl.Render()
		}
	}

	sketchOEl := d2themes.NewThemableElement("rect", nil)
	sketchOEl.SetTranslate(float64(shape.Pos.X), float64(shape.Pos.Y))
	sketchOEl.Width = float64(shape.Width)
	sketchOEl.Height = float64(shape.Height)
	renderedSO, err := d2themes.NewThemableSketchOverlay(sketchOEl, pathEl.Fill).Render()
	if err != nil {
		return "", err
	}
	output += renderedSO

	return output, nil
}

func Class(shape d2target.Shape) (string, error) {
	output := ""
	g := newGenerator()
	paths, err := computeRoughPathData(g, g.Rectangle(0, 0, float64(shape.Width), float64(shape.Height), baseOptions(float64(shape.StrokeWidth))))
	if err != nil {
		return "", err
	}
	pathEl := d2themes.NewThemableElement("path", nil)
	pathEl.SetTranslate(float64(shape.Pos.X), float64(shape.Pos.Y))
	pathEl.Fill, pathEl.Stroke = d2themes.ShapeTheme(shape)
	pathEl.FillPattern = shape.FillPattern
	pathEl.ClassName = "shape"
	pathEl.Style = shape.CSSStyle()
	for _, p := range paths {
		pathEl.D = p
		output += pathEl.Render()
	}

	box := geo.NewBox(
		geo.NewPoint(float64(shape.Pos.X), float64(shape.Pos.Y)),
		float64(shape.Width),
		float64(shape.Height),
	)

	rowHeight := box.Height / float64(2+len(shape.Class.Fields)+len(shape.Class.Methods))
	headerBox := geo.NewBox(box.TopLeft, box.Width, 2*rowHeight)

	paths, err = computeRoughPathData(g, g.Rectangle(0, 0, float64(shape.Width), headerBox.Height, baseOptions(1)))
	if err != nil {
		return "", err
	}
	pathEl = d2themes.NewThemableElement("path", nil)
	pathEl.SetTranslate(float64(shape.Pos.X), float64(shape.Pos.Y))
	pathEl.Fill = shape.Fill
	pathEl.FillPattern = shape.FillPattern
	pathEl.ClassName = "class_header"
	for _, p := range paths {
		pathEl.D = p
		output += pathEl.Render()
	}

	sketchOEl := d2themes.NewThemableElement("rect", nil)
	sketchOEl.SetTranslate(float64(shape.Pos.X), float64(shape.Pos.Y))
	sketchOEl.Width = float64(shape.Width)
	sketchOEl.Height = headerBox.Height
	renderedSO, err := d2themes.NewThemableSketchOverlay(sketchOEl, pathEl.Fill).Render()
	if err != nil {
		return "", err
	}
	output += renderedSO

	if shape.Label != "" {
		tl := label.InsideMiddleCenter.GetPointOnBox(
			headerBox,
			0,
			float64(shape.LabelWidth),
			float64(shape.LabelHeight),
		)

		textEl := d2themes.NewThemableElement("text", nil)
		textEl.X = tl.X + float64(shape.LabelWidth)/2
		textEl.Y = tl.Y + float64(shape.LabelHeight)*3/4
		textEl.Fill = shape.GetFontColor()
		textEl.ClassName = "text-mono"
		textEl.Style = fmt.Sprintf("text-anchor:%s;font-size:%vpx",
			"middle",
			4+shape.FontSize,
		)
		textEl.Content = svg.EscapeText(shape.Label)
		output += textEl.Render()
	}

	rowBox := geo.NewBox(box.TopLeft.Copy(), box.Width, rowHeight)
	rowBox.TopLeft.Y += headerBox.Height
	for _, f := range shape.Fields {
		output += classRow(shape, rowBox, f.VisibilityToken(), f.Name, f.Type, float64(shape.FontSize))
		rowBox.TopLeft.Y += rowHeight
	}

	paths, err = computeRoughPathData(g, g.Line(
		rowBox.TopLeft.X, rowBox.TopLeft.Y,
		rowBox.TopLeft.X+rowBox.Width, rowBox.TopLeft.Y,
		baseOptions(1)))
	if err != nil {
		return "", err
	}
	pathEl = d2themes.NewThemableElement("path", nil)
	pathEl.Fill = shape.Fill
	pathEl.FillPattern = shape.FillPattern
	pathEl.ClassName = "class_header"
	for _, p := range paths {
		pathEl.D = p
		output += pathEl.Render()
	}

	for _, m := range shape.Methods {
		output += classRow(shape, rowBox, m.VisibilityToken(), m.Name, m.Return, float64(shape.FontSize))
		rowBox.TopLeft.Y += rowHeight
	}

	return output, nil
}

func classRow(shape d2target.Shape, box *geo.Box, prefix, nameText, typeText string, fontSize float64) string {
	output := ""
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

	textEl := d2themes.NewThemableElement("text", nil)
	textEl.X = prefixTL.X
	textEl.Y = prefixTL.Y + fontSize*3/4
	textEl.Fill = shape.PrimaryAccentColor
	textEl.ClassName = "text-mono"
	textEl.Style = fmt.Sprintf("text-anchor:%s;font-size:%vpx", "start", fontSize)
	textEl.Content = prefix
	output += textEl.Render()

	textEl.X = prefixTL.X + d2target.PrefixWidth
	textEl.Fill = shape.Fill
	textEl.Content = svg.EscapeText(nameText)
	output += textEl.Render()

	textEl.X = typeTR.X
	textEl.Y = typeTR.Y + fontSize*3/4
	textEl.Fill = shape.SecondaryAccentColor
	textEl.Style = fmt.Sprintf("text-anchor:%s;font-size:%vpx", "end", fontSize)
	textEl.Content = svg.EscapeText(typeText)
	output += textEl.Render()

	return output
}

func computeRoughPathData(g *rough.Generator, drawable rough.Drawable) ([]string, error) {
	return extractPathData(computeRoughPaths(g, drawable))
}

func computeRoughPaths(g *rough.Generator, drawable rough.Drawable) []roughPath {
	pathInfos := g.ToPaths(drawable)
	paths := make([]roughPath, 0, len(pathInfos))
	for _, path := range pathInfos {
		paths = append(paths, roughPath{
			Attrs: attrs{D: truncatePathData(path.D)},
			Style: style{
				Stroke:      path.Stroke,
				StrokeWidth: rough.NumberString(path.StrokeWidth),
				Fill:        path.Fill,
			},
		})
	}
	return paths
}

type attrs struct {
	D string `json:"d"`
}

type style struct {
	Stroke      string `json:"stroke,omitempty"`
	StrokeWidth string `json:"strokeWidth,omitempty"`
	Fill        string `json:"fill,omitempty"`
}

type roughPath struct {
	Attrs attrs `json:"attrs"`
	Style style `json:"style"`
}

func (rp roughPath) StyleCSS() string {
	style := ""
	if rp.Style.StrokeWidth != "" {
		style += fmt.Sprintf("stroke-width:%s;", rp.Style.StrokeWidth)
	}
	return style
}

func truncatePathData(pathData string) string {
	// we want to have a fixed precision to the decimals in the path data
	return floatRE.ReplaceAllStringFunc(pathData, func(floatStr string) string {
		i := strings.Index(floatStr, ".")
		decimalLen := len(floatStr) - i - 1
		end := i + go2.Min(decimalLen, 6)
		return floatStr[:end+1]
	})
}

func extractPathData(roughPaths []roughPath) ([]string, error) {
	var paths []string
	for _, rp := range roughPaths {
		paths = append(paths, rp.Attrs.D)
	}
	return paths, nil
}

func arrowOptions(stroke string, strokeWidth int, seed float64) *rough.Options {
	return &rough.Options{
		StrokeWidth: rough.Float64(float64(strokeWidth)),
		Stroke:      rough.String(stroke),
		Seed:        rough.Float64(seed),
	}
}

func solidArrowOptions(stroke, fill string, strokeWidth int, fillWeight, seed float64) *rough.Options {
	o := arrowOptions(stroke, strokeWidth, seed)
	o.Fill = rough.String(fill)
	o.FillStyle = rough.String("solid")
	if fillWeight >= 0 {
		o.FillWeight = rough.Float64(fillWeight)
	}
	return o
}

func arrowheadDrawable(g *rough.Generator, arrowhead d2target.Arrowhead, stroke string, strokeWidth int) (arrow rough.Drawable, extra *rough.Drawable, ok bool) {
	// Note: selected each seed that looks the good for consistent renders
	switch arrowhead {
	case d2target.ArrowArrowhead:
		arrow = g.LinearPath([]rough.Point{{-10, -4}, {0, 0}, {-10, 4}}, arrowOptions(stroke, strokeWidth, 3))
	case d2target.TriangleArrowhead:
		arrow = g.Polygon([]rough.Point{{-10, -4}, {0, 0}, {-10, 4}}, solidArrowOptions(stroke, stroke, strokeWidth, -1, 2))
	case d2target.UnfilledTriangleArrowhead:
		arrow = g.Polygon([]rough.Point{{-10, -4}, {0, 0}, {-10, 4}}, solidArrowOptions(stroke, BG_COLOR, strokeWidth, -1, 2))
	case d2target.DiamondArrowhead:
		arrow = g.Polygon([]rough.Point{{-20, 0}, {-10, 5}, {0, 0}, {-10, -5}, {-20, 0}}, solidArrowOptions(stroke, BG_COLOR, strokeWidth, -1, 1))
	case d2target.FilledDiamondArrowhead:
		o := solidArrowOptions(stroke, stroke, strokeWidth, 4, 1)
		o.FillStyle = rough.String("zigzag")
		arrow = g.Polygon([]rough.Point{{-20, 0}, {-10, 5}, {0, 0}, {-10, -5}, {-20, 0}}, o)
	case d2target.CrossArrowhead:
		arrow = g.LinearPath([]rough.Point{{-6, -6}, {6, 6}, {0, 0}, {-6, 6}, {0, 0}, {6, -6}}, arrowOptions(stroke, strokeWidth, 3))
	case d2target.CfManyRequired:
		arrow = g.Path("M-15,-10 -15,10 M0,10 -15,0 M0,-10 -15,0", solidArrowOptions(stroke, stroke, strokeWidth, 4, 2))
	case d2target.CfMany:
		arrow = g.Path("M0,10 -15,0 M0,-10 -15,0", solidArrowOptions(stroke, stroke, strokeWidth, 4, 8))
		d := g.Circle(-20, 0, 8, solidArrowOptions(stroke, BG_COLOR, strokeWidth, 1, 4))
		extra = &d
	case d2target.CfOneRequired:
		arrow = g.Path("M-15,-10 -15,10 M-10,-10 -10,10", solidArrowOptions(stroke, stroke, strokeWidth, 4, 2))
	case d2target.CfOne:
		arrow = g.Path("M-10,-10 -10,10", solidArrowOptions(stroke, stroke, strokeWidth, 4, 3))
		d := g.Circle(-20, 0, 8, solidArrowOptions(stroke, BG_COLOR, strokeWidth, 1, 5))
		extra = &d
	case d2target.CircleArrowhead:
		arrow = g.Circle(-2, -1, 8, solidArrowOptions(stroke, BG_COLOR, strokeWidth, 1, 5))
	case d2target.BoxArrowhead:
		arrow = g.Polygon([]rough.Point{{0, -10}, {0, 10}, {-20, 10}, {-20, -10}}, solidArrowOptions(stroke, BG_COLOR, strokeWidth, -1, 1))
	case d2target.FilledBoxArrowhead:
		arrow = g.Polygon([]rough.Point{{0, -10}, {0, 10}, {-20, 10}, {-20, -10}}, solidArrowOptions(stroke, stroke, strokeWidth, -1, 1))
	default:
		return rough.Drawable{}, nil, false
	}
	return arrow, extra, true
}

func Arrowheads(connection d2target.Connection, srcAdj, dstAdj *geo.Point) (string, error) {
	arrowPaths := []string{}

	if connection.SrcArrow != d2target.NoArrowhead {
		g := newGenerator()
		arrow, extra, ok := arrowheadDrawable(g, connection.SrcArrow, connection.Stroke, connection.StrokeWidth)
		if !ok {
			return "", nil
		}

		startingSegment := geo.NewSegment(connection.Route[0], connection.Route[1])
		startingVector := startingSegment.ToVector().Reverse()
		angle := startingVector.Degrees()

		transform := fmt.Sprintf(`transform="translate(%f %f) rotate(%v)"`,
			startingSegment.Start.X+srcAdj.X, startingSegment.Start.Y+srcAdj.Y, angle,
		)

		roughPaths := computeRoughPaths(g, arrow)
		if extra != nil {
			roughPaths = append(roughPaths, computeRoughPaths(g, *extra)...)
		}

		pathEl := d2themes.NewThemableElement("path", nil)
		pathEl.ClassName = "connection"
		pathEl.Attributes = transform
		for _, rp := range roughPaths {
			pathEl.D = rp.Attrs.D
			pathEl.Fill = rp.Style.Fill
			pathEl.Stroke = rp.Style.Stroke
			pathEl.Style = rp.StyleCSS()
			arrowPaths = append(arrowPaths, pathEl.Render())
		}
	}

	if connection.DstArrow != d2target.NoArrowhead {
		g := newGenerator()
		arrow, extra, ok := arrowheadDrawable(g, connection.DstArrow, connection.Stroke, connection.StrokeWidth)
		if !ok {
			return "", nil
		}

		length := len(connection.Route)
		endingSegment := geo.NewSegment(connection.Route[length-2], connection.Route[length-1])
		endingVector := endingSegment.ToVector()
		angle := endingVector.Degrees()

		transform := fmt.Sprintf(`transform="translate(%f %f) rotate(%v)"`,
			endingSegment.End.X+dstAdj.X, endingSegment.End.Y+dstAdj.Y, angle,
		)

		roughPaths := computeRoughPaths(g, arrow)
		if extra != nil {
			roughPaths = append(roughPaths, computeRoughPaths(g, *extra)...)
		}

		pathEl := d2themes.NewThemableElement("path", nil)
		pathEl.ClassName = "connection"
		pathEl.Attributes = transform
		for _, rp := range roughPaths {
			pathEl.D = rp.Attrs.D
			pathEl.Fill = rp.Style.Fill
			pathEl.Stroke = rp.Style.Stroke
			pathEl.Style = rp.StyleCSS()
			arrowPaths = append(arrowPaths, pathEl.Render())
		}
	}

	return strings.Join(arrowPaths, " "), nil
}
