package d2sketch

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	_ "embed"

	"github.com/dop251/goja"

	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/d2themes"
	"oss.terrastruct.com/d2/lib/color"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/label"
	"oss.terrastruct.com/d2/lib/svg"
	"oss.terrastruct.com/util-go/go2"
)

//go:embed rough.js
var roughJS string

//go:embed setup.js
var setupJS string

//go:embed streaks.txt
var streaks string

type Runner goja.Runtime

var baseRoughProps = `fillWeight: 2.0,
hachureGap: 16,
fillStyle: "solid",
bowing: 2,
seed: 1,`

var floatRE = regexp.MustCompile(`(\d+)\.(\d+)`)

const (
	BG_COLOR = color.N7
	FG_COLOR = color.N1
)

func (r *Runner) run(js string) (goja.Value, error) {
	vm := (*goja.Runtime)(r)
	return vm.RunString(js)
}

func InitSketchVM() (*Runner, error) {
	vm := goja.New()
	if _, err := vm.RunString(roughJS); err != nil {
		return nil, err
	}
	if _, err := vm.RunString(setupJS); err != nil {
		return nil, err
	}
	r := Runner(*vm)
	return &r, nil
}

// DefineFillPatterns adds reusable patterns that are overlayed on shapes with
// fill. This gives it a subtle streaky effect that subtly looks hand-drawn but
// not distractingly so.
func DefineFillPatterns(buf *bytes.Buffer) {
	source := buf.String()
	fmt.Fprint(buf, "<defs>")

	defineFillPattern(buf, source, "bright", "rgba(0, 0, 0, 0.1)")
	defineFillPattern(buf, source, "normal", "rgba(0, 0, 0, 0.16)")
	defineFillPattern(buf, source, "dark", "rgba(0, 0, 0, 0.32)")
	defineFillPattern(buf, source, "darker", "rgba(255, 255, 255, 0.24)")

	fmt.Fprint(buf, "</defs>")
}

func defineFillPattern(buf *bytes.Buffer, source string, luminanceCategory, fill string) {
	trigger := fmt.Sprintf(`url(#streaks-%s)`, luminanceCategory)
	if strings.Contains(source, trigger) {
		fmt.Fprintf(buf, streaks, luminanceCategory, fill)
	}
}

func Rect(r *Runner, shape d2target.Shape) (string, error) {
	js := fmt.Sprintf(`node = rc.rectangle(0, 0, %d, %d, {
		fill: "#000",
		stroke: "#000",
		strokeWidth: %d,
		%s
	});`, shape.Width, shape.Height, shape.StrokeWidth, baseRoughProps)
	paths, err := computeRoughPathData(r, js)
	if err != nil {
		return "", err
	}
	output := ""
	pathEl := d2themes.NewThemableElement("path")
	pathEl.SetTranslate(float64(shape.Pos.X), float64(shape.Pos.Y))
	pathEl.Fill, pathEl.Stroke = d2themes.ShapeTheme(shape)
	pathEl.FillPattern = shape.FillPattern
	pathEl.ClassName = "shape"
	pathEl.Style = shape.CSSStyle()
	for _, p := range paths {
		pathEl.D = p
		output += pathEl.Render()
	}

	sketchOEl := d2themes.NewThemableElement("rect")
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

func DoubleRect(r *Runner, shape d2target.Shape) (string, error) {
	jsBigRect := fmt.Sprintf(`node = rc.rectangle(0, 0, %d, %d, {
		fill: "#000",
		stroke: "#000",
		strokeWidth: %d,
		%s
	});`, shape.Width, shape.Height, shape.StrokeWidth, baseRoughProps)
	pathsBigRect, err := computeRoughPathData(r, jsBigRect)
	if err != nil {
		return "", err
	}
	jsSmallRect := fmt.Sprintf(`node = rc.rectangle(0, 0, %d, %d, {
		fill: "#000",
		stroke: "#000",
		strokeWidth: %d,
		%s
	});`, shape.Width-d2target.INNER_BORDER_OFFSET*2, shape.Height-d2target.INNER_BORDER_OFFSET*2, shape.StrokeWidth, baseRoughProps)
	pathsSmallRect, err := computeRoughPathData(r, jsSmallRect)
	if err != nil {
		return "", err
	}

	output := ""

	pathEl := d2themes.NewThemableElement("path")
	pathEl.SetTranslate(float64(shape.Pos.X), float64(shape.Pos.Y))
	pathEl.Fill, pathEl.Stroke = d2themes.ShapeTheme(shape)
	pathEl.FillPattern = shape.FillPattern
	pathEl.ClassName = "shape"
	pathEl.Style = shape.CSSStyle()
	for _, p := range pathsBigRect {
		pathEl.D = p
		output += pathEl.Render()
	}

	pathEl = d2themes.NewThemableElement("path")
	pathEl.SetTranslate(float64(shape.Pos.X+d2target.INNER_BORDER_OFFSET), float64(shape.Pos.Y+d2target.INNER_BORDER_OFFSET))
	pathEl.Fill, pathEl.Stroke = d2themes.ShapeTheme(shape)
	// No need for inner to double paint
	pathEl.Fill = "transparent"
	pathEl.ClassName = "shape"
	pathEl.Style = shape.CSSStyle()
	for _, p := range pathsSmallRect {
		pathEl.D = p
		output += pathEl.Render()
	}

	sketchOEl := d2themes.NewThemableElement("rect")
	sketchOEl.SetTranslate(float64(shape.Pos.X), float64(shape.Pos.Y))
	sketchOEl.Width = float64(shape.Width)
	sketchOEl.Height = float64(shape.Height)
	renderedSO, err := d2themes.NewThemableSketchOverlay(sketchOEl, shape.Fill).Render()
	if err != nil {
		return "", err
	}
	output += renderedSO

	return output, nil
}

func Oval(r *Runner, shape d2target.Shape) (string, error) {
	js := fmt.Sprintf(`node = rc.ellipse(%d, %d, %d, %d, {
		fill: "#000",
		stroke: "#000",
		strokeWidth: %d,
		%s
	});`, shape.Width/2, shape.Height/2, shape.Width, shape.Height, shape.StrokeWidth, baseRoughProps)
	paths, err := computeRoughPathData(r, js)
	if err != nil {
		return "", err
	}
	output := ""
	pathEl := d2themes.NewThemableElement("path")
	pathEl.SetTranslate(float64(shape.Pos.X), float64(shape.Pos.Y))
	pathEl.Fill, pathEl.Stroke = d2themes.ShapeTheme(shape)
	pathEl.FillPattern = shape.FillPattern
	pathEl.ClassName = "shape"
	pathEl.Style = shape.CSSStyle()
	for _, p := range paths {
		pathEl.D = p
		output += pathEl.Render()
	}

	soElement := d2themes.NewThemableElement("ellipse")
	soElement.SetTranslate(float64(shape.Pos.X+shape.Width/2), float64(shape.Pos.Y+shape.Height/2))
	soElement.Rx = float64(shape.Width / 2)
	soElement.Ry = float64(shape.Height / 2)
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

func DoubleOval(r *Runner, shape d2target.Shape) (string, error) {
	jsBigCircle := fmt.Sprintf(`node = rc.ellipse(%d, %d, %d, %d, {
		fill: "#000",
		stroke: "#000",
		strokeWidth: %d,
		%s
	});`, shape.Width/2, shape.Height/2, shape.Width, shape.Height, shape.StrokeWidth, baseRoughProps)
	jsSmallCircle := fmt.Sprintf(`node = rc.ellipse(%d, %d, %d, %d, {
		fill: "#000",
		stroke: "#000",
		strokeWidth: %d,
		%s
	});`, shape.Width/2, shape.Height/2, shape.Width-d2target.INNER_BORDER_OFFSET*2, shape.Height-d2target.INNER_BORDER_OFFSET*2, shape.StrokeWidth, baseRoughProps)
	pathsBigCircle, err := computeRoughPathData(r, jsBigCircle)
	if err != nil {
		return "", err
	}
	pathsSmallCircle, err := computeRoughPathData(r, jsSmallCircle)
	if err != nil {
		return "", err
	}

	output := ""

	pathEl := d2themes.NewThemableElement("path")
	pathEl.SetTranslate(float64(shape.Pos.X), float64(shape.Pos.Y))
	pathEl.Fill, pathEl.Stroke = d2themes.ShapeTheme(shape)
	pathEl.FillPattern = shape.FillPattern
	pathEl.ClassName = "shape"
	pathEl.Style = shape.CSSStyle()
	for _, p := range pathsBigCircle {
		pathEl.D = p
		output += pathEl.Render()
	}

	pathEl = d2themes.NewThemableElement("path")
	pathEl.SetTranslate(float64(shape.Pos.X), float64(shape.Pos.Y))
	pathEl.Fill, pathEl.Stroke = d2themes.ShapeTheme(shape)
	// No need for inner to double paint
	pathEl.Fill = "transparent"
	pathEl.ClassName = "shape"
	pathEl.Style = shape.CSSStyle()
	for _, p := range pathsSmallCircle {
		pathEl.D = p
		output += pathEl.Render()
	}
	soElement := d2themes.NewThemableElement("ellipse")
	soElement.SetTranslate(float64(shape.Pos.X+shape.Width/2), float64(shape.Pos.Y+shape.Height/2))
	soElement.Rx = float64(shape.Width / 2)
	soElement.Ry = float64(shape.Height / 2)
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
func Paths(r *Runner, shape d2target.Shape, paths []string) (string, error) {
	output := ""
	for _, path := range paths {
		js := fmt.Sprintf(`node = rc.path("%s", {
		fill: "#000",
		stroke: "#000",
		strokeWidth: %d,
		%s
	});`, path, shape.StrokeWidth, baseRoughProps)
		sketchPaths, err := computeRoughPathData(r, js)
		if err != nil {
			return "", err
		}
		pathEl := d2themes.NewThemableElement("path")
		pathEl.Fill, pathEl.Stroke = d2themes.ShapeTheme(shape)
		pathEl.FillPattern = shape.FillPattern
		pathEl.ClassName = "shape"
		pathEl.Style = shape.CSSStyle()
		for _, p := range sketchPaths {
			pathEl.D = p
			output += pathEl.Render()
		}

		soElement := d2themes.NewThemableElement("path")
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

func Connection(r *Runner, connection d2target.Connection, path, attrs string) (string, error) {
	roughness := 0.5
	js := fmt.Sprintf(`node = rc.path("%s", {roughness: %f, seed: 1});`, path, roughness)
	paths, err := computeRoughPathData(r, js)
	if err != nil {
		return "", err
	}
	output := ""
	animatedClass := ""
	if connection.Animated {
		animatedClass = " animated-connection"
	}

	pathEl := d2themes.NewThemableElement("path")
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

// TODO cleanup
func Table(r *Runner, shape d2target.Shape) (string, error) {
	output := ""
	js := fmt.Sprintf(`node = rc.rectangle(0, 0, %d, %d, {
		fill: "#000",
		stroke: "#000",
		strokeWidth: %d,
		%s
	});`, shape.Width, shape.Height, shape.StrokeWidth, baseRoughProps)
	paths, err := computeRoughPathData(r, js)
	if err != nil {
		return "", err
	}
	pathEl := d2themes.NewThemableElement("path")
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

	js = fmt.Sprintf(`node = rc.rectangle(0, 0, %d, %f, {
		fill: "#000",
		%s
	});`, shape.Width, rowHeight, baseRoughProps)
	paths, err = computeRoughPathData(r, js)
	if err != nil {
		return "", err
	}
	pathEl = d2themes.NewThemableElement("path")
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

		textEl := d2themes.NewThemableElement("text")
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

		textEl := d2themes.NewThemableElement("text")
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

		js = fmt.Sprintf(`node = rc.line(%f, %f, %f, %f, {
		%s
	});`, rowBox.TopLeft.X, rowBox.TopLeft.Y, rowBox.TopLeft.X+rowBox.Width, rowBox.TopLeft.Y, baseRoughProps)
		paths, err = computeRoughPathData(r, js)
		if err != nil {
			return "", err
		}
		pathEl := d2themes.NewThemableElement("path")
		pathEl.Fill = shape.Fill
		pathEl.FillPattern = shape.FillPattern
		for _, p := range paths {
			pathEl.D = p
			output += pathEl.Render()
		}
	}

	sketchOEl := d2themes.NewThemableElement("rect")
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

func Class(r *Runner, shape d2target.Shape) (string, error) {
	output := ""
	js := fmt.Sprintf(`node = rc.rectangle(0, 0, %d, %d, {
		fill: "#000",
		stroke: "#000",
		strokeWidth: %d,
		%s
	});`, shape.Width, shape.Height, shape.StrokeWidth, baseRoughProps)
	paths, err := computeRoughPathData(r, js)
	if err != nil {
		return "", err
	}
	pathEl := d2themes.NewThemableElement("path")
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

	js = fmt.Sprintf(`node = rc.rectangle(0, 0, %d, %f, {
		fill: "#000",
		%s
	});`, shape.Width, headerBox.Height, baseRoughProps)
	paths, err = computeRoughPathData(r, js)
	if err != nil {
		return "", err
	}
	pathEl = d2themes.NewThemableElement("path")
	pathEl.SetTranslate(float64(shape.Pos.X), float64(shape.Pos.Y))
	pathEl.Fill = shape.Fill
	pathEl.FillPattern = shape.FillPattern
	pathEl.ClassName = "class_header"
	for _, p := range paths {
		pathEl.D = p
		output += pathEl.Render()
	}

	sketchOEl := d2themes.NewThemableElement("rect")
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

		textEl := d2themes.NewThemableElement("text")
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

	js = fmt.Sprintf(`node = rc.line(%f, %f, %f, %f, {
%s
	});`, rowBox.TopLeft.X, rowBox.TopLeft.Y, rowBox.TopLeft.X+rowBox.Width, rowBox.TopLeft.Y, baseRoughProps)
	paths, err = computeRoughPathData(r, js)
	if err != nil {
		return "", err
	}
	pathEl = d2themes.NewThemableElement("path")
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

	textEl := d2themes.NewThemableElement("text")
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

func computeRoughPathData(r *Runner, js string) ([]string, error) {
	if _, err := r.run(js); err != nil {
		return nil, err
	}
	roughPaths, err := extractRoughPaths(r)
	if err != nil {
		return nil, err
	}
	return extractPathData(roughPaths)
}

func computeRoughPaths(r *Runner, js string) ([]roughPath, error) {
	if _, err := r.run(js); err != nil {
		return nil, err
	}
	return extractRoughPaths(r)
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

func extractRoughPaths(r *Runner) ([]roughPath, error) {
	val, err := r.run("JSON.stringify(node.children, null, '  ')")
	if err != nil {
		return nil, err
	}

	var roughPaths []roughPath
	err = json.Unmarshal([]byte(val.String()), &roughPaths)
	if err != nil {
		return nil, err
	}

	// we want to have a fixed precision to the decimals in the path data
	for i := range roughPaths {
		// truncate all floats in path to only use up to 6 decimal places
		roughPaths[i].Attrs.D = floatRE.ReplaceAllStringFunc(roughPaths[i].Attrs.D, func(floatStr string) string {
			i := strings.Index(floatStr, ".")
			decimalLen := len(floatStr) - i - 1
			end := i + go2.Min(decimalLen, 6)
			return floatStr[:end+1]
		})
	}

	return roughPaths, nil
}

func extractPathData(roughPaths []roughPath) ([]string, error) {
	var paths []string
	for _, rp := range roughPaths {
		paths = append(paths, rp.Attrs.D)
	}
	return paths, nil
}

func ArrowheadJS(r *Runner, arrowhead d2target.Arrowhead, stroke string, strokeWidth int) (arrowJS, extraJS string) {
	// Note: selected each seed that looks the good for consistent renders
	switch arrowhead {
	case d2target.ArrowArrowhead:
		arrowJS = fmt.Sprintf(
			`node = rc.linearPath(%s, { strokeWidth: %d, stroke: "%s", seed: 3 })`,
			`[[-10, -4], [0, 0], [-10, 4]]`,
			strokeWidth,
			stroke,
		)
	case d2target.TriangleArrowhead:
		arrowJS = fmt.Sprintf(
			`node = rc.polygon(%s, { strokeWidth: %d, stroke: "%s", fill: "%s", fillStyle: "solid", seed: 2 })`,
			`[[-10, -4], [0, 0], [-10, 4]]`,
			strokeWidth,
			stroke,
			stroke,
		)
	case d2target.UnfilledTriangleArrowhead:
		arrowJS = fmt.Sprintf(
			`node = rc.polygon(%s, { strokeWidth: %d, stroke: "%s", fill: "%s", fillStyle: "solid", seed: 2 })`,
			`[[-10, -4], [0, 0], [-10, 4]]`,
			strokeWidth,
			stroke,
			BG_COLOR,
		)
	case d2target.DiamondArrowhead:
		arrowJS = fmt.Sprintf(
			`node = rc.polygon(%s, { strokeWidth: %d, stroke: "%s", fill: "%s", fillStyle: "solid", seed: 1 })`,
			`[[-20, 0], [-10, 5], [0, 0], [-10, -5], [-20, 0]]`,
			strokeWidth,
			stroke,
			BG_COLOR,
		)
	case d2target.FilledDiamondArrowhead:
		arrowJS = fmt.Sprintf(
			`node = rc.polygon(%s, { strokeWidth: %d, stroke: "%s", fill: "%s", fillStyle: "zigzag", fillWeight: 4, seed: 1 })`,
			`[[-20, 0], [-10, 5], [0, 0], [-10, -5], [-20, 0]]`,
			strokeWidth,
			stroke,
			stroke,
		)
	case d2target.CfManyRequired:
		arrowJS = fmt.Sprintf(
			// TODO why does fillStyle: "zigzag" error with path
			`node = rc.path(%s, { strokeWidth: %d, stroke: "%s", fill: "%s", fillStyle: "solid", fillWeight: 4, seed: 2 })`,
			`"M-15,-10 -15,10 M0,10 -15,0 M0,-10 -15,0"`,
			strokeWidth,
			stroke,
			stroke,
		)
	case d2target.CfMany:
		arrowJS = fmt.Sprintf(
			`node = rc.path(%s, { strokeWidth: %d, stroke: "%s", fill: "%s", fillStyle: "solid", fillWeight: 4, seed: 8 })`,
			`"M0,10 -15,0 M0,-10 -15,0"`,
			strokeWidth,
			stroke,
			stroke,
		)
		extraJS = fmt.Sprintf(
			`node = rc.circle(-20, 0, 8, { strokeWidth: %d, stroke: "%s", fill: "%s", fillStyle: "solid", fillWeight: 1, seed: 4 })`,
			strokeWidth,
			stroke,
			BG_COLOR,
		)
	case d2target.CfOneRequired:
		arrowJS = fmt.Sprintf(
			`node = rc.path(%s, { strokeWidth: %d, stroke: "%s", fill: "%s", fillStyle: "solid", fillWeight: 4, seed: 2 })`,
			`"M-15,-10 -15,10 M-10,-10 -10,10"`,
			strokeWidth,
			stroke,
			stroke,
		)
	case d2target.CfOne:
		arrowJS = fmt.Sprintf(
			`node = rc.path(%s, { strokeWidth: %d, stroke: "%s", fill: "%s", fillStyle: "solid", fillWeight: 4, seed: 3 })`,
			`"M-10,-10 -10,10"`,
			strokeWidth,
			stroke,
			stroke,
		)
		extraJS = fmt.Sprintf(
			`node = rc.circle(-20, 0, 8, { strokeWidth: %d, stroke: "%s", fill: "%s", fillStyle: "solid", fillWeight: 1, seed: 5 })`,
			strokeWidth,
			stroke,
			BG_COLOR,
		)
	case d2target.CircleArrowhead:
		arrowJS = fmt.Sprintf(
			`node = rc.circle(-2, 0, 8, { strokeWidth: %d, stroke: "%s", fill: "%s", fillStyle: "solid", fillWeight: 1, seed: 5 })`,
			strokeWidth,
			stroke,
			BG_COLOR,
		)
	}
	return
}

func Arrowheads(r *Runner, connection d2target.Connection, srcAdj, dstAdj *geo.Point) (string, error) {
	arrowPaths := []string{}

	if connection.SrcArrow != d2target.NoArrowhead {
		arrowJS, extraJS := ArrowheadJS(r, connection.SrcArrow, connection.Stroke, connection.StrokeWidth)
		if arrowJS == "" {
			return "", nil
		}

		startingSegment := geo.NewSegment(connection.Route[0], connection.Route[1])
		startingVector := startingSegment.ToVector().Reverse()
		angle := startingVector.Degrees()

		transform := fmt.Sprintf(`transform="translate(%f %f) rotate(%v)"`,
			startingSegment.Start.X+srcAdj.X, startingSegment.Start.Y+srcAdj.Y, angle,
		)

		roughPaths, err := computeRoughPaths(r, arrowJS)
		if err != nil {
			return "", err
		}
		if extraJS != "" {
			extraPaths, err := computeRoughPaths(r, extraJS)
			if err != nil {
				return "", err
			}
			roughPaths = append(roughPaths, extraPaths...)
		}

		pathEl := d2themes.NewThemableElement("path")
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
		arrowJS, extraJS := ArrowheadJS(r, connection.DstArrow, connection.Stroke, connection.StrokeWidth)
		if arrowJS == "" {
			return "", nil
		}

		length := len(connection.Route)
		endingSegment := geo.NewSegment(connection.Route[length-2], connection.Route[length-1])
		endingVector := endingSegment.ToVector()
		angle := endingVector.Degrees()

		transform := fmt.Sprintf(`transform="translate(%f %f) rotate(%v)"`,
			endingSegment.End.X+dstAdj.X, endingSegment.End.Y+dstAdj.Y, angle,
		)

		roughPaths, err := computeRoughPaths(r, arrowJS)
		if err != nil {
			return "", err
		}
		if extraJS != "" {
			extraPaths, err := computeRoughPaths(r, extraJS)
			if err != nil {
				return "", err
			}
			roughPaths = append(roughPaths, extraPaths...)
		}

		pathEl := d2themes.NewThemableElement("path")
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
